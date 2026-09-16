package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestMCPTokenAPIRevealRotateAndHidesFromConfig(t *testing.T) {
	testutil.UseConfigRoot(t, filepath.Join(t.TempDir(), "config"))
	result, err := application.Initialize(application.InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(API{Config: config.NewRuntimeStore(result.Config)})

	configRecorder := httptest.NewRecorder()
	handler.ServeHTTP(configRecorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if configRecorder.Code != http.StatusOK || strings.Contains(configRecorder.Body.String(), result.MCPToken) || !strings.Contains(configRecorder.Body.String(), `"mcp_token_revealable":true`) {
		t.Fatalf("config=%d %s", configRecorder.Code, configRecorder.Body.String())
	}

	reveal := httptest.NewRecorder()
	handler.ServeHTTP(reveal, httptest.NewRequest(http.MethodGet, "/api/auth/mcp-token", nil))
	if reveal.Code != http.StatusOK {
		t.Fatalf("reveal=%d %s", reveal.Code, reveal.Body.String())
	}
	var shown mcpTokenView
	if err := json.Unmarshal(reveal.Body.Bytes(), &shown); err != nil || shown.Token != result.MCPToken || !shown.Revealable {
		t.Fatalf("reveal body=%s err=%v", reveal.Body.String(), err)
	}

	rotate := httptest.NewRecorder()
	handler.ServeHTTP(rotate, httptest.NewRequest(http.MethodPost, "/api/auth/mcp-token", nil))
	if rotate.Code != http.StatusOK {
		t.Fatalf("rotate=%d %s", rotate.Code, rotate.Body.String())
	}
	var rotated mcpTokenView
	if err := json.Unmarshal(rotate.Body.Bytes(), &rotated); err != nil || rotated.Token == "" || rotated.Token == result.MCPToken {
		t.Fatalf("rotate body=%s err=%v", rotate.Body.String(), err)
	}
	if strings.Contains(rotate.Body.String(), result.MCPToken) {
		t.Fatal("rotate returned previous token")
	}

	configAfter := httptest.NewRecorder()
	handler.ServeHTTP(configAfter, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if strings.Contains(configAfter.Body.String(), rotated.Token) || strings.Contains(configAfter.Body.String(), result.MCPToken) {
		t.Fatalf("config leaked token after rotate: %s", configAfter.Body.String())
	}
}

func TestMCPTokenAPILegacyHashOnly(t *testing.T) {
	testutil.UseConfigRoot(t, filepath.Join(t.TempDir(), "config"))
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "sha256$legacy"
	cfg.Auth.AdminTokenHash = "sha256$admin"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	handler := New(API{Config: config.NewRuntimeStore(cfg)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/mcp-token", nil))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "rotate once") {
		t.Fatalf("legacy reveal=%d %s", recorder.Code, recorder.Body.String())
	}
}
