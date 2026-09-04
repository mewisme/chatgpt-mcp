package update

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/install"
)

var (
	ErrDevelopmentUpdate      = errors.New("development builds cannot self-update")
	ErrCurrentVersionMismatch = errors.New("running version does not match managed current version")
)

type Resolver interface {
	Latest(context.Context) (Release, error)
	Version(context.Context, string) (Release, error)
}

type ArtifactSource interface {
	Download(context.Context, Release) (Artifact, error)
}

type Updater struct {
	Resolver   Resolver
	Downloader ArtifactSource
	Install    func(install.Options) (install.Result, error)
}

type ApplyOptions struct {
	Layout         install.Layout
	CurrentVersion string
	TargetVersion  string
	NoAlias        bool
}

type ApplyResult struct {
	Current   string
	Target    string
	Changed   bool
	Downgrade bool
	Release   Release
	Install   install.Result
}

func (u Updater) Apply(ctx context.Context, options ApplyOptions) (ApplyResult, error) {
	current := strings.TrimSpace(options.CurrentVersion)
	if isDevelopmentVersion(current) {
		return ApplyResult{}, ErrDevelopmentUpdate
	}
	current, err := NormalizeVersion(current)
	if err != nil {
		return ApplyResult{}, err
	}
	managedCurrent, _, err := install.CurrentVersion(options.Layout)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("read managed current version: %w", err)
	}
	managedCurrent, err = NormalizeVersion(managedCurrent)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("managed current version: %w", err)
	}
	if managedCurrent != current {
		return ApplyResult{}, fmt.Errorf("%w: running %s, current %s", ErrCurrentVersionMismatch, current, managedCurrent)
	}
	resolver := u.Resolver
	if resolver == nil {
		resolver = Client{}
	}
	targetRequest := strings.TrimSpace(options.TargetVersion)
	var release Release
	if targetRequest == "" {
		release, err = resolver.Latest(ctx)
	} else {
		targetRequest, err = NormalizeVersion(targetRequest)
		if err == nil {
			release, err = resolver.Version(ctx, targetRequest)
		}
	}
	if err != nil {
		return ApplyResult{}, err
	}
	target, err := NormalizeVersion(release.Version)
	if err != nil {
		return ApplyResult{}, err
	}
	if targetRequest != "" && target != targetRequest {
		return ApplyResult{}, fmt.Errorf("resolved release version %s does not match requested version %s", target, targetRequest)
	}
	comparison, err := CompareVersions(current, target)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{Current: current, Target: target, Downgrade: comparison > 0, Release: release}
	if comparison == 0 || targetRequest == "" && comparison > 0 {
		return result, nil
	}
	downloader := u.Downloader
	if downloader == nil {
		downloader = Downloader{}
	}
	artifact, err := downloader.Download(ctx, release)
	if err != nil {
		return ApplyResult{}, err
	}
	defer func() { _ = artifact.Cleanup() }()
	installer := u.Install
	if installer == nil {
		installer = install.Install
	}
	installed, err := installer(install.Options{Layout: options.Layout, Version: target, Source: artifact.Binary, NoAlias: options.NoAlias})
	if err != nil {
		return ApplyResult{}, fmt.Errorf("install update %s: %w", target, err)
	}
	result.Changed = true
	result.Install = installed
	return result, nil
}
