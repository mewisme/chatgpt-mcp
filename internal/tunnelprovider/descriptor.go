package tunnelprovider

import (
	"errors"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
)

const (
	Prefix      = "tunnel/"
	OriginMCP   = "mcp-http"
	OriginAdmin = "admin-http"
)

var (
	ErrMCPHTTPDisabled   = errors.New("mcp HTTP is disabled")
	ErrMCPAuthDisabled   = errors.New("direct MCP HTTP authentication is disabled")
	ErrMCPTokenMissing   = errors.New("direct MCP HTTP token is not configured")
	ErrAdminHTTPDisabled = errors.New("admin HTTP is disabled")
	ErrAdminAuthDisabled = errors.New("admin authentication is disabled")
	ErrAdminTokenMissing = errors.New("admin credential is not configured")
	ErrListenerNotReady  = errors.New("listener is not ready")
	ErrUnsupportedOrigin = errors.New("unsupported tunnel origin kind")
	ErrProviderMismatch  = errors.New("tunnel provider does not match capability")
	ErrInvalidDescriptor = errors.New("tunnel provider descriptor is invalid")
	ErrNotInstalled      = errors.New("tunnel provider is not installed or enabled")
)

type Origin struct {
	Kind       string
	URL        string
	PublicPath string
}

func Capability(provider string) string {
	return Prefix + strings.TrimSpace(provider)
}

func ProviderName(capability string) string {
	return strings.TrimPrefix(strings.TrimSpace(capability), Prefix)
}

func ValidateDescriptor(provider string, desc runtimeplugin.DescribeResult) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidDescriptor)
	}
	if strings.TrimSpace(desc.Provider) != provider {
		return fmt.Errorf("%w: capability %s descriptor %q", ErrProviderMismatch, provider, desc.Provider)
	}
	if strings.TrimSpace(desc.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidDescriptor)
	}
	if len(desc.Targets) == 0 {
		return fmt.Errorf("%w: at least one target is required", ErrInvalidDescriptor)
	}
	seen := map[string]struct{}{}
	for _, target := range desc.Targets {
		id := strings.TrimSpace(target.ID)
		if id == "" {
			return fmt.Errorf("%w: target id is required", ErrInvalidDescriptor)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate target %q", ErrInvalidDescriptor, id)
		}
		seen[id] = struct{}{}
		if err := validateOriginKind(target.OriginKind); err != nil {
			return err
		}
	}
	return nil
}

func Target(desc runtimeplugin.DescribeResult, id string) (runtimeplugin.Target, error) {
	id = strings.TrimSpace(id)
	for _, target := range desc.Targets {
		if target.ID == id {
			return target, nil
		}
	}
	return runtimeplugin.Target{}, fmt.Errorf("%w: unknown target %q", ErrInvalidDescriptor, id)
}

func validateOriginKind(kind string) error {
	switch strings.TrimSpace(kind) {
	case OriginMCP, OriginAdmin:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOrigin, kind)
	}
}
