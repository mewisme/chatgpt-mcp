//go:build !windows

package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAliasLifecycle(t *testing.T) {
	layout := testLayout(t)
	staged, err := Stage(layout, "v1.0.0", testBinary(t, "binary"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(staged); err != nil {
		t.Fatal(err)
	}
	status, err := StatusAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AliasMissing {
		t.Fatalf("initial state = %q", status.State)
	}
	status, err = InstallAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AliasInstalled {
		t.Fatalf("installed state = %q", status.State)
	}
	if _, err := InstallAlias(layout); err != nil {
		t.Fatalf("idempotent install: %v", err)
	}
	status, err = RemoveAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AliasMissing {
		t.Fatalf("removed state = %q", status.State)
	}
	if _, err := RemoveAlias(layout); err != nil {
		t.Fatalf("idempotent remove: %v", err)
	}
}

func TestAliasRefusesConflict(t *testing.T) {
	layout := testLayout(t)
	staged, err := Stage(layout, "v1.0.0", testBinary(t, "binary"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(staged); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.BinDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.AliasPath, []byte("unrelated"), 0755); err != nil {
		t.Fatal(err)
	}
	status, err := StatusAlias(layout)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != AliasConflict {
		t.Fatalf("state = %q", status.State)
	}
	if _, err := InstallAlias(layout); !errors.Is(err, ErrAliasConflict) {
		t.Fatalf("install error = %v", err)
	}
	if _, err := RemoveAlias(layout); !errors.Is(err, ErrAliasConflict) {
		t.Fatalf("remove error = %v", err)
	}
	data, err := os.ReadFile(layout.AliasPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "unrelated" {
		t.Fatalf("conflicting alias mutated: %q", data)
	}
}

func TestManagedLayoutUsesMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "install")
	binDir := filepath.Join(t.TempDir(), "bin")
	metadata := &Metadata{Schema: MetadataSchema, Method: MethodDirect, Version: "v1.0.0", InstallDir: root, BinDir: binDir}
	layout, err := (Detection{Method: MethodDirect, Root: root, Metadata: metadata}).ManagedLayout()
	if err != nil {
		t.Fatal(err)
	}
	if layout.Root != root || layout.BinDir != binDir {
		t.Fatalf("layout = %+v", layout)
	}
	metadata.InstallDir = filepath.Join(t.TempDir(), "other")
	if _, err := (Detection{Method: MethodDirect, Root: root, Metadata: metadata}).ManagedLayout(); err == nil {
		t.Fatal("expected metadata root mismatch")
	}
}
