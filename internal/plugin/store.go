package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	coreversion "go.mewis.me/chatgpt-mcp/internal/version"
)

var ErrVersionInstalled = errors.New("plugin version is already installed")

type RuntimeContext struct {
	OS          string
	Arch        string
	CoreVersion string
}

type ActivationTrust struct {
	Registry  string
	Publisher string
	Trusted   bool
}

type InstalledPlugin struct {
	Manifest   Manifest
	Root       string
	Payload    string
	Entrypoint string
	Host       *HostExecutableSpec
}

type Store struct {
	layout   Layout
	runtime  RuntimeContext
	Builtins BuiltinRegistry
	mu       sync.Mutex
	peerMu   sync.Mutex
	peers    []*Store
}

func NewStore(layout Layout, context RuntimeContext) (*Store, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(context.OS) == "" {
		context.OS = runtime.GOOS
	}
	if strings.TrimSpace(context.Arch) == "" {
		context.Arch = runtime.GOARCH
	}
	if strings.TrimSpace(context.CoreVersion) == "" {
		context.CoreVersion = coreversion.Version
	}
	return &Store{layout: layout, runtime: context, Builtins: globalBuiltins(layout)}, nil
}

func (store *Store) Layout() Layout { return store.layout }

func (store *Store) SetPeers(peers ...*Store) {
	if store == nil {
		return
	}
	store.peerMu.Lock()
	store.peers = append([]*Store(nil), peers...)
	store.peerMu.Unlock()
}

func (store *Store) Peers() []*Store {
	if store == nil {
		return nil
	}
	store.peerMu.Lock()
	defer store.peerMu.Unlock()
	return append([]*Store(nil), store.peers...)
}

func (store *Store) rejectPeerConflict(id PluginID) error {
	for _, peer := range store.Peers() {
		if peer == nil || peer == store {
			continue
		}
		lock, err := LoadLock(peer.layout.LockPath())
		if err != nil {
			return err
		}
		if entry, ok := lock.Plugins[id]; ok && entry.Enabled {
			return ScopeConflictError{ID: id, Scopes: []PluginScope{peer.layout.EffectiveScope(), store.layout.EffectiveScope()}}
		}
	}
	return nil
}

func (store *Store) dependencyStores() []*Store {
	if store == nil {
		return nil
	}
	if store.layout.EffectiveScope() != ScopeWorkspace {
		return []*Store{store}
	}
	stores := []*Store{}
	for _, peer := range store.Peers() {
		if peer != nil && peer.layout.EffectiveScope() == ScopeGlobal {
			stores = append(stores, peer)
		}
	}
	return append(stores, store)
}

