package securemcptunnel

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/config"

	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestOpenAIFactoryUsesHTTPStreamableForBridgeTransport(t *testing.T) {
	bridge := startProbeBridge(t)
	backend, err := OpenAIFactory(nil)(tunnel.Config{
		ID:     "tunnel_0123456789abcdef0123456789abcdef",
		APIKey: "sk-test",
	}, &sdkmcp.StreamableClientTransport{
		Endpoint: bridge.URL, DisableStandaloneSSE: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.(*runtimeClient); !ok {
		t.Fatalf("got %T", backend)
	}
}

func TestHTTPStreamableConfigUsesBridgeURL(t *testing.T) {
	cfg, err := httpStreamableConfig(tunnel.Config{
		ID:     "tunnel_0123456789abcdef0123456789abcdef",
		APIKey: "sk-test",
	}, &sdkmcp.StreamableClientTransport{
		Endpoint: "http://127.0.0.1:9/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCP.TransportKind != config.MCPTransportHTTPStreamable {
		t.Fatalf("kind=%q", cfg.MCP.TransportKind)
	}
	if cfg.MCP.ServerURL.String() != "http://127.0.0.1:9/mcp" {
		t.Fatalf("url=%q", cfg.MCP.ServerURL)
	}
	if len(cfg.MCP.ExtraHeaders) != 0 {
		t.Fatalf("headers=%v", cfg.MCP.ExtraHeaders)
	}
}
