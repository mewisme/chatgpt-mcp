package mcp

import (
	"encoding/json"
	"net/http"
)

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", Error: &Error{Code: code, Message: message}})
}

func HandleRequest(runtime ToolRuntime, req Request) Response {
	response := Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "tools/list":
		response.Result = map[string]any{"tools": runtime.ListTools()}
	default:
		response.Error = &Error{Code: -32601, Message: "method not found"}
	}
	return response
}
