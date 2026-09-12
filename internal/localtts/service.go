package localtts

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/audiofmt"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/metadata"
	"fm-live-radio/internal/localtts/irodori/pipeline"
)

const (
	irodoriSampleRate        = 48000
	irodoriChannels          = 1
	irodoriSentenceGap       = 300 * time.Millisecond
	irodoriSentenceFailPause = 3 * time.Second
)

var ErrEmptyText = errors.New("irodori text is empty")
var ErrAllSentencesFailed = errors.New("irodori all sentences failed; refusing all-silence success")

type Service struct {
	mu      sync.Mutex
	arbiter *generation.Arbiter
}

func New() *Service {
	return NewWithArbiter(generation.SharedArbiter())
}

// NewWithArbiter provides an isolated scheduler for deterministic service
// tests. Product construction uses the process-shared arbiter via New.
func NewWithArbiter(arbiter *generation.Arbiter) *Service {
	if arbiter == nil {
		arbiter = generation.SharedArbiter()
	}
	return &Service{arbiter: arbiter}
}

func (s *Service) SynthesizeWav(ctx context.Context, cfg domain.AppConfig, text string) ([]byte, error) {
	return s.synthesizeWav(ctx, cfg, text, nil)
}

// SynthesizeWavWithObserver is the observable form used by lifecycle smoke
// tests. The normal product API remains SynthesizeWav; observers receive only
// real runtime/join events and never synthetic started/joined values.
func (s *Service) SynthesizeWavWithObserver(ctx context.Context, cfg domain.AppConfig, text string, observer pipeline.EventObserver) ([]byte, error) {
	return s.synthesizeWav(ctx, cfg, text, observer)
}

func (s *Service) synthesizeWav(ctx context.Context, cfg domain.AppConfig, text string, observer pipeline.EventObserver) ([]byte, error) {
	arbiter := s.arbiter
	if arbiter == nil {
		arbiter = generation.SharedArbiter()
	}
	reservation := generation.ReservationFromContext(ctx, generation.KindTalk)
	if reservation != nil && reservation.Arbiter() != arbiter {
		reservation.Release()
		reservation = nil
	}
	if reservation == nil {
		var err error
		reservation, err = arbiter.Reserve(ctx, generation.KindTalk)
		if err != nil {
			return nil, err
		}
	}
	defer reservation.Release()
	if err := preflightTTS(cfg, observer); err != nil {
		return nil, err
	}
	lease, err := reservation.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	wav, err := s.synthesizeSentences(ctx, cfg, text, observer)
	if observer != nil && err == nil {
		observer(pipeline.EventTalkEnd)
	}
	return wav, err
}

type ttsRuntime interface {
	SynthesizeWithOptions(pipeline.Options) error
	Close()
}

var loadTTSRuntime = func(opt pipeline.Options) (ttsRuntime, error) {
	return pipeline.LoadInitialise(opt)
}

// preflightTTS is replaceable by package tests so fake runtimes can exercise
// the scheduler without requiring an ONNX model bundle.
var preflightTTS = runPreflight

func (s *Service) synthesizeSentences(ctx context.Context, cfg domain.AppConfig, text string, observer pipeline.EventObserver) ([]byte, error) {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil, ErrEmptyText
	}

	combined := make([]byte, 0)
	tempDir, err := os.MkdirTemp("", "fm-live-radio-talk-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	gap, err := audiofmt.SilencePCM16(irodoriSampleRate, irodoriChannels, irodoriSentenceGap)
	if err != nil {
		return nil, err
	}
	failSilence, err := audiofmt.SilencePCM16(irodoriSampleRate, irodoriChannels, irodoriSentenceFailPause)
	if err != nil {
		return nil, err
	}

	// A Runtime is deliberately scoped to one Talk. This reuses the loaded
	// model/reference sessions across sentences without creating a process-wide
	// GPU cache. The service mutex serializes inference and owns the Close call.
	baseOpt := pipelineOptions(cfg)
	baseOpt.Observer = observer
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if observer != nil {
		observer(pipeline.EventLoadStart)
	}
	rt, err := loadTTSRuntime(baseOpt)
	if observer != nil {
		observer(pipeline.EventLoadEnd)
	}
	if err != nil {
		return nil, err
	}
	defer rt.Close()

	succeeded := 0
	for i, sentence := range sentences {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sentenceOpt := baseOpt
		// Preserve the legacy per-sentence randomness while keeping the loaded
		// model/reference runtime shared for the whole Talk.
		if strings.TrimSpace(cfg.Irodori.SeedMode) != "fixed" {
			sentenceOpt.Seed = resolveSeed(cfg.Irodori.SeedMode, cfg.Irodori.FixedSeed)
		}
		pcm, err := s.synthesizeSentencePCM(ctx, rt, sentenceOpt, sentence)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			pcm = failSilence
		} else {
			succeeded++
		}
		combined = append(combined, pcm...)
		if i < len(sentences)-1 {
			if observer != nil {
				observer(pipeline.EventGapStart)
			}
			combined = append(combined, gap...)
			if observer != nil {
				observer(pipeline.EventGapEnd)
			}
		}
	}
	if succeeded == 0 {
		return nil, ErrAllSentencesFailed
	}

	if observer != nil {
		observer(pipeline.EventCombineStart)
	}
	wav, err := audiofmt.EncodeWavPCM16(combined, irodoriSampleRate, irodoriChannels)
	if observer != nil {
		observer(pipeline.EventCombineEnd)
	}
	return wav, err
}

