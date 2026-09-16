package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/notification"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func TestDoctorCommandRegistered(t *testing.T) {
	cmd, _, err := newRootCommand().Find([]string{"doctor"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "doctor" || !cmd.Runnable() {
		t.Fatalf("doctor command = %q runnable=%t", cmd.Name(), cmd.Runnable())
	}
}

func TestDoctorUninitializedStillReportsLaterChecks(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v", err)
	}
	text := out.String()
	for _, expected := range []string{"FAIL", "config source", "plugin lock", "Summary:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestDoctorSecurityWarningDoesNotFail(t *testing.T) {
	saveLoopbackDoctorConfig(t)
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runDoctor(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "config security") || !strings.Contains(out.String(), "WARN") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorReportsCorruptPluginLockWithoutMutation(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	layout := pluginpkg.DefaultLayout()
	if err := os.WriteFile(layout.LockPath(), []byte(`{"schema":1,"plugins":`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := doctorCommand()
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	if _, err := pluginpkg.LoadLock(layout.LockPath()); err == nil {
		t.Fatal("corrupt lock was rewritten")
	}
	matches, err := filepath.Glob(layout.LockPath() + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("doctor quarantined lock: %#v root=%s output=%q", matches, root, out.String())
	}
	if !strings.Contains(out.String(), "plugin lock") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorReportsUnavailableWorkspaceWithoutHidingHealthyOne(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	healthy := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(missing, 0o700); err != nil {
		t.Fatal(err)
	}
	executeRequestCommand(t, root, []string{"workspace", "register", healthy})
	executeRequestCommand(t, root, []string{"workspace", "register", missing})
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "workspace registry") || !strings.Contains(text, "FAIL") {
		t.Fatalf("output = %q", text)
	}
	if !strings.Contains(text, "PASS") {
		t.Fatalf("healthy workspace hidden: %q", text)
	}
}

func TestDoctorMalformedConfigFails(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorStaleRuntimeControlWarns(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	token := "mcp_abcdefghijklmnopqrstuvwxyz1234"
	state := runtimecontrol.State{PID: 999999, Address: "127.0.0.1:9", Token: token, ConfigRoot: root}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, runtimecontrol.FileName), data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	_ = runDoctor(cmd, nil)
	text := out.String()
	if !strings.Contains(text, "WARN") || !strings.Contains(text, "stale") {
		t.Fatalf("output = %q", text)
	}
	if strings.Contains(text, token) {
		t.Fatalf("runtime-control token leaked: %q", text)
	}
}

func TestDoctorWorkspaceIdentityMismatch(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	ws := t.TempDir()
	executeRequestCommand(t, root, []string{"workspace", "register", ws})
	store := workspacestate.Store{WorkspaceRoot: ws}
	identity, err := store.LoadIdentity()
	if err != nil {
		t.Fatal(err)
	}
	identity.ID = "ws_mismatchedidentity"
	payload, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.IdentityPath(), payload, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err = runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "identity mismatch") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorProjectContextSourceFailure(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	ws := t.TempDir()
	executeRequestCommand(t, root, []string{"workspace", "register", ws})
	rules := workspacestate.Store{WorkspaceRoot: ws}.RulesRoot()
	if err := os.RemoveAll(rules); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rules, []byte("not-a-directory"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "project context") && !strings.Contains(out.String(), "context") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorCFTunnelMCPMissingAuth(t *testing.T) {
	saveLoopbackDoctorConfig(t)
	enableCFTunnelTarget(t, cftunnelplugin.TargetMCP)
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "CF Tunnel MCP") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorCFTunnelAdminMissingAuth(t *testing.T) {
	saveLoopbackDoctorConfig(t)
	enableCFTunnelTarget(t, cftunnelplugin.TargetAdmin)
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "CF Tunnel Admin") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorCFTunnelReportsTargetsIndependently(t *testing.T) {
	saveLoopbackDoctorConfig(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Auth.MCPEnabled = true
	cfg.Auth.MCPTokenHash = "mcp-hash"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	enableCFTunnelTarget(t, cftunnelplugin.TargetMCP)
	enableCFTunnelTarget(t, cftunnelplugin.TargetAdmin)
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err = runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "CF Tunnel MCP") || !strings.Contains(text, "CF Tunnel Admin") {
		t.Fatalf("output = %q", text)
	}
	if strings.Contains(text, "CF Tunnel MCP") && strings.Count(text, "FAIL") < 1 {
		t.Fatalf("expected admin fail: %q", text)
	}
}

func TestDoctorEnabledNotificationUnavailableIsWarning(t *testing.T) {
	if notification.PlatformProvider().Available(context.Background()) {
		t.Skip("desktop notification provider is available")
	}
	root := t.TempDir()
	testutil.UseConfigRoot(t, root)
	cfg := loopbackDoctorConfig()
	cfg.Notifications.Enabled = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runDoctor(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "WARN") || !strings.Contains(out.String(), "notifications") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorUpdateUnavailableIsWarning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := (&doctorState{}).checkUpdate(ctx)
	if result.Status != doctorWarn && result.Status != doctorPass {
		t.Fatalf("status = %s error=%s", result.Status, result.Error)
	}
	if result.Status == doctorFail {
		t.Fatal("update unavailability must not fail doctor")
	}
}

func TestDoctorUpstreamUnreachableFailsWithoutLeakingSecrets(t *testing.T) {
	saveLoopbackDoctorConfig(t)
	store := upstream.NewStore(upstream.Path())
	if err := store.Save([]upstream.Server{{
		ID: "gone", Name: "gone", Transport: "http", Enabled: true,
		URL: "http://127.0.0.1:1", AllowPrivateNetwork: true,
		Headers: map[string]string{"Authorization": "Bearer leaked-upstream-secret"},
	}}); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "upstream") || !strings.Contains(text, "FAIL") {
		t.Fatalf("output = %q", text)
	}
	if strings.Contains(text, "leaked-upstream-secret") {
		t.Fatalf("secret leaked: %q", text)
	}
}

func TestDoctorDuplicateTunnelIDsFail(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := []tunnel.InstanceConfig{{ID: "dup"}, {ID: "dup"}}
	cfg.Tunnel.Instances = &instances
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err = runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q root=%s", err, out.String(), root)
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorJSONIncludesStableIDs(t *testing.T) {
	root := saveLoopbackDoctorConfig(t)
	var out bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config-dir", root, "--log-format", "json", "doctor"})
	_ = cmd.Execute()
	if !strings.Contains(out.String(), `"id": "config.source"`) {
		t.Fatalf("json = %q", out.String())
	}
}

func saveLoopbackDoctorConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutil.UseConfigRoot(t, root)
	if err := config.Save(loopbackDoctorConfig()); err != nil {
		t.Fatal(err)
	}
	return root
}

func loopbackDoctorConfig() config.Config {
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	cfg.Admin.Enabled = false
	cfg.Notifications.Enabled = false
	return cfg
}

func enableCFTunnelTarget(t *testing.T, target string) {
	t.Helper()
	layout := pluginpkg.DefaultLayout()
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	store.Builtins = pluginpkg.BuiltinRegistry{cftunnelplugin.Plugin()}
	if err := store.SetEnabled("cf-tunnel", true); err != nil {
		t.Fatal(err)
	}
	if err := (pluginpkg.SettingsStore{Layout: layout}).Set(cftunnelplugin.Plugin().Schema, "cf-tunnel", target, true); err != nil {
		t.Fatal(err)
	}
}
