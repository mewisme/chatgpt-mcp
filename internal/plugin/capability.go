package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrCapabilityNotFound = errors.New("plugin capability provider not found")

type CapabilityProvider struct {
	PluginID    PluginID
	Version     Version
	Name        string
	Path        string
	Permissions []Permission
}

type CapabilityConflictError struct {
	Capability Capability
	Providers  []CapabilityProvider
}

func (err CapabilityConflictError) Error() string {
	providers := make([]string, 0, len(err.Providers))
	for _, provider := range err.Providers {
		providers = append(providers, fmt.Sprintf("%s@%s", provider.PluginID, provider.Version))
	}
	return fmt.Sprintf("plugin capability %s has multiple enabled providers: %s", err.Capability, strings.Join(providers, ", "))
}

type Resolver struct {
	providers map[Capability][]CapabilityProvider
}

func NewResolver(store *Store) (*Resolver, error) {
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	providers := map[Capability][]CapabilityProvider{}
	for _, idValue := range ids {
		id := PluginID(idValue)
		entry := lock.Plugins[id]
		if !entry.Enabled {
			continue
		}
		installed, err := store.Installed(id, entry.Version)
		if err != nil {
			return nil, fmt.Errorf("load active plugin %s@%s: %w", id, entry.Version, err)
		}
		digest, err := ManifestDigest(installed.Manifest)
		if err != nil {
			return nil, err
		}
		if digest != entry.ManifestDigest || installed.Manifest.Publisher != entry.Publisher {
			return nil, fmt.Errorf("active plugin %s@%s does not match lock integrity metadata", id, entry.Version)
		}
		artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
		if err != nil {
			return nil, err
		}
		if entry.ArtifactDigest != "sha256:"+artifact.SHA256 {
			return nil, fmt.Errorf("active plugin %s@%s artifact digest does not match lock metadata", id, entry.Version)
		}
		compatible, err := pluginCoreCompatible(store, installed.Manifest)
		if err != nil {
			return nil, fmt.Errorf("check active plugin %s@%s compatibility: %w", id, entry.Version, err)
		}
		if !compatible {
			continue
		}
		provider := CapabilityProvider{PluginID: id, Version: entry.Version, Name: installed.Manifest.Name, Path: installed.Entrypoint, Permissions: append([]Permission(nil), installed.Manifest.Permissions...)}
		for _, capability := range installed.Manifest.Provides {
			providers[capability] = append(providers[capability], provider)
		}
	}
	for capability := range providers {
		sort.Slice(providers[capability], func(i, j int) bool {
			left, right := providers[capability][i], providers[capability][j]
			if left.PluginID == right.PluginID {
				return left.Version < right.Version
			}
			return left.PluginID < right.PluginID
		})
	}
	return &Resolver{providers: providers}, nil
}

func ValidateDependencies(store *Store, manifest Manifest) error {
	if len(manifest.Dependencies.Capabilities) == 0 {
		return nil
	}
	resolver, err := NewResolver(store)
	if err != nil {
		return err
	}
	for _, capability := range manifest.Dependencies.Capabilities {
		if _, err := resolver.Resolve(capability); err != nil {
			return fmt.Errorf("plugin %s@%s requires capability %s: %w", manifest.ID, manifest.Version, capability, err)
		}
	}
	return nil
}

func pluginCoreCompatible(store *Store, manifest Manifest) (bool, error) {
	coreVersion := strings.TrimSpace(store.runtime.CoreVersion)
	if coreVersion == "" || coreVersion == "dev" {
		return true, nil
	}
	return manifest.CompatibleWithCore(coreVersion)
}

func (resolver *Resolver) Capabilities(prefix string) []Capability {
	prefix = strings.TrimSpace(prefix)
	capabilities := make([]Capability, 0, len(resolver.providers))
	for capability := range resolver.providers {
		if prefix == "" || strings.HasPrefix(string(capability), prefix) {
			capabilities = append(capabilities, capability)
		}
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i] < capabilities[j] })
	return capabilities
}

func (resolver *Resolver) Providers(capability Capability) []CapabilityProvider {
	providers := resolver.providers[capability]
	return cloneCapabilityProviders(providers)
}

func (resolver *Resolver) Resolve(capability Capability) (CapabilityProvider, error) {
	providers := resolver.providers[capability]
	if len(providers) == 0 {
		return CapabilityProvider{}, fmt.Errorf("%w: %s", ErrCapabilityNotFound, capability)
	}
	if len(providers) > 1 {
		return CapabilityProvider{}, CapabilityConflictError{Capability: capability, Providers: cloneCapabilityProviders(providers)}
	}
	provider := providers[0]
	provider.Permissions = append([]Permission(nil), provider.Permissions...)
	return provider, nil
}

func cloneCapabilityProviders(providers []CapabilityProvider) []CapabilityProvider {
	cloned := make([]CapabilityProvider, len(providers))
	for index, provider := range providers {
		provider.Permissions = append([]Permission(nil), provider.Permissions...)
		cloned[index] = provider
	}
	return cloned
}
