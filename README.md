# Metrics

**A Lightweight and Convenient Library for Prometheus Metrics in Go Services**

This library provides ready-to-use tools for collecting and exporting metrics:
- An HTTP server for the `/metrics` endpoint
- Middleware for HTTP requests
- A unary interceptor for gRPC
- Monitoring of gRPC connection availability via health checks
- Convenient wrappers around `prometheus/client_golang`

## Features

- Fully configurable HTTP metrics server (host, port, timeouts)
- Automatic collection of HTTP request durations (`http_request_duration_seconds`)
- Automatic collection of gRPC request durations (`grpc_request_duration_seconds`)
- A Gauge metric for gRPC connection availability (`grpc_connection_availability`)
- Helpers for creating Counter, Gauge, Histogram, and Vec metrics
- Configuration validation and sensible default values
- Support for a custom `prometheus.Registerer`

## Installation

```bash
go get github.com/jwm1rr0rb10/go-metrics 
```

---

The library depends on:

- `github.com/prometheus/client_golang`
- `github.com/julienschmidt/httprouter`
- `google.golang.org/grpc + google.golang.org/grpc/health/grpc_health_v1`

After adding the module, execute `go mod tidy`.

---

## Quick Start
## 1. Launching the Metrics Server

```go
package main

import (
	"context"
	"log"

	"github.com/jwm1rr0rb10/go-metrics"
)

func main() {
	cfg := metrics.NewConfig(
		metrics.WithHost("0.0.0.0"),
		metrics.WithPort(8081),
		metrics.WithReadTimeout(15*time.Second),
	)

	server, err := metrics.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Run(ctx); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	log.Printf("Metrics available at http://%s/metrics", cfg.Address()) 
	

	<-ctx.Done() // graceful shutdown
	if err := server.Close(); err != nil {
		log.Printf("Failed to close metrics server: %v", err)
	}
}

```

---

## 2. HTTP Middleware

```go
mux := http.NewServeMux()
mux.HandleFunc("/api/health", healthHandler)

// Wrap the middleware
handler := metrics.RequestDurationMetricHTTPMiddleware(mux)
http.ListenAndServe(":8080", handler)
```

Metric: `http_request_duration_seconds` (histogram) with labels `method`, `status_code`.

---

## 3. gRPC Interceptor

```go
grpcServer := grpc.NewServer(
	grpc.UnaryInterceptor(metrics.RequestDurationMetricUnaryServerInterceptor("auth_service")),
)
```
Metric: `grpc_request_duration_seconds` (histogram) with labels `service`, `method`, `is_error`

---

## 4. gRPC Connection Monitoring

```go
type myClient struct {
	conn grpc.ClientConnInterface
}

func (c *myClient) Connection() grpc.ClientConnInterface { return c.conn }

monitor := metrics.NewGRPCConnectionMonitor(
	&myClient{conn: externalConn},
	5*time.Second,      // Ping interval
	"my_service",       // from_service
	"payment_service",  // to_service
)

monitor.Start(ctx)

// On shutdown
defer monitor.Close()
```

Metric: `grpc_connection_availability` (gauge 0/1) with labels `from_service`, `to_service`.

---

### Configuration

```go
cfg := metrics.NewConfig(
	metrics.WithHost("127.0.0.1"),
	metrics.WithPort(9090),
	metrics.WithReadTimeout(30 * time.Second),
	metrics.WithWriteTimeout(30 * time.Second),
	metrics.WithReadHeaderTimeout(5 * time.Second),
)
```

Default values:


- host: 0.0.0.0
- port: 8080
- read timeout: 10s
- write timeout: 10s
- read header timeout: 5s

---

### Validation
`cfg.Validate()` (called automatically in `NewServer`).

---

Custom Metrics

```go
counter := metrics.NewCounter(prometheus.CounterOpts{
	Name: "my_custom_counter",
	Help: "The quantity of something",
})

gaugeVec := metrics.NewGaugeVec(prometheus.GaugeOpts{
	Name: "my_gauge",
	Help: "The meaning of something",
}, []string{"label1", "label2"})

```

You can change the case:

```go
metrics.SetRegisterer(myCustomRegisterer)
```

---

## Экспортируемые метрики
| Metrics                       | Type      | Label                       | Description                       |
|:------------------------------|:----------|:----------------------------|:----------------------------------|
| http_request_duration_seconds | Histogram | "method, status_code"       | HTTP Request Duration       |
| grpc_request_duration_seconds | Histogram | "service, method, is_error" | gRPC Request Duration        |
| grpc_connection_availability  | Gauge     | "from_service, to_service"  | gRPC Connection Availability (1/0) |

---

### Testing
The repository contains `metrics_test.go`, which includes tests for config validation and the server lifecycle.

---

### Complete Usage Example

Refer to the tests and the following files: `config.go`, `handler.go`, `http_middleware.go`, `grpc_middleware.go`, and `metrics_grpc_availability.go`.

---

## License

[MIT License](https://github.com/jwm1rr0rb10/go-metrics/blob/main/LICENSE) – © Raman Zaitsau [@jwm1rrr0rb10](https://github.com/jwm1rr0rb10)

---

### 🙌 Contributions

PRs and suggestions welcome! Feel free to fork, open issues, or create a pull request 🚀