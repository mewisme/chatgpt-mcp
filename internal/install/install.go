package install

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrDevelopmentBuild = errors.New("development build cannot be installed as a release without --force")

type Options struct {
	Layout  Layout
	Version string
	Source  string
	NoAlias bool
	Force   bool
}

type Result struct {
	Layout           Layout
	Version          string
	Source           string
	Staged           Staged
	Activation       Activation
	Canonical        CanonicalStatus
	Alias            AliasStatus
	AliasInstalled   bool
	AlreadyInstalled bool
}

func Install(options Options) (Result, error) {
	version := strings.TrimSpace(options.Version)
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
	canonicalBefore, err := StatusCanonical(layout)
	if err != nil {
		return Result{}, err
	}
	if canonicalBefore.State == CanonicalConflict {
		return Result{}, fmt.Errorf("%w: %s", ErrCanonicalConflict, canonicalBefore.Path)
	}
	aliasBefore := AliasStatus{State: AliasMissing, Path: layout.AliasPath, Target: layout.CurrentBinary}
	if !options.NoAlias {
		aliasBefore, err = StatusAlias(layout)
		if err != nil {
			return Result{}, err
		}
		if aliasBefore.State == AliasConflict {
			return Result{}, fmt.Errorf("%w: %s", ErrAliasConflict, aliasBefore.Path)
		}
	}
	metadataMatches := currentMetadataMatches(layout, version)
	staged, err := Stage(layout, version, resolvedSource)
	if err != nil {
		return Result{}, err
	}
	activation, err := Activate(staged)
	if err != nil {
		return Result{}, err
	}
	result := Result{Layout: layout, Version: version, Source: resolvedSource, Staged: staged, Activation: activation}
	rollback := func(cause error) (Result, error) {
		rollbackErr := Rollback(activation)
		if canonicalBefore.State == CanonicalMissing {
			_, _ = RemoveCanonical(layout)
		}
		if !options.NoAlias && aliasBefore.State == AliasMissing {
			_, _ = RemoveAlias(layout)
		}
		if rollbackErr != nil {
			return Result{}, fmt.Errorf("%w; rollback failed: %v", cause, rollbackErr)
		}
		return Result{}, cause
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
	result.AlreadyInstalled = metadataMatches && staged.Reused && activation.PreviousVersion == version && canonicalBefore.State == CanonicalInstalled && (options.NoAlias || aliasBefore.State == AliasInstalled)
	return result, nil
}

func currentMetadataMatches(layout Layout, version string) bool {
	metadata, err := ReadMetadata(layout.Metadata)
	return err == nil && metadata.Method == MethodDirect && metadata.Version == version && samePath(metadata.InstallDir, layout.Root) && samePath(metadata.BinDir, layout.BinDir)
}

func isDevelopmentVersion(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	return version == "dev" || version == "(devel)" || strings.HasPrefix(version, "dev-")
}
