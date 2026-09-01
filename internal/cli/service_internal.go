package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
)

type serviceRuntimeInfo struct {
	Managed bool
	ID      string
	Scope   string
}

type serviceRuntimeContextKey struct{}

func internalServiceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "_service", Short: "Internal managed service commands", Hidden: true}
	var serviceID, serviceScope, environmentHash string
	run := &cobra.Command{
		Use:    "run",
		Short:  "Run chatgpt-mcp as an internal managed service",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if environmentHash != "" {
				snapshot, err := managed.LoadEnvironment(config.RootPath(), environmentHash)
				if err != nil {
					return err
				}
				if err := managed.ApplyEnvironment(snapshot); err != nil {
					return err
				}
			}
			ctx := context.WithValue(cmd.Context(), serviceRuntimeContextKey{}, serviceRuntimeInfo{Managed: true, ID: serviceID, Scope: serviceScope})
			cmd.SetContext(ctx)
			return runServer(cmd, args)
		},
	}
	run.Flags().StringVar(&serviceID, "service-id", "", "managed service identity")
	run.Flags().StringVar(&serviceScope, "service-scope", "user", "managed service scope")
	run.Flags().StringVar(&environmentHash, "service-environment-hash", "", "managed environment snapshot hash")
	cmd.AddCommand(run)
	return cmd
}

func runtimeServiceInfo(cmd *cobra.Command) serviceRuntimeInfo {
	if cmd == nil || cmd.Context() == nil {
		return serviceRuntimeInfo{}
	}
	info, _ := cmd.Context().Value(serviceRuntimeContextKey{}).(serviceRuntimeInfo)
	return info
}
