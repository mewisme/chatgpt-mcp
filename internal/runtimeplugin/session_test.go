package runtimeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDecodeEnvelopeRejectsVersionMismatchAndMalformed(t *testing.T) {
	if _, err := decodeEnvelope([]byte(`{"version":99,"id":1,"result":{}}`)); !errors.Is(err, ErrProtocolVersion) {
		t.Fatalf("err = %v", err)
	}
	if _, err := decodeEnvelope([]byte(`{`)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v", err)
	}
}

func TestDescribeHandshake(t *testing.T) {
	session := startFixture(t, "")
	raw, err := session.Call(context.Background(), MethodDescribe, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result DescribeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Provider != "test" || len(result.Targets) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestUnsupportedProtocolVersionFailsSession(t *testing.T) {
	session := startFixture(t, "bad-version")
	_, err := session.Call(context.Background(), MethodDescribe, nil)
	if !errors.Is(err, ErrProtocolVersion) && !errors.Is(err, ErrClosed) && !errors.Is(err, ErrCrashed) {
		t.Fatalf("err = %v", err)
	}
}

func TestMalformedAndOversizedStdoutFail(t *testing.T) {
	for _, mode := range []string{"malformed", "oversized"} {
		session := startFixture(t, mode)
		_, err := session.Call(context.Background(), MethodDescribe, nil)
		if err == nil {
			t.Fatalf("%s: expected error", mode)
		}
	}
}

func TestStderrIsBounded(t *testing.T) {
	session := startFixture(t, "stderr-flood")
	_, _ = session.Call(context.Background(), MethodDescribe, nil)
	if !session.stderr.exceededLimit() {
		t.Fatal("stderr was not bounded")
	}
	if session.stderr.buf.Len() > MaxStderrBytes+1 {
		t.Fatalf("stderr len = %d", session.stderr.buf.Len())
	}
}

func TestRequestTimeoutDoesNotHang(t *testing.T) {
	session := startFixture(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := session.Call(ctx, MethodStart, StartParams{Target: "hang", Origin: "http://127.0.0.1:9"})
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestChildCrashIsIsolated(t *testing.T) {
	session := startFixture(t, "")
	_, err := session.Call(context.Background(), MethodStart, StartParams{Target: "crash", Origin: "http://127.0.0.1:9"})
	if !errors.Is(err, ErrCrashed) && !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v", err)
	}
}

func TestShutdownTerminatesProcess(t *testing.T) {
	session := startFixture(t, "")
	if err := session.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	select {
	case <-session.exited:
	case <-time.After(2 * time.Second):
		t.Fatal("process still running")
	}
}

func TestForcedTerminationAfterGracefulTimeout(t *testing.T) {
	session := startFixture(t, "ignore-shutdown")
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	started := time.Now()
	_ = session.Shutdown(ctx)
	if time.Since(started) > 4*time.Second {
		t.Fatal("forced termination took too long")
	}
	select {
	case <-session.exited:
	case <-time.After(2 * time.Second):
		t.Fatal("process still running")
	}
}

func TestUnknownMethodFailsClosed(t *testing.T) {
	session := startFixture(t, "")
	_, err := session.Call(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected unknown method error")
	}
	if _, err := session.Call(context.Background(), MethodDescribe, nil); err != nil {
		t.Fatalf("session closed after unknown method: %v", err)
	}
}

func TestNoShellIsInvolved(t *testing.T) {
	session := startFixture(t, "")
	path := session.CommandPath()
	base := strings.ToLower(filepath.Base(path))
	if base == "sh" || base == "bash" || base == "cmd.exe" || base == "cmd" || base == "powershell.exe" {
		t.Fatalf("shell used: %s", path)
	}
	if runtime.GOOS != "windows" && len(session.cmd.Args) != 1 {
		t.Fatalf("args = %#v", session.cmd.Args)
	}
}

func startFixture(t *testing.T, mode string) *Session {
	t.Helper()
	bin := buildFixture(t)
	ctx := context.Background()
	spec := Spec{ID: "test-tunnel", Version: "dev", Entrypoint: bin, WorkDir: filepath.Dir(bin), DataDir: t.TempDir()}
	if mode != "" {
		spec.ExtraEnv = []string{"TESTPLUGIN_MODE=" + mode}
	}
	session, err := Start(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Shutdown(context.Background()) })
	return session
}

func buildFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "testplugin")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/testplugin")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatal(err)
	}
	return bin
}
