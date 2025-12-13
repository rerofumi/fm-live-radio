package audio

import (
	"context"
	"errors"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type tokenEntry struct {
	path      string
	expiresAt time.Time
}

type Server struct {
	ln      net.Listener
	srv     *http.Server
	baseURL string

	mu     sync.Mutex
	tokens map[string]tokenEntry

	gcStop chan struct{}
}

func Start() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := &Server{
		ln:      ln,
		baseURL: "http://" + ln.Addr().String(),
		tokens:  map[string]tokenEntry{},
		gcStop:  make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/audio/", s.handleAudio)

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

	return s.baseURL + "/audio/" + tok, nil
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
				}
			}
			s.mu.Unlock()
		}
	}
}
