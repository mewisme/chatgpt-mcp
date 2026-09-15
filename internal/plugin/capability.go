package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrCapabilityNotFound = errors.New("plugin capability provider not found")

type CapabilityProvider struct {
	PluginID PluginID
	Version  Version
	Name     string
	Path     string
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
		provider := CapabilityProvider{PluginID: id, Version: entry.Version, Name: installed.Manifest.Name, Path: installed.Entrypoint}
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

func (resolver *Resolver) Providers(capability Capability) []CapabilityProvider {
	providers := resolver.providers[capability]
	return append([]CapabilityProvider(nil), providers...)
}

func (resolver *Resolver) Resolve(capability Capability) (CapabilityProvider, error) {
	providers := resolver.providers[capability]
	if len(providers) == 0 {
		return CapabilityProvider{}, fmt.Errorf("%w: %s", ErrCapabilityNotFound, capability)
	}
	if len(providers) > 1 {
		return CapabilityProvider{}, CapabilityConflictError{Capability: capability, Providers: append([]CapabilityProvider(nil), providers...)}
	}
	return providers[0], nil
}
