//go:build !windows

package install

import (
	"errors"
	"os"
	"testing"
)

func TestActivateAndRollback(t *testing.T) {
	layout := testLayout(t)
	first, err := Stage(layout, "v1.0.0", testBinary(t, "first"))
	if err != nil {
		t.Fatal(err)
	}
	firstActivation, err := Activate(first)
	if err != nil {
		t.Fatal(err)
	}
	if firstActivation.PreviousVersion != "" {
		t.Fatalf("previous version = %q", firstActivation.PreviousVersion)
	}
	assertCurrentVersion(t, layout, "v1.0.0")
	second, err := Stage(layout, "v1.1.0", testBinary(t, "second"))
	if err != nil {
		t.Fatal(err)
	}
	secondActivation, err := Activate(second)
	if err != nil {
		t.Fatal(err)
	}
	if secondActivation.PreviousVersion != "v1.0.0" {
		t.Fatalf("previous version = %q", secondActivation.PreviousVersion)
	}
	assertCurrentVersion(t, layout, "v1.1.0")
	if err := Rollback(secondActivation); err != nil {
		t.Fatal(err)
	}
	assertCurrentVersion(t, layout, "v1.0.0")
	if err := Rollback(firstActivation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(layout.Current); !os.IsNotExist(err) {
		t.Fatalf("current remains after rollback to no previous version: %v", err)
	}
}

func TestActivateIsIdempotent(t *testing.T) {
	layout := testLayout(t)
	staged, err := Stage(layout, "v1.0.0", testBinary(t, "same"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(staged); err != nil {
		t.Fatal(err)
	}
	activation, err := Activate(staged)
	if err != nil {
		t.Fatal(err)
	}
	if activation.PreviousVersion != "v1.0.0" {
		t.Fatalf("previous version = %q", activation.PreviousVersion)
	}
	assertCurrentVersion(t, layout, "v1.0.0")
}

func TestActivateRefusesUnmanagedCurrent(t *testing.T) {
	layout := testLayout(t)
	staged, err := Stage(layout, "v1.0.0", testBinary(t, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.Root, 0755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, layout.Current); err != nil {
		t.Fatal(err)
	}
	_, err = Activate(staged)
	if !errors.Is(err, ErrCurrentNotManaged) {
		t.Fatalf("error = %v", err)
	}
}

func TestActivateRefusesRealCurrentDirectory(t *testing.T) {
	layout := testLayout(t)
	staged, err := Stage(layout, "v1.0.0", testBinary(t, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.Current, 0755); err != nil {
		t.Fatal(err)
	}
	_, err = Activate(staged)
	if !errors.Is(err, ErrCurrentNotManaged) {
		t.Fatalf("error = %v", err)
	}
}

func assertCurrentVersion(t *testing.T, layout Layout, want string) {
	t.Helper()
	version, target, err := CurrentVersion(layout)
	if err != nil {
		t.Fatal(err)
	}
	if version != want {
		t.Fatalf("current version = %q, want %q", version, want)
	}
	expected, err := layout.VersionDir(want)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	expectedInfo, err := os.Stat(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(targetInfo, expectedInfo) {
		t.Fatalf("current target = %q, want filesystem target %q", target, expected)
	}
}
