package plugindev

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestFingerprintStableThenChangesWithPluginSource(t *testing.T) {
	root := repoRoot(t)
	first, err := Fingerprint(root, "caveman", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(root, "caveman", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("fingerprint unstable: %s %s", first, second)
	}
	probe := filepath.Join(root, "plugins", "caveman", "fingerprint_probe_hook.txt")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(probe) })
	changed, err := Fingerprint(root, "caveman", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("plugin source edit did not change fingerprint")
	}
}

func TestFingerprintIncludesSharedModuleSource(t *testing.T) {
	root := repoRoot(t)
	before, err := Fingerprint(root, "secure-mcp-tunnel", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(root, "internal", "runtimeplugin", "fingerprint_probe_hook.txt")
	if err := os.WriteFile(shared, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(shared) })
	after, err := Fingerprint(root, "secure-mcp-tunnel", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("shared Go dependency edit did not change fingerprint")
	}
}

func TestFingerprintChangesWhenAdminUISourceChanges(t *testing.T) {
	root := repoRoot(t)
	before, err := Fingerprint(root, "admin-ui", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(root, "plugins", "admin-ui", "fingerprint_probe_hook.txt")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(probe) })
	after, err := Fingerprint(root, "admin-ui", "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("admin-ui source edit did not change fingerprint")
	}
}

func TestCacheDirStaysInDevTree(t *testing.T) {
	root := fakeRepo(t)
	dir, err := CacheDir(root, "tui", "linux/amd64", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".cgm", "dev", "builds", "linux-amd64", "tui", "abc123")
	if dir != want {
		t.Fatalf("cache dir=%s want=%s", dir, want)
	}
	if _, err := CacheDir(root, "tui", "linux/amd64", "../escape"); err == nil {
		t.Fatal("escaped fingerprint accepted")
	}
}

func TestCachedExactMatchAndRebuildBypass(t *testing.T) {
	root := fakeRepo(t)
	dir, err := CacheDir(root, "tui", "linux/amd64", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if Cached(dir, "abc123") {
		t.Fatal("empty cache reported a hit")
	}
	if err := WriteCacheMarker(dir, "abc123"); err != nil {
		t.Fatal(err)
	}
	if !Cached(dir, "abc123") {
		t.Fatal("exact fingerprint missed")
	}
	if Cached(dir, "other") {
		t.Fatal("mismatched fingerprint reused")
	}
	if (Context{Mode: ModeAuto}).SkipCache() {
		t.Fatal("auto skipped cache")
	}
	if !(Context{Mode: ModeRebuild}).SkipCache() {
		t.Fatal("rebuild did not bypass cache")
	}
}

func TestConcurrentPromoteDoesNotCorrupt(t *testing.T) {
	root := fakeRepo(t)
	dest := filepath.Join(root, ".cgm", "dev", "builds", "linux-amd64", "tui", "abc123")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			lock, err := AcquireBootstrapLock(root)
			if err != nil {
				errs <- err
				return
			}
			defer func() { _ = lock.Release() }()
			stage, err := StageDir(filepath.Dir(dest))
			if err != nil {
				errs <- err
				return
			}
			if err := os.WriteFile(filepath.Join(stage, "payload"), []byte(strconv.Itoa(n)), 0o600); err != nil {
				errs <- err
				return
			}
			if err := WriteCacheMarker(stage, "abc123"); err != nil {
				errs <- err
				return
			}
			if err := PromoteDir(stage, dest); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "payload"))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := strconv.Atoi(string(data)); err != nil || n < 0 || n > 7 {
		t.Fatalf("corrupted payload %q", data)
	}
	if !Cached(dest, "abc123") {
		t.Fatal("promoted cache missing fingerprint marker")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := DetectAt("dev", cwd, envMap())
	if err != nil || !ctx.Enabled {
		t.Fatalf("test must run inside the source repo: %+v %v", ctx, err)
	}
	return ctx.Root
}
