package nativeinstruction

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadSkillFileRejectsReplacement(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "regular replacement", true: "external symlink"}[external], func(t *testing.T) {
			dir := t.TempDir()
			name := filepath.Join(dir, "note.md")
			if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := os.Rename(name, name+".old"); err != nil {
				t.Fatal(err)
			}
			if external {
				outside := filepath.Join(t.TempDir(), "secret")
				if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, name); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			} else if err := os.WriteFile(name, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			if data, err := readSkillFile(root, "note.md", info); err == nil || data != nil {
				t.Fatalf("replacement read: %q, %v", data, err)
			}
		})
	}
}

func TestLoadSkillTreeRejectsSymlinksAndOversize(t *testing.T) {
	t.Run("symlink root", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "skill")
		if err := os.Symlink(t.TempDir(), link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := loadSkillTree(link); err == nil {
			t.Fatal("accepted symlink root")
		}
	})
	t.Run("symlink child", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(dir, "assets")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := loadSkillTree(dir); err == nil {
			t.Fatal("accepted symlink child")
		}
	})
	t.Run("file size", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "large"), bytes.Repeat([]byte("x"), maxFileBytes+1), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSkillTree(dir); err == nil {
			t.Fatal("accepted oversized file")
		}
	})
	t.Run("tree size", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			if err := os.WriteFile(filepath.Join(dir, name), bytes.Repeat([]byte("x"), maxFileBytes), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := loadSkillTree(dir); err == nil {
			t.Fatal("accepted oversized tree")
		}
	})
}

func TestLoadRootedSkillTreeSurvivesRootReplacement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skill")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(dir, dir+".old"); err != nil {
		t.Skipf("cannot rename open root: %v", err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := loadRootedSkillTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(files["note"].Data) != "original" {
		t.Fatalf("read replacement tree: %q", files["note"].Data)
	}
}
