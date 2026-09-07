//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type wddmLUIDLookup struct {
	luid string
	err  error
}

var (
	wddmLUIDMu    sync.Mutex
	wddmLUIDCache = map[string]wddmLUIDLookup{}
)

func newWDDMProcessMemoryStream(_ context.Context, target gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
	script := wddmProcessMemoryPowerShellScript(target)
	// Keep process termination under the stream's explicit stop owner. Using
	// CommandContext here adds a second Kill owner: collector.Stop cancels the
	// context before calling stream.stop, so the CommandContext watcher can win
	// the race and make the explicit Kill report ERROR_ACCESS_DENIED. The
	// The collector still owns polling shutdown and Stop owns child OS
	// termination, recording its successful Kill before Wait.
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return gpuProcessMemoryStream{}, fmt.Errorf("WDDM process memory stdout: %w", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return gpuProcessMemoryStream{}, fmt.Errorf("start persistent WDDM process memory reader: %w", err)
	}
	var killed atomic.Bool
	return gpuProcessMemoryStream{
		reader: stdout,
		plannedStopWaitErrorAllowed: func(waitErr error) bool {
			var exitErr *exec.ExitError
			return killed.Load() && errors.As(waitErr, &exitErr)
		},
		stop: func() error {
			if cmd.Process == nil {
				return errors.New("WDDM process memory child has no process handle")
			}
			err := cmd.Process.Kill()
			if err != nil {
				return err
			}
			killed.Store(true)
			return nil
		},
		wait: cmd.Wait,
	}, nil
}

// queryWDDMAdapterLUID enumerates DXGI once per nvidia-smi adapter index. The
// returned LUID is the key used by the WDDM GPUProcessMemory counter names.
func queryWDDMAdapterLUID(index string) (string, error) {
	index = strings.TrimSpace(index)
	wddmLUIDMu.Lock()
	if cached, ok := wddmLUIDCache[index]; ok {
		wddmLUIDMu.Unlock()
		return cached.luid, cached.err
	}
	wddmLUIDMu.Unlock()
	out, err := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", dxgiAdapterEnumerationScript).CombinedOutput()
	if err != nil {
		err = fmt.Errorf("DXGI adapter enumeration: %w (%s)", err, strings.TrimSpace(string(out)))
		cacheWDDMLUID(index, wddmLUIDLookup{err: err})
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 2 || strings.TrimSpace(fields[0]) != index {
			continue
		}
		luid := normalizeAdapterLUID(fields[1])
		if luid == "" {
			break
		}
		cacheWDDMLUID(index, wddmLUIDLookup{luid: luid})
		return luid, nil
	}
	err = fmt.Errorf("NVIDIA DXGI adapter index %q has no LUID", index)
	cacheWDDMLUID(index, wddmLUIDLookup{err: err})
	return "", err
}

func cacheWDDMLUID(index string, lookup wddmLUIDLookup) {
	wddmLUIDMu.Lock()
	wddmLUIDCache[index] = lookup
	wddmLUIDMu.Unlock()
}

