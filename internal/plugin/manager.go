package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
)

const maxPluginArtifactSize int64 = 512 << 20
const DefaultRollbackRetention = 2

type Manager struct {
	Store          *Store
	RegistryClient RegistryClient
	HTTPClient     *http.Client
}

type InstallResult struct {
	Plugin    InstalledPlugin
	Registry  Registry
	Publisher Publisher
}

type OutdatedPlugin struct {
	ID      PluginID
	Current Version
	Latest  Version
}

func ParseReference(value string) (registry string, id PluginID, version Version, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", "", errors.New("plugin reference is required")
	}
	name := value
	if before, after, ok := strings.Cut(value, "@"); ok {
		name, version = before, Version(after)
		if err := validateVersion(string(version)); err != nil {
			return "", "", "", fmt.Errorf("invalid plugin version: %q", version)
		}
	}
	if before, after, ok := strings.Cut(name, "/"); ok {
		registry, name = before, after
		if !validCanonicalName(registry) {
			return "", "", "", fmt.Errorf("invalid plugin registry: %q", registry)
		}
	}
	id = PluginID(name)
	if !validCanonicalName(string(id)) {
		return "", "", "", fmt.Errorf("invalid plugin id: %q", id)
	}
	return registry, id, version, nil
}

func (manager Manager) Install(ctx context.Context, reference string) (result InstallResult, err error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.install", "Installing plugin", tracepkg.String("reference", reference))
	defer func() {
		fields := []tracepkg.Field{}
		if result.Plugin.Manifest.ID != "" {
			fields = append(fields, tracepkg.String("plugin_id", string(result.Plugin.Manifest.ID)), tracepkg.String("version", string(result.Plugin.Manifest.Version)), tracepkg.String("registry", result.Registry.Name))
		}
		span.Finish(err, fields...)
	}()
	if manager.Store == nil {
		return InstallResult{}, errors.New("plugin store is unavailable")
	}
	resolved, err := manager.Resolve(ctx, reference)
	if err != nil {
		return InstallResult{}, err
	}
	return manager.installResolved(ctx, resolved, false)
}

func (manager Manager) installResolved(ctx context.Context, resolved ResolvedPlugin, replaceExisting bool) (InstallResult, error) {
	manifest, _, err := manager.RegistryClient.FetchManifest(ctx, resolved)
	if err != nil {
		return InstallResult{}, err
	}
	artifact, err := manifest.Platform(manager.Store.runtime.OS, manager.Store.runtime.Arch)
	if err != nil {
		return InstallResult{}, err
	}
	if manager.Store.runtime.CoreVersion != "" && manager.Store.runtime.CoreVersion != "dev" {
		compatible, err := manifest.CompatibleWithCore(manager.Store.runtime.CoreVersion)
		if err != nil {
			return InstallResult{}, err
		}
		if !compatible {
			return InstallResult{}, fmt.Errorf("plugin %s@%s is incompatible with chatgpt-mcp %s", manifest.ID, manifest.Version, manager.Store.runtime.CoreVersion)
		}
	}
	archivePath, err := manager.downloadArtifact(ctx, resolved.Registry.URL, artifact)
	if err != nil {
		return InstallResult{}, err
	}
	extracted, err := os.MkdirTemp(manager.Store.layout.CacheRoot, ".plugin-extract-")
	if err != nil {
		return InstallResult{}, err
	}
	defer os.RemoveAll(extracted)
	if err := ExtractArchive(archivePath, artifact.Archive, extracted); err != nil {
		return InstallResult{}, err
	}
	if replaceExisting {
		if _, err := manager.Store.Installed(manifest.ID, manifest.Version); err == nil {
			if err := manager.Store.RemoveVersion(manifest.ID, manifest.Version); err != nil {
				return InstallResult{}, err
			}
		}
	}
	installed, err := manager.Store.Install(manifest, extracted)
	if errors.Is(err, ErrVersionInstalled) {
		installed, err = manager.Store.Installed(manifest.ID, manifest.Version)
	}
	if err != nil {
		return InstallResult{}, err
	}
	trust := ActivationTrust{Registry: resolved.Registry.Name, Publisher: resolved.Publisher.Name, Trusted: resolved.Publisher.Trusted}
	if err := manager.Store.Activate(manifest.ID, manifest.Version, trust); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Plugin: installed, Registry: resolved.Registry, Publisher: resolved.Publisher}, nil
}

