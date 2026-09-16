package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const PluginSettingsSchema = 1

type FieldKind string

const (
	FieldString FieldKind = "string"
	FieldBool   FieldKind = "boolean"
	FieldInt    FieldKind = "integer"
	FieldNumber FieldKind = "number"
	FieldEnum   FieldKind = "enum"
)

type SettingField struct {
	Key         string    `json:"key"`
	Kind        FieldKind `json:"type"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Default     any       `json:"default,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Enum        []string  `json:"enum,omitempty"`
	Sensitive   bool      `json:"sensitive,omitempty"`
}

type SettingsSchema struct {
	Fields []SettingField `json:"fields"`
}

type pluginConfigDocument struct {
	Schema int            `json:"schema"`
	Values map[string]any `json:"values"`
}

type SettingsApplyFunc func(id PluginID, effective map[string]any) error

type SettingsStore struct {
	Layout  Layout
	Secrets *secretstore.Store
	Apply   SettingsApplyFunc
}

var (
	ErrUnknownConfigKey   = errors.New("unknown plugin config key")
	ErrInvalidConfigValue = errors.New("invalid plugin config value")
)

func (schema SettingsSchema) Validate() error {
	seen := map[string]struct{}{}
	for _, field := range schema.Fields {
		if !validCanonicalName(field.Key) {
			return fmt.Errorf("invalid plugin config key: %q", field.Key)
		}
		if _, ok := seen[field.Key]; ok {
			return fmt.Errorf("duplicate plugin config key: %s", field.Key)
		}
		seen[field.Key] = struct{}{}
		switch field.Kind {
		case FieldString, FieldBool, FieldInt, FieldNumber:
		case FieldEnum:
			if len(field.Enum) == 0 {
				return fmt.Errorf("plugin config %s enum is empty", field.Key)
			}
		default:
			return fmt.Errorf("unsupported plugin config type %q for %s", field.Kind, field.Key)
		}
		if field.Sensitive && field.Kind != FieldString {
			return fmt.Errorf("plugin config %s sensitive fields must be strings", field.Key)
		}
		if field.Default != nil {
			if _, err := normalizeConfigValue(field, field.Default); err != nil {
				return fmt.Errorf("plugin config %s default: %w", field.Key, err)
			}
		} else if field.Required && !field.Sensitive {
			return fmt.Errorf("plugin config %s is required but has no default", field.Key)
		}
	}
	return nil
}

func (schema SettingsSchema) Field(key string) (SettingField, bool) {
	for _, field := range schema.Fields {
		if field.Key == key {
			return field, true
		}
	}
	return SettingField{}, false
}

func (schema SettingsSchema) Defaults() map[string]any {
	values := map[string]any{}
	for _, field := range schema.Fields {
		if field.Default == nil {
			continue
		}
		value, err := normalizeConfigValue(field, field.Default)
		if err != nil {
			continue
		}
		values[field.Key] = value
	}
	return values
}

func (schema SettingsSchema) Effective(overrides map[string]any) (map[string]any, error) {
	if err := schema.Validate(); err != nil {
		return nil, err
	}
	values := schema.Defaults()
	for key, raw := range overrides {
		field, ok := schema.Field(key)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownConfigKey, key)
		}
		value, err := normalizeConfigValue(field, raw)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	for _, field := range schema.Fields {
		if _, ok := values[field.Key]; !ok && field.Required && !field.Sensitive {
			return nil, fmt.Errorf("plugin config %s is required", field.Key)
		}
	}
	return values, nil
}

func (store SettingsStore) Get(schema SettingsSchema, id PluginID) (map[string]any, error) {
	overrides, err := store.loadOverrides(id)
	if err != nil {
		return nil, err
	}
	if err := store.resolveSecrets(schema, id, overrides); err != nil {
		return nil, err
	}
	return schema.Effective(overrides)
}

