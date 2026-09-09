package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func upgradeCommand() *cobra.Command {
	var targetVersion string
	var noRestart bool
	cmd := &cobra.Command{Use: "upgrade", Aliases: []string{"update"}, Short: "Check for and install chatgpt-mcp upgrades", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		logCommandStep(cmd, "UPDATE", "update.installation.detecting", "Detecting current installation")
		detection, err := install.DetectCurrent(version.Version)
		if err != nil {
			return fmt.Errorf("detect current installation: %w", err)
		}
		policy := updatepkg.PolicyForInstallation(detection)
		log := commandLogger(cmd)
		logCommandDebug(cmd, "UPDATE", "update.policy.resolved", "Update policy resolved", logger.WithDebug("method", policy.Method), logger.WithDebug("action", policy.Action))
		if policy.Action == updatepkg.PolicyDelegate {
			log.Notice("UPDATE", "update.delegated", policy.Message)
			log.Detail("method", policy.Method)
			log.Detail("run", policy.Command)
			return nil
		}
		if err := policy.Error(); err != nil {
			return err
		}
		layout, err := detection.ManagedLayout()
		if err != nil {
			return fmt.Errorf("managed direct installation not found: %w", err)
		}
		alias, err := install.StatusAlias(layout)
		if err != nil {
			return fmt.Errorf("inspect cgm alias state: %w", err)
		}
		if alias.State == install.AliasConflict {
			return fmt.Errorf("cannot preserve cgm alias state: %w: %s", install.ErrAliasConflict, alias.Path)
		}
		startCommandSpinner(cmd, log, "UPDATE", "update.updating", "Checking and applying update")
		logCommandStep(cmd, "UPDATE", "update.runtime.inspecting", "Inspecting managed runtime state")
		runtimeState, err := captureUpdateRuntimeState(cmd.Context())
		if err != nil {
			return fmt.Errorf("inspect managed runtime before update: %w", err)
		}
		updater := updatepkg.Updater{
			Resolver:   updatepkg.Client{UserAgent: "chatgpt-mcp/" + version.Version},
			Downloader: updatepkg.Downloader{UserAgent: "chatgpt-mcp/" + version.Version},
		}
		logCommandStep(cmd, "UPDATE", "update.release.applying", "Resolving, downloading, and activating release", logger.WithVerbose("target", targetVersion))
		result, err := updater.Apply(cmd.Context(), updatepkg.ApplyOptions{Layout: layout, CurrentVersion: version.Version, TargetVersion: targetVersion, NoAlias: alias.State == install.AliasMissing})
		if err != nil {
			return fmt.Errorf("apply update: %w", err)
		}
		if targetVersion == "" {
			cacheLatestRelease(cmd, layout, result.Target)
		}
		if !result.Changed {
			if result.Current == result.Target {
				log.Ready("UPDATE", "update.current", "Already up to date")
			} else {
				log.Notice("UPDATE", "update.ahead", "Current version is newer than the latest release")
			}
			log.Detail("current", result.Current)
			log.Detail("latest", result.Target)
			return nil
		}
		log.StopAnimation()
		logCommandStep(cmd, "UPDATE", "update.runtime.coordinating", "Coordinating updated managed runtime", logger.WithVerbose("restart", !noRestart))
		if err := coordinateUpdatedRuntime(cmd, result.Install, runtimeState, noRestart); err != nil {
			return fmt.Errorf("update to %s failed after activation: %w", result.Target, err)
		}
		if err := install.FinalizeResultContext(cmd.Context(), result.Install); err != nil {
			log.Warning("UPDATE", "update.cleanup-failed", "Update succeeded but old version cleanup failed", err)
		}
		message := "updated"
		if result.Downgrade {
			message = "version changed"
		}
		log.Success("UPDATE", message)
		log.Detail("previous", result.Current)
		log.Detail("current", result.Target)
		log.Detail("binary", result.Install.Staged.Binary)
		return nil
	}}
	cmd.Flags().StringVar(&targetVersion, "version", "", "install a specific release version (allows explicit downgrade)")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "do not restart a running managed runtime after updating")
	cmd.AddCommand(upgradeCheckCommand())
	return cmd
}

func upgradeCheckCommand() *cobra.Command {
	return &cobra.Command{Use: "check", Short: "Check the latest available release", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		log := commandLogger(cmd)
		logCommandStep(cmd, "UPDATE", "update.release.checking", "Resolving latest release", logger.WithVerbose("current", version.Version))
		startCommandSpinner(cmd, log, "UPDATE", "update.checking", "Checking for updates")
		checker := updatepkg.Checker{Source: updatepkg.Client{UserAgent: "chatgpt-mcp/" + version.Version}}
		result, err := checker.Check(cmd.Context(), version.Version)
		if err != nil {
			return fmt.Errorf("check latest release: %w", err)
		}
		cacheLatestReleaseForCurrentInstall(cmd, result.Latest)
		switch result.Status {
		case updatepkg.StatusAvailable:
			log.Notice("UPDATE", "update.available", "New version available")
		case updatepkg.StatusUpToDate:
			log.Ready("UPDATE", "update.current", "Already up to date")
		case updatepkg.StatusAhead:
			log.Notice("UPDATE", "update.ahead", "Current version is newer than the latest release")
		case updatepkg.StatusDevelopment:
			log.Notice("UPDATE", "update.development", "Development build; latest release shown for reference")
		default:
			return fmt.Errorf("unknown update status %q", result.Status)
		}
		log.Detail("current", result.Current)
		log.Detail("latest", result.Latest)
		if result.Status == updatepkg.StatusAvailable {
			log.Detail("run", cliUseName()+" upgrade")
		}
		return nil
	}}
}
