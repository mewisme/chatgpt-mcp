package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type isolationHTTPFixture struct {
	runtime *HTTPRuntime
	first   workspace.Workspace
	second  workspace.Workspace
}

func newIsolationHTTPFixture(t *testing.T) isolationHTTPFixture {
	t.Helper()
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := checkpoint.NewStore(filepath.Join(t.TempDir(), "checkpoints"))
	registry := tools.NewRegistry()
	tools.RegisterCore(registry, manager, checkpoints)
	toolRuntime := &tools.Runtime{Registry: registry, Workspaces: manager, Checkpoints: checkpoints, SessionBindings: tools.NewSessionWorkspaceBinder()}
	return isolationHTTPFixture{runtime: NewHTTPRuntimeWithTools(toolRuntime), first: first, second: second}
}

func callWorkspaceToolHTTP(t *testing.T, runtime *HTTPRuntime, sessionID, name string, args map[string]any, id int) tools.Result {
	t.Helper()
	params := map[string]any{"name": name, "arguments": args}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": params})
	if err != nil {
		t.Fatal(err)
	}
	req := modernRequest("tools/call", string(body))
	req.Header.Set(NameHeader, name)
	req.Header.Set(SessionIDHeader, sessionID)
	res := httptestResponse(runtime, req)
	if res.Code != 200 {
		t.Fatalf("%s status = %d: %s", name, res.Code, res.Body.String())
	}
	response := decodeResponse(t, res)
	if response.Error != nil {
		t.Fatalf("%s protocol error = %#v", name, response.Error)
	}
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	var result tools.Result
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func httptestResponse(runtime *HTTPRuntime, req *http.Request) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	return res
}

func TestHTTPSessionsCannotCrossBoundWorkspaces(t *testing.T) {
	fixture := newIsolationHTTPFixture(t)
	firstPath := filepath.Join(fixture.first.Path, "session-a.txt")
	secondPath := filepath.Join(fixture.second.Path, "session-b.txt")
	if result := callWorkspaceToolHTTP(t, fixture.runtime, "session-a", "write_file", map[string]any{"workspace_id": fixture.first.ID, "path": "session-a.txt", "content": "a"}, 1); result.IsError {
		t.Fatalf("session A first workspace failed: %#v", result)
	}
	if result := callWorkspaceToolHTTP(t, fixture.runtime, "session-b", "write_file", map[string]any{"workspace_id": fixture.second.ID, "path": "session-b.txt", "content": "b"}, 2); result.IsError {
		t.Fatalf("session B second workspace failed: %#v", result)
	}
	if data, err := os.ReadFile(firstPath); err != nil || string(data) != "a" {
		t.Fatalf("first workspace content = %q err=%v", data, err)
	}
	if data, err := os.ReadFile(secondPath); err != nil || string(data) != "b" {
		t.Fatalf("second workspace content = %q err=%v", data, err)
	}
	deniedPath := filepath.Join(fixture.second.Path, "must-not-exist.txt")
	result := callWorkspaceToolHTTP(t, fixture.runtime, "session-a", "write_file", map[string]any{"workspace_id": fixture.second.ID, "path": "must-not-exist.txt", "content": "wrong"}, 3)
	if !result.IsError || len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "cannot access") {
		t.Fatalf("cross-workspace call was not denied: %#v", result)
	}
	if _, err := os.Stat(deniedPath); !os.IsNotExist(err) {
		t.Fatalf("denied cross-workspace write mutated filesystem: %v", err)
	}
	binding, ok := fixture.runtime.Server.Tools.SessionBindings.Lookup("session-a")
	if !ok || binding.WorkspaceID != fixture.first.ID {
		t.Fatalf("session A binding = %#v ok=%t", binding, ok)
	}
}

func TestHTTPManySessionsCanShareWorkspace(t *testing.T) {
	fixture := newIsolationHTTPFixture(t)
	for i, sessionID := range []string{"session-a", "session-b"} {
		name := fmt.Sprintf("shared-%d.txt", i)
		result := callWorkspaceToolHTTP(t, fixture.runtime, sessionID, "write_file", map[string]any{"workspace_id": fixture.first.ID, "path": name, "content": sessionID}, 10+i)
		if result.IsError {
			t.Fatalf("%s sharing workspace failed: %#v", sessionID, result)
		}
		data, err := os.ReadFile(filepath.Join(fixture.first.Path, name))
		if err != nil || string(data) != sessionID {
			t.Fatalf("%s content = %q err=%v", sessionID, data, err)
		}
	}
}

func TestHTTPSessionProjectContextCannotSwitchWorkspaces(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	fixture := newIsolationHTTPFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.first.Path, "AGENTS.md"), []byte("first workspace"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.second.Path, "AGENTS.md"), []byte("second workspace"), 0644); err != nil {
		t.Fatal(err)
	}
	first := callWorkspaceToolHTTP(t, fixture.runtime, "project-session", "project_context", map[string]any{"workspace_id": fixture.first.ID, "include_git": false}, 20)
	if first.IsError || len(first.Content) == 0 || !strings.Contains(first.Content[0].Text, "first workspace") {
		t.Fatalf("first project_context failed: %#v", first)
	}
	denied := callWorkspaceToolHTTP(t, fixture.runtime, "project-session", "project_context", map[string]any{"workspace_id": fixture.second.ID, "include_git": false}, 21)
	if !denied.IsError || len(denied.Content) == 0 || !strings.Contains(denied.Content[0].Text, "cannot access") {
		t.Fatalf("cross-workspace project_context was not denied: %#v", denied)
	}
	binding, ok := fixture.runtime.Server.Tools.SessionBindings.Lookup("project-session")
	if !ok || binding.WorkspaceID != fixture.first.ID {
		t.Fatalf("project_context session binding = %#v ok=%t", binding, ok)
	}
}
