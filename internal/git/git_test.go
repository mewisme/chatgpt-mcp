package git

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestRunAndOrThrow(t *testing.T) {
	dir := t.TempDir()
	if result, err := Run(context.Background(), dir, "init", "--quiet"); err != nil || result.ExitCode != 0 {
		t.Fatalf("init result=%#v err=%v", result, err)
	}
	result, err := Run(context.Background(), dir, "rev-parse", "--is-inside-work-tree")
	if err != nil || result.ExitCode != 0 || result.Stdout != "true" {
		t.Fatalf("rev-parse result=%#v err=%v", result, err)
	}
	result, err = Run(context.Background(), dir, "rev-parse", "--verify", "missing-ref")
	if err != nil || result.ExitCode == 0 {
		t.Fatalf("missing ref result=%#v err=%v", result, err)
	}
	if _, err := OrThrow(context.Background(), dir, "rev-parse", "--verify", "missing-ref"); err == nil {
		t.Fatal("OrThrow accepted failed git command")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, dir, "status"); err == nil {
		t.Fatal("cancelled context accepted")
	}
}

func TestEnvironmentDisablesInteractiveFormatting(t *testing.T) {
	t.Setenv("PAGER", "less")
	values := map[string]string{}
	for _, item := range environment() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	if values["PAGER"] != "cat" || values["GIT_PAGER"] != "cat" || values["NO_COLOR"] != "1" {
		t.Fatalf("environment = %#v", values)
	}
	if runtime.GOOS != "windows" && values["PATH"] != os.Getenv("PATH") {
		t.Fatalf("PATH changed: %q", values["PATH"])
	}
}

func TestBoundedBufferKeepsTail(t *testing.T) {
	var buffer boundedBuffer
	input := strings.Repeat("x", maxOutputBytes+100)
	if written, err := buffer.Write([]byte(input)); err != nil || written != len(input) {
		t.Fatalf("write=%d err=%v", written, err)
	}
	output := buffer.String()
	if !buffer.truncated || !strings.HasPrefix(output, "[output truncated") || len(buffer.data) != maxOutputBytes {
		t.Fatalf("truncated=%t bytes=%d prefix=%q", buffer.truncated, len(buffer.data), output[:min(len(output), 40)])
	}
}
