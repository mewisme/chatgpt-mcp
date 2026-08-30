package cli

import (
	"context"

	"github.com/spf13/cobra"
)

type serviceRuntimeInfo struct {
	Managed bool
	ID      string
	Scope   string
}

type serviceRuntimeContextKey struct{}

func internalServiceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "_service", Hidden: true}
	var serviceID, serviceScope string
	run := &cobra.Command{
		Use:    "run",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.WithValue(cmd.Context(), serviceRuntimeContextKey{}, serviceRuntimeInfo{Managed: true, ID: serviceID, Scope: serviceScope})
			cmd.SetContext(ctx)
			return runServer(cmd, args)
		},
	}
	run.Flags().StringVar(&serviceID, "service-id", "", "managed service identity")
	run.Flags().StringVar(&serviceScope, "service-scope", "user", "managed service scope")
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