func (store *Store) disablePeerDependents(ids []PluginID) error {
	for _, id := range ids {
		for _, peer := range store.Peers() {
			if _, err := peer.DisableIfEnabled(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func globalBuiltins(layout Layout) BuiltinRegistry {
	if layout.EffectiveScope() == ScopeWorkspace {
		return nil
	}
	return compiledBuiltinClone()
}

func (store *Store) rejectDisallowedScope(manifest Manifest) error {
	scope := store.layout.EffectiveScope()
	if manifest.AllowsScope(scope) {
		return nil
	}
	return fmt.Errorf("%w: %s does not allow %s", ErrScopeNotAllowed, manifest.ID, scope)
}

func (store *Store) Install(manifest Manifest, payloadSource string) (InstalledPlugin, error) {
	unlock, err := store.lockMutation()
	if err != nil {
		return InstalledPlugin{}, err
	}
	defer unlock()
	if _, ok := store.Builtins.Lookup(manifest.ID); ok {
		return InstalledPlugin{}, fmt.Errorf("%w: %s", ErrBuiltinPlugin, manifest.ID)
	}
	if err := manifest.Validate(); err != nil {
		return InstalledPlugin{}, err
	}
	if err := store.rejectDisallowedScope(manifest); err != nil {
		return InstalledPlugin{}, err
	}
	artifact, err := manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return InstalledPlugin{}, err
	}
	hostPath := ""
	if artifact.HostBacked() {
		if strings.TrimSpace(payloadSource) == "" {
			hostPath, err = resolveHostExecutable(artifact)
			if err != nil {
				return InstalledPlugin{}, err
			}
		} else if artifact.Host.Portable == nil {
			return InstalledPlugin{}, errors.New("host plugin does not declare a portable install")
		}
	} else {
		info, err := os.Stat(payloadSource)
		if err != nil {
			return InstalledPlugin{}, fmt.Errorf("inspect plugin payload: %w", err)
		}
		if !info.IsDir() {
			return InstalledPlugin{}, errors.New("plugin payload source must be a directory")
		}
	}
	target := store.layout.InstalledVersionPath(manifest.ID, manifest.Version)
	if _, err := os.Lstat(target); err == nil {
		return InstalledPlugin{}, ErrVersionInstalled
	} else if !errors.Is(err, os.ErrNotExist) {
		return InstalledPlugin{}, err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return InstalledPlugin{}, err
	}
	staging, err := os.MkdirTemp(parent, ".install-")
	if err != nil {
		return InstalledPlugin{}, err
	}
	defer os.RemoveAll(staging)
	payloadTarget := filepath.Join(staging, "payload")
	hostTarget := filepath.Join(staging, "host")
	entrypoint := hostPath
	if artifact.HostBacked() && strings.TrimSpace(payloadSource) != "" {
		if err := copyPayloadTree(payloadSource, hostTarget); err != nil {
			return InstalledPlugin{}, err
		}
		entrypoint = filepath.Join(hostTarget, filepath.FromSlash(artifact.Host.Portable.Entrypoint))
		if err := validateEntrypoint(hostTarget, entrypoint); err != nil {
			return InstalledPlugin{}, err
		}
		digest, err := payloadTreeSHA256(hostTarget)
		if err != nil {
			return InstalledPlugin{}, err
		}
		if err := os.WriteFile(filepath.Join(staging, "host.sha256"), []byte(digest+"\n"), 0600); err != nil {
			return InstalledPlugin{}, err
		}
	} else if !artifact.HostBacked() {
		if err := copyPayloadTree(payloadSource, payloadTarget); err != nil {
			return InstalledPlugin{}, err
		}
		entrypoint = filepath.Join(payloadTarget, filepath.FromSlash(artifact.Entrypoint))
		if err := validateEntrypoint(payloadTarget, entrypoint); err != nil {
			return InstalledPlugin{}, err
		}
		if err := writePayloadIntegrity(staging, payloadTarget); err != nil {
			return InstalledPlugin{}, err
		}
		if _, err := DiscoverPayloadResources(payloadTarget); err != nil {
			return InstalledPlugin{}, err
		}
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return InstalledPlugin{}, err
	}
	if err := os.WriteFile(filepath.Join(staging, "plugin.json"), append(manifestData, '\n'), 0600); err != nil {
		return InstalledPlugin{}, err
	}
	if err := os.Rename(staging, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return InstalledPlugin{}, ErrVersionInstalled
		}
		return InstalledPlugin{}, err
	}
	if artifact.HostBacked() {
		if strings.TrimSpace(payloadSource) != "" {
			hostPath = filepath.Join(target, "host", filepath.FromSlash(artifact.Host.Portable.Entrypoint))
		}
		return InstalledPlugin{Manifest: manifest, Root: target, Entrypoint: hostPath, Host: artifact.Host}, nil
	}
	return InstalledPlugin{Manifest: manifest, Root: target, Payload: filepath.Join(target, "payload"), Entrypoint: filepath.Join(target, "payload", filepath.FromSlash(artifact.Entrypoint))}, nil
}

func (store *Store) Installed(id PluginID, version Version) (InstalledPlugin, error) {
	if !validCanonicalName(string(id)) {
		return InstalledPlugin{}, fmt.Errorf("invalid plugin id: %q", id)
	}
	if err := validateVersion(string(version)); err != nil {
		return InstalledPlugin{}, fmt.Errorf("invalid plugin version: %q", version)
	}
	root := store.layout.InstalledVersionPath(id, version)
	data, err := os.ReadFile(filepath.Join(root, "plugin.json"))
	if err != nil {
		return InstalledPlugin{}, err
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return InstalledPlugin{}, err
	}
	if manifest.ID != id || manifest.Version != version {
		return InstalledPlugin{}, errors.New("installed plugin manifest identity does not match store path")
	}
	artifact, err := manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return InstalledPlugin{}, err
	}
	if artifact.HostBacked() {
		if artifact.Host.Portable != nil {
			hostRoot := filepath.Join(root, "host")
			if _, err := os.Stat(hostRoot); err == nil {
				entrypoint := filepath.Join(hostRoot, filepath.FromSlash(artifact.Host.Portable.Entrypoint))
				if err := validateEntrypoint(hostRoot, entrypoint); err != nil {
					return InstalledPlugin{}, err
				}
				digestData, err := os.ReadFile(filepath.Join(root, "host.sha256"))
				if err != nil {
					return InstalledPlugin{}, fmt.Errorf("read portable host integrity metadata: %w", err)
				}
				digest := strings.TrimSpace(string(digestData))
				if !validSHA256(digest) {
					return InstalledPlugin{}, errors.New("portable host integrity metadata is invalid")
				}
				actual, err := payloadTreeSHA256(hostRoot)
				if err != nil {
					return InstalledPlugin{}, fmt.Errorf("portable host integrity verification failed: %w", err)
				}
				if actual != digest {
					return InstalledPlugin{}, fmt.Errorf("portable host integrity verification failed: expected %s, got %s", digest, actual)
				}
				return InstalledPlugin{Manifest: manifest, Root: root, Entrypoint: entrypoint, Host: artifact.Host}, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return InstalledPlugin{}, err
			}
		}
		entrypoint, err := resolveHostExecutable(artifact)
		if err != nil {
			return InstalledPlugin{}, err
		}
		return InstalledPlugin{Manifest: manifest, Root: root, Entrypoint: entrypoint, Host: artifact.Host}, nil
	}
	payload := filepath.Join(root, "payload")
	entrypoint := filepath.Join(payload, filepath.FromSlash(artifact.Entrypoint))
	if err := validateEntrypoint(payload, entrypoint); err != nil {
		return InstalledPlugin{}, err
	}
	return InstalledPlugin{Manifest: manifest, Root: root, Payload: payload, Entrypoint: entrypoint}, nil
}

func (store *Store) Activate(id PluginID, version Version, trust ActivationTrust) error {
	return store.ActivateWithState(id, version, trust, true)
}

func (store *Store) ActivateWithState(id PluginID, version Version, trust ActivationTrust, enabled bool) error {
	unlock, err := store.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()
	if !trust.Trusted {
		return errors.New("plugin publisher is not trusted")
	}
	if !validCanonicalName(trust.Registry) {
		return fmt.Errorf("invalid plugin registry: %q", trust.Registry)
	}
	installed, err := store.Installed(id, version)
	if err != nil {
		return err
	}
	if err := store.rejectDisallowedScope(installed.Manifest); err != nil {
		return err
	}
	if installed.Manifest.Publisher != trust.Publisher {
		return fmt.Errorf("trusted publisher %q does not match manifest publisher %q", trust.Publisher, installed.Manifest.Publisher)
	}
	compatible, err := pluginCoreCompatible(store, installed.Manifest)
	if err != nil {
		return err
	}
	if !compatible {
		return fmt.Errorf("plugin %s@%s is incompatible with chatgpt-mcp %s", id, version, store.runtime.CoreVersion)
	}
	if err := ValidateDependencies(store, installed.Manifest); err != nil {
		return err
	}
	if enabled {
		if err := store.rejectPeerConflict(id); err != nil {
			return err
		}
		if err := store.verifyInstalledIntegrity(context.Background(), installed); err != nil {
			return err
		}
	}
	if err := store.writeInstalledTrust(installed, trust); err != nil {
		return err
	}
	manifestDigest, err := ManifestDigest(installed.Manifest)
	if err != nil {
		return err
	}
	artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return err
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return err
	}
	saved, had := lock.Plugins[id]
	currentPayload := ""
	if had && saved.Enabled {
		previousInstalled, err := store.Installed(id, saved.Version)
		if err != nil {
			return err
		}
		currentPayload = previousInstalled.Payload
	}
	nextPayload := ""
	if enabled {
		nextPayload = installed.Payload
	}
	previous := cloneLock(lock)
	lock.Plugins[id] = LockPlugin{Registry: trust.Registry, Publisher: trust.Publisher, Version: version, ManifestDigest: manifestDigest, ArtifactDigest: platformLockDigest(artifact), Enabled: enabled}
	return store.commitLockAndProject(previous, lock, currentPayload, nextPayload, func(config *Config) error {
		return config.SetDesired(id, trust.Registry, version, enabled)
	}, func(config *Config) error {
		if !had {
			config.RemoveDesired(id)
			return nil
		}
		return config.SetDesired(id, saved.Registry, saved.Version, saved.Enabled)
	})
}

