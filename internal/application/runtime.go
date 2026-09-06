package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
)

const managedReadyTimeout = 15 * time.Second

type ExternalCommand struct {
	Command string
	Reason  string
}

type RuntimeOverview struct {
	Running       bool
	Status        runtimecontrol.RuntimeStatus
	UserService   ServiceOverview
	SystemService ServiceOverview
}

type ServiceOverview struct {
	Scope      managed.Scope
	Supported  bool
	ID         string
	Backend    string
	Installed  bool
	Running    bool
	PID        int
	ConfigRoot string
	Warning    string
	Err        string
}

type RuntimeActionResult struct {
	Action   string
	Scope    managed.Scope
	Changed  bool
	Status   runtimecontrol.RuntimeStatus
	Service  ServiceOverview
	External *ExternalCommand
}

var newServiceManager = managed.NewManager
var detectServiceScope = managed.DetectScope

func RuntimeStatus(ctx context.Context) (runtimecontrol.RuntimeStatus, bool, error) {
	var result runtimecontrol.RuntimeStatus
	state, err := runtimecontrol.Request(ctx, http.MethodGet, "/status", nil, &result)
	if err != nil {
		if runtimecontrol.IsUnavailable(err) {
			return runtimecontrol.RuntimeStatus{}, false, nil
		}
		return runtimecontrol.RuntimeStatus{}, false, err
	}
	if result.PID != state.PID {
		return runtimecontrol.RuntimeStatus{}, false, fmt.Errorf("runtime control PID mismatch: expected %d, got %d", state.PID, result.PID)
	}
	return result, true, nil
}

func LoadRuntimeOverview(ctx context.Context) (RuntimeOverview, error) {
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	status, running, err := RuntimeStatus(statusCtx)
	cancel()
	if err != nil {
		return RuntimeOverview{}, err
	}
	user := loadServiceOverview(managed.ScopeUser)
	system := ServiceOverview{Scope: managed.ScopeSystem, Supported: runtime.GOOS != "windows"}
	if system.Supported {
		system = loadServiceOverview(managed.ScopeSystem)
	}
	return RuntimeOverview{Running: running, Status: status, UserService: user, SystemService: system}, nil
}

func loadServiceOverview(scope managed.Scope) ServiceOverview {
	overview := ServiceOverview{Scope: scope, Supported: scope == managed.ScopeUser || runtime.GOOS != "windows"}
	if !overview.Supported {
		return overview
	}
	spec, manager, err := managedService(scope, "")
	if err != nil {
		overview.Err = err.Error()
		return overview
	}
	status, err := manager.Status(spec)
	if err != nil {
		overview.Err = err.Error()
		return overview
	}
	overview.ID, overview.Backend, overview.Installed, overview.Running, overview.PID, overview.ConfigRoot = spec.ID, managedBackend(manager, spec), status.Installed, status.Running, status.PID, spec.ConfigRoot
	overview.Warning = managed.PersistenceWarning(spec)
	return overview
}

func ManagedRuntimeAction(ctx context.Context, action string, scope managed.Scope) (RuntimeActionResult, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "up" && action != "down" && action != "restart" {
		return RuntimeActionResult{}, fmt.Errorf("unsupported runtime action: %s", action)
	}
	if scope == "" {
		scope = detectServiceScope()
	}
	if scope != managed.ScopeUser && scope != managed.ScopeSystem {
		return RuntimeActionResult{}, fmt.Errorf("unsupported service scope: %s", scope)
	}
	if scope == managed.ScopeSystem && runtime.GOOS == "windows" {
		return RuntimeActionResult{}, errors.New("system service scope is not supported on Windows; managed services use a per-user Scheduled Task")
	}
	if scope == managed.ScopeSystem && detectServiceScope() == managed.ScopeUser {
		command := "cgm --config-dir " + strconv.Quote(config.RootPath()) + " " + action + " --system"
		return RuntimeActionResult{Action: action, Scope: scope, External: &ExternalCommand{Command: command, Reason: "System service changes require elevation outside the TUI."}}, nil
	}
	spec, manager, err := managedService(scope, "")
	if err != nil {
		return RuntimeActionResult{}, err
	}
	switch action {
	case "up":
		return managedUp(ctx, spec, manager)
	case "down":
		return managedDown(ctx, spec, manager)
	default:
		return managedRestart(ctx, spec, manager)
	}
}