func (manager Manager) Uninstall(ctx context.Context, id PluginID, force bool) (err error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.uninstall", "Uninstalling plugin", tracepkg.String("plugin_id", string(id)), tracepkg.Bool("force", force))
	defer func() { span.Finish(err) }()
	if manager.Store == nil {
		return errors.New("plugin store is unavailable")
	}
	manager.Store.mu.Lock()
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		manager.Store.mu.Unlock()
		return err
	}
	previous := lock
	entry, ok := lock.Plugins[id]
	if !ok {
		manager.Store.mu.Unlock()
		return fmt.Errorf("plugin %s is not installed", id)
	}
	dependents, err := manager.activeDependents(id, entry.Version, lock)
	if err != nil {
		manager.Store.mu.Unlock()
		return err
	}
	if len(dependents) > 0 && !force {
		manager.Store.mu.Unlock()
		return fmt.Errorf("plugin %s is required by active plugin %s; use --force to uninstall", id, dependents[0])
	}
	for _, dependentID := range dependents {
		dependent := lock.Plugins[dependentID]
		dependent.Enabled = false
		lock.Plugins[dependentID] = dependent
	}
	delete(lock.Plugins, id)
	if err := manager.Store.writeLockAndDesired(previous, lock, func(config *Config) error {
		config.RemoveDesired(id)
		for _, dependentID := range dependents {
			dependent := lock.Plugins[dependentID]
			if err := config.SetDesired(dependentID, dependent.Registry, dependent.Version, false); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		manager.Store.mu.Unlock()
		return err
	}
	manager.Store.mu.Unlock()
	return manager.Store.RemoveVersion(id, entry.Version)
}

func (manager Manager) activeDependents(id PluginID, version Version, lock LockFile) ([]PluginID, error) {
	target, err := manager.Store.Installed(id, version)
	if err != nil {
		return nil, err
	}
	provided := make(map[Capability]struct{}, len(target.Manifest.Provides))
	for _, capability := range target.Manifest.Provides {
		provided[capability] = struct{}{}
	}
	dependents := []PluginID{}
	for otherID, entry := range lock.Plugins {
		if otherID == id || !entry.Enabled {
			continue
		}
		installed, err := manager.Store.Installed(otherID, entry.Version)
		if err != nil {
			return nil, err
		}
		for _, dependency := range installed.Manifest.Dependencies.Capabilities {
			if _, ok := provided[dependency]; ok {
				dependents = append(dependents, otherID)
				break
			}
		}
	}
	sort.Slice(dependents, func(i, j int) bool { return dependents[i] < dependents[j] })
	return dependents, nil
}

func (manager Manager) Verify(ctx context.Context, id PluginID) (err error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.verify", "Verifying installed plugin", tracepkg.String("plugin_id", string(id)))
	defer func() { span.Finish(err) }()
	if manager.Store == nil {
		return errors.New("plugin store is unavailable")
	}
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return err
	}
	entry, ok := lock.Plugins[id]
	if !ok {
		return fmt.Errorf("plugin %s is not installed", id)
	}
	installed, err := manager.Store.Installed(id, entry.Version)
	if err != nil {
		return err
	}
	digest, err := ManifestDigest(installed.Manifest)
	if err != nil {
		return err
	}
	if digest != entry.ManifestDigest || installed.Manifest.Publisher != entry.Publisher {
		return errors.New("plugin lock integrity verification failed")
	}
	artifact, err := installed.Manifest.Platform(manager.Store.runtime.OS, manager.Store.runtime.Arch)
	if err != nil {
		return err
	}
	if entry.ArtifactDigest != "sha256:"+artifact.SHA256 {
		return errors.New("plugin artifact lock integrity verification failed")
	}
	return nil
}

func (manager Manager) Outdated(ctx context.Context) (result []OutdatedPlugin, err error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.outdated", "Checking installed plugins for updates")
	defer func() { span.Finish(err, tracepkg.Int("outdated", len(result))) }()
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return nil, err
	}
	result = []OutdatedPlugin{}
	registrySnapshots := map[string][]RegistrySnapshot{}
	for id, entry := range lock.Plugins {
		snapshots, ok := registrySnapshots[entry.Registry]
		if !ok {
			snapshots, err = manager.loadSnapshots(ctx, entry.Registry)
			if err != nil {
				return nil, err
			}
			registrySnapshots[entry.Registry] = snapshots
		}
		resolved, err := ResolveAcross(snapshots, entry.Registry, id, "")
		if err != nil {
			return nil, err
		}
		comparison, err := comparePluginVersions(entry.Version, resolved.Version)
		if err != nil {
			return nil, err
		}
		if comparison < 0 {
			result = append(result, OutdatedPlugin{ID: id, Current: entry.Version, Latest: resolved.Version})
		}
	}
	return result, nil
}

