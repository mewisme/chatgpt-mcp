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
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
)

type updateRuntimeManager struct {
	installed bool
	starts    int
	stops     int
	control   *runtimeControl
}

func (m *updateRuntimeManager) Backend() string                              { return "fake" }
func (m *updateRuntimeManager) DefinitionMatches(managed.Spec) (bool, error) { return true, nil }
func (m *updateRuntimeManager) Install(managed.Spec) error                   { m.installed = true; return nil }
func (m *updateRuntimeManager) Uninstall(managed.Spec) error                 { m.installed = false; return nil }
func (m *updateRuntimeManager) Status(managed.Spec) (managed.Status, error) {
	return managed.Status{Installed: m.installed, Running: m.control != nil, Backend: "fake"}, nil
}
func (m *updateRuntimeManager) Start(spec managed.Spec) error {
	m.starts++
	stream := runtimeevent.NewStream(runtimeevent.Metadata{RunID: "run_update", PID: os.Getpid(), Managed: true, ServiceID: spec.ID, ServiceScope: string(spec.Scope)})
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_update", Managed: true, ServiceID: spec.ID, ServiceScope: string(spec.Scope), StartedAt: time.Now(), Events: stream, Reload: func(context.Context) (runtimeReloadResult, error) {
		return runtimeReloadResult{PID: os.Getpid(), ServerPort: 41001}, nil
	}, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_update", Managed: true, ServiceID: spec.ID, ServiceScope: string(spec.Scope), ConfigRoot: spec.ConfigRoot, ServerPort: 41001}
	}, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		return err
	}
	m.control = control
	return nil
}
func (m *updateRuntimeManager) Stop(managed.Spec) error {
	m.stops++
	if m.control != nil {
		err := m.control.Close()
		m.control = nil
		return err
	}
	return nil
}

func TestRestartManagedRuntimeInPlace(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	spec := managed.Spec{ID: managed.ID(root, managed.ScopeUser), Scope: managed.ScopeUser, ConfigRoot: root}
	manager := &updateRuntimeManager{installed: true}
	if err := manager.Start(spec); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if manager.control != nil {
			_ = manager.control.Close()
		}
	}()
	starts := manager.starts
	if err := restartManagedRuntimeInPlace(context.Background(), spec, manager); err != nil {
		t.Fatal(err)
	}
	if manager.stops != 1 || manager.starts != starts+1 || manager.control == nil {
		t.Fatalf("manager = %+v", manager)
	}
	state, err := captureUpdateRuntimeState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Running || !state.Status.Managed || state.Status.ServiceID != spec.ID {
		t.Fatalf("runtime state = %+v", state)
	}
}

func TestRestartManagedRuntimeInPlaceRequiresInstalledService(t *testing.T) {
	manager := &updateRuntimeManager{}
	if err := restartManagedRuntimeInPlace(context.Background(), managed.Spec{}, manager); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %v", err)
	}
	if manager.starts != 0 || manager.stops != 0 {
		t.Fatalf("manager mutated = %+v", manager)
	}
}

func TestCoordinateUpdatedRuntimeSkipsRestart(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	state := updateRuntimeState{Running: true, Status: runtimeStatusResult{PID: 123, Managed: true}}
	if err := coordinateUpdatedRuntime(cmd, install.Layout{}, state, true); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Runtime restart skipped") || !strings.Contains(text, "pid: 123") {
		t.Fatalf("output = %q", text)
	}
}

func TestCoordinateUpdatedRuntimeLeavesForegroundServerRunning(t *testing.T) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	state := updateRuntimeState{Running: true, Status: runtimeStatusResult{PID: 456}}
	if err := coordinateUpdatedRuntime(cmd, install.Layout{}, state, false); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Foreground runtime is still using the previous version") || !strings.Contains(text, "pid: 456") {
		t.Fatalf("output = %q", text)
	}
}

func TestRestartManagedRuntimeAfterUpdateRejectsServiceMismatch(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	layout, err := install.NewLayout(filepath.Join(t.TempDir(), "install"), filepath.Join(t.TempDir(), "bin"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	status := runtimeStatusResult{Managed: true, ConfigRoot: root, ServiceScope: string(managed.ScopeUser), ServiceID: "wrong"}
	if err := restartManagedRuntimeAfterUpdate(cmd, layout, status); err == nil || !strings.Contains(err.Error(), "service mismatch") {
		t.Fatalf("error = %v", err)
	}
}

func TestSaveManagedEnvironmentUsesSelectedConfig(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	spec := managed.Spec{ConfigRoot: root, Account: managed.Account{HomeDir: t.TempDir()}}
	hash, err := saveManagedEnvironment(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(hash) == "" {
		t.Fatal("environment hash is empty")
	}
}
