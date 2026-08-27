package cli

import (
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func ensureAuth(cfg *config.Config) error {
	changed := false
	if cfg.Auth.MCPEnabled && cfg.Auth.MCPTokenHash == "" {
		cfg.Auth.MCPTokenHash = auth.HashToken(auth.GenerateToken("mcp"))
		changed = true
	}
	if cfg.Auth.AdminEnabled && cfg.Admin.Enabled && cfg.Auth.AdminTokenHash == "" {
		cfg.Auth.AdminTokenHash = auth.HashToken(auth.GenerateToken("admin"))
		changed = true
	}
	if changed {
		return config.Save(*cfg)
	}
	return nil
}
