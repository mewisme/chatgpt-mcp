package admin

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

func TestConfigPresetsAPI(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config/presets", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"current":"default"`) || !strings.Contains(recorder.Body.String(), `"lan-admin"`) {
		t.Fatalf("list body = %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config/presets/lan", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"lan"`) {
		t.Fatalf("show = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/config/presets/lan", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("apply = %d: %s", recorder.Code, recorder.Body.String())
	}
	got := store.Snapshot()
	if got.Server.Expose.Mode != config.ExposureAll || got.Admin.Enabled {
		t.Fatalf("preset not applied: %#v", got)
	}
	if got.Auth.MCPTokenHash != "mcp-secret" || got.Auth.AdminTokenHash != "admin-secret" {
		t.Fatal("preset changed auth secrets")
	}
}

func TestConfigPresetAPIValidationFailureDoesNotMutate(t *testing.T) {
	cfg := config.Default()
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/config/presets/default", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot(); !reflect.DeepEqual(got, config.Default()) {
		t.Fatalf("config mutated on failed apply: %#v", got)
	}
}

func TestConfigPresetAPINotFound(t *testing.T) {
	cfg := config.Default()
	handler := New(API{Config: config.NewRuntimeStore(cfg)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config/presets/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}
