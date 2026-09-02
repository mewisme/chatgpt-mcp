package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

var root = newRootCommand()

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               cliUseName(),
		Short:             "Workspace-bound local MCP server for ChatGPT",
		RunE:              runServer,
		Version:           version.Short(),
		SilenceErrors:     true,
		SilenceUsage:      true,
		PersistentPreRunE: prepareCommand,
	}
	addExposeFlag(cmd)
	addConfigDirFlag(cmd)
	addLoggingFlags(cmd)
	cmd.AddCommand(
		initCommand(),
		uninitCommand(),
		upCommand(),
		downCommand(),
		restartCommand(),
		logsCommand(),
		configCommand(),
		authCommand(),
		workspaceCommand(),
		mcpCommand(),
		tunnelCommand(),
		serveCommand(),
		statusCommand(),
		internalServiceCommand(),
		&cobra.Command{Use: "version", Short: "Show the chatgpt-mcp version and build information", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, args []string) {
			commandLogger(cmd).Notice("VERSION", "cli.version", version.String())
		}},
	)
	return cmd
}

func cliUseName() string {
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("CHATGPT_MCP_CLI_NAME"))); value == "cgm" {
		return "cgm"
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"))
	if base == "cgm" {
		return "cgm"
	}
	return "chatgpt-mcp"
}

func initCommand() *cobra.Command {
	var force bool
	var formatName string
	var jsonFormat, yamlFormat, tomlFormat bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration and authentication tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := config.Source()
			if err != nil {
				return err
			}
			options := configOutputOptions{format: formatName, json: jsonFormat, yaml: yamlFormat, toml: tomlFormat}
			format, selected, err := resolveConfigOutputFormat(options)
			if err != nil {
				return err
			}
			if !selected {
				format = configformat.JSON
				if source.Exists {
					format = source.Format
				}
			}
			if source.Exists && !force {
				return errors.New("configuration already exists; use --force to rotate tokens and rewrite config")
			}
			if source.Exists && selected && format != source.Format {
				return fmt.Errorf("cannot change storage format with init --force; run chatgpt-mcp config convert %s first", format)
			}
			cfg := config.Default()
			mcpToken := auth.GenerateToken("mcp")
			adminToken := auth.GenerateToken("admin")
			cfg.Auth.MCPTokenHash = auth.HashToken(mcpToken)
			cfg.Auth.AdminTokenHash = auth.HashToken(adminToken)
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if source.Exists {
				if err := config.Save(cfg); err != nil {
					return err
				}
			} else if err := config.SaveAs(cfg, format); err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("INIT", "configuration created")
			if source.Exists {
				log.Detail("config", source.Path)
			} else {
				log.Detail("config", config.PathForFormat(format))
			}
			log.Detail("format", format)
			logEndpointDetails(log, cfg)
			log.Detail("mcp token", mcpToken)
			log.Detail("admin token", adminToken)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "rewrite config and rotate both tokens if already initialized")
	cmd.Flags().StringVar(&formatName, "format", "", "storage format: json, yaml, or toml")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "use JSON storage")
	cmd.Flags().BoolVar(&yamlFormat, "yaml", false, "use YAML storage")
	cmd.Flags().BoolVar(&tomlFormat, "toml", false, "use TOML storage")
	return cmd
}

func uninitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninit",
		Short: "Remove all local chatgpt-mcp configuration and state",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := config.RootPath()
			if err := purgeKeyringSecrets(root); err != nil {
				return err
			}
			if err := removeConfigRoot(root); err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("UNINIT", "local configuration and state removed")
			log.Detail("root", root)
			return nil
		},
	}
}

func purgeKeyringSecrets(root string) error {
	entries, err := config.TunnelKeyringEntries(root)
	if err != nil {
		return err
	}
	clusterEntries, err := config.ClusterKeyringEntries(root)
	if err != nil {
		return err
	}
	oauthEntries, err := mcpoauth.NewStore(configformat.StructuredPath(root, "oauth")).KeyringEntries()
	if err != nil {
		return err
	}
	upstreamEntries, err := upstream.NewStore(configformat.StructuredPath(root, "upstream")).KeyringEntries()
	if err != nil {
		return err
	}
	entries = append(entries, clusterEntries...)
	entries = append(entries, oauthEntries...)
	entries = append(entries, upstreamEntries...)
	changes := make([]secretstore.Change, 0, len(entries))
	for _, entry := range entries {
		changes = append(changes, secretstore.Change{Name: entry})
	}
	return secretstore.New(root).Apply(changes)
}

func removeConfigRoot(root string) error {
	clean := filepath.Clean(root)
	if clean == "." || clean == string(filepath.Separator) {
		return fmt.Errorf("refusing to remove unsafe config root: %s", clean)
	}
	volume := filepath.VolumeName(clean)
	if clean == volume+string(filepath.Separator) {
		return fmt.Errorf("refusing to remove volume root: %s", clean)
	}
	if clean != filepath.Clean(configformat.DefaultRootPath()) && !configformat.IsManagedRoot(clean) {
		return fmt.Errorf("refusing to remove unmanaged config root: %s", clean)
	}
	return os.RemoveAll(clean)
}

func authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage MCP and admin authentication"}
	cmd.AddCommand(
		authKindCommand("mcp"),
		authKindCommand("admin"),
		authStatusCommand(),
	)
	return cmd
}

func authKindCommand(kind string) *cobra.Command {
	cmd := &cobra.Command{Use: kind, Short: "Manage " + kind + " authentication"}
	cmd.AddCommand(authCreateCommand(kind), authToggleCommand(kind, true), authToggleCommand(kind, false))
	return cmd
}

func authCreateCommand(kind string) *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create or rotate the " + kind + " token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			token := auth.GenerateToken(kind)
			hash := auth.HashToken(token)
			if kind == "mcp" {
				cfg.Auth.MCPTokenHash = hash
				cfg.Auth.MCPEnabled = true
			} else {
				cfg.Auth.AdminTokenHash = hash
				cfg.Auth.AdminEnabled = true
				cfg.Admin.Enabled = true
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("AUTH", "token rotated", "type", kind)
			log.Detail(strings.ToUpper(kind), token)
			return nil
		},
	}
}

func authToggleCommand(kind string, enabled bool) *cobra.Command {
	action := "disable"
	if enabled {
		action = "enable"
	}
	return &cobra.Command{
		Use:   action,
		Short: action + " " + kind + " authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if kind == "mcp" {
				if enabled && cfg.Auth.MCPTokenHash == "" {
					return errors.New("MCP token is not configured; run chatgpt-mcp auth mcp create")
				}
				cfg.Auth.MCPEnabled = enabled
			} else {
				if enabled && cfg.Auth.AdminTokenHash == "" {
					return errors.New("admin token is not configured; run chatgpt-mcp auth admin create")
				}
				cfg.Auth.AdminEnabled = enabled
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			commandLogger(cmd).Success("AUTH", state, "type", kind)
			return nil
		},
	}
}

func authStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show authentication state without revealing token hashes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Info("AUTH", "authentication status")
			log.Detail("mcp", fmt.Sprintf("enabled=%t configured=%t", cfg.Auth.MCPEnabled, cfg.Auth.MCPTokenHash != ""))
			log.Detail("admin", fmt.Sprintf("enabled=%t configured=%t", cfg.Auth.AdminEnabled, cfg.Auth.AdminTokenHash != ""))
			return nil
		},
	}
}

func Execute() error {
	err := root.Execute()
	if err != nil {
		commandLogger(root).Failure("CLI", "cli.command.failed", "Command failed", err)
	}
	return err
}
