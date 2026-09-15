package cli

import (
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
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
