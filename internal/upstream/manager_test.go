package upstream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerListIsDeterministic(t *testing.T) {
	manager := NewManager(nil)
	if err := manager.Add(Server{ID: "b", Name: "B", Transport: "http"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(Server{ID: "a", Name: "A", Transport: "http"}); err != nil {
		t.Fatal(err)
	}
	servers := manager.List()
	if len(servers) != 2 || servers[0].ID != "a" || servers[1].ID != "b" {
		t.Fatalf("unexpected order: %+v", servers)
	}
}

func TestManagerRollsBackFailedPersist(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(NewStore(filepath.Join(file, "upstream.json")))
	if err := manager.Add(Server{ID: "a", Name: "A", Transport: "http"}); err == nil {
		t.Fatal("expected persistence error")
	}
	if len(manager.List()) != 0 {
		t.Fatalf("failed add must roll back: %+v", manager.List())
	}
}
