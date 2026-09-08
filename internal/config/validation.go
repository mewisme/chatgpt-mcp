package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/caveman"
	"go.mewis.me/chatgpt-mcp/internal/commandpattern"
	"go.mewis.me/chatgpt-mcp/internal/ponytail"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func Validate(cfg Config) error {
	if err := ValidateMCPTransports(cfg); err != nil {
		return err
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535: %d", cfg.Server.Port)
	}
	if cfg.Admin.Enabled && (cfg.Admin.Port < 1 || cfg.Admin.Port > 65535) {
		return fmt.Errorf("admin port must be between 1 and 65535: %d", cfg.Admin.Port)
	}
	if cfg.Server.Enabled && cfg.Admin.Enabled && cfg.Admin.Port == cfg.Server.Port {
		return errors.New("admin port must differ from server port")
	}
	if _, err := NormalizeAllowDirs(cfg.Permissions.AllowDirs); err != nil {
		return err
	}
	if _, err := NormalizeShellPath(cfg.Shell.Path); err != nil {
		return err
	}
	if _, err := NormalizeShellApprovalPolicy(cfg.Shell.ApprovalPolicy); err != nil {
		return err
	}
	if _, err := NormalizeShellApprovalCommands(cfg.Shell.ApprovalAllowCommands); err != nil {
		return err
	}
	if _, err := NormalizeShellApprovalCommands(cfg.Shell.ApprovalDenyCommands); err != nil {
		return err
	}
	if _, err := NormalizeShellEnvironmentPolicy(cfg.Shell.EnvironmentPolicy); err != nil {
		return err
	}
	if _, err := NormalizeShellEnvironmentAllow(cfg.Shell.EnvironmentAllow); err != nil {
		return err
	}
	if _, err := NormalizeShellSandboxPolicy(cfg.Shell.SandboxPolicy); err != nil {
		return err
	}
	if _, err := NormalizeShellNetworkPolicy(cfg.Shell.NetworkPolicy); err != nil {
		return err
	}
	if _, ok := ponytail.NormalizeRuntimeMode(cfg.Features.Ponytail.Mode); !ok {
		return fmt.Errorf("features.ponytail.mode must be lite, full, or ultra: %q", cfg.Features.Ponytail.Mode)
	}
	if _, ok := caveman.NormalizeRuntimeMode(cfg.Features.Caveman.Mode); !ok {
		return fmt.Errorf("features.caveman.mode must be lite, full, ultra, wenyan-lite, wenyan-full, or wenyan-ultra: %q", cfg.Features.Caveman.Mode)
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
	if exposure.Mode != ExposureNone && (cfg.Server.Enabled || cfg.Admin.Enabled) {
		if !cfg.Server.AllowInsecureHTTP {
			return errors.New("non-loopback HTTP exposure requires server.allow_insecure_http=true; prefer Secure MCP Tunnel or a TLS reverse proxy")
		}
		if cfg.Server.Enabled && (!cfg.Auth.MCPEnabled || cfg.Auth.MCPTokenHash == "") {
			return errors.New("network exposure requires MCP authentication with a configured token; run chatgpt-mcp auth mcp create")
		}
		if cfg.Admin.Enabled && (!cfg.Auth.AdminEnabled || cfg.Auth.AdminTokenHash == "") {
			return errors.New("network exposure with the admin endpoint enabled requires admin authentication with a configured token; run chatgpt-mcp auth admin create")
		}
	}
	if cfg.Server.Enabled && cfg.Auth.MCPEnabled && cfg.Auth.MCPTokenHash == "" {
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

func NormalizeShellApprovalPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "balanced":
		return "balanced", nil
	case "allow":
		return "allow", nil
	case "strict":
		return "strict", nil
	case "deny":
		return "deny", nil
	default:
		return "", fmt.Errorf("shell approval policy must be allow, balanced, strict, or deny: %q", value)
	}
}

func NormalizeShellApprovalCommands(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return nil, fmt.Errorf("shell approval command pattern must be a single line: %q", value)
		}
		if err := commandpattern.Validate(value); err != nil {
			return nil, err
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func NormalizeShellEnvironmentPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto", nil
	case "inherit":
		return "inherit", nil
	case "filtered":
		return "filtered", nil
	case "minimal":
		return "minimal", nil
	default:
		return "", fmt.Errorf("shell environment policy must be auto, inherit, filtered, or minimal: %q", value)
	}
}

func NormalizeShellSandboxPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto", nil
	case "off":
		return "off", nil
	case "required":
		return "required", nil
	default:
		return "", fmt.Errorf("shell sandbox policy must be auto, off, or required: %q", value)
	}
}

func NormalizeShellNetworkPolicy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto", nil
	case "inherit":
		return "inherit", nil
	case "deny":
		return "deny", nil
	default:
		return "", fmt.Errorf("shell network policy must be auto, inherit, or deny: %q", value)
	}
}

func NormalizeShellEnvironmentAllow(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for index, r := range value {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' && !(index > 0 && r >= '0' && r <= '9') {
				return nil, fmt.Errorf("shell environment variable name is invalid: %q", value)
			}
			if index == 0 && r >= '0' && r <= '9' {
				return nil, fmt.Errorf("shell environment variable name is invalid: %q", value)
			}
		}
		upper := strings.ToUpper(value)
		switch upper {
		case "CHATGPT_MCP_TOOL_CONTEXT", "CHATGPT_MCP_CONTROL_APPROVAL", "CHATGPT_MCP_CONFIG_DIR":
			return nil, fmt.Errorf("shell environment allow list cannot override protected variable %s", value)
		}
		if seen[upper] {
			continue
		}
		seen[upper] = true
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToUpper(result[i]) < strings.ToUpper(result[j]) })
	return result, nil
}

func ValidateMCPTransports(cfg Config) error {
	if !cfg.Server.Enabled && !cfg.Tunnel.Enabled {
		return errors.New("at least one MCP transport must be enabled: MCP HTTP (server.enabled) or OpenAI Secure MCP Tunnel (tunnel.enabled)")
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