func (store *Store) lookupBuiltin(id PluginID) (Builtin, bool) {
	if store != nil {
		if builtin, ok := store.Builtins.Lookup(id); ok {
			return builtin, true
		}
	}
	return compiledBuiltin(id)
}

func (store *Store) BuiltinEnabled(id PluginID) bool {
	builtin, ok := store.lookupBuiltin(id)
	if !ok {
		return false
	}
	if store == nil {
		return builtin.DefaultEnabled
	}
	config, err := store.layout.LoadConfig()
	if err != nil {
		return builtin.DefaultEnabled
	}
	if state, ok := config.Builtins[id]; ok {
		return state.Enabled
	}
	return builtin.DefaultEnabled
}

func (store *Store) setBuiltinEnabled(builtin Builtin, enabled bool) error {
	if !builtin.Disableable {
		return fmt.Errorf("%w: %s cannot be disabled", ErrBuiltinPlugin, builtin.ID)
	}
	if store.layout.EffectiveScope() != ScopeGlobal {
		return fmt.Errorf("built-in plugin %s is global-only", builtin.ID)
	}
	return MutateConfig(store.layout, func(config *Config) error {
		if enabled == builtin.DefaultEnabled {
			if config.Builtins != nil {
				delete(config.Builtins, builtin.ID)
				if len(config.Builtins) == 0 {
					config.Builtins = nil
				}
			}
			return nil
		}
		if config.Builtins == nil {
			config.Builtins = map[PluginID]BuiltinState{}
		}
		config.Builtins[builtin.ID] = BuiltinState{Enabled: enabled}
		return nil
	})
}

