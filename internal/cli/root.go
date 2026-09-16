package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

const mcpAuthReuseHelp = "Protects direct connections to /mcp. Secure MCP Tunnel uses separate tunnel credentials and is unaffected.\n\nReuse the Direct MCP HTTP token when adding this MCP server to ChatGPT. You do not need to generate a new token for each connection."

var clipboardWriteAll = clipboard.WriteAll

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
		installCommand(),
		upgradeCommand(),
		initCommand(),
		uninitCommand(),
		upCommand(),
		downCommand(),
		restartCommand(),
		logsCommand(),
		requestCommand(),
		tuiCommand(),
		configCommand(),
		aliasCommand(),
		authCommand(),
		workspaceCommand(),
		upstreamCommand(),
		mcpCommand(),
		pluginCommand(),
		tunnelCommand(),
		serveCommand(),
		statusCommand(),
		doctorCommand(),
		completionCommand(),
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
			logCommandStep(cmd, "INIT", "init.preparing", "Preparing configuration initialization")
			options := configOutputOptions{format: formatName, json: jsonFormat, yaml: yamlFormat, toml: tomlFormat}
			format, selected, err := resolveConfigOutputFormat(options)
			if err != nil {
				return err
			}
			logCommandDebug(cmd, "INIT", "init.format.resolved", "Configuration format resolved", logger.WithDebug("format", format), logger.WithDebug("selected", selected), logger.WithDebug("force", force))
			result, err := application.Initialize(application.InitOptions{Context: cmd.Context(), Force: force, Format: format, FormatSelected: selected})
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("INIT", "configuration created")
			log.Detail("config", result.ConfigPath)
			log.Detail("format", result.Format)
			logEndpointDetails(log, result.Config)
			log.Secret("Direct MCP HTTP token", result.MCPToken)
			log.Secret("admin token", result.AdminToken)
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
			logCommandStep(cmd, "UNINIT", "uninit.removing", "Removing local configuration and state", logger.WithVerbose("root", root))
			if err := application.UninitializeContext(cmd.Context(), root); err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("UNINIT", "local configuration and state removed")
			log.Detail("root", root)
			return nil
		},
	}
}

func purgeStoredSecrets(root string) error { return application.PurgeStoredSecrets(root) }
func removeConfigRoot(root string) error   { return application.RemoveConfigRoot(root) }

func authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage Direct MCP HTTP and admin authentication"}
	cmd.AddCommand(
		authKindCommand("mcp"),
		authKindCommand("admin"),
		authStatusCommand(),
	)
	return cmd
}

func authKindCommand(kind string) *cobra.Command {
	cmd := &cobra.Command{Use: kind, Short: "Manage " + kind + " authentication"}
	if kind == "mcp" {
		cmd.Short = "Manage Direct MCP HTTP authentication"
		cmd.Long = mcpAuthReuseHelp
		cmd.AddCommand(
			authMCPStatusCommand(),
			authMCPShowCommand(),
			authMCPCopyCommand(),
			authRotateCommand(kind),
			authDeprecatedCreateCommand(kind),
			authToggleCommand(kind, true),
			authToggleCommand(kind, false),
		)
		return cmd
	}
	cmd.AddCommand(authCreateCommand(kind), authToggleCommand(kind, true), authToggleCommand(kind, false))
	return cmd
}

func authRotateCommand(kind string) *cobra.Command {
	return authTokenRotateCommand(kind, "rotate", "Rotate the Direct MCP HTTP token and print the replacement")
}

func authDeprecatedCreateCommand(kind string) *cobra.Command {
	cmd := authTokenRotateCommand(kind, "create", "Deprecated alias for rotate")
	cmd.Deprecated = `use "cgm auth mcp rotate"`
	return cmd
}

func authCreateCommand(kind string) *cobra.Command {
	return authTokenRotateCommand(kind, "create", "Create or rotate the "+kind+" token")
}

