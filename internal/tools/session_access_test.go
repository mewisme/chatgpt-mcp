package tools

import (
	"sync"
	"testing"
	"time"
)

func TestSessionWorkspaceAccessAllowsMultipleWorkspaces(t *testing.T) {
	manager := NewSessionWorkspaceAccessManager()
	first, decision, count, err := manager.CheckOrGrant("session-a", "ws_x")
	if err != nil || decision != SessionWorkspaceAccessNew || count != 1 || first.WorkspaceID != "ws_x" {
		t.Fatalf("first grant = %#v/%s/%d/%v", first, decision, count, err)
	}
	second, decision, count, err := manager.CheckOrGrant("session-a", "ws_y")
	if err != nil || decision != SessionWorkspaceAccessNew || count != 2 || second.WorkspaceID != "ws_y" {
		t.Fatalf("second grant = %#v/%s/%d/%v", second, decision, count, err)
	}
	_, decision, count, err = manager.CheckOrGrant("session-a", "ws_x")
	if err != nil || decision != SessionWorkspaceAccessExisting || count != 2 {
		t.Fatalf("existing grant = %s/%d/%v", decision, count, err)
	}
	access, ok := manager.Lookup("session-a")
	if !ok || len(access.Workspaces) != 2 || access.Workspaces["ws_x"].WorkspaceID != "ws_x" || access.Workspaces["ws_y"].WorkspaceID != "ws_y" {
		t.Fatalf("access = %#v ok=%t", access, ok)
	}
}

func TestSessionWorkspaceAccessAllowsManySessionsForOneWorkspace(t *testing.T) {
	manager := NewSessionWorkspaceAccessManager()
	for _, sessionID := range []string{"session-a", "session-b", "session-c"} {
		grant, decision, count, err := manager.CheckOrGrant(sessionID, "ws_x")
		if err != nil || decision != SessionWorkspaceAccessNew || count != 1 || grant.WorkspaceID != "ws_x" {
			t.Fatalf("grant %s = %#v/%s/%d/%v", sessionID, grant, decision, count, err)
		}
	}
}

func TestSessionWorkspaceAccessConcurrentGrantsAreAtomic(t *testing.T) {
	manager := NewSessionWorkspaceAccessManager()
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, workspaceID := range []string{"ws_x", "ws_y"} {
		wg.Add(1)
		go func(workspaceID string) {
			defer wg.Done()
			<-start
			_, _, _, err := manager.CheckOrGrant("session-a", workspaceID)
			errs <- err
		}(workspaceID)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	access, ok := manager.Lookup("session-a")
	if !ok || len(access.Workspaces) != 2 {
		t.Fatalf("access = %#v ok=%t", access, ok)
	}
}

func TestSessionWorkspaceAccessUpdatesLastSeen(t *testing.T) {
	manager := NewSessionWorkspaceAccessManager()
	first := time.Unix(10, 0)
	second := time.Unix(20, 0)
	manager.now = func() time.Time { return first }
	grant, _, _, err := manager.CheckOrGrant("session-a", "ws_x")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return second }
	grant, _, _, err = manager.CheckOrGrant("session-a", "ws_x")
	if err != nil {
		t.Fatal(err)
	}
	if !grant.GrantedAt.Equal(first) || !grant.LastSeen.Equal(second) {
		t.Fatalf("grant = %#v", grant)
	}
	access, ok := manager.Lookup("session-a")
	if !ok || !access.CreatedAt.Equal(first) || !access.LastSeen.Equal(second) {
		t.Fatalf("access = %#v ok=%t", access, ok)
	}
}

func TestSessionWorkspaceAccessExpiresIdleSession(t *testing.T) {
	manager := NewSessionWorkspaceAccessManager()
	manager.ttl = time.Hour
	now := time.Unix(100, 0)
	manager.now = func() time.Time { return now }
	if _, _, _, err := manager.CheckOrGrant("session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if removed := manager.PurgeExpired(); removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	if _, ok := manager.Lookup("session-a"); ok {
		t.Fatal("expired session access remained")
	}
	if _, decision, count, err := manager.CheckOrGrant("session-a", "ws_y"); err != nil || decision != SessionWorkspaceAccessNew || count != 1 {
		t.Fatalf("grant after expiry = %s/%d/%v", decision, count, err)
	}
}

func TestSessionWorkspaceAccessAnyWorkspaceRefreshesSessionExpiry(t *testing.T) {
	manager := NewSessionWorkspaceAccessManager()
	manager.ttl = time.Hour
	now := time.Unix(100, 0)
	manager.now = func() time.Time { return now }
	if _, _, _, err := manager.CheckOrGrant("session-a", "ws_x"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(45 * time.Minute)
	if _, _, _, err := manager.CheckOrGrant("session-a", "ws_y"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(45 * time.Minute)
	if removed := manager.PurgeExpired(); removed != 0 {
		t.Fatalf("active access removed = %d", removed)
	}
	access, ok := manager.Lookup("session-a")
	if !ok || len(access.Workspaces) != 2 {
		t.Fatalf("access = %#v ok=%t", access, ok)
	}
}
