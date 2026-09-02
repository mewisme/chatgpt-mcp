package cli

import (
	"net"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type configKeyCompletion struct {
	Key         string
	Description string
	Settable    bool
}

var configKeyCompletions = []configKeyCompletion{
	{Key: "server.expose", Description: "server network exposure", Settable: true},
	{Key: "server.expose.mode", Description: "exposure mode", Settable: true},
	{Key: "server.expose.interfaces", Description: "exposed network interfaces", Settable: true},
	{Key: "server.port", Description: "MCP server port", Settable: true},
	{Key: "server.allow_insecure_http", Description: "allow authenticated HTTP beyond loopback", Settable: true},
	{Key: "admin.enabled", Description: "admin server enabled", Settable: true},
	{Key: "admin.port", Description: "admin server port", Settable: true},
	{Key: "auth.mcp_enabled", Description: "MCP authentication enabled", Settable: true},
	{Key: "auth.admin_enabled", Description: "admin authentication enabled", Settable: true},
	{Key: "auth.mcp_token_hash", Description: "MCP token hash (read-only)"},
	{Key: "auth.admin_token_hash", Description: "admin token hash (read-only)"},
	{Key: "cluster.enabled", Description: "cluster federation enabled", Settable: true},
	{Key: "cluster.relay_url", Description: "cluster WebSocket relay URL", Settable: true},
	{Key: "cluster.relay_token", Description: "cluster relay token", Settable: true},
	{Key: "cluster.relay_token_configured", Description: "cluster relay token configured (read-only)"},
	{Key: "permissions.allow_dirs", Description: "additional filesystem roots", Settable: true},
	{Key: "shell.path", Description: "additional executable search paths", Settable: true},
	{Key: "features.ponytail.enabled", Description: "Ponytail feature enabled", Settable: true},
	{Key: "features.caveman.enabled", Description: "Caveman feature enabled", Settable: true},
	{Key: "tunnel.enabled", Description: "OpenAI tunnel enabled", Settable: true},
	{Key: "tunnel.id", Description: "OpenAI tunnel ID", Settable: true},
	{Key: "tunnel.api_key", Description: "OpenAI tunnel runtime API key", Settable: true},
	{Key: "tunnel.admin_key", Description: "OpenAI tunnel admin key (manage with tunnel admin key)"},
	{Key: "tunnel.admin_organization_id", Description: "verified admin organization scope (read-only)"},
	{Key: "tunnel.admin_workspace_id", Description: "verified admin workspace scope (read-only)"},
	{Key: "tunnel.admin_tenant_id", Description: "verified admin tenant scope (read-only)"},
	{Key: "tunnel.control_plane_base_url", Description: "OpenAI tunnel control-plane URL", Settable: true},
	{Key: "tunnel.organization_id", Description: "OpenAI organization ID", Settable: true},
}

func completeConfigSelection(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]string{}
	for _, spec := range configKeyCompletions {
		seen[spec.Key] = spec.Description
		parts := strings.Split(spec.Key, ".")
		for index := 1; index < len(parts); index++ {
			prefix := strings.Join(parts[:index], ".")
			if _, ok := seen[prefix]; !ok {
				seen[prefix] = "configuration subtree"
			}
		}
	}
	values := make([]string, 0, len(seen))
	for key, description := range seen {
		values = append(values, key+"\t"+description)
	}
	sort.Strings(values)
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeConfigSet(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		values := make([]string, 0, len(configKeyCompletions))
		for _, spec := range configKeyCompletions {
			if spec.Settable {
				values = append(values, spec.Key+"\t"+spec.Description)
			}
		}
		sort.Strings(values)
		return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) > 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	key := args[0]
	switch key {
	case "server.allow_insecure_http", "admin.enabled", "auth.mcp_enabled", "auth.admin_enabled", "cluster.enabled", "features.ponytail.enabled", "features.caveman.enabled", "tunnel.enabled":
		return filterCompletions([]string{"true", "false"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	case "server.expose":
		return filterCompletions([]string{"none", "all", "0.0.0.0"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	case "server.expose.mode":
		return filterCompletions([]string{"none", "all", "0.0.0.0", "interfaces"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	case "server.expose.interfaces":
		interfaces, err := net.Interfaces()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		values := make([]string, 0, len(interfaces))
		for _, item := range interfaces {
			values = append(values, item.Name)
		}
		sort.Strings(values)
		return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
	case "permissions.allow_dirs", "shell.path":
		return nil, cobra.ShellCompDirectiveFilterDirs
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeConfigFormat(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletions([]string{"json", "yaml", "toml"}, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completePresetName(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletions(config.PresetNames(), toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeWorkspaceID(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return workspaceCompletions(cmd, toComplete)
}

func completeWorkspaceThenDirectory(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return workspaceCompletions(cmd, toComplete)
	}
	if len(args) == 1 {
		return nil, cobra.ShellCompDirectiveFilterDirs
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeDirectory(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func workspaceCompletions(cmd *cobra.Command, toComplete string) ([]string, cobra.ShellCompDirective) {
	prepareCompletionConfigRoot(cmd)
	items, err := workspace.NewManager(workspace.DefaultStorePath()).List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.ID+"\t"+item.Path)
	}
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeUpstreamID(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	prepareCompletionConfigRoot(cmd)
	manager, err := loadUpstreamManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	servers := manager.List()
	values := make([]string, 0, len(servers))
	for _, server := range servers {
		description := server.Name
		if strings.TrimSpace(description) == "" {
			description = server.Transport
		}
		values = append(values, server.ID+"\t"+description)
	}
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeSessionID(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	prepareCompletionConfigRoot(cmd)
	events, err := runtimeevent.Read(config.RootPath(), runtimeevent.Query{Tail: 500})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	values := []string{}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.RunID == "" || seen[event.RunID] {
			continue
		}
		seen[event.RunID] = true
		description := event.Time.Local().Format("2006-01-02 15:04:05")
		if event.Managed {
			description += " managed/" + event.ServiceScope
		} else {
			description += " foreground"
		}
		values = append(values, event.RunID+"\t"+description)
		if len(values) >= 20 {
			break
		}
	}
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeStatic(values ...string) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func prepareCompletionConfigRoot(cmd *cobra.Command) {
	_ = configureConfigDir(cmd)
}

func filterCompletions(values []string, prefix string) []string {
	if prefix == "" {
		return values
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		candidate, _, _ := strings.Cut(value, "\t")
		if strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix)) {
			result = append(result, value)
		}
	}
	return result
}
