package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"go.mewis.me/chatgpt-mcp/internal/application"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/pluginhost"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/version"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func pluginCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "plugin", Short: "Manage signed ChatGPT MCP plugins", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(pluginSearchCommand(), pluginInfoCommand(), pluginListCommand(), pluginInstallCommand(), pluginUninstallCommand(), pluginToggleCommand(true), pluginToggleCommand(false), pluginUpdateCommand(), pluginRollbackCommand(), pluginPruneCommand(), pluginOutdatedCommand(), pluginVerifyCommand(), pluginConfigCommand(), pluginRegistryCommand())
	return cmd
}

func newPluginManager() (*pluginpkg.Manager, pluginpkg.Layout, error) {
	return newPluginManagerFor(pluginpkg.DefaultLayout())
}

func newPluginManagerFromCmd(cmd *cobra.Command) (*pluginpkg.Manager, pluginpkg.Layout, error) {
	layout, err := application.ResolvePluginLayout(pluginScopeOptions(cmd))
	if err != nil {
		return nil, pluginpkg.Layout{}, err
	}
	return newPluginManagerFor(layout)
}

func newPluginManagerFor(layout pluginpkg.Layout) (*pluginpkg.Manager, pluginpkg.Layout, error) {
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{CoreVersion: version.Version})
	if err != nil {
		return nil, pluginpkg.Layout{}, err
	}
	pluginhost.Attach(store)
	if err := application.AttachPluginPeers(store); err != nil {
		return nil, pluginpkg.Layout{}, err
	}
	client := pluginpkg.RegistryClient{Layout: layout, UserAgent: "chatgpt-mcp/" + version.Version, LocalDir: pluginpkg.DefaultPluginBundleDir()}
	return &pluginpkg.Manager{Store: store, RegistryClient: client}, layout, nil
}

func addPluginScopeFlags(cmd *cobra.Command) {
	cmd.Flags().String("scope", "", "plugin scope (global|workspace)")
	cmd.Flags().String("workspace", "", "workspace id or path (implies --scope workspace)")
}

func pluginScopeOptions(cmd *cobra.Command) application.PluginScopeOptions {
	scope, _ := cmd.Flags().GetString("scope")
	workspaceRef, _ := cmd.Flags().GetString("workspace")
	return application.PluginScopeOptions{Scope: scope, Workspace: workspaceRef}
}

func formatPluginScopes(scopes []pluginpkg.PluginScope) string {
	parts := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		parts = append(parts, string(scope))
	}
	return strings.Join(parts, ", ")
}

func pluginSearchCommand() *cobra.Command {
	return &cobra.Command{Use: "search [query]", Short: "Search configured plugin registries", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
		if err != nil {
			return err
		}
		query := ""
		if len(args) > 0 {
			query = strings.ToLower(strings.TrimSpace(args[0]))
		}
		snapshots, err := manager.Snapshots(cmd.Context())
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		count := 0
		for _, snapshot := range snapshots {
			ids := sortedRegistryPluginIDs(snapshot.Index.Plugins)
			for _, id := range ids {
				entry := snapshot.Index.Plugins[id]
				haystack := strings.ToLower(string(id) + " " + entry.Name + " " + entry.Description)
				if query != "" && !strings.Contains(haystack, query) {
					continue
				}
				if _, ok := manager.LookupBuiltin(id); ok {
					continue
				}
				log.Detail(snapshot.Registry.Name+"/"+string(id), fmt.Sprintf("%s  %s", entry.Stable, entry.Description))
				count++
			}
		}
		if count == 0 {
			log.Notice("PLUGIN", "plugin.search.empty", "No plugins matched")
		}
		return nil
	}}
}

