package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/activity"
)

type HTTPRuntime struct {
	Server    *Runtime
	Activity  *activity.Stream
	Sessions  *SessionStore
	Lifecycle *Lifecycle
}

func NewHTTPRuntime() *HTTPRuntime {
	stream := activity.NewStream()
	sessions := NewSessionStore()
	return &HTTPRuntime{Server: NewRuntime(), Activity: stream, Sessions: sessions, Lifecycle: NewLifecycle(sessions, stream)}
}

func (h *HTTPRuntime) Handler() http.Handler { return h }

func (h HTTPRuntime) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.serveNotificationStream(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		id := ReadSessionID(r)
		if id == "" {
			writeError(w, ErrInvalidRequest, "missing MCP-Session-Id")
			return
		}
		if _, ok := h.Sessions.Get(id); !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		h.Lifecycle.Delete(id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrParse, err.Error())
		return
	}
	if err := ValidateRequest(req); err != nil {
		protocolErr := err.(*Error)
		writeErrorID(w, req.ID, protocolErr.Code, protocolErr.Message)
		return
	}
	if !IsSupportedMethod(req.Method) {
		writeErrorID(w, req.ID, ErrMethodNotFound, "method not found")
		return
	}
	params, err := DecodeParams(req.Params)
	if err != nil {
		protocolErr := err.(*Error)
		writeErrorID(w, req.ID, protocolErr.Code, protocolErr.Message)
		return
	}
	if err := ValidateParams(req.Method, params); err != nil {
		protocolErr := err.(*Error)
		writeErrorID(w, req.ID, protocolErr.Code, protocolErr.Message)
		return
	}
	if req.Method == "initialize" {
		id := h.Lifecycle.Create()
		SetSessionID(w, id)
	} else {
		id := ReadSessionID(r)
		if id == "" {
			writeErrorID(w, req.ID, ErrInvalidRequest, "missing MCP-Session-Id")
			return
		}
		if _, ok := h.Sessions.Get(id); !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		h.EmitActivity("session.reconnected", id)
	}
	h.EmitActivity("mcp.request", req.Method)
	if req.Method == "tools/call" {
		if name, ok := params["name"].(string); ok {
			h.EmitActivity("tool.call", name)
		}
	}
	result, err := h.Server.Handle(context.Background(), req.Method, params)
	if err != nil {
		writeErrorID(w, req.ID, ErrInternal, err.Error())
		return
	}
	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: result})
}
