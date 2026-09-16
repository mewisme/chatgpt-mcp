package config

import (
	"encoding/json"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func migrateLegacyFeatureSettings(configPath string, data []byte, _ *Config) error {
	if !configHasFeatures(configPath, data) {
		return nil
	}
	source := defaultLegacyFeatures()
	var file struct {
		Features legacyFeatures `json:"features"`
	}
	file.Features = source
	if err := configformat.UnmarshalPath(configPath, data, &file); err != nil {
		return err
	}
	store := settingsStoreForConfigPath(configPath)
	for _, item := range builtinFeatureSettings(file.Features) {
		if err := store.ImportMissing(item.Schema, item.ID, item.Values); err != nil {
			return err
		}
	}
	_, err := pruneDeprecatedConfigKeys(configPath, [][]string{{"features"}})
	return err
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

type legacyFeatures struct {
	Ponytail legacyFeature `json:"ponytail"`
	Caveman  legacyFeature `json:"caveman"`
}

type legacyFeature struct {
	Active bool
	Mode   string
}

func defaultLegacyFeatures() legacyFeatures {
	return legacyFeatures{Ponytail: legacyFeature{Active: true, Mode: "full"}, Caveman: legacyFeature{Active: true, Mode: "full"}}
}

func (f *legacyFeature) UnmarshalJSON(data []byte) error {
	var value struct {
		Active  *bool   `json:"active"`
		Enabled *bool   `json:"enabled"`
		Mode    *string `json:"mode"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Active != nil {
		f.Active = *value.Active
	} else if value.Enabled != nil {
		f.Active = *value.Enabled
	}
	if value.Mode != nil {
		f.Mode = *value.Mode
	}
	return nil
}

type featurePluginSetting struct {
	ID     pluginpkg.PluginID
	Schema pluginpkg.SettingsSchema
	Values map[string]any
}

func builtinFeatureSettings(src legacyFeatures) []featurePluginSetting {
	return []featurePluginSetting{
		{"ponytail", ponytailSettingsSchema(), map[string]any{"default_active": src.Ponytail.Active, "default_mode": src.Ponytail.Mode}},
		{"caveman", cavemanSettingsSchema(), map[string]any{"default_active": src.Caveman.Active, "default_mode": src.Caveman.Mode}},
	}
}

func ponytailSettingsSchema() pluginpkg.SettingsSchema {
	return pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
		{Key: "default_active", Kind: pluginpkg.FieldBool, Title: "Default active", Description: "controls whether Ponytail guidance is active by default", Default: true},
		{Key: "default_mode", Kind: pluginpkg.FieldEnum, Title: "Default mode", Description: "sets the default Ponytail intensity", Enum: []string{"lite", "full", "ultra"}, Default: "full"},
	}}
}

func cavemanSettingsSchema() pluginpkg.SettingsSchema {
	return pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
		{Key: "default_active", Kind: pluginpkg.FieldBool, Title: "Default active", Description: "controls whether Caveman response style is active by default", Default: true},
		{Key: "default_mode", Kind: pluginpkg.FieldEnum, Title: "Default mode", Description: "sets the default Caveman response intensity", Enum: []string{"lite", "full", "ultra", "wenyan-lite", "wenyan-full", "wenyan-ultra"}, Default: "full"},
	}}
}
