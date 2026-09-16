//go:build linux || darwin || windows

package state

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func isolatePresenceRoot(t *testing.T) {
	t.Helper()
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	if err := configformat.SetRootPath(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = configformat.SetRootPath("") })
}

func TestTUIPresenceNoHolder(t *testing.T) {
	isolatePresenceRoot(t)
	active, err := TUIReviewerActive()
	if err != nil || active {
		t.Fatalf("active=%t err=%v", active, err)
	}
}

func TestTUIPresenceOneHolder(t *testing.T) {
	isolatePresenceRoot(t)
	hold, err := HoldTUIPresence()
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Release()
	active, err := TUIReviewerActive()
	if err != nil || !active {
		t.Fatalf("active=%t err=%v", active, err)
	}
}

func TestTUIPresenceMultipleHolders(t *testing.T) {
	isolatePresenceRoot(t)
	first, err := HoldTUIPresence()
	if err != nil {
		t.Fatal(err)
	}
	second, err := HoldTUIPresence()
	if err != nil {
		t.Fatal(err)
	}
	active, err := TUIReviewerActive()
	if err != nil || !active {
		t.Fatalf("two holders active=%t err=%v", active, err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	active, err = TUIReviewerActive()
	if err != nil || !active {
		t.Fatalf("one holder active=%t err=%v", active, err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	active, err = TUIReviewerActive()
	if err != nil || active {
		t.Fatalf("released active=%t err=%v", active, err)
	}
}
