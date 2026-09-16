package securemcptunnel

import (
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	tunnelclient "github.com/openai/tunnel-client"

	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func OpenAIFactory(log *logger.Logger) tunnel.BackendFactory {
	if log == nil {
		log = logger.New(logger.Info)
	}
	return func(cfg tunnel.Config, transport sdkmcp.Transport) (tunnel.Backend, error) {
		return tunnelclient.New(tunnelclient.Config{
			TunnelID:            cfg.ID,
			APIKey:              cfg.APIKey,
			ControlPlaneBaseURL: cfg.ControlPlaneBaseURL,
			OrganizationID:      cfg.OrganizationID,
			PollTimeout:         2 * time.Second,
			LogWriter:           log.LineWriter("TUNNEL"),
		}, withSessionTransport(transport, cfg.ID, ""))
	}
}
