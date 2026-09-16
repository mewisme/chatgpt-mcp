package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReviewArgvRejectsUnsafeIDs(t *testing.T) {
	if _, err := ReviewArgv("/usr/bin/cgm", "req_ok"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "req_", "ws_1", "req_abc;rm", "req_abc && true", "req_abc\n", "req_abc id", "req_abc$HOST"} {
		if _, err := ReviewArgv("/usr/bin/cgm", id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
}

func TestReviewArgvIsStructured(t *testing.T) {
	args, err := ReviewArgv("/usr/bin/cgm", "req_abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "/usr/bin/cgm tui requests req_abcdef" {
		t.Fatalf("argv=%q", args)
	}
}

func TestWSLReviewArgvPreservesDistro(t *testing.T) {
	args, err := WSLReviewArgv("/mnt/c/Windows/system32/wt.exe", "Ubuntu-24.04", "/home/mew/cgm", "req_abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "/mnt/c/Windows/system32/wt.exe nt wsl.exe -d Ubuntu-24.04 -- /home/mew/cgm tui requests req_abcdef" {
		t.Fatalf("argv=%q", args)
	}
	if _, err := WSLReviewArgv("/wt.exe", "Ubuntu; calc.exe", "/cgm", "req_abcdef"); err == nil {
		t.Fatal("accepted unsafe distro")
	}
}

func TestOpenApprovalUsesReviewArgv(t *testing.T) {
	var ran runSpec
	h := testHost("linux", nil, nil)
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	if err := openApproval(context.Background(), h, func() (string, error) { return "/usr/bin/cgm", nil }, "req_abcdef"); err != nil {
		t.Fatal(err)
	}
	if ran.Name != "/usr/bin/cgm" || strings.Join(ran.Args, " ") != "tui requests req_abcdef" {
		t.Fatalf("open spec=%#v", ran)
	}
}

func TestOpenApprovalOnWSLUsesWindowsTerminal(t *testing.T) {
	var ran runSpec
	h := testHost("linux", map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, map[string]string{"wt.exe": "/mnt/c/Users/mew/AppData/Local/Microsoft/WindowsApps/wt.exe"})
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	if err := openApproval(context.Background(), h, func() (string, error) { return "/home/mew/bin/cgm", nil }, "req_abcdef"); err != nil {
		t.Fatal(err)
	}
	if ran.Name != "/mnt/c/Users/mew/AppData/Local/Microsoft/WindowsApps/wt.exe" {
		t.Fatalf("wt name=%q", ran.Name)
	}
	if strings.Join(ran.Args, " ") != "nt wsl.exe -d Ubuntu -- /home/mew/bin/cgm tui requests req_abcdef" {
		t.Fatalf("wt args=%q", ran.Args)
	}
}

func TestOpenApprovalRejectsUnsafeIDWithoutRunning(t *testing.T) {
	h := testHost("linux", nil, nil)
	h.runFn = func(context.Context, runSpec) error {
		return errors.New("should not run")
	}
	if err := openApproval(context.Background(), h, func() (string, error) { return "/usr/bin/cgm", nil }, "req_abc; reboot"); err == nil {
		t.Fatal("expected invalid id")
	}
}
