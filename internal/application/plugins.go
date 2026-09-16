package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/plugindev"
	"go.mewis.me/chatgpt-mcp/internal/pluginhost"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

type PluginService struct {
	Manager   *pluginpkg.Manager
	Layout    pluginpkg.Layout
	Workspace string
}

type InstalledPluginInfo struct {
	ID        pluginpkg.PluginID
	Origin    pluginpkg.Origin
	Lifecycle pluginpkg.Lifecycle
	Lock      pluginpkg.LockPlugin
	Installed pluginpkg.InstalledPlugin
	Scope     pluginpkg.PluginScope
	Workspace string
}

type MarketplacePluginInfo struct {
	Reference string
	Registry  pluginpkg.Registry
	ID        pluginpkg.PluginID
	Entry     pluginpkg.RegistryEntry
	Publisher pluginpkg.Publisher
	FetchedAt string
	Installed *pluginpkg.LockPlugin
}

type PluginDetail struct {
	Reference         string
	Manifest          pluginpkg.Manifest
	Registry          pluginpkg.Registry
	Publisher         pluginpkg.Publisher
	Installed         bool
	Enabled           bool
	Origin            pluginpkg.Origin
	Lifecycle         pluginpkg.Lifecycle
	SignatureStatus   string
	CoreCompatibility string
	Scope             pluginpkg.PluginScope
	Workspace         string
	Instructions      []pluginpkg.ProjectedResource
}

type PluginRegistryInfo struct {
	Registry pluginpkg.Registry
	Official bool
}

func NewPluginService() (*PluginService, error) {
	layout, err := plugindev.Prepare()
	if err != nil {
		return nil, err
	}
	return NewPluginServiceForLayout(layout)
}

func NewPluginServiceForOptions(opts PluginScopeOptions) (*PluginService, error) {
	if strings.TrimSpace(opts.Scope) == "" && strings.TrimSpace(opts.Workspace) == "" {
		return NewPluginService()
	}
	layout, err := ResolvePluginLayout(opts)
	if err != nil {
		return nil, err
	}
	service, err := NewPluginServiceForLayout(layout)
	if service != nil {
		service.Workspace = strings.TrimSpace(opts.Workspace)
	}
	return service, err
}

func NewPluginServiceForLayout(layout pluginpkg.Layout) (*PluginService, error) {
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{CoreVersion: version.Version})
	if err != nil {
		return nil, err
	}
	pluginhost.Attach(store)
	if err := AttachPluginPeers(store); err != nil {
		return nil, err
	}
	client := pluginpkg.RegistryClient{Layout: layout, UserAgent: "chatgpt-mcp/" + version.Version, LocalDir: pluginpkg.DefaultPluginBundleDir()}
	return &PluginService{Manager: &pluginpkg.Manager{Store: store, RegistryClient: client}, Layout: layout}, nil
}

func (service *PluginService) Installed() ([]InstalledPluginInfo, error) {
	if service == nil || service.Manager == nil || service.Manager.Store == nil {
		return nil, fmt.Errorf("plugin service is unavailable")
	}
	catalog, err := service.Manager.Catalog()
	if err != nil {
		return nil, err
	}
	items := make([]InstalledPluginInfo, 0, len(catalog))
	for _, entry := range catalog {
		items = append(items, InstalledPluginInfo{
			ID: entry.ID, Origin: entry.Origin, Lifecycle: entry.Lifecycle,
			Lock:      pluginpkg.LockPlugin{Registry: entry.Registry, Publisher: entry.Publisher, Version: entry.Version, Enabled: entry.Enabled},
			Installed: entry.Installed, Scope: service.Layout.EffectiveScope(), Workspace: service.Workspace,
		})
	}
	return items, nil
}

