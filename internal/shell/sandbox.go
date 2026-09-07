package shell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func wrapShellSandbox(ctx context.Context, cmd *exec.Cmd, cwd string, roots, shellPath []string, policy workspace.ShellSandboxPolicy) (*exec.Cmd, error) {
	if cmd == nil || policy == workspace.ShellSandboxOff || sandboxBypassApprovedHostMutation(ctx) || sandboxBypassApprovedControlPlane(ctx) {
		return cmd, nil
	}
	if runtime.GOOS != "linux" {
		if policy == workspace.ShellSandboxRequired {
			return nil, errors.New("shell sandbox is required but no supported OS sandbox is available")
		}
		return cmd, nil
	}
	bwrap := executableInPath("bwrap", trustedExecutablePath(shellPath))
	if bwrap == "" {
		if policy == workspace.ShellSandboxRequired {
			return nil, errors.New("shell sandbox is required but bubblewrap was not found in trusted executable paths")
		}
		return cmd, nil
	}
	args, err := bubblewrapArgs(cmd, cwd, roots, shellPath)
	if err != nil {
		if policy == workspace.ShellSandboxRequired {
			return nil, err
		}
		return cmd, nil
	}
	wrapped := exec.CommandContext(ctx, bwrap, args...)
	wrapped.Dir = cwd
	wrapped.Env = append([]string(nil), cmd.Env...)
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr
	return wrapped, nil
}

func sandboxBypassApprovedHostMutation(ctx context.Context) bool {
	grant, ok := controlguard.GrantFromContext(ctx)
	return ok && grant.Code == controlguard.CodeHostMutation
}

func sandboxBypassApprovedControlPlane(ctx context.Context) bool {
	_, ok := controlguard.ApprovalFromContext(ctx)
	return ok
}

func bubblewrapArgs(cmd *exec.Cmd, cwd string, roots, shellPath []string) ([]string, error) {
	if cmd == nil || strings.TrimSpace(cmd.Path) == "" {
		return nil, errors.New("shell sandbox requires an executable path")
	}
	args := []string{"--die-with-parent", "--new-session", "--unshare-pid", "--unshare-ipc", "--unshare-uts"}
	args = appendBubblewrapSystemMounts(args)
	args = append(args, "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp")
	rwRoots := collapseSandboxRoots(roots)
	for _, root := range rwRoots {
		if !filepath.IsAbs(root) {
			return nil, errors.New("shell sandbox root must be absolute")
		}
		args = appendBubblewrapParentDirs(args, root)
		args = append(args, "--bind", root, root)
	}
	roPaths := append([]string(nil), shellPath...)
	if directory := filepath.Dir(cmd.Path); directory != "." && directory != string(filepath.Separator) {
		roPaths = append(roPaths, directory)
	}
	for _, path := range collapseSandboxRoots(roPaths) {
		if sandboxCoveredBySystemMount(path) {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		args = appendBubblewrapParentDirs(args, path)
		args = append(args, "--ro-bind", path, path)
	}
	args = appendBubblewrapParentDirs(args, cwd)
	args = append(args, "--chdir", cwd, "--", cmd.Path)
	args = append(args, cmd.Args[1:]...)
	return args, nil
}

func appendBubblewrapSystemMounts(args []string) []string {
	for _, path := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64"} {
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err == nil {
				args = append(args, "--symlink", target, path)
			}
			continue
		}
		args = append(args, "--ro-bind", path, path)
	}
	if info, err := os.Stat("/etc"); err == nil && info.IsDir() {
		args = append(args, "--ro-bind", "/etc", "/etc")
	}
	return args
}

func appendBubblewrapParentDirs(args []string, path string) []string {
	parent := filepath.Dir(filepath.Clean(path))
	if parent == "." || parent == string(filepath.Separator) {
		return args
	}
	var parents []string
	for parent != "." && parent != string(filepath.Separator) {
		parents = append(parents, parent)
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	for index := len(parents) - 1; index >= 0; index-- {
		if sandboxCoveredBySystemMount(parents[index]) {
			continue
		}
		args = append(args, "--dir", parents[index])
	}
	return args
}

func collapseSandboxRoots(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !filepath.IsAbs(value) {
			continue
		}
		value = filepath.Clean(value)
		duplicate := false
		for _, existing := range cleaned {
			if existing == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			cleaned = append(cleaned, value)
		}
	}
	sort.Slice(cleaned, func(i, j int) bool {
		if len(cleaned[i]) == len(cleaned[j]) {
			return cleaned[i] < cleaned[j]
		}
		return len(cleaned[i]) < len(cleaned[j])
	})
	result := make([]string, 0, len(cleaned))
	for _, value := range cleaned {
		covered := false
		for _, root := range result {
			if sandboxPathWithin(root, value) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, value)
		}
	}
	return result
}

func sandboxPathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)))
}

func sandboxCoveredBySystemMount(path string) bool {
	path = filepath.Clean(path)
	for _, root := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc"} {
		if sandboxPathWithin(root, path) {
			return true
		}
	}
	return false
}
