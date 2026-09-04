package tools

import (
	"context"
	"errors"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
)

func (r *Runtime) prepareApprovalRetry(ctx context.Context, sessionID, workspaceID, source, name string, args map[string]any) (context.Context, approval.Request, *Result, error) {
	if r == nil || r.Approvals == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(workspaceID) == "" || name == ApprovalRequestToolName {
		return ctx, approval.Request{}, nil, nil
	}
	request, matched, err := r.Approvals.ClaimApproved(approval.RetryInput{
		SessionID: sessionID, WorkspaceID: workspaceID, Source: source, TargetTool: name, Arguments: args,
	})
	if err != nil {
		var mismatch *approval.MismatchError
		if errors.As(err, &mismatch) {
			result := approvalMismatchResult(mismatch)
			return ctx, approval.Request{}, &result, nil
		}
		return ctx, approval.Request{}, nil, err
	}
	if !matched {
		return ctx, approval.Request{}, nil, nil
	}
	return WithApprovalRequest(ctx, request.ID), request, nil, nil
}

func (r *Runtime) approvalResultForGuard(guard *controlguard.Error, sessionID, sessionHash, workspaceID, source, name string, args map[string]any, claimed approval.Request) (Result, bool, error) {
	if guard == nil || !guard.Approvable || claimed.ID != "" || r == nil || r.Approvals == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(workspaceID) == "" {
		return Result{}, false, nil
	}
	title := "Allow " + name
	if guard.Invocation != nil && strings.TrimSpace(guard.Invocation.Command) != "" {
		title = "Allow " + strings.TrimSpace(guard.Invocation.Command)
	}
	challenge, _, err := r.Approvals.CreateChallenge(approval.ChallengeInput{
		SessionID: sessionID, SessionHash: sessionHash, WorkspaceID: workspaceID, Source: source, TargetTool: name, Arguments: args,
		GuardCode: guard.Code, GuardReason: guard.Error(), Title: title,
	})
	if err != nil {
		return Result{}, false, err
	}
	return approvalRequiredResult(challenge), true, nil
}
