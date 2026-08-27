package mcp

import "net/http"

func Route(server *HTTPRuntime) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", server)
	mux.Handle("/mcp/", server)
	return mux
}
