package auth

import "net/http"

func Middleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || ValidateRequestToken(r, token) {
			next.ServeHTTP(w, r)
			return
		}
		unauthorized(w)
	})
}

func HashedMiddleware(enabled bool, tokenHash string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}
		if tokenHash != "" && ValidateRequestHash(r, tokenHash) {
			next.ServeHTTP(w, r)
			return
		}
		unauthorized(w)
	})
}

func ValidateRequestToken(r *http.Request, expected string) bool {
	return TokenFromRequest(r) == expected
}
func ValidateRequestHash(r *http.Request, expectedHash string) bool {
	token := TokenFromRequest(r)
	return token != "" && VerifyToken(token, expectedHash)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="chatgpt-mcp"`)
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}
