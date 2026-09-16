package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type RegistryHealth struct {
	Registry  Registry
	Status    string
	FetchedAt time.Time
	Error     string
}

type CoreCompatibilityIssue struct {
	ID          PluginID
	Version     Version
	Requirement string
	Error       string
}

func (manager Manager) RegistryHealth(ctx context.Context) ([]RegistryHealth, error) {
	if manager.Store == nil {
		return nil, errors.New("plugin store is unavailable")
	}
	config, err := manager.Store.layout.LoadConfig()
	if err != nil {
		return nil, err
	}
	registries := config.AllRegistries()
	result := make([]RegistryHealth, 0, len(registries))
	for _, registry := range registries {
		snapshot, refreshErr := manager.RegistryClient.Refresh(ctx, registry)
		if refreshErr == nil {
			result = append(result, RegistryHealth{Registry: registry, Status: "verified", FetchedAt: snapshot.FetchedAt})
			continue
		}
		cached, cacheErr := manager.RegistryClient.LoadCachedVerified(ctx, registry, DefaultRegistryCacheTTL)
		if cacheErr == nil {
			result = append(result, RegistryHealth{Registry: registry, Status: "verified-cache", FetchedAt: cached.FetchedAt, Error: refreshErr.Error()})
			continue
		}
		result = append(result, RegistryHealth{Registry: registry, Status: "unavailable", Error: errors.Join(refreshErr, cacheErr).Error()})
	}
	return result, nil
}

func (manager Manager) AssessCoreCompatibility(target string) ([]CoreCompatibilityIssue, error) {
	if manager.Store == nil {
		return nil, errors.New("plugin store is unavailable")
	}
	target = strings.TrimSpace(target)
	if target == "" || target == "dev" {
		return nil, nil
	}
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return nil, err
	}
	issues := []CoreCompatibilityIssue{}
	ids := make([]PluginID, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		entry := lock.Plugins[id]
		if !entry.Enabled {
			continue
		}
		installed, err := manager.Store.Installed(id, entry.Version)
		if err != nil {
			issues = append(issues, CoreCompatibilityIssue{ID: id, Version: entry.Version, Error: err.Error()})
			continue
		}
		compatible, err := installed.Manifest.CompatibleWithCore(target)
		if err != nil {
			issues = append(issues, CoreCompatibilityIssue{ID: id, Version: entry.Version, Requirement: installed.Manifest.Requires.ChatGPTMCP, Error: err.Error()})
			continue
		}
		if !compatible {
			issues = append(issues, CoreCompatibilityIssue{ID: id, Version: entry.Version, Requirement: installed.Manifest.Requires.ChatGPTMCP, Error: fmt.Sprintf("incompatible with chatgpt-mcp %s", target)})
		}
	}
	return issues, nil
}

func CapabilityConflicts(store *Store) ([]CapabilityConflictError, error) {
	if store == nil {
		return nil, errors.New("plugin store is unavailable")
	}
	resolver, err := NewResolver(store)
	if err != nil {
		return nil, err
	}
	conflicts := []CapabilityConflictError{}
	for _, capability := range resolver.Capabilities("") {
		providers := resolver.Providers(capability)
		if len(providers) > 1 {
			conflicts = append(conflicts, CapabilityConflictError{Capability: capability, Providers: providers})
		}
	}
	return conflicts, nil
}
