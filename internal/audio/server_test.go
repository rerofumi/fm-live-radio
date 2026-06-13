package audio

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fm-live-radio/internal/audiofmt"
)

func writeSineWav(t *testing.T, path string, sampleRate, channels int, duration time.Duration, amp int16) {
	t.Helper()
	frames := int(duration.Seconds() * float64(sampleRate))
	pcm := make([]byte, frames*channels*2)
	for f := 0; f < frames; f++ {
		v := int32(math.Round(float64(amp) * math.Sin(2*math.Pi*440*float64(f)/float64(sampleRate))))
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		for c := 0; c < channels; c++ {
			off := (f*channels + c) * 2
			pcm[off] = byte(v)
			pcm[off+1] = byte(v >> 8)
		}
	}
	wav, err := audiofmt.EncodeWavPCM16(pcm, sampleRate, channels)
	if err != nil {
		t.Fatalf("EncodeWavPCM16: %v", err)
	}
	if err := os.WriteFile(path, wav, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func writeRawFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func doGet(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = resp.Body.Close()
	return resp, body
}

func TestServerRegisterFile_ReturnsLoudnessURLForWAV(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "tone.wav")
	writeSineWav(t, wavPath, 1000, 1, 200*time.Millisecond, 16384)

	s := newTestServer(t)
	url, err := s.RegisterFile(wavPath, time.Minute)
	if err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}
	if url == "" {
		t.Fatalf("empty audio URL")
	}

	loudURL := s.LoudnessURLForAudioURL(url)
	if loudURL == "" {
		t.Fatalf("LoudnessURLForAudioURL returned empty for %q", url)
	}
	want := s.BaseURL() + "/loudness/" + filepath.Base(url)
	if loudURL != want {
		t.Fatalf("loudness URL = %q, want %q", loudURL, want)
	}

	resp, body := doGet(t, loudURL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("CORS Access-Control-Allow-Origin = %q, want *", got)
	}

	var env LoudnessEnvelopeResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if env.WindowMS != 50 {
		t.Errorf("WindowMS = %d, want 50", env.WindowMS)
	}
	if env.SampleRate != 1000 {
		t.Errorf("SampleRate = %d, want 1000", env.SampleRate)
	}
	if len(env.RMS) == 0 {
		t.Fatalf("len(RMS) = 0, want > 0")
	}
	if env.Peak == nil {
		t.Errorf("Peak is nil; expected non-nil slice for valid WAV")
	} else {
		for i, p := range env.Peak {
			if p <= 0 || p > 1 {
				t.Errorf("Peak[%d] = %v, want (0, 1]", i, p)
			}
		}
	}
}

func TestServerRegisterFile_NonWAVReturns204(t *testing.T) {
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "raw.bin")
	writeRawFile(t, rawPath, []byte("not a wav file"))

	s := newTestServer(t)
	url, err := s.RegisterFile(rawPath, time.Minute)
	if err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}
	if url == "" {
		t.Fatalf("empty audio URL")
	}

	loudURL := s.LoudnessURLForAudioURL(url)
	if loudURL == "" {
		t.Fatalf("LoudnessURLForAudioURL returned empty")
	}

	resp, body := doGet(t, loudURL)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", resp.StatusCode, body)
	}
	if len(body) != 0 {
		t.Errorf("expected empty body for 204, got %d bytes: %q", len(body), body)
	}
}

func TestServerLoudness_UnknownTokenReturns404(t *testing.T) {
	s := newTestServer(t)
	resp, body := doGet(t, s.BaseURL()+"/loudness/does-not-exist")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.StatusCode, body)
	}
}

func TestServerLoudness_ExpiredTokenReturns404AndDropsCache(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "tone.wav")
	writeSineWav(t, wavPath, 1000, 1, 200*time.Millisecond, 16384)

	s := newTestServer(t)
	url, err := s.RegisterFile(wavPath, time.Nanosecond)
	if err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	loudURL := s.LoudnessURLForAudioURL(url)
	resp, body := doGet(t, loudURL)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.StatusCode, body)
	}

	tok := filepath.Base(loudURL)
	s.mu.Lock()
	_, hasLoudness := s.loudness[tok]
	_, hasToken := s.tokens[tok]
	s.mu.Unlock()
	if hasLoudness {
		t.Errorf("loudness cache should be cleared for expired token")
	}
	if hasToken {
		t.Errorf("token map should be cleared for expired token")
	}
}

func TestServerLoudness_OPTIONSPreflightReturns204WithCORS(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "tone.wav")
	writeSineWav(t, wavPath, 1000, 1, 200*time.Millisecond, 16384)

	s := newTestServer(t)
	url, err := s.RegisterFile(wavPath, time.Minute)
	if err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}
	loudURL := s.LoudnessURLForAudioURL(url)

	req, err := http.NewRequest(http.MethodOptions, loudURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS Allow-Origin = %q, want *", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("CORS Allow-Methods empty")
	}
}
