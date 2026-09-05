package cli

import (
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func workspaceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Aliases: []string{"ws"}, Short: "Manage registered workspace roots"}
	cmd.AddCommand(
		workspaceRegisterCommand(),
		workspaceListCommand(),
		workspaceShowCommand(),
		workspaceUnregisterCommand(),
		workspaceAccessCommand(),
		workspaceContainerCommand(),
	)
	return cmd
}

func workspaceContainerCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "container", Aliases: []string{"ctr"}, Short: "Manage workspace containers"}
	cmd.AddCommand(
		workspaceContainerListCommand(),
		workspaceContainerCreateCommand(),
		workspaceContainerShowCommand(),
		workspaceContainerRenameCommand(),
		workspaceContainerDeleteCommand(),
		workspaceContainerMembershipCommand(true),
		workspaceContainerMembershipCommand(false),
	)
	return cmd
}

func workspaceContainerListCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List workspace containers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		values, err := workspace.NewManager(workspace.DefaultStorePath()).ListContainers()
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, values)
		}
		log := commandLogger(cmd)
		log.Success("WORKSPACE", "workspace containers loaded", "count", len(values))
		for _, value := range values {
			log.Detail(value.ID, value.Name+" · "+strconv.Itoa(len(value.WorkspaceIDs))+" workspaces")
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func workspaceContainerCreateCommand() *cobra.Command {
	return &cobra.Command{Use: "create <name>", Short: "Create a workspace container", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		value, err := workspace.NewManager(workspace.DefaultStorePath()).CreateContainer(args[0])
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("WORKSPACE", "workspace container created")
		log.Detail("id", value.ID)
		log.Detail("name", value.Name)
		return nil
	}}
}

func workspaceContainerShowCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "show <wsc_id>", Short: "Show one workspace container", Args: cobra.ExactArgs(1), ValidArgsFunction: completeWorkspaceContainerID, RunE: func(cmd *cobra.Command, args []string) error {
		manager := workspace.NewManager(workspace.DefaultStorePath())
		value, err := manager.GetContainer(args[0])
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, value)
		}
		log := commandLogger(cmd)
		log.Info("WORKSPACE", "workspace container details")
		log.Detail("id", value.ID)
		log.Detail("name", value.Name)
		if len(value.WorkspaceIDs) == 0 {
			log.Detail("workspaces", "none")
		} else {
			log.Detail("workspaces", value.WorkspaceIDs)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func workspaceContainerRenameCommand() *cobra.Command {
	return &cobra.Command{Use: "rename <wsc_id> <name>", Short: "Rename a workspace container", Args: cobra.ExactArgs(2), ValidArgsFunction: completeWorkspaceContainerThenName, RunE: func(cmd *cobra.Command, args []string) error {
		value, err := workspace.NewManager(workspace.DefaultStorePath()).RenameContainer(args[0], args[1])
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("WORKSPACE", "workspace container renamed")
		log.Detail("id", value.ID)
		log.Detail("name", value.Name)
		return nil
	}}
}

func workspaceContainerDeleteCommand() *cobra.Command {
	return &cobra.Command{Use: "delete <wsc_id>", Aliases: []string{"rm"}, Short: "Delete a workspace container without unregistering workspaces", Args: cobra.ExactArgs(1), ValidArgsFunction: completeWorkspaceContainerID, RunE: func(cmd *cobra.Command, args []string) error {
		manager := workspace.NewManager(workspace.DefaultStorePath())
		value, err := manager.GetContainer(args[0])
		if err != nil {
			return err
		}
		if err := manager.DeleteContainer(args[0]); err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("WORKSPACE", "workspace container deleted")
		log.Detail("id", value.ID)
		log.Detail("name", value.Name)
		log.Detail("workspaces", "unchanged")
		return nil
	}}
}

func workspaceContainerMembershipCommand(add bool) *cobra.Command {
	use, short, action := "add <wsc_id> <workspace_id...>", "Add workspaces to a workspace container", "added"
	if !add {
		use, short, action = "remove <wsc_id> <workspace_id...>", "Remove workspaces from a workspace container", "removed"
	}
	return &cobra.Command{Use: use, Short: short, Args: cobra.MinimumNArgs(2), ValidArgsFunction: completeWorkspaceContainerThenWorkspaces, RunE: func(cmd *cobra.Command, args []string) error {
		manager := workspace.NewManager(workspace.DefaultStorePath())
		var value workspace.WorkspaceContainer
		var err error
		if add {
			value, err = manager.AddWorkspacesToContainer(args[0], args[1:])
		} else {
			value, err = manager.RemoveWorkspacesFromContainer(args[0], args[1:])
		}
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("WORKSPACE", "workspace container membership updated")
		log.Detail("container", value.ID)
		log.Detail(action, args[1:])
		return nil
	}}
}

func workspaceAccessCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "access", Short: "Manage workspace-specific filesystem access"}
	var listJSON bool
	list := &cobra.Command{Use: "list <workspace_id>", Aliases: []string{"ls"}, Short: "List workspace-specific additional directories", Args: cobra.ExactArgs(1), ValidArgsFunction: completeWorkspaceID, RunE: func(cmd *cobra.Command, args []string) error {
		manager := workspace.NewManager(workspace.DefaultStorePath())
		item, err := manager.Get(args[0])
		if err != nil {
			return err
		}
		if listJSON {
			return printJSON(cmd, item.AllowDirs)
		}
		log := commandLogger(cmd)
		log.Success("WORKSPACE", "allowed directories loaded", "count", len(item.AllowDirs))
		log.Detail("workspace", item.ID)
		if len(item.AllowDirs) == 0 {
			log.Detail("allow dirs", "none")
		} else {
			log.Detail("allow dirs", item.AllowDirs)
		}
		return nil
	}}
	list.Flags().BoolVar(&listJSON, "json", false, "print JSON")
	cmd.AddCommand(
		&cobra.Command{Use: "add <workspace_id> <path>", Short: "Grant a workspace access to an additional directory", Args: cobra.ExactArgs(2), ValidArgsFunction: completeWorkspaceThenDirectory, RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			item, err := manager.AddAllowDir(args[0], args[1])
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("WORKSPACE", "allowed directory added")
			log.Detail("id", item.ID)
			log.Detail("allow_dir", args[1])
			return nil
		}},
		&cobra.Command{Use: "remove <workspace_id> <path>", Short: "Revoke an additional directory from a workspace", Args: cobra.ExactArgs(2), ValidArgsFunction: completeWorkspaceThenDirectory, RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			item, err := manager.RemoveAllowDir(args[0], args[1])
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("WORKSPACE", "allowed directory removed")
			log.Detail("id", item.ID)
			return nil
		}},
		list,
	)
	return cmd
}

func workspaceRegisterCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "register [path]",
		Short:             "Register a canonical workspace root and return its stable workspace_id",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeDirectory,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else {
				var err error
				path, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			manager := workspace.NewManager(workspace.DefaultStorePath())
			item, err := manager.Register(path)
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("WORKSPACE", "workspace registered")
			log.Detail("id", item.ID)
			log.Detail("root", item.Path)
			return nil
		},
	}
}

func workspaceListCommand() *cobra.Command {
	var asJSON, forceInteractive, noInteractive bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered workspace roots",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			items, err := manager.List()
			if err != nil {
				return err
			}
			interactiveMode, err := interactive.ResolveMode(cmd.InOrStdin(), cmd.OutOrStdout(), forceInteractive, noInteractive, asJSON)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, items)
			}
			if interactiveMode {
				return runInteractiveBrowser(cmd, "Registered workspaces", workspaceInteractiveRows(items), workspaceInteractiveRefresh(), workspaceCopyIDAction())
			}
			log := commandLogger(cmd)
			log.Success("WORKSPACE", "registered workspaces loaded", "count", len(items))
			for _, item := range items {
				log.Detail(item.ID, item.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&forceInteractive, "interactive", false, "force interactive workspace list")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "disable interactive workspace list")
	return cmd
}

func workspaceShowCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:               "show <workspace_id>",
		Short:             "Show one registered workspace",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeWorkspaceID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			item, err := manager.Get(args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, item)
			}
			log := commandLogger(cmd)
			log.Info("WORKSPACE", "workspace details")
			log.Detail("id", item.ID)
			log.Detail("root", item.Path)
			if len(item.AllowDirs) == 0 {
				log.Detail("allow dirs", "none")
			} else {
				log.Detail("allow dirs", item.AllowDirs)
			}
			if len(item.LegacyIDs) > 0 {
				log.Detail("legacy ids", item.LegacyIDs)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func workspaceUnregisterCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "unregister <workspace_id>",
		Short:             "Remove a workspace handle without deleting project files",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeWorkspaceID,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			item, err := manager.Get(args[0])
			if err != nil {
				return err
			}
			if err := manager.Unregister(args[0]); err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("WORKSPACE", "workspace unregistered")
			log.Detail("id", item.ID)
			log.Detail("root", item.Path)
			log.Detail("files", "unchanged")
			return nil
		},
	}
}
