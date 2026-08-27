package mcp

import "net/http"

const SessionHeader = "Mcp-Session-Id"

func SessionID(r *http.Request) string {
	return r.Header.Get(SessionHeader)
}

func SetSessionID(w http.ResponseWriter, id string) {
	w.Header().Set(SessionHeader, id)
}
