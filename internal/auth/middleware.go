package auth

import (
	"net/http"
	"strings"
)

func Middleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || ValidateRequestToken(r, token) {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func ValidateRequestToken(r *http.Request, expected string) bool {
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") && strings.TrimPrefix(header, "Bearer ") == expected {
		return true
	}
	if r.Header.Get("X-MCP-Token") == expected {
		return true
	}
	return false
}
