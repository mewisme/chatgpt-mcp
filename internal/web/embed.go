package web

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

const missingAdminUI = "Admin UI plugin is not installed or enabled.\nInstall with: cgm plugin install admin-ui\n"

func Handler(store *pluginpkg.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		static, err := adminUIFS(store)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(missingAdminUI))
			return
		}
		spaHandler{static: static, files: http.FileServer(http.FS(static))}.ServeHTTP(w, r)
	})
}

func adminUIFS(store *pluginpkg.Store) (fs.FS, error) {
	if store == nil {
		return nil, errors.New("plugin store is unavailable")
	}
	resolver, err := pluginpkg.NewResolver(store)
	if err != nil {
		return nil, err
	}
	provider, err := resolver.Resolve(pluginpkg.CapabilityWebUIAdmin)
	if err != nil {
		return nil, err
	}
	return os.DirFS(filepath.Dir(provider.Path)), nil
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
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
