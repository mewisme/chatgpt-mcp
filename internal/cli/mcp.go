package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type upstreamFlags struct {
	name              string
	transport         string
	enabled           bool
	command           string
	args              []string
	env               []string
	cwd               string
	url               string
	headers           []string
	bearerTokenEnvVar string
	authType          string
	authScope         string
	toolPrefix        string
	expose            string
	tools             []string
	disabledTools     []string
	idleTimeout       int
}

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Manage upstream MCP servers"}
	server := &cobra.Command{Use: "server", Short: "Manage configured upstream MCP servers"}
	server.AddCommand(
		mcpServerListCommand(),
		mcpServerAddCommand(),
		mcpServerConfigureCommand(),
		mcpServerShowCommand(),
		mcpServerRemoveCommand(),
		mcpServerToggleCommand(true),
		mcpServerToggleCommand(false),
		mcpServerStatusCommand(),
		mcpServerToolsCommand(),
		mcpServerAuthCommand(),
	)
	cmd.AddCommand(server)
	return cmd
}

func mcpServerListCommand() *cobra.Command {
	var asJSON, refresh bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List configured upstream MCP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			if refresh {
				ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
				defer cancel()
				statuses := manager.ListStatuses(ctx, true)
				if asJSON {
					return printJSON(cmd, statuses)
				}
				log := commandLogger(cmd)
				log.Success("MCP", "upstream status loaded", "count", len(statuses))
				for _, status := range statuses {
					log.Detail(status.ID, fmt.Sprintf("%s enabled=%t health=%s tools=%d expose=%s", status.Transport, status.Enabled, status.Health, status.ToolCount, status.Expose))
				}
				return nil
			}
			servers := manager.List()
			if asJSON {
				views := make([]upstream.Server, len(servers))
				for index, server := range servers {
					views[index] = redactUpstreamServer(server)
				}
				return printJSON(cmd, views)
			}
			log := commandLogger(cmd)
			log.Success("MCP", "upstream servers loaded", "count", len(servers))
			for _, server := range servers {
				endpoint := server.URL
				if server.Transport == "stdio" {
					endpoint = server.Command
				}
				log.Detail(server.ID, fmt.Sprintf("%s enabled=%t expose=%s endpoint=%s", server.Transport, server.Enabled, server.Expose, endpoint))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "connect to each enabled server and refresh health")
	return cmd
}

func mcpServerAddCommand() *cobra.Command {
	var flags upstreamFlags
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add an upstream MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			if _, exists := manager.Get(args[0]); exists {
				return fmt.Errorf("upstream server already exists: %s; use mcp server configure", args[0])
			}
			server := upstream.Server{ID: args[0], Name: args[0], Enabled: true, Expose: "all"}
			server, err = applyUpstreamFlags(cmd, server, flags, true)
			if err != nil {
				return err
			}
			if err := manager.Add(server); err != nil {
				return err
			}
			normalized, _ := manager.Get(args[0])
			log := commandLogger(cmd)
			log.Success("MCP", "upstream server added", "id", normalized.ID)
			log.Detail("transport", normalized.Transport)
			log.Detail("prefix", normalized.ToolPrefix)
			log.Detail("expose", normalized.Expose)
			return nil
		},
	}
	bindUpstreamFlags(cmd, &flags, true)
	return cmd
}

func mcpServerConfigureCommand() *cobra.Command {
	var flags upstreamFlags
	cmd := &cobra.Command{
		Use:               "configure <id>",
		Aliases:           []string{"set"},
		Short:             "Update selected fields on an existing upstream MCP server",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUpstreamID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			server, ok := manager.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			server, err = applyUpstreamFlags(cmd, server, flags, false)
			if err != nil {
				return err
			}
			if err := manager.Add(server); err != nil {
				return err
			}
			commandLogger(cmd).Success("MCP", "upstream server updated", "id", server.ID)
			return nil
		},
	}
	bindUpstreamFlags(cmd, &flags, false)
	return cmd
}

func mcpServerShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "show <id>",
		Short:             "Show one upstream server with secrets redacted",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUpstreamID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			server, ok := manager.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			return printJSON(cmd, redactUpstreamServer(server))
		},
	}
}

func mcpServerRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "remove <id>",
		Short:             "Remove an upstream MCP server",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUpstreamID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			if _, ok := manager.Get(args[0]); !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			if err := manager.Remove(args[0]); err != nil {
				return err
			}
			commandLogger(cmd).Success("MCP", "upstream server removed", "id", args[0])
			return nil
		},
	}
}

func mcpServerToggleCommand(enabled bool) *cobra.Command {
	action := "disable"
	if enabled {
		action = "enable"
	}
	return &cobra.Command{
		Use:               action + " <id>",
		Short:             action + " an upstream MCP server",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUpstreamID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			server, ok := manager.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			server.Enabled = enabled
			if err := manager.Add(server); err != nil {
				return err
			}
			commandLogger(cmd).Success("MCP", action+"d", "id", args[0])
			return nil
		},
	}
}

func mcpServerStatusCommand() *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:               "status <id>",
		Aliases:           []string{"st"},
		Short:             "Check one upstream MCP server",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUpstreamID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			status := manager.CheckHealth(ctx, args[0], refresh)
			return printJSON(cmd, status)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", true, "force a new upstream connection/tool list")
	return cmd
}

