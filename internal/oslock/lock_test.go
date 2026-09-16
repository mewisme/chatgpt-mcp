//go:build linux || darwin || windows

package oslock

import (
	"path/filepath"
	"testing"
)

func TestSharedAndExclusiveLocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	first, err := Acquire(path, Shared)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, ok, err := TryAcquire(path, Shared)
	if err != nil || !ok {
		t.Fatalf("second shared lock ok=%t err=%v", ok, err)
	}
	defer second.Release()
	if lock, ok, err := TryAcquire(path, Exclusive); err != nil || ok || lock != nil {
		t.Fatalf("exclusive lock while shared held lock=%#v ok=%t err=%v", lock, ok, err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	exclusive, ok, err := TryAcquire(path, Exclusive)
	if err != nil || !ok {
		t.Fatalf("exclusive lock ok=%t err=%v", ok, err)
	}
	defer exclusive.Release()
}