func pluginInfoCommand() *cobra.Command {
	return &cobra.Command{Use: "info <plugin>[@version]", Short: "Show plugin registry metadata", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
		if err != nil {
			return err
		}
		_, id, _, err := pluginpkg.ParseReference(args[0])
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		if builtin, ok := manager.LookupBuiltin(id); ok {
			log.Detail("id", string(builtin.ID))
			log.Detail("name", builtin.Name)
			log.Detail("origin", pluginpkg.OriginBuiltin.Label())
			log.Detail("version", version.Version)
			log.Detail("type", string(builtin.Type))
			log.Detail("scopes", formatPluginScopes(builtin.AllowedScopes()))
			log.Detail("description", builtin.Description)
			return nil
		}
		resolved, err := manager.Resolve(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		log.Detail("id", resolved.Registry.Name+"/"+string(resolved.PluginID))
		log.Detail("name", resolved.Entry.Name)
		log.Detail("version", resolved.Version)
		log.Detail("publisher", resolved.Publisher.Name)
		log.Detail("type", resolved.Entry.Type)
		log.Detail("scopes", formatPluginScopes(resolved.Entry.AllowedScopes()))
		log.Detail("description", resolved.Entry.Description)
		return nil
	}}
}

func pluginListCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List installed plugins", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		items, err := manager.Catalog()
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		for _, item := range items {
			state := "disabled"
			if item.Enabled {
				state = "enabled"
			}
			log.Detail(string(item.ID), fmt.Sprintf("%s  %s  %s", item.Version, item.Origin.Label(), state))
		}
		if len(items) == 0 {
			log.Notice("PLUGIN", "plugin.list.empty", "No plugins installed")
		}
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginInstallCommand() *cobra.Command {
	var portable bool
	cmd := &cobra.Command{Use: "install <plugin>[@version]", Short: "Install and enable a signed plugin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := resolvePluginInstallOptions(cmd, args[0])
		if err != nil {
			return err
		}
		layout, err := application.ResolvePluginLayout(opts)
		if err != nil {
			return err
		}
		manager, _, err := newPluginManagerFor(layout)
		if err != nil {
			return err
		}
		var result pluginpkg.InstallResult
		if portable {
			result, err = manager.InstallWithOptions(cmd.Context(), args[0], pluginpkg.InstallOptions{HostInstall: pluginpkg.HostInstallPortable})
		} else {
			result, err = manager.Install(cmd.Context(), args[0])
			if err != nil {
				result, err = promptPluginHostInstall(cmd, manager, args[0], err)
			}
		}
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("PLUGIN", "plugin installed", "id", result.Plugin.Manifest.ID, "version", result.Plugin.Manifest.Version)
		log.Detail("registry", result.Registry.Name)
		log.Detail("publisher", result.Publisher.Name)
		return nil
	}}
	cmd.Flags().BoolVar(&portable, "portable", false, "install a manifest-declared portable host dependency into plugin data")
	addPluginScopeFlags(cmd)
	return cmd
}

type pluginInstallScopeChoice struct {
	opts  application.PluginScopeOptions
	label string
}

func resolvePluginInstallOptions(cmd *cobra.Command, reference string) (application.PluginScopeOptions, error) {
	opts := pluginScopeOptions(cmd)
	if strings.TrimSpace(opts.Scope) != "" || strings.TrimSpace(opts.Workspace) != "" {
		return opts, nil
	}
	manager, _, err := newPluginManager()
	if err != nil {
		return application.PluginScopeOptions{}, err
	}
	resolved, err := manager.Resolve(cmd.Context(), reference)
	if err != nil {
		return application.PluginScopeOptions{}, err
	}
	filled, err := application.ApplyPluginInstallScope(opts, resolved.Entry.AllowedScopes())
	if err == nil {
		return filled, nil
	}
	if !errors.Is(err, application.ErrPluginScopeRequired) || !cliStdinIsTerminal(cmd) {
		return application.PluginScopeOptions{}, err
	}
	items, listErr := workspace.NewManager(workspace.DefaultStorePath()).List()
	if listErr != nil {
		return application.PluginScopeOptions{}, listErr
	}
	available := make([]workspace.Workspace, 0, len(items))
	for _, item := range items {
		if item.Available() {
			available = append(available, item)
		}
	}
	return promptPluginInstallScope(cmd, resolved.Entry.AllowedScopes(), available)
}

