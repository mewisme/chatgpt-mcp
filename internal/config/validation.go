package config

import (
	"errors"
	"fmt"

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
	exposure := NormalizeExposure(cfg.Server.Expose)
	switch exposure.Mode {
	case ExposureNone, ExposureAll:
		if len(cfg.Server.Expose.Interfaces) != 0 {
			return errors.New("server expose interfaces must be empty unless mode is interfaces")
		}
	case ExposureInterfaces:
		if len(exposure.Interfaces) == 0 {
			return errors.New("server expose interfaces mode requires at least one interface")
		}
	default:
		return fmt.Errorf("server expose mode must be none, all, or interfaces: %q", cfg.Server.Expose.Mode)
	}
	if cfg.Auth.MCPEnabled && cfg.Auth.MCPTokenHash == "" {
		return errors.New("MCP auth is enabled but no token is configured; run chatgpt-mcp auth mcp-create")
	}
	if cfg.Admin.Enabled && cfg.Auth.AdminEnabled && cfg.Auth.AdminTokenHash == "" {
		return errors.New("admin auth is enabled but no token is configured; run chatgpt-mcp auth admin-create")
	}
	if err := tunnel.ValidateConfig(cfg.Tunnel); err != nil {
		return err
	}
	return nil
}
