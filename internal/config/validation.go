package config

import (
	"errors"
	"fmt"
	"strings"
)

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.Server.Host) == "" {
		return errors.New("server host is required")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535: %d", cfg.Server.Port)
	}
	if cfg.Admin.Enabled && (cfg.Admin.Port < 1 || cfg.Admin.Port > 65535) {
		return fmt.Errorf("admin port must be between 1 and 65535: %d", cfg.Admin.Port)
	}
	if cfg.Auth.MCPEnabled && cfg.Auth.MCPTokenHash == "" {
		return errors.New("MCP auth is enabled but no token is configured; run chatgpt-mcp auth mcp-create")
	}
	if cfg.Admin.Enabled && cfg.Auth.AdminEnabled && cfg.Auth.AdminTokenHash == "" {
		return errors.New("admin auth is enabled but no token is configured; run chatgpt-mcp auth admin-create")
	}
	if cfg.Tunnel.Enabled && strings.TrimSpace(cfg.Tunnel.Command) == "" {
		return errors.New("tunnel is enabled but command is empty")
	}
	return nil
}
