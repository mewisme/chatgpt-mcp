package instructioncontext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type EnvironmentOptions struct {
	WorkspaceID    string
	WorkspaceRoot  string
	CWD            string
	EffectiveRoots []string
	AdminEnabled   bool
	AdminPort      int
}

func LoadEnvironmentSnapshot(opts EnvironmentOptions) (EnvironmentSnapshot, error) {
	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	if workspaceID == "" {
		return EnvironmentSnapshot{}, errors.New("workspace id is required")
	}
	workspaceRoot, err := canonicalEnvironmentPath(opts.WorkspaceRoot)
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("workspace root: %w", err)
	}
	cwdValue := strings.TrimSpace(opts.CWD)
	if cwdValue == "" {
		cwdValue = workspaceRoot
	}
	cwd, err := canonicalEnvironmentPath(cwdValue)
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("cwd: %w", err)
	}
	roots := normalizeEnvironmentRoots(opts.EffectiveRoots)
	if len(roots) == 0 {
		roots = []string{workspaceRoot}
	}
	if !withinAnyRoot(roots, workspaceRoot) {
		return EnvironmentSnapshot{}, errors.New("workspace root is outside effective roots")
	}
	if !withinAnyRoot(roots, cwd) {
		return EnvironmentSnapshot{}, errors.New("cwd is outside effective roots")
	}
	admin := AdminSnapshot{Enabled: opts.AdminEnabled}
	if opts.AdminEnabled {
		if opts.AdminPort < 1 || opts.AdminPort > 65535 {
			return EnvironmentSnapshot{}, fmt.Errorf("admin port must be between 1 and 65535: %d", opts.AdminPort)
		}
		admin.URL = fmt.Sprintf("http://127.0.0.1:%d/", opts.AdminPort)
	}
	return EnvironmentSnapshot{
		Platform: runtime.GOOS, OS: runtime.GOOS, Arch: runtime.GOARCH, Go: runtime.Version(), PID: os.Getpid(),
		WorkspaceID: workspaceID, WorkspaceRoot: workspaceRoot, CWD: cwd, EffectiveRoots: roots, Admin: admin,
	}, nil
}

func canonicalEnvironmentPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("path is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved := filepath.Clean(absolute)
	if canonical, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = filepath.Clean(canonical)
	}
	return resolved, nil
}

func normalizeEnvironmentRoots(values []string) []string {
	seen := map[string]bool{}
	roots := make([]string, 0, len(values))
	for _, value := range values {
		root, err := canonicalEnvironmentPath(value)
		if err != nil || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}
