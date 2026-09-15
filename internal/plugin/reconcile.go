package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

type ReconcileReport struct {
	CorruptLock    bool
	QuarantinePath string
	Disabled       []PluginID
	Issues         map[PluginID]string
}

func Reconcile(store *Store) (ReconcileReport, error) {
	if store == nil {
		return ReconcileReport{}, errors.New("plugin store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		if !errors.Is(err, ErrLockCorrupt) {
			return ReconcileReport{}, err
		}
		return reconcileCorruptLock(store, err)
	}
	manifests := map[PluginID]Manifest{}
	disabled := map[PluginID]struct{}{}
	issues := map[PluginID]string{}
	for id, entry := range lock.Plugins {
		if !entry.Enabled {
			continue
		}
		manifest, err := reconcileLockEntry(store, id, entry)
		if err != nil {
			entry.Enabled = false
			lock.Plugins[id] = entry
			disabled[id] = struct{}{}
			issues[id] = err.Error()
			continue
		}
		manifests[id] = manifest
	}
	for {
		capabilities := map[Capability]struct{}{}
		for id, manifest := range manifests {
			if lock.Plugins[id].Enabled {
				for _, capability := range manifest.Provides {
					capabilities[capability] = struct{}{}
				}
			}
		}
		changed := false
		for id, manifest := range manifests {
			entry := lock.Plugins[id]
			if !entry.Enabled {
				continue
			}
			for _, dependency := range manifest.Dependencies.Capabilities {
				if _, ok := capabilities[dependency]; ok {
					continue
				}
				entry.Enabled = false
				lock.Plugins[id] = entry
				disabled[id] = struct{}{}
				issues[id] = fmt.Sprintf("missing required capability %s", dependency)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	if len(disabled) == 0 {
		return ReconcileReport{}, nil
	}
	if err := WriteLock(store.layout.LockPath(), lock); err != nil {
		return ReconcileReport{}, err
	}
	report := ReconcileReport{Disabled: make([]PluginID, 0, len(disabled)), Issues: issues}
	for id := range disabled {
		report.Disabled = append(report.Disabled, id)
	}
	sort.Slice(report.Disabled, func(i, j int) bool { return report.Disabled[i] < report.Disabled[j] })
	return report, nil
}

func reconcileLockEntry(store *Store, id PluginID, entry LockPlugin) (Manifest, error) {
	installed, err := store.Installed(id, entry.Version)
	if err != nil {
		return Manifest{}, err
	}
	digest, err := ManifestDigest(installed.Manifest)
	if err != nil {
		return Manifest{}, err
	}
	if digest != entry.ManifestDigest || installed.Manifest.Publisher != entry.Publisher {
		return Manifest{}, errors.New("plugin lock integrity verification failed")
	}
	artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return Manifest{}, err
	}
	if entry.ArtifactDigest != platformLockDigest(artifact) {
		return Manifest{}, errors.New("plugin artifact lock integrity verification failed")
	}
	if artifact.HostBacked() {
		if err := preflightHostPath(context.Background(), installed.Entrypoint, artifact.Host); err != nil {
			return Manifest{}, err
		}
	}
	compatible, err := pluginCoreCompatible(store, installed.Manifest)
	if err != nil {
		return Manifest{}, err
	}
	if !compatible {
		return Manifest{}, fmt.Errorf("plugin %s@%s is incompatible with chatgpt-mcp %s", id, entry.Version, store.runtime.CoreVersion)
	}
	return installed.Manifest, nil
}

func reconcileCorruptLock(store *Store, cause error) (ReconcileReport, error) {
	lockPath := store.layout.LockPath()
	if _, err := os.Stat(lockPath); errors.Is(err, os.ErrNotExist) {
		return ReconcileReport{}, cause
	} else if err != nil {
		return ReconcileReport{}, errors.Join(cause, err)
	}
	quarantine := fmt.Sprintf("%s.corrupt-%s", lockPath, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.Rename(lockPath, quarantine); err != nil {
		return ReconcileReport{}, errors.Join(cause, fmt.Errorf("quarantine corrupt plugin lock: %w", err))
	}
	if err := WriteLock(lockPath, NewLockFile()); err != nil {
		if restoreErr := os.Rename(quarantine, lockPath); restoreErr != nil {
			return ReconcileReport{}, errors.Join(cause, err, fmt.Errorf("restore corrupt plugin lock after recovery failure: %w", restoreErr))
		}
		return ReconcileReport{}, errors.Join(cause, err)
	}
	return ReconcileReport{CorruptLock: true, QuarantinePath: quarantine}, nil
}
