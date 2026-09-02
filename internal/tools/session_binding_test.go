package tools

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSessionWorkspaceBinderAllowsManySessionsForOneWorkspace(t *testing.T) {
	binder := NewSessionWorkspaceBinder()
	for _, sessionID := range []string{"session-a", "session-b", "session-c"} {
		binding, decision, err := binder.CheckOrBind(sessionID, "ws_x")
		if err != nil || decision != SessionBindingNew || binding.WorkspaceID != "ws_x" {
			t.Fatalf("bind %s = %#v/%s/%v", sessionID, binding, decision, err)
		}
	}
	if _, decision, err := binder.CheckOrBind("session-a", "ws_x"); err != nil || decision != SessionBindingExisting {
		t.Fatalf("existing binding = %s/%v", decision, err)
	}
}

func TestSessionWorkspaceBinderDeniesWorkspaceSwitch(t *testing.T) {
	binder := NewSessionWorkspaceBinder()
	if _, _, err := binder.CheckOrBind("session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	binding, decision, err := binder.CheckOrBind("session-a", "ws_y")
	if !errors.Is(err, ErrSessionWorkspaceMismatch) || decision != SessionBindingDenied || binding.WorkspaceID != "ws_x" {
		t.Fatalf("switch = %#v/%s/%v", binding, decision, err)
	}
}

func TestSessionWorkspaceBinderFirstBindIsAtomic(t *testing.T) {
	binder := NewSessionWorkspaceBinder()
	start := make(chan struct{})
	type result struct {
		workspace string
		decision  SessionBindingDecision
		err       error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, workspaceID := range []string{"ws_x", "ws_y"} {
		wg.Add(1)
		go func(workspaceID string) {
			defer wg.Done()
			<-start
			_, decision, err := binder.CheckOrBind("session-a", workspaceID)
			results <- result{workspace: workspaceID, decision: decision, err: err}
		}(workspaceID)
	}
	close(start)
	wg.Wait()
	close(results)
	allowed, denied := 0, 0
	for item := range results {
		if item.err == nil && item.decision == SessionBindingNew {
			allowed++
			continue
		}
		if errors.Is(item.err, ErrSessionWorkspaceMismatch) && item.decision == SessionBindingDenied {
			denied++
			continue
		}
		t.Fatalf("unexpected result = %#v", item)
	}
	if allowed != 1 || denied != 1 {
		t.Fatalf("allowed/denied = %d/%d", allowed, denied)
	}
}

func TestSessionWorkspaceBinderUpdatesLastSeen(t *testing.T) {
	binder := NewSessionWorkspaceBinder()
	first := time.Unix(10, 0)
	second := time.Unix(20, 0)
	binder.now = func() time.Time { return first }
	binding, _, err := binder.CheckOrBind("session-a", "ws_x")
	if err != nil {
		t.Fatal(err)
	}
	binder.now = func() time.Time { return second }
	binding, _, err = binder.CheckOrBind("session-a", "ws_x")
	if err != nil {
		t.Fatal(err)
	}
	if !binding.BoundAt.Equal(first) || !binding.LastSeen.Equal(second) {
		t.Fatalf("binding = %#v", binding)
	}
}

func TestSessionWorkspaceBinderExpiresIdleBinding(t *testing.T) {
	binder := NewSessionWorkspaceBinder()
	binder.ttl = time.Hour
	now := time.Unix(100, 0)
	binder.now = func() time.Time { return now }
	if _, _, err := binder.CheckOrBind("session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if removed := binder.PurgeExpired(); removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	if _, ok := binder.Lookup("session-a"); ok {
		t.Fatal("expired binding remained")
	}
	if _, decision, err := binder.CheckOrBind("session-a", "ws_y"); err != nil || decision != SessionBindingNew {
		t.Fatalf("rebind after expiry = %s/%v", decision, err)
	}
}

func TestSessionWorkspaceBinderActivityRefreshesExpiry(t *testing.T) {
	binder := NewSessionWorkspaceBinder()
	binder.ttl = time.Hour
	now := time.Unix(100, 0)
	binder.now = func() time.Time { return now }
	if _, _, err := binder.CheckOrBind("session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(45 * time.Minute)
	if _, _, err := binder.CheckOrBind("session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(45 * time.Minute)
	if removed := binder.PurgeExpired(); removed != 0 {
		t.Fatalf("active binding removed = %d", removed)
	}
	if binding, ok := binder.Lookup("session-a"); !ok || binding.WorkspaceID != "ws_x" {
		t.Fatalf("binding = %#v ok=%t", binding, ok)
	}
}
