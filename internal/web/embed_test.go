package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestHandlerRequiresAdminUIPlugin(t *testing.T) {
	store := testAdminUIStore(t, false)
	recorder := httptest.NewRecorder()
	SecurityHeaders(Handler(store)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "cgm plugin install admin-ui") {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	assertSecurityHeaders(t, recorder)
}

func TestSPAHandlerServesInstalledAdminUI(t *testing.T) {
	store := testAdminUIStore(t, true)
	handler := SecurityHeaders(Handler(store))
	for _, target := range []string{"/", "/index.html", "/overview", "/workspaces", "/tools", "/servers", "/tunnel", "/activity", "/settings"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Admin UI test") {
			t.Fatalf("%s: response = %d %q", target, recorder.Code, recorder.Body.String())
		}
		if location := recorder.Header().Get("Location"); location != "" {
			t.Fatalf("%s: unexpected redirect to %s", target, location)
		}
		assertSecurityHeaders(t, recorder)
	}
}

func TestHandlerServesStaticAssetFromPluginPayload(t *testing.T) {
	store := testAdminUIStore(t, true)
	recorder := httptest.NewRecorder()
	Handler(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "console.log('admin')" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func testAdminUIStore(t *testing.T, installed bool) *pluginpkg.Store {
	t.Helper()
	root := t.TempDir()
	layout := pluginpkg.Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: "windows", Arch: "amd64", CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		return store
	}
	payload := filepath.Join(root, "payload")
	if err := os.MkdirAll(filepath.Join(payload, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "index.html"), []byte("<!doctype html><title>Admin UI test</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "assets", "app.js"), []byte("console.log('admin')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchema, ID: "admin-ui", Name: "Admin UI", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "web-ui",
		Provides: []pluginpkg.Capability{pluginpkg.CapabilityWebUIAdmin}, Permissions: []pluginpkg.Permission{},
		Platforms: map[string]pluginpkg.PlatformArtifact{"any/any": {Artifact: "admin-ui-1.0.0.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "index.html"}},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("admin-ui", "1.0.0", pluginpkg.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	return store
}

func assertSecurityHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("Content-Security-Policy") == "" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("security headers missing: %#v", recorder.Header())
	}
}
