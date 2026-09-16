package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

type TunnelProviderRef struct {
	Provider string
	PluginID pluginpkg.PluginID
	Name     string
	Version  pluginpkg.Version
	Path     string
	WorkDir  string
	Enabled  bool
}

type ProviderNotInstalledError struct {
	Provider string
	Install  string
}

func (err ProviderNotInstalledError) Error() string {
	msg := fmt.Sprintf("tunnel provider %q is not installed or enabled", err.Provider)
	if err.Install != "" {
		return msg + "; install with " + err.Install
	}
	return msg
}

func (err ProviderNotInstalledError) Unwrap() error { return tunnelprovider.ErrNotInstalled }

func ListTunnelProviders() ([]TunnelProviderRef, error) {
	service, err := NewPluginService()
	if err != nil {
		return nil, err
	}
	resolver, err := pluginpkg.NewResolver(service.Manager.Store)
	if err != nil {
		return nil, err
	}
	refs := make([]TunnelProviderRef, 0)
	for _, capability := range resolver.Capabilities(tunnelprovider.Prefix) {
		provider, err := resolver.Resolve(capability)
		if err != nil {
			return nil, err
		}
		refs = append(refs, refFromProvider(tunnelprovider.ProviderName(string(capability)), provider, true))
	}
	return refs, nil
}

func LookupTunnelProvider(name string) (TunnelProviderRef, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return TunnelProviderRef{}, fmt.Errorf("%w: provider is required", tunnelprovider.ErrInvalidDescriptor)
	}
	service, err := NewPluginService()
	if err != nil {
		return TunnelProviderRef{}, err
	}
	capability := pluginpkg.Capability(tunnelprovider.Capability(name))
	store := service.Manager.Store
	if resolver, err := pluginpkg.NewResolver(store); err == nil {
		if provider, err := resolver.Resolve(capability); err == nil {
			return refFromProvider(name, provider, true), nil
		}
	}
	lock, err := pluginpkg.LoadLock(store.Layout().LockPath())
	if err != nil {
		return TunnelProviderRef{}, err
	}
	for id, entry := range lock.Plugins {
		installed, instErr := store.Installed(id, entry.Version)
		if instErr != nil {
			continue
		}
		if providesTunnel(installed.Manifest.Provides, capability) {
			return TunnelProviderRef{Provider: name, PluginID: id, Name: installed.Manifest.Name, Version: entry.Version, Path: installed.Entrypoint, WorkDir: installed.Payload, Enabled: entry.Enabled}, nil
		}
	}
	for _, builtin := range store.Builtins {
		if providesTunnel(builtin.Provides, capability) {
			return TunnelProviderRef{Provider: name, PluginID: builtin.ID, Name: builtin.Name, Enabled: store.BuiltinEnabled(builtin.ID)}, nil
		}
	}
	return TunnelProviderRef{}, missingProviderError(name, service)
}

func StartTunnelProvider(ctx context.Context, cfg config.Config, provider, target string) error {
	ref, err := LookupTunnelProvider(provider)
	if err != nil {
		return err
	}
	targets, err := parseProviderTargets(target, nil)
	if err != nil {
		return err
	}
	for _, id := range targets {
		kind, err := originKindForTarget(id, nil)
		if err != nil {
			return err
		}
		if _, err := tunnelprovider.ResolveOrigin(cfg, kind); err != nil {
			return err
		}
	}
	service, err := NewPluginService()
	if err != nil {
		return err
	}
	if err := service.SetEnabled(ctx, ref.PluginID, true); err != nil {
		return err
	}
	for _, id := range targets {
		if err := service.SetPluginSetting(ctx, ref.PluginID, id, "true"); err != nil {
			return err
		}
	}
	if _, running, err := RuntimeStatus(ctx); err == nil && running {
		var status runtimecontrol.TunnelProviderStatus
		if _, err := runtimecontrol.Request(ctx, "POST", "/tunnel-providers/start", map[string]string{"provider": provider, "target": target}, &status); err != nil && !runtimecontrol.IsUnavailable(err) {
			return err
		}
	}
	return nil
}

func StopTunnelProvider(ctx context.Context, provider, target string) error {
	ref, err := LookupTunnelProvider(provider)
	if err != nil {
		return err
	}
	targets, err := parseProviderTargets(target, nil)
	if err != nil {
		return err
	}
	service, err := NewPluginService()
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(target), "all") {
		if err := service.SetEnabled(ctx, ref.PluginID, false); err != nil {
			return err
		}
	}
	for _, id := range targets {
		if err := service.SetPluginSetting(ctx, ref.PluginID, id, "false"); err != nil {
			return err
		}
	}
	if _, running, err := RuntimeStatus(ctx); err == nil && running {
		var status runtimecontrol.TunnelProviderStatus
		if _, err := runtimecontrol.Request(ctx, "POST", "/tunnel-providers/stop", map[string]string{"provider": provider, "target": target}, &status); err != nil && !runtimecontrol.IsUnavailable(err) {
			return err
		}
	}
	return nil
}