func authTokenRotateCommand(kind, use, short string) *cobra.Command {
	label := strings.ToUpper(kind) + " token"
	long := ""
	if kind == "mcp" {
		label = "Direct MCP HTTP token"
		long = mcpAuthReuseHelp + "\n\nRotation invalidates the previous token immediately. Do not rotate just to add this server to ChatGPT again."
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "AUTH", "auth.token.rotating", "Creating or rotating authentication token", logger.WithVerbose("type", kind))
			token, _, err := application.RotateAuthToken(cmd.Context(), kind)
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("AUTH", "token rotated", "type", kind)
			log.Secret(label, token)
			return nil
		},
	}
}

func authMCPStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show Direct MCP HTTP authentication state without revealing the token",
		Long:    mcpAuthReuseHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "AUTH", "auth.status.loading", "Loading authentication state")
			status, err := application.GetAuthStatusContext(cmd.Context())
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Info("AUTH", "Direct MCP HTTP authentication protects /mcp only; Secure MCP Tunnel is unaffected")
			log.Info("AUTH", "Reuse this token when adding this MCP server to ChatGPT. You do not need to generate a new token for each connection.")
			log.Detail("mcp", fmt.Sprintf("enabled=%t configured=%t revealable=%t legacy_bearer=%t", status.MCPEnabled, status.MCPConfigured, status.MCPRevealable, status.MCPLegacyBearer))
			return nil
		},
	}
}

func authMCPShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the stored Direct MCP HTTP token",
		Long:  mcpAuthReuseHelp + "\n\nPrints the current token. If only a legacy hash exists, rotate once first.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "AUTH", "auth.token.showing", "Revealing Direct MCP HTTP token")
			token, err := application.RevealMCPToken()
			if err != nil {
				return err
			}
			commandLogger(cmd).Secret("Direct MCP HTTP token", token)
			return nil
		},
	}
}

func authMCPCopyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "copy",
		Short: "Copy the stored Direct MCP HTTP token to the clipboard",
		Long:  mcpAuthReuseHelp + "\n\nCopies the current token without rotating it. If only a legacy hash exists, rotate once first.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "AUTH", "auth.token.copying", "Copying Direct MCP HTTP token")
			token, err := application.RevealMCPToken()
			if err != nil {
				return err
			}
			if err := copyClipboard(token); err != nil {
				return err
			}
			commandLogger(cmd).Success("AUTH", "Direct MCP HTTP token copied")
			return nil
		},
	}
}

func copyClipboard(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("nothing to copy")
	}
	return clipboardWriteAll(value)
}

func authToggleCommand(kind string, enabled bool) *cobra.Command {
	action := "disable"
	if enabled {
		action = "enable"
	}
	short := action + " " + kind + " authentication"
	long := ""
	if kind == "mcp" {
		short = action + " Direct MCP HTTP authentication"
		long = mcpAuthReuseHelp + "\n\nAffects direct /mcp HTTP authentication only."
	}
	return &cobra.Command{
		Use:   action,
		Short: short,
		Long:  long,
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "AUTH", "auth.state.updating", "Updating authentication state", logger.WithVerbose("type", kind), logger.WithVerbose("enabled", enabled))
			if _, err := application.SetAuthEnabled(cmd.Context(), kind, enabled); err != nil {
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
		Short:   "Show Direct MCP HTTP and admin authentication state without revealing tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "AUTH", "auth.status.loading", "Loading authentication state")
			status, err := application.GetAuthStatusContext(cmd.Context())
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Info("AUTH", "Direct MCP HTTP authentication protects /mcp only; Secure MCP Tunnel is unaffected")
			log.Detail("mcp", fmt.Sprintf("enabled=%t configured=%t revealable=%t legacy_bearer=%t", status.MCPEnabled, status.MCPConfigured, status.MCPRevealable, status.MCPLegacyBearer))
			log.Detail("admin", fmt.Sprintf("enabled=%t configured=%t", status.AdminEnabled, status.AdminConfigured))
			return nil
		},
	}
}

func Execute() error {
	return executeCommand(root)
}

func executeCommand(command *cobra.Command) error {
	started := time.Now()
	executed, err := command.ExecuteC()
	if executed == nil {
		executed = command
	}
	if err != nil {
		logCommandFailure(executed, err, started)
		closeCommandLogger(executed)
		return err
	}
	logCommandCompleted(executed, started)
	closeCommandLogger(executed)
	return nil
}