func promptPluginInstallScope(cmd *cobra.Command, allowed []pluginpkg.PluginScope, workspaces []workspace.Workspace) (application.PluginScopeOptions, error) {
	choices := make([]pluginInstallScopeChoice, 0, 1+len(workspaces))
	for _, scope := range allowed {
		switch scope {
		case pluginpkg.ScopeGlobal:
			choices = append(choices, pluginInstallScopeChoice{opts: application.PluginScopeOptions{Scope: string(pluginpkg.ScopeGlobal)}, label: "Global"})
		case pluginpkg.ScopeWorkspace:
			if len(workspaces) == 0 {
				continue
			}
			for _, item := range workspaces {
				choices = append(choices, pluginInstallScopeChoice{
					opts:  application.PluginScopeOptions{Scope: string(pluginpkg.ScopeWorkspace), Workspace: item.ID},
					label: "Workspace: " + filepath.Base(item.Path),
				})
			}
		}
	}
	if len(choices) == 0 {
		return application.PluginScopeOptions{}, application.ErrPluginScopeRequired
	}
	writer := cmd.OutOrStdout()
	fmt.Fprintln(writer, "Install scope")
	for index, choice := range choices {
		fmt.Fprintf(writer, "  %d. %s\n", index+1, choice.label)
	}
	fmt.Fprint(writer, "Selection [Enter to cancel]: ")
	line, readErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if readErr != nil && strings.TrimSpace(line) == "" {
		return application.PluginScopeOptions{}, application.ErrPluginScopeRequired
	}
	selection := strings.TrimSpace(line)
	if selection == "" {
		return application.PluginScopeOptions{}, application.ErrPluginScopeRequired
	}
	index, parseErr := strconv.Atoi(selection)
	if parseErr != nil || index < 1 || index > len(choices) {
		return application.PluginScopeOptions{}, fmt.Errorf("invalid install scope selection %q", selection)
	}
	return choices[index-1].opts, nil
}

func cliStdinIsTerminal(cmd *cobra.Command) bool {
	input, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(input.Fd()))
}

type pluginHostInstallChoice struct {
	portable bool
	hint     pluginpkg.HostInstallHint
	label    string
}

func promptPluginHostInstall(cmd *cobra.Command, manager *pluginpkg.Manager, reference string, installErr error) (pluginpkg.InstallResult, error) {
	var prerequisite *pluginpkg.HostPrerequisiteError
	if !errors.As(installErr, &prerequisite) {
		return pluginpkg.InstallResult{}, installErr
	}
	input, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return pluginpkg.InstallResult{}, installErr
	}
	choices := make([]pluginHostInstallChoice, 0, len(prerequisite.Install)+1)
	if prerequisite.Portable != nil {
		choices = append(choices, pluginHostInstallChoice{portable: true, label: "Install verified portable binary locally in plugin data"})
	}
	for _, hint := range prerequisite.Install {
		if hint.Runnable() {
			choices = append(choices, pluginHostInstallChoice{hint: hint, label: "Install globally: " + hint.DisplayCommand()})
		}
	}
	if len(choices) == 0 {
		return pluginpkg.InstallResult{}, installErr
	}
	writer := cmd.OutOrStdout()
	fmt.Fprintln(writer, prerequisite.Reason)
	fmt.Fprintln(writer, "Choose how to install the required host dependency:")
	for index, choice := range choices {
		fmt.Fprintf(writer, "  %d. %s\n", index+1, choice.label)
	}
	for _, hint := range prerequisite.Install {
		if !hint.Runnable() {
			fmt.Fprintf(writer, "  - Manual: %s\n", hint.DisplayCommand())
		}
	}
	fmt.Fprint(writer, "Selection [Enter to cancel]: ")
	line, readErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if readErr != nil && strings.TrimSpace(line) == "" {
		return pluginpkg.InstallResult{}, installErr
	}
	selection := strings.TrimSpace(line)
	if selection == "" {
		return pluginpkg.InstallResult{}, installErr
	}
	index, parseErr := strconv.Atoi(selection)
	if parseErr != nil || index < 1 || index > len(choices) {
		return pluginpkg.InstallResult{}, fmt.Errorf("invalid host install selection %q", selection)
	}
	choice := choices[index-1]
	if choice.portable {
		return manager.InstallWithOptions(cmd.Context(), reference, pluginpkg.InstallOptions{HostInstall: pluginpkg.HostInstallPortable})
	}
	fmt.Fprintf(writer, "Running: %s\n", choice.hint.DisplayCommand())
	if _, err := pluginpkg.RunHostInstallHint(cmd.Context(), choice.hint); err != nil {
		return pluginpkg.InstallResult{}, err
	}
	return manager.Install(cmd.Context(), reference)
}

func pluginUninstallCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{Use: "uninstall <plugin>", Short: "Disable and uninstall the active plugin version", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		id, err := simplePluginID(args[0])
		if err != nil {
			return err
		}
		if err := manager.Uninstall(cmd.Context(), id, force); err != nil {
			return err
		}
		commandLogger(cmd).Success("PLUGIN", "plugin uninstalled", "id", id)
		return nil
	}}
	cmd.Flags().BoolVar(&force, "force", false, "uninstall even when active plugins depend on provided capabilities")
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginToggleCommand(enabled bool) *cobra.Command {
	action := "disable"
	traceName := "plugin.disable"
	if enabled {
		action = "enable"
		traceName = "plugin.enable"
	}
	cmd := &cobra.Command{Use: action + " <plugin>", Short: action + " an installed plugin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		id, err := simplePluginID(args[0])
		if err != nil {
			return err
		}
		span := tracepkg.Start(cmd.Context(), "PLUGIN", traceName, action+" plugin", tracepkg.String("plugin_id", string(id)))
		if err := manager.Store.SetEnabled(id, enabled); err != nil {
			span.Fail(err)
			return err
		}
		span.End()
		commandLogger(cmd).Success("PLUGIN", "plugin "+action+"d", "id", id)
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginUpdateCommand() *cobra.Command {
	var all bool
	cmd := &cobra.Command{Use: "update [plugin]", Short: "Update installed plugins to registry stable versions", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		if all {
			if len(args) != 0 {
				return fmt.Errorf("plugin update --all does not accept a plugin argument")
			}
			outdated, err := manager.Outdated(cmd.Context())
			if err != nil {
				return err
			}
			for _, item := range outdated {
				if _, err := manager.Update(cmd.Context(), item.ID); err != nil {
					return err
				}
			}
			commandLogger(cmd).Success("PLUGIN", "plugins updated", "count", len(outdated))
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("plugin update requires a plugin or --all")
		}
		id, err := simplePluginID(args[0])
		if err != nil {
			return err
		}
		result, err := manager.Update(cmd.Context(), id)
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("PLUGIN", "plugin updated", "id", id, "version", result.Plugin.Manifest.Version)
		return nil
	}}
	cmd.Flags().BoolVar(&all, "all", false, "update all outdated plugins")
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "rollback <plugin> [version]", Short: "Rollback to a retained signed plugin version", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		id, err := simplePluginID(args[0])
		if err != nil {
			return err
		}
		var target pluginpkg.Version
		if len(args) == 2 {
			_, _, target, err = pluginpkg.ParseReference(string(id) + "@" + strings.TrimSpace(args[1]))
			if err != nil {
				return err
			}
		}
		result, err := manager.Rollback(cmd.Context(), id, target)
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("PLUGIN", "plugin rolled back", "id", id, "version", result.Plugin.Manifest.Version)
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginPruneCommand() *cobra.Command {
	var retain int
	var pruneCache bool
	cmd := &cobra.Command{Use: "prune [plugin]", Short: "Prune old plugin versions and optional cache", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		span := tracepkg.Start(cmd.Context(), "PLUGIN", "plugin.prune", "Pruning plugin versions and cache", tracepkg.Int("retain_inactive", retain), tracepkg.Bool("cache", pruneCache))
		removedCount := 0
		if len(args) == 1 {
			id, err := simplePluginID(args[0])
			if err != nil {
				span.Fail(err)
				return err
			}
			removed, err := manager.PruneVersions(id, retain)
			if err != nil {
				span.Fail(err)
				return err
			}
			removedCount = len(removed)
		} else {
			removed, err := manager.PruneAllVersions(retain)
			if err != nil {
				span.Fail(err)
				return err
			}
			for _, versions := range removed {
				removedCount += len(versions)
			}
		}
		if pruneCache {
			if err := manager.PruneCache(); err != nil {
				span.Fail(err)
				return err
			}
		}
		span.End(tracepkg.Int("removed_versions", removedCount))
		commandLogger(cmd).Success("PLUGIN", "plugin state pruned", "removed_versions", removedCount, "cache", pruneCache)
		return nil
	}}
	cmd.Flags().IntVar(&retain, "retain", pluginpkg.DefaultRollbackRetention, "inactive versions to retain per active plugin")
	cmd.Flags().BoolVar(&pruneCache, "cache", false, "remove plugin registry/download cache and stale extraction directories")
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginOutdatedCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "outdated", Short: "List installed plugins with newer stable versions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		items, err := manager.Outdated(cmd.Context())
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		for _, item := range items {
			log.Detail(string(item.ID), fmt.Sprintf("%s -> %s", item.Current, item.Latest))
		}
		if len(items) == 0 {
			log.Ready("PLUGIN", "plugin.current", "All installed plugins are current")
		}
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginVerifyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "verify <plugin>", Short: "Verify installed plugin lock and manifest integrity", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManagerFromCmd(cmd)
		if err != nil {
			return err
		}
		id, err := simplePluginID(args[0])
		if err != nil {
			return err
		}
		if err := manager.Verify(cmd.Context(), id); err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("PLUGIN", "plugin verified", "id", id)
		if resources, err := manager.Store.InstructionResources(id); err == nil && len(resources) > 0 {
			var rules, skills int
			for _, item := range resources {
				if item.Kind == "skill" {
					skills++
				} else {
					rules++
				}
				log.Detail(item.Kind, item.Name+"  "+item.State)
			}
			log.Detail("rules", strconv.Itoa(rules))
			log.Detail("skills", strconv.Itoa(skills))
		}
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginConfigCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Read and update plugin configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(pluginConfigListCommand(), pluginConfigGetCommand(), pluginConfigSetCommand(), pluginConfigResetCommand())
	return cmd
}

func pluginConfigListCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list <plugin>", Aliases: []string{"ls"}, Short: "List effective plugin configuration", Args: cobra.ExactArgs(1), ValidArgsFunction: completePluginID, RunE: func(cmd *cobra.Command, args []string) error {
		service, id, err := pluginConfigTarget(cmd, args[0])
		if err != nil {
			return err
		}
		schema, values, err := service.PluginSettings(id)
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		for _, field := range schema.Fields {
			log.Detail(field.Key, pluginSettingDisplay(field, values[field.Key]))
		}
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginConfigGetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "get <plugin> <key>", Short: "Show one plugin configuration value", Args: cobra.ExactArgs(2), ValidArgsFunction: completePluginConfigKey, RunE: func(cmd *cobra.Command, args []string) error {
		service, id, err := pluginConfigTarget(cmd, args[0])
		if err != nil {
			return err
		}
		field, value, err := service.PluginSetting(id, args[1])
		if err != nil {
			return err
		}
		commandLogger(cmd).Detail(field.Key, pluginSettingDisplay(field, value))
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginConfigSetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "set <plugin> <key> <value>", Short: "Set one plugin configuration value", Args: cobra.ExactArgs(3), ValidArgsFunction: completePluginConfigSet, RunE: func(cmd *cobra.Command, args []string) error {
		service, id, err := pluginConfigTarget(cmd, args[0])
		if err != nil {
			return err
		}
		if err := service.SetPluginSetting(cmd.Context(), id, args[1], args[2]); err != nil {
			return err
		}
		commandLogger(cmd).Success("PLUGIN", "plugin config saved", "id", id, "key", args[1])
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginConfigResetCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "reset <plugin> [key]", Short: "Reset plugin configuration to schema defaults", Args: cobra.RangeArgs(1, 2), ValidArgsFunction: completePluginConfigKey, RunE: func(cmd *cobra.Command, args []string) error {
		service, id, err := pluginConfigTarget(cmd, args[0])
		if err != nil {
			return err
		}
		key := ""
		if len(args) > 1 {
			key = args[1]
		}
		if err := service.ResetPluginSetting(cmd.Context(), id, key); err != nil {
			return err
		}
		log := commandLogger(cmd)
		if key == "" {
			log.Success("PLUGIN", "plugin config reset", "id", id)
			return nil
		}
		log.Success("PLUGIN", "plugin config reset", "id", id, "key", key)
		return nil
	}}
	addPluginScopeFlags(cmd)
	return cmd
}

func pluginConfigTarget(cmd *cobra.Command, raw string) (*application.PluginService, pluginpkg.PluginID, error) {
	id, err := simplePluginID(raw)
	if err != nil {
		return nil, "", err
	}
	service, err := application.NewPluginServiceForOptions(pluginScopeOptions(cmd))
	if err != nil {
		return nil, "", err
	}
	return service, id, nil
}

func pluginSettingDisplay(field pluginpkg.SettingField, value any) string {
	if field.Sensitive {
		if configured, ok := value.(bool); ok && configured {
			return "configured"
		}
		return "not configured"
	}
	return pluginpkg.FormatSettingValue(value)
}

func pluginRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "registry", Short: "Manage plugin registries", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(pluginRegistryListCommand(), pluginRegistryAddCommand(), pluginRegistryRemoveCommand())
	return cmd
}

func pluginRegistryListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List configured plugin registries", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, layout, err := newPluginManager()
		if err != nil {
			return err
		}
		config, err := pluginpkg.LoadConfig(layout.ConfigPath())
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		for _, registry := range config.AllRegistries() {
			log.Detail(registry.Name, registry.URL)
		}
		return nil
	}}
}

func pluginRegistryAddCommand() *cobra.Command {
	var unqualified bool
	var issuer string
	var repository string
	cmd := &cobra.Command{Use: "add <name> <url>", Short: "Add a plugin registry", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		_, layout, err := newPluginManager()
		if err != nil {
			return err
		}
		trust := pluginpkg.SigstoreIdentity{Issuer: strings.TrimSpace(issuer), Repository: strings.TrimSpace(repository)}
		span := tracepkg.Start(cmd.Context(), "PLUGIN", "plugin.registry.add", "Adding plugin registry", tracepkg.String("registry", strings.TrimSpace(args[0])), tracepkg.URL("url", args[1]))
		if err := pluginpkg.MutateConfig(layout, func(config *pluginpkg.Config) error {
			return config.AddRegistry(args[0], args[1], unqualified, trust)
		}); err != nil {
			span.Fail(err)
			return err
		}
		span.End()
		commandLogger(cmd).Success("PLUGIN", "registry added", "name", args[0])
		return nil
	}}
	cmd.Flags().BoolVar(&unqualified, "unqualified", false, "allow unqualified plugin names to resolve from this registry")
	cmd.Flags().StringVar(&issuer, "issuer", "https://token.actions.githubusercontent.com", "pinned Sigstore OIDC issuer for registry metadata")
	cmd.Flags().StringVar(&repository, "repository", "", "pinned GitHub owner/repository signing registry metadata")
	_ = cmd.MarkFlagRequired("repository")
	return cmd
}

