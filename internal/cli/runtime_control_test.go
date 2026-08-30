package cli

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
)

func TestRuntimeControlReloadStatusAndShutdownRoundTrip(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	journal, err := runtimeevent.NewJournal(root, runtimeevent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stream := runtimeevent.NewStream(runtimeevent.Metadata{RunID: "run_test", PID: os.Getpid()})
	shutdown := make(chan struct{}, 1)
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_test", StartedAt: time.Now(), Events: stream, Reload: func(context.Context) (runtimeReloadResult, error) {
		return runtimeReloadResult{PID: os.Getpid(), NetworkRestarted: true, ServerPort: 41001, AdminEnabled: true, AdminPort: 41002, Exposure: config.ExposureNone}, nil
	}, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_test", ConfigRoot: root, ServerPort: 41001, AdminEnabled: true, AdminPort: 41002, Exposure: config.ExposureNone}
	}, Shutdown: func() {
		select {
		case shutdown <- struct{}{}:
		default:
		}
	}, ClearLogs: journal.Clear})
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
	status, err := requestRuntimeStatus(ctx)
	if err != nil || status.RunID != "run_test" || status.ServerPort != 41001 {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	if err := requestRuntimeShutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not invoked")
	}
}

func TestRuntimeControlRejectsUnauthenticatedEvents(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_test", Events: runtimeevent.NewStream(runtimeevent.Metadata{}), Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult { return runtimeStatusResult{PID: os.Getpid()} }, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + control.state.Address + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.StatusCode)
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
