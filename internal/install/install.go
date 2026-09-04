package install

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrDevelopmentBuild = errors.New("development build cannot be installed as a release without --force")

type Options struct {
	Layout        Layout
	Version       string
	Source        string
	NoAlias       bool
	Force         bool
	MigrateLegacy bool
}

type Result struct {
	Layout           Layout
	Version          string
	Source           string
	Staged           Staged
	Activation       Activation
	PreviousMetadata *Metadata
	Canonical        CanonicalStatus
	Alias            AliasStatus
	AliasInstalled   bool
	AlreadyInstalled bool
	Legacy           LegacyCleanupResult
}

func Install(options Options) (Result, error) {
	version := normalizeInstallVersion(options.Version)
	if version == "" {
		return Result{}, errors.New("install version is required")
	}
	if isDevelopmentVersion(version) && !options.Force {
		return Result{}, ErrDevelopmentBuild
	}
	layout := options.Layout
	if strings.TrimSpace(layout.Root) == "" {
		var err error
		layout, err = DefaultLayout()
		if err != nil {
			return Result{}, err
		}
	}
	source := strings.TrimSpace(options.Source)
	if source == "" {
		var err error
		source, err = os.Executable()
		if err != nil {
			return Result{}, err
		}
	}
	resolvedSource, err := resolveSourceBinary(source)
	if err != nil {
		return Result{}, err
	}
	legacyItems := []LegacyInstallation{}
	if options.MigrateLegacy {
		legacyItems, err = FindLegacyInstallations(layout, resolvedSource)
		if err != nil {
			return Result{}, err
		}
	}
	canonicalBefore, err := StatusCanonical(layout)
	if err != nil {
		return Result{}, err
	}
	var canonicalLegacy *LegacyInstallation
	if canonicalBefore.State == CanonicalConflict {
		if options.MigrateLegacy {
			canonicalLegacy, err = inspectCanonicalLegacy(layout, resolvedSource)
			if err != nil {
				return Result{}, err
			}
		}
		if canonicalLegacy == nil || !canonicalLegacy.Removable {
			if canonicalLegacy != nil && canonicalLegacy.Reason != "" {
				return Result{}, fmt.Errorf("%w: %s (%s)", ErrCanonicalConflict, canonicalBefore.Path, canonicalLegacy.Reason)
			}
			return Result{}, fmt.Errorf("%w: %s", ErrCanonicalConflict, canonicalBefore.Path)
		}
		if removableLegacyAt(legacyItems, canonicalLegacy.Path) == nil {
			legacyItems = append(legacyItems, *canonicalLegacy)
		}
	}
	aliasBefore := AliasStatus{State: AliasMissing, Path: layout.AliasPath, Target: layout.CurrentBinary}
	migrateAlias := false
	if !options.NoAlias {
		aliasBefore, err = StatusAlias(layout)
		if err != nil {
			return Result{}, err
		}
		if aliasBefore.State == AliasConflict {
			if options.MigrateLegacy {
				migrateAlias, err = legacyAliasMatchesAny(layout, legacyItems)
				if err != nil {
					return Result{}, err
				}
			}
			if !migrateAlias {
				return Result{}, fmt.Errorf("%w: %s", ErrAliasConflict, aliasBefore.Path)
			}
		}
	}
	previousMetadata, err := existingMetadata(layout.Metadata)
	if err != nil {
		return Result{}, err
	}
	metadataMatches := metadataMatches(previousMetadata, layout, version)
	staged, err := Stage(layout, version, resolvedSource)
	if err != nil {
		return Result{}, err
	}
	activation, err := Activate(staged)
	if err != nil {
		return Result{}, err
	}
	result := Result{Layout: layout, Version: version, Source: resolvedSource, Staged: staged, Activation: activation, PreviousMetadata: previousMetadata}
	backups := []legacyBackup{}
	rollback := func(cause error) (Result, error) {
		rollbackErr := Rollback(activation)
		if canonicalBefore.State == CanonicalMissing || canonicalLegacy != nil {
			if _, removeErr := RemoveCanonical(layout); removeErr != nil {
				rollbackErr = errors.Join(rollbackErr, removeErr)
			}
		}
		if !options.NoAlias && (aliasBefore.State == AliasMissing || migrateAlias) {
			if _, removeErr := RemoveAlias(layout); removeErr != nil {
				rollbackErr = errors.Join(rollbackErr, removeErr)
			}
		}
		if restoreErr := restoreLegacyBackups(backups); restoreErr != nil {
			rollbackErr = errors.Join(rollbackErr, restoreErr)
		}
		if rollbackErr != nil {
			return Result{}, fmt.Errorf("%w; rollback failed: %v", cause, rollbackErr)
		}
		return Result{}, cause
	}
	if canonicalLegacy != nil {
		backup, err := backupLegacyPath(canonicalLegacy.Path)
		if err != nil {
			return rollback(err)
		}
		backups = append(backups, backup)
	}
	if migrateAlias {
		backup, err := backupLegacyPath(layout.AliasPath)
		if err != nil {
			return rollback(err)
		}
		backups = append(backups, backup)
	}
	canonical, err := InstallCanonical(layout)
	if err != nil {
		return rollback(err)
	}
	result.Canonical = canonical
	if !options.NoAlias {
		alias, err := InstallAlias(layout)
		if err != nil {
			return rollback(err)
		}
		result.Alias = alias
		result.AliasInstalled = alias.State == AliasInstalled
	}
	metadata := Metadata{Schema: MetadataSchema, Method: MethodDirect, Version: version, InstallDir: layout.Root, BinDir: layout.BinDir}
	if err := WriteMetadata(layout.Metadata, metadata); err != nil {
		return rollback(err)
	}
	if canonicalLegacy != nil {
		result.Legacy.Removed = append(result.Legacy.Removed, *canonicalLegacy)
	}
	if migrateAlias {
		result.Legacy.RemovedAliases = append(result.Legacy.RemovedAliases, layout.AliasPath)
	}
	discardLegacyBackups(backups, &result.Legacy)
	if options.MigrateLegacy {
		cleanup, cleanupErr := CleanupLegacyInstallations(LegacyCleanupOptions{Layout: layout, Source: resolvedSource})
		if cleanupErr != nil {
			result.Legacy.Failed = append(result.Legacy.Failed, LegacyCleanupFailure{Path: "PATH", Err: cleanupErr})
		} else {
			result.Legacy.Removed = append(result.Legacy.Removed, cleanup.Removed...)
			result.Legacy.RemovedAliases = append(result.Legacy.RemovedAliases, cleanup.RemovedAliases...)
			result.Legacy.Preserved = append(result.Legacy.Preserved, cleanup.Preserved...)
			result.Legacy.Failed = append(result.Legacy.Failed, cleanup.Failed...)
		}
	}
	result.AlreadyInstalled = metadataMatches && staged.Reused && activation.PreviousVersion == version && canonicalBefore.State == CanonicalInstalled && (options.NoAlias || aliasBefore.State == AliasInstalled)
	return result, nil
}

func normalizeInstallVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || isDevelopmentVersion(version) {
		return version
	}
	if version[0] == 'V' {
		return "v" + version[1:]
	}
	if version[0] != 'v' {
		return "v" + version
	}
	return version
}

func existingMetadata(path string) (*Metadata, error) {
	metadata, err := ReadMetadata(path)
	if errors.Is(err, ErrMetadataNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &metadata, nil
}

func metadataMatches(metadata *Metadata, layout Layout, version string) bool {
	return metadata != nil && metadata.Method == MethodDirect && metadata.Version == version && samePath(metadata.InstallDir, layout.Root) && samePath(metadata.BinDir, layout.BinDir)
}

func isDevelopmentVersion(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	return version == "dev" || version == "(devel)" || strings.HasPrefix(version, "dev-")
}
