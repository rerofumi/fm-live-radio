package main

// This file contains the platform-neutral part of the WDDM process-memory
// collector.  The Windows reader is deliberately kept in
// gpu_process_memory_windows.go so the benchmark remains buildable on other
// platforms without a WMI/PowerShell dependency.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultGPUProcessMemoryInterval = 100 * time.Millisecond

type gpuProcessMemoryTarget struct {
	PID         int
	GPUIndex    string
	AdapterLUID string
	GPUUUID     string
	Interval    time.Duration
}

type gpuProcessMemorySample struct {
	PID                 int
	AdapterLUID         string
	DedicatedBytes      uint64
	SharedBytes         uint64
	TotalCommittedBytes uint64
	ObservedAt          time.Time
}

type gpuProcessMemoryObservation struct {
	PID            int
	GPUIndex       string
	AdapterLUID    string
	GPUUUID        string
	Source         string
	Available      bool
	Reason         string
	SampleCount    int
	FirstSampleUTC time.Time
	LastSampleUTC  time.Time
	PeakDedicated  uint64
}

type gpuProcessMemoryStream struct {
	reader io.ReadCloser
	stop   func() error
	wait   func() error
	// plannedStopWaitErrorAllowed is set only by a stream implementation that
	// can prove the child was killed successfully by stop.  The old boolean is
	// retained for source compatibility with focused probes, but is not trusted
	// as evidence on its own.
	plannedStopWaitErrorAllowed func(error) bool
	plannedStopWaitErrorOK      bool
}

type gpuProcessMemoryStreamFactory func(context.Context, gpuProcessMemoryTarget) (gpuProcessMemoryStream, error)

type wddmProcessMemoryCollector struct {
	target  gpuProcessMemoryTarget
	factory gpuProcessMemoryStreamFactory

	mu               sync.Mutex
	started          bool
	samples          []gpuProcessMemorySample
	cancel           context.CancelFunc
	stream           gpuProcessMemoryStream
	consumeW         sync.WaitGroup
	stopRequested    bool
	stopped          bool
	readerDone       bool
	unexpectedEOF    bool
	fatalReasons     []string
	gapReasons       []string
	initialGaps      []string
	integrityReasons []string
}

func newWDDMProcessMemoryCollector(target gpuProcessMemoryTarget) *wddmProcessMemoryCollector {
	if target.Interval <= 0 {
		target.Interval = defaultGPUProcessMemoryInterval
	}
	target.AdapterLUID = normalizeAdapterLUID(target.AdapterLUID)
	return &wddmProcessMemoryCollector{target: target, factory: newWDDMProcessMemoryStream}
}

// newWDDMProcessMemoryCollectorWithFactory is used by focused lifecycle tests
// and keeps process ownership testable without starting PowerShell.
func newWDDMProcessMemoryCollectorWithFactory(target gpuProcessMemoryTarget, factory gpuProcessMemoryStreamFactory) *wddmProcessMemoryCollector {
	collector := newWDDMProcessMemoryCollector(target)
	if factory != nil {
		collector.factory = factory
	}
	return collector
}

