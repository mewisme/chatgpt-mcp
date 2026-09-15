package plugin

import (
	"path/filepath"
	"testing"
)

func TestLayoutPathsAreSeparated(t *testing.T) {
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	if err := layout.Validate(); err != nil {
		t.Fatal(err)
	}
	if layout.LockPath() == layout.InstalledVersionPath("bash", "1.0.0") || layout.DownloadsPath() == layout.PluginsPath() {
		t.Fatal("plugin roots overlap")
	}
}

func TestLayoutRejectsSharedRoots(t *testing.T) {
	root := t.TempDir()
	if err := (Layout{ConfigRoot: root, DataRoot: root, CacheRoot: filepath.Join(root, "cache")}).Validate(); err == nil {
		t.Fatal("shared config/data root accepted")
	}
}
