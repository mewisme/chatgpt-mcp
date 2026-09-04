package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
)

func TestRequestCLIListViewApproveDenyAliasesAndOutput(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	manager := approval.NewManager("instance-test")
	first := seedApprovalRequest(t, manager, "session-a", "ws_a", "cgm config set server.port 41001")
	second := seedApprovalRequest(t, manager, "session-b", "ws_b", "cgm update")
	control, err := startRuntimeControl(runtimeControlOptions{Approvals: manager, Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult { return runtimeStatusResult{PID: os.Getpid()} }, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()

	plain := executeRequestCommand(t, root, []string{"request", "ls"})
	if !strings.Contains(plain, first.ID) || !strings.Contains(plain, second.ID) || !strings.Contains(plain, "status=pending") {
		t.Fatalf("plain request list = %q", plain)
	}
	firstPrefix := uniqueRequestPrefix(first.ID, second.ID)
	viewJSON := executeRequestCommand(t, root, []string{"req", "info", firstPrefix, "--json"})
	var viewed approval.Request
	if err := json.Unmarshal([]byte(strings.TrimSpace(viewJSON)), &viewed); err != nil || viewed.ID != first.ID || viewed.Status != approval.StatusPending {
		t.Fatalf("view json = %q value=%#v err=%v", viewJSON, viewed, err)
	}
	viewPlain := executeRequestCommand(t, root, []string{"request", "show", firstPrefix})
	if !strings.Contains(viewPlain, first.Title) || !strings.Contains(viewPlain, "workspace") || !strings.Contains(viewPlain, "arguments") {
		t.Fatalf("plain request view = %q", viewPlain)
	}

	approvedJSON := executeRequestCommand(t, root, []string{"req", "accept", firstPrefix, "--reason", "reviewed", "--json"})
	var approved approval.Request
	if err := json.Unmarshal([]byte(strings.TrimSpace(approvedJSON)), &approved); err != nil || approved.Status != approval.StatusApproved || approved.ResolvedBy != "cli" || approved.Reason != "reviewed" || approved.RetryUntil.IsZero() {
		t.Fatalf("approve json = %q value=%#v err=%v", approvedJSON, approved, err)
	}
	secondPrefix := uniqueRequestPrefix(second.ID, first.ID)
	denied := executeRequestCommand(t, root, []string{"request", "reject", secondPrefix, "--reason", "not now"})
	if !strings.Contains(denied, "Control approval request denied") || !strings.Contains(denied, "not now") {
		t.Fatalf("plain deny output = %q", denied)
	}
	stored, err := manager.Resolve(second.ID)
	if err != nil || stored.Status != approval.StatusDenied || stored.ResolvedBy != "cli" || stored.Reason != "not now" {
		t.Fatalf("denied request = %#v err=%v", stored, err)
	}
}

func TestRequestCLIAmbiguousPrefixAndStoppedRuntimeFailClosed(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	manager := approval.NewManager("instance-test")
	seedApprovalRequest(t, manager, "session-a", "ws_a", "cgm update")
	seedApprovalRequest(t, manager, "session-b", "ws_b", "cgm install")
	control, err := startRuntimeControl(runtimeControlOptions{Approvals: manager, Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult { return runtimeStatusResult{PID: os.Getpid()} }, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executeRequestCommandError(root, []string{"request", "view", "req_"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous prefix err=%v", err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := executeRequestCommandError(root, []string{"request", "list"}); err == nil || !strings.Contains(err.Error(), "no running server") {
		t.Fatalf("stopped runtime err=%v", err)
	}
}

func seedApprovalRequest(t *testing.T, manager *approval.Manager, sessionID, workspaceID, command string) approval.Request {
	t.Helper()
	challenge, _, err := manager.CreateChallenge(approval.ChallengeInput{
		SessionID: sessionID, SessionHash: "hash-" + sessionID, WorkspaceID: workspaceID, Source: "tunnel", TargetTool: "run_command",
		Arguments: map[string]any{"workspace_id": workspaceID, "command": command}, GuardCode: controlguard.CodeControlPlaneMutation, GuardReason: "control-plane mutation denied", Title: "Allow " + command,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := manager.CreateRequest(challenge.ID, sessionID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func uniqueRequestPrefix(id string, others ...string) string {
	for length := len("req_") + 1; length < len(id); length++ {
		prefix := id[:length]
		unique := true
		for _, other := range others {
			if strings.HasPrefix(other, prefix) {
				unique = false
				break
			}
		}
		if unique {
			return prefix
		}
	}
	return id
}

func executeRequestCommand(t *testing.T, root string, args []string) string {
	t.Helper()
	output, err := executeRequestCommandError(root, args)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func executeRequestCommandError(root string, args []string) (string, error) {
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(append([]string{"--config-dir", root}, args...))
	err := cmd.Execute()
	return output.String(), err
}