const dxgiAdapterEnumerationScript = `Add-Type -TypeDefinition @"
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
public static class FMRadioDxgi {
 [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
 public struct Desc { [MarshalAs(UnmanagedType.ByValTStr, SizeConst=128)] public string Description; public uint VendorId,DeviceId,SubSysId,Revision; public UIntPtr DedicatedVideoMemory,DedicatedSystemMemory,SharedSystemMemory; public uint LuidLow; public int LuidHigh; }
 [DllImport("dxgi.dll")] static extern int CreateDXGIFactory(ref Guid iid, out IntPtr factory);
 [UnmanagedFunctionPointer(CallingConvention.StdCall)] delegate int EnumAdapters(IntPtr factory, uint index, out IntPtr adapter);
 [UnmanagedFunctionPointer(CallingConvention.StdCall)] delegate int GetDesc(IntPtr adapter, out Desc desc);
 public static Desc[] Read() {
  var iid = new Guid("7b7166ec-21c7-44ae-b21a-c9ae321ae369"); IntPtr factory;
  Marshal.ThrowExceptionForHR(CreateDXGIFactory(ref iid, out factory)); var list = new List<Desc>();
  try { var ev = Marshal.GetDelegateForFunctionPointer<EnumAdapters>(Marshal.ReadIntPtr(Marshal.ReadIntPtr(factory), 7*IntPtr.Size));
   for (uint i=0;;i++) { IntPtr adapter; int hr=ev(factory,i,out adapter); if (hr==unchecked((int)0x887A0002)) break; Marshal.ThrowExceptionForHR(hr);
    try { var gd=Marshal.GetDelegateForFunctionPointer<GetDesc>(Marshal.ReadIntPtr(Marshal.ReadIntPtr(adapter), 8*IntPtr.Size)); Desc d; Marshal.ThrowExceptionForHR(gd(adapter,out d)); list.Add(d); }
    finally { Marshal.Release(adapter); }
   }
  } finally { Marshal.Release(factory); }
  return list.ToArray();
 }
}
"@
$nvidiaOrdinal = 0
[FMRadioDxgi]::Read() | ForEach-Object {
  if ($_.VendorId -eq 4318) {
    [string]::Join([char]9, @($nvidiaOrdinal, ('luid_0x{0:X8}_0x{1:X8}' -f ([uint32]$_.LuidHigh),([uint32]$_.LuidLow))))
    $nvidiaOrdinal++
  }
}`

func wddmProcessMemoryPowerShellScript(target gpuProcessMemoryTarget) string {
	// PID and interval are numeric. The LUID is restricted to the DXGI token
	// shape before it reaches this script, and single quotes are still escaped
	// for defense in depth.
	pid := strconv.Itoa(target.PID)
	interval := strconv.FormatInt(target.Interval.Milliseconds(), 10)
	luid := strings.ReplaceAll(normalizeAdapterLUID(target.AdapterLUID), "'", "''")
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$targetPid = %s
$targetLuid = '%s'
$intervalMs = %s
$filter = "Name LIKE 'pid_${targetPid}_%%'"
$namePattern = '^pid_([0-9]+)_(luid_0x[0-9a-f]+_0x[0-9a-f]+)_phys_([0-9]+)$'
while ($true) {
  $started = [Diagnostics.Stopwatch]::StartNew()
  try {
    $rows = @(Get-CimInstance -ClassName Win32_PerfFormattedData_GPUPerformanceCounters_GPUProcessMemory -Filter $filter | Select-Object Name,DedicatedUsage,SharedUsage,TotalCommitted)
    $matching = @()
    $physical = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    foreach ($row in $rows) {
      $name = [string]$row.Name
      if (-not ($name -match $namePattern)) { continue }
      if ([int64]$matches[1] -ne $targetPid) { continue }
      if (-not ([string]$matches[2]).Equals($targetLuid, [StringComparison]::OrdinalIgnoreCase)) { continue }
      if (-not $physical.Add([string]$matches[3])) { throw "duplicate WDDM physical instance: $name" }
      $matching += $row
    }
    if ($matching.Count -gt 0) {
      $dedicated = [Convert]::ToUInt64(($matching | Measure-Object DedicatedUsage -Sum).Sum)
      $shared = [Convert]::ToUInt64(($matching | Measure-Object SharedUsage -Sum).Sum)
      $committed = [Convert]::ToUInt64(($matching | Measure-Object TotalCommitted -Sum).Sum)
      # Keep the raw source identity in the emitted record. PID/LUID were
      # checked above; do not synthesize a pid_26 alias from LIKE results.
      $name = [string]$matching[0].Name
      [Console]::WriteLine([string]::Join([char]9, @("sample",$name,$dedicated,$shared,$committed)))
    } else {
      [Console]::WriteLine([string]::Join([char]9, @("empty","no matching WDDM process instance")))
    }
  } catch {
    [Console]::WriteLine([string]::Join([char]9, @("error",$_.Exception.Message)))
  }
  [Console]::Out.Flush()
  $remaining = $intervalMs - [int]$started.ElapsedMilliseconds
  if ($remaining -gt 0) { Start-Sleep -Milliseconds $remaining }
}`, pid, luid, interval)
}
