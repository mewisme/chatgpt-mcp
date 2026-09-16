package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	RegistrySchema          = 1
	PublishersSchema        = 1
	OfficialRegistryName    = "official"
	OfficialRegistryBaseURL = "https://github.com/mewisme/chatgpt-mcp/releases/download/plugins"
	OfficialSigstoreIssuer  = "https://token.actions.githubusercontent.com"
	OfficialSigstoreRepo    = "mewisme/chatgpt-mcp"
)

type RegistryIndex struct {
	Schema      int                        `json:"schema"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Plugins     map[PluginID]RegistryEntry `json:"plugins"`
}

type RegistryEntry struct {
	Publisher   string             `json:"publisher"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Type        PluginType         `json:"type"`
	Scopes      []PluginScope      `json:"scopes,omitempty"`
	Stable      Version            `json:"stable,omitempty"`
	Beta        Version            `json:"beta,omitempty"`
	Versions    map[Version]string `json:"versions"`
}

type PublisherIndex struct {
	Schema     int                  `json:"schema"`
	Publishers map[string]Publisher `json:"publishers"`
}

type RegistrySnapshot struct {
	Registry   Registry
	Index      RegistryIndex
	Publishers PublisherIndex
	FetchedAt  time.Time
}

type ResolvedPlugin struct {
	Registry     Registry
	PluginID     PluginID
	Version      Version
	Entry        RegistryEntry
	Publisher    Publisher
	ManifestName string
}

func OfficialRegistry() Registry {
	identity := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}
	return Registry{Name: OfficialRegistryName, URL: OfficialRegistryBaseURL, UnqualifiedResolution: true, Trust: &identity}
}

func ParseRegistryIndex(data []byte) (RegistryIndex, error) {
	var index RegistryIndex
	if err := decodeStrictJSON(data, &index); err != nil {
		return RegistryIndex{}, fmt.Errorf("decode plugin registry index: %w", err)
	}
	if err := index.Validate(); err != nil {
		return RegistryIndex{}, err
	}
	return index, nil
}

func ParsePublisherIndex(data []byte) (PublisherIndex, error) {
	var index PublisherIndex
	if err := decodeStrictJSON(data, &index); err != nil {
		return PublisherIndex{}, fmt.Errorf("decode plugin publisher index: %w", err)
	}
	if err := index.Validate(); err != nil {
		return PublisherIndex{}, err
	}
	return index, nil
}

func (index RegistryIndex) Validate() error {
	if index.Schema != RegistrySchema {
		return fmt.Errorf("unsupported plugin registry schema: %d", index.Schema)
	}
	if index.GeneratedAt.IsZero() {
		return errors.New("plugin registry generated_at is required")
	}
	if index.Plugins == nil {
		return errors.New("plugin registry plugins map is required")
	}
	ids := make([]string, 0, len(index.Plugins))
	for id := range index.Plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, idValue := range ids {
		if !validCanonicalName(idValue) {
			return fmt.Errorf("invalid registry plugin id: %q", idValue)
		}
		entry := index.Plugins[PluginID(idValue)]
		if !validCanonicalName(entry.Publisher) {
			return fmt.Errorf("invalid publisher for registry plugin %s: %q", idValue, entry.Publisher)
		}
		if strings.TrimSpace(entry.Name) == "" {
			return fmt.Errorf("registry plugin %s name is required", idValue)
		}
		if _, ok := knownPluginTypes[entry.Type]; !ok {
			return fmt.Errorf("registry plugin %s has unknown type %q", idValue, entry.Type)
		}
		if len(entry.Scopes) > 0 {
			if err := validatePluginScopes(entry.Scopes); err != nil {
				return fmt.Errorf("registry plugin %s: %w", idValue, err)
			}
		}
		if len(entry.Versions) == 0 {
			return fmt.Errorf("registry plugin %s has no versions", idValue)
		}
		for version, manifestName := range entry.Versions {
			if err := validateVersion(string(version)); err != nil {
				return fmt.Errorf("registry plugin %s has invalid version %q", idValue, version)
			}
			if !safeAssetName(manifestName) || !strings.HasSuffix(manifestName, ".json") {
				return fmt.Errorf("registry plugin %s version %s has unsafe manifest asset %q", idValue, version, manifestName)
			}
		}
		for channel, version := range map[string]Version{"stable": entry.Stable, "beta": entry.Beta} {
			if version == "" {
				continue
			}
			if _, ok := entry.Versions[version]; !ok {
				return fmt.Errorf("registry plugin %s %s version %s is not listed", idValue, channel, version)
			}
		}
	}
	return nil
}

