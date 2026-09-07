// Package serve runs a small loopback HTTP server that streams cover and page
// bytes straight into <img> tags. The webview (wails:// origin) treats plain
// http as mixed content, so the app enables WebView2's
// --allow-insecure-localhost flag and each URL carries a per-session token.
// If that ever fails, the app falls back to blob/data URLs behind the same
// CoverRef/PageRef abstraction.
package serve

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Provider supplies the bytes served by the media endpoints.
type Provider interface {
	// Cover returns the thumbnail bytes + content type for a comic.
	Cover(id int64) ([]byte, string, error)
	// Page returns the raw bytes + content type of one page of a comic.
	Page(id int64, n int) ([]byte, string, error)
}

// Server is a running loopback media server.
type Server struct {
	URL   string // base, e.g. http://127.0.0.1:51324
	Token string
	prov  Provider
	ln    net.Listener
	hs    *http.Server
}

// NewToken returns a fresh per-session token.
func NewToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Start launches the server on an ephemeral loopback port.
func Start(p Provider) (*Server, error) {
	tok, err := NewToken()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{
		URL:   "http://" + ln.Addr().String(),
		Token: tok,
		prov:  p,
		ln:    ln,
	}
	s.hs = &http.Server{Handler: s.handler()}
	go s.hs.Serve(ln)
	return s, nil
}

// Close stops the server.
func (s *Server) Close() {
	if s.hs != nil {
		_ = s.hs.Close()
	}
}

// CoverURL builds the URL the frontend uses for a cover image.
func (s *Server) CoverURL(id int64) string {
	return fmt.Sprintf("%s/cover/%d?t=%s", s.URL, id, s.Token)
}

// PageURL builds the URL for one reader page.
func (s *Server) PageURL(id int64, n int) string {
	return fmt.Sprintf("%s/page/%d/%d?t=%s", s.URL, id, n, s.Token)
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/cover/", s.handleCover)
	mux.HandleFunc("/page/", s.handlePage)
	return mux
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Query().Get("t") != s.Token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/cover/"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	b, mime, err := s.prov.Cover(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Write(b)
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/page/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	id, err1 := strconv.ParseInt(parts[0], 10, 64)
	n, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		http.Error(w, "bad id/page", http.StatusBadRequest)
		return
	}
	b, mime, err := s.prov.Page(id, n)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(b)
}
