package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
)

func TestStatusReportsManagedRuntime(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute).UTC()
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_status", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, Events: runtimeevent.NewStream(runtimeevent.Metadata{}), Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_status", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, ConfigRoot: root, ServerPort: cfg.Server.Port, AdminEnabled: cfg.Admin.Enabled, AdminPort: cfg.Admin.Port, Exposure: cfg.Server.Expose.Mode, TunnelEnabled: true, TunnelConfigured: true, TunnelReady: true, TunnelID: "tunnel_status", TunnelSummary: runtimecontrol.TunnelSummary{Total: 1, Enabled: 1, Configured: 1, Ready: 1}, Tunnels: []runtimecontrol.TunnelRuntimeStatus{{ID: "tunnel_status", Enabled: true, Configured: true, Ready: true}}}
	}, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"✓ ChatGPT MCP is running", "Runtime", "session     run_status", "managed     system ·", "service     chatgpt-mcp-system-test", "Endpoints", "Config", "auth        mcp off · admin off", "Tunnels", "status      1/1 ready", "tunnel 1", "tunnel_status", "connected"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("status missing %q: %s", expected, text)
		}
	}
	if strings.Index(text, "Config") > strings.Index(text, "Tunnels") {
		t.Fatalf("tunnel should render after the core status sections: %s", text)
	}
	for _, unexpected := range []string{"initialized:", "format:", "mcp local:"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("status unexpectedly contains %q: %s", unexpected, text)
		}
	}
}

func TestStatusReportsStartingRuntime(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_starting", Managed: true, ServiceID: "chatgpt-mcp-user-test", ServiceScope: "user", StartedAt: time.Now().UTC(), Events: runtimeevent.NewStream(runtimeevent.Metadata{}), Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_starting", Starting: true, Managed: true, ServiceID: "chatgpt-mcp-user-test", ServiceScope: "user", ConfigRoot: root, ServerEnabled: false, TunnelEnabled: true, TunnelConfigured: true, TunnelRunning: true}
	}, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "ChatGPT MCP is starting") || strings.Contains(text, "ChatGPT MCP is running") {
		t.Fatalf("starting status=%q", text)
	}
}

func TestStatusVerboseReportsOperationalDetails(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute).UTC()
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_verbose", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, Events: runtimeevent.NewStream(runtimeevent.Metadata{}), Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_verbose", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, ConfigRoot: root, ServerPort: cfg.Server.Port, AdminEnabled: cfg.Admin.Enabled, AdminPort: cfg.Admin.Port, Exposure: cfg.Server.Expose.Mode, TunnelConfigured: true, TunnelID: "tunnel_verbose"}
	}, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "--verbose", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"started", "managed     true", "scope       system", "backend", "initialized true", "format"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("verbose status missing %q: %s", expected, text)
		}
	}
}

func TestStatusNotInitialized(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "! ChatGPT MCP is not initialized") || !strings.Contains(text, "chatgpt-mcp init") {
		t.Fatalf("unexpected uninitialized status: %s", text)
	}
}

func TestStatusHelpers(t *testing.T) {
	if got := compactStatusPath("/definitely/not/home/config.toml"); got == "" {
		t.Fatal("compactStatusPath returned empty path")
	}
	for duration, expected := range map[time.Duration]string{5 * time.Second: "5s", 2*time.Minute + 3*time.Second: "2m 03s", 3*time.Hour + 4*time.Minute: "3h 04m", 25*time.Hour + 2*time.Minute: "1d 01h 02m"} {
		if got := formatStatusUptime(time.Now().Add(-duration)); got != expected {
			t.Fatalf("formatStatusUptime(%s) = %q, want %q", duration, got, expected)
		}
	}
}

