package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	installpkg "go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func installCommand() *cobra.Command {
	var noAlias, force, noLegacyCleanup bool
	cmd := &cobra.Command{Use: "install", Short: "Install this binary into the managed versioned layout", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		log := commandLogger(cmd)
		logCommandStep(cmd, "INSTALL", "install.plan", "Preparing installation", logger.WithVerbose("version", version.Version), logger.WithDebug("no_alias", noAlias), logger.WithDebug("force", force), logger.WithDebug("migrate_legacy", !noLegacyCleanup))
		result, err := installpkg.Install(installpkg.Options{Context: cmd.Context(), Version: version.Version, NoAlias: noAlias, Force: force, MigrateLegacy: !noLegacyCleanup})
		if err != nil {
			return fmt.Errorf("install managed binary: %w", err)
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
		if err := reconcileCorePluginsAfterInstall(cmd, log, "INSTALL", result); err != nil {
			return err
		}
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
		logCommandStep(cmd, "INSTALL", "install.layout.resolving", "Resolving managed installation layout")
		layout, err := installpkg.DefaultLayout()
		if err != nil {
			return fmt.Errorf("resolve managed installation layout: %w", err)
		}
		logCommandDebug(cmd, "INSTALL", "install.layout.resolved", "Managed installation layout resolved", logger.WithDebug("root", layout.Root))
		source, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve current executable: %w", err)
		}
		log := commandLogger(cmd)
		result, err := installpkg.CleanupLegacyInstallations(installpkg.LegacyCleanupOptions{Context: cmd.Context(), Layout: layout, Source: source, PreserveSource: true})
		if err != nil {
			return fmt.Errorf("clean legacy installations: %w", err)
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

func reconcileCorePluginsAfterInstall(cmd *cobra.Command, log *logger.Logger, component string, result installpkg.Result) error {
	report, recErr := application.ReconcileCorePlugins(cmd.Context())
	logCorePluginReport(log, component, report)
	if err := application.RollbackRequiredCoreFailure(cmd.Context(), result, report); err != nil {
		return err
	}
	if recErr != nil {
		log.Warning(component, "plugin.core.unavailable", "Core plugin catalog unavailable", recErr)
	}
	return nil
}

func logCorePluginReport(log *logger.Logger, component string, report pluginpkg.CoreReconcileReport) {
	if len(report.Items) == 0 {
		log.Notice(component, "plugin.core.none", "No core plugins declared")
		return
	}
	for _, item := range report.Items {
		fields := []logger.Field{logger.With("plugin", string(item.ID)), logger.With("action", string(item.Action))}
		if item.Version != "" {
			fields = append(fields, logger.With("version", string(item.Version)))
		}
		switch item.Action {
		case pluginpkg.CoreActionFailed:
			err := errors.New(item.Error)
			if item.Error == "" {
				err = errors.New("core plugin failed")
			}
			log.Warning(component, "plugin.core.failed", "Core plugin "+string(item.ID)+" failed", err, fields...)
		case pluginpkg.CoreActionSkipped:
			log.Notice(component, "plugin.core.skipped", "Core plugin "+string(item.ID)+" skipped", fields...)
		case pluginpkg.CoreActionRetained:
			log.Notice(component, "plugin.core.retained", "Core plugin "+string(item.ID)+" already installed", fields...)
		default:
			log.Success(component, "core plugin "+string(item.ID)+" "+string(item.Action))
		}
	}
}
