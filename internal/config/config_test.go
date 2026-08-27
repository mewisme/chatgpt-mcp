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
