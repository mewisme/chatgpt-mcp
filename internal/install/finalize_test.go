package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackResultRestoresCurrentAndMetadata(t *testing.T) {
	layout := testLayout(t)
	first, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "old"), NoAlias: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.PreviousMetadata != nil {
		t.Fatalf("first install previous metadata = %+v", first.PreviousMetadata)
	}
	second, err := Install(Options{Layout: layout, Version: "v1.1.0", Source: testBinary(t, "new"), NoAlias: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.PreviousMetadata == nil || second.PreviousMetadata.Version != "v1.0.0" {
		t.Fatalf("previous metadata = %+v", second.PreviousMetadata)
	}
	if err := RollbackResult(second); err != nil {
		t.Fatal(err)
	}
	version, _, err := CurrentVersion(layout)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.0.0" {
		t.Fatalf("current version = %q", version)
	}
	metadata, err := ReadMetadata(layout.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Version != "v1.0.0" {
		t.Fatalf("metadata version = %q", metadata.Version)
	}
}

func TestFinalizeResultKeepsCurrentAndPrevious(t *testing.T) {
	layout := testLayout(t)
	if _, err := Install(Options{Layout: layout, Version: "v1.0.0", Source: testBinary(t, "oldest"), NoAlias: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{Layout: layout, Version: "v1.1.0", Source: testBinary(t, "old"), NoAlias: true}); err != nil {
		t.Fatal(err)
	}
	result, err := Install(Options{Layout: layout, Version: "v1.2.0", Source: testBinary(t, "new"), NoAlias: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := FinalizeResult(result); err != nil {
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
}
