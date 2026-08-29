package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type Config struct {
	Server ServerConfig  `json:"server"`
	Admin  AdminConfig   `json:"admin"`
	Auth   AuthConfig    `json:"auth"`
	Tunnel tunnel.Config `json:"tunnel"`
}

type ServerConfig struct {
	Port   int  `json:"port"`
	Expose bool `json:"expose"`
}

type AdminConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

type AuthConfig struct {
	MCPEnabled     bool   `json:"mcp_enabled"`
	AdminEnabled   bool   `json:"admin_enabled"`
	MCPTokenHash   string `json:"mcp_token_hash,omitempty"`
	AdminTokenHash string `json:"admin_token_hash,omitempty"`
}

func Default() Config {
	return Config{Server: ServerConfig{Port: 37421, Expose: false}, Admin: AdminConfig{Enabled: true, Port: 37422}, Auth: AuthConfig{MCPEnabled: true, AdminEnabled: true}, Tunnel: tunnel.Config{Enabled: false}}
}

func Path() string { return DefaultPath() }

func Load() (Config, error) {
	return loadAt(Path(), TunnelSecretPath())
}

func loadAt(configPath, secretPath string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if err := migrateLegacyServerConfig(data, &cfg); err != nil {
		return cfg, err
	}
	secret, err := loadTunnelSecretAt(secretPath)
	if err != nil {
		return cfg, err
	}
	if secret != "" {
		cfg.Tunnel.APIKey = secret
	}
	return cfg, nil
}

func migrateLegacyServerConfig(data []byte, cfg *Config) error {
	var legacy struct {
		Server struct {
			Host   string `json:"host"`
			Expose *bool  `json:"expose"`
		} `json:"server"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	if legacy.Server.Expose != nil || strings.TrimSpace(legacy.Server.Host) == "" {
		return nil
	}
	host := strings.Trim(strings.ToLower(strings.TrimSpace(legacy.Server.Host)), "[]")
	cfg.Server.Expose = host != "127.0.0.1" && host != "::1" && host != "localhost"
	return nil
}

func Save(cfg Config) error {
	return saveAt(Path(), TunnelSecretPath(), cfg)
}

func saveAt(configPath, secretPath string, cfg Config) error {
	if err := saveTunnelSecretAt(secretPath, cfg.Tunnel.APIKey); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	persisted := cfg
	persisted.Tunnel.APIKey = ""
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(configPath, append(data, '\n'), 0600)
}
