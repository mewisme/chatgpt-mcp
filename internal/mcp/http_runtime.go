package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/activity"
)

type HTTPRuntime struct {
	Server   *Runtime
	Activity *activity.Stream
}

func NewHTTPRuntime() *HTTPRuntime {
	return &HTTPRuntime{Server: NewRuntime(), Activity: activity.NewStream()}
}

func (h *HTTPRuntime) Handler() http.Handler { return h }

func (h HTTPRuntime) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, -32700, err.Error())
		return
	}
	h.Activity.Publish(activity.Event{Kind: "mcp.request", Message: req.Method})
	var params map[string]any
	_ = json.Unmarshal(req.Params, &params)
	result, err := h.Server.Handle(context.Background(), req.Method, params)
	if err != nil {
		writeError(w, -32000, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: result})
}
