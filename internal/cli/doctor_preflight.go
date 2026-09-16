package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/redact"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func (d *doctorState) checkInstall(ctx context.Context) doctorResult {
	exe, err := os.Executable()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "current executable could not be resolved", Error: redact.Text(err.Error())}
	}
	detection, err := install.DetectCurrent(version.Version)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "install detection failed", Error: redact.Text(err.Error()), Details: []string{exe}}
	}
	details := []string{exe, "method " + string(detection.Method), "version " + version.Short()}
	if detection.Root != "" {
		details = append(details, "root "+detection.Root)
	}
	if detection.Method == install.MethodUnknown || detection.Method == install.MethodDevelopment || detection.Method == install.MethodGo {
		return doctorResult{Status: doctorPass, Summary: "development or unpackaged executable", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: "install metadata is consistent", Details: details}
}

func (d *doctorState) checkConfigIntegrity(ctx context.Context) doctorResult {
	result, err := config.VerifyRuntime()
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "configuration integrity check failed", Error: redact.Text(err.Error())}
	}
	return doctorResult{Status: doctorPass, Summary: "configuration files decode as " + string(result.Format), Details: []string{fmt.Sprintf("%d structured files", result.Files)}}
}

func (d *doctorState) checkConfigValidate(ctx context.Context) doctorResult {
	if err := config.Validate(d.cfg); err != nil {
		return doctorResult{Status: doctorFail, Summary: "configuration is invalid", Error: redact.Text(err.Error())}
	}
	return doctorResult{Status: doctorPass, Summary: "configuration validates"}
}

func (d *doctorState) checkConfigSecurity(ctx context.Context) doctorResult {
	warnings := config.SecurityWarnings(d.cfg)
	if len(warnings) == 0 {
		return doctorResult{Status: doctorPass, Summary: "no configuration security warnings"}
	}
	return doctorResult{Status: doctorWarn, Summary: "configuration has security warnings", Details: warnings}
}

func (d *doctorState) checkStoragePaths(ctx context.Context) doctorResult {
	root := config.RootPath()
	info, err := os.Stat(root)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "config root is inaccessible", Error: redact.Text(err.Error()), Details: []string{root}}
	}
	if !info.IsDir() {
		return doctorResult{Status: doctorFail, Summary: "config root is not a directory", Details: []string{root}}
	}
	details := []string{root}
	logs := filepath.Join(root, "logs")
	switch logInfo, err := os.Stat(logs); {
	case err == nil && !logInfo.IsDir():
		return doctorResult{Status: doctorFail, Summary: "logs path exists but is not a directory", Details: []string{logs}}
	case err == nil:
		details = append(details, logs)
	case os.IsNotExist(err):
		details = append(details, "logs not created yet")
	default:
		return doctorResult{Status: doctorFail, Summary: "logs path is inaccessible", Error: redact.Text(err.Error()), Details: []string{logs}}
	}
	return doctorResult{Status: doctorPass, Summary: "config and log paths are usable", Details: details}
}

func (d *doctorState) checkRuntimeControl(ctx context.Context) doctorResult {
	state, err := runtimecontrol.LoadContext(ctx)
	if runtimecontrol.IsUnavailable(err) {
		return doctorResult{Status: doctorSkip, Summary: "runtime is not running"}
	}
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "runtime control state is unreadable", Error: redact.Text(err.Error())}
	}
	details := []string{fmt.Sprintf("pid %d", state.PID), state.Address}
	if state.Managed {
		details = append(details, "managed "+state.ServiceScope)
	}
	if !pidAlive(state.PID) {
		return doctorResult{Status: doctorWarn, Summary: "runtime control state is stale", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: "runtime control state is reachable", Details: details}
}

