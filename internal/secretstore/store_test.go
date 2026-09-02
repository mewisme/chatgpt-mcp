package secretstore

import (
	"errors"
	"testing"
)

func TestStoreMemoryRoundTrip(t *testing.T) {
	cleanup := UseMemoryForTesting()
	defer cleanup()
	store := New(t.TempDir())
	name := Name("tunnel", "runtime")
	if err := store.Set(name, "secret"); err != nil {
		t.Fatal(err)
	}
	if value, err := store.Get(name); err != nil || value != "secret" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if err := store.Set(name, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestStoreApplyRollsBack(t *testing.T) {
	backend := &failingBackend{memoryBackend: newMemoryBackend(), failAccount: Name("second")}
	store := &Store{service: "test", backend: backend}
	if err := store.Set(Name("first"), "old"); err != nil {
		t.Fatal(err)
	}
	err := store.Apply([]Change{{Name: Name("first"), Value: "new"}, {Name: Name("second"), Value: "value"}})
	if err == nil {
		t.Fatal("expected failure")
	}
	if value, getErr := store.Get(Name("first")); getErr != nil || value != "old" {
		t.Fatalf("rollback value=%q err=%v", value, getErr)
	}
}

type failingBackend struct {
	*memoryBackend
	failAccount string
}

func (f *failingBackend) Set(service, account, value string) error {
	if account == f.failAccount {
		return errors.New("boom")
	}
	return f.memoryBackend.Set(service, account, value)
}
