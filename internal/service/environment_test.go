package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCaptureEnvironmentKeepsExecutionPathWithoutSecrets(t *testing.T) {
	accountHome := filepath.FromSlash("/home/mew")
	goBin := filepath.FromSlash("/home/mew/go/bin")
	nodeBin := filepath.FromSlash("/custom/node/bin")
	systemBin := filepath.FromSlash("/usr/bin")
	t.Setenv("PATH", strings.Join([]string{nodeBin, systemBin}, string(os.PathListSeparator)))
	t.Setenv("OPENAI_API_KEY", "secret")
	t.Setenv("LANG", "en_US.UTF-8")
	snapshot := CaptureEnvironment(Account{Username: "mew", HomeDir: accountHome}, []string{goBin, nodeBin})
	path := snapshot.Values["PATH"]
	for _, expected := range []string{goBin, nodeBin, systemBin} {
		if !strings.Contains(path, expected) {
			t.Fatalf("PATH %q missing %q", path, expected)
		}
	}
	if strings.Count(path, nodeBin) != 1 {
		t.Fatalf("PATH did not deduplicate entries: %q", path)
	}
	if _, exists := snapshot.Values["OPENAI_API_KEY"]; exists {
		t.Fatal("managed environment captured arbitrary secret")
	}
	if snapshot.Values["LANG"] != "en_US.UTF-8" {
		t.Fatalf("LANG = %q", snapshot.Values["LANG"])
	}
	if runtime.GOOS == "windows" {
		if snapshot.Values["USERPROFILE"] != accountHome || snapshot.Values["USERNAME"] != "mew" {
			t.Fatalf("windows identity = %#v", snapshot.Values)
		}
	} else if snapshot.Values["HOME"] != accountHome || snapshot.Values["USER"] != "mew" || snapshot.Values["LOGNAME"] != "mew" {
		t.Fatalf("unix identity = %#v", snapshot.Values)
	}
}

func TestManagedEnvironmentRoundTripAndHashGuard(t *testing.T) {
	root := t.TempDir()
	snapshot := EnvironmentSnapshot{Version: environmentVersion, Values: map[string]string{"PATH": "/tools:/usr/bin", "LANG": "C.UTF-8"}}
	hash, err := SaveEnvironment(root, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadEnvironment(root, hash)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Values["PATH"] != snapshot.Values["PATH"] {
		t.Fatalf("snapshot = %#v", loaded)
	}
	if _, err := LoadEnvironment(root, "wrong"); err == nil {
		t.Fatal("environment hash mismatch was accepted")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(EnvironmentPath(root))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("environment mode = %o, want 600", info.Mode().Perm())
		}
	}
}