func (service *PluginService) Marketplace(ctx context.Context) ([]MarketplacePluginInfo, error) {
	if service == nil || service.Manager == nil {
		return nil, fmt.Errorf("plugin service is unavailable")
	}
	snapshots, err := service.Manager.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	lock, err := pluginpkg.LoadLock(service.Layout.LockPath())
	if err != nil {
		return nil, err
	}
	items := []MarketplacePluginInfo{}
	for _, snapshot := range snapshots {
		ids := make([]pluginpkg.PluginID, 0, len(snapshot.Index.Plugins))
		for id := range snapshot.Index.Plugins {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if _, ok := service.Manager.LookupBuiltin(id); ok {
				continue
			}
			entry := snapshot.Index.Plugins[id]
			publisher := snapshot.Publishers.Publishers[entry.Publisher]
			item := MarketplacePluginInfo{Reference: snapshot.Registry.Name + "/" + string(id), Registry: snapshot.Registry, ID: id, Entry: entry, Publisher: publisher, FetchedAt: snapshot.FetchedAt.UTC().Format("2006-01-02 15:04:05Z")}
			if installed, ok := lock.Plugins[id]; ok {
				copy := installed
				item.Installed = &copy
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Reference < items[j].Reference })
	return items, nil
}

func (service *PluginService) Outdated(ctx context.Context) ([]pluginpkg.OutdatedPlugin, error) {
	if service == nil || service.Manager == nil {
		return nil, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Outdated(ctx)
}

func (service *PluginService) Registries() ([]PluginRegistryInfo, error) {
	if service == nil {
		return nil, fmt.Errorf("plugin service is unavailable")
	}
	config, err := pluginpkg.LoadConfig(service.Layout.ConfigPath())
	if err != nil {
		return nil, err
	}
	registries := config.AllRegistries()
	items := make([]PluginRegistryInfo, 0, len(registries))
	for _, registry := range registries {
		items = append(items, PluginRegistryInfo{Registry: registry, Official: registry.Name == pluginpkg.OfficialRegistryName})
	}
	return items, nil
}

func (service *PluginService) InstalledDetail(id pluginpkg.PluginID) (PluginDetail, error) {
	if service == nil || service.Manager == nil || service.Manager.Store == nil {
		return PluginDetail{}, fmt.Errorf("plugin service is unavailable")
	}
	if builtin, ok := service.Manager.LookupBuiltin(id); ok {
		manifest := builtin.CatalogManifest(version.Version)
		return service.withScope(PluginDetail{
			Reference: string(id), Manifest: manifest, Registry: pluginpkg.Registry{Name: pluginpkg.BuiltinRegistryName},
			Publisher: pluginpkg.Publisher{Name: pluginpkg.BuiltinPublisher, Trusted: true, Source: "compiled into chatgpt-mcp"},
			Installed: true, Enabled: service.Manager.Store.BuiltinEnabled(id), Origin: pluginpkg.OriginBuiltin, Lifecycle: builtin.Lifecycle(),
			SignatureStatus: "built-in", CoreCompatibility: "compiled into chatgpt-mcp",
		}), nil
	}
	lock, err := pluginpkg.LoadLock(service.Layout.LockPath())
	if err != nil {
		return PluginDetail{}, err
	}
	entry, ok := lock.Plugins[id]
	if !ok {
		return PluginDetail{}, fmt.Errorf("plugin %s is not installed", id)
	}
	installed, err := service.Manager.Store.Installed(id, entry.Version)
	if err != nil {
		return PluginDetail{}, err
	}
	config, err := pluginpkg.LoadConfig(service.Layout.ConfigPath())
	if err != nil {
		return PluginDetail{}, err
	}
	var registry pluginpkg.Registry
	for _, candidate := range config.AllRegistries() {
		if candidate.Name == entry.Registry {
			registry = candidate
			break
		}
	}
	publisher := pluginpkg.Publisher{Name: entry.Publisher, Trusted: true}
	if registry.Name == pluginpkg.OfficialRegistryName {
		publisher.Source = "https://github.com/" + pluginpkg.OfficialSigstoreRepo
		publisher.Sigstore = pluginpkg.SigstoreIdentity{Issuer: pluginpkg.OfficialSigstoreIssuer, Repository: pluginpkg.OfficialSigstoreRepo}
	} else if registry.Trust != nil {
		publisher.Sigstore = *registry.Trust
	}
	if registry.Name != "" {
		if snapshot, cacheErr := service.Manager.RegistryClient.LoadCachedVerified(context.Background(), registry, 0); cacheErr == nil {
			if cached, ok := snapshot.Publishers.Publishers[entry.Publisher]; ok {
				publisher = cached
			}
		}
	}
	verification := "verified installed state"
	if entry.Registry == pluginpkg.RegistryLocalDev {
		verification = "local-dev"
	} else if verifyErr := service.Manager.Verify(context.Background(), id); verifyErr != nil {
		verification = "verification failed: " + verifyErr.Error()
	}
	instructions, _ := service.Manager.Store.InstructionResources(id)
	return service.withScope(PluginDetail{Reference: entry.Registry + "/" + string(id), Manifest: installed.Manifest, Registry: registry, Publisher: publisher, Installed: true, Enabled: entry.Enabled, Origin: pluginpkg.OriginInstalled, Lifecycle: pluginpkg.ArtifactLifecycle(), SignatureStatus: verification, CoreCompatibility: pluginCoreCompatibility(installed.Manifest), Instructions: instructions}), nil
}

func (service *PluginService) MarketplaceDetail(ctx context.Context, reference string) (PluginDetail, error) {
	if service == nil || service.Manager == nil {
		return PluginDetail{}, fmt.Errorf("plugin service is unavailable")
	}
	resolved, err := service.Manager.Resolve(ctx, reference)
	if err != nil {
		return PluginDetail{}, err
	}
	manifest, _, err := service.Manager.RegistryClient.FetchManifest(ctx, resolved)
	if err != nil {
		return PluginDetail{}, err
	}
	lock, err := pluginpkg.LoadLock(service.Layout.LockPath())
	if err != nil {
		return PluginDetail{}, err
	}
	entry, installed := lock.Plugins[resolved.PluginID]
	return service.withScope(PluginDetail{Reference: resolved.Registry.Name + "/" + string(resolved.PluginID), Manifest: manifest, Registry: resolved.Registry, Publisher: resolved.Publisher, Installed: installed, Enabled: installed && entry.Enabled, Origin: pluginpkg.OriginInstalled, Lifecycle: pluginpkg.ArtifactLifecycle(), SignatureStatus: "verified signed manifest", CoreCompatibility: pluginCoreCompatibility(manifest)}), nil
}

func (service *PluginService) Install(ctx context.Context, reference string) (pluginpkg.InstallResult, error) {
	if service == nil || service.Manager == nil {
		return pluginpkg.InstallResult{}, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Install(ctx, strings.TrimSpace(reference))
}

func (service *PluginService) InstallPortable(ctx context.Context, reference string) (pluginpkg.InstallResult, error) {
	if service == nil || service.Manager == nil {
		return pluginpkg.InstallResult{}, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.InstallWithOptions(ctx, strings.TrimSpace(reference), pluginpkg.InstallOptions{HostInstall: pluginpkg.HostInstallPortable})
}

func (service *PluginService) RunHostInstallHint(ctx context.Context, hint pluginpkg.HostInstallHint) (string, error) {
	if service == nil {
		return "", fmt.Errorf("plugin service is unavailable")
	}
	return pluginpkg.RunHostInstallHint(ctx, hint)
}

func (service *PluginService) Update(ctx context.Context, id pluginpkg.PluginID) (pluginpkg.InstallResult, error) {
	if service == nil || service.Manager == nil {
		return pluginpkg.InstallResult{}, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Update(ctx, id)
}

func (service *PluginService) Rollback(ctx context.Context, id pluginpkg.PluginID) (pluginpkg.InstallResult, error) {
	if service == nil || service.Manager == nil {
		return pluginpkg.InstallResult{}, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Rollback(ctx, id, "")
}

func (service *PluginService) Prune(id pluginpkg.PluginID) ([]pluginpkg.Version, error) {
	if service == nil || service.Manager == nil {
		return nil, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.PruneVersions(id, pluginpkg.DefaultRollbackRetention)
}

func (service *PluginService) Uninstall(ctx context.Context, id pluginpkg.PluginID, force bool) error {
	if service == nil || service.Manager == nil {
		return fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Uninstall(ctx, id, force)
}

func (service *PluginService) SetEnabled(ctx context.Context, id pluginpkg.PluginID, enabled bool) (err error) {
	if service == nil || service.Manager == nil || service.Manager.Store == nil {
		return fmt.Errorf("plugin service is unavailable")
	}
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.tui.toggle", "Changing plugin enabled state", tracepkg.String("plugin_id", string(id)), tracepkg.Bool("enabled", enabled))
	defer func() { span.Finish(err) }()
	return service.Manager.Store.SetEnabled(id, enabled)
}

func (service *PluginService) Verify(ctx context.Context, id pluginpkg.PluginID) error {
	if service == nil || service.Manager == nil {
		return fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Verify(ctx, id)
}

func (service *PluginService) AddRegistry(ctx context.Context, name, rawURL string, unqualified bool, trust pluginpkg.SigstoreIdentity) (err error) {
	if service == nil {
		return fmt.Errorf("plugin service is unavailable")
	}
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.registry.add", "Adding plugin registry", tracepkg.String("registry", strings.TrimSpace(name)), tracepkg.URL("url", rawURL))
	defer func() { span.Finish(err) }()
	return pluginpkg.MutateConfig(service.Layout, func(config *pluginpkg.Config) error {
		return config.AddRegistry(name, rawURL, unqualified, trust)
	})
}

func (service *PluginService) RemoveRegistry(ctx context.Context, name string) (err error) {
	if service == nil {
		return fmt.Errorf("plugin service is unavailable")
	}
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.registry.remove", "Removing plugin registry", tracepkg.String("registry", strings.TrimSpace(name)))
	defer func() { span.Finish(err) }()
	return pluginpkg.MutateConfig(service.Layout, func(config *pluginpkg.Config) error {
		return config.RemoveRegistry(name)
	})
}

func (service *PluginService) withScope(detail PluginDetail) PluginDetail {
	if service != nil {
		detail.Scope = service.Layout.EffectiveScope()
		detail.Workspace = service.Workspace
	}
	return detail
}

func pluginCoreCompatibility(manifest pluginpkg.Manifest) string {
	if version.Version == "" || version.Version == "dev" {
		return "not evaluated (development core)"
	}
	compatible, err := manifest.CompatibleWithCore(version.Version)
	if err != nil {
		return "unknown: " + err.Error()
	}
	if compatible {
		return "compatible with " + version.Version
	}
	return "incompatible with " + version.Version
}
