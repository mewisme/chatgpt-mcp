package plugindev

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/oslock"
	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestLayoutStaysInsideRepoDevTree(t *testing.T) {
	repo := fakeRepo(t)
	testutil.UseConfigRoot(t, t.TempDir())
	layout, err := Layout(repo, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(repo, ".cgm", "dev", "plugins", "linux-amd64")
	if layout.ConfigRoot != want {
		t.Fatalf("config root=%s want=%s", layout.ConfigRoot, want)
	}
	global := plugin.DefaultLayout()
	if layout.ConfigRoot == global.ConfigRoot || layout.DataRoot == global.DataRoot {
		t.Fatalf("dev layout collided with release store: dev=%#v global=%#v", layout, global)
	}
}

func TestProvenanceRoundTrip(t *testing.T) {
	repo := fakeRepo(t)
	installed := plugin.InstalledPlugin{Root: t.TempDir()}
	in := Provenance{Schema: provenanceSchema, Origin: plugin.RegistryLocalDev, RepoRoot: repo, PluginID: "tui", SourceFingerprint: "abc", ArtifactDigest: "sha256:dead", BuiltAt: time.Unix(1, 0).UTC()}
	if err := WriteProvenance(installed, in); err != nil {
		t.Fatal(err)
	}
	got, err := ReadProvenance(installed)
	if err != nil {
		t.Fatal(err)
	}
	if got.PluginID != "tui" || got.RepoRoot != repo || got.Origin != plugin.RegistryLocalDev {
		t.Fatalf("got=%+v", got)
	}
}

func TestBootstrapLockIsExclusive(t *testing.T) {
	repo := fakeRepo(t)
	lock, err := AcquireBootstrapLock(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Release() })
	path, err := BootstrapLockPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := oslock.TryAcquire(path, oslock.Exclusive); err != nil || ok {
		t.Fatalf("second lock ok=%t err=%v", ok, err)
	}
}

func TestLayoutRejectsUnverifiedRoot(t *testing.T) {
	if _, err := Layout(t.TempDir(), runtime.GOOS, runtime.GOARCH); err == nil {
		t.Fatal("unverified root produced a layout")
	}
}

func TestRemovingDevDirResetsLayoutPaths(t *testing.T) {
	repo := fakeRepo(t)
	layout, err := Layout(repo, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.ConfigRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ConfigRoot, "marker"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(repo, ".cgm")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.ConfigRoot); !os.IsNotExist(err) {
		t.Fatalf("dev state survived reset: %v", err)
	}
}