func managedService(scope managed.Scope, binary string) (managed.Spec, managed.Manager, error) {
	account, err := managed.InvokingAccount(scope)
	if err != nil {
		return managed.Spec{}, nil, err
	}
	if strings.TrimSpace(binary) == "" {
		binary = os.Args[0]
	}
	spec, err := managed.NewSpec(config.RootPath(), binary, scope, account)
	if err != nil {
		return managed.Spec{}, nil, err
	}
	return spec, newServiceManager(), nil
}

func prepareManagedSpec(spec managed.Spec) (managed.Spec, error) {
	source, err := config.Source()
	if err != nil {
		return spec, err
	}
	if !source.Exists {
		return spec, errors.New("chatgpt-mcp is not initialized; run cgm init first")
	}
	if _, err := config.VerifyRuntime(); err != nil {
		return spec, err
	}
	cfg, err := config.Load()
	if err != nil {
		return spec, err
	}
	hash, err := managed.SaveEnvironment(spec.ConfigRoot, managed.CaptureEnvironment(spec.Account, cfg.Shell.Path))
	if err != nil {
		return spec, err
	}
	spec.EnvironmentHash = hash
	return spec, nil
}

func managedUp(ctx context.Context, spec managed.Spec, manager managed.Manager) (RuntimeActionResult, error) {
	var err error
	spec, err = prepareManagedSpec(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	current, running, err := runtimeStatusFast(ctx)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	if err := validateRuntimeOwner(current, running, spec, "up"); err != nil {
		return RuntimeActionResult{}, err
	}
	backend, err := manager.Status(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	matches, err := manager.DefinitionMatches(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	if running && backend.Installed && matches {
		return runtimeActionResult("up", spec, manager, current, false), nil
	}
	if running {
		if err := requestRuntimeShutdown(ctx); err != nil {
			return RuntimeActionResult{}, err
		}
		if err := waitRuntimeStopped(ctx, managedReadyTimeout); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if backend.Running {
		if err := stopManagedBackend(spec, manager); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if !backend.Installed || !matches {
		if err := manager.Install(spec); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if err := manager.Start(spec); err != nil {
		return RuntimeActionResult{}, err
	}
	status, err := waitManagedReady(ctx, spec, "", managedReadyTimeout)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	return runtimeActionResult("up", spec, manager, status, true), nil
}

func managedDown(ctx context.Context, spec managed.Spec, manager managed.Manager) (RuntimeActionResult, error) {
	current, running, err := runtimeStatusFast(ctx)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	if err := validateRuntimeOwner(current, running, spec, "down"); err != nil {
		return RuntimeActionResult{}, err
	}
	backend, err := manager.Status(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	if !running && !backend.Installed {
		return runtimeActionResult("down", spec, manager, runtimecontrol.RuntimeStatus{}, false), nil
	}
	if running {
		if err := requestRuntimeShutdown(ctx); err != nil {
			return RuntimeActionResult{}, err
		}
		if err := waitRuntimeStopped(ctx, managedReadyTimeout); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if backend.Running || backend.Installed {
		if err := stopManagedBackend(spec, manager); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if backend.Installed {
		if err := manager.Uninstall(spec); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	return runtimeActionResult("down", spec, manager, runtimecontrol.RuntimeStatus{}, true), nil
}

func managedRestart(ctx context.Context, spec managed.Spec, manager managed.Manager) (RuntimeActionResult, error) {
	var err error
	spec, err = prepareManagedSpec(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	current, running, err := runtimeStatusFast(ctx)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	if err := validateRuntimeOwner(current, running, spec, "restart"); err != nil {
		return RuntimeActionResult{}, err
	}
	backend, err := manager.Status(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	if !backend.Installed {
		return managedUp(ctx, spec, manager)
	}
	matches, err := manager.DefinitionMatches(spec)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	previousRunID := current.RunID
	if running {
		if err := requestRuntimeShutdown(ctx); err != nil {
			return RuntimeActionResult{}, err
		}
		if err := waitRuntimeStopped(ctx, managedReadyTimeout); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if err := stopManagedBackend(spec, manager); err != nil {
		return RuntimeActionResult{}, err
	}
	if !matches {
		if err := manager.Install(spec); err != nil {
			return RuntimeActionResult{}, err
		}
	}
	if err := manager.Start(spec); err != nil {
		return RuntimeActionResult{}, err
	}
	status, err := waitManagedReady(ctx, spec, previousRunID, managedReadyTimeout)
	if err != nil {
		return RuntimeActionResult{}, err
	}
	return runtimeActionResult("restart", spec, manager, status, true), nil
}

func validateRuntimeOwner(status runtimecontrol.RuntimeStatus, running bool, spec managed.Spec, action string) error {
	if !running {
		return nil
	}
	if !status.Managed {
		if action == "down" {
			return fmt.Errorf("runtime is running in foreground mode (pid %d); leave the TUI and stop the foreground process explicitly", status.PID)
		}
		return fmt.Errorf("runtime is already running outside the managed service (pid %d); stop the foreground process first", status.PID)
	}
	if status.ServiceID == spec.ID && status.ServiceScope == string(spec.Scope) {
		return nil
	}
	if status.ServiceScope == string(managed.ScopeSystem) && spec.Scope == managed.ScopeUser {
		return errors.New("runtime is managed by a system service; use the system-scope action")
	}
	if status.ServiceScope == string(managed.ScopeUser) && spec.Scope == managed.ScopeSystem {
		return errors.New("runtime is managed by a user service; use the user-scope action")
	}
	return fmt.Errorf("another managed service is already running for this config (service %s, pid %d)", status.ServiceID, status.PID)
}

func runtimeStatusFast(ctx context.Context) (runtimecontrol.RuntimeStatus, bool, error) {
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return RuntimeStatus(statusCtx)
}

func requestRuntimeShutdown(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := runtimecontrol.Request(requestCtx, http.MethodPost, "/shutdown", nil, &map[string]bool{})
	return err
}

func stopManagedBackend(spec managed.Spec, manager managed.Manager) error {
	if err := manager.Stop(spec); err != nil {
		status, statusErr := manager.Status(spec)
		if statusErr == nil && status.Installed && !status.Running && status.PID == 0 {
			return nil
		}
		return err
	}
	return nil
}

func waitRuntimeStopped(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, running, err := runtimeStatusFast(ctx)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return errors.New("managed runtime did not stop")
}

func waitManagedReady(ctx context.Context, spec managed.Spec, previousRunID string, timeout time.Duration) (runtimecontrol.RuntimeStatus, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		status, running, err := runtimeStatusFast(ctx)
		if err != nil {
			lastErr = err
		} else if running {
			if err := validateRuntimeOwner(status, true, spec, "up"); err != nil {
				return runtimecontrol.RuntimeStatus{}, err
			}
			if previousRunID != "" && status.RunID == previousRunID {
				lastErr = errors.New("previous managed runtime is still shutting down")
			} else {
				return status, nil
			}
		}
		select {
		case <-ctx.Done():
			return runtimecontrol.RuntimeStatus{}, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return runtimecontrol.RuntimeStatus{}, fmt.Errorf("managed service did not become ready: %w", lastErr)
	}
	return runtimecontrol.RuntimeStatus{}, errors.New("managed service did not become ready")
}

func runtimeActionResult(action string, spec managed.Spec, manager managed.Manager, status runtimecontrol.RuntimeStatus, changed bool) RuntimeActionResult {
	serviceStatus, err := manager.Status(spec)
	overview := ServiceOverview{Scope: spec.Scope, Supported: true, ID: spec.ID, Backend: managedBackend(manager, spec), ConfigRoot: spec.ConfigRoot, Warning: managed.PersistenceWarning(spec)}
	if err != nil {
		overview.Err = err.Error()
	} else {
		overview.Installed, overview.Running, overview.PID = serviceStatus.Installed, serviceStatus.Running, serviceStatus.PID
	}
	return RuntimeActionResult{Action: action, Scope: spec.Scope, Changed: changed, Status: status, Service: overview}
}

func managedBackend(manager managed.Manager, spec managed.Spec) string {
	if runtime.GOOS == "linux" && spec.Scope == managed.ScopeUser {
		return "systemd --user"
	}
	return manager.Backend()
}