func DescribeTunnelProvider(ctx context.Context, host *runtimeplugin.Host, ref TunnelProviderRef) (runtimeplugin.DescribeResult, error) {
	if strings.TrimSpace(ref.Path) == "" {
		return runtimeplugin.DescribeResult{}, errors.New("tunnel provider has no executable payload")
	}
	session, err := host.Ensure(ctx, runtimeplugin.Spec{
		ID: string(ref.PluginID), Version: string(ref.Version), Entrypoint: ref.Path, WorkDir: ref.WorkDir,
		DataDir: pluginpkg.DefaultLayout().PluginRuntimeDataDir(ref.PluginID),
	})
	if err != nil {
		return runtimeplugin.DescribeResult{}, err
	}
	raw, err := session.Call(ctx, runtimeplugin.MethodDescribe, nil)
	if err != nil {
		return runtimeplugin.DescribeResult{}, err
	}
	var desc runtimeplugin.DescribeResult
	if err := json.Unmarshal(raw, &desc); err != nil {
		return runtimeplugin.DescribeResult{}, err
	}
	if err := tunnelprovider.ValidateDescriptor(ref.Provider, desc); err != nil {
		return runtimeplugin.DescribeResult{}, err
	}
	return desc, nil
}

func StartTunnelProviderProcess(ctx context.Context, cfg config.Config, host *runtimeplugin.Host, ref TunnelProviderRef, target string) error {
	desc, err := DescribeTunnelProvider(ctx, host, ref)
	if err != nil {
		return err
	}
	targets, err := parseProviderTargets(target, &desc)
	if err != nil {
		return err
	}
	session, ok := host.Get(string(ref.PluginID))
	if !ok {
		return errors.New("tunnel provider process is unavailable")
	}
	for _, id := range targets {
		item, err := tunnelprovider.Target(desc, id)
		if err != nil {
			return err
		}
		origin, err := tunnelprovider.ResolveOrigin(cfg, item.OriginKind)
		if err != nil {
			return err
		}
		params := origin.StartParams(id)
		if _, err := session.Call(ctx, runtimeplugin.MethodStart, params); err != nil {
			return err
		}
	}
	return nil
}

func StopTunnelProviderProcess(ctx context.Context, host *runtimeplugin.Host, ref TunnelProviderRef, target string) error {
	session, ok := host.Get(string(ref.PluginID))
	if !ok {
		return nil
	}
	targets, err := parseProviderTargets(target, nil)
	if err != nil {
		return err
	}
	for _, id := range targets {
		if _, err := session.Call(ctx, runtimeplugin.MethodStop, runtimeplugin.StopParams{Target: id}); err != nil {
			return err
		}
	}
	if strings.EqualFold(strings.TrimSpace(target), "all") {
		return host.Shutdown(ctx, string(ref.PluginID))
	}
	return nil
}

func parseProviderTargets(target string, desc *runtimeplugin.DescribeResult) ([]string, error) {
	value := strings.ToLower(strings.TrimSpace(target))
	if value == "" {
		value = "all"
	}
	if value == "all" {
		if desc != nil {
			ids := make([]string, 0, len(desc.Targets))
			for _, item := range desc.Targets {
				ids = append(ids, item.ID)
			}
			return ids, nil
		}
		return []string{"mcp", "admin"}, nil
	}
	if desc != nil {
		if _, err := tunnelprovider.Target(*desc, value); err != nil {
			return nil, err
		}
	}
	return []string{value}, nil
}

func originKindForTarget(target string, desc *runtimeplugin.DescribeResult) (string, error) {
	if desc != nil {
		item, err := tunnelprovider.Target(*desc, target)
		if err != nil {
			return "", err
		}
		return item.OriginKind, nil
	}
	switch target {
	case "mcp":
		return tunnelprovider.OriginMCP, nil
	case "admin":
		return tunnelprovider.OriginAdmin, nil
	default:
		return "", fmt.Errorf("%w: %q", tunnelprovider.ErrInvalidDescriptor, target)
	}
}

func refFromProvider(name string, provider pluginpkg.CapabilityProvider, enabled bool) TunnelProviderRef {
	workDir := ""
	if provider.Path != "" {
		workDir = filepath.Dir(provider.Path)
	}
	return TunnelProviderRef{Provider: name, PluginID: provider.PluginID, Name: provider.Name, Version: provider.Version, Path: provider.Path, WorkDir: workDir, Enabled: enabled}
}

