package plugindev

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestBootstrapInstallsWorkflowCorePlugins(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	repo := fakeRepo(t)
	addCoreSource(t, repo, "fixture")
	writeWorkflow(t, repo, coreSpec{ID: "fixture", Required: true, Enabled: true}, coreSpec{ID: "skip-me"})
	builds := 0
	pluginBuilder = func(_, id, output string) error {
		builds++
		if id != "fixture" {
			t.Fatalf("built non-core plugin %s", id)
		}
		return writeStubArtifact(output, id)
	}
	t.Cleanup(func() { pluginBuilder = workflowBuild })
	ctx := Context{Enabled: true, Root: repo, Mode: ModeAuto}
	layout, err := Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(layout.ConfigRoot, filepath.Join(".cgm", "dev", "plugins")) {
		t.Fatalf("layout=%s", layout.ConfigRoot)
	}
	store, err := plugin.NewStore(layout, plugin.RuntimeContext{CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Installed("fixture", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := ReadProvenance(installed)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.Origin != plugin.RegistryLocalDev || provenance.SourceFingerprint == "" {
		t.Fatalf("provenance=%+v", provenance)
	}
	global := plugin.DefaultLayout()
	if _, err := os.Stat(global.LockPath()); !os.IsNotExist(err) {
		t.Fatalf("production lock mutated: %v", err)
	}
	if _, err := Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("cache not reused: builds=%d", builds)
	}
	ctx.Mode = ModeRebuild
	if _, err := Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if builds != 2 {
		t.Fatalf("rebuild did not bypass cache: builds=%d", builds)
	}
}

func TestBootstrapExplicitBundleWins(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	repo := fakeRepo(t)
	t.Setenv("CHATGPT_MCP_PLUGIN_BUNDLE", t.TempDir())
	pluginBuilder = func(_, _, _ string) error {
		t.Fatal("builder ran despite explicit bundle")
		return nil
	}
	t.Cleanup(func() { pluginBuilder = workflowBuild })
	layout, err := Bootstrap(Context{Enabled: true, Root: repo, Mode: ModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	if layout.ConfigRoot != plugin.DefaultLayout().ConfigRoot {
		t.Fatalf("bundle did not win: %s", layout.ConfigRoot)
	}
}

func TestBootstrapRequiredFailureIsFatal(t *testing.T) {
	repo := fakeRepo(t)
	addCoreSource(t, repo, "fixture")
	writeWorkflow(t, repo, coreSpec{ID: "fixture", Required: true, Enabled: true})
	pluginBuilder = func(_, _, _ string) error { return errors.New("compile failed") }
	t.Cleanup(func() { pluginBuilder = workflowBuild })
	_, err := Bootstrap(Context{Enabled: true, Root: repo, Mode: ModeAuto})
	if err == nil || !strings.Contains(err.Error(), "fixture") || !strings.Contains(err.Error(), "build") {
		t.Fatalf("err=%v", err)
	}
}

func TestBootstrapOptionalFailureIsDegraded(t *testing.T) {
	repo := fakeRepo(t)
	addCoreSource(t, repo, "fixture")
	writeWorkflow(t, repo, coreSpec{ID: "fixture", Required: false, Enabled: true})
	pluginBuilder = func(_, _, _ string) error { return errors.New("compile failed") }
	t.Cleanup(func() { pluginBuilder = workflowBuild })
	if _, err := Bootstrap(Context{Enabled: true, Root: repo, Mode: ModeAuto}); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapSkipsDisallowedPlatform(t *testing.T) {
	repo := fakeRepo(t)
	addCoreSource(t, repo, "fixture")
	writeWorkflow(t, repo, coreSpec{ID: "fixture", Required: true, Enabled: true, Platforms: []string{"plan9/arm"}})
	pluginBuilder = func(_, _, _ string) error {
		t.Fatal("builder ran for disallowed platform")
		return nil
	}
	t.Cleanup(func() { pluginBuilder = workflowBuild })
	if _, err := Bootstrap(Context{Enabled: true, Root: repo, Mode: ModeAuto}); err != nil {
		t.Fatal(err)
	}
}

func TestSkipAutoDevEnvHonorsBundleAndCompletion(t *testing.T) {
	if !skipAutoDevEnv(envMap("CHATGPT_MCP_PLUGIN_BUNDLE", "/tmp/bundle")) {
		t.Fatal("explicit bundle allowed auto-dev")
	}
	if !cheapArgs([]string{"--version"}) || !cheapArgs([]string{"help"}) || !cheapArgs([]string{"completion", "bash"}) {
		t.Fatal("cheap invocation missed")
	}
	if cheapArgs([]string{"plugin", "list"}) || cheapArgs([]string{"serve"}) {
		t.Fatal("runtime command treated as cheap")
	}
}

type coreSpec struct {
	ID        string
	Required  bool
	Enabled   bool
	Platforms []string
}

func addCoreSource(t *testing.T, repo, id string) {
	t.Helper()
	dir := filepath.Join(repo, "plugins", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"id":"`+id+`","version":"1.0.0"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeWorkflow(t *testing.T, repo string, specs ...coreSpec) {
	t.Helper()
	plugins := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		item := map[string]any{"id": spec.ID, "build": map[string]any{"command": []string{"true"}}}
		if spec.ID != "skip-me" {
			core := map[string]any{"required": spec.Required, "enabled": spec.Enabled}
			if len(spec.Platforms) > 0 {
				core["platforms"] = spec.Platforms
			}
			item["core"] = core
		}
		plugins = append(plugins, item)
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "plugins": plugins})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "plugins", "workflow.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeStubArtifact(output, id string) error {
	if err := os.MkdirAll(output, 0o700); err != nil {
		return err
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	artifact := id + "-1.0.0-" + strings.ReplaceAll(platform, "/", "-") + ".zip"
	zipPath := filepath.Join(output, artifact)
	if err := writeZipFile(zipPath, "bin/"+id, []byte("payload")); err != nil {
		return err
	}
	digest, err := fileDigest(zipPath)
	if err != nil {
		return err
	}
	manifest := plugin.Manifest{
		Schema: plugin.ManifestSchema, ID: plugin.PluginID(id), Name: id, Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "runtime",
		Provides: []plugin.Capability{plugin.Capability("tunnel/" + id)}, Permissions: []plugin.Permission{plugin.PermissionProcessExecute},
		Scopes: []plugin.PluginScope{plugin.ScopeGlobal},
		Platforms: map[string]plugin.PlatformArtifact{
			platform: {Artifact: artifact, SHA256: digest, Archive: "zip", Entrypoint: "bin/" + id},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(output, id+"-1.0.0.json"), append(data, '\n'), 0o600)
}

func writeZipFile(path, name string, payload []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	if _, err := entry.Write(payload); err != nil {
		return err
	}
	return writer.Close()
}
