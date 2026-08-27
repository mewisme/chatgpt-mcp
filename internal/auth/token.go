package auth

import "net/http"

func TokenFromRequest(r *http.Request) string {
	if value := r.Header.Get("Authorization"); len(value) > 7 && value[:7] == "Bearer " {
		return value[7:]
	}
	return r.Header.Get("X-MCP-Token")
}