func providesTunnel(capabilities []pluginpkg.Capability, want pluginpkg.Capability) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func ConfiguredTunnelProviderStatus(cfg config.Config, provider string) (runtimecontrol.TunnelProviderStatus, error) {
	ref, err := LookupTunnelProvider(provider)
	if err != nil {
		return runtimecontrol.TunnelProviderStatus{}, err
	}
	service, err := NewPluginService()
	if err != nil {
		return runtimecontrol.TunnelProviderStatus{}, err
	}
	schema := tunnelProviderSettingsSchema(service, ref.PluginID)
	values := map[string]any{}
	if loaded, err := service.Manager.SettingsStore().Get(schema, ref.PluginID); err == nil {
		values = loaded
	}
	targets := make([]runtimecontrol.TunnelProviderTargetStatus, 0, 2)
	for _, id := range []string{"mcp", "admin"} {
		item := runtimecontrol.TunnelProviderTargetStatus{Target: id, Desired: ref.Enabled && pluginSettingBool(values, id)}
		if item.Desired {
			kind, kindErr := originKindForTarget(id, nil)
			if kindErr != nil {
				item.LastError = kindErr.Error()
			} else if _, originErr := tunnelprovider.ResolveOrigin(cfg, kind); originErr != nil {
				item.LastError = originErr.Error()
			}
		}
		targets = append(targets, item)
	}
	return runtimecontrol.TunnelProviderStatus{Provider: ref.Provider, Name: ref.Name, PluginID: string(ref.PluginID), Enabled: ref.Enabled, Targets: targets}, nil
}

func ListConfiguredTunnelProviders(cfg config.Config) []runtimecontrol.TunnelProviderStatus {
	seen := map[string]struct{}{}
	out := make([]runtimecontrol.TunnelProviderStatus, 0)
	add := func(provider string) {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			return
		}
		if _, ok := seen[provider]; ok {
			return
		}
		item, err := ConfiguredTunnelProviderStatus(cfg, provider)
		if err != nil {
			return
		}
		seen[provider] = struct{}{}
		out = append(out, item)
	}
	if refs, err := ListTunnelProviders(); err == nil {
		for _, ref := range refs {
			add(ref.Provider)
		}
	}
	add("cf")
	return out
}

func MergeTunnelProviderStatus(cfg config.Config, runtime runtimecontrol.RuntimeStatus) []runtimecontrol.TunnelProviderStatus {
	if len(runtime.TunnelProviders) > 0 {
		return runtime.TunnelProviders
	}
	if runtime.CFTunnel != nil {
		return []runtimecontrol.TunnelProviderStatus{runtime.CFTunnel.AsProvider()}
	}
	return ListConfiguredTunnelProviders(cfg)
}

func MarketplaceTunnelProviders() []string {
	service, err := NewPluginService()
	if err != nil || service == nil || service.Manager == nil {
		return nil
	}
	snapshot, err := service.Manager.RegistryClient.LoadCached(pluginpkg.OfficialRegistry(), 24*time.Hour)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	names := make([]string, 0)
	for _, entry := range snapshot.Index.Plugins {
		for _, capability := range entry.Provides {
			if !strings.HasPrefix(string(capability), tunnelprovider.Prefix) {
				continue
			}
			name := tunnelprovider.ProviderName(string(capability))
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	return names
}

func KnownTunnelProviderNames() []string {
	seen := map[string]struct{}{"cf": {}}
	names := []string{"cf"}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if refs, err := ListTunnelProviders(); err == nil {
		for _, ref := range refs {
			add(ref.Provider)
		}
	}
	for _, name := range MarketplaceTunnelProviders() {
		add(name)
	}
	return names
}

func tunnelProviderSettingsSchema(service *PluginService, id pluginpkg.PluginID) pluginpkg.SettingsSchema {
	if service != nil && service.Manager != nil {
		if schema, err := service.Manager.SettingsSchema(id); err == nil && len(schema.Fields) > 0 {
			return schema
		}
	}
	return pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
		{Key: "mcp", Kind: pluginpkg.FieldBool, Title: "Expose MCP HTTP", Default: false},
		{Key: "admin", Kind: pluginpkg.FieldBool, Title: "Expose Admin HTTP", Default: false},
	}}
}

func pluginSettingBool(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func missingProviderError(name string, service *PluginService) error {
	result := ProviderNotInstalledError{Provider: name}
	if service == nil || service.Manager == nil {
		return result
	}
	capability := pluginpkg.Capability(tunnelprovider.Capability(name))
	id := pluginpkg.PluginID(name + "-tunnel")
	snapshot, err := service.Manager.RegistryClient.LoadCached(pluginpkg.OfficialRegistry(), 24*time.Hour)
	if err != nil {
		return result
	}
	if _, ok := snapshot.Index.Plugins[id]; ok {
		result.Install = "cgm plugin install " + string(id)
		return result
	}
	for pluginID, entry := range snapshot.Index.Plugins {
		if providesTunnel(entry.Provides, capability) {
			result.Install = "cgm plugin install " + string(pluginID)
			return result
		}
	}
	return result
}
