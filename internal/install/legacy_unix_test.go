//go:build !windows

package install

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanupLegacyInstallationsRemovesVerifiedStandaloneAndAlias(t *testing.T) {
	layout := testLayout(t)
	legacyDir := t.TempDir()
	legacyBinary := filepath.Join(legacyDir, layout.BinaryName)
	copyTestExecutable(t, legacyBinary)
	legacyAlias := filepath.Join(legacyDir, layout.AliasName)
	if err := os.Symlink(layout.BinaryName, legacyAlias); err != nil {
		t.Fatal(err)
	}
	setLegacyTestEnvironment(t, legacyDir)
	result, err := CleanupLegacyInstallations(LegacyCleanupOptions{Layout: layout})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 1 || !samePath(result.Removed[0].Path, legacyBinary) {
		t.Fatalf("removed = %+v", result.Removed)
	}
	if len(result.RemovedAliases) != 1 || !samePath(result.RemovedAliases[0], legacyAlias) {
		t.Fatalf("removed aliases = %+v", result.RemovedAliases)
	}
	for _, path := range []string{legacyBinary, legacyAlias} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy path still exists %s: %v", path, err)
		}
	}
}

func TestCleanupLegacyInstallationsPreservesPackageManagersGoAndUnknown(t *testing.T) {
	layout := testLayout(t)
	home := t.TempDir()
	homebrewDir := filepath.Join(t.TempDir(), "Cellar", "chatgpt-mcp", "1.0.0", "bin")
	goDir := filepath.Join(home, "go", "bin")
	unknownDir := t.TempDir()
	for _, dir := range []string{homebrewDir, goDir, unknownDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	homebrewBinary := filepath.Join(homebrewDir, layout.BinaryName)
	goBinary := filepath.Join(goDir, layout.BinaryName)
	unknownBinary := filepath.Join(unknownDir, layout.BinaryName)
	copyTestExecutable(t, homebrewBinary)
	copyTestExecutable(t, goBinary)
	if err := os.WriteFile(unknownBinary, []byte("not chatgpt-mcp"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("SCOOP", "")
	t.Setenv("PATH", homebrewDir+string(os.PathListSeparator)+goDir+string(os.PathListSeparator)+unknownDir)
	result, err := CleanupLegacyInstallations(LegacyCleanupOptions{Layout: layout})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 || len(result.Preserved) != 3 {
		t.Fatalf("cleanup = %+v", result)
	}
	methods := map[Method]bool{}
	for _, item := range result.Preserved {
		methods[item.Method] = true
		if _, err := os.Stat(item.Path); err != nil {
			t.Fatalf("preserved path %s: %v", item.Path, err)
		}
	}
	if !methods[MethodHomebrew] || !methods[MethodGo] || !methods[MethodUnknown] {
		t.Fatalf("preserved methods = %+v", methods)
	}
}

func TestCleanupLegacyInstallationsPreservesCurrentExecutable(t *testing.T) {
	layout := testLayout(t)
	currentDir := t.TempDir()
	current := filepath.Join(currentDir, layout.BinaryName)
	copyTestExecutable(t, current)
	setLegacyTestEnvironment(t, currentDir)
	result, err := CleanupLegacyInstallations(LegacyCleanupOptions{Layout: layout, Source: current, PreserveSource: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 || len(result.Preserved) != 1 {
		t.Fatalf("cleanup = %+v", result)
	}
	if result.Preserved[0].Reason != "current executable" {
		t.Fatalf("reason = %q", result.Preserved[0].Reason)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("current executable was removed: %v", err)
	}
}

func TestSamePathResolvesSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "alias")
	if err := os.Symlink(realDir, aliasDir); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(realDir, "chatgpt-mcp")
	if err := os.WriteFile(realPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}
	if !samePath(filepath.Join(aliasDir, "chatgpt-mcp"), realPath) {
		t.Fatalf("symlinked path was not canonicalized: alias=%q real=%q", aliasDir, realDir)
	}
}

func TestInstallMigratesShadowingStandaloneFromPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "install")
	binDir := filepath.Join(t.TempDir(), "bin")
	legacyDir := t.TempDir()
	layout, err := NewLayout(root, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	legacyBinary := filepath.Join(legacyDir, layout.BinaryName)
	copyTestExecutable(t, legacyBinary)
	setLegacyTestEnvironment(t, legacyDir+string(os.PathListSeparator)+binDir)
	result, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "release"), NoAlias: true, MigrateLegacy: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Legacy.Removed) != 1 || !samePath(result.Legacy.Removed[0].Path, legacyBinary) {
		t.Fatalf("legacy cleanup = %+v", result.Legacy)
	}
	if _, err := os.Stat(legacyBinary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy binary still exists: %v", err)
	}
	resolved, err := exec.LookPath(layout.BinaryName)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(resolved, layout.CanonicalBinary) {
		t.Fatalf("resolved command = %q, want %q", resolved, layout.CanonicalBinary)
	}
}

func TestInstallMigratesVerifiedLegacyCanonicalConflict(t *testing.T) {
	root := filepath.Join(t.TempDir(), "install")
	binDir := filepath.Join(t.TempDir(), "bin")
	layout, err := NewLayout(root, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	copyTestExecutable(t, layout.CanonicalBinary)
	if err := os.Symlink(layout.BinaryName, layout.AliasPath); err != nil {
		t.Fatal(err)
	}
	setLegacyTestEnvironment(t, binDir)
	result, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "release"), MigrateLegacy: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Legacy.Removed) != 1 || !samePath(result.Legacy.Removed[0].Path, layout.CanonicalBinary) {
		t.Fatalf("legacy cleanup = %+v", result.Legacy)
	}
	if len(result.Legacy.RemovedAliases) != 1 || !samePath(result.Legacy.RemovedAliases[0], layout.AliasPath) {
		t.Fatalf("legacy aliases = %+v", result.Legacy.RemovedAliases)
	}
	status, err := StatusCanonical(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != CanonicalInstalled {
		t.Fatalf("canonical state = %q", status.State)
	}
	alias, err := StatusAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if alias.State != AliasInstalled {
		t.Fatalf("alias state = %q", alias.State)
	}
}

func TestInstallDoesNotMigratePackageManagedCanonicalConflict(t *testing.T) {
	root := filepath.Join(t.TempDir(), "install")
	binDir := filepath.Join(t.TempDir(), "Cellar", "chatgpt-mcp", "1.0.0", "bin")
	layout, err := NewLayout(root, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	copyTestExecutable(t, layout.CanonicalBinary)
	setLegacyTestEnvironment(t, binDir)
	_, err = Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "release"), NoAlias: true, MigrateLegacy: true})
	if !errors.Is(err, ErrCanonicalConflict) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(layout.CanonicalBinary); err != nil {
		t.Fatalf("package-managed binary was removed: %v", err)
	}
}

func copyTestExecutable(t *testing.T, destination string) {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyBinary(source, destination); err != nil {
		t.Fatal(err)
	}
	if !verifyChatGPTMCPBinary(destination) {
		t.Fatalf("test executable was not recognized as chatgpt-mcp: %s", destination)
	}
}

func setLegacyTestEnvironment(t *testing.T, path string) {
	t.Helper()
	t.Setenv("PATH", path)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", filepath.Join(t.TempDir(), "gopath"))
	t.Setenv("SCOOP", "")
}
