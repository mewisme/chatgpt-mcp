package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
)

//go:embed dist/*
var assets embed.FS

func Handler() http.Handler {
	static, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return spaHandler{files: http.FileServer(http.FS(static))}
}

type spaHandler struct{ files http.Handler }

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		_, err := fs.Stat(embeddedFS(), path.Clean("dist"+r.URL.Path))
		if err == nil {
			h.files.ServeHTTP(w, r)
			return
		}
	}
	r.URL.Path = "/index.html"
	h.files.ServeHTTP(w, r)
}

func embeddedFS() fs.FS {
	value, _ := fs.Sub(assets, "dist")
	return value
}
