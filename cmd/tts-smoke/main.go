// tts-smoke is an explicit, process-scoped v4 service smoke harness. It is
// intentionally separate from the parity command so EP, seed, model path and
// expected abnormal conditions are visible in every invocation.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts"
	"fm-live-radio/internal/localtts/irodori/metadata"
	"fm-live-radio/internal/localtts/irodori/pipeline"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	model := flag.String("model", "model/irodori-v4.1", "model directory")
	ep := flag.String("ep", "cpu", "cpu, cuda, or auto")
	seed := flag.Uint("seed", 0, "synthesis seed")
	text := flag.String("text", "こんにちは。これは複数文のスモークです。", "synthesis text")
	ref := flag.String("ref", "narrator/narrator_01.wav", "reference WAV; empty for null speaker")
	out := flag.String("out", filepath.Join(os.TempDir(), "irodori-tts-smoke.wav"), "output WAV")
	steps := flag.Int("steps", 2, "denoising steps")
	seconds := flag.Float64("seconds", 0.5, "requested output seconds")
	durationScale := flag.Float64("duration-scale", 1, "duration predictor scale")
	cancelAfter := flag.Duration("cancel-after", 0, "when non-zero, cancel at the real inference-start event; value is kept for CLI compatibility")
	expectError := flag.Bool("expect-error", false, "treat only the requested inference/cancellation error as expected")
	serviceMode := flag.Bool("service", false, "run through localtts.Service to exercise sentence splitting/runtime reuse")
	flag.Parse()
	log := &eventLog{}

	if *ep != "cpu" && *ep != "cuda" && *ep != "auto" {
		return finish(false, smokeErrorOperational, fmt.Errorf("unsupported execution provider %q", *ep))
	}
	if *durationScale <= 0 || *seconds < 0 || *steps <= 0 {
		return finish(false, smokeErrorOperational, fmt.Errorf("duration-scale, seconds, and steps must be positive (seconds may be zero for predictor mode)"))
	}
	// A cancellation requested at the inference-start event must not turn a
	// known-invalid input into an expected cancellation.  The v4 normalizer
	// strips semicolons, so a semicolon-only text is an empty synthesis request.
	if strings.Trim(strings.TrimSpace(*text), ";") == "" {
		return finish(false, smokeErrorOperational, fmt.Errorf("synthesis text is empty after normalization"))
	}
	if *serviceMode {
		lib := generation.ResolveORTLibraryPathForEP(*ep)
		if lib == "" {
			return finish(false, smokeErrorOperational, fmt.Errorf("matching %s ORT DLL not found", *ep))
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var cancelOnce sync.Once
		observer := func(event pipeline.RuntimeEvent) {
			log.add(event)
			if *cancelAfter > 0 && event == pipeline.EventInferenceStart {
				cancelOnce.Do(cancel)
			}
		}
		cfg := domain.AppConfig{Irodori: domain.IrodoriConfig{
			ModelDir: *model, RefWAV: *ref, Seconds: *seconds, NumSteps: *steps,
			SeedMode: "fixed", FixedSeed: uint32(*seed), CfgText: 3, CfgCaption: 3,
			CfgSpeaker: 5, DurationScale: *durationScale,
		}, LocalInference: domain.LocalInferenceConfig{ORTLibraryPath: lib, ExecutionProvider: *ep}}
		loadBefore := pipeline.RuntimeLoadCount()
		closeBefore := pipeline.RuntimeCloseCount()
		beforeTemp, tempErr := tempWAVSet()
		if tempErr != nil {
			return finish(false, smokeErrorInvariant, fmt.Errorf("temp WAV snapshot: %w", tempErr))
		}
		_ = os.Remove(*out)
		svc := localtts.New()
		wav, err := svc.SynthesizeWavWithObserver(ctx, cfg, *text, observer)
		if err != nil {
			_ = os.Remove(*out)
			afterTemp, snapErr := tempWAVSet()
			if snapErr != nil {
				return finish(false, smokeErrorInvariant, fmt.Errorf("temp WAV snapshot: %w", snapErr))
			}
			if ctx.Err() != nil && !sameTempSet(beforeTemp, afterTemp) {
				return finish(false, smokeErrorInvariant, fmt.Errorf("service cancellation left temporary WAV (events=%v)", log.events()))
			}
			if ctx.Err() != nil && !hasLifecycle(log.events()) {
				return finish(false, smokeErrorInvariant, fmt.Errorf("service cancellation lifecycle incomplete (events=%v)", log.events()))
			}
			if ctx.Err() != nil {
				if err := requireCounts(loadBefore, closeBefore, 1, 1); err != nil {
					return finish(false, smokeErrorInvariant, err)
				}
				// Prove that the same Service/process remains usable after the
				// joined cancellation and Runtime.Close sequence.
				regenCtx := context.Background()
				regen, regenErr := svc.SynthesizeWavWithObserver(regenCtx, cfg, *text, observer)
				if regenErr != nil || len(regen) == 0 {
					return finish(false, smokeErrorInvariant, fmt.Errorf("service regeneration after cancel: %v (events=%v)", regenErr, log.events()))
				}
				if err := requireCounts(loadBefore, closeBefore, 2, 2); err != nil {
					return finish(false, smokeErrorInvariant, err)
				}
				if _, statErr := os.Stat(*out); !os.IsNotExist(statErr) {
					return finish(false, smokeErrorInvariant, fmt.Errorf("service cancellation left output %q", *out))
				}
				afterTemp, snapErr = tempWAVSet()
				if snapErr != nil || !sameTempSet(beforeTemp, afterTemp) {
					return finish(false, smokeErrorInvariant, fmt.Errorf("service regeneration left temporary WAV (events=%v)", log.events()))
				}
				return finish(*expectError, smokeErrorCancellation, fmt.Errorf("service: %w (events=%v, load_count=%d, close_count=%d)", err, log.events(), pipeline.RuntimeLoadCount()-loadBefore, pipeline.RuntimeCloseCount()-closeBefore))
			}
			if !hasEvent(log.events(), pipeline.EventLoadStart) || !hasEvent(log.events(), pipeline.EventLoaded) {
				return finish(false, smokeErrorOperational, fmt.Errorf("service preflight/load: %w", err))
			}
			return finish(*expectError, smokeErrorInference, fmt.Errorf("service: %w (events=%v, load_count=%d, close_count=%d)", err, log.events(), pipeline.RuntimeLoadCount()-loadBefore, pipeline.RuntimeCloseCount()-closeBefore))
		}
		if ctx.Err() != nil {
			_ = os.Remove(*out)
			return finish(false, smokeErrorInvariant, fmt.Errorf("service completed after cancellation (events=%v)", log.events()))
		}
		if err := os.WriteFile(*out, wav, 0600); err != nil {
			return finish(false, smokeErrorInvariant, fmt.Errorf("service output: %w", err))
		}
		if err := requireCounts(loadBefore, closeBefore, 1, 1); err != nil {
			return finish(false, smokeErrorInvariant, err)
		}
		afterTemp, snapErr := tempWAVSet()
		if snapErr != nil || !hasLifecycle(log.events()) || !sameTempSet(beforeTemp, afterTemp) {
			return finish(false, smokeErrorInvariant, fmt.Errorf("service success lifecycle or temporary WAV invariant failed (events=%v)", log.events()))
		}
		if *expectError {
			return finish(false, smokeErrorInvariant, fmt.Errorf("expected an error but service smoke succeeded"))
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"model": *model, "execution_provider": *ep, "seed": uint32(*seed),
			"output": *out, "service": true, "cancel_after": cancelAfter.String(),
			"cancel_requested": ctx.Err() != nil, "events": log.events(),
			"load_count":  pipeline.RuntimeLoadCount() - loadBefore,
			"close_count": pipeline.RuntimeCloseCount() - closeBefore,
		})
		return nil
	}

	// The service performs this same check before ORT initialisation. Keeping it
	// here makes abnormal preflight cases independently runnable in a process.
	log.add(pipeline.EventPreflightStart)
	if _, err := os.Stat(filepath.Join(*model, "manifest.json")); err == nil {
		m, err := metadata.LoadManifest(*model)
		if err == nil {
			err = m.VerifyHashes(*model)
		}
		if err != nil {
			return finish(false, smokeErrorOperational, fmt.Errorf("v4 preflight: %w", err))
		}
	}
	log.add(pipeline.EventPreflightEnd)

	lib := generation.ResolveORTLibraryPathForEP(*ep)
	if lib == "" {
		return finish(false, smokeErrorOperational, fmt.Errorf("matching %s ORT DLL not found", *ep))
	}
	if err := generation.ConfigureExecutionProvider(*ep, 0); err != nil {
		return finish(false, smokeErrorOperational, err)
	}
	if err := generation.Init(lib); err != nil {
		return finish(false, smokeErrorOperational, err)
	}

	o := pipeline.DefaultOptions()
	o.ModelDir, o.Text, o.RefWAV = *model, *text, *ref
	o.OutputWAV, o.NumSteps, o.Seconds = *out, *steps, *seconds
	o.Seed, o.DurationScale = uint32(*seed), *durationScale
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var cancelOnce sync.Once
	observer := func(event pipeline.RuntimeEvent) {
		log.add(event)
		if *cancelAfter > 0 && event == pipeline.EventInferenceStart {
			cancelOnce.Do(cancel)
		}
	}
	o.Observer = observer
	loadBefore := pipeline.RuntimeLoadCount()
	closeBefore := pipeline.RuntimeCloseCount()
	beforeTemp, tempErr := tempWAVSet()
	if tempErr != nil {
		return finish(false, smokeErrorInvariant, fmt.Errorf("temp WAV snapshot: %w", tempErr))
	}
	_ = os.Remove(*out)
	log.add(pipeline.EventLoadStart)
	rt, err := pipeline.LoadInitialise(o)
	log.add(pipeline.EventLoadEnd)
	if err != nil {
		return finish(false, smokeErrorOperational, fmt.Errorf("runtime load: %w", err))
	}
	defer rt.Close()

	done := make(chan error, 1)
	go func() { done <- rt.SynthesizeWithOptions(o) }()
	var synthErr error
	joined := false
	if *cancelAfter > 0 {
		select {
		case synthErr = <-done:
			joined = true
		case <-ctx.Done():
			// No ORT abort API is assumed. Always join before Runtime.Close.
			synthErr = <-done
			joined = true
		}
	} else {
		synthErr = <-done
		joined = true
	}
	log.add(pipeline.EventInferenceJoined)
	if ctx.Err() != nil {
		_ = os.Remove(*out)
		rt.Close()
		if err := checkDirectInvariants(beforeTemp, *out, log.events(), joined, loadBefore, closeBefore); err != nil {
			return finish(false, smokeErrorInvariant, err)
		}
		return finish(*expectError, smokeErrorCancellation, fmt.Errorf("synthesize canceled (joined=%t, events=%v, load_count=%d, close_count=%d)", joined, log.events(), pipeline.RuntimeLoadCount()-loadBefore, pipeline.RuntimeCloseCount()-closeBefore))
	}
	if synthErr != nil {
		rt.Close()
		if err := checkDirectInvariants(beforeTemp, *out, log.events(), joined, loadBefore, closeBefore); err != nil {
			return finish(false, smokeErrorInvariant, err)
		}
		return finish(*expectError, smokeErrorInference, fmt.Errorf("synthesize: %w (joined=%t, events=%v, load_count=%d, close_count=%d)", synthErr, joined, log.events(), pipeline.RuntimeLoadCount()-loadBefore, pipeline.RuntimeCloseCount()-closeBefore))
	}
	if _, err := os.Stat(*out); err != nil {
		rt.Close()
		return finish(false, smokeErrorInvariant, fmt.Errorf("output WAV: %w", err))
	}
	rt.Close()
	if err := requireCounts(loadBefore, closeBefore, 1, 1); err != nil {
		return finish(false, smokeErrorInvariant, err)
	}
	afterTemp, snapErr := tempWAVSet()
	if snapErr != nil || !hasLifecycle(log.events()) || !sameTempSet(beforeTemp, afterTemp) {
		return finish(false, smokeErrorInvariant, fmt.Errorf("direct success lifecycle or temporary WAV invariant failed (events=%v)", log.events()))
	}
	if *expectError {
		return finish(false, smokeErrorInvariant, fmt.Errorf("expected an error but smoke succeeded"))
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"model": *model, "execution_provider": *ep, "seed": uint32(*seed),
		"output": *out, "steps": *steps, "seconds": *seconds,
		"cancel_after": cancelAfter.String(), "preflight": true,
		"cancel_requested": ctx.Err() != nil, "joined": joined, "events": log.events(), "load_count": pipeline.RuntimeLoadCount() - loadBefore, "close_count": pipeline.RuntimeCloseCount() - closeBefore,
	})
	return nil
}

