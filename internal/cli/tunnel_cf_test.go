package cli

import (
	"bytes"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func TestRenderCFTunnelStatusSeparatesFromSecureMCP(t *testing.T) {
	var out bytes.Buffer
	renderCFTunnelStatus(&out, config.Default(), &runtimecontrol.CFTunnelStatus{
		PluginEnabled: true,
		Targets: []runtimecontrol.CFTunnelTargetStatus{
			{Target: cftunnelplugin.TargetMCP, Desired: true, Ready: true, URL: "https://mcp.trycloudflare.com"},
			{Target: cftunnelplugin.TargetAdmin, Desired: true, LastError: "edge down"},
		},
	}, "all")
	text := out.String()
	for _, expected := range []string{"CF Tunnel", "not Secure MCP", "https://mcp.trycloudflare.com · ephemeral", "degraded · edge down"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
	if strings.Contains(text, "tunnel_") {
		t.Fatalf("leaked secure mcp id: %q", text)
	}
}

func TestRenderStatusCFTunnelOmitsWhenDisabled(t *testing.T) {
	var out bytes.Buffer
	renderStatusCFTunnel(&out, statusSnapshot{Config: config.Default()})
	if out.Len() != 0 {
		t.Fatalf("output = %q", out.String())
	}
}
