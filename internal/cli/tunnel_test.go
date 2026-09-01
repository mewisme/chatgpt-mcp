package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestLogTunnelLifecycleReconnect(t *testing.T) {
	previous := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = previous }()

	var output bytes.Buffer
	log := logger.NewWithOptions(logger.Options{Level: logger.Info, Mode: logger.ModeVerbose, Writer: &output})
	logTunnelLifecycle(log, tunnel.LifecycleEvent{State: tunnel.LifecycleReconnecting, ID: "tunnel_test", Attempt: 3, RetryIn: 4 * time.Second})
	text := output.String()
	for _, expected := range []string{"→ Reconnecting tunnel", "tunnel_id: tunnel_test", "attempt: 3", "retry_in: 4s"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestConfigureManagedTunnelRequiresSeparateRuntimeKey(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel.AdminKey = "admin-only"
	cfg.Tunnel.AdminWorkspaceID = "ws_admin"
	metadata := tunnel.Metadata{ID: "tunnel_test", OrganizationIDs: []string{"org_test"}}
	if err := configureManagedTunnel(&cfg, metadata, "", false); err == nil {
		t.Fatal("admin key was accepted as a runtime key")
	}
	if err := configureManagedTunnel(&cfg, metadata, "runtime-key", true); err != nil {
		t.Fatal(err)
	}
	if cfg.Tunnel.APIKey != "runtime-key" || cfg.Tunnel.AdminKey != "admin-only" || cfg.Tunnel.ID != "tunnel_test" || !cfg.Tunnel.Enabled {
		t.Fatalf("tunnel config = %#v", cfg.Tunnel)
	}
}

func TestTunnelCommandAdminHierarchy(t *testing.T) {
	cmd := tunnelCommand()
	for _, path := range [][]string{{"admin-key", "set"}, {"admin-key", "status"}, {"admin-key", "verify"}, {"admin-key", "remove"}, {"list"}, {"get"}, {"create"}, {"update"}, {"delete"}} {
		resolved, _, err := cmd.Find(path)
		if err != nil || resolved.Name() != path[len(path)-1] {
			t.Fatalf("tunnel path %v resolved to %v: %v", path, resolved, err)
		}
	}
}