type smokeErrorKind string

const (
	smokeErrorOperational  smokeErrorKind = "operational"
	smokeErrorInference    smokeErrorKind = "inference"
	smokeErrorCancellation smokeErrorKind = "cancellation"
	smokeErrorInvariant    smokeErrorKind = "invariant"
)

func finish(expected bool, kind smokeErrorKind, err error) error {
	if err == nil {
		return nil
	}
	if expected && (kind == smokeErrorInference || kind == smokeErrorCancellation) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"expected_error": err.Error()})
		return nil
	}
	return err
}

func requireCounts(loadBefore, closeBefore uint64, wantLoads, wantCloses uint64) error {
	loads := pipeline.RuntimeLoadCount() - loadBefore
	closes := pipeline.RuntimeCloseCount() - closeBefore
	if loads != wantLoads || closes != wantCloses {
		return fmt.Errorf("runtime load/close count mismatch: loads=%d closes=%d want=%d/%d", loads, closes, wantLoads, wantCloses)
	}
	return nil
}

func checkDirectInvariants(beforeTemp map[string]struct{}, output string, events []pipeline.RuntimeEvent, joined bool, loadBefore, closeBefore uint64) error {
	if !joined {
		return fmt.Errorf("direct cancellation did not join inference (events=%v)", events)
	}
	if !hasLifecycle(events) {
		return fmt.Errorf("direct cancellation lifecycle incomplete (events=%v)", events)
	}
	if err := requireCounts(loadBefore, closeBefore, 1, 1); err != nil {
		return err
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		return fmt.Errorf("direct cancellation left output %q", output)
	}
	afterTemp, snapErr := tempWAVSet()
	if snapErr != nil || !sameTempSet(beforeTemp, afterTemp) {
		return fmt.Errorf("direct cancellation left temporary WAV (events=%v)", events)
	}
	return nil
}