func (index PublisherIndex) Validate() error {
	if index.Schema != PublishersSchema {
		return fmt.Errorf("unsupported plugin publisher schema: %d", index.Schema)
	}
	if index.Publishers == nil {
		return errors.New("plugin publishers map is required")
	}
	for id, publisher := range index.Publishers {
		if !validCanonicalName(id) || publisher.Name != id {
			return fmt.Errorf("invalid plugin publisher identity: %q", id)
		}
		if strings.TrimSpace(publisher.Source) == "" {
			return fmt.Errorf("plugin publisher %s source is required", id)
		}
		if publisher.Trusted && (strings.TrimSpace(publisher.Sigstore.Issuer) == "" || strings.TrimSpace(publisher.Sigstore.Repository) == "") {
			return fmt.Errorf("trusted plugin publisher %s requires Sigstore identity", id)
		}
	}
	return nil
}

func (snapshot RegistrySnapshot) Validate() error {
	if !validCanonicalName(snapshot.Registry.Name) || strings.TrimSpace(snapshot.Registry.URL) == "" {
		return errors.New("invalid plugin registry descriptor")
	}
	if err := snapshot.Index.Validate(); err != nil {
		return err
	}
	if err := snapshot.Publishers.Validate(); err != nil {
		return err
	}
	for id, entry := range snapshot.Index.Plugins {
		publisher, ok := snapshot.Publishers.Publishers[entry.Publisher]
		if !ok {
			return fmt.Errorf("registry plugin %s references unknown publisher %s", id, entry.Publisher)
		}
		if snapshot.Registry.Name == OfficialRegistryName && !publisher.Trusted {
			return fmt.Errorf("official registry plugin %s references untrusted publisher %s", id, entry.Publisher)
		}
	}
	return nil
}

func (snapshot RegistrySnapshot) Resolve(id PluginID, version Version) (ResolvedPlugin, error) {
	if err := snapshot.Validate(); err != nil {
		return ResolvedPlugin{}, err
	}
	entry, ok := snapshot.Index.Plugins[id]
	if !ok {
		return ResolvedPlugin{}, fmt.Errorf("plugin %s not found in registry %s", id, snapshot.Registry.Name)
	}
	if version == "" {
		version = entry.Stable
	}
	if version == "" {
		return ResolvedPlugin{}, fmt.Errorf("plugin %s has no stable version in registry %s", id, snapshot.Registry.Name)
	}
	manifestName, ok := entry.Versions[version]
	if !ok {
		return ResolvedPlugin{}, fmt.Errorf("plugin %s version %s not found in registry %s", id, version, snapshot.Registry.Name)
	}
	publisher := snapshot.Publishers.Publishers[entry.Publisher]
	return ResolvedPlugin{Registry: snapshot.Registry, PluginID: id, Version: version, Entry: entry, Publisher: publisher, ManifestName: manifestName}, nil
}

func ResolveAcross(snapshots []RegistrySnapshot, registryName string, id PluginID, version Version) (ResolvedPlugin, error) {
	registryName = strings.TrimSpace(registryName)
	if registryName != "" {
		for _, snapshot := range snapshots {
			if snapshot.Registry.Name == registryName {
				return snapshot.Resolve(id, version)
			}
		}
		return ResolvedPlugin{}, fmt.Errorf("plugin registry %s is not configured", registryName)
	}
	var matches []ResolvedPlugin
	for _, snapshot := range snapshots {
		if snapshot.Registry.Name != OfficialRegistryName && !snapshot.Registry.UnqualifiedResolution {
			continue
		}
		resolved, err := snapshot.Resolve(id, version)
		if err == nil {
			matches = append(matches, resolved)
		}
	}
	if len(matches) == 0 {
		return ResolvedPlugin{}, fmt.Errorf("plugin %s was not found in enabled registries", id)
	}
	if len(matches) > 1 {
		registries := make([]string, 0, len(matches))
		for _, match := range matches {
			registries = append(registries, match.Registry.Name)
		}
		sort.Strings(registries)
		return ResolvedPlugin{}, fmt.Errorf("plugin %s is ambiguous across registries: %s", id, strings.Join(registries, ", "))
	}
	return matches[0], nil
}
