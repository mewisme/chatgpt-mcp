package application

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
)

func ListApprovalRequests(ctx context.Context) ([]approval.Request, error) {
	var result []approval.Request
	_, err := runtimecontrol.Request(ctx, http.MethodGet, "/requests", nil, &result)
	return result, err
}

func GetApprovalRequest(ctx context.Context, id string) (approval.Request, error) {
	var result approval.Request
	_, err := runtimecontrol.Request(ctx, http.MethodGet, "/requests/view?id="+url.QueryEscape(strings.TrimSpace(id)), nil, &result)
	return result, err
}

func ResolveApprovalRequest(ctx context.Context, id string, approve bool, reason string) (approval.Request, error) {
	action := "deny"
	if approve {
		action = "approve"
	}
	var result approval.Request
	_, err := runtimecontrol.Request(ctx, http.MethodPost, "/requests/"+action, map[string]string{"id": strings.TrimSpace(id), "reason": strings.TrimSpace(reason)}, &result)
	return result, err
}

func CreateDummyApprovalRequest(ctx context.Context, workspaceID, title, command string) (approval.Request, error) {
	var result approval.Request
	_, err := runtimecontrol.Request(ctx, http.MethodPost, "/requests/create-dummy", map[string]string{
		"workspace_id": strings.TrimSpace(workspaceID), "title": strings.TrimSpace(title), "command": strings.TrimSpace(command),
	}, &result)
	return result, err
}
