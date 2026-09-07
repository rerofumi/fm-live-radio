//go:build windows

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWDDMScriptValidatesRawPIDAndLUIDBeforeAggregation(t *testing.T) {
	script := wddmProcessMemoryPowerShellScript(gpuProcessMemoryTarget{PID: 26, AdapterLUID: "luid_0x00000000_0x000185a3"})
	for _, want := range []string{"$namePattern", "$name -match $namePattern", "duplicate WDDM physical instance"} {
		if !strings.Contains(script, want) {
			t.Fatalf("WDDM script missing strict identity check %q", want)
		}
	}
	if strings.Contains(script, `$name = "pid_${targetPid}_${targetLuid}"`) {
		t.Fatal("WDDM script must not relabel wildcard candidates to target PID")
	}
}

func TestWDDMProcessMemoryStreamExplicitStopOwnsChildTermination(t *testing.T) {
	for _, delay := range []time.Duration{0, 30 * time.Millisecond} {
		t.Run(fmt.Sprint(delay), func(t *testing.T) {
			target := gpuProcessMemoryTarget{
				PID:         2147483647,
				AdapterLUID: "luid_0x1_0x2",
				Interval:    100 * time.Millisecond,
			}
			collector := newWDDMProcessMemoryCollectorWithFactory(target, func(ctx context.Context, target gpuProcessMemoryTarget) (gpuProcessMemoryStream, error) {
				stream, err := newWDDMProcessMemoryStream(ctx, target)
				if err != nil {
					return stream, err
				}
				originalStop := stream.stop
				stream.stop = func() error {
					time.Sleep(delay)
					return originalStop()
				}
				return stream, nil
			})
			if err := collector.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { collector.Stop() })
			// Keep this as a controlled lifecycle assertion; no GPU sample is
			// required to verify ownership of the real PowerShell child.
			collector.recordSample(gpuProcessMemorySample{
				PID:            target.PID,
				AdapterLUID:    target.AdapterLUID,
				DedicatedBytes: 1 << 20,
				ObservedAt:     time.Now(),
			})
			observation := collector.Stop()
			t.Logf("native planned Stop with stop scheduling delay %s: %+v", delay, observation)
			if !observation.Available {
				t.Fatalf("explicit Stop owner must complete planned child shutdown: %+v", observation)
			}
		})
	}
}