func (d *doctorState) checkNetworkPlan(ctx context.Context) doctorResult {
	plan, err := resolveListenerPlan(d.cfg.Server.Expose)
	if err != nil {
		return doctorResult{Status: doctorFail, Summary: "listener plan is invalid", Error: redact.Text(err.Error())}
	}
	return doctorResult{Status: doctorPass, Summary: "listener plan resolved", Details: []string{fmt.Sprintf("%d addresses", len(plan.Addresses)), string(d.cfg.Server.Expose.Mode)}}
}

func (d *doctorState) checkNetworkHealth(ctx context.Context) doctorResult {
	if !d.cfg.Server.Enabled && !d.cfg.Admin.Enabled {
		return doctorResult{Status: doctorSkip, Summary: "no local HTTP listeners are enabled"}
	}
	if err := waitRuntimeHTTPReady(ctx, d.cfg, 2*time.Second); err != nil {
		return doctorResult{Status: doctorFail, Summary: "local HTTP health probe failed", Error: redact.Text(err.Error())}
	}
	return doctorResult{Status: doctorPass, Summary: "local HTTP listeners responded"}
}

func (d *doctorState) checkService(ctx context.Context, scopeName string) doctorResult {
	scope := managed.ScopeUser
	if scopeName == "system" {
		scope = managed.ScopeSystem
	}
	overview := application.LoadServiceOverview(scope)
	id := "user"
	if scope == managed.ScopeSystem {
		id = "system"
	}
	if !overview.Supported {
		return doctorResult{Status: doctorSkip, Summary: id + " managed service is not supported on this platform"}
	}
	if overview.Err != "" {
		return doctorResult{Status: doctorWarn, Summary: id + " managed service could not be inspected", Error: redact.Text(overview.Err)}
	}
	if !overview.Installed {
		return doctorResult{Status: doctorSkip, Summary: id + " managed service is not installed"}
	}
	details := []string{overview.Backend, overview.ID}
	if overview.Warning != "" {
		return doctorResult{Status: doctorWarn, Summary: id + " managed service is installed with a warning", Details: append(details, overview.Warning)}
	}
	if overview.Running {
		return doctorResult{Status: doctorPass, Summary: id + " managed service is running", Details: details}
	}
	return doctorResult{Status: doctorPass, Summary: id + " managed service is installed", Details: details}
}

func (d *doctorState) checkAuthMCP(ctx context.Context) doctorResult {
	err := application.MCPExposureError(d.cfg)
	switch {
	case err == nil:
		return doctorResult{Status: doctorPass, Summary: "Direct MCP HTTP authentication is configured"}
	case errors.Is(err, tunnelprovider.ErrMCPHTTPDisabled):
		return doctorResult{Status: doctorSkip, Summary: "MCP HTTP is disabled"}
	case d.cfg.Server.AllowUnauthenticatedLoopback && (errors.Is(err, tunnelprovider.ErrMCPAuthDisabled) || errors.Is(err, tunnelprovider.ErrMCPTokenMissing)):
		return doctorResult{Status: doctorWarn, Summary: "Direct MCP HTTP authentication is disabled on loopback"}
	default:
		return doctorResult{Status: doctorFail, Summary: "Direct MCP HTTP authentication is not ready", Error: redact.Text(err.Error())}
	}
}

func (d *doctorState) checkAuthAdmin(ctx context.Context) doctorResult {
	err := application.AdminExposureError(d.cfg)
	switch {
	case err == nil:
		return doctorResult{Status: doctorPass, Summary: "Admin authentication is configured"}
	case errors.Is(err, tunnelprovider.ErrAdminHTTPDisabled):
		return doctorResult{Status: doctorSkip, Summary: "Admin HTTP is disabled"}
	case d.cfg.Server.AllowUnauthenticatedLoopback && (errors.Is(err, tunnelprovider.ErrAdminAuthDisabled) || errors.Is(err, tunnelprovider.ErrAdminTokenMissing)):
		return doctorResult{Status: doctorWarn, Summary: "Admin authentication is disabled on loopback"}
	default:
		return doctorResult{Status: doctorFail, Summary: "Admin authentication is not ready", Error: redact.Text(err.Error())}
	}
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
