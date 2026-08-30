package cli

import (
	"os"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func workspaceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "Manage registered workspace roots"}
	cmd.AddCommand(
		workspaceRegisterCommand(),
		workspaceListCommand(),
		workspaceShowCommand(),
		workspaceUnregisterCommand(),
	)
	return cmd
}

func workspaceRegisterCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "register [path]",
		Short: "Register a canonical workspace root and return its stable workspace_id",
		Args:  cobra.MaximumNArgs(1),
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
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered workspace roots",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			items, err := manager.List()
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, items)
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
	return cmd
}

func workspaceShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <workspace_id>",
		Short: "Show one registered workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewManager(workspace.DefaultStorePath())
			item, err := manager.Get(args[0])
			if err != nil {
				return err
			}
			return printJSON(cmd, item)
		},
	}
}

func workspaceUnregisterCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unregister <workspace_id>",
		Short: "Remove a workspace handle without deleting project files",
		Args:  cobra.ExactArgs(1),
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
