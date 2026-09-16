package formatter

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunEchoesRenderedText(t *testing.T) {
	bin := buildHelper(t, `package main
import ("encoding/json"; "os")
func main() {
	var req struct{ Source string `+"`json:\"source\"`"+` }
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil { os.Exit(1) }
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"text": "R:"+req.Source}); err != nil { os.Exit(1) }
}
`)
	got, err := Run(context.Background(), bin, Request{Source: "hello", Width: 80})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "R:hello" {
		t.Fatalf("got %q", got)
	}
}

func TestRunRejectsOversizedSource(t *testing.T) {
	_, err := Run(context.Background(), "true", Request{Source: strings.Repeat("a", MaxSourceBytes+1)})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunTimesOut(t *testing.T) {
	bin := buildHelper(t, `package main
import "time"
func main() { time.Sleep(time.Minute) }
`)
	start := time.Now()
	_, err := Run(context.Background(), bin, Request{Source: "x"})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("timeout took too long")
	}
}

func TestRunRejectsOversizedOutput(t *testing.T) {
	bin := buildHelper(t, `package main
import ("fmt"; "strings")
func main() { fmt.Print(`+"`"+`{"text":"`+"`"+` + strings.Repeat("a", 2<<20) + `+"`"+`"}`+"`"+`) }
`)
	_, err := Run(context.Background(), bin, Request{Source: "x"})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeRequestRejectsOversizedPayload(t *testing.T) {
	_, err := DecodeRequest(strings.NewReader(strings.Repeat("a", MaxRequestBytes+1)))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestFallbackKeepsRawMarkdown(t *testing.T) {
	if got := Fallback(" # Title \n"); got != "# Title\n" {
		t.Fatalf("got %q", got)
	}
}

func buildHelper(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin
}
