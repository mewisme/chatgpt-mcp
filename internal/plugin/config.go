package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

const ConfigSchema = 1

type Config struct {
	Schema     int                 `json:"schema"`
	Registries map[string]Registry `json:"registries"`
}

func NewConfig() Config { return Config{Schema: ConfigSchema, Registries: map[string]Registry{}} }

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewConfig(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := decodeStrictJSON(data, &config); err != nil {
		return Config{}, fmt.Errorf("decode plugin config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func WriteConfig(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(path, append(data, '\n'), 0600)
}

func (config Config) Validate() error {
	if config.Schema != ConfigSchema {
		return fmt.Errorf("unsupported plugin config schema: %d", config.Schema)
	}
	if config.Registries == nil {
		return errors.New("plugin registries map is required")
	}
	for name, registry := range config.Registries {
		if name == OfficialRegistryName {
			return errors.New("official plugin registry is built in and cannot be overridden")
		}
		if registry.Name != name {
			return fmt.Errorf("plugin registry key %q does not match descriptor name %q", name, registry.Name)
		}
		if err := validateRegistryDescriptor(registry); err != nil {
			return err
		}
	}
	return nil
}

func (config Config) AllRegistries() []Registry {
	registries := []Registry{OfficialRegistry()}
	names := make([]string, 0, len(config.Registries))
	for name := range config.Registries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		registries = append(registries, config.Registries[name])
	}
	return registries
}

func (config *Config) AddRegistry(name, rawURL string, unqualified bool, trust SigstoreIdentity) error {
	name = strings.TrimSpace(name)
	registry := Registry{Name: name, URL: strings.TrimRight(strings.TrimSpace(rawURL), "/"), UnqualifiedResolution: unqualified, Trust: &trust}
	if name == OfficialRegistryName {
		return errors.New("official plugin registry is built in and cannot be overridden")
	}
	if err := validateRegistryDescriptor(registry); err != nil {
		return err
	}
	if config.Registries == nil {
		config.Registries = map[string]Registry{}
	}
	if _, exists := config.Registries[name]; exists {
		return fmt.Errorf("plugin registry %s already exists", name)
	}
	config.Registries[name] = registry
	return nil
}

func (config *Config) RemoveRegistry(name string) error {
	name = strings.TrimSpace(name)
	if name == OfficialRegistryName {
		return errors.New("official plugin registry cannot be removed")
	}
	if _, ok := config.Registries[name]; !ok {
		return fmt.Errorf("plugin registry %s is not configured", name)
	}
	delete(config.Registries, name)
	return nil
}
