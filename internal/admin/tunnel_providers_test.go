package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestTunnelProvidersList(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	testutil.InstallStubTunnelPlugin(t, false)
	recorder := httptest.NewRecorder()
	New(API{Config: config.NewRuntimeStore(config.Default())}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tunnel-providers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"provider":"cf"`) {
		t.Fatalf("body=%s", recorder.Body.String())
	}
}

func TestTunnelProvidersListEmptyWithoutPlugin(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	recorder := httptest.NewRecorder()
	New(API{Config: config.NewRuntimeStore(config.Default())}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tunnel-providers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); strings.Contains(body, `"provider":"cf"`) {
		t.Fatalf("body=%s", body)
	}
}

func TestTunnelProviderMissing(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	recorder := httptest.NewRecorder()
	New(API{Config: config.NewRuntimeStore(config.Default())}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tunnel-providers/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