func (c *wddmProcessMemoryCollector) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("GPU process memory collector already started")
	}
	if c.target.PID <= 0 {
		return fmt.Errorf("GPU process memory collector requires a positive PID")
	}
	if c.target.AdapterLUID == "" {
		return fmt.Errorf("GPU process memory collector requires an adapter LUID")
	}
	if c.factory == nil {
		return fmt.Errorf("GPU process memory collector has no stream factory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.factory(ctx, c.target)
	if err != nil {
		cancel()
		return err
	}
	if stream.reader == nil {
		cancel()
		return fmt.Errorf("GPU process memory stream has no reader")
	}
	c.cancel = cancel
	c.stream = stream
	c.started = true
	c.consumeW.Add(1)
	go c.consume(stream.reader)
	return nil
}

func (c *wddmProcessMemoryCollector) consume(reader io.Reader) {
	defer c.consumeW.Done()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "error\t") {
			c.recordFatal(strings.TrimSpace(strings.TrimPrefix(line, "error\t")))
			continue
		}
		if strings.HasPrefix(line, "empty\t") {
			c.recordGap(strings.TrimSpace(strings.TrimPrefix(line, "empty\t")))
			continue
		}
		sample, ok, reason := parseWDDMProcessMemoryLine(line, c.target.PID, c.target.AdapterLUID)
		if !ok {
			c.recordFatal(reason)
			continue
		}
		// Keep the monotonic component supplied by time.Now.  UTC conversion is
		// reserved for serialization, because UTC() strips monotonic timing.
		sample.ObservedAt = time.Now()
		c.recordSample(sample)
	}
	c.mu.Lock()
	stopping := c.stopRequested
	c.readerDone = true
	c.mu.Unlock()
	if err := scanner.Err(); err != nil {
		// A reader error is never made benign by a planned stop.  A successful
		// child kill normally produces EOF; an actual read error is corruption.
		c.recordFatal("WDDM process memory reader: " + err.Error())
	} else if !stopping {
		c.mu.Lock()
		c.unexpectedEOF = true
		c.mu.Unlock()
		c.recordFatal("WDDM process memory reader ended unexpectedly")
	}
}

// Stop closes the reader, joins the child process, and then joins the scanner
// goroutine.  The order matters: Wait alone can leave a blocked scanner, while
// a goroutine-only wait can leave a PowerShell child behind.
func (c *wddmProcessMemoryCollector) Stop() gpuProcessMemoryObservation {
	c.mu.Lock()
	if !c.started {
		obs := c.observationLocked()
		c.mu.Unlock()
		return obs
	}
	c.started = false
	c.stopRequested = true
	cancel := c.cancel
	stream := c.stream
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	stopErr := error(nil)
	if stream.stop != nil {
		stopErr = stream.stop()
		if stopErr != nil {
			c.recordFatal("WDDM process memory planned stop: " + stopErr.Error())
		}
	}
	if stream.wait != nil {
		if waitErr := stream.wait(); waitErr != nil {
			allowed := false
			if stream.plannedStopWaitErrorAllowed != nil {
				allowed = stream.plannedStopWaitErrorAllowed(waitErr)
			}
			if !allowed {
				c.recordFatal("WDDM process memory child wait: " + waitErr.Error())
			}
		}
	}
	c.consumeW.Wait()

	c.mu.Lock()
	c.stopped = true
	defer c.mu.Unlock()
	return c.observationLocked()
}

func (c *wddmProcessMemoryCollector) Snapshot() gpuProcessMemoryObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observationLocked()
}

func (c *wddmProcessMemoryCollector) recordFatal(reason string) {
	if reason == "" {
		return
	}
	c.mu.Lock()
	c.fatalReasons = append(c.fatalReasons, reason)
	c.mu.Unlock()
}

func (c *wddmProcessMemoryCollector) recordGap(reason string) {
	if reason == "" {
		return
	}
	c.mu.Lock()
	if len(c.samples) == 0 {
		c.initialGaps = append(c.initialGaps, reason)
	} else {
		c.gapReasons = append(c.gapReasons, reason)
	}
	c.mu.Unlock()
}

func (c *wddmProcessMemoryCollector) recordSample(sample gpuProcessMemorySample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sample.ObservedAt.IsZero() {
		c.integrityReasons = appendUniqueReason(c.integrityReasons, "sample timestamp is zero")
	}
	if since := time.Since(sample.ObservedAt); since < 0 {
		c.integrityReasons = appendUniqueReason(c.integrityReasons, "sample timestamp is in the future")
	}
	if len(c.samples) > 0 {
		previous := c.samples[len(c.samples)-1].ObservedAt
		delta := sample.ObservedAt.Sub(previous)
		if delta < 0 {
			c.integrityReasons = appendUniqueReason(c.integrityReasons, fmt.Sprintf("sample timestamp moved backwards (%s)", delta.Round(time.Millisecond)))
		} else if delta > c.sampleFreshnessLimit() {
			c.integrityReasons = appendUniqueReason(c.integrityReasons, fmt.Sprintf("sample gap exceeded freshness limit (%s)", delta.Round(time.Millisecond)))
		}
	}
	c.samples = append(c.samples, sample)
}

