package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
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
	for _, expected := range []string{"⠋ Reconnecting tunnel", "tunnel_id: tunnel_test", "attempt: 3", "retry_in: 4s"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestTunnelCommandHierarchy(t *testing.T) {
	cmd := tunnelCommand()
	for _, path := range [][]string{{"admin", "list"}, {"admin", "add"}, {"admin", "update"}, {"admin", "verify"}, {"admin", "remove"}, {"managed", "list"}, {"managed", "get"}, {"managed", "create"}, {"managed", "update"}, {"managed", "delete"}, {"list"}, {"status"}, {"add"}, {"attach"}, {"update"}, {"detach"}, {"enable"}, {"disable"}, {"start"}, {"stop"}, {"run"}} {
		resolved, _, err := cmd.Find(path)
		if err != nil || resolved.Name() != path[len(path)-1] {
			t.Fatalf("tunnel path %v resolved to %v: %v", path, resolved, err)
		}
	}
	for _, legacy := range [][]string{{"use"}, {"select"}, {"switch"}, {"configure"}, {"admin", "key"}} {
		if resolved, remaining, err := cmd.Find(legacy); err == nil && len(remaining) == 0 && resolved.Name() == legacy[len(legacy)-1] {
			t.Fatalf("legacy tunnel path %v is still registered as %s", legacy, resolved.CommandPath())
		}
	}
}

func TestTunnelRunRequiresTunnelID(t *testing.T) {
	cmd := tunnelRunCommand()
	if err := cmd.Args(cmd, nil); err == nil {
		t.Fatal("tunnel run accepted a missing tunnel id")
	}
	if err := cmd.Args(cmd, []string{"tunnel_a"}); err != nil {
		t.Fatalf("tunnel run rejected one tunnel id: %v", err)
	}
}

func TestTunnelAdminUpdateCommandPreservesBlankKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-secret" || r.URL.Query().Get("organization_id") != "org_new" {
			t.Fatalf("request=%s %s auth=%q query=%s", r.Method, r.URL.Path, r.Header.Get("Authorization"), r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"tunnels":[{"id":"tunnel_one"},{"id":"tunnel_two"}]}`))
	}))
	defer server.Close()
	rootDir := filepath.Join(t.TempDir(), "config")
	testutil.UseConfigRoot(t, rootDir)
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_old", ReadAccess: true, ManageAccess: true, ControlPlaneBaseURL: server.URL}}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config-dir", rootDir, "tunnel", "admin", "update", "work", "--organization-id", "org_new"})
	if _, err := cmd.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.RuntimeTunnels().Admins
	if len(got) != 1 || got[0].AdminKey != "admin-secret" || got[0].OrganizationID != "org_new" || got[0].WorkspaceID != "" || !got[0].ReadAccess {
		t.Fatalf("admins=%#v", got)
	}
}

func TestTunnelAddAndUpdateMutateOnlyTarget(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "config")
	testutil.UseConfigRoot(t, rootDir)
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_one", APIKey: "runtime-one", OrganizationID: "org_old"}, {Enabled: true, ID: "tunnel_two", APIKey: "runtime-two"}}
	admins := []tunnel.AdminConfig{}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config-dir", rootDir, "tunnel", "add", "tunnel_three", "--runtime-api-key", "runtime-three"})
	if _, err := cmd.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	cmd = newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config-dir", rootDir, "tunnel", "update", "tunnel_one", "--organization-id", "org_new", "--disabled"})
	if _, err := cmd.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.RuntimeTunnels().Instances
	if len(got) != 3 || got[0].ID != "tunnel_one" || got[0].Enabled || got[0].APIKey != "runtime-one" || got[0].OrganizationID != "org_new" || got[1].ID != "tunnel_two" || !got[1].Enabled || got[1].APIKey != "runtime-two" || got[2].ID != "tunnel_three" || got[2].APIKey != "runtime-three" {
		t.Fatalf("instances=%#v", got)
	}
}
