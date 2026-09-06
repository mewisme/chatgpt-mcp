package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecHelpers(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "config")
	spec, err := NewSpec(root, binary, ScopeUser, Account{Username: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != ID(root, ScopeUser) || spec.Scope != ScopeUser || spec.Binary != filepath.Clean(binary) || spec.Account.Username != "tester" {
		t.Fatalf("spec = %#v", spec)
	}
	if _, err := NewSpec("", binary, ScopeUser, Account{}); err == nil {
		t.Fatal("empty config root accepted")
	}
	if _, err := StableBinaryPath("definitely-not-a-real-chatgpt-mcp-binary"); err == nil {
		t.Fatal("missing binary accepted")
	}
	spec.EnvironmentHash = "env-hash"
	joined := strings.Join(Args(spec), " ")
	for _, value := range []string{spec.ConfigRoot, spec.ID, string(spec.Scope), spec.EnvironmentHash} {
		if !strings.Contains(joined, value) {
			t.Fatalf("args %q missing %q", joined, value)
		}
	}
}

func TestRunCommand(t *testing.T) {
	output, err := runCommand("go", "env", "GOOS")
	if err != nil || strings.TrimSpace(output) == "" {
		t.Fatalf("go env output=%q err=%v", output, err)
	}
	if output, err := runCommand("go", "env", "-definitely-invalid-flag"); err == nil || output == "" {
		t.Fatalf("invalid command output=%q err=%v", output, err)
	}
	if _, ok := commandSucceeded("go", "env", "GOARCH"); !ok {
		t.Fatal("expected successful command")
	}
	if _, ok := commandSucceeded("definitely-not-a-real-chatgpt-mcp-command"); ok {
		t.Fatal("missing command succeeded")
	}
}
