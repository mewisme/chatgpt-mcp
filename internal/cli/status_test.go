package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
)

func TestStatusReportsManagedRuntime(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute).UTC()
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_status", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, Events: runtimeevent.NewStream(runtimeevent.Metadata{}), Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_status", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, ConfigRoot: root, ServerPort: cfg.Server.Port, AdminEnabled: cfg.Admin.Enabled, AdminPort: cfg.Admin.Port, Exposure: cfg.Server.Expose.Mode, TunnelEnabled: true, TunnelConfigured: true, TunnelReady: true, TunnelID: "tunnel_status"}
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
	for _, expected := range []string{"✓ ChatGPT MCP is running", "Runtime", "session     run_status", "managed     system ·", "service     chatgpt-mcp-system-test", "Endpoints", "Tunnel", "status      connected", "id          tunnel_status", "Config", "auth        mcp off · admin off"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("status missing %q: %s", expected, text)
		}
	}
	for _, unexpected := range []string{"initialized:", "format:", "mcp local:"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("status unexpectedly contains %q: %s", unexpected, text)
		}
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
