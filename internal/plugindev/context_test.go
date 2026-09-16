package plugindev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAtRequiresDevelopmentVersion(t *testing.T) {
	root := fakeRepo(t)
	ctx, err := DetectAt("v0.2.24", root, envMap(EnvRoot, root))
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Enabled || ctx.Root != "" || ctx.Mode != ModeAuto {
		t.Fatalf("release build enabled dev mode: %+v", ctx)
	}
}

func TestDetectAtFindsRepoFromNestedCwd(t *testing.T) {
	root := fakeRepo(t)
	nested := filepath.Join(root, "internal", "plugindev")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, err := DetectAt("dev", nested, envMap())
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.Enabled || ctx.Root != root || ctx.Mode != ModeAuto {
		t.Fatalf("ctx=%+v root=%s", ctx, root)
	}
}

func TestDetectAtHonorsOffAndRebuild(t *testing.T) {
	root := fakeRepo(t)
	off, err := DetectAt("dev", root, envMap(EnvPlugins, "off"))
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled || off.Mode != ModeOff {
		t.Fatalf("off=%+v", off)
	}
	rebuild, err := DetectAt("(devel)", root, envMap(EnvPlugins, "rebuild", EnvRoot, root))
	if err != nil {
		t.Fatal(err)
	}
	if !rebuild.Enabled || rebuild.Mode != ModeRebuild || rebuild.Root != root {
		t.Fatalf("rebuild=%+v", rebuild)
	}
}

func TestDetectAtRejectsInvalidModeAndRoot(t *testing.T) {
	root := fakeRepo(t)
	if _, err := DetectAt("dev", root, envMap(EnvPlugins, "yes")); err == nil || !strings.Contains(err.Error(), EnvPlugins) {
		t.Fatalf("mode err=%v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := DetectAt("dev", root, envMap(EnvRoot, missing)); err == nil || !strings.Contains(err.Error(), EnvRoot) {
		t.Fatalf("missing root err=%v", err)
	}
	file := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.WriteFile(file, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DetectAt("dev", root, envMap(EnvRoot, file)); err == nil || !strings.Contains(err.Error(), EnvRoot) {
		t.Fatalf("file root err=%v", err)
	}
}

func TestDetectAtRejectsUnrelatedDirectory(t *testing.T) {
	ctx, err := DetectAt("dev", t.TempDir(), envMap())
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Enabled || ctx.Root != "" {
		t.Fatalf("trusted arbitrary directory: %+v", ctx)
	}
	wrong := t.TempDir()
	if err := os.WriteFile(filepath.Join(wrong, "go.mod"), []byte("module example.com/other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DetectAt("dev", wrong, envMap(EnvRoot, wrong)); err == nil || !strings.Contains(err.Error(), EnvRoot) {
		t.Fatalf("wrong module err=%v", err)
	}
}

func TestDetectAtRejectsParentOverride(t *testing.T) {
	root := fakeRepo(t)
	if _, err := DetectAt("dev", root, envMap(EnvRoot, filepath.Join(root, ".."))); err == nil || !strings.Contains(err.Error(), EnvRoot) {
		t.Fatalf("parent override err=%v", err)
	}
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(modulePrefix+"\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plugins", "registry"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugins", "workflow.json"), []byte(`{"schema":1,"plugins":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugins", "registry", "index.json"), []byte(`{"schema":1,"plugins":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return mustAbs(t, root)
}

func envMap(kv ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		values[kv[i]] = kv[i+1]
	}
	return func(key string) string { return values[key] }
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
