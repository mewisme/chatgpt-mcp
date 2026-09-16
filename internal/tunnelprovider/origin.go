package tunnelprovider

import (
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
)

func ResolveOrigin(cfg config.Config, kind string) (Origin, error) {
	switch strings.TrimSpace(kind) {
	case OriginMCP:
		if err := MCPExposureError(cfg); err != nil {
			return Origin{}, err
		}
		if cfg.Server.Port <= 0 {
			return Origin{}, ErrListenerNotReady
		}
		return Origin{Kind: OriginMCP, URL: loopbackURL(cfg.Server.Port), PublicPath: "/mcp"}, nil
	case OriginAdmin:
		if err := AdminExposureError(cfg); err != nil {
			return Origin{}, err
		}
		if cfg.Admin.Port <= 0 {
			return Origin{}, ErrListenerNotReady
		}
		return Origin{Kind: OriginAdmin, URL: loopbackURL(cfg.Admin.Port), PublicPath: "/"}, nil
	case OriginPrivate:
		return Origin{}, fmt.Errorf("%w: private MCP bridge is owned by core runtime", ErrUnsupportedOrigin)
	default:
		return Origin{}, fmt.Errorf("%w: %q", ErrUnsupportedOrigin, kind)
	}
}

func MCPExposureError(cfg config.Config) error {
	if !cfg.Server.Enabled {
		return ErrMCPHTTPDisabled
	}
	if !cfg.Auth.MCPEnabled {
		return ErrMCPAuthDisabled
	}
	if strings.TrimSpace(cfg.Auth.MCPTokenHash) == "" {
		return ErrMCPTokenMissing
	}
	return nil
}

func AdminExposureError(cfg config.Config) error {
	if !cfg.Admin.Enabled {
		return ErrAdminHTTPDisabled
	}
	if !cfg.Auth.AdminEnabled {
		return ErrAdminAuthDisabled
	}
	if strings.TrimSpace(cfg.Auth.AdminTokenHash) == "" {
		return ErrAdminTokenMissing
	}
	return nil
}

func (o Origin) StartParams(target string) runtimeplugin.StartParams {
	return runtimeplugin.StartParams{Target: strings.TrimSpace(target), Origin: o.URL, PublicPath: o.PublicPath}
}

func loopbackURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}
