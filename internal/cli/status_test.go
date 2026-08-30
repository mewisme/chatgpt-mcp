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
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_status", Managed: true, ServiceID: "chatgpt-mcp-system-test", ServiceScope: "system", StartedAt: started, ConfigRoot: root, ServerPort: cfg.Server.Port, AdminEnabled: cfg.Admin.Enabled, AdminPort: cfg.Admin.Port, Exposure: cfg.Server.Expose.Mode}
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
	for _, expected := range []string{"runtime: running", "managed: true", "scope: system", "service: chatgpt-mcp-system-test", "pid:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("status missing %q: %s", expected, text)
		}
	}
}
