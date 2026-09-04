package approval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/controlguard"
)

func TestManagerCoalescesChallengeAndRequest(t *testing.T) {
	manager, now := testManager()
	first, created, err := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	if err != nil || !created {
		t.Fatalf("first challenge = %#v created=%t err=%v", first, created, err)
	}
	*now = now.Add(10 * time.Second)
	second, created, err := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	if err != nil || created || second.ID != first.ID || !second.ExpiresAt.Equal(now.Add(DefaultChallengeTTL)) {
		t.Fatalf("second challenge = %#v created=%t err=%v", second, created, err)
	}
	request, created, err := manager.CreateRequest(first.ID, "session-a", "ws_x")
	if err != nil || !created || request.Status != StatusPending {
		t.Fatalf("request = %#v created=%t err=%v", request, created, err)
	}
	reused, created, err := manager.CreateRequest(first.ID, "session-a", "ws_x")
	if err != nil || created || reused.ID != request.ID {
		t.Fatalf("reused request = %#v created=%t err=%v", reused, created, err)
	}
}

func TestManagerBindsChallengeToSessionAndWorkspace(t *testing.T) {
	manager, _ := testManager()
	challenge, _, err := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{{"session-b", "ws_x"}, {"session-a", "ws_y"}} {
		if _, _, err := manager.CreateRequest(challenge.ID, input[0], input[1]); !errors.Is(err, ErrChallengeMismatch) {
			t.Fatalf("request %v err=%v", input, err)
		}
	}
}

func TestManagerAllowsOnlyOneActiveRequestPerSession(t *testing.T) {
	manager, _ := testManager()
	first, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	second, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm install"))
	if _, _, err := manager.CreateRequest(first.ID, "session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateRequest(second.ID, "session-a", "ws_x"); !errors.Is(err, ErrSessionRequestActive) {
		t.Fatalf("second active request err=%v", err)
	}
}

func TestManagerApprovalExactRetryAndOneShotConsumption(t *testing.T) {
	manager, _ := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	approved, err := manager.Approve(request.ID, "cli", "reviewed")
	if err != nil || approved.Status != StatusApproved || approved.RetryUntil.IsZero() {
		t.Fatalf("approved = %#v err=%v", approved, err)
	}
	unrelated, matched, err := manager.MatchApproved(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "git_status", Arguments: map[string]any{"workspace_id": "ws_x"}})
	if err != nil || matched || unrelated.ID != "" {
		t.Fatalf("unrelated match = %#v/%t/%v", unrelated, matched, err)
	}
	_, matched, err = manager.MatchApproved(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update --version v2.0.0"}})
	var mismatch *MismatchError
	if matched || !errors.As(err, &mismatch) || mismatch.RequestID != request.ID {
		t.Fatalf("mismatch = matched=%t err=%v typed=%#v", matched, err, mismatch)
	}
	matchedRequest, matched, err := manager.MatchApproved(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"command": "cgm update", "workspace_id": "ws_x"}})
	if err != nil || !matched || matchedRequest.ID != request.ID {
		t.Fatalf("exact match = %#v/%t/%v", matchedRequest, matched, err)
	}
	consumed, err := manager.Consume(request.ID)
	if err != nil || consumed.Status != StatusConsumed || consumed.ConsumedAt.IsZero() {
		t.Fatalf("consumed = %#v err=%v", consumed, err)
	}
	if _, err := manager.Consume(request.ID); !errors.Is(err, ErrRequestNotApproved) {
		t.Fatalf("second consume err=%v", err)
	}
	if _, matched, err := manager.MatchApproved(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update"}}); err != nil || matched {
		t.Fatalf("consumed grant remained active: matched=%t err=%v", matched, err)
	}
}

