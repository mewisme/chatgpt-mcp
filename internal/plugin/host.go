package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const hostCheckTimeout = 2 * time.Second

func resolveHostExecutable(artifact PlatformArtifact) (string, error) {
	if !artifact.HostBacked() || artifact.Host == nil {
		return "", errors.New("plugin platform is not host-backed")
	}
	path, err := exec.LookPath(artifact.Host.Executable)
	if err != nil {
		return "", hostPrerequisiteError(artifact.Host, fmt.Sprintf("required host executable %q is not installed or not on PATH", artifact.Host.Executable))
	}
	return path, nil
}

func preflightHostExecutable(ctx context.Context, artifact PlatformArtifact) (string, error) {
	path, err := resolveHostExecutable(artifact)
	if err != nil {
		return "", err
	}
	for index, check := range artifact.Host.Checks {
		if err := runHostCheck(ctx, path, check); err != nil {
			name := strings.TrimSpace(check.Name)
			if name == "" {
				name = fmt.Sprintf("check %d", index+1)
			}
			return "", hostPrerequisiteError(artifact.Host, fmt.Sprintf("host prerequisite %s failed: %v", name, err))
		}
	}
	return path, nil
}

func runHostCheck(ctx context.Context, path string, check HostExecutableCheck) error {
	runCtx, cancel := context.WithTimeout(nonNilContext(ctx), hostCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, check.Args...)
	cmd.Env = safeHookEnvironment()
	stdout, stderr := &boundedWrapperBuffer{}, &boundedWrapperBuffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	exitCode, err := hostExitCode(err)
	if err != nil {
		return err
	}
	if stdout.exceeded || stderr.exceeded {
		return fmt.Errorf("host check output exceeds %d-byte limit", maxWrapperOutputBytes)
	}
	accepted := check.SuccessExitCodes
	if len(accepted) == 0 {
		accepted = []int{0}
	}
	if !containsExitCode(accepted, exitCode) {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("exit code %d", exitCode)
		}
		return errors.New(message)
	}
	output := strings.TrimSpace(stdout.String())
	prefix := expandHostTemplate(check.StdoutPrefix, path)
	if prefix != "" && !strings.HasPrefix(output, prefix) {
		return fmt.Errorf("stdout does not start with %q", prefix)
	}
	contains := expandHostTemplate(check.StdoutContains, path)
	if contains != "" && !strings.Contains(output, contains) {
		return fmt.Errorf("stdout does not contain %q", contains)
	}
	return nil
}

func hostPrerequisiteError(host *HostExecutableSpec, message string) error {
	if host == nil || len(host.Install) == 0 {
		return errors.New(message)
	}
	var builder strings.Builder
	builder.WriteString(message)
	builder.WriteString("\nRecommended installation commands:")
	for _, hint := range host.Install {
		builder.WriteString("\n- ")
		if label := strings.TrimSpace(hint.Label); label != "" {
			builder.WriteString(label)
			builder.WriteString(": ")
		}
		builder.WriteString(strings.TrimSpace(hint.Command))
	}
	return errors.New(builder.String())
}

func hostExitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

func expandHostTemplate(value, path string) string {
	return strings.ReplaceAll(value, "{executable}", wrapperExecutableName(filepath.Base(path)))
}

func platformLockDigest(artifact PlatformArtifact) string {
	if !artifact.HostBacked() {
		return "sha256:" + artifact.SHA256
	}
	data, _ := json.Marshal(artifact.Host)
	sum := sha256.Sum256(append([]byte("host:"), data...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
