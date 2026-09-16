package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	OriginBuiltin   Origin = "builtin"
	OriginInstalled Origin = "installed"

	BuiltinRegistryName = "builtin"
	BuiltinPublisher    = "chatgpt-mcp"
)

var ErrBuiltinPlugin = errors.New("built-in plugin does not support artifact lifecycle")

type Origin string

func (origin Origin) Label() string {
	if origin == OriginBuiltin {
		return "Built-in"
	}
	return "Installed"
}

type Lifecycle struct {
	Install   bool `json:"install"`
	Uninstall bool `json:"uninstall"`
	Enable    bool `json:"enable"`
	Disable   bool `json:"disable"`
	Update    bool `json:"update"`
	Rollback  bool `json:"rollback"`
	Prune     bool `json:"prune"`
	Verify    bool `json:"verify"`
	Configure bool `json:"configure"`
}

func ArtifactLifecycle() Lifecycle {
	return Lifecycle{Install: true, Uninstall: true, Enable: true, Disable: true, Update: true, Rollback: true, Prune: true, Verify: true}
}

type Builtin struct {
	ID             PluginID
	Name           string
	Type           PluginType
	Provides       []Capability
	Permissions    []Permission
	Schema         SettingsSchema
	Scopes         []PluginScope
	Disableable    bool
	DefaultEnabled bool
	Description    string
}

func (builtin Builtin) Validate() error {
	if !validCanonicalName(string(builtin.ID)) {
		return fmt.Errorf("invalid built-in plugin id: %q", builtin.ID)
	}
	if strings.TrimSpace(builtin.Name) == "" {
		return errors.New("built-in plugin name is required")
	}
	if _, ok := knownPluginTypes[builtin.Type]; !ok {
		return fmt.Errorf("unsupported built-in plugin type: %q", builtin.Type)
	}
	return validatePluginScopes(builtin.AllowedScopes())
}

func (builtin Builtin) Origin() Origin { return OriginBuiltin }

func (builtin Builtin) Lifecycle() Lifecycle {
	return Lifecycle{Enable: builtin.Disableable, Disable: builtin.Disableable, Verify: true, Configure: len(builtin.Schema.Fields) > 0}
}

func (builtin Builtin) CatalogManifest(coreVersion string) Manifest {
	return Manifest{
		ID:          builtin.ID,
		Name:        builtin.Name,
		Publisher:   BuiltinPublisher,
		Version:     catalogCoreVersion(coreVersion),
		Type:        builtin.Type,
		Provides:    append([]Capability(nil), builtin.Provides...),
		Permissions: append([]Permission(nil), builtin.Permissions...),
	}
}

func catalogCoreVersion(value string) Version {
	value = strings.TrimSpace(value)
	if value == "" {
		return "dev"
	}
	return Version(value)
}

type BuiltinRegistry []Builtin

func (registry BuiltinRegistry) Validate() error {
	seen := map[PluginID]struct{}{}
	for _, builtin := range registry {
		if err := builtin.Validate(); err != nil {
			return err
		}
		if _, ok := seen[builtin.ID]; ok {
			return fmt.Errorf("duplicate built-in plugin id: %s", builtin.ID)
		}
		seen[builtin.ID] = struct{}{}
	}
	return nil
}

func (registry BuiltinRegistry) Lookup(id PluginID) (Builtin, bool) {
	for _, builtin := range registry {
		if builtin.ID == id {
			return builtin, true
		}
	}
	return Builtin{}, false
}

type CatalogPlugin struct {
	ID        PluginID
	Origin    Origin
	Lifecycle Lifecycle
	Enabled   bool
	Version   Version
	Registry  string
	Publisher string
	Installed InstalledPlugin
	Schema    SettingsSchema
	Scopes    []PluginScope
}

func (manager Manager) LookupBuiltin(id PluginID) (Builtin, bool) {
	if manager.Store == nil {
		return Builtin{}, false
	}
	return manager.Store.Builtins.Lookup(id)
}

func (manager Manager) SettingsSchema(id PluginID) (SettingsSchema, error) {
	if builtin, ok := manager.LookupBuiltin(id); ok {
		if len(builtin.Schema.Fields) == 0 {
			return SettingsSchema{}, fmt.Errorf("%w: %s", ErrNoPluginConfig, id)
		}
		return builtin.Schema, nil
	}
	if _, err := manager.CatalogPlugin(id); err != nil {
		return SettingsSchema{}, err
	}
	return SettingsSchema{}, fmt.Errorf("%w: %s", ErrNoPluginConfig, id)
}

func (manager Manager) SettingsStore() SettingsStore {
	if manager.Store == nil {
		return SettingsStore{}
	}
	return SettingsStore{Layout: manager.Store.Layout()}
}

func (manager Manager) rejectBuiltinArtifact(id PluginID) error {
	if _, ok := manager.LookupBuiltin(id); ok {
		return fmt.Errorf("%w: %s", ErrBuiltinPlugin, id)
	}
	return nil
}

func (manager Manager) Catalog() ([]CatalogPlugin, error) {
	if manager.Store == nil {
		return nil, errors.New("plugin store is unavailable")
	}
	if err := manager.Store.Builtins.Validate(); err != nil {
		return nil, err
	}
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return nil, err
	}
	items := make([]CatalogPlugin, 0, len(lock.Plugins)+len(manager.Store.Builtins))
	seen := map[PluginID]struct{}{}
	for _, builtin := range manager.Store.Builtins {
		items = append(items, builtinCatalogPlugin(builtin, manager.Store.runtime.CoreVersion))
		seen[builtin.ID] = struct{}{}
	}
	ids := make([]PluginID, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		entry := lock.Plugins[id]
		installed, err := manager.Store.Installed(id, entry.Version)
		if err != nil {
			return nil, fmt.Errorf("load installed plugin %s@%s: %w", id, entry.Version, err)
		}
		items = append(items, CatalogPlugin{
			ID: id, Origin: OriginInstalled, Lifecycle: ArtifactLifecycle(), Enabled: entry.Enabled,
			Version: entry.Version, Registry: entry.Registry, Publisher: entry.Publisher, Installed: installed,
			Scopes: installed.Manifest.AllowedScopes(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (manager Manager) CatalogPlugin(id PluginID) (CatalogPlugin, error) {
	items, err := manager.Catalog()
	if err != nil {
		return CatalogPlugin{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return CatalogPlugin{}, fmt.Errorf("plugin %s is not installed", id)
}

func builtinCatalogPlugin(builtin Builtin, coreVersion string) CatalogPlugin {
	manifest := builtin.CatalogManifest(coreVersion)
	return CatalogPlugin{
		ID: builtin.ID, Origin: OriginBuiltin, Lifecycle: builtin.Lifecycle(), Enabled: builtin.DefaultEnabled,
		Version: manifest.Version, Registry: BuiltinRegistryName, Publisher: BuiltinPublisher,
		Installed: InstalledPlugin{Manifest: manifest}, Schema: builtin.Schema, Scopes: builtin.AllowedScopes(),
	}
}
