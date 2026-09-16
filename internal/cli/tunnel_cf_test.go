package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
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

func TestRenderCFTunnelStatusReconnecting(t *testing.T) {
	var out bytes.Buffer
	renderCFTunnelStatus(&out, config.Default(), &runtimecontrol.CFTunnelStatus{
		PluginEnabled: true,
		Targets:       []runtimecontrol.CFTunnelTargetStatus{{Target: cftunnelplugin.TargetMCP, Desired: true, Restarting: true, LastError: "edge down"}},
	}, "all")
	if !strings.Contains(out.String(), "reconnecting · edge down") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLogCFTunnelLifecycleRedactsSecrets(t *testing.T) {
	previous := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = previous }()
	var output bytes.Buffer
	log := logger.NewWithOptions(logger.Options{Level: logger.Info, Mode: logger.ModeVerbose, Writer: &output})
	logCFTunnelLifecycle(log, cftunnelplugin.LifecycleEvent{State: cftunnelplugin.LifecycleDegraded, Target: "mcp", Error: "provision failed secret=<redacted>"})
	text := output.String()
	if !strings.Contains(text, "CF Tunnel degraded") || !strings.Contains(text, "target: mcp") {
		t.Fatalf("output = %q", text)
	}
}

func TestTunnelProviderMissingInstallHint(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cmd := tunnelCommand()
	cmd.SetArgs([]string{"missing", "status"})
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderStatusCFTunnelOmitsWhenDisabled(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	var out bytes.Buffer
	renderStatusCFTunnel(&out, statusSnapshot{Config: config.Default()})
	if out.Len() != 0 {
		t.Fatalf("output = %q", out.String())
	}
}
