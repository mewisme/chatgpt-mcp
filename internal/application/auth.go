package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

type AuthStatus struct {
	MCPEnabled      bool
	MCPConfigured   bool
	AdminEnabled    bool
	AdminConfigured bool
}

func GetAuthStatus() (AuthStatus, error) {
	cfg, err := config.Load()
	if err != nil {
		return AuthStatus{}, err
	}
	return authStatus(cfg), nil
}

func RotateAuthToken(ctx context.Context, kind string) (string, AuthStatus, error) {
	kind, err := normalizeAuthKind(kind)
	if err != nil {
		return "", AuthStatus{}, err
	}
	previous, err := config.Load()
	if err != nil {
		return "", AuthStatus{}, err
	}
	cfg := previous
	token := auth.GenerateToken(kind)
	hash := auth.HashToken(token)
	if kind == "mcp" {
		cfg.Auth.MCPTokenHash = hash
		cfg.Auth.MCPEnabled = true
	} else {
		cfg.Auth.AdminTokenHash = hash
		cfg.Auth.AdminEnabled = true
		cfg.Admin.Enabled = true
	}
	if err := config.Validate(cfg); err != nil {
		return "", AuthStatus{}, err
	}
	if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
		return "", AuthStatus{}, err
	}
	return token, authStatus(cfg), nil
}

func SetAuthEnabled(ctx context.Context, kind string, enabled bool) (AuthStatus, error) {
	kind, err := normalizeAuthKind(kind)
	if err != nil {
		return AuthStatus{}, err
	}
	previous, err := config.Load()
	if err != nil {
		return AuthStatus{}, err
	}
	cfg := previous
	if kind == "mcp" {
		if enabled && cfg.Auth.MCPTokenHash == "" {
			return AuthStatus{}, errors.New("MCP token is not configured; create one first")
		}
		cfg.Auth.MCPEnabled = enabled
	} else {
		if enabled && cfg.Auth.AdminTokenHash == "" {
			return AuthStatus{}, errors.New("admin token is not configured; create one first")
		}
		cfg.Auth.AdminEnabled = enabled
	}
	if err := config.Validate(cfg); err != nil {
		return AuthStatus{}, err
	}
	if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
		return AuthStatus{}, err
	}
	return authStatus(cfg), nil
}

func normalizeAuthKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "mcp" && kind != "admin" {
		return "", fmt.Errorf("unsupported auth kind: %s", kind)
	}
	return kind, nil
}

func authStatus(cfg config.Config) AuthStatus {
	return AuthStatus{
		MCPEnabled: cfg.Auth.MCPEnabled, MCPConfigured: cfg.Auth.MCPTokenHash != "",
		AdminEnabled: cfg.Auth.AdminEnabled, AdminConfigured: cfg.Auth.AdminTokenHash != "",
	}
}
