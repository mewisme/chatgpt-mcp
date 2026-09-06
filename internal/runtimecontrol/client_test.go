package runtimecontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestRequestUsesAuthenticatedLoopbackState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/test" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer runtime-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["value"] != "ok" {
			t.Fatalf("input=%#v", input)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "done"})
	}))
	defer server.Close()
	root := setupRuntimeControlRoot(t)
	writeRuntimeControlState(t, root, server.URL, "runtime-secret")
	var output map[string]string
	state, err := Request(t.Context(), http.MethodPost, "/test", map[string]string{"value": "ok"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if state.PID != os.Getpid() || output["result"] != "done" {
		t.Fatalf("state=%#v output=%#v", state, output)
	}
}

func TestRequestPropagatesStructuredRuntimeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "request already resolved"})
	}))
	defer server.Close()
	root := setupRuntimeControlRoot(t)
	writeRuntimeControlState(t, root, server.URL, "runtime-secret")
	if _, err := Request(t.Context(), http.MethodGet, "/test", nil, nil); err == nil || err.Error() != "request already resolved" {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadRejectsMissingOrNonLoopbackState(t *testing.T) {
	root := setupRuntimeControlRoot(t)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "no running server") {
		t.Fatalf("missing err=%v", err)
	}
	state := State{PID: os.Getpid(), Address: "8.8.8.8:1234", Token: "token", ConfigRoot: root}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "not loopback") {
		t.Fatalf("non-loopback err=%v", err)
	}
}

func TestRequestRejectsRelativePath(t *testing.T) {
	root := setupRuntimeControlRoot(t)
	writeRuntimeControlState(t, root, "http://127.0.0.1:1", "runtime-secret")
	if _, err := Request(context.Background(), http.MethodGet, "requests", nil, nil); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("err=%v", err)
	}
}

func setupRuntimeControlRoot(t *testing.T) string {
	t.Helper()
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeRuntimeControlState(t *testing.T, root, rawURL, token string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	state := State{PID: os.Getpid(), Address: parsed.Host, Token: token, StartedAt: time.Now().UTC(), ConfigRoot: root}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), data, 0600); err != nil {
		t.Fatal(err)
	}
}
