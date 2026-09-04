package cli

import (
	"github.com/spf13/cobra"
	installpkg "go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func installCommand() *cobra.Command {
	var noAlias, force bool
	cmd := &cobra.Command{Use: "install", Short: "Install this binary into the managed versioned layout", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		log := commandLogger(cmd)
		defer log.Close()
		startCommandSpinner(cmd, log, "INSTALL", "install.installing", "Installing chatgpt-mcp")
		result, err := installpkg.Install(installpkg.Options{Version: version.Version, NoAlias: noAlias, Force: force})
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
		return nil
	}}
	cmd.Flags().BoolVar(&noAlias, "no-alias", false, "do not install the cgm command alias")
	cmd.Flags().BoolVar(&force, "force", false, "allow installing a development build")
	return cmd
}