func mcpServerToolsCommand() *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:               "tools <id>",
		Short:             "List tools exposed by one upstream MCP server",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeUpstreamID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			values, err := manager.Tools(ctx, args[0], refresh)
			if err != nil {
				return err
			}
			server, _ := manager.Get(args[0])
			proxied := map[string]bool{}
			for _, name := range manager.ProxiedToolNames(server, values) {
				proxied[name] = true
			}
			log := commandLogger(cmd)
			log.Success("MCP", "upstream tools loaded", "count", len(values))
			for _, tool := range values {
				proxy := upstream.ProxyName(server.ToolPrefix, tool.Name)
				state := "hidden"
				if proxied[proxy] {
					state = proxy
				}
				log.Detail(tool.Name, state)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "force a new upstream connection/tool list")
	return cmd
}

func bindUpstreamFlags(cmd *cobra.Command, flags *upstreamFlags, create bool) {
	cmd.Flags().StringVar(&flags.name, "name", "", "display name")
	cmd.Flags().StringVar(&flags.transport, "transport", "", "transport: http or stdio")
	cmd.Flags().BoolVar(&flags.enabled, "enabled", true, "enable server")
	cmd.Flags().StringVar(&flags.command, "command", "", "stdio command")
	cmd.Flags().StringSliceVar(&flags.args, "arg", nil, "stdio command argument")
	cmd.Flags().StringSliceVar(&flags.env, "env", nil, "stdio environment KEY=VALUE")
	cmd.Flags().StringVar(&flags.cwd, "cwd", "", "stdio working directory")
	cmd.Flags().StringVar(&flags.url, "url", "", "HTTP MCP URL")
	cmd.Flags().StringSliceVar(&flags.headers, "header", nil, "HTTP header KEY=VALUE")
	cmd.Flags().StringVar(&flags.bearerTokenEnvVar, "bearer-token-env", "", "environment variable containing HTTP bearer token")
	cmd.Flags().StringVar(&flags.authType, "auth", "", "auth mode: auto, oauth, none")
	cmd.Flags().StringVar(&flags.authScope, "auth-scope", "", "OAuth scope")
	cmd.Flags().StringVar(&flags.toolPrefix, "tool-prefix", "", "dynamic proxy tool prefix")
	cmd.Flags().StringVar(&flags.expose, "expose", "", "none, meta_only, allowlist, or all")
	cmd.Flags().StringSliceVar(&flags.tools, "tool", nil, "allowlisted upstream tool")
	cmd.Flags().StringSliceVar(&flags.disabledTools, "disable-tool", nil, "hidden upstream tool")
	cmd.Flags().IntVar(&flags.idleTimeout, "idle-timeout", 0, "idle timeout in seconds")
	if create {
		_ = cmd.MarkFlagRequired("transport")
	}
}

func applyUpstreamFlags(cmd *cobra.Command, server upstream.Server, flags upstreamFlags, create bool) (upstream.Server, error) {
	changed := func(name string) bool { return create || cmd.Flags().Changed(name) }
	if changed("name") && strings.TrimSpace(flags.name) != "" {
		server.Name = flags.name
	}
	if cmd.Flags().Changed("transport") || create {
		server.Transport = flags.transport
	}
	if cmd.Flags().Changed("enabled") || create {
		server.Enabled = flags.enabled
	}
	if changed("command") {
		server.Command = flags.command
	}
	if cmd.Flags().Changed("arg") {
		server.Args = append([]string(nil), flags.args...)
	}
	if cmd.Flags().Changed("env") {
		env, err := parseAssignments(flags.env, "env")
		if err != nil {
			return upstream.Server{}, err
		}
		server.Env = env
	}
	if changed("cwd") {
		server.CWD = flags.cwd
	}
	if changed("url") {
		server.URL = flags.url
	}
	if cmd.Flags().Changed("header") {
		headers, err := parseAssignments(flags.headers, "header")
		if err != nil {
			return upstream.Server{}, err
		}
		server.Headers = headers
	}
	if changed("bearer-token-env") {
		server.BearerTokenEnvVar = flags.bearerTokenEnvVar
	}
	if changed("auth") {
		server.Auth.Type = flags.authType
	}
	if changed("auth-scope") {
		server.Auth.Scope = flags.authScope
	}
	if changed("tool-prefix") {
		server.ToolPrefix = flags.toolPrefix
	}
	if changed("expose") && strings.TrimSpace(flags.expose) != "" {
		server.Expose = flags.expose
	}
	if cmd.Flags().Changed("tool") {
		server.Tools = append([]string(nil), flags.tools...)
	}
	if cmd.Flags().Changed("disable-tool") {
		server.DisabledTools = append([]string(nil), flags.disabledTools...)
	}
	if cmd.Flags().Changed("idle-timeout") {
		server.IdleTimeoutSec = flags.idleTimeout
	}
	return upstream.NormalizeServer(server)
}

func parseAssignments(values []string, label string) (map[string]string, error) {
	result := map[string]string{}
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s must use KEY=VALUE: %s", label, value)
		}
		result[key] = item
	}
	return result, nil
}

func redactUpstreamServer(server upstream.Server) upstream.Server {
	value := server
	value.Headers = cloneStringMap(server.Headers)
	for key := range value.Headers {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "api-key") || strings.Contains(lower, "cookie") {
			value.Headers[key] = "<redacted>"
		}
	}
	value.Env = cloneStringMap(server.Env)
	for key := range value.Env {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "key") {
			value.Env[key] = "<redacted>"
		}
	}
	return value
}

func cloneStringMap(value map[string]string) map[string]string {
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func loadUpstreamManager() (*upstream.Manager, error) {
	manager := upstream.NewManager(upstream.NewStore(upstream.Path()))
	if err := manager.Load(); err != nil {
		return nil, err
	}
	return manager, nil
}

func printJSON(cmd *cobra.Command, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(string(data))
	return nil
}
