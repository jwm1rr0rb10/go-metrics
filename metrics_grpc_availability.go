package metrics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	defaultPingInterval = 5 * time.Second
	defaultCheckTimeout = 2 * time.Second
)

type monitorOptions struct {
	pingInterval  time.Duration
	checkTimeout  time.Duration
	healthService string
}

// MonitorOption configures a GRPCConnectionMonitor.
type MonitorOption func(*monitorOptions)

// WithPingInterval sets how often the health check runs (default: 5s).
func WithPingInterval(d time.Duration) MonitorOption {
	return func(o *monitorOptions) { o.pingInterval = d }
}

// WithCheckTimeout sets the timeout of a single health check (default: 2s).
func WithCheckTimeout(d time.Duration) MonitorOption {
	return func(o *monitorOptions) { o.checkTimeout = d }
}

// WithHealthService sets the service name sent in HealthCheckRequest
// (default: "", i.e. the overall server health).
func WithHealthService(name string) MonitorOption {
	return func(o *monitorOptions) { o.healthService = name }
}

// GRPCConnectionMonitor periodically checks a downstream gRPC service through
// the standard grpc.health.v1 protocol and exports grpc_connection_availability.
// It is safe for concurrent use.
type GRPCConnectionMonitor struct {
	health      grpc_health_v1.HealthClient
	gauge       Gauge
	deleteGauge func()
	opts        monitorOptions
	available   atomic.Bool

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewGRPCConnectionMonitor creates a monitor for conn. serviceFrom and
// serviceTo become the from_service and to_service labels.
func (m *Metrics) NewGRPCConnectionMonitor(
	conn grpc.ClientConnInterface,
	serviceFrom, serviceTo string,
	opts ...MonitorOption,
) (*GRPCConnectionMonitor, error) {
	if conn == nil {
		return nil, ErrNilConnection
	}
	o := monitorOptions{pingInterval: defaultPingInterval, checkTimeout: defaultCheckTimeout}
	for _, opt := range opts {
		opt(&o)
	}
	if o.pingInterval <= 0 || o.checkTimeout <= 0 {
		return nil, ErrInvalidInterval
	}

	return &GRPCConnectionMonitor{
		health: grpc_health_v1.NewHealthClient(conn),
		gauge:  m.grpcConnAvailability.WithLabelValues(serviceFrom, serviceTo),
		deleteGauge: func() {
			m.grpcConnAvailability.DeleteLabelValues(serviceFrom, serviceTo)
		},
		opts: o,
	}, nil
}

// IsAvailable reports the result of the latest health check.
func (s *GRPCConnectionMonitor) IsAvailable() bool {
	return s.available.Load()
}

// Start runs the first check synchronously and then keeps checking in the
// background until ctx is canceled or Close is called. It returns
// ErrMonitorStarted if called more than once.
func (s *GRPCConnectionMonitor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return ErrMonitorStarted
	}
	s.started = true

	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})

	s.check(ctx)

	go func() {
		defer close(s.done)
		ticker := time.NewTicker(s.opts.pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.check(ctx)
			}
		}
	}()
	return nil
}

func (s *GRPCConnectionMonitor) check(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.opts.checkTimeout)
	defer cancel()

	resp, err := s.health.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: s.opts.healthService})
	serving := err == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING

	s.available.Store(serving)
	if serving {
		s.gauge.Set(1)
	} else {
		s.gauge.Set(0)
	}
}

// Close stops the monitor, waits for the background goroutine to exit and
// removes the availability series so no stale value is exported.
// It is idempotent.
func (s *GRPCConnectionMonitor) Close() error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel = nil
	s.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	s.available.Store(false)
	s.deleteGauge()
	return nil
}
