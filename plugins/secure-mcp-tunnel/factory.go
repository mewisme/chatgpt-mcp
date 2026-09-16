package securemcptunnel

import (
	"os"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	tunnelclient "github.com/openai/tunnel-client"

	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func OpenAIFactory(log *logger.Logger) tunnel.BackendFactory {
	if log == nil {
		log = logger.NewWithWriter(logger.Info, os.Stderr)
	}
	return func(cfg tunnel.Config, transport sdkmcp.Transport) (tunnel.Backend, error) {
		if st, ok := transport.(*sdkmcp.StreamableClientTransport); ok && st != nil {
			return newHTTPStreamableBackend(cfg, log, st)
		}
		return tunnelclient.New(embedClientConfig(cfg, log), wrapPluginMCPTransport(transport, cfg.ID, ""))
	}
}

func embedClientConfig(cfg tunnel.Config, log *logger.Logger) tunnelclient.Config {
	return tunnelclient.Config{
		TunnelID:            cfg.ID,
		APIKey:              cfg.APIKey,
		ControlPlaneBaseURL: cfg.ControlPlaneBaseURL,
		OrganizationID:      cfg.OrganizationID,
		PollTimeout:         2 * time.Second,
		LogWriter:           log.LineWriter("TUNNEL"),
	}
}
