package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func aliasCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "alias", Short: "Manage the cgm command alias"}
	cmd.AddCommand(aliasInstallCommand(), aliasRemoveCommand(), aliasStatusCommand())
	return cmd
}

func aliasInstallCommand() *cobra.Command {
	return &cobra.Command{Use: "install", Short: "Install the cgm command alias", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		layout, err := managedAliasLayout()
		if err != nil {
			return err
		}
		status, err := install.InstallAlias(layout)
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("ALIAS", "alias installed")
		log.Detail("path", status.Path)
		log.Detail("target", status.Target)
		return nil
	}}
}

func aliasRemoveCommand() *cobra.Command {
	return &cobra.Command{Use: "remove", Short: "Remove the cgm command alias", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		layout, err := managedAliasLayout()
		if err != nil {
			return err
		}
		before, err := install.StatusAlias(layout)
		if err != nil {
			return err
		}
		status, err := install.RemoveAlias(layout)
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		if before.State == install.AliasMissing {
			log.Notice("ALIAS", "alias.missing", "Alias is not installed")
		} else {
			log.Success("ALIAS", "alias removed")
		}
		log.Detail("path", status.Path)
		return nil
	}}
}

func aliasStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"st"}, Short: "Show the cgm command alias status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		layout, err := managedAliasLayout()
		if err != nil {
			return err
		}
		status, err := install.StatusAlias(layout)
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		switch status.State {
		case install.AliasInstalled:
			log.Ready("ALIAS", "alias.installed", "Alias installed")
		case install.AliasMissing:
			log.Notice("ALIAS", "alias.missing", "Alias not installed")
		case install.AliasConflict:
			log.Warning("ALIAS", "alias.conflict", "Alias path is occupied by another file", nil)
		default:
			return fmt.Errorf("unknown alias state %q", status.State)
		}
		log.Detail("path", status.Path)
		log.Detail("target", status.Target)
		return nil
	}}
}

func managedAliasLayout() (install.Layout, error) {
	detection, err := install.DetectCurrent(version.Version)
	if err != nil {
		return install.Layout{}, err
	}
	layout, err := detection.ManagedLayout()
	if err != nil {
		return install.Layout{}, fmt.Errorf("managed installation not found; run chatgpt-mcp install: %w", err)
	}
	return layout, nil
}