func (c *wddmProcessMemoryCollector) sampleFreshnessLimit() time.Duration {
	freshness := c.target.Interval * 3
	if freshness < 250*time.Millisecond {
		freshness = 250 * time.Millisecond
	}
	return freshness
}

func (c *wddmProcessMemoryCollector) observationLocked() gpuProcessMemoryObservation {
	return c.observationForSamplesLocked(c.samples, "no matching WDDM process sample")
}

// snapshotSince returns only samples observed after start. The collector's
// health state remains global, so an error or gap in any part of a Talk cannot
// be hidden by selecting a seemingly healthy interval.
func (c *wddmProcessMemoryCollector) snapshotSince(start int) (gpuProcessMemoryObservation, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if start < 0 || start > len(c.samples) {
		start = len(c.samples)
	}
	return c.observationForSamplesLocked(c.samples[start:], "no WDDM process sample in interval"), len(c.samples)
}

func (c *wddmProcessMemoryCollector) observationForSamplesLocked(samples []gpuProcessMemorySample, emptyReason string) gpuProcessMemoryObservation {
	obs := gpuProcessMemoryObservation{
		PID:         c.target.PID,
		GPUIndex:    c.target.GPUIndex,
		AdapterLUID: c.target.AdapterLUID,
		GPUUUID:     c.target.GPUUUID,
		Source:      "Win32_PerfFormattedData_GPUPerformanceCounters_GPUProcessMemory.DedicatedUsage",
	}
	obs.SampleCount = len(samples)
	if len(samples) > 0 {
		obs.FirstSampleUTC = samples[0].ObservedAt.UTC()
		obs.LastSampleUTC = samples[0].ObservedAt.UTC()
		for _, sample := range samples {
			if sample.DedicatedBytes > obs.PeakDedicated {
				obs.PeakDedicated = sample.DedicatedBytes
			}
			if sample.ObservedAt.Before(samples[0].ObservedAt) {
				obs.FirstSampleUTC = sample.ObservedAt.UTC()
			}
			if sample.ObservedAt.After(samples[0].ObservedAt) {
				obs.LastSampleUTC = sample.ObservedAt.UTC()
			}
		}
		obs.Reason = fmt.Sprintf("samples=%d; dedicated_usage_bytes=%d", obs.SampleCount, obs.PeakDedicated)
	} else {
		obs.Reason = emptyReason
	}
	if len(c.fatalReasons) > 0 {
		obs.Reason += "; " + strings.Join(uniqueStrings(c.fatalReasons), "; ")
	}
	if len(c.gapReasons) > 0 {
		obs.Reason += "; gaps=" + strings.Join(uniqueStrings(c.gapReasons), "; ")
	}
	if len(c.initialGaps) > 0 {
		obs.Reason += "; initial_gaps=" + strings.Join(uniqueStrings(c.initialGaps), "; ")
	}
	if c.unexpectedEOF {
		obs.Reason += "; unexpected reader EOF"
	}
	if c.started && c.readerDone {
		obs.Reason += "; reader is no longer alive"
	}
	// A planned Stop makes reader death expected only after the stream has been
	// stopped and joined. During an active interval, reader liveness is part of
	// measurement integrity.
	healthy := len(samples) > 0 && len(c.fatalReasons) == 0 && len(c.integrityReasons) == 0 && !c.unexpectedEOF
	if len(c.gapReasons) > 0 && len(c.samples) > 0 {
		healthy = false
	}
	if c.started && c.readerDone {
		healthy = false
	}
	if len(c.samples) > 0 {
		for i, sample := range c.samples {
			if time.Since(sample.ObservedAt) < 0 {
				c.integrityReasons = appendUniqueReason(c.integrityReasons, "sample timestamp is in the future")
				healthy = false
			}
			if i > 0 {
				delta := sample.ObservedAt.Sub(c.samples[i-1].ObservedAt)
				if delta < 0 {
					c.integrityReasons = appendUniqueReason(c.integrityReasons, fmt.Sprintf("sample timestamp moved backwards (%s)", delta.Round(time.Millisecond)))
					healthy = false
				}
			}
		}
		last := c.samples[len(c.samples)-1].ObservedAt
		since := time.Since(last)
		if since < 0 {
			c.integrityReasons = appendUniqueReason(c.integrityReasons, "sample timestamp is in the future")
			healthy = false
		} else if since > c.sampleFreshnessLimit() {
			c.integrityReasons = appendUniqueReason(c.integrityReasons, fmt.Sprintf("sample became stale (%s)", since.Round(time.Millisecond)))
			healthy = false
		}
	}
	if len(c.integrityReasons) > 0 {
		obs.Reason += "; integrity=" + strings.Join(uniqueStrings(c.integrityReasons), "; ")
	}
	if healthy {
		obs.Available = true
		return obs
	}
	return obs
}

