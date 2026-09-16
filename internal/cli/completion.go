package cli

import (
	"net"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type configKeyCompletion struct {
	Key         string
	Description string
	Settable    bool
}

func configKeyCompletions() []configKeyCompletion {
	fields := config.Fields()
	result := make([]configKeyCompletion, 0, len(fields)+1)
	result = append(result, configKeyCompletion{Key: "server.expose", Description: "server network exposure", Settable: true})
	for _, spec := range fields {
		result = append(result, configKeyCompletion{Key: spec.Key, Description: spec.Description, Settable: spec.Editable})
	}
	return result
}

func completeConfigSelection(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]string{}
	for _, spec := range configKeyCompletions() {
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
		completions := configKeyCompletions()
		values := make([]string, 0, len(completions))
		for _, spec := range completions {
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
	if spec, ok := config.FieldByKey(key); ok && spec.Kind == config.FieldEnum {
		return filterCompletions(spec.Options, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	if spec, ok := config.FieldByKey(key); ok && spec.Kind == config.FieldBool {
		return filterCompletions([]string{"true", "false"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	switch key {
	case "server.expose":
		return filterCompletions([]string{"none", "all", "0.0.0.0"}, toComplete), cobra.ShellCompDirectiveNoFileComp
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

func completeWorkspaceID(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return workspaceCompletions(cmd, toComplete)
}

func completeWorkspaceContainerID(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return workspaceContainerCompletions(cmd, toComplete)
}

func completeWorkspaceContainerThenName(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return workspaceContainerCompletions(cmd, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeWorkspaceContainerThenWorkspaces(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return workspaceContainerCompletions(cmd, toComplete)
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

func completeTunnelFilter(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	prepareCompletionConfigRoot(cmd)
	items, err := application.LocalTunnels()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(items)*2)
	for _, item := range items {
		name := ""
		if item.Status.Metadata != nil {
			name = strings.TrimSpace(item.Status.Metadata.Name)
		}
		if name != "" {
			values = append(values, item.ID+"\t"+name, name+"\t"+item.ID)
			continue
		}
		values = append(values, item.ID)
	}
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func workspaceContainerCompletions(cmd *cobra.Command, toComplete string) ([]string, cobra.ShellCompDirective) {
	prepareCompletionConfigRoot(cmd)
	items, err := workspace.NewManager(workspace.DefaultStorePath()).ListContainers()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.ID+"\t"+item.Name)
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

func completeTunnelDispatch(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	prepareCompletionConfigRoot(cmd)
	switch len(args) {
	case 0:
		return completeTunnelProviderNames(toComplete)
	case 1:
		return filterCompletions([]string{"status", "start", "stop"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	case 2:
		if args[1] == "status" || args[1] == "start" || args[1] == "stop" {
			return filterCompletions([]string{"mcp", "admin", "all"}, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeTunnelProviderAction(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return filterCompletions([]string{"status", "start", "stop"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	case 1:
		if args[0] == "status" || args[0] == "start" || args[0] == "stop" {
			return filterCompletions([]string{"mcp", "admin", "all"}, toComplete), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeTunnelProviderNames(toComplete string) ([]string, cobra.ShellCompDirective) {
	values := make([]string, 0)
	for _, name := range application.KnownTunnelProviderNames() {
		values = append(values, name+"\ttunnel provider")
	}
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completePluginID(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return pluginIDCompletions(cmd, toComplete)
}

func completePluginConfigKey(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return pluginIDCompletions(cmd, toComplete)
	}
	if len(args) > 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return pluginConfigKeyCompletions(cmd, args[0], toComplete)
}

func completePluginConfigSet(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return pluginIDCompletions(cmd, toComplete)
	case 1:
		return pluginConfigKeyCompletions(cmd, args[0], toComplete)
	case 2:
		return pluginConfigValueCompletions(cmd, args[0], args[1], toComplete)
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func pluginIDCompletions(cmd *cobra.Command, toComplete string) ([]string, cobra.ShellCompDirective) {
	prepareCompletionConfigRoot(cmd)
	service, err := application.NewPluginService()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items, err := service.Installed()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item.ID))
	}
	sort.Strings(values)
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func pluginConfigKeyCompletions(cmd *cobra.Command, pluginID, toComplete string) ([]string, cobra.ShellCompDirective) {
	schema, _, err := pluginConfigSchema(cmd, pluginID)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		label := field.Title
		if label == "" {
			label = field.Description
		}
		if label == "" {
			values = append(values, field.Key)
			continue
		}
		values = append(values, field.Key+"\t"+label)
	}
	sort.Strings(values)
	return filterCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func pluginConfigValueCompletions(cmd *cobra.Command, pluginID, key, toComplete string) ([]string, cobra.ShellCompDirective) {
	schema, _, err := pluginConfigSchema(cmd, pluginID)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	field, ok := schema.Field(key)
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	switch field.Kind {
	case pluginpkg.FieldBool:
		return filterCompletions([]string{"true", "false"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	case pluginpkg.FieldEnum:
		return filterCompletions(field.Enum, toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func pluginConfigSchema(cmd *cobra.Command, pluginID string) (pluginpkg.SettingsSchema, pluginpkg.PluginID, error) {
	prepareCompletionConfigRoot(cmd)
	id, err := simplePluginID(pluginID)
	if err != nil {
		return pluginpkg.SettingsSchema{}, "", err
	}
	service, err := application.NewPluginService()
	if err != nil {
		return pluginpkg.SettingsSchema{}, "", err
	}
	schema, err := service.Manager.SettingsSchema(id)
	return schema, id, err
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
