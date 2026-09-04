package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
)

const requestControlTimeout = 5 * time.Second

func requestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "request", Aliases: []string{"req"}, Short: "Review and resolve control approval requests"}
	cmd.AddCommand(requestListCommand(), requestViewCommand(), requestResolveCommand(true), requestResolveCommand(false), requestCreateCommand())
	return cmd
}

func requestCreateCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "create", Short: "Create control approval requests for testing"}
	cmd.AddCommand(requestCreateDummyCommand())
	return cmd
}

func requestCreateDummyCommand() *cobra.Command {
	var workspaceID, title, command string
	var asJSON bool
	cmd := &cobra.Command{Use: "dummy", Short: "Create a dummy pending approval request for UI testing", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		log := commandLogger(cmd)
		defer log.Close()
		if !asJSON {
			startCommandSpinner(cmd, log, "REQUEST", "request.creating", "Creating dummy approval request")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), requestControlTimeout)
		defer cancel()
		request, err := requestRuntimeApprovalCreateDummy(ctx, workspaceID, title, command)
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, request)
		}
		log.Success("REQUEST", "dummy control approval request created", "id", request.ID)
		log.Detail("workspace", request.WorkspaceID)
		log.Detail("expires", request.ExpiresAt.Format(time.RFC3339Nano))
		return nil
	}}
	cmd.Flags().StringVar(&workspaceID, "workspace", "ws_dummy", "workspace ID shown on the dummy request")
	cmd.Flags().StringVar(&title, "title", "Allow dummy command", "request title shown in approval UIs")
	cmd.Flags().StringVar(&command, "command", "echo dummy approval", "dummy run_command value shown in exact arguments")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func requestListCommand() *cobra.Command {
	var asJSON, forceInteractive, noInteractive bool
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List control approval requests from the running runtime", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		log := commandLogger(cmd)
		defer log.Close()
		if !asJSON {
			startCommandSpinner(cmd, log, "REQUEST", "request.loading", "Loading approval requests")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), requestControlTimeout)
		defer cancel()
		requests, err := requestRuntimeApprovalList(ctx)
		if err != nil {
			return err
		}
		interactiveMode, err := interactive.ResolveMode(cmd.InOrStdin(), cmd.OutOrStdout(), forceInteractive, noInteractive, asJSON)
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, requests)
		}
		if interactiveMode {
			log.Close()
			model := newRequestInteractiveModel(cmd.Context(), requests, defaultRequestInteractiveClient())
			_, err := interactive.Run(cmd.Context(), model, cmd.InOrStdin(), cmd.OutOrStdout())
			return err
		}
		log.Success("REQUEST", "control approval requests loaded", "count", len(requests))
		for _, request := range requests {
			log.Detail(request.ID, fmt.Sprintf("status=%s workspace=%s tool=%s title=%s", request.Status, request.WorkspaceID, request.TargetTool, request.Title))
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&forceInteractive, "interactive", false, "force interactive request list")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "disable interactive request list")
	return cmd
}

func requestViewCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "view <request_id>", Aliases: []string{"show", "info"}, Short: "Show one control approval request by ID or unique prefix", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		log := commandLogger(cmd)
		defer log.Close()
		if !asJSON {
			startCommandSpinner(cmd, log, "REQUEST", "request.loading", "Loading approval request")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), requestControlTimeout)
		defer cancel()
		request, err := requestRuntimeApprovalView(ctx, args[0])
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, request)
		}
		log.Close()
		printApprovalRequest(cmd, request)
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func requestResolveCommand(approve bool) *cobra.Command {
	action, past := "deny", "denied"
	label := "Deny"
	aliases := []string{"reject"}
	if approve {
		action, past = "approve", "approved"
		label = "Approve"
		aliases = []string{"accept", "allow"}
	}
	progress := "Denying approval request"
	if approve {
		progress = "Approving approval request"
	}
	var asJSON bool
	var reason string
	cmd := &cobra.Command{Use: action + " <request_id>", Aliases: aliases, Short: label + " one pending control approval request by ID or unique prefix", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		log := commandLogger(cmd)
		defer log.Close()
		if !asJSON {
			startCommandSpinner(cmd, log, "REQUEST", "request.resolving", progress)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), requestControlTimeout)
		defer cancel()
		var request approval.Request
		var err error
		if approve {
			request, err = requestRuntimeApprovalApprove(ctx, args[0], reason)
		} else {
			request, err = requestRuntimeApprovalDeny(ctx, args[0], reason)
		}
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, request)
		}
		log.Success("REQUEST", "control approval request "+past, "id", request.ID)
		log.Detail("status", request.Status)
		if !request.RetryUntil.IsZero() {
			log.Detail("retry_until", request.RetryUntil.Format(time.RFC3339Nano))
		}
		if request.Reason != "" {
			log.Detail("reason", request.Reason)
		}
		return nil
	}}
	cmd.Flags().StringVar(&reason, "reason", "", "record an optional approval resolution reason")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func printApprovalRequest(cmd *cobra.Command, request approval.Request) {
	log := commandLogger(cmd)
	log.Info("REQUEST", "control approval request", "id", request.ID)
	log.Detail("status", request.Status)
	log.Detail("title", request.Title)
	log.Detail("workspace", request.WorkspaceID)
	log.Detail("tool", request.TargetTool)
	if request.Source != "" {
		log.Detail("source", request.Source)
	}
	if request.SessionHash != "" {
		log.Detail("session", request.SessionHash)
	}
	log.Detail("guard", request.GuardCode)
	log.Detail("created", request.CreatedAt.Format(time.RFC3339Nano))
	log.Detail("expires", request.ExpiresAt.Format(time.RFC3339Nano))
	if !request.ResolvedAt.IsZero() {
		log.Detail("resolved", request.ResolvedAt.Format(time.RFC3339Nano))
	}
	if request.ResolvedBy != "" {
		log.Detail("resolved_by", request.ResolvedBy)
	}
	if request.Reason != "" {
		log.Detail("reason", request.Reason)
	}
	if !request.RetryUntil.IsZero() {
		log.Detail("retry_until", request.RetryUntil.Format(time.RFC3339Nano))
	}
	if !request.ConsumedAt.IsZero() {
		log.Detail("consumed", request.ConsumedAt.Format(time.RFC3339Nano))
	}
	if len(request.Arguments) > 0 {
		log.Detail("arguments", string(request.Arguments))
	}
	if request.GuardReason != "" {
		log.Detail("reason_guard", request.GuardReason)
	}
}
