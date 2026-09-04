//go:build !windows

package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallLifecycle(t *testing.T) {
	layout := testLayout(t)
	source := testBinary(t, "release-v1")
	result, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyInstalled {
		t.Fatal("first install reported already installed")
	}
	if result.Canonical.State != CanonicalInstalled || result.Alias.State != AliasInstalled || !result.AliasInstalled {
		t.Fatalf("install result = %+v", result)
	}
	assertCurrentVersion(t, layout, "v1.0.0")
	metadata, err := ReadMetadata(layout.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Method != MethodDirect || metadata.Version != "v1.0.0" || metadata.InstallDir != layout.Root || metadata.BinDir != layout.BinDir {
		t.Fatalf("metadata = %+v", metadata)
	}
	result, err = Install(Options{Layout: layout, Version: "v1.0.0", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AlreadyInstalled {
		t.Fatal("second install was not idempotent")
	}
}

func TestInstallRepairsMissingAlias(t *testing.T) {
	layout := testLayout(t)
	source := testBinary(t, "release-v1")
	if _, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: source}); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveAlias(layout); err != nil {
		t.Fatal(err)
	}
	result, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyInstalled {
		t.Fatal("repair install reported already installed")
	}
	if result.Alias.State != AliasInstalled {
		t.Fatalf("alias state = %q", result.Alias.State)
	}
}

func TestInstallNoAlias(t *testing.T) {
	layout := testLayout(t)
	result, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "release-v1"), NoAlias: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.AliasInstalled {
		t.Fatal("alias installed with --no-alias")
	}
	status, err := StatusAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AliasMissing {
		t.Fatalf("alias state = %q", status.State)
	}
	if result.Canonical.State != CanonicalInstalled {
		t.Fatalf("canonical state = %q", result.Canonical.State)
	}
}

func TestInstallDevelopmentRequiresForce(t *testing.T) {
	layout := testLayout(t)
	source := testBinary(t, "dev")
	if _, err := Install(Options{Layout: layout, Version: "dev", Source: source}); !errors.Is(err, ErrDevelopmentBuild) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(layout.Versions); !os.IsNotExist(err) {
		t.Fatalf("development refusal mutated install: %v", err)
	}
	if _, err := Install(Options{Layout: layout, Version: "dev", Source: source, Force: true}); err != nil {
		t.Fatalf("forced development install: %v", err)
	}
	assertCurrentVersion(t, layout, "dev")
}

func TestInstallPreflightsCanonicalConflict(t *testing.T) {
	layout := testLayout(t)
	if err := os.MkdirAll(layout.BinDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.CanonicalBinary, []byte("unrelated"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "release-v1")})
	if !errors.Is(err, ErrCanonicalConflict) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(layout.Versions); !os.IsNotExist(err) {
		t.Fatalf("conflict preflight mutated versions: %v", err)
	}
}

func TestDefaultLayoutHonorsInstallEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	binDir := filepath.Join(t.TempDir(), "bin")
	t.Setenv(EnvInstallDir, root)
	t.Setenv(EnvBinDir, binDir)
	layout, err := DefaultLayout()
	if err != nil {
		t.Fatal(err)
	}
	if layout.Root != root || layout.BinDir != binDir {
		t.Fatalf("layout = %+v", layout)
	}
}