func TestManagerClaimApprovedIsAtomic(t *testing.T) {
	manager, _ := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	if _, err := manager.Approve(request.ID, "cli", "reviewed"); err != nil {
		t.Fatal(err)
	}
	input := RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update"}}
	start := make(chan struct{})
	type claimResult struct {
		request Request
		matched bool
		err     error
	}
	results := make(chan claimResult, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, matched, err := manager.ClaimApproved(input)
			results <- claimResult{request: value, matched: matched, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	claimed := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.matched {
			claimed++
			if result.request.Status != StatusConsumed || result.request.ConsumedAt.IsZero() {
				t.Fatalf("claimed request = %#v", result.request)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed = %d, want 1", claimed)
	}
}

func TestManagerClaimMismatchDoesNotConsumeApproval(t *testing.T) {
	manager, _ := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	if _, err := manager.Approve(request.ID, "cli", "reviewed"); err != nil {
		t.Fatal(err)
	}
	_, matched, err := manager.ClaimApproved(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update --version v2.0.0"}})
	var mismatch *MismatchError
	if matched || !errors.As(err, &mismatch) {
		t.Fatalf("mismatch claim = matched=%t err=%v", matched, err)
	}
	value, ok := manager.Get(request.ID)
	if !ok || value.Status != StatusApproved {
		t.Fatalf("approval was consumed by mismatch: %#v ok=%t", value, ok)
	}
}

func TestManagerCLICapabilityExactMismatchReplayAndExpiry(t *testing.T) {
	t.Run("exact and replay", func(t *testing.T) {
		manager, _ := testManager()
		challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
		request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
		if _, err := manager.Approve(request.ID, "test", ""); err != nil {
			t.Fatal(err)
		}
		claimed, capability, matched, err := manager.ClaimApprovedCLI(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update"}}, CLIInvocation{Program: "cgm", Args: []string{"update"}})
		if err != nil || !matched || capability == "" || claimed.Status != StatusConsumed {
			t.Fatalf("claim = %#v capability=%q matched=%t err=%v", claimed, capability, matched, err)
		}
		requestID, err := manager.ConsumeCLI(capability, []string{"update"})
		if err != nil || requestID != request.ID {
			t.Fatalf("consume = %q err=%v", requestID, err)
		}
		if _, err := manager.ConsumeCLI(capability, []string{"update"}); !errors.Is(err, ErrCapabilityNotFound) {
			t.Fatalf("replay err=%v", err)
		}
	})

	t.Run("mismatch preserves token", func(t *testing.T) {
		manager, _ := testManager()
		challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
		request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
		_, _ = manager.Approve(request.ID, "test", "")
		_, capability, matched, err := manager.ClaimApprovedCLI(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update"}}, CLIInvocation{Program: "cgm", Args: []string{"update"}})
		if err != nil || !matched {
			t.Fatalf("claim matched=%t err=%v", matched, err)
		}
		_, err = manager.ConsumeCLI(capability, []string{"update", "--version", "v2.0.0"})
		var mismatch *CapabilityMismatchError
		if !errors.As(err, &mismatch) || len(mismatch.Expected) != 1 || mismatch.Expected[0] != "update" {
			t.Fatalf("mismatch=%#v err=%v", mismatch, err)
		}
		if requestID, err := manager.ConsumeCLI(capability, []string{"update"}); err != nil || requestID != request.ID {
			t.Fatalf("exact after mismatch = %q err=%v", requestID, err)
		}
	})

	t.Run("expiry", func(t *testing.T) {
		manager, now := testManager()
		challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
		request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
		_, _ = manager.Approve(request.ID, "test", "")
		_, capability, _, err := manager.ClaimApprovedCLI(RetryInput{SessionID: "session-a", WorkspaceID: "ws_x", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_x", "command": "cgm update"}}, CLIInvocation{Program: "cgm", Args: []string{"update"}})
		if err != nil {
			t.Fatal(err)
		}
		*now = now.Add(DefaultCLICapabilityTTL)
		if _, err := manager.ConsumeCLI(capability, []string{"update"}); !errors.Is(err, ErrCapabilityExpired) {
			t.Fatalf("expired capability err=%v", err)
		}
	})
}

func TestManagerPendingAndApprovedExpiry(t *testing.T) {
	manager, now := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	*now = now.Add(DefaultRequestTTL)
	manager.PurgeExpired()
	expired, ok := manager.Get(request.ID)
	if !ok || expired.Status != StatusExpired {
		t.Fatalf("pending expiry = %#v ok=%t", expired, ok)
	}

	challenge, _, _ = manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	request, _, _ = manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	approved, err := manager.Approve(request.ID, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	*now = approved.RetryUntil
	manager.PurgeExpired()
	expired, ok = manager.Get(request.ID)
	if !ok || expired.Status != StatusExpired || expired.Reason != "approved retry window expired" {
		t.Fatalf("approved expiry = %#v ok=%t", expired, ok)
	}
}

func TestManagerChallengeExpiry(t *testing.T) {
	manager, now := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	*now = challenge.ExpiresAt
	if _, _, err := manager.CreateRequest(challenge.ID, "session-a", "ws_x"); !errors.Is(err, ErrChallengeExpired) {
		t.Fatalf("expired challenge err=%v", err)
	}
}

func TestManagerDenyCancelAndList(t *testing.T) {
	manager, _ := testManager()
	firstChallenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	first, _, _ := manager.CreateRequest(firstChallenge.ID, "session-a", "ws_x")
	denied, err := manager.Deny(first.ID, "cli", "not now")
	if err != nil || denied.Status != StatusDenied || denied.ResolvedBy != "cli" || denied.Reason != "not now" {
		t.Fatalf("denied = %#v err=%v", denied, err)
	}
	secondChallenge, _, _ := manager.CreateChallenge(testChallenge("session-b", "ws_y", "cgm install"))
	second, _, _ := manager.CreateRequest(secondChallenge.ID, "session-b", "ws_y")
	cancelled, err := manager.Cancel(second.ID, "mcp", "request context cancelled")
	if err != nil || cancelled.Status != StatusCancelled {
		t.Fatalf("cancelled = %#v err=%v", cancelled, err)
	}
	all := manager.List(Filter{})
	if len(all) != 2 {
		t.Fatalf("list = %#v", all)
	}
	if filtered := manager.List(Filter{WorkspaceID: "ws_x", Status: StatusDenied}); len(filtered) != 1 || filtered[0].ID != first.ID {
		t.Fatalf("filtered = %#v", filtered)
	}
}

func TestManagerPendingLimits(t *testing.T) {
	manager, _ := testManager()
	manager.pendingLimit = 2
	manager.workspacePendingLimit = 1
	first, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	if _, _, err := manager.CreateRequest(first.ID, "session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	second, _, _ := manager.CreateChallenge(testChallenge("session-b", "ws_x", "cgm install"))
	if _, _, err := manager.CreateRequest(second.ID, "session-b", "ws_x"); !errors.Is(err, ErrPendingLimit) {
		t.Fatalf("workspace pending limit err=%v", err)
	}
	third, _, _ := manager.CreateChallenge(testChallenge("session-b", "ws_y", "cgm install"))
	if _, _, err := manager.CreateRequest(third.ID, "session-b", "ws_y"); err != nil {
		t.Fatal(err)
	}
	fourth, _, _ := manager.CreateChallenge(testChallenge("session-c", "ws_z", "cgm update"))
	if _, _, err := manager.CreateRequest(fourth.ID, "session-c", "ws_z"); !errors.Is(err, ErrPendingLimit) {
		t.Fatalf("global pending limit err=%v", err)
	}
}

func TestManagerConcurrentRequestCreationCoalesces(t *testing.T) {
	manager, _ := testManager()
	challenge, _, err := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	start := make(chan struct{})
	type result struct {
		request Request
		created bool
		err     error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			request, created, err := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
			results <- result{request: request, created: created, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	createdCount := 0
	requestID := ""
	for item := range results {
		if item.err != nil {
			t.Fatal(item.err)
		}
		if item.created {
			createdCount++
		}
		if requestID == "" {
			requestID = item.request.ID
		} else if item.request.ID != requestID {
			t.Fatalf("request ids diverged: %s != %s", item.request.ID, requestID)
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestManagerConcurrentResolutionHasSingleWinner(t *testing.T) {
	manager, _ := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	request, _, _ := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, approve := range []bool{true, false} {
		wg.Add(1)
		go func(approve bool) {
			defer wg.Done()
			<-start
			if approve {
				_, err := manager.Approve(request.ID, "test", "")
				results <- err
				return
			}
			_, err := manager.Deny(request.ID, "test", "")
			results <- err
		}(approve)
	}
	close(start)
	wg.Wait()
	close(results)
	success, resolved := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrRequestResolved):
			resolved++
		default:
			t.Fatalf("unexpected resolution error: %v", err)
		}
	}
	if success != 1 || resolved != 1 {
		t.Fatalf("success/resolved = %d/%d", success, resolved)
	}
}

func TestManagerWaitWakesOnApproval(t *testing.T) {
	manager := NewManager("instance-test")
	manager.requestTTL = 5 * time.Second
	challenge, _, err := manager.CreateChallenge(testChallenge("session-a", "ws_x", "cgm update"))
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := manager.CreateRequest(challenge.ID, "session-a", "ws_x")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		_, _ = manager.Approve(request.ID, "test", "")
		close(done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resolved, err := manager.Wait(ctx, request.ID)
	if err != nil || resolved.Status != StatusApproved {
		t.Fatalf("wait = %#v err=%v", resolved, err)
	}
	<-done
}

func TestManagerKeepsPrivateBindingIdentity(t *testing.T) {
	manager, _ := testManager()
	challenge, _, _ := manager.CreateChallenge(testChallenge("raw-secret-session", "ws_x", "cgm update"))
	request, _, _ := manager.CreateRequest(challenge.ID, "raw-secret-session", "ws_x")
	if request.sessionID != "raw-secret-session" || request.challengeID != challenge.ID || request.Digest == "" {
		t.Fatalf("internal identity missing: %#v", request)
	}
}

func testManager() (*Manager, *time.Time) {
	manager := NewManager("instance-test")
	now := time.Unix(1_700_000_000, 0).UTC()
	manager.now = func() time.Time { return now }
	sequence := 0
	manager.newID = func(prefix string) (string, error) {
		sequence++
		return fmt.Sprintf("%s_%02d", prefix, sequence), nil
	}
	return manager, &now
}

func testChallenge(sessionID, workspaceID, command string) ChallengeInput {
	return ChallengeInput{
		SessionID: sessionID, SessionHash: "hash-" + sessionID, WorkspaceID: workspaceID, Source: "tunnel", TargetTool: "run_command",
		Arguments: map[string]any{"workspace_id": workspaceID, "command": command}, GuardCode: controlguard.CodeControlPlaneMutation, GuardReason: "control-plane mutation denied", Title: "Allow " + command,
	}
}
