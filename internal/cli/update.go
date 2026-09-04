package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/install"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func updateCommand() *cobra.Command {
	var targetVersion string
	cmd := &cobra.Command{Use: "update", Short: "Check for and install chatgpt-mcp updates", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		detection, err := install.DetectCurrent(version.Version)
		if err != nil {
			return err
		}
		layout, err := detection.ManagedLayout()
		if err != nil {
			return fmt.Errorf("managed direct installation not found: %w", err)
		}
		alias, err := install.StatusAlias(layout)
		if err != nil {
			return err
		}
		if alias.State == install.AliasConflict {
			return fmt.Errorf("cannot preserve cgm alias state: %w: %s", install.ErrAliasConflict, alias.Path)
		}
		updater := updatepkg.Updater{
			Resolver:   updatepkg.Client{UserAgent: "chatgpt-mcp/" + version.Version},
			Downloader: updatepkg.Downloader{UserAgent: "chatgpt-mcp/" + version.Version},
		}
		result, err := updater.Apply(cmd.Context(), updatepkg.ApplyOptions{Layout: layout, CurrentVersion: version.Version, TargetVersion: targetVersion, NoAlias: alias.State == install.AliasMissing})
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
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
	cmd.AddCommand(updateCheckCommand())
	return cmd
}

func updateCheckCommand() *cobra.Command {
	return &cobra.Command{Use: "check", Short: "Check the latest available release", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		checker := updatepkg.Checker{Source: updatepkg.Client{UserAgent: "chatgpt-mcp/" + version.Version}}
		result, err := checker.Check(cmd.Context(), version.Version)
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
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
			log.Detail("run", cliUseName()+" update")
		}
		return nil
	}}
}
