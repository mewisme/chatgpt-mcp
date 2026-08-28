package mcp

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

type HTTPRuntime struct {
	Server   *Runtime
	Activity *activity.Stream
}

func NewHTTPRuntime() *HTTPRuntime { return NewHTTPRuntimeWithTools(tools.NewRuntime()) }

func NewHTTPRuntimeWithTools(toolRuntime *tools.Runtime) *HTTPRuntime {
	return &HTTPRuntime{Server: NewRuntimeWithTools(toolRuntime), Activity: activity.NewStream()}
}

func (h *HTTPRuntime) Handler() http.Handler { return h }

func (h HTTPRuntime) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := decodeHTTPRequest(w, r, &req); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
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
	if headerErr := ValidateHTTPHeaders(r, req, params); headerErr != nil {
		writeErrorStatusID(w, http.StatusBadRequest, req.ID, headerErr.Code, headerErr.Message)
		return
	}
	if err := ValidateParams(req.Method, params); err != nil {
		protocolErr := err.(*Error)
		writeErrorID(w, req.ID, protocolErr.Code, protocolErr.Message)
		return
	}

	h.EmitActivity("mcp.request", req.Method)
	if req.Method == "tools/call" {
		if name, ok := params["name"].(string); ok {
			h.EmitActivity("tool.call", name)
		}
	}

	result, err := h.Server.Handle(r.Context(), req.Method, params)
	if err != nil {
		var protocolErr *Error
		if errors.As(err, &protocolErr) {
			writeErrorID(w, req.ID, protocolErr.Code, protocolErr.Message)
			return
		}
		writeErrorID(w, req.ID, ErrInternal, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: result})
}
