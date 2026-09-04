package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/approval"
)

const ApprovalRequestToolName = "request_control_approval"

type approvalRequiredResponse struct {
	Code        string    `json:"code"`
	ChallengeID string    `json:"challenge_id"`
	WorkspaceID string    `json:"workspace_id"`
	TargetTool  string    `json:"target_tool"`
	Arguments   any       `json:"arguments"`
	GuardCode   string    `json:"guard_code"`
	Reason      string    `json:"reason"`
	Title       string    `json:"title"`
	ExpiresAt   time.Time `json:"expires_at"`
	RequestTool string    `json:"request_tool"`
}

type approvalResolutionResponse struct {
	ID          string          `json:"id"`
	Status      approval.Status `json:"status"`
	WorkspaceID string          `json:"workspace_id"`
	TargetTool  string          `json:"target_tool"`
	Arguments   any             `json:"arguments"`
	RetryUntil  time.Time       `json:"retry_until,omitempty"`
	Instruction string          `json:"instruction"`
}

type approvalMismatchTarget struct {
	Tool      string `json:"tool"`
	Arguments any    `json:"arguments"`
}

type approvalMismatchResponse struct {
	Code        string                 `json:"code"`
	RequestID   string                 `json:"request_id"`
	Expected    approvalMismatchTarget `json:"expected"`
	Actual      approvalMismatchTarget `json:"actual"`
	Instruction string                 `json:"instruction"`
}

func RegisterApprovalTools(registry *Registry, runtime *Runtime) {
	if registry == nil {
		return
	}
	registry.MustRegister(ApprovalRequestToolName, coreSchema(
		ApprovalRequestToolName,
		"Request local human approval for a recent control-guard challenge. The request remains bound to the same MCP session, workspace, target tool, and exact arguments.",
		`{"type":"object","properties":{"workspace_id":{"type":"string"},"challenge_id":{"type":"string"}},"required":["workspace_id","challenge_id"],"additionalProperties":false}`,
		`{"type":"object","properties":{"id":{"type":"string"},"status":{"type":"string"},"workspace_id":{"type":"string"},"target_tool":{"type":"string"},"arguments":{},"retry_until":{"type":"string"},"instruction":{"type":"string"}},"required":["id","status","workspace_id","target_tool","arguments","instruction"],"additionalProperties":false}`,
		RiskCommand,
	), func(ctx context.Context, args map[string]any) (Result, error) {
		workspaceID, err := requiredString(args, "workspace_id")
		if err != nil {
			return Result{}, err
		}
		challengeID, err := requiredString(args, "challenge_id")
		if err != nil {
			return Result{}, err
		}
		if runtime == nil || runtime.Approvals == nil {
			return Result{}, errors.New("control approval manager is unavailable")
		}
		sessionID := MCPSessionID(ctx)
		if sessionID == "" {
			return Result{}, errors.New("MCP session id is required for control approval requests")
		}
		request, _, err := runtime.Approvals.CreateRequest(challengeID, sessionID, workspaceID)
		if err != nil {
			return Result{}, err
		}
		resolved, err := runtime.Approvals.Wait(ctx, request.ID)
		if err != nil {
			return Result{}, err
		}
		return approvalResolutionResult(resolved), nil
	})
}

func approvalRequiredResult(challenge approval.Challenge) Result {
	arguments := decodeApprovalArguments(challenge.Arguments)
	response := approvalRequiredResponse{
		Code: "approval_required", ChallengeID: challenge.ID, WorkspaceID: challenge.WorkspaceID, TargetTool: challenge.TargetTool, Arguments: arguments,
		GuardCode: string(challenge.GuardCode), Reason: challenge.GuardReason, Title: challenge.Title, ExpiresAt: challenge.ExpiresAt, RequestTool: ApprovalRequestToolName,
	}
	text := fmt.Sprintf("This control-plane action requires local approval. Call %s with workspace_id %q and challenge_id %q. If approved, retry %s with exactly the arguments shown in the structured response.", ApprovalRequestToolName, challenge.WorkspaceID, challenge.ID, challenge.TargetTool)
	return Result{Content: []Content{{Type: "text", Text: text}}, StructuredContent: response, IsError: true, ResultType: "complete"}
}

func approvalResolutionResult(request approval.Request) Result {
	instruction := "Do not retry the guarded action."
	if request.Status == approval.StatusApproved {
		instruction = "Retry the target tool with exactly these arguments before retry_until."
	}
	response := approvalResolutionResponse{
		ID: request.ID, Status: request.Status, WorkspaceID: request.WorkspaceID, TargetTool: request.TargetTool, Arguments: decodeApprovalArguments(request.Arguments),
		RetryUntil: request.RetryUntil, Instruction: instruction,
	}
	result := JSONResult(response)
	if request.Status != approval.StatusApproved {
		result.IsError = true
	}
	return result
}

func approvalMismatchResult(mismatch *approval.MismatchError) Result {
	if mismatch == nil {
		return ErrorResult(errors.New("approval retry does not match approved arguments"))
	}
	response := approvalMismatchResponse{
		Code: "approval_mismatch", RequestID: mismatch.RequestID,
		Expected:    approvalMismatchTarget{Tool: mismatch.TargetTool, Arguments: decodeApprovalArguments(mismatch.Expected)},
		Actual:      approvalMismatchTarget{Tool: mismatch.TargetTool, Arguments: decodeApprovalArguments(mismatch.Actual)},
		Instruction: "Retry the exact approved target and arguments, or abandon this approval request.",
	}
	result := JSONResult(response)
	result.IsError = true
	return result
}

func decodeApprovalArguments(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}
