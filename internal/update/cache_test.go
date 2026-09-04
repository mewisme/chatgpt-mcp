package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "update.json")
	checkedAt := time.Date(2026, 9, 4, 12, 0, 0, 0, time.FixedZone("UTC+7", 7*60*60))
	if err := WriteCache(path, "1.2.3", checkedAt); err != nil {
		t.Fatal(err)
	}
	cache, err := ReadCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if cache.Schema != CacheSchema || cache.Latest != "v1.2.3" || !cache.CheckedAt.Equal(checkedAt.UTC()) {
		t.Fatalf("cache = %+v", cache)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("cache permissions = %v err=%v", info.Mode().Perm(), err)
	}
}

func TestReadFreshCacheRecomputesStatusForCurrentVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if err := WriteCache(path, "v1.2.0", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		current string
		status  Status
	}{
		{"v1.1.0", StatusAvailable},
		{"v1.2.0", StatusUpToDate},
		{"v1.3.0", StatusAhead},
		{"dev", StatusDevelopment},
	}
	for _, test := range tests {
		cached, ok, err := ReadFreshCache(path, test.current, now, DefaultCacheTTL)
		if err != nil {
			t.Fatal(err)
		}
		if !ok || cached.Status != test.status || cached.Latest != "v1.2.0" {
			t.Fatalf("current=%s cached=%+v ok=%v", test.current, cached, ok)
		}
	}
}

func TestReadFreshCacheRejectsStaleAndFutureEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	for name, checkedAt := range map[string]time.Time{
		"stale":  now.Add(-DefaultCacheTTL - time.Second),
		"future": now.Add(time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			if err := WriteCache(path, "v1.2.0", checkedAt); err != nil {
				t.Fatal(err)
			}
			if cached, ok, err := ReadFreshCache(path, "v1.0.0", now, DefaultCacheTTL); err != nil || ok {
				t.Fatalf("cached=%+v ok=%v err=%v", cached, ok, err)
			}
		})
	}
}

func TestReadFreshCacheMissingIsNotAnError(t *testing.T) {
	cached, ok, err := ReadFreshCache(filepath.Join(t.TempDir(), "missing.json"), "v1.0.0", time.Now(), DefaultCacheTTL)
	if err != nil || ok || cached.Status != "" {
		t.Fatalf("cached=%+v ok=%v err=%v", cached, ok, err)
	}
	if _, err := ReadCache(filepath.Join(t.TempDir(), "missing.json")); !errors.Is(err, ErrCacheNotFound) {
		t.Fatalf("error = %v", err)
	}
}