func appendUniqueReason(values []string, reason string) []string {
	for _, value := range values {
		if value == reason {
			return values
		}
	}
	return append(values, reason)
}

var wddmProcessMemoryNamePattern = regexp.MustCompile(`(?i)^pid_([0-9]+)_(luid_0x[0-9a-f]+_0x[0-9a-f]+)_phys_([0-9]+)$`)

func parseWDDMProcessMemoryLine(line string, expectedPID int, expectedLUID string) (gpuProcessMemorySample, bool, string) {
	fields := strings.Split(line, "\t")
	if len(fields) < 5 || strings.TrimSpace(fields[0]) != "sample" {
		return gpuProcessMemorySample{}, false, "malformed WDDM process memory sample"
	}
	name := strings.TrimSpace(fields[1])
	if !wddmProcessMemoryNameMatches(name, expectedPID, expectedLUID) {
		return gpuProcessMemorySample{}, false, "WDDM process memory sample PID/LUID does not match target"
	}
	values := make([]uint64, 3)
	for i := range values {
		value := strings.TrimSpace(fields[i+2])
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return gpuProcessMemorySample{}, false, fmt.Sprintf("invalid WDDM %s value %q", []string{"DedicatedUsage", "SharedUsage", "TotalCommitted"}[i], value)
		}
		values[i] = parsed
	}
	return gpuProcessMemorySample{
		PID:                 expectedPID,
		AdapterLUID:         normalizeAdapterLUID(expectedLUID),
		DedicatedBytes:      values[0],
		SharedBytes:         values[1],
		TotalCommittedBytes: values[2],
	}, true, ""
}

func wddmProcessMemoryNameMatches(name string, expectedPID int, expectedLUID string) bool {
	if expectedPID <= 0 || normalizeAdapterLUID(expectedLUID) == "" {
		return false
	}
	matches := wddmProcessMemoryNamePattern.FindStringSubmatch(strings.TrimSpace(name))
	if len(matches) != 4 {
		return false
	}
	pid, err := strconv.Atoi(matches[1])
	if err != nil || pid != expectedPID {
		return false
	}
	return strings.EqualFold(normalizeAdapterLUID(matches[2]), normalizeAdapterLUID(expectedLUID))
}

func normalizeAdapterLUID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, " ", "")
	if !strings.HasPrefix(strings.ToLower(value), "luid_") {
		value = "luid_" + value
	}
	return strings.ToLower(value)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
