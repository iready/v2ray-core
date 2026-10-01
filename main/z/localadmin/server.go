package localadmin

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

// Server 本地配置后台 HTTP 服务。
type Server struct {
	port int
	ln   net.Listener
	h    *Handler
}

func NewServer(ln net.Listener, port int, store *Store, hooks Hooks) *Server {
	return &Server{
		port: port,
		ln:   ln,
		h:    NewHandler(store, hooks),
	}
}

func (s *Server) Serve() error {
	router := NewRouter(s.h, buildFS, indexPage)
	log.Printf("本地配置后台: http://127.0.0.1:%d", s.port)
	return http.Serve(s.ln, router)
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.port)
}
