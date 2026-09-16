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
const maxHostInstallOutputBytes = 1 << 20

type HostPrerequisiteError struct {
	Executable string
	Reason     string
	Install    []HostInstallHint
	Portable   *HostPortableInstall
}

func (err *HostPrerequisiteError) Error() string {
	if err == nil {
		return "host prerequisite unavailable"
	}
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(err.Reason))
	if err.Portable != nil {
		builder.WriteString("\n- Portable local: install the verified release binary into plugin data")
	}
	if len(err.Install) > 0 {
		builder.WriteString("\nRecommended global installation commands:")
		for _, hint := range err.Install {
			builder.WriteString("\n- ")
			if label := strings.TrimSpace(hint.Label); label != "" {
				builder.WriteString(label)
				builder.WriteString(": ")
			}
			builder.WriteString(hint.DisplayCommand())
		}
	}
	return builder.String()
}

func CheckHostPrerequisite(ctx context.Context, artifact PlatformArtifact) error {
	if !artifact.HostBacked() {
		return nil
	}
	_, err := preflightHostExecutable(ctx, artifact)
	return err
}

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
	if err := preflightHostPath(ctx, path, artifact.Host); err != nil {
		return "", err
	}
	return path, nil
}

func preflightHostPath(ctx context.Context, path string, host *HostExecutableSpec) error {
	if host == nil {
		return errors.New("host prerequisite configuration is unavailable")
	}
	for index, check := range host.Checks {
		if err := runHostCheck(ctx, path, check); err != nil {
			name := strings.TrimSpace(check.Name)
			if name == "" {
				name = fmt.Sprintf("check %d", index+1)
			}
			return hostPrerequisiteError(host, fmt.Sprintf("host prerequisite %s failed: %v", name, err))
		}
	}
	return nil
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
	if host == nil {
		return errors.New(message)
	}
	install := append([]HostInstallHint(nil), host.Install...)
	var portable *HostPortableInstall
	if host.Portable != nil {
		copy := *host.Portable
		portable = &copy
	}
	return &HostPrerequisiteError{Executable: host.Executable, Reason: message, Install: install, Portable: portable}
}

func RunHostInstallHint(ctx context.Context, hint HostInstallHint) (string, error) {
	if err := validateHostInstallHint(hint); err != nil {
		return "", err
	}
	if !hint.Runnable() {
		return "", errors.New("host install hint is display-only")
	}
	cmd := exec.CommandContext(nonNilContext(ctx), hint.Executable, hint.Args...)
	output := &limitedBuffer{limit: maxHostInstallOutputBytes}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	if output.exceeded {
		return output.String(), fmt.Errorf("host install output exceeds %d-byte limit", maxHostInstallOutputBytes)
	}
	if err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			message = err.Error()
		}
		return output.String(), fmt.Errorf("run %s: %s", hint.DisplayCommand(), message)
	}
	return output.String(), nil
}

type limitedBuffer struct {
	buffer   strings.Builder
	limit    int
	exceeded bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = original > 0
		return original, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		buffer.exceeded = true
	}
	_, _ = buffer.buffer.Write(data)
	return original, nil
}

func (buffer *limitedBuffer) String() string { return buffer.buffer.String() }

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
