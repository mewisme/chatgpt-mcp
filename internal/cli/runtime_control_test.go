package cli

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestRuntimeControlReloadRoundTrip(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	control, err := startRuntimeControl(func(context.Context) (runtimeReloadResult, error) {
		return runtimeReloadResult{PID: os.Getpid(), NetworkRestarted: true, ServerPort: 41001, AdminEnabled: true, AdminPort: 41002, Exposure: config.ExposureNone}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := requestRuntimeReload(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.PID != os.Getpid() || !result.NetworkRestarted || result.ServerPort != 41001 || result.AdminPort != 41002 {
		t.Fatalf("reload result = %#v", result)
	}
}

func TestRuntimeReloadRequiresRunningServerInSelectedConfigDir(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := requestRuntimeReload(ctx); err == nil {
		t.Fatal("reload succeeded without a running server")
	}
}
