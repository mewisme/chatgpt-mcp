package config

import (
	"reflect"
	"testing"
)

func TestPresetNamesAreDeterministic(t *testing.T) {
	want := []string{"default", "headless", "lan", "lan-admin"}
	if got := PresetNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %#v, want %#v", got, want)
	}
}

func TestApplyPresetPreservesSecretsAndTunnelDetails(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPTokenHash = "mcp-secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	cfg.Tunnel.APIKey = "tunnel-secret"
	cfg.Tunnel.ID = "tunnel-id"
	cfg.Tunnel.ControlPlaneBaseURL = "https://api.openai.com"
	cfg.Tunnel.OrganizationID = "org-test"

	if err := ApplyPreset(&cfg, "lan"); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "0.0.0.0" || cfg.Admin.Enabled {
		t.Fatalf("preset not applied: %#v", cfg)
	}
	if cfg.Auth.MCPTokenHash != "mcp-secret" || cfg.Auth.AdminTokenHash != "admin-secret" {
		t.Fatal("auth secrets changed")
	}
	if cfg.Tunnel.APIKey != "tunnel-secret" || cfg.Tunnel.ID != "tunnel-id" ||
		cfg.Tunnel.ControlPlaneBaseURL != "https://api.openai.com" || cfg.Tunnel.OrganizationID != "org-test" {
		t.Fatal("tunnel details changed")
	}
}

func TestApplyPresetDoesNotMutateOnValidationFailure(t *testing.T) {
	cfg := Default()
	before := cfg
	if err := ApplyPreset(&cfg, "default"); err == nil {
		t.Fatal("preset unexpectedly bypassed missing token validation")
	}
	if !reflect.DeepEqual(cfg, before) {
		t.Fatalf("config mutated on failure: before=%#v after=%#v", before, cfg)
	}
}

func TestMatchPreset(t *testing.T) {
	cfg := Default()
	if got := MatchPreset(cfg); got != "default" {
		t.Fatalf("preset = %q", got)
	}
	cfg.Server.Port++
	if got := MatchPreset(cfg); got != "custom" {
		t.Fatalf("preset = %q", got)
	}
}

func TestUnknownPreset(t *testing.T) {
	if _, err := PresetByName("missing"); err == nil {
		t.Fatal("unknown preset was accepted")
	}
}
