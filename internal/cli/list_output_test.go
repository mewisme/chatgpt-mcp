package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestWorkspaceListDefaultsToPlainAndSupportsJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	workspaceRoot := t.TempDir()
	registered := executeRequestCommand(t, root, []string{"workspace", "register", workspaceRoot})
	if !strings.Contains(registered, "Workspace registered") {
		t.Fatalf("register=%q", registered)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"workspace", "list", "--json"})
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
	plain := executeRequestCommand(t, root, []string{"workspace", "list"})
	if !strings.Contains(plain, items[0].Path) || !strings.Contains(plain, "Registered workspaces loaded") {
		t.Fatalf("plain=%q", plain)
	}
}

func TestWorkspaceListShowsUnavailableIndexedWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	healthy := t.TempDir()
	missing := t.TempDir()
	executeRequestCommand(t, root, []string{"workspace", "register", healthy})
	registered := executeRequestCommand(t, root, []string{"workspace", "register", missing})
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	plain := executeRequestCommand(t, root, []string{"workspace", "list"})
	if !strings.Contains(plain, "unavailable") {
		t.Fatalf("plain=%q registered=%q", plain, registered)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"workspace", "list", "--json"})
	var items []workspace.Workspace
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &items); err != nil || len(items) != 2 {
		t.Fatalf("json=%q items=%#v err=%v", jsonOutput, items, err)
	}
	unavailable := 0
	for _, item := range items {
		if !item.Available() {
			unavailable++
			if item.Error == "" {
				t.Fatalf("unavailable workspace missing error: %#v", item)
			}
		}
	}
	if unavailable != 1 {
		t.Fatalf("unavailable count=%d items=%#v", unavailable, items)
	}
}

func TestMCPServerListDefaultsToPlainAndSupportsRedactedJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	if _, err := executeRequestCommandError(root, []string{"mcp", "server", "add", "demo", "--transport", "http", "--url", "https://mcp.example.test", "--header", "Authorization=secret-value"}); err != nil {
		t.Fatal(err)
	}
	plain := executeRequestCommand(t, root, []string{"mcp", "server", "list"})
	if !strings.Contains(plain, "demo") || !strings.Contains(plain, "https://mcp.example.test") || strings.Contains(plain, "secret-value") {
		t.Fatalf("plain=%q", plain)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"mcp", "server", "list", "--json"})
	if strings.Contains(jsonOutput, "secret-value") || !strings.Contains(jsonOutput, "redacted") {
		t.Fatalf("json=%q", jsonOutput)
	}
}

func TestManagedTunnelListDefaultsToPlainAndSupportsJSON(t *testing.T) {
	useSecureMCPAdmin(t)
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
	instances := []tunnel.InstanceConfig{}
	admins := []tunnel.AdminConfig{{ID: "default", AdminKey: "admin-test", WorkspaceID: "ws_admin", ReadAccess: true, ManageAccess: true, ControlPlaneBaseURL: server.URL}}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	if err := config.SaveAs(cfg, configformat.JSON); err != nil {
		t.Fatal(err)
	}
	plain := executeRequestCommand(t, root, []string{"tunnel", "managed", "list"})
	if !strings.Contains(plain, "tunnel_one") || strings.HasPrefix(strings.TrimSpace(plain), "[") {
		t.Fatalf("plain=%q", plain)
	}
	jsonOutput := executeRequestCommand(t, root, []string{"tunnel", "managed", "list", "--json"})
	var items []application.ManagedTunnelDiscovery
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &items); err != nil || len(items) != 1 || items[0].Metadata.Name != "One" {
		t.Fatalf("json=%q items=%#v err=%v", jsonOutput, items, err)
	}
}

func TestWorkspaceShowAndAccessListDefaultToText(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	workspaceRoot := t.TempDir()
	registered := executeRequestCommand(t, root, []string{"workspace", "register", workspaceRoot})
	id := strings.TrimSpace(strings.Split(strings.Split(registered, "id:")[1], "\n")[0])
	showJSON := executeRequestCommand(t, root, []string{"workspace", "show", id, "--json"})
	var item workspace.Workspace
	if err := json.Unmarshal([]byte(strings.TrimSpace(showJSON)), &item); err != nil || item.ID != id {
		t.Fatalf("show json=%q item=%#v err=%v", showJSON, item, err)
	}
	show := executeRequestCommand(t, root, []string{"workspace", "show", id})
	if !strings.Contains(show, "Workspace details") || !strings.Contains(show, item.Path) || !strings.Contains(show, filepath.Join(item.Path, ".cgm")) || strings.HasPrefix(strings.TrimSpace(show), "{") {
		t.Fatalf("show=%q canonical=%q requested=%q", show, item.Path, workspaceRoot)
	}
	access := executeRequestCommand(t, root, []string{"workspace", "access", "list", id})
	if !strings.Contains(access, "Allowed directories loaded") || !strings.Contains(access, "allow dirs: none") || strings.HasPrefix(strings.TrimSpace(access), "[") {
		t.Fatalf("access=%q", access)
	}
	accessJSON := executeRequestCommand(t, root, []string{"workspace", "access", "list", id, "--json"})
	var allowDirs []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(accessJSON)), &allowDirs); err != nil || len(allowDirs) != 0 {
		t.Fatalf("access json=%q allowDirs=%#v err=%v", accessJSON, allowDirs, err)
	}
}

func TestWorkspacePurgeRequiresConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	workspaceRoot := t.TempDir()
	registered := executeRequestCommand(t, root, []string{"workspace", "register", workspaceRoot})
	id := strings.TrimSpace(strings.Split(strings.Split(registered, "id:")[1], "\n")[0])
	output, err := executeRequestCommandError(root, []string{"workspace", "purge", id})
	if err == nil || !strings.Contains(err.Error(), "--confirm") && !strings.Contains(output, "--confirm") {
		t.Fatalf("purge without confirm output=%q err=%v", output, err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".cgm")); err != nil {
		t.Fatalf("local state removed without confirm: %v", err)
	}
	deleted := executeRequestCommand(t, root, []string{"workspace", "purge", id, "--confirm"})
	if !strings.Contains(deleted, "Workspace local state deleted") {
		t.Fatalf("purge=%q", deleted)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".cgm")); !os.IsNotExist(err) {
		t.Fatalf("local state remains: %v", err)
	}
}
