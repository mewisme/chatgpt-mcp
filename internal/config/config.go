package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/features"
	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type Config struct {
	Server      ServerConfig      `json:"server"`
	Admin       AdminConfig       `json:"admin"`
	Auth        AuthConfig        `json:"auth"`
	Permissions PermissionsConfig `json:"permissions"`
	Features    FeaturesConfig    `json:"features"`
	Tunnel      tunnel.Config     `json:"tunnel"`
}

type PermissionsConfig struct {
	AllowDirs []string `json:"allow_dirs"`
}

type ServerConfig struct {
	Port   int            `json:"port"`
	Expose ExposureConfig `json:"expose"`
}

type ExposureMode string

const (
	ExposureNone       ExposureMode = "none"
	ExposureAll        ExposureMode = "all"
	ExposureWildcard   ExposureMode = "0.0.0.0"
	ExposureInterfaces ExposureMode = "interfaces"
)

type ExposureConfig struct {
	Mode       ExposureMode `json:"mode"`
	Interfaces []string     `json:"interfaces"`
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

type FeaturesConfig = features.Config

func Default() Config {
	return Config{Server: ServerConfig{Port: 37421, Expose: ExposureConfig{Mode: ExposureNone, Interfaces: []string{}}}, Admin: AdminConfig{Enabled: true, Port: 37422}, Auth: AuthConfig{MCPEnabled: true, AdminEnabled: true}, Permissions: PermissionsConfig{AllowDirs: []string{}}, Features: features.Default(), Tunnel: tunnel.Config{Enabled: false}}
}

func (value *ExposureConfig) UnmarshalJSON(data []byte) error {
	var legacy bool
	if err := json.Unmarshal(data, &legacy); err == nil {
		if legacy {
			*value = ExposureConfig{Mode: ExposureWildcard, Interfaces: []string{}}
		} else {
			*value = ExposureConfig{Mode: ExposureNone, Interfaces: []string{}}
		}
		return nil
	}
	type exposureAlias ExposureConfig
	var decoded exposureAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("server.expose must be a boolean or exposure object: %w", err)
	}
	*value = NormalizeExposure(ExposureConfig(decoded))
	return nil
}

func NormalizeExposure(value ExposureConfig) ExposureConfig {
	value.Mode = ExposureMode(strings.ToLower(strings.TrimSpace(string(value.Mode))))
	if value.Mode != ExposureInterfaces {
		value.Interfaces = []string{}
		return value
	}
	names := make([]string, 0, len(value.Interfaces))
	seen := map[string]struct{}{}
	for _, name := range value.Interfaces {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	value.Interfaces = names
	return value
}

func ParseExposure(raw string) (ExposureConfig, error) {
	value := strings.TrimSpace(raw)
	switch strings.ToLower(value) {
	case "all":
		return ExposureConfig{Mode: ExposureAll, Interfaces: []string{}}, nil
	case "true", "0.0.0.0", "wildcard":
		return ExposureConfig{Mode: ExposureWildcard, Interfaces: []string{}}, nil
	case "false", "none":
		return ExposureConfig{Mode: ExposureNone, Interfaces: []string{}}, nil
	case "", "interfaces":
		return ExposureConfig{}, errors.New("server exposure must be none, all, 0.0.0.0, or a comma-separated interface list")
	}
	exposure := NormalizeExposure(ExposureConfig{Mode: ExposureInterfaces, Interfaces: strings.Split(value, ",")})
	if len(exposure.Interfaces) == 0 {
		return ExposureConfig{}, errors.New("server exposure interface list cannot be empty")
	}
	return exposure, nil
}

func ExposureEqual(left, right ExposureConfig) bool {
	left = NormalizeExposure(left)
	right = NormalizeExposure(right)
	if left.Mode != right.Mode || len(left.Interfaces) != len(right.Interfaces) {
		return false
	}
	for index := range left.Interfaces {
		if left.Interfaces[index] != right.Interfaces[index] {
			return false
		}
	}
	return true
}

func Load() (Config, error) {
	source, err := Source()
	if err != nil {
		return Config{}, err
	}
	return loadAt(source.Path, configformat.StructuredPathFrom(source.Path, "tunnel"))
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
	if err := configformat.UnmarshalPath(configPath, data, &cfg); err != nil {
		return cfg, err
	}
	cfg.Server.Expose = NormalizeExposure(cfg.Server.Expose)
	if err := migrateLegacyServerConfig(configPath, data, &cfg); err != nil {
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

func migrateLegacyServerConfig(path string, data []byte, cfg *Config) error {
	var legacy struct {
		Server map[string]any `json:"server"`
	}
	if err := configformat.UnmarshalPath(path, data, &legacy); err != nil {
		return err
	}
	if _, exists := legacy.Server["expose"]; exists {
		return nil
	}
	host, _ := legacy.Server["host"].(string)
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "" {
		return nil
	}
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		cfg.Server.Expose = ExposureConfig{Mode: ExposureNone, Interfaces: []string{}}
	} else if host == "0.0.0.0" {
		cfg.Server.Expose = ExposureConfig{Mode: ExposureWildcard, Interfaces: []string{}}
	} else {
		cfg.Server.Expose = ExposureConfig{Mode: ExposureAll, Interfaces: []string{}}
	}
	return nil
}

func Save(cfg Config) error {
	source, err := Source()
	if err != nil {
		return err
	}
	return saveAt(source.Path, configformat.StructuredPathFrom(source.Path, "tunnel"), cfg)
}

func SaveAs(cfg Config, format configformat.Format) error {
	path := PathForFormat(format)
	return saveAt(path, configformat.StructuredPathFrom(path, "tunnel"), cfg)
}

func saveAt(configPath, secretPath string, cfg Config) error {
	return saveAtWithSecretSaver(configPath, secretPath, cfg, saveTunnelSecretAt)
}

func saveAtWithSecretSaver(configPath, secretPath string, cfg Config, saveSecret func(string, string) error) error {
	root := filepath.Dir(configPath)
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err := configformat.MarkRoot(root); err != nil {
		return err
	}
	persisted := cfg
	allowDirs, err := NormalizeAllowDirs(persisted.Permissions.AllowDirs)
	if err != nil {
		return err
	}
	persisted.Permissions.AllowDirs = allowDirs
	persisted.Server.Expose = NormalizeExposure(persisted.Server.Expose)
	persisted.Tunnel.APIKey = ""
	data, err := configformat.MarshalPath(configPath, persisted)
	if err != nil {
		return err
	}
	configSnapshot, err := snapshotFile(configPath)
	if err != nil {
		return err
	}
	secretSnapshot, err := snapshotFile(secretPath)
	if err != nil {
		return err
	}
	if err := state.WriteFileAtomic(configPath, data, 0600); err != nil {
		return err
	}
	if err := saveSecret(secretPath, cfg.Tunnel.APIKey); err != nil {
		return errors.Join(err, restoreSnapshot(configPath, configSnapshot), restoreSnapshot(secretPath, secretSnapshot))
	}
	return nil
}

type fileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func snapshotFile(path string) (fileSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func restoreSnapshot(path string, snapshot fileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return state.WriteFileAtomic(path, snapshot.data, snapshot.mode)
}
