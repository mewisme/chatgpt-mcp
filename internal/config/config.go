package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Server ServerConfig `json:"server"`
	Admin  AdminConfig  `json:"admin"`
	Auth   AuthConfig   `json:"auth"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type AdminConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

type AuthConfig struct {
	MCPEnabled   bool `json:"mcp_enabled"`
	AdminEnabled bool `json:"admin_enabled"`
}

func Default() Config {
	return Config{Server: ServerConfig{Host: "127.0.0.1", Port: 3000}, Admin: AdminConfig{Enabled: true, Port: 3001}, Auth: AuthConfig{MCPEnabled: true, AdminEnabled: true}}
}

func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "chatgpt-mcp", "config.json")
}

func Load() (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func Save(cfg Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
