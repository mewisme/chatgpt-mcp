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
	layout  Layout
	runtime RuntimeContext
	mu      sync.Mutex
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
	return &Store{layout: layout, runtime: context}, nil
}

func (store *Store) Layout() Layout { return store.layout }

func (store *Store) Install(manifest Manifest, payloadSource string) (InstalledPlugin, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := manifest.Validate(); err != nil {
		return InstalledPlugin{}, err
	}
	artifact, err := manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return InstalledPlugin{}, err
	}
	hostPath := ""
	if artifact.HostBacked() {
		hostPath, err = resolveHostExecutable(artifact)
		if err != nil {
			return InstalledPlugin{}, err
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
	entrypoint := hostPath
	if !artifact.HostBacked() {
		if err := copyPayloadTree(payloadSource, payloadTarget); err != nil {
			return InstalledPlugin{}, err
		}
		entrypoint = filepath.Join(payloadTarget, filepath.FromSlash(artifact.Entrypoint))
		if err := validateEntrypoint(payloadTarget, entrypoint); err != nil {
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
	store.mu.Lock()
	defer store.mu.Unlock()
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
	previous := lock
	if artifact.HostBacked() && enabled {
		if _, err := preflightHostExecutable(context.Background(), artifact); err != nil {
			return err
		}
	}
	lock.Plugins[id] = LockPlugin{Registry: trust.Registry, Publisher: trust.Publisher, Version: version, ManifestDigest: manifestDigest, ArtifactDigest: platformLockDigest(artifact), Enabled: enabled}
	return store.writeLockAndDesired(previous, lock, func(config *Config) error {
		return config.SetDesired(id, trust.Registry, version, enabled)
	})
}

func (store *Store) SetEnabled(id PluginID, enabled bool) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return err
	}
	previous := lock
	entry, ok := lock.Plugins[id]
	if !ok {
		return fmt.Errorf("plugin %s is not active", id)
	}
	if enabled {
		installed, err := store.Installed(id, entry.Version)
		if err != nil {
			return err
		}
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
		if err := ValidateDependencies(store, installed.Manifest); err != nil {
			return err
		}
		if artifact.HostBacked() {
			if _, err := preflightHostExecutable(context.Background(), artifact); err != nil {
				return err
			}
		}
	}
	entry.Enabled = enabled
	lock.Plugins[id] = entry
	return store.writeLockAndDesired(previous, lock, func(config *Config) error {
		return config.SetDesired(id, entry.Registry, entry.Version, enabled)
	})
}

func (store *Store) DisableIfEnabled(id PluginID) (bool, error) {
	if store == nil {
		return false, errors.New("plugin store is unavailable")
	}
	if !validCanonicalName(string(id)) {
		return false, fmt.Errorf("invalid plugin id: %q", id)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		return false, err
	}
	previous := lock
	entry, ok := lock.Plugins[id]
	if !ok || !entry.Enabled {
		return false, nil
	}
	entry.Enabled = false
	lock.Plugins[id] = entry
	if err := store.writeLockAndDesired(previous, lock, func(config *Config) error {
		return config.SetDesired(id, entry.Registry, entry.Version, false)
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (store *Store) writeLockAndDesired(previous, next LockFile, mutate func(*Config) error) error {
	config, err := LoadConfig(store.layout.ConfigPath())
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
	if err := WriteConfig(store.layout.ConfigPath(), config); err != nil {
		if rollbackErr := WriteLock(store.layout.LockPath(), previous); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("restore previous plugin lock: %w", rollbackErr))
		}
		return err
	}
	return nil
}

func (store *Store) RemoveVersion(id PluginID, version Version) error {
	store.mu.Lock()
	defer store.mu.Unlock()
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
