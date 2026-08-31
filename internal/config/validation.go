package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func Validate(cfg Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535: %d", cfg.Server.Port)
	}
	if cfg.Admin.Enabled && (cfg.Admin.Port < 1 || cfg.Admin.Port > 65535) {
		return fmt.Errorf("admin port must be between 1 and 65535: %d", cfg.Admin.Port)
	}
	if cfg.Admin.Enabled && cfg.Admin.Port == cfg.Server.Port {
		return errors.New("admin port must differ from server port")
	}
	if _, err := NormalizeAllowDirs(cfg.Permissions.AllowDirs); err != nil {
		return err
	}
	if _, err := NormalizeShellPath(cfg.Shell.Path); err != nil {
		return err
	}
	exposure := NormalizeExposure(cfg.Server.Expose)
	switch exposure.Mode {
	case ExposureNone, ExposureAll, ExposureWildcard:
		if len(cfg.Server.Expose.Interfaces) != 0 {
			return errors.New("server expose interfaces must be empty unless mode is interfaces")
		}
	case ExposureInterfaces:
		if len(exposure.Interfaces) == 0 {
			return errors.New("server expose interfaces mode requires at least one interface")
		}
	default:
		return fmt.Errorf("server expose mode must be none, all, 0.0.0.0, or interfaces: %q", cfg.Server.Expose.Mode)
	}
	if exposure.Mode == ExposureWildcard {
		if !cfg.Auth.MCPEnabled || cfg.Auth.MCPTokenHash == "" {
			return errors.New("0.0.0.0 exposure requires MCP authentication with a configured token; run chatgpt-mcp auth mcp create")
		}
		if !cfg.Auth.AdminEnabled || cfg.Auth.AdminTokenHash == "" {
			return errors.New("0.0.0.0 exposure requires admin authentication with a configured token; run chatgpt-mcp auth admin create")
		}
	}
	if cfg.Auth.MCPEnabled && cfg.Auth.MCPTokenHash == "" {
		return errors.New("MCP auth is enabled but no token is configured; run chatgpt-mcp auth mcp create")
	}
	if cfg.Admin.Enabled && cfg.Auth.AdminEnabled && cfg.Auth.AdminTokenHash == "" {
		return errors.New("admin auth is enabled but no token is configured; run chatgpt-mcp auth admin create")
	}
	if err := tunnel.ValidateConfig(cfg.Tunnel); err != nil {
		return err
	}
	return nil
}

func NormalizeShellPath(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("shell path must be absolute: %q", value)
		}
		path = filepath.Clean(path)
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, path)
	}
	return result, nil
}

func NormalizeAllowDirs(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value)
		if path == "" || !filepath.IsAbs(path) {
			return nil, fmt.Errorf("permissions allow dir must be an absolute path: %q", value)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("permissions allow dir %s: %w", path, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("permissions allow dir is not a directory: %s", path)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("permissions allow dir %s: %w", path, err)
		}
		canonical = filepath.Clean(canonical)
		if _, exists := seen[canonical]; exists {
			return nil, fmt.Errorf("duplicate permissions allow dir: %s", canonical)
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
	}
	sort.Strings(result)
	return result, nil
}