func (store SettingsStore) Public(schema SettingsSchema, id PluginID) (map[string]any, error) {
	overrides, err := store.loadOverrides(id)
	if err != nil {
		return nil, err
	}
	values, err := schema.Effective(publicOverrides(schema, overrides))
	if err != nil {
		return nil, err
	}
	for _, field := range schema.Fields {
		if !field.Sensitive {
			continue
		}
		raw, ok := overrides[field.Key]
		values[field.Key] = secretstore.IsMarker(fmt.Sprint(raw)) || ok && strings.TrimSpace(fmt.Sprint(raw)) != ""
	}
	return values, nil
}

func (store SettingsStore) Set(schema SettingsSchema, id PluginID, key string, value any) error {
	field, ok := schema.Field(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownConfigKey, key)
	}
	normalized, err := normalizeConfigValue(field, value)
	if err != nil {
		return err
	}
	return store.mutate(schema, id, func(overrides map[string]any) error {
		overrides[key] = normalized
		return nil
	})
}

func (store SettingsStore) Reset(schema SettingsSchema, id PluginID, key string) error {
	key = strings.TrimSpace(key)
	if key != "" {
		if _, ok := schema.Field(key); !ok {
			return fmt.Errorf("%w: %s", ErrUnknownConfigKey, key)
		}
	}
	return store.mutate(schema, id, func(overrides map[string]any) error {
		if key == "" {
			for name := range overrides {
				delete(overrides, name)
			}
			return nil
		}
		delete(overrides, key)
		return nil
	})
}

func (store SettingsStore) mutate(schema SettingsSchema, id PluginID, mutate func(map[string]any) error) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if !validCanonicalName(string(id)) {
		return fmt.Errorf("invalid plugin id: %q", id)
	}
	if err := store.Layout.Validate(); err != nil {
		return err
	}
	path := store.Layout.PluginConfigPath(id)
	previous, existed, err := readOptionalFile(path)
	if err != nil {
		return err
	}
	overrides, err := decodePluginConfig(previous)
	if err != nil {
		return err
	}
	currentSecrets, err := store.secretSnapshot(schema, id)
	if err != nil {
		return err
	}
	if err := mutate(overrides); err != nil {
		return err
	}
	if _, err := schema.Effective(publicOverrides(schema, overrides)); err != nil {
		return err
	}
	secretChanges, persistValues, err := store.preparePersist(schema, id, overrides)
	if err != nil {
		return err
	}
	if err := store.secrets().Apply(secretChanges); err != nil {
		return err
	}
	if err := writePluginConfig(path, persistValues); err != nil {
		_ = store.secrets().Apply(currentSecrets)
		return err
	}
	effective, err := store.Get(schema, id)
	if err != nil {
		_ = restoreOptionalFile(path, previous, existed)
		_ = store.secrets().Apply(currentSecrets)
		return err
	}
	if store.Apply == nil {
		return nil
	}
	if err := store.Apply(id, effective); err != nil {
		_ = restoreOptionalFile(path, previous, existed)
		_ = store.secrets().Apply(currentSecrets)
		return err
	}
	return nil
}

func (store SettingsStore) loadOverrides(id PluginID) (map[string]any, error) {
	if !validCanonicalName(string(id)) {
		return nil, fmt.Errorf("invalid plugin id: %q", id)
	}
	data, _, err := readOptionalFile(store.Layout.PluginConfigPath(id))
	if err != nil {
		return nil, err
	}
	return decodePluginConfig(data)
}

func (store SettingsStore) resolveSecrets(schema SettingsSchema, id PluginID, overrides map[string]any) error {
	for _, field := range schema.Fields {
		if !field.Sensitive {
			continue
		}
		raw, ok := overrides[field.Key]
		if !ok || !secretstore.IsMarker(fmt.Sprint(raw)) {
			continue
		}
		value, err := store.secrets().Get(pluginSecretName(id, field.Key))
		if errors.Is(err, secretstore.ErrNotFound) {
			delete(overrides, field.Key)
			continue
		}
		if err != nil {
			return err
		}
		overrides[field.Key] = value
	}
	return nil
}

