package tunnelprovider

import (
	"errors"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

func TestResolveOriginRequiresPublicAuth(t *testing.T) {
	cfg := config.Default()
	if _, err := ResolveOrigin(cfg, OriginMCP); !errors.Is(err, ErrMCPTokenMissing) {
		t.Fatalf("err = %v", err)
	}
	cfg.Auth.MCPEnabled = false
	if _, err := ResolveOrigin(cfg, OriginMCP); !errors.Is(err, ErrMCPAuthDisabled) {
		t.Fatalf("err = %v", err)
	}
	cfg.Server.AllowUnauthenticatedLoopback = true
	if _, err := ResolveOrigin(cfg, OriginMCP); !errors.Is(err, ErrMCPAuthDisabled) {
		t.Fatalf("loopback exception accepted: %v", err)
	}
	cfg.Server.Enabled = false
	if _, err := ResolveOrigin(cfg, OriginMCP); !errors.Is(err, ErrMCPHTTPDisabled) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveOriginAdminAndLoopbackURL(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	mcp, err := ResolveOrigin(cfg, OriginMCP)
	if err != nil {
		t.Fatal(err)
	}
	if mcp.URL != "http://127.0.0.1:37421" || mcp.PublicPath != "/mcp" {
		t.Fatalf("mcp = %#v", mcp)
	}
	admin, err := ResolveOrigin(cfg, OriginAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if admin.URL != "http://127.0.0.1:37422" || admin.PublicPath != "/" {
		t.Fatalf("admin = %#v", admin)
	}
	if _, err := ResolveOrigin(cfg, "tcp"); !errors.Is(err, ErrUnsupportedOrigin) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartParamsNeverIncludeSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "super-secret-token-hash"
	origin, err := ResolveOrigin(cfg, OriginMCP)
	if err != nil {
		t.Fatal(err)
	}
	params := origin.StartParams("mcp")
	if strings.Contains(params.Origin, "secret") || params.Origin != origin.URL {
		t.Fatalf("params = %#v", params)
	}
	if params.PublicPath != "/mcp" || params.Target != "mcp" {
		t.Fatalf("params = %#v", params)
	}
}
