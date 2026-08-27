package config

import "testing"

func TestTunnelOrigin(t *testing.T) {
	cfg := Default()
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 4321
	if got := TunnelOrigin(cfg); got != "http://127.0.0.1:4321" {
		t.Fatalf("unexpected origin: %s", got)
	}
}

func TestValidateRequiresAuthTokens(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing auth token validation error")
	}
	cfg.Auth.MCPTokenHash = "configured"
	cfg.Auth.AdminTokenHash = "configured"
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTunnelCommand(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel.Enabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected tunnel command validation error")
	}
}
