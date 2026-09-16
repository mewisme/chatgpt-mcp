package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestStartSecureMCPInstanceMissingPlugin(t *testing.T) {
	_, err := StartSecureMCPInstance(context.Background(), runtimeplugin.NewHost(), "tunnel_a")
	if !errors.Is(err, tunnel.ErrPluginMissing) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "cgm plugin install "+tunnel.PluginIDSecureMCP) {
		t.Fatalf("missing repair hint: %v", err)
	}
}

func TestStartSecureMCPInstanceRequiresID(t *testing.T) {
	_, err := StartSecureMCPInstance(context.Background(), runtimeplugin.NewHost(), "  ")
	if err == nil || !strings.Contains(err.Error(), "tunnel id is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestStopSecureMCPInstanceMissingPluginDoesNotUseCoreFallback(t *testing.T) {
	_, err := StopSecureMCPInstance(context.Background(), runtimeplugin.NewHost(), true, "tunnel_a")
	if !errors.Is(err, tunnel.ErrPluginMissing) {
		t.Fatalf("err = %v", err)
	}
}

func TestErrIfLastUsableMCPTransportTwoInstances(t *testing.T) {
	statuses := []tunnel.Status{
		{ID: "tunnel_a", Enabled: true, Ready: true},
		{ID: "tunnel_b", Enabled: true, Ready: true},
	}
	if err := ErrIfLastUsableMCPTransport(false, statuses, "tunnel_a"); err != nil {
		t.Fatal(err)
	}
	if err := ErrIfLastUsableMCPTransport(false, statuses[:1], "tunnel_a"); err == nil || !strings.Contains(err.Error(), "cannot stop the last usable MCP transport") {
		t.Fatalf("err = %v", err)
	}
	if err := ErrIfLastUsableMCPTransport(true, statuses[:1], "tunnel_a"); err != nil {
		t.Fatal(err)
	}
}
