package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
)

const (
	DefaultLifecycleTimeout = 45 * time.Second
	defaultPollInterval     = 150 * time.Millisecond
)

type RuntimeProbe func(context.Context) (runtimecontrol.RuntimeStatus, bool, error)
type RuntimeShutdown func(context.Context) error

type LifecycleEvent struct {
	Phase   string
	Message string
}

type LifecycleObserver func(LifecycleEvent)

type Lifecycle struct {
	Manager  Manager
	Spec     Spec
	Probe    RuntimeProbe
	Shutdown RuntimeShutdown
	Timeout  time.Duration
	Observe  LifecycleObserver
}

type LifecycleResult struct {
	Status  runtimecontrol.RuntimeStatus
	Changed bool
}

func (l Lifecycle) Up(ctx context.Context) (LifecycleResult, error) {
	if err := l.validate(); err != nil {
		return LifecycleResult{}, err
	}
	current, running, err := l.Probe(ctx)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err := ValidateRuntimeOwner(current, running, l.Spec, "up"); err != nil {
		return LifecycleResult{}, err
	}
	backend, err := l.Manager.Status(l.Spec)
	if err != nil {
		return LifecycleResult{}, err
	}
	matches, err := l.Manager.DefinitionMatches(l.Spec)
	if err != nil {
		return LifecycleResult{}, err
	}
	if running && backend.Installed && matches {
		if current.Starting {
			current, err = l.waitReady(ctx, "")
			if err != nil {
				return LifecycleResult{}, err
			}
		}
		return LifecycleResult{Status: current}, nil
	}
	if running {
		l.emit("runtime.stopping", "Stopping existing managed runtime")
		if err := l.shutdownAndWait(ctx); err != nil {
			return LifecycleResult{}, err
		}
	}
	if backend.Running || (running && backend.Installed) {
		if err := StopBackend(l.Manager, l.Spec); err != nil {
			return LifecycleResult{}, err
		}
	}
	if !backend.Installed || !matches {
		l.emit("definition.installing", "Installing managed service definition")
		if err := l.Manager.Install(l.Spec); err != nil {
			return LifecycleResult{}, err
		}
	}
	l.emit("backend.starting", "Starting managed service backend")
	if err := l.Manager.Start(l.Spec); err != nil {
		return LifecycleResult{}, err
	}
	l.emit("runtime.waiting", "Waiting for managed runtime readiness")
	status, err := l.waitReady(ctx, "")
	if err != nil {
		return LifecycleResult{}, err
	}
	return LifecycleResult{Status: status, Changed: true}, nil
}

func (l Lifecycle) Down(ctx context.Context) (LifecycleResult, error) {
	if err := l.validate(); err != nil {
		return LifecycleResult{}, err
	}
	current, running, err := l.Probe(ctx)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err := ValidateRuntimeOwner(current, running, l.Spec, "down"); err != nil {
		return LifecycleResult{}, err
	}
	backend, err := l.Manager.Status(l.Spec)
	if err != nil {
		return LifecycleResult{}, err
	}
	if !running && !backend.Installed {
		return LifecycleResult{}, nil
	}
	if running {
		l.emit("runtime.stopping", "Stopping managed runtime")
		if err := l.shutdownAndWait(ctx); err != nil {
			return LifecycleResult{}, err
		}
	}
	if backend.Running || backend.Installed {
		if err := StopBackend(l.Manager, l.Spec); err != nil {
			return LifecycleResult{}, err
		}
	}
	if backend.Installed {
		l.emit("definition.uninstalling", "Removing managed service definition")
		if err := l.Manager.Uninstall(l.Spec); err != nil {
			return LifecycleResult{}, err
		}
	}
	return LifecycleResult{Changed: true}, nil
}

func (l Lifecycle) Restart(ctx context.Context) (LifecycleResult, error) {
	if err := l.validate(); err != nil {
		return LifecycleResult{}, err
	}
	current, running, err := l.Probe(ctx)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err := ValidateRuntimeOwner(current, running, l.Spec, "restart"); err != nil {
		return LifecycleResult{}, err
	}
	backend, err := l.Manager.Status(l.Spec)
	if err != nil {
		return LifecycleResult{}, err
	}
	if !backend.Installed {
		return l.Up(ctx)
	}
	matches, err := l.Manager.DefinitionMatches(l.Spec)
	if err != nil {
		return LifecycleResult{}, err
	}
	previousRunID := current.RunID
	if running {
		l.emit("runtime.stopping", "Stopping current managed runtime")
		if err := l.shutdownAndWait(ctx); err != nil {
			return LifecycleResult{}, err
		}
	}
	if err := StopBackend(l.Manager, l.Spec); err != nil {
		return LifecycleResult{}, err
	}
	if !matches {
		l.emit("definition.installing", "Updating managed service definition")
		if err := l.Manager.Install(l.Spec); err != nil {
			return LifecycleResult{}, err
		}
	}
	l.emit("backend.starting", "Starting managed service backend")
	if err := l.Manager.Start(l.Spec); err != nil {
		return LifecycleResult{}, err
	}
	l.emit("runtime.waiting", "Waiting for managed runtime readiness")
	status, err := l.waitReady(ctx, previousRunID)
	if err != nil {
		return LifecycleResult{}, err
	}
	return LifecycleResult{Status: status, Changed: true}, nil
}

