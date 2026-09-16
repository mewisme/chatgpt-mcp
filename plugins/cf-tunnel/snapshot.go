package cftunnel

import (
	"errors"
	"fmt"
	"strings"
)

const (
	TargetMCP   = "mcp"
	TargetAdmin = "admin"
)

var (
	ErrMCPHTTPDisabled   = errors.New("mcp HTTP is disabled")
	ErrMCPAuthDisabled   = errors.New("direct MCP HTTP authentication is disabled")
	ErrMCPTokenMissing   = errors.New("direct MCP HTTP token is not configured")
	ErrAdminHTTPDisabled = errors.New("admin HTTP is disabled")
	ErrAdminAuthDisabled = errors.New("admin authentication is disabled")
	ErrAdminTokenMissing = errors.New("admin credential is not configured")
	ErrListenerNotReady  = errors.New("listener is not ready")
)

type Endpoint struct {
	Ready   bool
	Port    int
	AuthErr error
}

type Snapshot struct {
	PluginEnabled bool
	DesiredMCP    bool
	DesiredAdmin  bool
	MCP           Endpoint
	Admin         Endpoint
}

func loopbackOrigin(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func ParseTarget(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TargetMCP:
		return TargetMCP, nil
	case TargetAdmin:
		return TargetAdmin, nil
	case "all":
		return "all", nil
	default:
		return "", fmt.Errorf("cf-tunnel target must be mcp, admin, or all")
	}
}
