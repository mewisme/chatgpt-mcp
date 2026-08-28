package config

import (
	"fmt"
	"strings"
)

type Preset struct {
	Name             string       `json:"name"`
	Description      string       `json:"description"`
	Server           ServerConfig `json:"server"`
	Admin            AdminConfig  `json:"admin"`
	MCPAuthEnabled   bool         `json:"mcp_auth_enabled"`
	AdminAuthEnabled bool         `json:"admin_auth_enabled"`
	TunnelEnabled    bool         `json:"tunnel_enabled"`
}

var builtInPresets = []Preset{
	{
		Name: "default", Description: "Loopback MCP and admin endpoints with authentication enabled.",
		Server: ServerConfig{Host: "127.0.0.1", Port: 37421}, Admin: AdminConfig{Enabled: true, Port: 37422},
		MCPAuthEnabled: true, AdminAuthEnabled: true, TunnelEnabled: false,
	},
	{
		Name: "headless", Description: "Loopback MCP endpoint only; admin UI disabled.",
		Server: ServerConfig{Host: "127.0.0.1", Port: 37421}, Admin: AdminConfig{Enabled: false, Port: 37422},
		MCPAuthEnabled: true, AdminAuthEnabled: true, TunnelEnabled: false,
	},
	{
		Name: "lan", Description: "Expose MCP on all interfaces while keeping admin UI disabled.",
		Server: ServerConfig{Host: "0.0.0.0", Port: 37421}, Admin: AdminConfig{Enabled: false, Port: 37422},
		MCPAuthEnabled: true, AdminAuthEnabled: true, TunnelEnabled: false,
	},
	{
		Name: "lan-admin", Description: "Expose MCP and authenticated admin UI on all interfaces.",
		Server: ServerConfig{Host: "0.0.0.0", Port: 37421}, Admin: AdminConfig{Enabled: true, Port: 37422},
		MCPAuthEnabled: true, AdminAuthEnabled: true, TunnelEnabled: false,
	},
}

func Presets() []Preset {
	return append([]Preset(nil), builtInPresets...)
}

func PresetNames() []string {
	names := make([]string, len(builtInPresets))
	for index, preset := range builtInPresets {
		names[index] = preset.Name
	}
	return names
}

func PresetByName(name string) (Preset, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, preset := range builtInPresets {
		if preset.Name == key {
			return preset, nil
		}
	}
	return Preset{}, fmt.Errorf("unknown config preset %q; available: %s", name, strings.Join(PresetNames(), ", "))
}

func ApplyPreset(cfg *Config, name string) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	preset, err := PresetByName(name)
	if err != nil {
		return err
	}
	next := *cfg
	next.Server = preset.Server
	next.Admin = preset.Admin
	next.Auth.MCPEnabled = preset.MCPAuthEnabled
	next.Auth.AdminEnabled = preset.AdminAuthEnabled
	next.Tunnel.Enabled = preset.TunnelEnabled
	if err := Validate(next); err != nil {
		return err
	}
	*cfg = next
	return nil
}

func MatchPreset(cfg Config) string {
	for _, preset := range builtInPresets {
		if cfg.Server == preset.Server &&
			cfg.Admin == preset.Admin &&
			cfg.Auth.MCPEnabled == preset.MCPAuthEnabled &&
			cfg.Auth.AdminEnabled == preset.AdminAuthEnabled &&
			cfg.Tunnel.Enabled == preset.TunnelEnabled {
			return preset.Name
		}
	}
	return "custom"
}
