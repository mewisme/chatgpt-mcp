package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/install"
)

type fakeResolver struct {
	latest   Release
	versions map[string]Release
}

func (f fakeResolver) Latest(context.Context) (Release, error) { return f.latest, nil }
func (f fakeResolver) Version(_ context.Context, version string) (Release, error) {
	release, ok := f.versions[version]
	if !ok {
		return Release{}, errors.New("release not found")
	}
	return release, nil
}

type fakeArtifactSource struct {
	binary string
	calls  *int
}

func (f fakeArtifactSource) Download(context.Context, Release) (Artifact, error) {
	(*f.calls)++
	return Artifact{Dir: filepath.Dir(f.binary), Binary: f.binary}, nil
}

func TestUpdaterApplyLatest(t *testing.T) {
	layout := updateTestLayout(t)
	installCurrentVersion(t, layout, "v1.0.0", "old")
	binary, artifactDir := updateTestBinary(t, "new")
	calls := 0
	updater := Updater{Resolver: fakeResolver{latest: Release{Version: "v1.1.0"}}, Downloader: fakeArtifactSource{binary: binary, calls: &calls}}
	result, err := updater.Apply(context.Background(), ApplyOptions{Layout: layout, CurrentVersion: "v1.0.0", NoAlias: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Target != "v1.1.0" || result.Downgrade || calls != 1 {
		t.Fatalf("result = %+v, calls = %d", result, calls)
	}
	version, _, err := install.CurrentVersion(layout)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.1.0" {
		t.Fatalf("current version = %q", version)
	}
	if _, err := os.Stat(filepath.Join(layout.Versions, "v1.0.0")); err != nil {
		t.Fatalf("previous version was removed: %v", err)
	}
	if _, err := os.Stat(artifactDir); !os.IsNotExist(err) {
		t.Fatalf("artifact directory was not cleaned up: %v", err)
	}
}

func TestUpdaterNoopsWhenLatestIsNotNewer(t *testing.T) {
	for _, latest := range []string{"v1.0.0", "v0.9.0"} {
		t.Run(latest, func(t *testing.T) {
			layout := updateTestLayout(t)
			installCurrentVersion(t, layout, "v1.0.0", "current")
			calls := 0
			updater := Updater{Resolver: fakeResolver{latest: Release{Version: latest}}, Downloader: fakeArtifactSource{calls: &calls}}
			result, err := updater.Apply(context.Background(), ApplyOptions{Layout: layout, CurrentVersion: "v1.0.0", NoAlias: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.Changed || calls != 0 {
				t.Fatalf("result = %+v, calls = %d", result, calls)
			}
		})
	}
}

func TestUpdaterExplicitVersionAllowsDowngrade(t *testing.T) {
	layout := updateTestLayout(t)
	installCurrentVersion(t, layout, "v1.2.0", "old")
	binary, _ := updateTestBinary(t, "downgrade")
	calls := 0
	resolver := fakeResolver{versions: map[string]Release{"v1.1.0": {Version: "v1.1.0"}}}
	result, err := (Updater{Resolver: resolver, Downloader: fakeArtifactSource{binary: binary, calls: &calls}}).Apply(context.Background(), ApplyOptions{Layout: layout, CurrentVersion: "v1.2.0", TargetVersion: "1.1.0", NoAlias: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.Downgrade || result.Target != "v1.1.0" || calls != 1 {
		t.Fatalf("result = %+v, calls = %d", result, calls)
	}
}

func TestUpdaterExplicitVersionRejectsResolverMismatch(t *testing.T) {
	layout := updateTestLayout(t)
	installCurrentVersion(t, layout, "v1.0.0", "current")
	resolver := fakeResolver{versions: map[string]Release{"v1.1.0": {Version: "v1.2.0"}}}
	if _, err := (Updater{Resolver: resolver}).Apply(context.Background(), ApplyOptions{Layout: layout, CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0", NoAlias: true}); err == nil {
		t.Fatal("resolver version mismatch was accepted")
	}
}

func TestUpdaterRefusesDevelopmentBuild(t *testing.T) {
	_, err := (Updater{Resolver: fakeResolver{}}).Apply(context.Background(), ApplyOptions{CurrentVersion: "dev"})
	if !errors.Is(err, ErrDevelopmentUpdate) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdaterRefusesStaleRunningVersion(t *testing.T) {
	layout := updateTestLayout(t)
	installCurrentVersion(t, layout, "v1.1.0", "current")
	_, err := (Updater{Resolver: fakeResolver{}}).Apply(context.Background(), ApplyOptions{Layout: layout, CurrentVersion: "v1.0.0"})
	if !errors.Is(err, ErrCurrentVersionMismatch) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdaterPreservesMissingAlias(t *testing.T) {
	layout := updateTestLayout(t)
	installCurrentVersion(t, layout, "v1.0.0", "old")
	binary, _ := updateTestBinary(t, "new")
	calls := 0
	updater := Updater{Resolver: fakeResolver{latest: Release{Version: "v1.1.0"}}, Downloader: fakeArtifactSource{binary: binary, calls: &calls}}
	if _, err := updater.Apply(context.Background(), ApplyOptions{Layout: layout, CurrentVersion: "v1.0.0", NoAlias: true}); err != nil {
		t.Fatal(err)
	}
	status, err := install.StatusAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != install.AliasMissing {
		t.Fatalf("alias state = %q", status.State)
	}
}

func updateTestLayout(t *testing.T) install.Layout {
	t.Helper()
	root := filepath.Join(t.TempDir(), "install")
	layout, err := install.NewLayout(root, filepath.Join(root, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func updateTestBinary(t *testing.T, content string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "chatgpt-mcp")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

func installCurrentVersion(t *testing.T, layout install.Layout, version, content string) {
	t.Helper()
	binary, _ := updateTestBinary(t, content)
	if _, err := install.Install(install.Options{Layout: layout, Version: version, Source: binary, NoAlias: true}); err != nil {
		t.Fatal(err)
	}
}