func (manager Manager) Snapshots(ctx context.Context) ([]RegistrySnapshot, error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.registry.snapshots", "Loading plugin registry snapshots")
	snapshots, err := manager.loadSnapshots(ctx, "")
	span.Finish(err, tracepkg.Int("registries", len(snapshots)))
	return snapshots, err
}

func (manager Manager) Resolve(ctx context.Context, reference string) (ResolvedPlugin, error) {
	registryName, id, requestedVersion, err := ParseReference(reference)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	snapshots, err := manager.loadSnapshots(ctx, registryName)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	return ResolveAcross(snapshots, registryName, id, requestedVersion)
}

func (manager Manager) Update(ctx context.Context, id PluginID) (result InstallResult, err error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.update", "Updating plugin", tracepkg.String("plugin_id", string(id)))
	defer func() {
		fields := []tracepkg.Field{}
		if result.Plugin.Manifest.Version != "" {
			fields = append(fields, tracepkg.String("version", string(result.Plugin.Manifest.Version)))
		}
		span.Finish(err, fields...)
	}()
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return InstallResult{}, err
	}
	entry, ok := lock.Plugins[id]
	if !ok {
		return InstallResult{}, fmt.Errorf("plugin %s is not installed", id)
	}
	result, err = manager.Install(ctx, entry.Registry+"/"+string(id))
	if err != nil {
		return InstallResult{}, err
	}
	if !entry.Enabled {
		if err := manager.Store.SetEnabled(id, false); err != nil {
			return result, fmt.Errorf("restore disabled plugin state after update: %w", err)
		}
	}
	if _, err := manager.PruneVersions(id, DefaultRollbackRetention); err != nil {
		return result, fmt.Errorf("plugin updated to %s but rollback version pruning failed: %w", result.Plugin.Manifest.Version, err)
	}
	return result, nil
}

func (manager Manager) PruneVersions(id PluginID, retainInactive int) ([]Version, error) {
	if manager.Store == nil {
		return nil, errors.New("plugin store is unavailable")
	}
	if retainInactive < 0 {
		return nil, errors.New("rollback retention cannot be negative")
	}
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return nil, err
	}
	active, ok := lock.Plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin %s is not installed", id)
	}
	versions, err := manager.Store.InstalledVersions(id)
	if err != nil {
		return nil, err
	}
	sort.Slice(versions, func(i, j int) bool {
		comparison, compareErr := comparePluginVersions(versions[i], versions[j])
		if compareErr != nil {
			return versions[i] > versions[j]
		}
		return comparison > 0
	})
	keptInactive := 0
	removed := []Version{}
	for _, version := range versions {
		if version == active.Version {
			continue
		}
		if keptInactive < retainInactive {
			keptInactive++
			continue
		}
		if err := manager.Store.RemoveVersion(id, version); err != nil {
			return removed, err
		}
		removed = append(removed, version)
	}
	return removed, nil
}

func (manager Manager) Rollback(ctx context.Context, id PluginID, target Version) (result InstallResult, err error) {
	span := tracepkg.Start(ctx, "PLUGIN", "plugin.rollback", "Rolling back plugin", tracepkg.String("plugin_id", string(id)), tracepkg.String("target_version", string(target)))
	defer func() { span.Finish(err, tracepkg.String("version", string(result.Plugin.Manifest.Version))) }()
	if manager.Store == nil {
		return InstallResult{}, errors.New("plugin store is unavailable")
	}
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return InstallResult{}, err
	}
	entry, ok := lock.Plugins[id]
	if !ok {
		return InstallResult{}, fmt.Errorf("plugin %s is not installed", id)
	}
	if target == "" {
		target, err = manager.previousVersion(id, entry.Version)
		if err != nil {
			return InstallResult{}, err
		}
	} else {
		comparison, compareErr := comparePluginVersions(target, entry.Version)
		if compareErr != nil {
			return InstallResult{}, compareErr
		}
		if comparison >= 0 {
			return InstallResult{}, fmt.Errorf("rollback target %s must be older than active version %s", target, entry.Version)
		}
	}
	installed, err := manager.Store.Installed(id, target)
	if err != nil {
		return InstallResult{}, fmt.Errorf("rollback version %s is not retained locally: %w", target, err)
	}
	if installed.Manifest.Publisher != entry.Publisher {
		return InstallResult{}, fmt.Errorf("rollback publisher %q does not match active publisher %q", installed.Manifest.Publisher, entry.Publisher)
	}
	resolved, err := manager.Resolve(ctx, entry.Registry+"/"+string(id)+"@"+string(target))
	if err != nil {
		return InstallResult{}, err
	}
	result, err = manager.installResolved(ctx, resolved, true)
	if err != nil {
		return InstallResult{}, err
	}
	if !entry.Enabled {
		if err := manager.Store.SetEnabled(id, false); err != nil {
			return result, fmt.Errorf("restore disabled plugin state after rollback: %w", err)
		}
	}
	return result, nil
}