// runPreflight is deliberately a separate phase.  Its end event is emitted
// before runtime loading starts, so benchmark spans are disjoint and a bad
// bundle cannot be mistaken for a sentence failure.
func runPreflight(cfg domain.AppConfig, observer pipeline.EventObserver) error {
	if observer != nil {
		observer(pipeline.EventPreflightStart)
		defer observer(pipeline.EventPreflightEnd)
	}
	if strings.TrimSpace(cfg.Irodori.ModelDir) == "" {
		return generation.ErrProviderNotConfigured
	}
	if err := validateModelAssets(cfg.Irodori.ModelDir); err != nil {
		return err
	}
	if err := generation.ConfigureExecutionProvider(cfg.LocalInference.ExecutionProvider, cfg.LocalInference.DeviceID); err != nil {
		return err
	}
	if err := generation.Init(cfg.LocalInference.ORTLibraryPath); err != nil {
		return err
	}
	return nil
}

func (s *Service) synthesizeSentencePCM(ctx context.Context, rt ttsRuntime, baseOpt pipeline.Options, text string) ([]byte, error) {
	tempDir, err := os.MkdirTemp("", "fm-live-radio-sentence-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	outPath := filepath.Join(tempDir, fmt.Sprintf("irodori_%d.wav", time.Now().UnixNano()))
	defer os.Remove(outPath)

	opt := baseOpt
	opt.Text = text
	opt.OutputWAV = outPath
	if err := synthesizeToFile(ctx, rt, opt); err != nil {
		if opt.Observer != nil {
			// synthesizeToFile has joined the actual worker before returning.
			opt.Observer(pipeline.EventServiceJoined)
		}
		return nil, err
	}
	if opt.Observer != nil {
		// This event is after the Service's worker receive, not a synthetic
		// replacement for the runtime's inference-end event.
		opt.Observer(pipeline.EventServiceJoined)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	wav, err := audiofmt.DecodeWavPCM16(data)
	if err != nil {
		return nil, err
	}
	if wav.SampleRate != irodoriSampleRate || wav.Channels != irodoriChannels {
		return nil, fmt.Errorf("irodori wav format mismatch: %d Hz, %d channels", wav.SampleRate, wav.Channels)
	}
	envelope, err := audiofmt.ComputeWavLoudnessEnvelope(data, 50)
	if err != nil {
		return nil, fmt.Errorf("irodori wav loudness: %w", err)
	}
	for i := range envelope.RMS {
		if envelope.RMS[i] > 0 && envelope.Peak[i] > 0 {
			return wav.PCM, nil
		}
	}
	return nil, errors.New("irodori wav is silent")
}

func pipelineOptions(cfg domain.AppConfig) pipeline.Options {
	opt := pipeline.DefaultOptions()
	opt.ModelDir = cfg.Irodori.ModelDir
	opt.Seconds = cfg.Irodori.Seconds
	opt.NumSteps = cfg.Irodori.NumSteps
	opt.CfgText = cfg.Irodori.CfgText
	opt.CfgCaption = cfg.Irodori.CfgCaption
	opt.CfgSpeaker = cfg.Irodori.CfgSpeaker
	opt.DurationScale = cfg.Irodori.DurationScale
	opt.RefWAV = resolveReferenceWAV(cfg.Irodori)
	opt.Seed = resolveSeed(cfg.Irodori.SeedMode, cfg.Irodori.FixedSeed)
	return opt
}

// synthesizeToFile waits for the inference goroutine even after cancellation.
// ORT has no portable in-flight cancellation API; returning early and closing
// the Runtime would race with Run and can corrupt later Talk/BGM requests.
func synthesizeToFile(ctx context.Context, rt ttsRuntime, opt pipeline.Options) error {
	done := make(chan error, 1)
	go func() {
		done <- rt.SynthesizeWithOptions(opt)
	}()

	select {
	case err := <-done:
		if opt.Observer != nil {
			opt.Observer(pipeline.EventInferenceJoined)
		}
		return err
	case <-ctx.Done():
		// Wait for ORT to leave the session before the caller's deferred Close.
		<-done
		if opt.Observer != nil {
			opt.Observer(pipeline.EventInferenceJoined)
		}
		return ctx.Err()
	}
}

func resolveReferenceWAV(cfg domain.IrodoriConfig) string {
	if strings.TrimSpace(cfg.RefWAV) != "" {
		return cfg.RefWAV
	}
	entries, err := os.ReadDir(cfg.NarratorDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".wav") {
			return filepath.Join(cfg.NarratorDir, entry.Name())
		}
	}
	return ""
}

func resolveSeed(mode string, fixed uint32) uint32 {
	switch strings.TrimSpace(mode) {
	case "fixed":
		return fixed
	case "sequential":
		return uint32(time.Now().Unix())
	default:
		return rand.Uint32()
	}
}

func validateModelAssets(modelDir string) error {
	if _, err := os.Stat(filepath.Join(modelDir, "manifest.json")); err == nil {
		m, err := metadata.LoadManifest(modelDir)
		if err != nil {
			return fmt.Errorf("irodori v4 preflight rejected: %w", err)
		}
		if err := m.VerifyHashes(modelDir); err != nil {
			return fmt.Errorf("irodori v4 preflight integrity check failed: %w", err)
		}
		return nil
	}
	if _, err := os.Stat(filepath.Join(modelDir, "tokenizer.json")); err != nil {
		return err
	}
	md, err := metadata.Load(modelDir)
	if err != nil {
		return err
	}
	for name := range md.Exports {
		p := md.FilePath(modelDir, name)
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return err
		}
	}
	return nil
}

func splitSentences(text string) []string {
	var sentences []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		if s != "" {
			sentences = append(sentences, s)
		}
		b.Reset()
	}

	for _, r := range text {
		switch r {
		case '\r', '\n':
			flush()
			continue
		}
		b.WriteRune(r)
		switch r {
		case '。', '！', '？', '!', '?':
			flush()
		}
	}
	flush()

	return sentences
}
