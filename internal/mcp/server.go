package mcp

import "net/http"

// Server is the MCP runtime HTTP boundary.
// MCP protocol implementation will be attached here in later phases.
type Server struct {
	Handler http.Handler
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) HandlerFunc() http.Handler {
	if s.Handler != nil {
		return s.Handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"mcp runtime not implemented"}`))
	})
}
