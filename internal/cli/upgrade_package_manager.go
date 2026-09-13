package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

type packageManagerRunFunc func(context.Context, updatepkg.PackageManagerCommand) (string, error)
type packageBinaryLookupFunc func(string) (string, error)
type packageBinaryVersionFunc func(context.Context, string) (string, error)

func runPackageManagedUpgrade(cmd *cobra.Command, detection install.Detection, targetVersion string, noRestart bool) error {
	plan, ok := updatepkg.PackageManagerPlanFor(detection.Method)
	if !ok {
		return fmt.Errorf("package manager update plan unavailable for %s", detection.Method)
	}
	if strings.TrimSpace(targetVersion) != "" {
		return fmt.Errorf("--version is unavailable for %s installations; package-manager upgrades follow the latest published manifest", plan.Name)
	}
	log := commandLogger(cmd)
	startCommandSpinner(cmd, log, "UPDATE", "update.checking", "Checking for updates")
	checker := updatepkg.Checker{Source: updatepkg.Client{UserAgent: "chatgpt-mcp/" + version.Version}}
	check, err := checker.Check(cmd.Context(), version.Version)
	log.StopAnimation()
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}
	switch check.Status {
	case updatepkg.StatusUpToDate:
		log.Ready("UPDATE", "update.current", "Already up to date")
		log.Detail("current", check.Current)
		log.Detail("latest", check.Latest)
		return nil
	case updatepkg.StatusAhead:
		log.Notice("UPDATE", "update.ahead", "Current version is newer than the latest release")
		log.Detail("current", check.Current)
		log.Detail("latest", check.Latest)
		return nil
	case updatepkg.StatusDevelopment:
		return updatepkg.ErrDevelopmentUpdate
	case updatepkg.StatusAvailable:
		log.Ready("UPDATE", "update.available", "Update available")
		log.Detail("current", check.Current)
		log.Detail("latest", check.Latest)
	default:
		return fmt.Errorf("unknown update status %q", check.Status)
	}

	logCommandStep(cmd, "UPDATE", "update.runtime.inspecting", "Inspecting managed runtime state")
	runtimeState, err := captureUpdateRuntimeState(cmd.Context())
	if err != nil {
		return fmt.Errorf("inspect managed runtime before update: %w", err)
	}

	if err := runPackageManagerPhase(cmd, log, plan, "refresh", runPackageManagerCommand); err != nil {
		return err
	}
	if err := runPackageManagerPhase(cmd, log, plan, "apply", runPackageManagerCommand); err != nil {
		return err
	}

	startCommandSpinner(cmd, log, "UPDATE", "update.package.verify", "Verifying installed version")
	binary, installedVersion, err := verifyPackageManagedVersion(cmd.Context(), check.Latest, exec.LookPath, runPackageBinaryVersion)
	log.StopAnimation()
	if err != nil {
		return fmt.Errorf("verify %s update: %w", plan.Name, err)
	}
	log.Ready("UPDATE", "update.applied", "Update applied")
	log.Detail("previous", check.Current)
	log.Detail("current", installedVersion)
	log.Detail("binary", binary)

	if err := coordinatePackageManagedRuntime(cmd, binary, runtimeState, noRestart); err != nil {
		return fmt.Errorf("update to %s installed but runtime coordination failed: %w", installedVersion, err)
	}
	log.Success("UPDATE", "Update complete")
	return nil
}

func runPackageManagerPhase(cmd *cobra.Command, log *logger.Logger, plan updatepkg.PackageManagerPlan, phase string, run packageManagerRunFunc) error {
	command := plan.Refresh
	message := "Refreshing " + plan.Name + " metadata"
	done := plan.Name + " metadata refreshed"
	event := "update.package.refresh"
	if phase == "apply" {
		command = plan.Apply
		message = "Applying update via " + plan.Name
		done = plan.Name + " update command completed"
		event = "update.package.apply"
	}
	startCommandSpinner(cmd, log, "UPDATE", event, message)
	output, err := run(cmd.Context(), command)
	log.StopAnimation()
	if strings.TrimSpace(output) != "" {
		logCommandDebug(cmd, "UPDATE", event+".output", plan.Name+" command output", logger.WithDebug("output", strings.TrimSpace(output)))
	}
	if err != nil {
		return fmt.Errorf("%s: %w", strings.ToLower(message), err)
	}
	log.Ready("UPDATE", event+".complete", done)
	return nil
}

func runPackageManagerCommand(ctx context.Context, command updatepkg.PackageManagerCommand) (string, error) {
	var process *exec.Cmd
	if runtime.GOOS == "windows" && strings.EqualFold(command.Name, "scoop") {
		shell, err := packagePowerShell()
		if err != nil {
			return "", err
		}
		statement := "& scoop"
		if len(command.Args) > 0 {
			statement += " " + strings.Join(command.Args, " ")
		}
		process = exec.CommandContext(ctx, shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", statement)
	} else {
		process = exec.CommandContext(ctx, command.Name, command.Args...)
	}
	output, err := process.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err == nil {
		return text, nil
	}
	if text == "" {
		return "", err
	}
	return text, fmt.Errorf("%w: %s", err, text)
}

func packagePowerShell() (string, error) {
	for _, name := range []string{"pwsh", "powershell.exe", "powershell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("PowerShell is required to run Scoop")
}

func verifyPackageManagedVersion(ctx context.Context, target string, lookup packageBinaryLookupFunc, readVersion packageBinaryVersionFunc) (string, string, error) {
	target, err := updatepkg.NormalizeVersion(target)
	if err != nil {
		return "", "", err
	}
	var binary string
	for _, name := range []string{"chatgpt-mcp", "cgm"} {
		binary, err = lookup(name)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", "", errors.New("updated chatgpt-mcp command was not found on PATH")
	}
	output, err := readVersion(ctx, binary)
	if err != nil {
		return binary, "", err
	}
	installed, err := packageVersionFromOutput(output)
	if err != nil {
		return binary, "", err
	}
	comparison, err := updatepkg.CompareVersions(installed, target)
	if err != nil {
		return binary, installed, err
	}
	if comparison < 0 {
		return binary, installed, fmt.Errorf("package metadata did not install %s; installed version is %s", target, installed)
	}
	return binary, installed, nil
}

func runPackageBinaryVersion(ctx context.Context, binary string) (string, error) {
	output, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err == nil {
		return text, nil
	}
	if text == "" {
		return "", err
	}
	return text, fmt.Errorf("%w: %s", err, text)
}

func packageVersionFromOutput(output string) (string, error) {
	for _, field := range strings.Fields(output) {
		candidate := strings.Trim(field, "()[]{}<>,;:\"")
		if version, err := updatepkg.NormalizeVersion(candidate); err == nil {
			return version, nil
		}
	}
	return "", fmt.Errorf("unable to parse installed version from %q", strings.TrimSpace(output))
}
