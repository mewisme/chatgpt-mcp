package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const SessionHeader = "Mcp-Session-Id"

func ReadSessionID(r *http.Request) string { return r.Header.Get(SessionHeader) }

func EnsureSessionID(r *http.Request, store *SessionStore) string {
	if id := ReadSessionID(r); id != "" {
		if _, ok := store.Get(id); ok {
			return id
		}
	}
	id := randomSessionID()
	store.Set(&Session{ID: id})
	return id
}

func SetSessionID(w http.ResponseWriter, id string) { w.Header().Set(SessionHeader, id) }

func randomSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
