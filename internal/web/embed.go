package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var assets embed.FS

func Handler() http.Handler {
	static, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return spaHandler{static: static, files: http.FileServer(http.FS(static))}
}

type spaHandler struct {
	static fs.FS
	files  http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if clean != "." && clean != "index.html" {
		if info, err := fs.Stat(h.static, clean); err == nil && !info.IsDir() {
			h.files.ServeHTTP(w, r)
			return
		}
	}
	h.serveIndex(w)
}

func (h spaHandler) serveIndex(w http.ResponseWriter) {
	data, err := fs.ReadFile(h.static, "index.html")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(".html"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
