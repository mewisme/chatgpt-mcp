package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
)

var (
	ErrMCPTokenMissing       = errors.New("direct MCP HTTP token is not configured")
	ErrMCPTokenNotRevealable = errors.New("direct MCP HTTP token is configured but not stored; rotate once with cgm auth mcp create")
)

type AuthStatus struct {
	MCPEnabled              bool
	MCPConfigured           bool
	MCPRevealable           bool
	MCPLegacyBearer         bool
	AdminEnabled            bool
	AdminConfigured         bool
	UnauthenticatedLoopback bool
	CleartextHTTP           bool
}

func GetAuthStatus() (AuthStatus, error) {
	return GetAuthStatusContext(context.Background())
}

func GetAuthStatusContext(ctx context.Context) (AuthStatus, error) {
	span := tracepkg.Start(ctx, "AUTH", "auth.status", "Loading authentication status")
	cfg, _, err := loadConfigTraced(ctx, "auth.status.config.load", "Loading configuration for authentication status")
	if err != nil {
		span.FailMessage("Authentication status load failed", err)
		return AuthStatus{}, err
	}
	status := authStatus(cfg)
	span.EndMessage("Authentication status loaded", tracepkg.Bool("mcp_enabled", status.MCPEnabled), tracepkg.Bool("mcp_configured", status.MCPConfigured), tracepkg.Bool("mcp_revealable", status.MCPRevealable), tracepkg.Bool("admin_enabled", status.AdminEnabled), tracepkg.Bool("admin_configured", status.AdminConfigured), tracepkg.Bool("unauthenticated_loopback", status.UnauthenticatedLoopback), tracepkg.Bool("cleartext_http", status.CleartextHTTP))
	return status, nil
}

func RotateAuthToken(ctx context.Context, kind string) (string, AuthStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "AUTH", "auth.token.rotate", "Rotating authentication token", tracepkg.String("requested_kind", kind))
	normalizeSpan := tracepkg.Start(ctx, "AUTH", "auth.kind.normalize", "Normalizing authentication kind", tracepkg.String("input", kind))
	kind, err := normalizeAuthKind(kind)
	if err != nil {
		normalizeSpan.FailMessage("Authentication kind normalization failed", err)
		span.FailMessage("Authentication token rotation failed", err)
		return "", AuthStatus{}, err
	}
	normalizeSpan.EndMessage("Authentication kind normalized", tracepkg.String("kind", kind))
	previous, _, err := loadConfigTraced(ctx, "auth.config.load", "Loading configuration for authentication")
	if err != nil {
		span.FailMessage("Authentication token rotation failed", err)
		return "", AuthStatus{}, err
	}
	cfg := previous
	generateSpan := tracepkg.Start(ctx, "AUTH", "auth.token.generate", "Generating authentication token", tracepkg.String("kind", kind))
	token := auth.GenerateToken(kind)
	generateSpan.EndMessage("Authentication token generated", tracepkg.String("kind", kind), tracepkg.Bool("generated", true))
	hashSpan := tracepkg.Start(ctx, "AUTH", "auth.token.hash", "Hashing authentication token", tracepkg.String("kind", kind))
	hash := auth.HashToken(token)
	hashSpan.EndMessage("Authentication token hashed", tracepkg.String("kind", kind))
	if kind == "mcp" {
		cfg.Auth.MCPTokenHash = hash
		cfg.Auth.MCPEnabled = true
	} else {
		cfg.Auth.AdminTokenHash = hash
		cfg.Auth.AdminEnabled = true
		cfg.Admin.Enabled = true
	}
	validateSpan := tracepkg.Start(ctx, "AUTH", "auth.config.validate", "Validating authentication configuration", tracepkg.String("kind", kind))
	if err := config.Validate(cfg); err != nil {
		validateSpan.FailMessage("Authentication configuration validation failed", err)
		span.FailMessage("Authentication token rotation failed", err)
		return "", AuthStatus{}, err
	}
	validateSpan.EndMessage("Authentication configuration validated", tracepkg.String("kind", kind))
	var restore func() error
	if kind == "mcp" {
		restore, err = replaceMCPToken(token)
		if err != nil {
			span.FailMessage("Authentication token rotation failed", err)
			return "", AuthStatus{}, err
		}
	}
	if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
		if restore != nil {
			_ = restore()
		}
		span.FailMessage("Authentication token rotation failed", err)
		return "", AuthStatus{}, err
	}
	status := authStatus(cfg)
	span.EndMessage("Authentication token rotated", tracepkg.String("kind", kind), tracepkg.Bool("enabled", authEnabled(status, kind)), tracepkg.Bool("configured", authConfigured(status, kind)), tracepkg.Bool("revealable", status.MCPRevealable))
	return token, status, nil
}

