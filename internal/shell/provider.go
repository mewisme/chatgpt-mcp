package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

const bashCapability pluginpkg.Capability = "shell/bash"

var ErrBashUnavailable = errors.New("bash runtime is not installed")

type Provider struct {
	Executable string
	Language   string
	Source     string
	PluginID   pluginpkg.PluginID
	Version    pluginpkg.Version
	Path       []string
}

func (provider Provider) Label() string {
	if provider.PluginID != "" {
		return "plugin/" + string(provider.PluginID)
	}
	return provider.Source
}

type ProviderResolver struct {
	mu         sync.RWMutex
	configured string
	goos       string
	store      *pluginpkg.Store
	lookPath   func(string) (string, error)
}

func NewProviderResolver(store *pluginpkg.Store) *ProviderResolver {
	return &ProviderResolver{goos: runtime.GOOS, store: store, lookPath: exec.LookPath}
}

func DefaultProviderResolver() *ProviderResolver {
	store, _ := pluginpkg.NewStore(pluginpkg.DefaultLayout(), pluginpkg.RuntimeContext{})
	return NewProviderResolver(store)
}

func (resolver *ProviderResolver) SetConfiguredExecutable(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("shell executable must be absolute: %q", value)
		}
		value = filepath.Clean(value)
		if !isBashExecutable(value) {
			return fmt.Errorf("shell executable must be Bash: %s", value)
		}
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("shell executable %s: %w", value, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("shell executable is not a regular file: %s", value)
		}
	}
	resolver.mu.Lock()
	resolver.configured = value
	resolver.mu.Unlock()
	return nil
}

func (resolver *ProviderResolver) ConfiguredExecutable() string {
	if resolver == nil {
		return ""
	}
	resolver.mu.RLock()
	defer resolver.mu.RUnlock()
	return resolver.configured
}

func (resolver *ProviderResolver) Resolve() (Provider, error) {
	if resolver == nil {
		return Provider{}, missingBashError(runtime.GOOS)
	}
	resolver.mu.RLock()
	configured, goos, store, lookPath := resolver.configured, resolver.goos, resolver.store, resolver.lookPath
	resolver.mu.RUnlock()
	if configured != "" {
		return bashProvider(configured, "configured", "", ""), nil
	}
	if goos == "windows" && store != nil {
		plugins, err := pluginpkg.NewResolver(store)
		if err != nil {
			return Provider{}, fmt.Errorf("resolve Bash plugin capability: %w", err)
		}
		provider, err := plugins.Resolve(bashCapability)
		if err == nil {
			return bashProvider(provider.Path, "plugin", provider.PluginID, provider.Version), nil
		}
		if !errors.Is(err, pluginpkg.ErrCapabilityNotFound) {
			return Provider{}, err
		}
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if executable, err := lookPath("bash"); err == nil {
		return bashProvider(executable, "system", "", ""), nil
	}
	return Provider{}, missingBashError(goos)
}

func bashProvider(executable, source string, pluginID pluginpkg.PluginID, version pluginpkg.Version) Provider {
	executable = filepath.Clean(executable)
	return Provider{Executable: executable, Language: "bash", Source: source, PluginID: pluginID, Version: version, Path: []string{filepath.Dir(executable)}}
}

func missingBashError(goos string) error {
	if goos == "windows" {
		return fmt.Errorf("%w.\nInstall it with:\n  cgm plugin install bash", ErrBashUnavailable)
	}
	return fmt.Errorf("%w. Install Bash and ensure it is available on PATH", ErrBashUnavailable)
}

func isBashExecutable(value string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(value)))
	return base == "bash" || base == "bash.exe"
}