func (manager Manager) previousVersion(id PluginID, active Version) (Version, error) {
	versions, err := manager.Store.InstalledVersions(id)
	if err != nil {
		return "", err
	}
	var previous Version
	for _, version := range versions {
		comparison, err := comparePluginVersions(version, active)
		if err != nil {
			return "", err
		}
		if comparison >= 0 {
			continue
		}
		if previous == "" {
			previous = version
			continue
		}
		newer, err := comparePluginVersions(version, previous)
		if err != nil {
			return "", err
		}
		if newer > 0 {
			previous = version
		}
	}
	if previous == "" {
		return "", fmt.Errorf("plugin %s has no retained rollback version older than %s", id, active)
	}
	return previous, nil
}

func (manager Manager) loadSnapshots(ctx context.Context, requiredRegistry string) ([]RegistrySnapshot, error) {
	config, err := LoadConfig(manager.Store.layout.ConfigPath())
	if err != nil {
		return nil, err
	}
	snapshots := make([]RegistrySnapshot, 0, len(config.Registries)+1)
	found := requiredRegistry == ""
	for _, registry := range config.AllRegistries() {
		if requiredRegistry != "" && registry.Name != requiredRegistry {
			continue
		}
		found = true
		snapshot, refreshErr := manager.RegistryClient.Refresh(ctx, registry)
		if refreshErr != nil {
			snapshot, err = manager.RegistryClient.LoadCached(registry, DefaultRegistryCacheTTL)
			if err != nil {
				if requiredRegistry != "" || registry.Name == OfficialRegistryName || registry.UnqualifiedResolution {
					return nil, fmt.Errorf("refresh plugin registry %s: %w", registry.Name, refreshErr)
				}
				continue
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	if !found {
		return nil, fmt.Errorf("plugin registry %s is not configured", requiredRegistry)
	}
	return snapshots, nil
}

func (manager Manager) downloadArtifact(ctx context.Context, baseURL string, artifact PlatformArtifact) (string, error) {
	if err := os.MkdirAll(manager.Store.layout.DownloadsPath(), 0700); err != nil {
		return "", err
	}
	path := filepath.Join(manager.Store.layout.DownloadsPath(), artifact.Artifact)
	if verifyFileSHA256(path, artifact.SHA256) == nil {
		return path, nil
	}
	_ = os.Remove(path)
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return "", errors.New("plugin artifact URL must use HTTPS")
	}
	assetURL, err := base.Parse(url.PathEscape(artifact.Artifact))
	if err != nil || assetURL.Host != base.Host {
		return "", errors.New("plugin artifact escaped registry host")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "chatgpt-mcp/plugin-installer")
	client := securePluginHTTPClient(manager.HTTPClient, 2*time.Minute, base.Host)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("plugin artifact download returned %s", response.Status)
	}
	if response.ContentLength > maxPluginArtifactSize {
		return "", errors.New("plugin artifact exceeds size limit")
	}
	temp, err := os.CreateTemp(manager.Store.layout.DownloadsPath(), ".download-")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	written, copyErr := io.Copy(temp, io.LimitReader(response.Body, maxPluginArtifactSize+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if written <= 0 || written > maxPluginArtifactSize {
		return "", errors.New("plugin artifact has invalid size")
	}
	if err := verifyFileSHA256(tempPath, artifact.SHA256); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return "", err
	}
	return path, nil
}

func verifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return errors.New("plugin artifact digest mismatch")
	}
	return nil
}

func comparePluginVersions(left, right Version) (int, error) {
	return updatepkg.CompareVersions(string(left), string(right))
}

func securePluginHTTPClient(base *http.Client, timeout time.Duration, allowedHost string) *http.Client {
	client := &http.Client{Timeout: timeout}
	if base != nil {
		*client = *base
		if client.Timeout == 0 {
			client.Timeout = timeout
		}
	}
	previous := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return errors.New("plugin download redirect must use HTTPS")
		}
		if !strings.EqualFold(request.URL.Host, allowedHost) {
			return errors.New("plugin download redirect changed registry host")
		}
		if previous != nil {
			return previous(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 plugin download redirects")
		}
		return nil
	}
	return client
}
