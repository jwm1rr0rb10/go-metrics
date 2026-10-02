package metrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server runs the metrics HTTP server. It is safe for concurrent use: Close
// may be called from any goroutine while Run or Serve is running.
type Server struct {
	cfg *Config

	mu         sync.Mutex
	httpServer *http.Server
	closed     bool
}

// NewServer initializes a new metrics server.
func NewServer(cfg *Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid metrics config: %w", err)
	}
	return &Server{cfg: cfg}, nil
}

// Run listens on the configured address and serves until ctx is canceled or
// Close is called. Listen errors (e.g. port in use) are returned immediately.
// Returns nil after a graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.cfg.Address())
	if err != nil {
		return fmt.Errorf("metrics server listen: %w", err)
	}
	return s.Serve(ctx, ln)
}

// Serve is like Run but accepts an existing listener (useful for tests or
// socket activation). It takes ownership of ln.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.mu.Lock()
	switch {
	case s.closed:
		s.mu.Unlock()
		_ = ln.Close()
		return ErrServerClosed
	case s.httpServer != nil:
		s.mu.Unlock()
		_ = ln.Close()
		return ErrServerStarted
	}

	mux := http.NewServeMux()
	mux.Handle(s.cfg.path, promhttp.HandlerFor(s.cfg.gatherer, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	srv := &http.Server{
		Handler:           mux,
		ReadTimeout:       s.cfg.readTimeout,
		WriteTimeout:      s.cfg.writeTimeout,
		ReadHeaderTimeout: s.cfg.readHeaderTimeout,
	}
	s.httpServer = srv
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("metrics server shutdown failed: %w", err)
		}
		return nil
	}
}

// Close forces an immediate shutdown (non-graceful). Prefer canceling the
// context passed to Run. After Close the server cannot be started again.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}
