package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func pluginCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "plugin", Short: "Manage signed ChatGPT MCP plugins", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(pluginSearchCommand(), pluginInfoCommand(), pluginListCommand(), pluginInstallCommand(), pluginUninstallCommand(), pluginToggleCommand(true), pluginToggleCommand(false), pluginUpdateCommand(), pluginRollbackCommand(), pluginPruneCommand(), pluginOutdatedCommand(), pluginVerifyCommand(), pluginRegistryCommand())
	return cmd
}

func newPluginManager() (*pluginpkg.Manager, pluginpkg.Layout, error) {
	layout := pluginpkg.DefaultLayout()
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{CoreVersion: version.Version})
	if err != nil {
		return nil, pluginpkg.Layout{}, err
	}
	client := pluginpkg.RegistryClient{Layout: layout, UserAgent: "chatgpt-mcp/" + version.Version}
	return &pluginpkg.Manager{Store: store, RegistryClient: client}, layout, nil
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
		resolved, err := manager.Resolve(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Detail("id", resolved.Registry.Name+"/"+string(resolved.PluginID))
		log.Detail("name", resolved.Entry.Name)
		log.Detail("version", resolved.Version)
		log.Detail("publisher", resolved.Publisher.Name)
		log.Detail("type", resolved.Entry.Type)
		log.Detail("description", resolved.Entry.Description)
		return nil
	}}
}

func pluginListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List installed plugins", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, layout, err := newPluginManager()
		if err != nil {
			return err
		}
		lock, err := pluginpkg.LoadLock(layout.LockPath())
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(lock.Plugins))
		for id := range lock.Plugins {
			ids = append(ids, string(id))
		}
		sort.Strings(ids)
		log := commandLogger(cmd)
		for _, id := range ids {
			entry := lock.Plugins[pluginpkg.PluginID(id)]
			state := "disabled"
			if entry.Enabled {
				state = "enabled"
			}
			log.Detail(id, fmt.Sprintf("%s  %s  %s", entry.Version, entry.Registry, state))
		}
		if len(ids) == 0 {
			log.Notice("PLUGIN", "plugin.list.empty", "No plugins installed")
		}
		return nil
	}}
}

func pluginInstallCommand() *cobra.Command {
	return &cobra.Command{Use: "install <plugin>[@version]", Short: "Install and enable a signed plugin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
		if err != nil {
			return err
		}
		result, err := manager.Install(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("PLUGIN", "plugin installed", "id", result.Plugin.Manifest.ID, "version", result.Plugin.Manifest.Version)
		log.Detail("registry", result.Registry.Name)
		log.Detail("publisher", result.Publisher.Name)
		return nil
	}}
}

func pluginUninstallCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{Use: "uninstall <plugin>", Short: "Disable and uninstall the active plugin version", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
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
	return cmd
}

func pluginToggleCommand(enabled bool) *cobra.Command {
	action := "disable"
	traceName := "plugin.disable"
	if enabled {
		action = "enable"
		traceName = "plugin.enable"
	}
	return &cobra.Command{Use: action + " <plugin>", Short: action + " an installed plugin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
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
}

func pluginUpdateCommand() *cobra.Command {
	var all bool
	cmd := &cobra.Command{Use: "update [plugin]", Short: "Update installed plugins to registry stable versions", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
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
	return cmd
}

func pluginRollbackCommand() *cobra.Command {
	return &cobra.Command{Use: "rollback <plugin> [version]", Short: "Rollback to a retained signed plugin version", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
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
}

func pluginPruneCommand() *cobra.Command {
	var retain int
	var pruneCache bool
	cmd := &cobra.Command{Use: "prune [plugin]", Short: "Prune old plugin versions and optional cache", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
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
	return cmd
}

func pluginOutdatedCommand() *cobra.Command {
	return &cobra.Command{Use: "outdated", Short: "List installed plugins with newer stable versions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		manager, _, err := newPluginManager()
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
}

func pluginVerifyCommand() *cobra.Command {
	return &cobra.Command{Use: "verify <plugin>", Short: "Verify installed plugin lock and manifest integrity", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, _, err := newPluginManager()
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
		commandLogger(cmd).Success("PLUGIN", "plugin verified", "id", id)
		return nil
	}}
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
		config, err := pluginpkg.LoadConfig(layout.ConfigPath())
		if err != nil {
			return err
		}
		trust := pluginpkg.SigstoreIdentity{Issuer: strings.TrimSpace(issuer), Repository: strings.TrimSpace(repository)}
		span := tracepkg.Start(cmd.Context(), "PLUGIN", "plugin.registry.add", "Adding plugin registry", tracepkg.String("registry", strings.TrimSpace(args[0])), tracepkg.URL("url", args[1]))
		if err := config.AddRegistry(args[0], args[1], unqualified, trust); err != nil {
			span.Fail(err)
			return err
		}
		if err := pluginpkg.WriteConfig(layout.ConfigPath(), config); err != nil {
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
		config, err := pluginpkg.LoadConfig(layout.ConfigPath())
		if err != nil {
			return err
		}
		if err := config.RemoveRegistry(args[0]); err != nil {
			return err
		}
		span := tracepkg.Start(cmd.Context(), "PLUGIN", "plugin.registry.remove", "Removing plugin registry", tracepkg.String("registry", strings.TrimSpace(args[0])))
		if err := pluginpkg.WriteConfig(layout.ConfigPath(), config); err != nil {
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
