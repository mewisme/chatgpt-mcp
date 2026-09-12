package config

import (
	"errors"
	"sync"
	"testing"
)

func TestRuntimeStoreSnapshotIsIsolated(t *testing.T) {
	cfg := Default()
	cfg.Server.Expose = ExposureConfig{Mode: ExposureInterfaces, Interfaces: []string{"Ethernet"}}
	cfg.Shell.Path = []string{"/trusted/bin"}
	store := NewRuntimeStore(cfg)
	snapshot := store.Snapshot()
	snapshot.Server.Expose.Interfaces[0] = "mutated"
	snapshot.Shell.Path[0] = "/mutated/bin"
	if got := store.Snapshot().Server.Expose.Interfaces[0]; got != "Ethernet" {
		t.Fatalf("stored config mutated through snapshot: %q", got)
	}
	if got := store.Snapshot().Shell.Path[0]; got != "/trusted/bin" {
		t.Fatalf("stored shell path mutated through snapshot: %q", got)
	}
}

func TestRuntimeStoreFailedUpdateDoesNotCommit(t *testing.T) {
	store := NewRuntimeStore(Default())
	expected := errors.New("persist failed")
	if _, err := store.Update(func(next Config) (Config, error) {
		next.Server.Port++
		return next, expected
	}); !errors.Is(err, expected) {
		t.Fatalf("update error = %v", err)
	}
	if got := store.Snapshot().Server.Port; got != Default().Server.Port {
		t.Fatalf("failed update committed port %d", got)
	}
}

func TestRuntimeStoreSerializesConcurrentUpdates(t *testing.T) {
	store := NewRuntimeStore(Default())
	const updates = 64
	var wg sync.WaitGroup
	for range updates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Update(func(next Config) (Config, error) {
				next.Server.Port++
				return next, nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got, want := store.Snapshot().Server.Port, Default().Server.Port+updates; got != want {
		t.Fatalf("port = %d, want %d", got, want)
	}
}
