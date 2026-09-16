package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestPluginConfigAPIGetPutReset(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	service, err := application.NewPluginService()
	if err != nil {
		t.Fatal(err)
	}
	handler := New(API{Plugins: service})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plugins", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var listed []pluginListItem
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	found := map[string]pluginListItem{}
	for _, item := range listed {
		found[item.ID] = item
	}
	for _, id := range []string{"ponytail", "caveman"} {
		item, ok := found[id]
		if !ok || item.Origin != pluginpkg.OriginBuiltin || !item.Lifecycle.Configure || item.Lifecycle.Install {
			t.Fatalf("%s list item=%#v ok=%t", id, item, ok)
		}
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plugins/ponytail/config", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var view pluginConfigView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ID != "ponytail" || view.Scope != "global" || view.Origin != pluginpkg.OriginBuiltin || view.Values["default_active"] != true || view.Values["default_mode"] != "full" {
		t.Fatalf("get view=%#v", view)
	}
	if len(view.Schema.Fields) == 0 {
		t.Fatal("schema missing")
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/plugins/ponytail/config", strings.NewReader(`{"values":{"default_active":false,"default_mode":"lite"}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Values["default_active"] != false || view.Values["default_mode"] != "lite" {
		t.Fatalf("put view=%#v", view)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/plugins/ponytail/config", strings.NewReader(`{"values":{"default_mode":"review"}}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid put status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/plugins/ponytail/config/reset", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Values["default_active"] != true || view.Values["default_mode"] != "full" {
		t.Fatalf("reset view=%#v", view)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plugins/missing/config", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
