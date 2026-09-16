package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreDepsExcludeCFTunnel(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps .: %v\n%s", err, out)
	}
	text := string(out)
	for _, forbidden := range []string{
		"go.mewis.me/chatgpt-mcp/plugins/cf-tunnel",
		"go.mewis.me/chatgpt-mcp/pkg/cloudflared",
		"github.com/quic-go/quic-go",
		"zombiezen.com/go/capnproto2",
	} {
		for _, line := range strings.Split(text, "\n") {
			if line == forbidden || strings.HasPrefix(line, forbidden+"/") {
				t.Errorf("core dependency graph includes %s", line)
			}
		}
	}
}

func TestCoreAndPluginBinarySizes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary size measurement")
	}
	root := moduleRoot(t)
	dir := t.TempDir()
	core := filepath.Join(dir, "cgm")
	plugin := filepath.Join(dir, "cf-tunnel")
	coreBuild := exec.Command("go", "build", "-o", core, ".")
	coreBuild.Dir = root
	if out, err := coreBuild.CombinedOutput(); err != nil {
		t.Fatalf("core build: %v\n%s", err, out)
	}
	pluginBuild := exec.Command("go", "build", "-o", plugin, "./plugins/cf-tunnel/cmd/cf-tunnel")
	pluginBuild.Dir = root
	if out, err := pluginBuild.CombinedOutput(); err != nil {
		t.Fatalf("plugin build: %v\n%s", err, out)
	}
	coreInfo, err := os.Stat(core)
	if err != nil {
		t.Fatal(err)
	}
	pluginInfo, err := os.Stat(plugin)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("core=%d plugin=%d", coreInfo.Size(), pluginInfo.Size())
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
