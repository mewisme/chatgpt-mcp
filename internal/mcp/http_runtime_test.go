package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func modernRequest(method, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set(ProtocolVersionHeader, SupportedProtocolVersion)
	req.Header.Set(MethodHeader, method)
	return req
}

func decodeResponse(t *testing.T, res *httptest.ResponseRecorder) Response {
	t.Helper()
	var response Response
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestHTTPRuntimeDiscoverIsStateless(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("server/discover", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test-client","version":"1.0.0"}}}}`)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", res.Code, http.StatusOK, res.Body.String())
	}
	if res.Header().Get("MCP-Session-Id") != "" {
		t.Fatal("modern response must not create MCP-Session-Id")
	}
	response := decodeResponse(t, res)
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", response.Result)
	}
	versions, ok := result["supportedVersions"].([]any)
	if !ok || len(versions) != 1 || versions[0] != SupportedProtocolVersion {
		t.Fatalf("supportedVersions = %#v", result["supportedVersions"])
	}
	if result["cacheScope"] != defaultCacheScope || result["ttlMs"] != float64(defaultCacheTTLMS) {
		t.Fatalf("cache hints = %#v/%#v", result["ttlMs"], result["cacheScope"])
	}
}

func TestHTTPRuntimeToolsListHasCacheHints(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("tools/list", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", res.Code, http.StatusOK, res.Body.String())
	}
	response := decodeResponse(t, res)
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", response.Result)
	}
	if result["cacheScope"] != defaultCacheScope || result["ttlMs"] != float64(defaultCacheTTLMS) {
		t.Fatalf("cache hints = %#v/%#v", result["ttlMs"], result["cacheScope"])
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools = %#v", result["tools"])
	}
	var previous string
	for i, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("tool[%d] = %#v", i, item)
		}
		name, _ := tool["name"].(string)
		if previous != "" && name < previous {
			t.Fatalf("tools are not sorted: %q before %q", previous, name)
		}
		previous = name
	}
}

func TestHTTPRuntimeToolCallRequiresMatchingNameHeader(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("tools/call", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"git_status","arguments":{"working_directory":"."}}}`)
	req.Header.Set(NameHeader, "read_text_file")
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	response := decodeResponse(t, res)
	if response.Error == nil || response.Error.Code != ErrHeaderMismatch {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrHeaderMismatch)
	}
}

func TestHTTPRuntimeRejectsMissingProtocolHeader(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":4,"method":"tools/list"}`))
	req.Header.Set(MethodHeader, "tools/list")
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	response := decodeResponse(t, res)
	if response.Error == nil || response.Error.Code != ErrUnsupportedProtocolVersion {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrUnsupportedProtocolVersion)
	}
}

func TestHTTPRuntimeRejectsMethodHeaderMismatch(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("tools/call", `{"jsonrpc":"2.0","id":5,"method":"tools/list"}`)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	response := decodeResponse(t, res)
	if response.Error == nil || response.Error.Code != ErrHeaderMismatch {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrHeaderMismatch)
	}
}

func TestHTTPRuntimeRejectsInvalidToolParams(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("tools/call", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"arguments":{}}}`)
	req.Header.Set(NameHeader, "missing")
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	response := decodeResponse(t, res)
	if response.Error == nil || response.Error.Code != ErrHeaderMismatch {
		t.Fatalf("error = %#v, want header validation before params validation", response.Error)
	}
}

func TestHTTPRuntimeRejectsLegacyMethods(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("initialize", `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}`)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	response := decodeResponse(t, res)
	if response.Error == nil || response.Error.Code != ErrMethodNotFound {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrMethodNotFound)
	}
}

func TestHTTPRuntimeRejectsGETAndDELETE(t *testing.T) {
	runtime := NewHTTPRuntime()
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		res := httptest.NewRecorder()
		runtime.ServeHTTP(res, req)
		if res.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want %d", method, res.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHTTPRuntimeRejectsOversizedBody(t *testing.T) {
	runtime := NewHTTPRuntime()
	body := `{"jsonrpc":"2.0","id":8,"method":"server/discover","params":{},"padding":"` + strings.Repeat("x", int(MaxRequestBodyBytes)) + `"}`
	req := modernRequest("server/discover", body)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHTTPRuntimeRejectsTrailingJSON(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := modernRequest("server/discover", `{"jsonrpc":"2.0","id":9,"method":"server/discover"}{}`)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	response := decodeResponse(t, res)
	if response.Error == nil || response.Error.Code != ErrParse {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrParse)
	}
}
