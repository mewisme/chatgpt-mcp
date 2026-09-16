package pluginbuild

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompressBinarySkipsDarwin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("not-upx"), 0755); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := compressBinary(path, "darwin"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if after.Size() != before.Size() {
		t.Fatalf("darwin binary rewritten: %d -> %d", before.Size(), after.Size())
	}
}

func TestCompressBinaryRequiresUPXUnlessOptedOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("payload"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv(EnvUPX, "")
	if err := compressBinary(path, "linux"); err == nil {
		t.Fatal("expected missing upx to fail")
	}
	t.Setenv(EnvUPX, EnvUPXOff)
	if err := compressBinary(path, "linux"); err != nil {
		t.Fatal(err)
	}
}

func TestCompressBinaryUPXRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("upx"); err != nil {
		t.Skip("upx not installed")
	}
	if runtime.GOOS == "windows" {
		t.Skip("helper compresses linux/windows ELF/PE; this smoke uses the host go binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tiny")
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := compressBinary(bin, "linux"); err != nil {
		if runtime.GOOS != "linux" {
			t.Skip(err)
		}
		t.Fatal(err)
	}
}

func TestCompressBinaryWindowsPE(t *testing.T) {
	if _, err := exec.LookPath("upx"); err != nil {
		t.Skip("upx not installed")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tiny.exe")
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, src)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := compressBinary(bin, "windows"); err != nil {
		t.Fatal(err)
	}
}