func TestRenderStatusConfigUsesCachedUpdateWithoutNetwork(t *testing.T) {
	checkedAt := time.Date(2026, 9, 4, 12, 0, 0, 0, time.Local)
	available := &updatepkg.CachedCheck{CheckResult: updatepkg.CheckResult{Current: "v1.0.0", Latest: "v1.1.0", Status: updatepkg.StatusAvailable}, CheckedAt: checkedAt}
	snapshot := statusSnapshot{Source: configformat.Source{Path: "/tmp/config.toml", Exists: true}, Config: config.Default(), Update: available}
	var output bytes.Buffer
	renderStatusConfig(&output, snapshot, false)
	if !strings.Contains(output.String(), "v1.1.0 available") || strings.Contains(output.String(), "checked") {
		t.Fatalf("cached available output = %q", output.String())
	}

	output.Reset()
	snapshot.Update = &updatepkg.CachedCheck{CheckResult: updatepkg.CheckResult{Current: "v1.1.0", Latest: "v1.1.0", Status: updatepkg.StatusUpToDate}, CheckedAt: checkedAt}
	renderStatusConfig(&output, snapshot, false)
	if strings.Contains(output.String(), "update") {
		t.Fatalf("non-verbose up-to-date cache should stay hidden: %q", output.String())
	}

	output.Reset()
	renderStatusConfig(&output, snapshot, true)
	if !strings.Contains(output.String(), "up to date") || !strings.Contains(output.String(), "checked") {
		t.Fatalf("verbose cached update output = %q", output.String())
	}
}

func TestRenderStatusConfigSurfacesSecurityWarnings(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Expose.Mode = config.ExposureAll
	cfg.Server.AllowInsecureHTTP = true
	snapshot := statusSnapshot{Source: configformat.Source{Path: "/tmp/config.toml", Exists: true}, Config: cfg}
	var output bytes.Buffer
	renderStatusConfig(&output, snapshot, false)
	text := output.String()
	if !strings.Contains(text, "cleartext HTTP") {
		t.Fatalf("expected cleartext warning: %q", text)
	}
}

func TestStatusTunnelStateTracksTransientStartup(t *testing.T) {
	base := runtimeStatusResult{TunnelEnabled: true, TunnelConfigured: true}
	for name, test := range map[string]struct {
		status runtimeStatusResult
		want   string
	}{
		"starting":     {status: base, want: "starting"},
		"connecting":   {status: runtimeStatusResult{TunnelEnabled: true, TunnelConfigured: true, TunnelRunning: true}, want: "connecting"},
		"reconnecting": {status: runtimeStatusResult{TunnelEnabled: true, TunnelConfigured: true, TunnelRestarting: true, TunnelLastError: "retry"}, want: "reconnecting"},
		"connected":    {status: runtimeStatusResult{TunnelEnabled: true, TunnelConfigured: true, TunnelRunning: true, TunnelReady: true}, want: "connected"},
		"degraded":     {status: runtimeStatusResult{TunnelEnabled: true, TunnelConfigured: true, TunnelLastError: "failed"}, want: "degraded"},
	} {
		t.Run(name, func(t *testing.T) {
			got := statusTunnelState(test.status, true)
			if got != test.want {
				t.Fatalf("state = %q, want %q", got, test.want)
			}
			if transientTunnelState(got) != (test.want == "starting" || test.want == "connecting" || test.want == "reconnecting") {
				t.Fatalf("transientTunnelState(%q) mismatch", got)
			}
		})
	}
}

func TestNewTunnelRuntimeStatusCopiesCachedName(t *testing.T) {
	status := tunnel.Status{ID: "tunnel_prod", Enabled: true, Ready: true, Metadata: &tunnel.Metadata{Name: "  Production  "}}
	item := newTunnelRuntimeStatus(status, true)
	if item.ID != "tunnel_prod" || item.Name != "Production" || !item.Ready || !item.Configured {
		t.Fatalf("item=%#v", item)
	}
	if newTunnelRuntimeStatus(tunnel.Status{ID: "tunnel_anon"}, true).Name != "" {
		t.Fatal("missing metadata should leave name empty")
	}
}

func TestLoadCachedTunnelNamesIgnoresBadMetadata(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	instances := []tunnel.InstanceConfig{
		{ID: "tunnel_good", Enabled: true, APIKey: "key-a"},
		{ID: "tunnel_bad", Enabled: true, APIKey: "key-b"},
	}
	cfg := config.Default()
	cfg.Tunnel.Instances = &instances
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_good", Name: "Good"}); err != nil {
		t.Fatal(err)
	}
	path, err := config.TunnelMetadataPath("tunnel_bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	names := loadCachedTunnelNames(cfg.RuntimeTunnels())
	if names["tunnel_good"] != "Good" {
		t.Fatalf("names=%v", names)
	}
	if _, ok := names["tunnel_bad"]; ok {
		t.Fatalf("bad metadata leaked: %v", names)
	}
}