type eventLog struct {
	mu     sync.Mutex
	values []pipeline.RuntimeEvent
}

func (l *eventLog) add(event pipeline.RuntimeEvent) {
	l.mu.Lock()
	l.values = append(l.values, event)
	l.mu.Unlock()
}

func (l *eventLog) events() []pipeline.RuntimeEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]pipeline.RuntimeEvent(nil), l.values...)
}

func hasLifecycle(events []pipeline.RuntimeEvent) bool {
	if len(events) == 0 {
		return false
	}
	loaded, closeStarts, closeEnds := 0, 0, 0
	closeStart := -1
	closeEnd := -1
	active, ended, joined := false, false, false
	preflightStart, preflightEnd, loadStart, loadEnd := 0, 0, 0, 0
	for i, event := range events {
		switch event {
		case pipeline.EventPreflightStart:
			preflightStart++
		case pipeline.EventPreflightEnd:
			preflightEnd++
		case pipeline.EventLoadStart:
			loadStart++
		case pipeline.EventLoaded:
			loaded++
		case pipeline.EventLoadEnd:
			loadEnd++
		case pipeline.EventInferenceStart:
			if closeStart >= 0 || active {
				return false
			}
			active, ended, joined = true, false, false
		case pipeline.EventInferenceEnd:
			if !active || ended {
				return false
			}
			ended = true
		case pipeline.EventInferenceJoined:
			if !active || !ended || joined {
				return false
			}
			joined = true
			active = false
		case pipeline.EventCloseStart:
			if active || !joined || closeStart >= 0 {
				return false
			}
			closeStarts++
			closeStart = i
		case pipeline.EventCloseEnd:
			if closeStart < 0 || closeEnd >= 0 {
				return false
			}
			closeEnds++
			closeEnd = i
		}
	}
	return preflightStart == 1 && preflightEnd == 1 && loadStart == 1 && loaded == 1 && loadEnd == 1 && closeStarts == 1 && closeEnds == 1 && closeEnd > closeStart && !active && joined
}

func hasEvent(events []pipeline.RuntimeEvent, want pipeline.RuntimeEvent) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}

func tempWAVSet() (map[string]struct{}, error) {
	set := map[string]struct{}{}
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return set, err
	}
	collect := func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.EqualFold(filepath.Ext(name), ".wav") && strings.HasPrefix(name, "irodori_") {
			set[path] = struct{}{}
		}
		return nil
	}
	for _, entry := range entries {
		path := filepath.Join(os.TempDir(), entry.Name())
		if !entry.IsDir() {
			if err := collect(path, entry, nil); err != nil {
				return set, err
			}
			continue
		}
		// Restrict recursion to directories owned by localtts/tts-smoke. Other
		// system temp directories may be inaccessible and are not our assets.
		if strings.HasPrefix(entry.Name(), "fm-live-radio-talk-") || strings.HasPrefix(entry.Name(), "fm-live-radio-sentence-") {
			if err := filepath.WalkDir(path, collect); err != nil {
				return set, err
			}
		}
	}
	return set, nil
}

func sameTempSet(before, after map[string]struct{}) bool {
	if len(before) != len(after) {
		return false
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			return false
		}
	}
	return true
}
