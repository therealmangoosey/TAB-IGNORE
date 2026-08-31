// Package srv serves the local library over HTTP with standard Range support
// and exposes a small /api surface for automation. It also exposes an optional
// LAN UPnP/DLNA MediaServer for compatible Smart TVs and players.
package srv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmangoosey/TAB-IGNORE/internal/lib"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

// Server is the local HTTP streaming server plus the LAN media server.
type Server struct {
	Addr     string
	LAN      bool
	Token    string
	Library  *lib.Library
	StatusFn func(ctx context.Context) (hermit.Status, error)
	Log      io.Writer
	server   *http.Server
	listener net.Listener
	dlna     *dlnaServer
}

// New creates a server. The DLNA server is enabled by default and can be
// disabled with HERMIT_MEDIA_SERVER=0. It listens separately from the loopback
// API server so TV access never requires exposing the control API.
func New(addr, token string, library *lib.Library, statusFn func(context.Context) (hermit.Status, error)) *Server {
	s := &Server{Addr: addr, Token: token, LAN: token != "", Library: library, StatusFn: statusFn, Log: io.Discard}
	if os.Getenv("HERMIT_MEDIA_SERVER") != "0" {
		s.dlna = newDLNAServer(library)
	}
	return s
}

// Start binds and serves until ctx is canceled.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.server = &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()
	if s.dlna != nil {
		s.dlna.start(ctx, func(string) {})
	}
	err = s.server.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ListenAddr returns the bound address.
func (s *Server) ListenAddr() string {
	if s.listener == nil {
		return s.Addr
	}
	return s.listener.Addr().String()
}

// Handler builds the mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/media/", s.handleMedia)
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	if !s.LAN || s.Token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != s.Token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": hermitVersion()})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.StatusFn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "status unavailable"})
		return
	}
	st, err := s.StatusFn(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	if s.Library == nil {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/media/")
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(s.Library.Root, rel)
	rootAbs, _ := filepath.Abs(s.Library.Root)
	fullAbs, _ := filepath.Abs(full)
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func hermitVersion() string {
	return "devel"
}

// Debugf logs through the configured writer.
func (s *Server) Debugf(format string, args ...any) {
	fmt.Fprintf(s.Log, format+"\n", args...)
}