func TestRenderStatusTunnelBodyCollection(t *testing.T) {
	prodID := "tunnel_6a9462c95f008191a665c3330bcd8368"
	stageID := "tunnel_6aa986b0e4d881919ac3ce543ce094ac"
	dupA := "tunnel_aaaaaaaaaaaaaaaaaaaaaaaac3330bcd"
	dupB := "tunnel_bbbbbbbbbbbbbbbbbbbbbbbb3ce094ac"
	healthy := statusSnapshot{
		Running: true,
		Runtime: runtimeStatusResult{
			TunnelSummary: runtimecontrol.TunnelSummary{Total: 2, Enabled: 2, Configured: 2, Running: 2, Ready: 2},
			Tunnels: []runtimecontrol.TunnelRuntimeStatus{
				{ID: prodID, Name: "Production", Enabled: true, Configured: true, Running: true, Ready: true},
				{ID: stageID, Name: "Staging", Enabled: true, Configured: true, Running: true, Ready: true},
			},
		},
	}
	var output bytes.Buffer
	renderStatusTunnelBody(&output, healthy, false)
	text := output.String()
	for _, expected := range []string{"status      2/2 ready", "tunnel 1", "tunnel 2", "name      Production", "name      Staging", "id        " + prodID, "id        " + stageID, "status    connected"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("healthy missing %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "attached") {
		t.Fatalf("healthy leaked attached counter: %s", text)
	}

	mixed := healthy
	mixed.Runtime.Tunnels = append([]runtimecontrol.TunnelRuntimeStatus(nil), healthy.Runtime.Tunnels...)
	mixed.Runtime.TunnelSummary.Ready = 1
	mixed.Runtime.Tunnels[1].Ready = false
	mixed.Runtime.Tunnels[1].Restarting = true
	output.Reset()
	renderStatusTunnelBody(&output, mixed, false)
	if !strings.Contains(output.String(), "1/2 ready · 1 reconnecting") || !strings.Contains(output.String(), "reconnecting") {
		t.Fatalf("reconnecting=%q", output.String())
	}

	degraded := healthy
	degraded.Runtime.Tunnels = append([]runtimecontrol.TunnelRuntimeStatus(nil), healthy.Runtime.Tunnels...)
	degraded.Runtime.TunnelSummary.Ready = 1
	degraded.Runtime.Tunnels[1].Ready = false
	degraded.Runtime.Tunnels[1].Running = false
	degraded.Runtime.Tunnels[1].LastError = "dial timeout"
	output.Reset()
	renderStatusTunnelBody(&output, degraded, false)
	if !strings.Contains(output.String(), "1/2 ready · 1 degraded") || !strings.Contains(output.String(), "degraded") {
		t.Fatalf("degraded=%q", output.String())
	}

	disabled := healthy
	disabled.Runtime.Tunnels = append([]runtimecontrol.TunnelRuntimeStatus(nil), healthy.Runtime.Tunnels...)
	disabled.Runtime.TunnelSummary.Ready, disabled.Runtime.TunnelSummary.Enabled = 1, 1
	disabled.Runtime.Tunnels[1].Enabled, disabled.Runtime.Tunnels[1].Ready, disabled.Runtime.Tunnels[1].Running = false, false, false
	output.Reset()
	renderStatusTunnelBody(&output, disabled, false)
	if !strings.Contains(output.String(), "1/2 ready · 1 disabled") {
		t.Fatalf("disabled=%q", output.String())
	}

	unconfigured := healthy
	unconfigured.Runtime.Tunnels = append([]runtimecontrol.TunnelRuntimeStatus(nil), healthy.Runtime.Tunnels...)
	unconfigured.Runtime.TunnelSummary = runtimecontrol.TunnelSummary{Total: 2, Enabled: 2, Ready: 0}
	unconfigured.Runtime.Tunnels[0].Configured, unconfigured.Runtime.Tunnels[0].Ready, unconfigured.Runtime.Tunnels[0].Running = false, false, false
	unconfigured.Runtime.Tunnels[1].Configured, unconfigured.Runtime.Tunnels[1].Ready, unconfigured.Runtime.Tunnels[1].Running = false, false, false
	output.Reset()
	renderStatusTunnelBody(&output, unconfigured, false)
	if !strings.Contains(output.String(), "0/2 ready · 2 not configured") {
		t.Fatalf("unconfigured=%q", output.String())
	}

	duplicates := healthy
	duplicates.Runtime.Tunnels = []runtimecontrol.TunnelRuntimeStatus{
		{ID: dupA, Name: "Production", Enabled: true, Configured: true, Running: true, Ready: true},
		{ID: dupB, Name: "Production", Enabled: true, Configured: true, Running: true, Ready: true},
	}
	output.Reset()
	renderStatusTunnelBody(&output, duplicates, false)
	text = output.String()
	if !strings.Contains(text, "Production · c3330bcd") || !strings.Contains(text, "Production · 3ce094ac") {
		t.Fatalf("duplicates=%q", text)
	}
	if !strings.Contains(text, dupA) || !strings.Contains(text, dupB) {
		t.Fatalf("duplicate ids missing: %s", text)
	}

	missing := healthy
	missing.Runtime.Tunnels = append([]runtimecontrol.TunnelRuntimeStatus(nil), healthy.Runtime.Tunnels...)
	missing.Runtime.Tunnels[0].Name = ""
	output.Reset()
	renderStatusTunnelBody(&output, missing, false)
	if !strings.Contains(output.String(), prodID) || !strings.Contains(output.String(), "Staging") {
		t.Fatalf("missing name=%q", output.String())
	}

	offline := statusSnapshot{
		Running: false,
		Config: config.Config{Tunnel: tunnel.Config{Instances: &[]tunnel.InstanceConfig{
			{ID: prodID, Enabled: true, APIKey: "a"},
			{ID: stageID, Enabled: true, APIKey: "b"},
		}}},
		TunnelNames: map[string]string{prodID: "Production", stageID: "Staging"},
	}
	output.Reset()
	renderStatusTunnelBody(&output, offline, false)
	text = output.String()
	if !strings.Contains(text, "runtime offline · 2 configured") || !strings.Contains(text, "Production") || !strings.Contains(text, "Staging") || !strings.Contains(text, "offline") {
		t.Fatalf("offline=%q", text)
	}
	if !strings.Contains(text, prodID) || !strings.Contains(text, stageID) {
		t.Fatalf("offline missing ids: %s", text)
	}

	output.Reset()
	renderStatusTunnelBody(&output, statusSnapshot{Running: true}, false)
	if !strings.Contains(output.String(), "none configured") {
		t.Fatalf("empty=%q", output.String())
	}
	if strings.Contains(output.String(), "Production") || strings.Contains(output.String(), "tunnel_") || strings.Contains(output.String(), "tunnel 1") {
		t.Fatalf("empty should not render tunnel rows: %q", output.String())
	}

	verbose := degraded
	output.Reset()
	renderStatusTunnelBody(&output, verbose, true)
	text = output.String()
	for _, expected := range []string{"total       2", "ready       1", "tunnel 1", "tunnel 2", "Production", "Staging", prodID, stageID, "dial timeout", "connected", "degraded"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("verbose missing %q: %s", expected, text)
		}
	}

	aligned := statusSnapshot{
		Running: true,
		Runtime: runtimeStatusResult{
			TunnelSummary: runtimecontrol.TunnelSummary{Total: 2, Enabled: 2, Configured: 2, Running: 2, Ready: 2},
			Tunnels: []runtimecontrol.TunnelRuntimeStatus{
				{ID: prodID, Name: "MCP_Tunnel_WSL", Enabled: true, Configured: true, Running: true, Ready: true},
				{ID: stageID, Name: "WSL_Tunnel", Enabled: true, Configured: true, Running: true, Ready: true},
			},
		},
	}
	output.Reset()
	renderStatusTunnelBody(&output, aligned, false)
	text = output.String()
	for _, expected := range []string{"status      2/2 ready", "tunnel 1", "tunnel 2", "name      MCP_Tunnel_WSL", "name      WSL_Tunnel", "id        " + prodID, "status    connected"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("aligned missing %q: %s", expected, text)
		}
	}
}

func TestRenderStatusTunnelProvidersOmitsSecureMCP(t *testing.T) {
	var output bytes.Buffer
	renderStatusTunnelProviders(&output, statusSnapshot{
		Running: true,
		Runtime: runtimeStatusResult{
			TunnelProviders: []runtimecontrol.TunnelProviderStatus{
				{
					Provider: "secure-mcp",
					Name:     "Secure MCP Tunnel",
					Enabled:  true,
					Targets: []runtimecontrol.TunnelProviderTargetStatus{
						{Target: "tunnel_6a9462c95f008191a665c3330bcd8368", Running: true},
						{Target: "tunnel_6aa986b0e4d881919ac3ce543ce094ac", Running: true},
					},
				},
			},
		},
	})
	if text := output.String(); text != "" {
		t.Fatalf("secure mcp leaked into provider status: %q", text)
	}
}
