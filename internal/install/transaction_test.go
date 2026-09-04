package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStageCreatesImmutableVersion(t *testing.T) {
	layout := testLayout(t)
	source := testBinary(t, "first")
	staged, err := Stage(layout, "v1.2.3", source)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Reused {
		t.Fatal("first stage unexpectedly reused an existing version")
	}
	data, err := os.ReadFile(staged.Binary)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first" {
		t.Fatalf("staged binary = %q", data)
	}
	stagedAgain, err := Stage(layout, "v1.2.3", source)
	if err != nil {
		t.Fatal(err)
	}
	if !stagedAgain.Reused {
		t.Fatal("identical version was not reused")
	}
}

func TestStageRejectsVersionContentConflict(t *testing.T) {
	layout := testLayout(t)
	if _, err := Stage(layout, "v1.2.3", testBinary(t, "first")); err != nil {
		t.Fatal(err)
	}
	_, err := Stage(layout, "v1.2.3", testBinary(t, "second"))
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("error = %v", err)
	}
}

func TestStageRejectsInvalidSource(t *testing.T) {
	layout := testLayout(t)
	if _, err := Stage(layout, "v1.0.0", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing source to fail")
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Stage(layout, "v1.0.0", empty); err == nil {
		t.Fatal("expected empty source to fail")
	}
}

func TestCleanupKeepsSelectedVersions(t *testing.T) {
	layout := testLayout(t)
	for _, version := range []string{"v1.0.0", "v1.1.0", "v1.2.0"} {
		if _, err := Stage(layout, version, testBinary(t, version)); err != nil {
			t.Fatal(err)
		}
	}
	staging := filepath.Join(layout.Versions, ".staging-stale")
	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(layout.Versions, "keep-me.txt")
	if err := os.WriteFile(marker, []byte("marker"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(layout, "v1.1.0", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.Versions, "v1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("old version still exists: %v", err)
	}
	for _, version := range []string{"v1.1.0", "v1.2.0"} {
		if _, err := os.Stat(filepath.Join(layout.Versions, version)); err != nil {
			t.Fatalf("kept version %s: %v", version, err)
		}
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging directory still exists: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("non-version file should be preserved: %v", err)
	}
}

func testLayout(t *testing.T) Layout {
	t.Helper()
	root := filepath.Join(t.TempDir(), "install")
	layout, err := NewLayout(root, filepath.Join(root, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func testBinary(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chatgpt-mcp")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
