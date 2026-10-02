# go-metrics

[![CI](https://github.com/jwm1rr0rb10/go-metrics/actions/workflows/ci.yml/badge.svg)](https://github.com/jwm1rr0rb10/go-metrics/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jwm1rr0rb10/go-metrics/v2.svg)](https://pkg.go.dev/github.com/jwm1rr0rb10/go-metrics/v2)
![Go](https://img.shields.io/badge/go-1.27.1%2B-00ADD8?logo=go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Русская версия](READMEru.md)

Prometheus instrumentation for Go services that run under heavy load. It covers HTTP, gRPC (server and client, unary and streaming), downstream availability checks and the `/metrics` endpoint. It adds little overhead, keeps label cardinality bounded and has no global state.

## Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Usage](#usage)
  - [Metrics instance](#metrics-instance)
  - [/metrics server](#metrics-server)
  - [HTTP middleware](#http-middleware)
  - [gRPC interceptors](#grpc-interceptors)
  - [Downstream availability](#downstream-availability)
  - [Custom metrics](#custom-metrics)
- [Configuration reference](#configuration-reference)
- [Exported metrics](#exported-metrics)
- [PromQL examples](#promql-examples)
- [Guarantees](#guarantees)
- [Performance](#performance)
- [Migrating from v1](#migrating-from-v1)
- [Development](#development)
- [License](#license)

## Features

- **HTTP middleware**: `method`, `route` and `status_code` labels. Panics are recorded and re-raised. SSE and WebSocket keep working.
- **gRPC interceptors**: server and client, unary and streaming, labeled by the real `grpc_code`.
- **Availability monitor** for downstream gRPC services, based on the standard `grpc.health.v1` protocol.
- **`/metrics` server** with timeouts, graceful shutdown and support for a custom registry.
- **Configurable histograms**: buckets and optional native (sparse) histograms.
- **Namespace and constant labels** for every metric.
- **No import side effects**: you create collectors explicitly. Registration conflicts return an error instead of panicking.

## Requirements

| Component | Version |
|---|---|
| Go | **1.27.1+** |
| `github.com/prometheus/client_golang` | 1.24+ |
| `google.golang.org/grpc` | 1.84+ |
| Prometheus (native histograms only) | a version with native histogram scraping enabled |

Route labels from `http.ServeMux` rely on `Request.Pattern`, which is available in every supported Go version.

## Installation

```bash
go get github.com/jwm1rr0rb10/go-metrics/v2
```

## Quick start

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	"github.com/jwm1rr0rb10/go-metrics/v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := prometheus.NewRegistry()
	m, err := metrics.New(
		metrics.WithRegisterer(reg),
		metrics.WithNamespace("myapp"),
		metrics.WithConstLabels(metrics.Labels{"service": "auth"}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// :9090/metrics
	srv, err := metrics.NewServer(metrics.NewConfig(
		metrics.WithPort(9090),
		metrics.WithServerGatherer(m.Gatherer()),
	))
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	// HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {})
	go func() { _ = http.ListenAndServe(":8080", m.HTTPMiddleware()(mux)) }()

	// gRPC
	_ = grpc.NewServer(
		grpc.ChainUnaryInterceptor(m.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(m.StreamServerInterceptor()),
	)

	<-ctx.Done()
}
```

A runnable version is in [`example_test.go`](example_test.go).

## Usage

### Metrics instance

`metrics.New` creates and registers every collector the package exports. Create **one** instance per registry and share it across the process.

```go
m, err := metrics.New(
	metrics.WithRegisterer(reg),                     // default: prometheus.DefaultRegisterer
	metrics.WithNamespace("myapp"),                  // myapp_http_requests_total, ...
	metrics.WithConstLabels(metrics.Labels{"env": "prod"}),
	metrics.WithHTTPBuckets([]float64{.005, .01, .05, .1, .5, 1}),
	metrics.WithNativeHistograms(1.1, 160),
)
```

- If registration conflicts with an existing collector, `New` returns an error and leaves nothing half-registered. `MustNew` panics instead, which is convenient in `main`.
- `m.Gatherer()` returns the registry when it is also a `prometheus.Gatherer`. For a wrapped registerer, pass the gatherer explicitly with `WithGatherer`.
- `m.Unregister()` removes all collectors, which is useful in tests.

### `/metrics` server

```go
srv, err := metrics.NewServer(metrics.NewConfig(
	metrics.WithHost("0.0.0.0"),
	metrics.WithPort(9090),
	metrics.WithPath("/metrics"),
	metrics.WithServerGatherer(m.Gatherer()), // required with a custom registry
))
if err != nil {
	return err // validation error: port, host, path, timeouts
}

err = srv.Run(ctx)            // listen and serve; nil after graceful shutdown on ctx cancel
err = srv.Serve(ctx, ln)      // same with your own net.Listener (tests, socket activation)
err = srv.Close()             // immediate stop, safe from any goroutine
```

`Run` returns listen errors such as a busy port immediately. Calling `Run` or `Serve` twice returns `ErrServerStarted`. After `Close` they return `ErrServerClosed`. The endpoint supports the OpenMetrics format.

### HTTP middleware

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser)

handler := m.HTTPMiddleware()(mux) // route="GET /users/{id}"
```

The middleware has the standard `func(http.Handler) http.Handler` shape, so it chains with any other middleware.

**Route label.** By default the label comes from `r.Pattern`, which `http.ServeMux` fills in. Requests no route matched get `route="unmatched"`. The raw URL path is never used. For other routers:

```go
// chi
m.HTTPMiddleware(metrics.WithRouteFunc(func(r *http.Request) string {
	return chi.RouteContext(r.Context()).RoutePattern()
}))

// a fixed label for a single handler
m.HTTPMiddleware(metrics.WithRoute("/static"))
```

**Behavior:**
- Non-standard HTTP methods are recorded as `OTHER`.
- The status code is the first final code sent. 1xx informational codes are ignored, except `101`. A missing `WriteHeader` call counts as `200`.
- A panic is recorded as `500` if no header was written yet, then re-raised unchanged so your recovery middleware still sees it.
- The wrapped writer supports `http.Flusher`, `io.ReaderFrom`, `http.Hijacker` (only if the original writer supports it) and `http.ResponseController`.

### gRPC interceptors

```go
server := grpc.NewServer(
	grpc.ChainUnaryInterceptor(m.UnaryServerInterceptor()),
	grpc.ChainStreamInterceptor(m.StreamServerInterceptor()),
)

conn, err := grpc.NewClient(target,
	grpc.WithTransportCredentials(creds),
	grpc.WithChainUnaryInterceptor(m.UnaryClientInterceptor()),
	grpc.WithChainStreamInterceptor(m.StreamClientInterceptor()),
)
```

- Labels: `grpc_type` (`unary`, `client_stream`, `server_stream`, `bidi_stream`), `grpc_service` (`pkg.Service`), `grpc_method`, `grpc_code` (`OK`, `NotFound`, `Unavailable`, ...).
- A panic in a server handler is recorded as `Internal` and re-raised.
- A client stream is recorded when `RecvMsg` returns `io.EOF` or an error. For client-streaming RPCs it is recorded after the single response arrives. Streams the caller abandons without reading to the end are not recorded.
- To tag metrics with your service name, use `WithConstLabels(metrics.Labels{"service": "auth"})`.

### Downstream availability

```go
mon, err := m.NewGRPCConnectionMonitor(conn, "auth", "payments",
	metrics.WithPingInterval(5*time.Second),
	metrics.WithCheckTimeout(2*time.Second),
	metrics.WithHealthService(""), // "" = overall server health
)
if err != nil {
	return err
}
if err := mon.Start(ctx); err != nil { // first check is synchronous
	return err
}
defer mon.Close()

if !mon.IsAvailable() {
	// fall back, open a circuit breaker, ...
}
```

The monitor is safe for concurrent use. A second `Start` returns `ErrMonitorStarted`. `Close` is idempotent, waits for the background goroutine and deletes the series so no stale value stays exported.

### Custom metrics

```go
orders, err := m.NewCounterVec(metrics.CounterOpts{
	Name: "orders_total",
	Help: "Orders created.",
}, []string{"status"})
if err != nil {
	return err
}
orders.WithLabelValues("paid").Inc()
```

`NewCounter`, `NewCounterVec`, `NewGauge`, `NewGaugeVec`, `NewHistogram` and `NewHistogramVec` register in `m.Registerer()` and return an error on conflict. `CounterOpts`, `Labels` and similar names are aliases of the Prometheus types.

## Configuration reference

**`metrics.New` options**

| Option | Default | Description |
|---|---|---|
| `WithRegisterer(r)` | `prometheus.DefaultRegisterer` | Where collectors are registered |
| `WithGatherer(g)` | the registerer if it is a `Gatherer`, otherwise `DefaultGatherer` | Returned by `m.Gatherer()` |
| `WithNamespace(ns)` | none | Metric name prefix `ns_` |
| `WithConstLabels(l)` | none | Labels added to every series |
| `WithHTTPBuckets(b)` | `DefaultBuckets` (1ms … 10s) | HTTP histogram buckets |
| `WithGRPCBuckets(b)` | `DefaultBuckets` (1ms … 10s) | gRPC histogram buckets |
| `WithNativeHistograms(f, max)` | off | Native histograms, growth factor `f > 1`, at most `max` buckets |

**`metrics.NewConfig` options (server)**

| Option | Default |
|---|---|
| `WithHost` | `0.0.0.0` |
| `WithPort` | `8080` (1–65535) |
| `WithPath` | `/metrics` |
| `WithServerGatherer` | `prometheus.DefaultGatherer` |
| `WithReadTimeout` | `10s` |
| `WithWriteTimeout` | `10s` |
| `WithReadHeaderTimeout` | `5s` |
| `WithShutdownTimeout` | `10s` |

**`HTTPMiddleware` options**: `WithRouteFunc(fn)` (default reads `r.Pattern`) and `WithRoute(name)`.

**`NewGRPCConnectionMonitor` options**: `WithPingInterval` (default `5s`), `WithCheckTimeout` (default `2s`), `WithHealthService` (default `""`).

All errors are exported (`ErrInvalidPort`, `ErrServerStarted`, ...) and work with `errors.Is`.

## Exported metrics

| Metric | Type | Labels |
|---|---|---|
| `http_requests_total` | counter | `method`, `route`, `status_code` |
| `http_request_duration_seconds` | histogram | `method`, `route`, `status_code` |
| `grpc_server_handled_total` | counter | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_server_handling_seconds` | histogram | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_client_handled_total` | counter | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_client_handling_seconds` | histogram | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_connection_availability` | gauge (0/1) | `from_service`, `to_service` |

`WithNamespace` prefixes every name, and `WithConstLabels` adds its labels to every series.

## PromQL examples

```promql
# p99 latency per route
histogram_quantile(0.99, sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))

# HTTP 5xx error ratio
sum(rate(http_requests_total{status_code=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))

# gRPC server error ratio per method (excluding client-side errors)
sum by (grpc_method) (rate(grpc_server_handled_total{grpc_code!~"OK|NotFound|InvalidArgument|Canceled"}[5m]))
  / sum by (grpc_method) (rate(grpc_server_handled_total[5m]))

# downstream services that are currently unavailable
grpc_connection_availability == 0
```

## Guarantees

- **Bounded cardinality.** Every label value comes from a finite set: normalized methods, route templates, status codes and gRPC codes. Client input cannot create new series.
- **Transparent to the application.** Panics are re-raised unchanged. Response writer capabilities are preserved. Errors from handlers and RPCs are returned as they are.
- **Concurrency safety.** `Server`, `GRPCConnectionMonitor` and all middlewares are safe for concurrent use. CI runs the tests with `-race`.
- **Supply-chain checks.** CI runs `govulncheck` on every push.

## Performance

Measured with Go 1.27.1, `make bench`:

```
BenchmarkUnaryServerInterceptor   ~280 ns/op    0 B/op   0 allocs/op
BenchmarkHTTPMiddleware           ~380 ns/op   40 B/op   2 allocs/op   (includes ServeMux: ~110 ns, 1 alloc)
```

The HTTP middleware adds one allocation per request (the response writer wrapper). The gRPC interceptor adds none.

## Migrating from v1

v2 uses a new module path (`/v2`), so v1 users are not affected until they switch.

| v1 | v2 |
|---|---|
| `import ".../go-metrics"` | `import ".../go-metrics/v2"` |
| Go 1.25 | Go 1.27.1 |
| global metrics registered on import | `m, err := metrics.New(...)` |
| `SetRegisterer(r)` / `Registerer` variable | `metrics.WithRegisterer(r)` |
| `RequestDurationMetricHTTPMiddleware(h)` | `m.HTTPMiddleware()(h)` |
| `RequestDurationMetricUnaryServerInterceptor("svc")` | `m.UnaryServerInterceptor()` + `WithConstLabels(Labels{"service": "svc"})` |
| `NewGRPCConnectionMonitor(svc, d, from, to)` | `m.NewGRPCConnectionMonitor(conn, from, to, WithPingInterval(d))` |
| `monitor.Start(ctx)` | `err := monitor.Start(ctx)` |
| `metrics.NewCounter(opts)` | `m.NewCounter(opts)` (returns an error) |
| `Option` (config) | `ConfigOption` |
| `Run` returns `context.Canceled` on shutdown | returns `nil` |
| HTTP labels `method, status_code` | `method, route, status_code` |
| `grpc_requests_total` / `grpc_request_duration_seconds` with `is_error` | `grpc_server_handled_total` / `grpc_server_handling_seconds` with `grpc_code` |
| default buckets 10ms … 5s | 1ms … 10s |

Dashboards and alerts that use the v1 gRPC metric names or `is_error` need updating.

## Development

```bash
make test    # go test -race -cover
make lint    # golangci-lint (config in .golangci.yml)
make bench   # benchmarks
make vuln    # govulncheck
make tags    # tag and push the release from ./version
```

PRs and issues are welcome.

## License

[MIT](LICENSE) © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)