func SetAuthEnabled(ctx context.Context, kind string, enabled bool) (AuthStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "AUTH", "auth.state.set", "Setting authentication state", tracepkg.String("requested_kind", kind), tracepkg.Bool("enabled", enabled))
	normalizeSpan := tracepkg.Start(ctx, "AUTH", "auth.kind.normalize", "Normalizing authentication kind", tracepkg.String("input", kind))
	kind, err := normalizeAuthKind(kind)
	if err != nil {
		normalizeSpan.FailMessage("Authentication kind normalization failed", err)
		span.FailMessage("Authentication state update failed", err)
		return AuthStatus{}, err
	}
	normalizeSpan.EndMessage("Authentication kind normalized", tracepkg.String("kind", kind))
	previous, _, err := loadConfigTraced(ctx, "auth.config.load", "Loading configuration for authentication")
	if err != nil {
		span.FailMessage("Authentication state update failed", err)
		return AuthStatus{}, err
	}
	cfg := previous
	if kind == "mcp" {
		if enabled && cfg.Auth.MCPTokenHash == "" {
			err := errors.New("direct MCP HTTP token is not configured; create one first")
			span.FailMessage("Authentication state update failed", err)
			return AuthStatus{}, err
		}
		cfg.Auth.MCPEnabled = enabled
	} else {
		if enabled && cfg.Auth.AdminTokenHash == "" {
			err := errors.New("admin token is not configured; create one first")
			span.FailMessage("Authentication state update failed", err)
			return AuthStatus{}, err
		}
		cfg.Auth.AdminEnabled = enabled
	}
	validateSpan := tracepkg.Start(ctx, "AUTH", "auth.config.validate", "Validating authentication configuration", tracepkg.String("kind", kind), tracepkg.Bool("enabled", enabled))
	if err := config.Validate(cfg); err != nil {
		validateSpan.FailMessage("Authentication configuration validation failed", err)
		span.FailMessage("Authentication state update failed", err)
		return AuthStatus{}, err
	}
	validateSpan.EndMessage("Authentication configuration validated")
	if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
		span.FailMessage("Authentication state update failed", err)
		return AuthStatus{}, err
	}
	status := authStatus(cfg)
	span.EndMessage("Authentication state updated", tracepkg.String("kind", kind), tracepkg.Bool("enabled", authEnabled(status, kind)), tracepkg.Bool("configured", authConfigured(status, kind)))
	return status, nil
}

func RevealMCPToken() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.Auth.MCPTokenHash) == "" {
		return "", ErrMCPTokenMissing
	}
	token, err := config.GetMCPToken()
	if errors.Is(err, secretstore.ErrNotFound) || token == "" {
		return "", ErrMCPTokenNotRevealable
	}
	if err != nil {
		return "", err
	}
	if !auth.VerifyToken(token, cfg.Auth.MCPTokenHash) {
		return "", ErrMCPTokenNotRevealable
	}
	return token, nil
}

func replaceMCPToken(token string) (func() error, error) {
	previous, err := config.GetMCPToken()
	if errors.Is(err, secretstore.ErrNotFound) {
		previous, err = "", nil
	}
	if err != nil {
		return nil, err
	}
	if err := config.SetMCPToken(token); err != nil {
		return nil, err
	}
	return func() error { return config.SetMCPToken(previous) }, nil
}

func normalizeAuthKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "mcp" && kind != "admin" {
		return "", fmt.Errorf("unsupported auth kind: %s", kind)
	}
	return kind, nil
}

func authStatus(cfg config.Config) AuthStatus {
	status := AuthStatus{
		MCPEnabled: cfg.Auth.MCPEnabled, MCPConfigured: cfg.Auth.MCPTokenHash != "", MCPLegacyBearer: cfg.Auth.MCPLegacyBearer,
		AdminEnabled: cfg.Auth.AdminEnabled, AdminConfigured: cfg.Auth.AdminTokenHash != "",
		UnauthenticatedLoopback: config.UnauthenticatedLoopbackActive(cfg),
		CleartextHTTP:           config.CleartextHTTPActive(cfg),
	}
	if status.MCPConfigured {
		status.MCPRevealable = mcpTokenRevealable(cfg.Auth.MCPTokenHash)
	}
	return status
}

func mcpTokenRevealable(hash string) bool {
	token, err := config.GetMCPToken()
	return err == nil && token != "" && auth.VerifyToken(token, hash)
}

func authEnabled(status AuthStatus, kind string) bool {
	if kind == "admin" {
		return status.AdminEnabled
	}
	return status.MCPEnabled
}

func authConfigured(status AuthStatus, kind string) bool {
	if kind == "admin" {
		return status.AdminConfigured
	}
	return status.MCPConfigured
}