func (l Lifecycle) validate() error {
	if l.Manager == nil || l.Probe == nil || l.Shutdown == nil {
		return errors.New("managed lifecycle dependencies are unavailable")
	}
	if l.Spec.ID == "" {
		return errors.New("managed service specification is unavailable")
	}
	return nil
}

func (l Lifecycle) timeout() time.Duration {
	if l.Timeout > 0 {
		return l.Timeout
	}
	return DefaultLifecycleTimeout
}

func (l Lifecycle) emit(phase, message string) {
	if l.Observe != nil {
		l.Observe(LifecycleEvent{Phase: phase, Message: message})
	}
}

func (l Lifecycle) shutdownAndWait(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, min(5*time.Second, l.timeout()))
	err := l.Shutdown(shutdownCtx)
	cancel()
	if err != nil {
		return err
	}
	return l.waitStopped(ctx)
}

func (l Lifecycle) waitStopped(ctx context.Context) error {
	return WaitRuntimeStopped(ctx, l.Probe, l.timeout())
}

func WaitRuntimeStopped(ctx context.Context, probe RuntimeProbe, timeout time.Duration) error {
	if probe == nil {
		return errors.New("managed runtime probe is unavailable")
	}
	if timeout <= 0 {
		timeout = DefaultLifecycleTimeout
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, running, err := probe(ctx)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		if status.Lifecycle != "" {
			waitCtx, cancel := context.WithTimeout(ctx, min(10*time.Second, time.Until(deadline)))
			_, waitErr := runtimecontrol.WaitStatusChange(waitCtx, status.Lifecycle)
			cancel()
			if waitErr == nil {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		if err := waitLifecyclePoll(ctx); err != nil {
			return err
		}
	}
	return errors.New("managed runtime did not stop")
}

func (l Lifecycle) waitReady(ctx context.Context, previousRunID string) (runtimecontrol.RuntimeStatus, error) {
	return WaitRuntimeReady(ctx, l.Spec, l.Probe, previousRunID, l.timeout())
}

func WaitRuntimeReady(ctx context.Context, spec Spec, probe RuntimeProbe, previousRunID string, timeout time.Duration) (runtimecontrol.RuntimeStatus, error) {
	if probe == nil {
		return runtimecontrol.RuntimeStatus{}, errors.New("managed runtime probe is unavailable")
	}
	if timeout <= 0 {
		timeout = DefaultLifecycleTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		status, running, err := probe(ctx)
		if err != nil {
			lastErr = err
		} else if running {
			if err := ValidateRuntimeOwner(status, true, spec, "up"); err != nil {
				return runtimecontrol.RuntimeStatus{}, err
			}
			if previousRunID != "" && status.RunID == previousRunID {
				lastErr = errors.New("previous managed runtime is still shutting down")
			} else if status.Starting {
				lastErr = errors.New("managed runtime is still starting")
				if status.Lifecycle != "" {
					waitCtx, cancel := context.WithTimeout(ctx, min(10*time.Second, time.Until(deadline)))
					_, waitErr := runtimecontrol.WaitStatusChange(waitCtx, status.Lifecycle)
					cancel()
					if waitErr == nil {
						continue
					}
					if ctx.Err() != nil {
						return runtimecontrol.RuntimeStatus{}, ctx.Err()
					}
				}
			} else {
				return status, nil
			}
		}
		if err := waitLifecyclePoll(ctx); err != nil {
			return runtimecontrol.RuntimeStatus{}, err
		}
	}
	if lastErr != nil {
		return runtimecontrol.RuntimeStatus{}, fmt.Errorf("managed service did not become ready: %w", lastErr)
	}
	return runtimecontrol.RuntimeStatus{}, errors.New("managed service did not become ready")
}

func waitLifecyclePoll(ctx context.Context) error {
	timer := time.NewTimer(defaultPollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func StopBackend(manager Manager, spec Spec) error {
	if err := manager.Stop(spec); err != nil {
		status, statusErr := manager.Status(spec)
		if statusErr == nil && status.Installed && !status.Running && status.PID == 0 {
			return nil
		}
		return err
	}
	return nil
}

func ValidateRuntimeOwner(status runtimecontrol.RuntimeStatus, running bool, spec Spec, action string) error {
	if !running {
		return nil
	}
	if !status.Managed {
		return fmt.Errorf("runtime is already running outside the managed service (pid %d)", status.PID)
	}
	if status.ServiceID == spec.ID && status.ServiceScope == string(spec.Scope) {
		return nil
	}
	if status.ServiceScope == string(ScopeSystem) && spec.Scope == ScopeUser {
		return fmt.Errorf("runtime is managed by a system service; use cgm %s --system", action)
	}
	if status.ServiceScope == string(ScopeUser) && spec.Scope == ScopeSystem {
		return fmt.Errorf("runtime is managed by a user service; use cgm %s", action)
	}
	return fmt.Errorf("another managed service is already running for this config (service %s, pid %d)", status.ServiceID, status.PID)
}
