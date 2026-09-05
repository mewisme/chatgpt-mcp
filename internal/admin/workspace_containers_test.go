package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestWorkspaceContainerAPICRUDAndMembership(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := New(API{Workspaces: manager})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/workspace-containers", strings.NewReader(`{"name":"Backend"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var container workspace.WorkspaceContainer
	if err := json.Unmarshal(recorder.Body.Bytes(), &container); err != nil {
		t.Fatal(err)
	}
	if container.ID == "" || container.Name != "Backend" {
		t.Fatalf("container=%#v", container)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/workspace-containers/"+container.ID+"/workspaces", strings.NewReader(`{"workspace_ids":["`+first.ID+`","`+second.ID+`"]}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), first.ID) || !strings.Contains(recorder.Body.String(), second.ID) {
		t.Fatalf("add status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/workspaces/"+first.ID+"/containers", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), container.ID) {
		t.Fatalf("workspace containers status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/workspaces/"+first.ID+"/containers", strings.NewReader(`{"container_ids":["`+container.ID+`"]}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("workspace remove status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/workspace-containers/"+container.ID, strings.NewReader(`{"name":"Services"}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"Services"`) {
		t.Fatalf("rename status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/workspace-containers/"+container.ID, nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
