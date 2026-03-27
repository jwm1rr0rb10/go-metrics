package metrics

import (
	"context"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

var (
	gRPCConnectionAvailability = NewGaugeVec(
		GaugeOpts{
			Name: "grpc_connection_availability",
			Help: "Current availability (1 = available, 0 = unavailable)",
		},
		[]string{"from_service", "to_service"},
	)
)

// GRPCService defines the interface for gRPC connections.
type GRPCService interface {
	Connection() grpc.ClientConnInterface
}

// GRPCConnectionMonitor periodically checks gRPC service health.
type GRPCConnectionMonitor struct {
	service     GRPCService
	pingTimer   time.Duration
	serviceFrom string
	serviceTo   string
	available   int32
	cancel      context.CancelFunc
}

// NewGRPCConnectionMonitor creates a new monitor.
func NewGRPCConnectionMonitor(
	service GRPCService,
	pingTimer time.Duration,
	serviceFrom string,
	serviceTo string,
) *GRPCConnectionMonitor {
	return &GRPCConnectionMonitor{
		service:     service,
		pingTimer:   pingTimer,
		serviceFrom: serviceFrom,
		serviceTo:   serviceTo,
	}
}

// IsAvailable returns the current connection status.
func (s *GRPCConnectionMonitor) IsAvailable() bool {
	return atomic.LoadInt32(&s.available) == 1
}

// Start begins health checks.
func (s *GRPCConnectionMonitor) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	healthClient := grpc_health_v1.NewHealthClient(s.service.Connection())
	gRPCConnectionAvailability.WithLabelValues(s.serviceFrom, s.serviceTo).Set(0)

	checkConnection := func() {
		ctxCheck, cancelCheck := context.WithTimeout(ctx, 2*time.Second)
		defer cancelCheck()

		check, err := healthClient.Check(ctxCheck, &grpc_health_v1.HealthCheckRequest{})
		status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
		if err == nil {
			status = check.Status
		}

		if status == grpc_health_v1.HealthCheckResponse_SERVING {
			atomic.StoreInt32(&s.available, 1)
			gRPCConnectionAvailability.WithLabelValues(s.serviceFrom, s.serviceTo).Set(1)
		} else {
			atomic.StoreInt32(&s.available, 0)
			gRPCConnectionAvailability.WithLabelValues(s.serviceFrom, s.serviceTo).Set(0)
		}
	}

	ticker := time.NewTicker(s.pingTimer)
	go func() {
		defer ticker.Stop()
		checkConnection() // initial check
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkConnection()
			}
		}
	}()
}

// Close stops the monitor.
func (s *GRPCConnectionMonitor) Close() error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	return nil
}
