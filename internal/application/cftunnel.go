package application

import (
	"context"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

func MCPExposureError(cfg config.Config) error {
	return tunnelprovider.MCPExposureError(cfg)
}

func AdminExposureError(cfg config.Config) error {
	return tunnelprovider.AdminExposureError(cfg)
}

func StartCFTunnel(ctx context.Context, cfg config.Config, target string) error {
	return StartTunnelProvider(ctx, cfg, "cf", target)
}

func StopCFTunnel(ctx context.Context, target string) error {
	return StopTunnelProvider(ctx, "cf", target)
}
