package cli

import (
	"os"

	"github.com/spf13/cobra"
	installpkg "go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func installCommand() *cobra.Command {
	var noAlias, force, noLegacyCleanup bool
	cmd := &cobra.Command{Use: "install", Short: "Install this binary into the managed versioned layout", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		log := commandLogger(cmd)
		defer log.Close()
		startCommandSpinner(cmd, log, "INSTALL", "install.installing", "Installing chatgpt-mcp")
		result, err := installpkg.Install(installpkg.Options{Version: version.Version, NoAlias: noAlias, Force: force, MigrateLegacy: !noLegacyCleanup})
		if err != nil {
			return err
		}
		if result.AlreadyInstalled {
			log.Notice("INSTALL", "install.already-installed", "Already installed")
		} else {
			log.Success("INSTALL", "binary installed")
		}
		log.Detail("version", result.Version)
		log.Detail("binary", result.Staged.Binary)
		log.Detail("current", result.Layout.CurrentBinary)
		log.Detail("command", result.Canonical.Path)
		if result.AliasInstalled {
			log.Detail("alias", result.Alias.Path)
		} else if noAlias {
			log.Detail("alias", "not installed (--no-alias)")
		}
		logLegacyCleanup(log, result.Legacy)
		return nil
	}}
	cmd.Flags().BoolVar(&noAlias, "no-alias", false, "do not install the cgm command alias")
	cmd.Flags().BoolVar(&force, "force", false, "allow installing a development build")
	cmd.Flags().BoolVar(&noLegacyCleanup, "no-legacy-cleanup", false, "preserve verified legacy standalone installations found in PATH")
	cmd.AddCommand(installCleanupLegacyCommand())
	return cmd
}

func installCleanupLegacyCommand() *cobra.Command {
	return &cobra.Command{Use: "cleanup", Aliases: []string{"migrate"}, Short: "Remove verified legacy standalone installations from PATH", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		layout, err := installpkg.DefaultLayout()
		if err != nil {
			return err
		}
		source, err := os.Executable()
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		defer log.Close()
		startCommandSpinner(cmd, log, "INSTALL", "install.legacy-cleanup", "Cleaning legacy installations")
		result, err := installpkg.CleanupLegacyInstallations(installpkg.LegacyCleanupOptions{Layout: layout, Source: source, PreserveSource: true})
		if err != nil {
			return err
		}
		logLegacyCleanup(log, result)
		return nil
	}}
}

func logLegacyCleanup(log interface {
	Detail(string, any)
	Warn(string, string, ...any)
}, result installpkg.LegacyCleanupResult) {
	for _, legacy := range result.Removed {
		log.Detail("removed legacy", legacy.Path)
	}
	for _, alias := range result.RemovedAliases {
		log.Detail("removed legacy alias", alias)
	}
	for _, legacy := range result.Preserved {
		if legacy.PackageManaged || legacy.Method == installpkg.MethodGo || legacy.Reason == "current executable" {
			log.Detail("preserved", legacy.Path+" ("+legacy.Reason+")")
		}
	}
	for _, failure := range result.Failed {
		log.Warn("INSTALL", "legacy cleanup failed", "path", failure.Path, "error", failure.Err)
	}
}
