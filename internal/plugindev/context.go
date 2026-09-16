package plugindev

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/version"
)

const (
	EnvRoot    = "CHATGPT_MCP_DEV_ROOT"
	EnvPlugins = "CHATGPT_MCP_DEV_PLUGINS"

	ModeAuto    = "auto"
	ModeOff     = "off"
	ModeRebuild = "rebuild"

	modulePrefix = "module go.mewis.me/chatgpt-mcp"
)

type Context struct {
	Enabled bool
	Root    string
	Mode    string
}

func Detect() (Context, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Context{}, err
	}
	return DetectAt(version.Version, cwd, os.Getenv)
}

func DetectAt(coreVersion, cwd string, getenv func(string) string) (Context, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	mode, err := parseMode(getenv(EnvPlugins))
	if err != nil {
		return Context{}, err
	}
	ctx := Context{Mode: mode}
	if mode == ModeOff || !version.IsDevelopment(coreVersion) {
		return ctx, nil
	}
	root, err := resolveRoot(cwd, getenv(EnvRoot))
	if err != nil {
		return Context{}, err
	}
	if root == "" {
		return ctx, nil
	}
	ctx.Enabled = true
	ctx.Root = root
	return ctx, nil
}

func parseMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ModeAuto:
		return ModeAuto, nil
	case ModeOff:
		return ModeOff, nil
	case ModeRebuild:
		return ModeRebuild, nil
	default:
		return "", fmt.Errorf("%s must be auto, off, or rebuild", EnvPlugins)
	}
}

func resolveRoot(cwd, override string) (string, error) {
	if override = strings.TrimSpace(override); override != "" {
		root, err := verifiedRoot(override)
		if err != nil {
			return "", fmt.Errorf("%s: %w", EnvRoot, err)
		}
		return root, nil
	}
	dir := cwd
	if dir == "" {
		return "", nil
	}
	for {
		root, err := verifiedRoot(dir)
		if err == nil {
			return root, nil
		}
		if !errors.Is(err, errUnverifiedRoot) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

var errUnverifiedRoot = errors.New("unverified repository root")

func verifiedRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errUnverifiedRoot
	}
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: %s does not exist", errUnverifiedRoot, cleaned)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", errUnverifiedRoot, cleaned)
	}
	mod, err := os.ReadFile(filepath.Join(cleaned, "go.mod"))
	if err != nil {
		return "", errUnverifiedRoot
	}
	if !hasModuleLine(string(mod)) {
		return "", errUnverifiedRoot
	}
	for _, rel := range []string{filepath.Join("plugins", "workflow.json"), filepath.Join("plugins", "registry", "index.json")} {
		info, err := os.Stat(filepath.Join(cleaned, rel))
		if err != nil || info.IsDir() {
			return "", errUnverifiedRoot
		}
	}
	return cleaned, nil
}

func hasModuleLine(source string) bool {
	for _, line := range strings.Split(source, "\n") {
		if strings.TrimSpace(line) == modulePrefix {
			return true
		}
	}
	return false
}
