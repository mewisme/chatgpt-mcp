package jsruntime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requirePermissionNode(t *testing.T) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	help, err := exec.Command(node, "--help").CombinedOutput()
	if err != nil {
		t.Skipf("node --help failed: %v", err)
	}
	if !strings.Contains(string(help), "--permission") && !strings.Contains(string(help), "--experimental-permission") {
		t.Skip("node permission model unavailable")
	}
}

func TestStatePersistsAcrossEval(t *testing.T) {
	requirePermissionNode(t)
	root := t.TempDir()
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })

	first, err := manager.Eval(context.Background(), "ws_test", root, `globalThis.count = (globalThis.count || 0) + 1; return globalThis.count`, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Eval(context.Background(), "ws_test", root, `globalThis.count += 1; return globalThis.count`, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if first.Value == nil || *first.Value != "1" || second.Value == nil || *second.Value != "2" {
		t.Fatalf("values = %#v %#v", first, second)
	}
}

func TestWorkspaceFilesystemAllowed(t *testing.T) {
	requirePermissionNode(t)
	root := t.TempDir()
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	result, err := manager.Eval(context.Background(), "ws_test", root, `const fs = require("fs"); fs.writeFileSync("inside.txt", "ok"); return fs.readFileSync("inside.txt", "utf8")`, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Value == nil || *result.Value != "'ok'" {
		t.Fatalf("result = %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "inside.txt"))
	if err != nil || string(data) != "ok" {
		t.Fatalf("file = %q err=%v", data, err)
	}
}

func TestOutsideFilesystemDenied(t *testing.T) {
	requirePermissionNode(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	encoded, _ := json.Marshal(outside)
	_, err := manager.Eval(context.Background(), "ws_test", root, `const fs = require("fs"); fs.writeFileSync(`+string(encoded)+`, "no")`, 5*time.Second)
	if err == nil {
		t.Fatal("outside write was not denied")
	}
	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatalf("outside file exists: %v", statErr)
	}
}

func TestSymlinkFilesystemEscapeDenied(t *testing.T) {
	requirePermissionNode(t)
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	_, err := manager.Eval(context.Background(), "ws_test", root, `const fs = require("fs"); fs.writeFileSync("outside-link/escape.txt", "no")`, 5*time.Second)
	if err == nil {
		t.Fatal("symlink escape write was not denied")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("outside file exists through symlink: %v", statErr)
	}
}

func TestResetDropsState(t *testing.T) {
	requirePermissionNode(t)
	root := t.TempDir()
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.Eval(context.Background(), "ws_test", root, `globalThis.value = 42`, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reset("ws_test"); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Eval(context.Background(), "ws_test", root, `return globalThis.value`, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != nil {
		t.Fatalf("value persisted after reset: %#v", result)
	}
}