func (store SettingsStore) preparePersist(schema SettingsSchema, id PluginID, overrides map[string]any) ([]secretstore.Change, map[string]any, error) {
	persist := map[string]any{}
	changes := []secretstore.Change{}
	seen := map[string]struct{}{}
	for key, raw := range overrides {
		field, ok := schema.Field(key)
		if !ok {
			return nil, nil, fmt.Errorf("%w: %s", ErrUnknownConfigKey, key)
		}
		if field.Sensitive {
			text, _ := raw.(string)
			persist[key] = secretstore.Marker
			changes = append(changes, secretstore.Change{Name: pluginSecretName(id, key), Value: text})
			seen[key] = struct{}{}
			continue
		}
		persist[key] = raw
	}
	for _, field := range schema.Fields {
		if !field.Sensitive {
			continue
		}
		if _, ok := seen[field.Key]; ok {
			continue
		}
		changes = append(changes, secretstore.Change{Name: pluginSecretName(id, field.Key)})
	}
	return changes, persist, nil
}

func (store SettingsStore) secretSnapshot(schema SettingsSchema, id PluginID) ([]secretstore.Change, error) {
	changes := []secretstore.Change{}
	for _, field := range schema.Fields {
		if !field.Sensitive {
			continue
		}
		name := pluginSecretName(id, field.Key)
		value, err := store.secrets().Get(name)
		if errors.Is(err, secretstore.ErrNotFound) {
			changes = append(changes, secretstore.Change{Name: name})
			continue
		}
		if err != nil {
			return nil, err
		}
		changes = append(changes, secretstore.Change{Name: name, Value: value})
	}
	return changes, nil
}

func (store SettingsStore) secrets() *secretstore.Store {
	if store.Secrets != nil {
		return store.Secrets
	}
	return secretstore.New(store.Layout.ConfigRoot)
}

func pluginSecretName(id PluginID, key string) string {
	return secretstore.Name("plugin", string(id), key)
}

func publicOverrides(schema SettingsSchema, overrides map[string]any) map[string]any {
	public := map[string]any{}
	for key, value := range overrides {
		field, ok := schema.Field(key)
		if !ok || field.Sensitive {
			continue
		}
		public[key] = value
	}
	return public
}

func normalizeConfigValue(field SettingField, value any) (any, error) {
	switch field.Kind {
	case FieldBool:
		switch typed := value.(type) {
		case bool:
			return typed, nil
		default:
			return nil, fmt.Errorf("%w: %s must be boolean", ErrInvalidConfigValue, field.Key)
		}
	case FieldString:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s must be a string", ErrInvalidConfigValue, field.Key)
		}
		return text, nil
	case FieldEnum:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s must be a string", ErrInvalidConfigValue, field.Key)
		}
		for _, option := range field.Enum {
			if option == text {
				return text, nil
			}
		}
		return nil, fmt.Errorf("%w: %s is not a valid option", ErrInvalidConfigValue, field.Key)
	case FieldInt:
		number, err := coerceNumber(value)
		if err != nil || math.Trunc(number) != number {
			return nil, fmt.Errorf("%w: %s must be an integer", ErrInvalidConfigValue, field.Key)
		}
		return int64(number), nil
	case FieldNumber:
		number, err := coerceNumber(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %s must be a number", ErrInvalidConfigValue, field.Key)
		}
		return number, nil
	default:
		return nil, fmt.Errorf("%w: %s has unsupported type", ErrInvalidConfigValue, field.Key)
	}
}

func coerceNumber(value any) (float64, error) {
	switch typed := value.(type) {
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case float64:
		return typed, nil
	case json.Number:
		return typed.Float64()
	default:
		return 0, ErrInvalidConfigValue
	}
}

func decodePluginConfig(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var document pluginConfigDocument
	if err := decodeStrictJSON(data, &document); err != nil {
		return nil, fmt.Errorf("decode plugin config: %w", err)
	}
	if document.Schema != PluginSettingsSchema {
		return nil, fmt.Errorf("unsupported plugin config schema: %d", document.Schema)
	}
	if document.Values == nil {
		return map[string]any{}, nil
	}
	return document.Values, nil
}

func writePluginConfig(path string, values map[string]any) error {
	if len(values) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(pluginConfigDocument{Schema: PluginSettingsSchema, Values: values}, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, append(data, '\n'), 0600)
}

func readOptionalFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func restoreOptionalFile(path string, previous []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return state.WriteFileAtomic(path, previous, 0600)
}
