package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

const defaultConfigBundleFile = "chatgpt-mcp-config.cgm"

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Aliases: []string{"cfg"}, Short: "Read and update validated runtime configuration"}
	cmd.AddCommand(
		&cobra.Command{Use: "path", Short: "Show the active configuration path, format, and root", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			source, err := config.Source()
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Detail("config", source.Path)
			log.Detail("format", source.Format)
			log.Detail("root", config.RootPath())
			return nil
		}},
		configGetCommand(),
		configListCommand(),
		configSetCommand(),
		configReloadCommand(),
		configMigrateCommand(),
		configConvertCommand(),
		configExportCommand(),
		configImportCommand(),
		configVerifyCommand(),
	)
	return cmd
}

func configExportCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "export [file]",
		Short: "Export portable configuration, state, and secrets into one sealed bundle",
		Long:  "Export portable configuration, state, and secrets into one sealed bundle. The default file is " + defaultConfigBundleFile + " in the current directory.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.ExportConfig(configBundleFile(args), force)
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("CONFIG", "configuration exported", "files", result.Files, "secrets", result.Secrets)
			log.Detail("file", result.Path)
			log.Detail("source", result.Source.OS+"/"+result.Source.Arch)
			if result.SkippedFiles > 0 {
				log.Detail("non-portable/runtime files skipped", result.SkippedFiles)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing export file")
	return cmd
}

func configImportCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import a portable configuration bundle and restore its secrets",
		Long:  "Import a portable configuration bundle and restore its secrets. The default file is " + defaultConfigBundleFile + " in the current directory.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
			defer cancel()
			result, err := application.ImportConfig(ctx, configBundleFile(args), force)
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("CONFIG", "configuration imported", "files", result.Files, "secrets", result.Secrets)
			log.Detail("source", result.Source.OS+"/"+result.Source.Arch)
			log.Detail("target", result.Target.OS+"/"+result.Target.Arch)
			if result.SkippedPaths > 0 {
				log.Detail("unavailable platform paths skipped", result.SkippedPaths)
			}
			if result.SkippedFiles > 0 {
				log.Detail("orphaned workspace state skipped", result.SkippedFiles)
			}
			if result.BackupPath != "" {
				log.Detail("previous config backup retained", result.BackupPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace existing configuration/state")
	return cmd
}

func configBundleFile(args []string) string {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return args[0]
	}
	return defaultConfigBundleFile
}

func configReloadCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload persisted configuration into the running server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log := commandLogger(cmd)
			defer log.Close()
			startCommandSpinner(cmd, log, "CONFIG", "config.reloading", "Reloading configuration")
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			result, err := application.ReloadConfig(ctx)
			if err != nil {
				return err
			}
			log.Success("CONFIG", "configuration reloaded")
			log.Detail("pid", result.PID)
			log.Detail("network restarted", result.NetworkRestarted)
			log.Detail("mcp http", onOff(result.ServerEnabled))
			if result.ServerEnabled {
				log.Detail("mcp port", result.ServerPort)
			}
			if result.AdminEnabled {
				log.Detail("admin port", result.AdminPort)
			} else {
				log.Detail("admin", "disabled")
			}
			log.Detail("expose", result.Exposure)
			return nil
		},
	}
}

func configGetCommand() *cobra.Command {
	options := configOutputOptions{}
	cmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Get a redacted config value or subtree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key := ""
			if len(args) > 0 {
				key = args[0]
			}
			return printConfigSelection(cmd, cfg, key, false, options)
		},
	}
	addConfigOutputFlags(cmd, &options)
	cmd.ValidArgsFunction = completeConfigSelection
	return cmd
}

func configListCommand() *cobra.Command {
	options := configOutputOptions{}
	cmd := &cobra.Command{
		Use:     "list [key]",
		Aliases: []string{"ls"},
		Short:   "List redacted configuration with optional subtree and output format",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			key := ""
			if len(args) > 0 {
				key = args[0]
			}
			return printConfigSelection(cmd, cfg, key, true, options)
		},
	}
	addConfigOutputFlags(cmd, &options)
	cmd.ValidArgsFunction = completeConfigSelection
	return cmd
}

func configSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set one typed configuration value; key=value is also accepted",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, raw, err := parseConfigSetArgs(args)
			if err != nil {
				return err
			}
			if _, err := application.SetConfigField(key, raw); err != nil {
				return err
			}
			commandLogger(cmd).Success("CONFIG", "value saved", "key", key)
			return nil
		},
	}
	cmd.ValidArgsFunction = completeConfigSet
	return cmd
}

func parseConfigSetArgs(args []string) (string, string, error) {
	if len(args) == 2 {
		key := strings.TrimSpace(args[0])
		if key == "" {
			return "", "", errors.New("config key is required")
		}
		return key, args[1], nil
	}
	key, value, ok := strings.Cut(args[0], "=")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", errors.New("use config set <key> <value> or config set key=value")
	}
	return strings.TrimSpace(key), value, nil
}

func setConfigValue(cfg *config.Config, key, raw string) error {
	return config.SetValue(cfg, key, raw)
}

func getConfigValue(cfg config.Config, key string) (any, error) {
	return config.RedactedValueAt(cfg, key)
}

func configMigrateCommand() *cobra.Command {
	return &cobra.Command{Use: "migrate", Short: "Migrate legacy plaintext credentials into the secret file store", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := migrateLegacySecrets(); err != nil {
			return err
		}
		commandLogger(cmd).Success("CONFIG", "credentials migrated to secret file store")
		return nil
	}}
}

func migrateLegacySecrets() error {
	return application.MigrateLegacySecrets()
}

func configConvertCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "convert <json|yaml|toml>",
		Aliases:           []string{"transform"},
		Short:             "Convert all structured chatgpt-mcp config/state files to one format",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeConfigFormat,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := configformat.Parse(args[0])
			if err != nil {
				return err
			}
			converted, err := application.ConvertConfig(format)
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("CONFIG", "configuration format converted", "format", format, "files", converted)
			log.Detail("config", config.PathForFormat(format))
			return nil
		},
	}
}

func configVerifyCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "verify",
		Aliases: []string{"validate"},
		Short:   "Verify structured config/state format consistency and configuration validity",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.VerifyConfig()
			if err != nil {
				return err
			}
			commandLogger(cmd).Success("CONFIG", "configuration verified", "format", result.Format, "files", result.Files)
			return nil
		},
	}
}
