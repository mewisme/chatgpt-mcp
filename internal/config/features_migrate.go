package config

import (
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	cavemanplugin "go.mewis.me/chatgpt-mcp/plugins/caveman"
	ponytailplugin "go.mewis.me/chatgpt-mcp/plugins/ponytail"
)

func migrateLegacyFeatureSettings(configPath string, data []byte, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	store := settingsStoreForConfigPath(configPath)
	for _, item := range builtinFeatureSettings(*cfg) {
		if err := store.ImportMissing(item.Schema, item.ID, item.Values); err != nil {
			return err
		}
	}
	if err := hydrateFeatureSettings(configPath, cfg); err != nil {
		return err
	}
	if !configHasFeatures(configPath, data) {
		return nil
	}
	_, err := pruneDeprecatedConfigKeys(configPath, [][]string{{"features"}})
	return err
}

func persistFeatureSettings(configPath string, cfg Config) error {
	store := settingsStoreForConfigPath(configPath)
	for _, item := range builtinFeatureSettings(cfg) {
		if err := store.SyncOverrides(item.Schema, item.ID, item.Values); err != nil {
			return err
		}
	}
	return nil
}

func hydrateFeatureSettings(configPath string, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	store := settingsStoreForConfigPath(configPath)
	for _, item := range builtinFeatureSettings(*cfg) {
		values, err := store.Get(item.Schema, item.ID)
		if err != nil {
			return err
		}
		applyFeatureSettings(cfg, item.ID, values)
	}
	return nil
}

func settingsStoreForConfigPath(configPath string) pluginpkg.SettingsStore {
	root := filepath.Clean(filepath.Dir(configPath))
	return pluginpkg.SettingsStore{Layout: pluginpkg.Layout{ConfigRoot: root, DataRoot: root + "-data", CacheRoot: root + "-cache"}}
}

func configHasFeatures(configPath string, data []byte) bool {
	format, err := configformat.Detect(configPath)
	if err != nil {
		return false
	}
	decoded, err := configformat.DecodeGeneric(format, data)
	if err != nil {
		return false
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return false
	}
	_, exists := root["features"]
	return exists
}

type featurePluginSetting struct {
	ID     pluginpkg.PluginID
	Schema pluginpkg.SettingsSchema
	Values map[string]any
}

func builtinFeatureSettings(cfg Config) []featurePluginSetting {
	return []featurePluginSetting{
		{ponytailplugin.Plugin().ID, ponytailplugin.Plugin().Schema, map[string]any{"default_active": cfg.Features.Ponytail.Active, "default_mode": cfg.Features.Ponytail.Mode}},
		{cavemanplugin.Plugin().ID, cavemanplugin.Plugin().Schema, map[string]any{"default_active": cfg.Features.Caveman.Active, "default_mode": cfg.Features.Caveman.Mode}},
	}
}

func applyFeatureSettings(cfg *Config, id pluginpkg.PluginID, values map[string]any) {
	active := boolSetting(values, "default_active", true)
	mode := stringSetting(values, "default_mode", "full")
	switch id {
	case "ponytail":
		cfg.Features.Ponytail.Active = active
		cfg.Features.Ponytail.Mode = mode
	case "caveman":
		cfg.Features.Caveman.Active = active
		cfg.Features.Caveman.Mode = mode
	}
}

func boolSetting(values map[string]any, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	value, ok := raw.(bool)
	if !ok {
		return fallback
	}
	return value
}

func stringSetting(values map[string]any, key, fallback string) string {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}
