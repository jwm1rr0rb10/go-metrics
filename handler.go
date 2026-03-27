package metrics

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server runs the metrics HTTP server.
type Server struct {
	cfg        *Config
	httpServer *http.Server
}

// NewServer initializes a new metrics server.
func NewServer(cfg *Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Server{cfg: cfg}, nil
}

// Run starts the metrics server.
func (s *Server) Run(_ context.Context) error {
	router := httprouter.New()
	router.Handler(http.MethodGet, "/metrics", promhttp.Handler())

	s.httpServer = &http.Server{
		Addr:              s.cfg.address,
		Handler:           router,
		ReadTimeout:       s.cfg.readTimeout,
		WriteTimeout:      s.cfg.writeTimeout,
		ReadHeaderTimeout: s.cfg.readHeaderTimeout,
	}
	return s.httpServer.ListenAndServe()
}

// Close shuts down the server.
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}
