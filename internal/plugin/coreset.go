package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CoreAction string

const (
	CoreActionInstalled CoreAction = "installed"
	CoreActionUpdated   CoreAction = "updated"
	CoreActionEnabled   CoreAction = "enabled"
	CoreActionRetained  CoreAction = "retained"
	CoreActionSkipped   CoreAction = "skipped"
	CoreActionFailed    CoreAction = "failed"
)

type CoreSpec struct {
	ID        PluginID
	Version   Version
	Required  bool
	Enabled   bool
	Platforms []string
}

type CoreReconcileItem struct {
	ID       PluginID
	Version  Version
	Action   CoreAction
	Required bool
	Error    string
}

type CoreReconcileReport struct {
	Items []CoreReconcileItem
}

func (report CoreReconcileReport) RequiredError() error {
	for _, item := range report.Items {
		if item.Required && item.Action == CoreActionFailed {
			if strings.TrimSpace(item.Error) == "" {
				return fmt.Errorf("required core plugin %s failed", item.ID)
			}
			return fmt.Errorf("required core plugin %s failed: %s", item.ID, item.Error)
		}
	}
	return nil
}

func CoreSetFromIndex(index RegistryIndex) []CoreSpec {
	ids := make([]string, 0, len(index.Plugins))
	for id := range index.Plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	specs := make([]CoreSpec, 0)
	for _, idValue := range ids {
		id := PluginID(idValue)
		entry := index.Plugins[id]
		if entry.Core == nil {
			continue
		}
		specs = append(specs, CoreSpec{
			ID: id, Version: entry.Stable, Required: entry.Core.Required,
			Enabled: entry.Core.defaultEnabled(), Platforms: append([]string(nil), entry.Core.Platforms...),
		})
	}
	return specs
}

func DefaultPluginBundleDir() string {
	if dir := strings.TrimSpace(os.Getenv("CHATGPT_MCP_PLUGIN_BUNDLE")); dir != "" {
		return dir
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Join(filepath.Dir(exe), "plugins")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

func (manager Manager) ReconcileCoreSet(ctx context.Context) (CoreReconcileReport, error) {
	if manager.Store == nil {
		return CoreReconcileReport{}, fmt.Errorf("plugin store is unavailable")
	}
	snapshots, err := manager.Snapshots(ctx)
	if err != nil {
		return CoreReconcileReport{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Registry.Name == OfficialRegistryName {
			return manager.reconcileCoreSet(ctx, snapshot)
		}
	}
	return CoreReconcileReport{}, fmt.Errorf("official plugin registry unavailable")
}

func (manager Manager) reconcileCoreSet(ctx context.Context, snapshot RegistrySnapshot) (CoreReconcileReport, error) {
	lock, err := LoadLock(manager.Store.layout.LockPath())
	if err != nil {
		return CoreReconcileReport{}, err
	}
	report := CoreReconcileReport{}
	for _, spec := range CoreSetFromIndex(snapshot.Index) {
		item, nextLock := manager.reconcileCoreSpec(ctx, snapshot, lock, spec)
		lock = nextLock
		report.Items = append(report.Items, item)
	}
	return report, report.RequiredError()
}

func (manager Manager) reconcileCoreSpec(ctx context.Context, snapshot RegistrySnapshot, lock LockFile, spec CoreSpec) (CoreReconcileItem, LockFile) {
	item := CoreReconcileItem{ID: spec.ID, Version: spec.Version, Required: spec.Required}
	fail := func(err error) CoreReconcileItem {
		item.Action = CoreActionFailed
		item.Error = err.Error()
		return item
	}
	if !corePlatformsMatch(spec.Platforms, manager.Store.runtime.OS, manager.Store.runtime.Arch) {
		item.Action = CoreActionSkipped
		return item, lock
	}
	resolved, err := snapshot.Resolve(spec.ID, spec.Version)
	if err != nil {
		return fail(err), lock
	}
	current, installed := lock.Plugins[spec.ID]
	enabled := spec.Enabled || spec.Required
	switch {
	case !installed:
		if _, err := manager.installResolvedWithOptions(ctx, resolved, false, enabled, InstallOptions{}); err != nil {
			return fail(err), lock
		}
		item.Action = CoreActionInstalled
	case current.Version != spec.Version:
		if _, err := manager.installResolvedWithOptions(ctx, resolved, true, current.Enabled || spec.Required, InstallOptions{}); err != nil {
			return fail(err), lock
		}
		if _, err := manager.PruneVersions(spec.ID, DefaultRollbackRetention); err != nil {
			return fail(err), lock
		}
		item.Action = CoreActionUpdated
	case spec.Required && !current.Enabled:
		if err := manager.Store.SetEnabled(spec.ID, true); err != nil {
			return fail(err), lock
		}
		item.Action = CoreActionEnabled
	default:
		item.Action = CoreActionRetained
	}
	if err := manager.Verify(ctx, spec.ID); err != nil {
		return fail(err), lock
	}
	if next, err := LoadLock(manager.Store.layout.LockPath()); err == nil {
		return item, next
	}
	return item, lock
}

func corePlatformsMatch(platforms []string, goos, arch string) bool {
	if len(platforms) == 0 {
		return true
	}
	current := strings.ToLower(strings.TrimSpace(goos) + "/" + strings.TrimSpace(arch))
	for _, platform := range platforms {
		if strings.ToLower(strings.TrimSpace(platform)) == current {
			return true
		}
	}
	return false
}
