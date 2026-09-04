package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func updateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "update", Short: "Check for and install chatgpt-mcp updates"}
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
