package application

import (
	"context"
	"fmt"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func (service *PluginService) PluginSettings(id pluginpkg.PluginID) (pluginpkg.SettingsSchema, map[string]any, error) {
	schema, store, err := service.pluginSettings(id)
	if err != nil {
		return pluginpkg.SettingsSchema{}, nil, err
	}
	values, err := store.Public(schema, id)
	if err != nil {
		return pluginpkg.SettingsSchema{}, nil, err
	}
	return schema, values, nil
}

func (service *PluginService) PluginSetting(id pluginpkg.PluginID, key string) (pluginpkg.SettingField, any, error) {
	schema, values, err := service.PluginSettings(id)
	if err != nil {
		return pluginpkg.SettingField{}, nil, err
	}
	field, ok := schema.Field(key)
	if !ok {
		return pluginpkg.SettingField{}, nil, fmt.Errorf("%w: %s", pluginpkg.ErrUnknownConfigKey, key)
	}
	return field, values[key], nil
}

func (service *PluginService) SetPluginSetting(ctx context.Context, id pluginpkg.PluginID, key, raw string) error {
	schema, store, err := service.pluginSettings(id)
	if err != nil {
		return err
	}
	field, ok := schema.Field(key)
	if !ok {
		return fmt.Errorf("%w: %s", pluginpkg.ErrUnknownConfigKey, key)
	}
	value, err := pluginpkg.ParseSettingValue(field, raw)
	if err != nil {
		return err
	}
	if err := store.Set(schema, id, key, value); err != nil {
		return err
	}
	_, _, err = reloadPersistedConfigIfRunning(ctx)
	return err
}

func (service *PluginService) ResetPluginSetting(ctx context.Context, id pluginpkg.PluginID, key string) error {
	schema, store, err := service.pluginSettings(id)
	if err != nil {
		return err
	}
	if err := store.Reset(schema, id, key); err != nil {
		return err
	}
	_, _, err = reloadPersistedConfigIfRunning(ctx)
	return err
}

func (service *PluginService) pluginSettings(id pluginpkg.PluginID) (pluginpkg.SettingsSchema, pluginpkg.SettingsStore, error) {
	if service == nil || service.Manager == nil {
		return pluginpkg.SettingsSchema{}, pluginpkg.SettingsStore{}, fmt.Errorf("plugin service is unavailable")
	}
	schema, err := service.Manager.SettingsSchema(id)
	if err != nil {
		return pluginpkg.SettingsSchema{}, pluginpkg.SettingsStore{}, err
	}
	return schema, pluginpkg.SettingsStore{Layout: service.Layout}, nil
}