func pluginRegistryRemoveCommand() *cobra.Command {
	return &cobra.Command{Use: "remove <name>", Short: "Remove a plugin registry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, layout, err := newPluginManager()
		if err != nil {
			return err
		}
		span := tracepkg.Start(cmd.Context(), "PLUGIN", "plugin.registry.remove", "Removing plugin registry", tracepkg.String("registry", strings.TrimSpace(args[0])))
		if err := pluginpkg.MutateConfig(layout, func(config *pluginpkg.Config) error {
			return config.RemoveRegistry(args[0])
		}); err != nil {
			span.Fail(err)
			return err
		}
		span.End()
		commandLogger(cmd).Success("PLUGIN", "registry removed", "name", args[0])
		return nil
	}}
}

func simplePluginID(value string) (pluginpkg.PluginID, error) {
	registry, id, pluginVersion, err := pluginpkg.ParseReference(value)
	if err != nil {
		return "", err
	}
	if registry != "" || pluginVersion != "" {
		return "", fmt.Errorf("expected an installed plugin id, got %q", value)
	}
	return id, nil
}

func sortedRegistryPluginIDs(entries map[pluginpkg.PluginID]pluginpkg.RegistryEntry) []pluginpkg.PluginID {
	ids := make([]pluginpkg.PluginID, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
