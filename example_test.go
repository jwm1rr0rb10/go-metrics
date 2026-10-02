package metrics_test

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jwm1rr0rb10/go-metrics/v2"
)

func Example() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Print(err)
	}
}

func run(ctx context.Context) error {
	reg := prometheus.NewRegistry()
	m, err := metrics.New(
		metrics.WithRegisterer(reg),
		metrics.WithNamespace("myapp"),
		metrics.WithConstLabels(metrics.Labels{"service": "auth"}),
	)
	if err != nil {
		return err
	}

	// Metrics endpoint on :9090/metrics.
	srv, err := metrics.NewServer(metrics.NewConfig(
		metrics.WithPort(9090),
		metrics.WithServerGatherer(m.Gatherer()),
	))
	if err != nil {
		return err
	}
	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	// HTTP: the route label is taken from the ServeMux pattern.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(http.ResponseWriter, *http.Request) {})
	go func() { _ = http.ListenAndServe(":8080", m.HTTPMiddleware()(mux)) }()

	// gRPC server and client.
	_ = grpc.NewServer(
		grpc.ChainUnaryInterceptor(m.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(m.StreamServerInterceptor()),
	)
	conn, err := grpc.NewClient("payments:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(m.UnaryClientInterceptor()),
		grpc.WithChainStreamInterceptor(m.StreamClientInterceptor()),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Downstream availability gauge.
	mon, err := m.NewGRPCConnectionMonitor(conn, "auth", "payments", metrics.WithPingInterval(5*time.Second))
	if err != nil {
		return err
	}
	if err := mon.Start(ctx); err != nil {
		return err
	}
	defer mon.Close()

	<-ctx.Done()
	return nil
}