func (store *Store) SetEnabled(id PluginID, enabled bool) error {
	if builtin, ok := store.lookupBuiltin(id); ok {
		return store.setBuiltinEnabled(builtin, enabled)
	}
	unlock, err := store.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return err
	}
	entry, ok := lock.Plugins[id]
	if !ok {
		return fmt.Errorf("plugin %s is not active", id)
	}
	if _, builtin := store.Builtins.Lookup(id); builtin {
		return fmt.Errorf("%w: %s", ErrBuiltinPlugin, id)
	}
	installed, err := store.Installed(id, entry.Version)
	if err != nil {
		return err
	}
	if enabled {
		digest, err := ManifestDigest(installed.Manifest)
		if err != nil {
			return err
		}
		artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
		if err != nil {
			return err
		}
		if digest != entry.ManifestDigest || installed.Manifest.Publisher != entry.Publisher || entry.ArtifactDigest != platformLockDigest(artifact) {
			return fmt.Errorf("plugin %s lock integrity verification failed", id)
		}
		compatible, err := pluginCoreCompatible(store, installed.Manifest)
		if err != nil {
			return err
		}
		if !compatible {
			return fmt.Errorf("plugin %s@%s is incompatible with chatgpt-mcp %s", id, entry.Version, store.runtime.CoreVersion)
		}
		if err := store.rejectPeerConflict(id); err != nil {
			return err
		}
		if err := ValidateDependencies(store, installed.Manifest); err != nil {
			return err
		}
		if err := store.verifyInstalledIntegrity(context.Background(), installed); err != nil {
			return err
		}
	} else if err := store.rejectPeerDependents(id, entry.Version); err != nil {
		return err
	}
	currentPayload, nextPayload := "", ""
	if entry.Enabled {
		currentPayload = installed.Payload
	}
	if enabled {
		nextPayload = installed.Payload
	}
	saved := entry
	previous := cloneLock(lock)
	entry.Enabled = enabled
	lock.Plugins[id] = entry
	return store.commitLockAndProject(previous, lock, currentPayload, nextPayload, func(config *Config) error {
		return config.SetDesired(id, entry.Registry, entry.Version, enabled)
	}, func(config *Config) error {
		return config.SetDesired(id, saved.Registry, saved.Version, saved.Enabled)
	})
}

