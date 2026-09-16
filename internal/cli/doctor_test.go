package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestDoctorCommandRegistered(t *testing.T) {
	cmd, _, err := newRootCommand().Find([]string{"doctor"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "doctor" || !cmd.Runnable() {
		t.Fatalf("doctor command = %q runnable=%t", cmd.Name(), cmd.Runnable())
	}
}

func TestDoctorUninitializedStillReportsLaterChecks(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v", err)
	}
	text := out.String()
	for _, expected := range []string{"FAIL", "config.source", "plugin.lock", "Summary:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}

func TestDoctorSecurityWarningDoesNotFail(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runDoctor(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "config.security") || !strings.Contains(out.String(), "WARN") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDoctorQuarantinesCorruptPluginLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	layout := pluginpkg.DefaultLayout()
	if err := os.WriteFile(layout.LockPath(), []byte(`{"schema":1,"plugins":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runDoctor(doctorCommand(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginpkg.LoadLock(layout.LockPath()); err != nil {
		t.Fatalf("recovered lock invalid: %v", err)
	}
	matches, err := filepath.Glob(layout.LockPath() + ".corrupt-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("quarantined locks = %#v err=%v", matches, err)
	}
}

func TestDoctorReportsUnavailableWorkspaceWithoutHidingHealthyOne(t *testing.T) {
	root := t.TempDir()
	testutil.UseConfigRoot(t, root)
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	healthy := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(missing, 0o700); err != nil {
		t.Fatal(err)
	}
	executeRequestCommand(t, root, []string{"workspace", "register", healthy})
	executeRequestCommand(t, root, []string{"workspace", "register", missing})
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	cmd := doctorCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runDoctor(cmd, nil)
	if !errors.Is(err, errDoctorFailed) {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "workspace.registry") || !strings.Contains(text, "FAIL") || !strings.Contains(text, "identity") {
		t.Fatalf("output = %q", text)
	}
	if !strings.Contains(text, "PASS") {
		t.Fatalf("healthy workspace hidden: %q", text)
	}
}
