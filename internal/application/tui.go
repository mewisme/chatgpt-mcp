package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

const terminalUICapability = pluginpkg.Capability("terminal-ui/default")

var ErrTerminalUIMissing = errors.New("tui core plugin is not installed")

func RunTerminalUI(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	provider, err := LookupTerminalUI()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, provider.Path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errOut
	cmd.Env = append(os.Environ(), configformat.EnvConfigDir+"="+config.RootPath())
	return cmd.Run()
}

func LookupTerminalUI() (pluginpkg.CapabilityProvider, error) {
	service, err := NewPluginService()
	if err != nil {
		return pluginpkg.CapabilityProvider{}, terminalUIMissing()
	}
	store := service.Manager.Store
	if resolver, err := pluginpkg.NewResolver(store); err == nil {
		if provider, err := resolver.Resolve(terminalUICapability); err == nil && strings.TrimSpace(provider.Path) != "" {
			return provider, nil
		}
	}
	lock, err := pluginpkg.LoadLock(store.Layout().LockPath())
	if err != nil {
		return pluginpkg.CapabilityProvider{}, terminalUIMissing()
	}
	for id, entry := range lock.Plugins {
		installed, instErr := store.Installed(id, entry.Version)
		if instErr != nil {
			continue
		}
		if !providesCapability(installed.Manifest.Provides, terminalUICapability) {
			continue
		}
		if !entry.Enabled {
			return pluginpkg.CapabilityProvider{}, fmt.Errorf("tui core plugin is installed but disabled. Enable with: cgm plugin enable tui")
		}
		if strings.TrimSpace(installed.Entrypoint) == "" {
			return pluginpkg.CapabilityProvider{}, terminalUIMissing()
		}
		return pluginpkg.CapabilityProvider{PluginID: id, Version: entry.Version, Name: installed.Manifest.Name, Path: installed.Entrypoint}, nil
	}
	return pluginpkg.CapabilityProvider{}, terminalUIMissing()
}

func providesCapability(provides []pluginpkg.Capability, want pluginpkg.Capability) bool {
	for _, capability := range provides {
		if capability == want {
			return true
		}
	}
	return false
}

func terminalUIMissing() error {
	return fmt.Errorf("%w. Repair with: cgm plugin install tui", ErrTerminalUIMissing)
}
