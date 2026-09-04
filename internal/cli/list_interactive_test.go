package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestWorkspaceListInteractiveFlagsPreserveNonTTYOutputs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	workspaceRoot := t.TempDir()
	registered := executeRequestCommand(t, root, []string{"workspace", "register", workspaceRoot})
	if !strings.Contains(registered, "Workspace registered") {
		t.Fatalf("register=%q", registered)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"workspace", "list", "--json", "--interactive"})
	var items []workspace.Workspace
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &items); err != nil || len(items) != 1 {
		t.Fatalf("json=%q items=%#v err=%v", jsonOutput, items, err)
	}
	registeredInfo, err := os.Stat(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	listedInfo, err := os.Stat(items[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(registeredInfo, listedInfo) {
		t.Fatalf("listed path %q does not identify registered root %q", items[0].Path, workspaceRoot)
	}
	plain := executeRequestCommand(t, root, []string{"workspace", "list", "--no-interactive"})
	if !strings.Contains(plain, items[0].Path) || !strings.Contains(plain, "Registered workspaces loaded") {
		t.Fatalf("plain=%q", plain)
	}
	if _, err := executeRequestCommandError(root, []string{"workspace", "list", "--interactive"}); err == nil || !strings.Contains(err.Error(), "requires terminal") {
		t.Fatalf("forced interactive err=%v", err)
	}
}

func TestMCPServerListInteractiveFlagsPreserveNonTTYOutputsAndRedaction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	if _, err := executeRequestCommandError(root, []string{"mcp", "server", "add", "demo", "--transport", "http", "--url", "https://mcp.example.test", "--header", "Authorization=secret-value"}); err != nil {
		t.Fatal(err)
	}
	plain := executeRequestCommand(t, root, []string{"mcp", "server", "list", "--no-interactive"})
	if !strings.Contains(plain, "demo") || !strings.Contains(plain, "https://mcp.example.test") || strings.Contains(plain, "secret-value") {
		t.Fatalf("plain=%q", plain)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"mcp", "server", "list", "--json", "--interactive"})
	if strings.Contains(jsonOutput, "secret-value") || !strings.Contains(jsonOutput, "redacted") {
		t.Fatalf("json=%q", jsonOutput)
	}
	if _, err := executeRequestCommandError(root, []string{"mcp", "server", "list", "--interactive"}); err == nil || !strings.Contains(err.Error(), "requires terminal") {
		t.Fatalf("forced interactive err=%v", err)
	}
}

func TestTunnelListInteractiveFlagsPreserveNonTTYJSON(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tunnels" || r.URL.Query().Get("workspace_id") != "ws_admin" {
			t.Fatalf("request=%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer admin-test" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"tunnels":[{"id":"tunnel_one","name":"One","description":"First","workspace_ids":["ws_admin"]}]}`))
	}))
	defer server.Close()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Tunnel.AdminKey = "admin-test"
	cfg.Tunnel.AdminWorkspaceID = "ws_admin"
	cfg.Tunnel.ControlPlaneBaseURL = server.URL
	if err := config.SaveAs(cfg, configformat.JSON); err != nil {
		t.Fatal(err)
	}
	output := executeRequestCommand(t, root, []string{"tunnel", "list", "--no-interactive"})
	var items []tunnel.Metadata
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &items); err != nil || len(items) != 1 || items[0].ID != "tunnel_one" {
		t.Fatalf("output=%q items=%#v err=%v", output, items, err)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"tunnel", "list", "--json", "--interactive"})
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &items); err != nil || len(items) != 1 || items[0].Name != "One" {
		t.Fatalf("json=%q items=%#v err=%v", jsonOutput, items, err)
	}
	if _, err := executeRequestCommandError(root, []string{"tunnel", "list", "--interactive"}); err == nil || !strings.Contains(err.Error(), "requires terminal") {
		t.Fatalf("forced interactive err=%v", err)
	}
}

func TestInteractiveRowsExposeUsefulDetailsWithoutUpstreamSecrets(t *testing.T) {
	workspaceRows := workspaceInteractiveRows([]workspace.Workspace{{ID: "ws_one", Path: "/tmp/project", AllowDirs: []string{"/tmp/shared"}}})
	if len(workspaceRows) != 1 || !strings.Contains(workspaceRows[0].Detail, "/tmp/shared") {
		t.Fatalf("workspace rows=%#v", workspaceRows)
	}
	upstreamRows := upstreamInteractiveRows([]upstream.Server{{ID: "demo", Name: "Demo", Transport: "http", Enabled: true, URL: "https://mcp.example.test", Expose: "all", Headers: map[string]string{"Authorization": "secret-value"}}})
	if len(upstreamRows) != 1 || strings.Contains(upstreamRows[0].Detail, "secret-value") || !strings.Contains(upstreamRows[0].Detail, "<redacted>") {
		t.Fatalf("upstream rows=%#v", upstreamRows)
	}
	tunnelRows := tunnelInteractiveRows([]tunnel.Metadata{{ID: "tunnel_one", Name: "One", WorkspaceIDs: []string{"ws_admin"}}})
	if len(tunnelRows) != 1 || !strings.Contains(tunnelRows[0].Summary, "ws_admin") {
		t.Fatalf("tunnel rows=%#v", tunnelRows)
	}
}

func TestWorkspaceCopyIDActionCopiesExactID(t *testing.T) {
	action := workspaceCopyIDAction()
	notice, cmd, err := action.Run(interactive.Row{ID: "ws_example"})
	if err != nil || notice != "Copied ws_example" || cmd == nil {
		t.Fatalf("notice=%q cmd=%v err=%v", notice, cmd, err)
	}
	if got, want := cmd(), tea.SetClipboard("ws_example")(); !reflect.DeepEqual(got, want) {
		t.Fatalf("clipboard command=%#v want=%#v", got, want)
	}
}
