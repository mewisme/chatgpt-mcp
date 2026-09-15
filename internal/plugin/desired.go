package plugin

import (
	"errors"
	"sort"
)

type DesiredReport struct {
	Desired      int
	Satisfied    int
	Missing      []PluginID
	Incompatible []PluginID
	Pending      []PluginID
	LockError    string
}

func (manager Manager) AssessDesired() (DesiredReport, error) {
	if manager.Store == nil {
		return DesiredReport{}, errors.New("plugin store is unavailable")
	}
	config, err := LoadConfig(manager.Store.layout.ConfigPath())
	if err != nil {
		return DesiredReport{}, err
	}
	report := DesiredReport{Desired: len(config.Desired)}
	lock, lockErr := LoadLock(manager.Store.layout.LockPath())
	if lockErr != nil {
		report.LockError = lockErr.Error()
		lock = NewLockFile()
	}
	ids := make([]PluginID, 0, len(config.Desired))
	for id := range config.Desired {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		desired := config.Desired[id]
		installed, err := manager.Store.Installed(id, desired.Version)
		if err != nil {
			report.Missing = append(report.Missing, id)
			continue
		}
		compatible, err := pluginCoreCompatible(manager.Store, installed.Manifest)
		if err != nil || !compatible {
			report.Incompatible = append(report.Incompatible, id)
			continue
		}
		active, ok := lock.Plugins[id]
		if !ok || active.Registry != desired.Registry || active.Version != desired.Version || active.Enabled != desired.Enabled {
			report.Pending = append(report.Pending, id)
			continue
		}
		report.Satisfied++
	}
	return report, nil
}
