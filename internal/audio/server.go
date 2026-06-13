package audio

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/audiofmt"

	"github.com/google/uuid"
)

// loudnessWindowMs is the analysis window size used for envelope precomputation.
const loudnessWindowMs = 50

type tokenEntry struct {
	path      string
	expiresAt time.Time
}

// loudnessEntry holds the marshalled JSON response for /loudness/<token> so
// repeated requests do not recompute or re-marshal the envelope.
type loudnessEntry struct {
	json []byte
}

// LoudnessEnvelopeResponse is the JSON body served by /loudness/<token>.
type LoudnessEnvelopeResponse struct {
	WindowMS    int       `json:"windowMs"`
	SampleRate  int       `json:"sampleRate"`
	DurationSec float64   `json:"durationSec"`
	RMS         []float64 `json:"rms"`
	Peak        []float64 `json:"peak,omitempty"`
}

type Server struct {
	ln      net.Listener
	srv     *http.Server
	baseURL string

	mu       sync.Mutex
	tokens   map[string]tokenEntry
	loudness map[string]loudnessEntry

	gcStop chan struct{}
}

func Start() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := &Server{
		ln:       ln,
		baseURL:  "http://" + ln.Addr().String(),
		tokens:   map[string]tokenEntry{},
		loudness: map[string]loudnessEntry{},
		gcStop:   make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/audio/", s.handleAudio)
	mux.HandleFunc("/loudness/", s.handleLoudness)

	s.srv = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = s.srv.Serve(ln)
	}()

	go s.gcLoop()

	return s, nil
}

func (s *Server) BaseURL() string { return s.baseURL }

func (s *Server) Close(ctx context.Context) error {
	close(s.gcStop)
	err := s.srv.Shutdown(ctx)
	_ = s.ln.Close()
	return err
}

// RegisterFile registers a local file as a token-protected resource and
// returns the audio URL. If the file is a 16-bit PCM WAV, a loudness envelope
// is precomputed and cached for /loudness/<token>. Envelope failures are
// logged but never fail RegisterFile.
func (s *Server) RegisterFile(path string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("empty path")
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", errors.New("path is directory")
	}
	tok := uuid.NewString()
	exp := time.Now().Add(ttl)

	s.mu.Lock()
	s.tokens[tok] = tokenEntry{path: path, expiresAt: exp}
	s.mu.Unlock()

	// Best-effort envelope precompute; failure must not affect audio URL.
	s.precomputeLoudness(tok, path)

	return s.baseURL + "/audio/" + tok, nil
}

// LoudnessURLForAudioURL returns the corresponding /loudness/<token> URL for an
// audio URL produced by RegisterFile. Returns "" if the input is not a known
// audio URL shape.
func (s *Server) LoudnessURLForAudioURL(audioURL string) string {
	prefix := s.baseURL + "/audio/"
	if !strings.HasPrefix(audioURL, prefix) {
		return ""
	}
	tok := strings.TrimPrefix(audioURL, prefix)
	if tok == "" {
		return ""
	}
	return s.baseURL + "/loudness/" + tok
}

func (s *Server) precomputeLoudness(token, path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".wav" {
		return
	}
	env, err := audiofmt.ComputeWavLoudnessEnvelopeFile(path, loudnessWindowMs)
	if err != nil {
		log.Printf("WARN: loudness envelope compute failed for %s: %v", path, err)
		return
	}
	resp := LoudnessEnvelopeResponse{
		WindowMS:    env.WindowMS,
		SampleRate:  env.SampleRate,
		DurationSec: env.DurationSec,
		RMS:         env.RMS,
		Peak:        env.Peak,
	}
	buf, err := json.Marshal(resp)
	if err != nil {
		log.Printf("WARN: loudness envelope marshal failed for %s: %v", path, err)
		return
	}
	s.mu.Lock()
	s.loudness[token] = loudnessEntry{json: buf}
	s.mu.Unlock()
}

func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.URL.Path, "/audio/")
	if tok == "" {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	e, ok := s.tokens[tok]
	if ok && time.Now().After(e.expiresAt) {
		delete(s.tokens, tok)
		delete(s.loudness, tok)
		ok = false
	}
	s.mu.Unlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	// Best-effort content type.
	ext := strings.ToLower(filepath.Ext(e.path))
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	http.ServeFile(w, r, e.path)
}

// handleLoudness serves the JSON envelope for a token. The endpoint is the
// only one that needs CORS for fetch() from the Wails dev server origin; the
// local app uses HEAD/GET via XHR. Token expiry deletes both the audio token
// and any cached envelope.
func (s *Server) handleLoudness(w http.ResponseWriter, r *http.Request) {
	// CORS for local app fetch. Restrict to non-credentialed local usage.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	tok := strings.TrimPrefix(r.URL.Path, "/loudness/")
	if tok == "" {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	te, tok2 := s.tokens[tok]
	if tok2 && time.Now().After(te.expiresAt) {
		delete(s.tokens, tok)
		delete(s.loudness, tok)
		tok2 = false
	}
	if !tok2 {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	le, hasEnv := s.loudness[tok]
	s.mu.Unlock()

	if !hasEnv {
		// Non-WAV / decode failure: signal "no envelope available".
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(le.json)
}

func (s *Server) gcLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.gcStop:
			return
		case <-t.C:
			now := time.Now()
			s.mu.Lock()
			for k, v := range s.tokens {
				if now.After(v.expiresAt) {
					delete(s.tokens, k)
					delete(s.loudness, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