func (store *Store) rejectPeerDependents(id PluginID, version Version) error {
	if store == nil || store.layout.EffectiveScope() != ScopeGlobal {
		return nil
	}
	dependents, err := store.peerDependents(id, version)
	if err != nil {
		return err
	}
	if len(dependents) == 0 {
		return nil
	}
	return fmt.Errorf("plugin %s is required by active workspace plugin %s", id, dependents[0])
}

func (store *Store) peerDependents(id PluginID, version Version) ([]PluginID, error) {
	provided, err := store.providedCapabilities(id, version)
	if err != nil {
		return nil, err
	}
	if len(provided) == 0 {
		return nil, nil
	}
	dependents := []PluginID{}
	for _, peer := range store.Peers() {
		if peer == nil || peer == store || peer.layout.EffectiveScope() != ScopeWorkspace {
			continue
		}
		ids, err := peer.dependentsOf(id, provided)
		if err != nil {
			return nil, err
		}
		dependents = append(dependents, ids...)
	}
	sort.Slice(dependents, func(i, j int) bool { return dependents[i] < dependents[j] })
	return dependents, nil
}

func (store *Store) providedCapabilities(id PluginID, version Version) (map[Capability]struct{}, error) {
	installed, err := store.Installed(id, version)
	if err != nil {
		return nil, err
	}
	provided := make(map[Capability]struct{}, len(installed.Manifest.Provides))
	for _, capability := range installed.Manifest.Provides {
		provided[capability] = struct{}{}
	}
	return provided, nil
}

