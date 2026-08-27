package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenHashRoundTrip(t *testing.T) {
	token := GenerateToken("test")
	first := HashToken(token)
	second := HashToken(token)
	if first != second {
		t.Fatal("random bearer token hashes should be deterministic")
	}
	if !VerifyToken(token, first) {
		t.Fatal("expected token verification to succeed")
	}
	if VerifyToken(token+"x", first) {
		t.Fatal("expected invalid token verification to fail")
	}
}

func TestHashedMiddlewareRequiresConfiguredToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	HashedMiddleware(true, "", next).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}

	token := GenerateToken("test")
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	HashedMiddleware(true, HashToken(token), next).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
}
