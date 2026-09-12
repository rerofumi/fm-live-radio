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
	"fm-live-radio/internal/fileprotect"

	"github.com/google/uuid"
)

// loudnessWindowMs is the analysis window size used for envelope precomputation.
const loudnessWindowMs = 50

type tokenEntry struct {
	path      string
	expiresAt time.Time
	release   func()
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

	gcStop    chan struct{}
	closeOnce sync.Once
	// now is injectable so token expiry and cleanup tests can advance time
	// without sleeping. Production servers use time.Now.
	now func() time.Time
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
		now:      time.Now,
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
	var releases []func()
	s.closeOnce.Do(func() {
		close(s.gcStop)
		s.mu.Lock()
		releases = make([]func(), 0, len(s.tokens))
		for token, entry := range s.tokens {
			if entry.release != nil {
				releases = append(releases, entry.release)
			}
			delete(s.tokens, token)
			delete(s.loudness, token)
		}
		s.mu.Unlock()
	})
	for _, release := range releases {
		release()
	}
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
	exp := s.currentTime().Add(ttl)
	release := fileprotect.Acquire(path)

	s.mu.Lock()
	s.tokens[tok] = tokenEntry{path: path, expiresAt: exp, release: release}
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

	var expiredRelease func()
	var requestRelease func()
	s.mu.Lock()
	e, ok := s.tokens[tok]
	if ok && s.currentTime().After(e.expiresAt) {
		expiredRelease = e.release
		delete(s.tokens, tok)
		delete(s.loudness, tok)
		ok = false
	}
	if ok {
		requestRelease = fileprotect.Acquire(e.path)
	}
	s.mu.Unlock()
	if expiredRelease != nil {
		expiredRelease()
	}

	if !ok {
		http.NotFound(w, r)
		return
	}

	// Best-effort content type.
	ext := strings.ToLower(filepath.Ext(e.path))
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	// Hold a request-scoped reference so expiry/GC cannot remove the file while
	// ServeFile is reading it.
	defer requestRelease()
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

	var expiredRelease func()
	s.mu.Lock()
	te, tok2 := s.tokens[tok]
	if tok2 && s.currentTime().After(te.expiresAt) {
		expiredRelease = te.release
		delete(s.tokens, tok)
		delete(s.loudness, tok)
		tok2 = false
	}
	le, hasEnv := s.loudness[tok]
	s.mu.Unlock()
	if expiredRelease != nil {
		expiredRelease()
	}
	if !tok2 {
		http.NotFound(w, r)
		return
	}

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
			s.CleanupExpired(s.currentTime())
		}
	}
}

// CleanupExpired removes tokens whose TTL has elapsed at now. It is also the
// explicit deterministic cleanup boundary used by tests and shutdown hooks;
// releases happen after the token map lock is dropped.
func (s *Server) CleanupExpired(now time.Time) {
	releases := make([]func(), 0)
	s.mu.Lock()
	for k, v := range s.tokens {
		if now.After(v.expiresAt) {
			if v.release != nil {
				releases = append(releases, v.release)
			}
			delete(s.tokens, k)
			delete(s.loudness, k)
		}
	}
	s.mu.Unlock()
	for _, release := range releases {
		release()
	}
}

func (s *Server) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// ReleaseAudioURL explicitly transfers ownership away from a registered
// token before its TTL. It is safe to call repeatedly and is useful when the
// player discards a selected item during shutdown or replacement.
func (s *Server) ReleaseAudioURL(audioURL string) bool {
	prefix := s.baseURL + "/audio/"
	if !strings.HasPrefix(audioURL, prefix) {
		return false
	}
	tok := strings.TrimPrefix(audioURL, prefix)
	s.mu.Lock()
	e, ok := s.tokens[tok]
	if ok {
		delete(s.tokens, tok)
		delete(s.loudness, tok)
	}
	s.mu.Unlock()
	if ok && e.release != nil {
		e.release()
	}
	return ok
}