func (store *Store) dependentsOf(id PluginID, provided map[Capability]struct{}) ([]PluginID, error) {
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return nil, err
	}
	dependents := []PluginID{}
	for otherID, entry := range lock.Plugins {
		if otherID == id || !entry.Enabled {
			continue
		}
		installed, err := store.Installed(otherID, entry.Version)
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

func (store *Store) DisableIfEnabled(id PluginID) (bool, error) {
	if store == nil {
		return false, errors.New("plugin store is unavailable")
	}
	if !validCanonicalName(string(id)) {
		return false, fmt.Errorf("invalid plugin id: %q", id)
	}
	unlock, err := store.lockMutation()
	if err != nil {
		return false, err
	}
	defer unlock()
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return false, err
	}
	entry, ok := lock.Plugins[id]
	if !ok || !entry.Enabled {
		return false, nil
	}
	installed, err := store.Installed(id, entry.Version)
	if err != nil {
		return false, err
	}
	saved := entry
	cloned := cloneLock(lock)
	entry.Enabled = false
	lock.Plugins[id] = entry
	if err := store.commitLockAndProject(cloned, lock, installed.Payload, "", func(config *Config) error {
		return config.SetDesired(id, entry.Registry, entry.Version, false)
	}, func(config *Config) error {
		return config.SetDesired(id, saved.Registry, saved.Version, saved.Enabled)
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (store *Store) writeLockAndDesired(previous, next LockFile, mutate func(*Config) error) error {
	config, err := store.layout.LoadConfig()
	if err != nil {
		return err
	}
	if mutate != nil {
		if err := mutate(&config); err != nil {
			return err
		}
	}
	if err := WriteLock(store.layout.LockPath(), next); err != nil {
		return err
	}
	if err := store.layout.WriteConfig(config); err != nil {
		if rollbackErr := WriteLock(store.layout.LockPath(), previous); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("restore previous plugin lock: %w", rollbackErr))
		}
		return err
	}
	return nil
}

func (store *Store) commitLockAndProject(previous, next LockFile, currentPayload, nextPayload string, mutate, restore func(*Config) error) error {
	if err := preflightProjectionPayloads(store.layout, currentPayload, nextPayload); err != nil {
		return err
	}
	if err := store.writeLockAndDesired(previous, next, mutate); err != nil {
		return err
	}
	if err := SyncProjections(store.layout, currentPayload, nextPayload); err != nil {
		if restoreErr := store.writeLockAndDesired(next, previous, restore); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		return err
	}
	return nil
}

func cloneLock(lock LockFile) LockFile {
	plugins := make(map[PluginID]LockPlugin, len(lock.Plugins))
	for id, entry := range lock.Plugins {
		plugins[id] = entry
	}
	return LockFile{Schema: lock.Schema, Plugins: plugins}
}

func unprojectLockEntries(store *Store, lock LockFile, ids []PluginID) error {
	payloads := make([]string, 0, len(ids))
	seen := make(map[PluginID]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		entry, ok := lock.Plugins[id]
		if !ok || !entry.Enabled {
			continue
		}
		installed, err := store.Installed(id, entry.Version)
		if err != nil {
			return err
		}
		payloads = append(payloads, installed.Payload)
	}
	for _, payload := range payloads {
		if err := preflightProjectionPayloads(store.layout, payload, ""); err != nil {
			return err
		}
	}
	for _, payload := range payloads {
		if err := SyncProjections(store.layout, payload, ""); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) RemoveVersion(id PluginID, version Version) error {
	unlock, err := store.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return err
	}
	if entry, ok := lock.Plugins[id]; ok && entry.Version == version {
		return fmt.Errorf("plugin %s@%s is referenced by active lock state", id, version)
	}
	return os.RemoveAll(store.layout.InstalledVersionPath(id, version))
}

func (store *Store) InstalledVersions(id PluginID) ([]Version, error) {
	if !validCanonicalName(string(id)) {
		return nil, fmt.Errorf("invalid plugin id: %q", id)
	}
	entries, err := os.ReadDir(filepath.Join(store.layout.PluginsPath(), string(id)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	versions := make([]Version, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || validateVersion(entry.Name()) != nil {
			continue
		}
		versions = append(versions, Version(entry.Name()))
	}
	sort.Slice(versions, func(i, j int) bool { return string(versions[i]) < string(versions[j]) })
	return versions, nil
}

func copyPayloadTree(source, target string) error {
	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, 0700)
		}
		destination := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin payload symlink is not allowed: %s", relative)
		}
		if entry.IsDir() {
			return os.Mkdir(destination, info.Mode().Perm()|0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("plugin payload special file is not allowed: %s", relative)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		return copyRegularFile(current, destination, info.Mode().Perm())
	})
}

func copyRegularFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode&0777)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func validateEntrypoint(payloadRoot, entrypoint string) error {
	relative, err := filepath.Rel(payloadRoot, entrypoint)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("plugin entrypoint escapes payload root")
	}
	info, err := os.Stat(entrypoint)
	if err != nil {
		return fmt.Errorf("plugin entrypoint is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("plugin entrypoint must be a regular file")
	}
	return nil
}
