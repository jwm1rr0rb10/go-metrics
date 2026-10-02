package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCConnectionMonitor(t *testing.T) {
	m, _ := newTestMetrics(t)
	hs, conn := startHealthServer(t, nil)

	mon, err := m.NewGRPCConnectionMonitor(conn, "api", "payments", WithPingInterval(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	gauge := m.grpcConnAvailability.WithLabelValues("api", "payments")

	if err := mon.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !mon.IsAvailable() || testutil.ToFloat64(gauge) != 1 {
		t.Fatal("expected available after synchronous first check")
	}
	if err := mon.Start(context.Background()); !errors.Is(err, ErrMonitorStarted) {
		t.Errorf("second Start: got %v", err)
	}

	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	waitFor(t, "monitor to observe NOT_SERVING", func() bool {
		return !mon.IsAvailable() && testutil.ToFloat64(gauge) == 0
	})

	if err := mon.Close(); err != nil {
		t.Fatal(err)
	}
	if n := testutil.CollectAndCount(m.grpcConnAvailability); n != 0 {
		t.Errorf("Close must remove the series, %d left", n)
	}
	if err := mon.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestGRPCConnectionMonitor_Validation(t *testing.T) {
	m, _ := newTestMetrics(t)
	_, conn := startHealthServer(t, nil)

	if _, err := m.NewGRPCConnectionMonitor(nil, "a", "b"); !errors.Is(err, ErrNilConnection) {
		t.Errorf("nil conn: got %v", err)
	}
	if _, err := m.NewGRPCConnectionMonitor(conn, "a", "b", WithPingInterval(0)); !errors.Is(err, ErrInvalidInterval) {
		t.Errorf("zero interval: got %v", err)
	}
	if _, err := m.NewGRPCConnectionMonitor(conn, "a", "b", WithCheckTimeout(-1)); !errors.Is(err, ErrInvalidInterval) {
		t.Errorf("negative timeout: got %v", err)
	}
}
