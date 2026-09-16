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
		"go.mewis.me/chatgpt-mcp/plugins/secure-mcp-tunnel",
		"go.mewis.me/chatgpt-mcp/plugins/tui",
		"go.mewis.me/chatgpt-mcp/plugins/markdown-formatter",
		"go.mewis.me/chatgpt-mcp/pkg/cloudflared",
		"github.com/quic-go/quic-go",
		"zombiezen.com/go/capnproto2",
		"github.com/openai/tunnel-client",
		"charm.land/bubbletea/v2",
		"charm.land/bubbles/v2",
		"charm.land/huh/v2",
		"charm.land/glamour/v2",
		"charm.land/lipgloss/v2",
		"github.com/alecthomas/chroma/v2",
		"github.com/yuin/goldmark",
		"github.com/yuin/goldmark-emoji",
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
	secure := filepath.Join(dir, "secure-mcp-tunnel")
	tui := filepath.Join(dir, "tui")
	markdown := filepath.Join(dir, "markdown-formatter")
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
	secureBuild := exec.Command("go", "build", "-o", secure, "./plugins/secure-mcp-tunnel/cmd/secure-mcp-tunnel")
	secureBuild.Dir = root
	if out, err := secureBuild.CombinedOutput(); err != nil {
		t.Fatalf("secure-mcp-tunnel build: %v\n%s", err, out)
	}
	tuiBuild := exec.Command("go", "build", "-o", tui, "./plugins/tui/cmd/tui")
	tuiBuild.Dir = root
	if out, err := tuiBuild.CombinedOutput(); err != nil {
		t.Fatalf("tui build: %v\n%s", err, out)
	}
	markdownBuild := exec.Command("go", "build", "-o", markdown, "./plugins/markdown-formatter/cmd/markdown-formatter")
	markdownBuild.Dir = root
	if out, err := markdownBuild.CombinedOutput(); err != nil {
		t.Fatalf("markdown-formatter build: %v\n%s", err, out)
	}
	coreInfo, err := os.Stat(core)
	if err != nil {
		t.Fatal(err)
	}
	pluginInfo, err := os.Stat(plugin)
	if err != nil {
		t.Fatal(err)
	}
	secureInfo, err := os.Stat(secure)
	if err != nil {
		t.Fatal(err)
	}
	tuiInfo, err := os.Stat(tui)
	if err != nil {
		t.Fatal(err)
	}
	markdownInfo, err := os.Stat(markdown)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("core=%d cf-tunnel=%d secure-mcp-tunnel=%d tui=%d markdown-formatter=%d", coreInfo.Size(), pluginInfo.Size(), secureInfo.Size(), tuiInfo.Size(), markdownInfo.Size())
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
