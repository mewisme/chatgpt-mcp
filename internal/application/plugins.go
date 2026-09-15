package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

type PluginService struct {
	Manager *pluginpkg.Manager
	Layout  pluginpkg.Layout
}

type InstalledPluginInfo struct {
	ID        pluginpkg.PluginID
	Lock      pluginpkg.LockPlugin
	Installed pluginpkg.InstalledPlugin
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
	SignatureStatus   string
	CoreCompatibility string
}

type PluginRegistryInfo struct {
	Registry pluginpkg.Registry
	Official bool
}

func NewPluginService() (*PluginService, error) {
	layout := pluginpkg.DefaultLayout()
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{CoreVersion: version.Version})
	if err != nil {
		return nil, err
	}
	client := pluginpkg.RegistryClient{Layout: layout, UserAgent: "chatgpt-mcp/" + version.Version}
	return &PluginService{Manager: &pluginpkg.Manager{Store: store, RegistryClient: client}, Layout: layout}, nil
}

func (service *PluginService) Installed() ([]InstalledPluginInfo, error) {
	if service == nil || service.Manager == nil || service.Manager.Store == nil {
		return nil, fmt.Errorf("plugin service is unavailable")
	}
	lock, err := pluginpkg.LoadLock(service.Layout.LockPath())
	if err != nil {
		return nil, err
	}
	ids := make([]pluginpkg.PluginID, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	items := make([]InstalledPluginInfo, 0, len(ids))
	for _, id := range ids {
		entry := lock.Plugins[id]
		installed, err := service.Manager.Store.Installed(id, entry.Version)
		if err != nil {
			return nil, fmt.Errorf("load installed plugin %s@%s: %w", id, entry.Version, err)
		}
		items = append(items, InstalledPluginInfo{ID: id, Lock: entry, Installed: installed})
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
	return PluginDetail{Reference: entry.Registry + "/" + string(id), Manifest: installed.Manifest, Registry: registry, Publisher: publisher, Installed: true, Enabled: entry.Enabled, SignatureStatus: "verified at install", CoreCompatibility: pluginCoreCompatibility(installed.Manifest)}, nil
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
	return PluginDetail{Reference: resolved.Registry.Name + "/" + string(resolved.PluginID), Manifest: manifest, Registry: resolved.Registry, Publisher: resolved.Publisher, Installed: installed, Enabled: installed && entry.Enabled, SignatureStatus: "verified signed manifest", CoreCompatibility: pluginCoreCompatibility(manifest)}, nil
}

func (service *PluginService) Install(ctx context.Context, reference string) (pluginpkg.InstallResult, error) {
	if service == nil || service.Manager == nil {
		return pluginpkg.InstallResult{}, fmt.Errorf("plugin service is unavailable")
	}
	return service.Manager.Install(ctx, strings.TrimSpace(reference))
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
	config, err := pluginpkg.LoadConfig(service.Layout.ConfigPath())
	if err != nil {
		return err
	}
	if err := config.AddRegistry(name, rawURL, unqualified, trust); err != nil {
		return err
	}
	return pluginpkg.WriteConfig(service.Layout.ConfigPath(), config)
}

func (service *PluginService) RemoveRegistry(ctx context.Context, name string) (err error) {
	if service == nil {
		return fmt.Errorf("plugin service is unavailable")
	}
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.registry.remove", "Removing plugin registry", tracepkg.String("registry", strings.TrimSpace(name)))
	defer func() { span.Finish(err) }()
	config, err := pluginpkg.LoadConfig(service.Layout.ConfigPath())
	if err != nil {
		return err
	}
	if err := config.RemoveRegistry(name); err != nil {
		return err
	}
	return pluginpkg.WriteConfig(service.Layout.ConfigPath(), config)
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
