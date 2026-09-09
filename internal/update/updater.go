package update

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"

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
	applySpan := tracepkg.Start(ctx, "UPDATE", "update.apply", "Applying update", tracepkg.String("running_version", options.CurrentVersion), tracepkg.String("requested_target", strings.TrimSpace(options.TargetVersion)), tracepkg.Bool("no_alias", options.NoAlias))
	current := strings.TrimSpace(options.CurrentVersion)
	if isDevelopmentVersion(current) {
		applySpan.FailMessage("Update rejected", ErrDevelopmentUpdate)
		return ApplyResult{}, ErrDevelopmentUpdate
	}
	current, err := NormalizeVersion(current)
	if err != nil {
		applySpan.FailMessage("Update version normalization failed", err)
		return ApplyResult{}, err
	}
	currentSpan := tracepkg.Start(ctx, "UPDATE", "update.current.read", "Reading managed current version", tracepkg.String("metadata", options.Layout.Metadata), tracepkg.String("running_version", current))
	managedCurrent, _, err := install.CurrentVersion(options.Layout)
	if err != nil {
		currentSpan.FailMessage("Managed current version read failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, fmt.Errorf("read managed current version: %w", err)
	}
	managedCurrent, err = NormalizeVersion(managedCurrent)
	if err != nil {
		currentSpan.FailMessage("Managed current version normalization failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, fmt.Errorf("managed current version: %w", err)
	}
	currentSpan.EndMessage("Managed current version read", tracepkg.String("managed_version", managedCurrent), tracepkg.Bool("matches_running", managedCurrent == current))
	if managedCurrent != current {
		err := fmt.Errorf("%w: running %s, current %s", ErrCurrentVersionMismatch, current, managedCurrent)
		applySpan.FailMessage("Update rejected", err)
		return ApplyResult{}, err
	}
	resolver := u.Resolver
	if resolver == nil {
		resolver = Client{}
	}
	targetRequest := strings.TrimSpace(options.TargetVersion)
	var release Release
	resolveSpan := tracepkg.Start(ctx, "UPDATE", "update.target.resolve", "Resolving update target", tracepkg.String("current", current), tracepkg.String("requested_target", targetRequest))
	if targetRequest == "" {
		release, err = resolver.Latest(ctx)
	} else {
		targetRequest, err = NormalizeVersion(targetRequest)
		if err == nil {
			release, err = resolver.Version(ctx, targetRequest)
		}
	}
	if err != nil {
		resolveSpan.FailMessage("Update target resolution failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, err
	}
	target, err := NormalizeVersion(release.Version)
	if err != nil {
		resolveSpan.FailMessage("Update target normalization failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, err
	}
	if targetRequest != "" && target != targetRequest {
		err := fmt.Errorf("resolved release version %s does not match requested version %s", target, targetRequest)
		resolveSpan.FailMessage("Update target resolution failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, err
	}
	comparison, err := CompareVersions(current, target)
	if err != nil {
		resolveSpan.FailMessage("Update target comparison failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, err
	}
	result := ApplyResult{Current: current, Target: target, Downgrade: comparison > 0, Release: release}
	decision := "update"
	if comparison == 0 {
		decision = "current"
	} else if targetRequest == "" && comparison > 0 {
		decision = "ahead"
	} else if comparison > 0 {
		decision = "downgrade"
	}
	resolveSpan.EndMessage("Update target resolved", tracepkg.String("target", target), tracepkg.Int("comparison", comparison), tracepkg.Bool("downgrade", result.Downgrade), tracepkg.String("decision", decision), tracepkg.String("archive", release.ArchiveName))
	if comparison == 0 || targetRequest == "" && comparison > 0 {
		applySpan.EndMessage("Update not required", tracepkg.String("current", current), tracepkg.String("target", target), tracepkg.String("decision", decision), tracepkg.Bool("changed", false))
		return result, nil
	}
	downloader := u.Downloader
	if downloader == nil {
		downloader = Downloader{}
	}
	downloadSpan := tracepkg.Start(ctx, "UPDATE", "update.artifact.download", "Downloading verified release artifact", tracepkg.String("version", target), tracepkg.String("archive", release.ArchiveName))
	artifact, err := downloader.Download(ctx, release)
	if err != nil {
		downloadSpan.FailMessage("Verified release artifact download failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, err
	}
	downloadSpan.EndMessage("Verified release artifact downloaded", tracepkg.String("directory", artifact.Dir), tracepkg.String("binary", artifact.Binary))
	defer func() {
		cleanupSpan := tracepkg.Start(ctx, "UPDATE", "update.artifact.cleanup", "Cleaning update artifact", tracepkg.String("path", artifact.Dir))
		if cleanupErr := artifact.Cleanup(); cleanupErr != nil {
			cleanupSpan.FailMessage("Update artifact cleanup failed", cleanupErr)
		} else {
			cleanupSpan.EndMessage("Update artifact cleaned")
		}
	}()
	installer := u.Install
	if installer == nil {
		installer = install.Install
	}
	installSpan := tracepkg.Start(ctx, "UPDATE", "update.install", "Installing resolved update", tracepkg.String("version", target), tracepkg.String("source", artifact.Binary), tracepkg.String("layout", options.Layout.Root))
	installed, err := installer(install.Options{Context: ctx, Layout: options.Layout, Version: target, Source: artifact.Binary, NoAlias: options.NoAlias})
	if err != nil {
		installSpan.FailMessage("Resolved update installation failed", err)
		applySpan.FailMessage("Update failed", err)
		return ApplyResult{}, fmt.Errorf("install update %s: %w", target, err)
	}
	installSpan.EndMessage("Resolved update installed", tracepkg.String("binary", installed.Staged.Binary), tracepkg.Bool("already_installed", installed.AlreadyInstalled))
	result.Changed = true
	result.Install = installed
	applySpan.EndMessage("Update applied", tracepkg.String("previous", current), tracepkg.String("current", target), tracepkg.Bool("changed", true), tracepkg.Bool("downgrade", result.Downgrade))
	return result, nil
}
