package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

var staticTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".txt":         "text/plain; charset=utf-8",
	".map":         "application/json; charset=utf-8",
	".webmanifest": "application/manifest+json",
}

const fallbackIndex = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="doppel-token" content="__DOPPEL_TOKEN__">
<title>WhatsApp Doppel</title>
<style>body{font:16px -apple-system,BlinkMacSystemFont,sans-serif;display:grid;place-items:center;height:100vh;margin:0;background:#0f172a;color:#e2e8f0}</style>
</head><body><main><h1>WhatsApp Doppel is running</h1><p>The web interface was not bundled into this build.</p></main></body></html>`

type etagCache struct {
	mu sync.Mutex
	m  map[string]string
}

func (c *etagCache) get(name string, data []byte) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]string{}
	}
	if e, ok := c.m[name]; ok {
		return e
	}
	sum := sha256.Sum256(data)
	e := `"` + hex.EncodeToString(sum[:8]) + `"`
	c.m[name] = e
	return e
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "index.html" {
		s.serveIndex(w, r)
		return
	}
	base := path.Base(name)
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || strings.HasSuffix(name, ".go") {
		http.NotFound(w, r)
		return
	}
	var data []byte
	var err error
	if s.d.Web != nil {
		data, err = fs.ReadFile(s.d.Web, name)
	} else {
		err = fs.ErrNotExist
	}
	if err != nil {
		// Client-side routes (no extension) fall back to the app shell.
		if path.Ext(name) == "" {
			s.serveIndex(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	ct := staticTypes[strings.ToLower(path.Ext(name))]
	if ct == "" {
		ct = mime.TypeByExtension(path.Ext(name))
	}
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(withETag(w, s.etags.get(name, data)), r, "", time.Time{}, bytes.NewReader(data))
}

func withETag(w http.ResponseWriter, etag string) http.ResponseWriter {
	w.Header().Set("ETag", etag)
	return w
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	var page []byte
	if s.d.Web != nil {
		page, _ = fs.ReadFile(s.d.Web, "index.html")
	}
	if len(page) == 0 {
		page = []byte(fallbackIndex)
	}
	page = bytes.ReplaceAll(page, []byte("__DOPPEL_TOKEN__"), []byte(html.EscapeString(s.token)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	if r.Method == http.MethodHead {
		return
	}
	w.Write(page)
}
