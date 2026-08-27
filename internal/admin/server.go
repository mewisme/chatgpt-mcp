package admin

import "net/http"

type Server struct{}

func (s *Server) Handler() http.Handler {
	return http.NewServeMux()
}
