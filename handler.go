package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
		return nil, fmt.Errorf("invalid metrics config: %w", err)
	}
	return &Server{cfg: cfg}, nil
}

// Run starts the metrics server and blocks until the context is canceled
// or the server encounters an error. Uses graceful shutdown on context cancellation.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	s.httpServer = &http.Server{
		Addr:              s.cfg.address,
		Handler:           mux,
		ReadTimeout:       s.cfg.readTimeout,
		WriteTimeout:      s.cfg.writeTimeout,
		ReadHeaderTimeout: s.cfg.readHeaderTimeout,
	}

	errCh := make(chan error, 1)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("metrics server shutdown failed: %w", err)
		}
		return ctx.Err()
	}
}

// Close forces an immediate shutdown (non-graceful). Prefer canceling the context passed to Run().
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}
