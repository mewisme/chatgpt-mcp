package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`

func initializeSession(t *testing.T, runtime *HTTPRuntime) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initializeBody))
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, want %d", res.Code, http.StatusOK)
	}
	id := res.Header().Get(SessionHeader)
	if id == "" {
		t.Fatal("missing MCP-Session-Id")
	}
	if _, ok := runtime.Sessions.Get(id); !ok {
		t.Fatal("session was not stored")
	}
	return id
}

func TestHTTPRuntimeInitializeCreatesSession(t *testing.T) { initializeSession(t, NewHTTPRuntime()) }

func TestHTTPRuntimeInvalidToolParams(t *testing.T) {
	runtime := NewHTTPRuntime()
	id := initializeSession(t, runtime)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"arguments":{}}}`))
	req.Header.Set(SessionHeader, id)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	var response Response
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != float64(2) && response.ID != 2 {
		t.Fatalf("response id = %#v", response.ID)
	}
	if response.Error == nil || response.Error.Code != ErrInvalidParams {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrInvalidParams)
	}
}

func TestHTTPRuntimeNotificationHasNoJSONRPCResponse(t *testing.T) {
	runtime := NewHTTPRuntime()
	id := initializeSession(t, runtime)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set(SessionHeader, id)
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	if res.Body.Len() != 0 {
		t.Fatalf("unexpected response body: %s", res.Body.String())
	}
}

func TestHTTPRuntimePublishesSessionNotification(t *testing.T) {
	runtime := NewHTTPRuntime()
	id := initializeSession(t, runtime)
	session, _ := runtime.Sessions.Get(id)
	runtime.PublishToolsChanged()
	select {
	case notification := <-session.Notifications:
		if notification.Method != "notifications/tools/list_changed" {
			t.Fatalf("method = %q", notification.Method)
		}
	default:
		t.Fatal("notification was not delivered")
	}
}

func TestSessionDeleteClosesStreamSignal(t *testing.T) {
	runtime := NewHTTPRuntime()
	id := initializeSession(t, runtime)
	session, _ := runtime.Sessions.Get(id)
	runtime.Lifecycle.Delete(id)
	select {
	case <-session.Done:
	default:
		t.Fatal("session done signal was not closed")
	}
}

func TestHTTPRuntimeRejectsOversizedBody(t *testing.T) {
	runtime := NewHTTPRuntime()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{},"padding":"` + strings.Repeat("x", int(MaxRequestBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHTTPRuntimeRejectsTrailingJSON(t *testing.T) {
	runtime := NewHTTPRuntime()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initializeBody+`{}`))
	res := httptest.NewRecorder()
	runtime.ServeHTTP(res, req)
	var response Response
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != ErrParse {
		t.Fatalf("error = %#v, want code %d", response.Error, ErrParse)
	}
}
