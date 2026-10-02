# go-metrics

[![CI](https://github.com/jwm1rr0rb10/go-metrics/actions/workflows/ci.yml/badge.svg)](https://github.com/jwm1rr0rb10/go-metrics/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jwm1rr0rb10/go-metrics/v2.svg)](https://pkg.go.dev/github.com/jwm1rr0rb10/go-metrics/v2)
![Go](https://img.shields.io/badge/go-1.27.1%2B-00ADD8?logo=go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English version](README.md)

Prometheus-инструментация для Go-сервисов под высокой нагрузкой. Покрывает HTTP, gRPC (сервер и клиент, unary и стримы), проверку доступности downstream-сервисов и эндпоинт `/metrics`. Даёт минимальный оверхед, держит кардинальность лейблов ограниченной и не использует глобальное состояние.

## Содержание

- [Возможности](#возможности)
- [Требования](#требования)
- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [Использование](#использование)
  - [Экземпляр Metrics](#экземпляр-metrics)
  - [Сервер /metrics](#сервер-metrics)
  - [HTTP-middleware](#http-middleware)
  - [gRPC-перехватчики](#grpc-перехватчики)
  - [Доступность downstream-сервисов](#доступность-downstream-сервисов)
  - [Свои метрики](#свои-метрики)
- [Справочник опций](#справочник-опций)
- [Экспортируемые метрики](#экспортируемые-метрики)
- [Примеры PromQL](#примеры-promql)
- [Гарантии](#гарантии)
- [Производительность](#производительность)
- [Миграция с v1](#миграция-с-v1)
- [Разработка](#разработка)
- [Лицензия](#лицензия)

## Возможности

- **HTTP-middleware**: лейблы `method`, `route` и `status_code`. Паника записывается и пробрасывается дальше. SSE и WebSocket продолжают работать.
- **gRPC-перехватчики**: сервер и клиент, unary и стримы, с настоящим `grpc_code`.
- **Монитор доступности** downstream gRPC-сервисов на стандартном протоколе `grpc.health.v1`.
- **Сервер `/metrics`** с таймаутами, graceful shutdown и поддержкой кастомного регистри.
- **Настраиваемые гистограммы**: бакеты и опциональные native (sparse) histograms.
- **Namespace и константные лейблы** для всех метрик.
- **Никаких побочных эффектов при импорте**: коллекторы создаются явно. Конфликт регистрации возвращает ошибку, а не паникует.

## Требования

| Компонент | Версия |
|---|---|
| Go | **1.27.1+** |
| `github.com/prometheus/client_golang` | 1.24+ |
| `google.golang.org/grpc` | 1.84+ |
| Prometheus (только для native histograms) | версия с включённым сбором native histograms |

Лейбл `route` для `http.ServeMux` берётся из `Request.Pattern`. Это поле есть во всех поддерживаемых версиях Go.

## Установка

```bash
go get github.com/jwm1rr0rb10/go-metrics/v2
```

## Быстрый старт

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

Полный пример, который компилируется вместе с тестами, лежит в [`example_test.go`](example_test.go).

## Использование

### Экземпляр Metrics

`metrics.New` создаёт и регистрирует все коллекторы пакета. Создавайте **один** экземпляр на регистри и переиспользуйте его во всём процессе.

```go
m, err := metrics.New(
	metrics.WithRegisterer(reg),                     // по умолчанию prometheus.DefaultRegisterer
	metrics.WithNamespace("myapp"),                  // myapp_http_requests_total, ...
	metrics.WithConstLabels(metrics.Labels{"env": "prod"}),
	metrics.WithHTTPBuckets([]float64{.005, .01, .05, .1, .5, 1}),
	metrics.WithNativeHistograms(1.1, 160),
)
```

- Если регистрация конфликтует с уже существующим коллектором, `New` возвращает ошибку и не оставляет ничего зарегистрированным наполовину. `MustNew` вместо этого паникует, это удобно в `main`.
- `m.Gatherer()` возвращает регистри, если он одновременно `prometheus.Gatherer`. Для обёрнутого registerer передайте gatherer явно через `WithGatherer`.
- `m.Unregister()` снимает все коллекторы с регистрации, это пригодится в тестах.

### Сервер `/metrics`

```go
srv, err := metrics.NewServer(metrics.NewConfig(
	metrics.WithHost("0.0.0.0"),
	metrics.WithPort(9090),
	metrics.WithPath("/metrics"),
	metrics.WithServerGatherer(m.Gatherer()), // обязательно при кастомном регистри
))
if err != nil {
	return err // ошибка валидации: порт, хост, путь, таймауты
}

err = srv.Run(ctx)            // слушает и обслуживает; nil после graceful shutdown по отмене ctx
err = srv.Serve(ctx, ln)      // то же со своим net.Listener (тесты, socket activation)
err = srv.Close()             // немедленная остановка, безопасна из любой горутины
```

`Run` сразу возвращает ошибку прослушивания, например если порт занят. Повторный `Run` или `Serve` возвращает `ErrServerStarted`, а после `Close` — `ErrServerClosed`. Эндпоинт поддерживает формат OpenMetrics.

### HTTP-middleware

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser)

handler := m.HTTPMiddleware()(mux) // route="GET /users/{id}"
```

У middleware стандартная сигнатура `func(http.Handler) http.Handler`, поэтому она встраивается в любую цепочку.

**Лейбл route.** По умолчанию берётся из `r.Pattern`, который заполняет `http.ServeMux`. Запросы без совпавшего роута получают `route="unmatched"`. Сырой путь URL не используется никогда. Для других роутеров:

```go
// chi
m.HTTPMiddleware(metrics.WithRouteFunc(func(r *http.Request) string {
	return chi.RouteContext(r.Context()).RoutePattern()
}))

// фиксированный лейбл для отдельного хендлера
m.HTTPMiddleware(metrics.WithRoute("/static"))
```

**Поведение:**
- Нестандартные HTTP-методы записываются как `OTHER`.
- Статус — первый финальный код ответа. Информационные коды 1xx не учитываются, кроме `101`. Если `WriteHeader` не вызывался, записывается `200`.
- Паника записывается как `500`, если заголовок ещё не отправлен, и пробрасывается дальше без изменений: ваш recovery-middleware её увидит.
- Обёрнутый writer поддерживает `http.Flusher`, `io.ReaderFrom`, `http.Hijacker` (только если его поддерживает исходный writer) и `http.ResponseController`.

### gRPC-перехватчики

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

- Лейблы: `grpc_type` (`unary`, `client_stream`, `server_stream`, `bidi_stream`), `grpc_service` (`pkg.Service`), `grpc_method`, `grpc_code` (`OK`, `NotFound`, `Unavailable`, ...).
- Паника в серверном хендлере записывается как `Internal` и пробрасывается дальше.
- Клиентский стрим записывается, когда `RecvMsg` возвращает `io.EOF` или ошибку. Для client-streaming RPC — после получения единственного ответа. Стримы, которые вызывающий код бросил, не дочитав до конца, не записываются.
- Чтобы пометить метрики именем своего сервиса, используйте `WithConstLabels(metrics.Labels{"service": "auth"})`.

### Доступность downstream-сервисов

```go
mon, err := m.NewGRPCConnectionMonitor(conn, "auth", "payments",
	metrics.WithPingInterval(5*time.Second),
	metrics.WithCheckTimeout(2*time.Second),
	metrics.WithHealthService(""), // "" = общее здоровье сервера
)
if err != nil {
	return err
}
if err := mon.Start(ctx); err != nil { // первая проверка синхронная
	return err
}
defer mon.Close()

if !mon.IsAvailable() {
	// fallback, circuit breaker, ...
}
```

Монитор безопасен для конкурентного использования. Повторный `Start` возвращает `ErrMonitorStarted`. `Close` можно вызывать многократно: он дожидается остановки фоновой горутины и удаляет серию, чтобы не экспортировать устаревшее значение.

### Свои метрики

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

`NewCounter`, `NewCounterVec`, `NewGauge`, `NewGaugeVec`, `NewHistogram` и `NewHistogramVec` регистрируют метрику в `m.Registerer()` и возвращают ошибку при конфликте. `CounterOpts`, `Labels` и подобные имена — алиасы типов Prometheus.

## Справочник опций

**Опции `metrics.New`**

| Опция | По умолчанию | Описание |
|---|---|---|
| `WithRegisterer(r)` | `prometheus.DefaultRegisterer` | Куда регистрируются коллекторы |
| `WithGatherer(g)` | registerer, если он `Gatherer`, иначе `DefaultGatherer` | Что возвращает `m.Gatherer()` |
| `WithNamespace(ns)` | нет | Префикс имён `ns_` |
| `WithConstLabels(l)` | нет | Лейблы, добавляемые к каждой серии |
| `WithHTTPBuckets(b)` | `DefaultBuckets` (1ms … 10s) | Бакеты HTTP-гистограммы |
| `WithGRPCBuckets(b)` | `DefaultBuckets` (1ms … 10s) | Бакеты gRPC-гистограмм |
| `WithNativeHistograms(f, max)` | выключено | Native histograms с фактором роста `f > 1`, не больше `max` бакетов |

**Опции `metrics.NewConfig` (сервер)**

| Опция | По умолчанию |
|---|---|
| `WithHost` | `0.0.0.0` |
| `WithPort` | `8080` (1–65535) |
| `WithPath` | `/metrics` |
| `WithServerGatherer` | `prometheus.DefaultGatherer` |
| `WithReadTimeout` | `10s` |
| `WithWriteTimeout` | `10s` |
| `WithReadHeaderTimeout` | `5s` |
| `WithShutdownTimeout` | `10s` |

**Опции `HTTPMiddleware`**: `WithRouteFunc(fn)` (по умолчанию читает `r.Pattern`) и `WithRoute(name)`.

**Опции `NewGRPCConnectionMonitor`**: `WithPingInterval` (по умолчанию `5s`), `WithCheckTimeout` (по умолчанию `2s`), `WithHealthService` (по умолчанию `""`).

Все ошибки экспортируются (`ErrInvalidPort`, `ErrServerStarted`, ...) и проверяются через `errors.Is`.

## Экспортируемые метрики

| Метрика | Тип | Лейблы |
|---|---|---|
| `http_requests_total` | counter | `method`, `route`, `status_code` |
| `http_request_duration_seconds` | histogram | `method`, `route`, `status_code` |
| `grpc_server_handled_total` | counter | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_server_handling_seconds` | histogram | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_client_handled_total` | counter | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_client_handling_seconds` | histogram | `grpc_type`, `grpc_service`, `grpc_method`, `grpc_code` |
| `grpc_connection_availability` | gauge (0/1) | `from_service`, `to_service` |

`WithNamespace` добавляет префикс ко всем именам, а `WithConstLabels` — свои лейблы к каждой серии.

## Примеры PromQL

```promql
# p99 задержки по роутам
histogram_quantile(0.99, sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))

# доля HTTP-ответов 5xx
sum(rate(http_requests_total{status_code=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))

# доля серверных ошибок gRPC по методам (без клиентских ошибок)
sum by (grpc_method) (rate(grpc_server_handled_total{grpc_code!~"OK|NotFound|InvalidArgument|Canceled"}[5m]))
  / sum by (grpc_method) (rate(grpc_server_handled_total[5m]))

# downstream-сервисы, недоступные прямо сейчас
grpc_connection_availability == 0
```

## Гарантии

- **Ограниченная кардинальность.** Каждый лейбл берёт значения из конечного набора: нормализованные методы, шаблоны роутов, коды статусов и коды gRPC. Ввод клиента не создаёт новых серий.
- **Прозрачность для приложения.** Паники пробрасываются без изменений. Возможности response writer сохраняются. Ошибки хендлеров и RPC возвращаются как есть.
- **Потокобезопасность.** `Server`, `GRPCConnectionMonitor` и все middleware безопасны для конкурентного использования. В CI тесты запускаются с `-race`.
- **Проверка зависимостей.** CI запускает `govulncheck` на каждый push.

## Производительность

Замеры на Go 1.27.1, `make bench`:

```
BenchmarkUnaryServerInterceptor   ~280 ns/op    0 B/op   0 allocs/op
BenchmarkHTTPMiddleware           ~380 ns/op   40 B/op   2 allocs/op   (включая ServeMux: ~110 ns, 1 alloc)
```

HTTP-middleware добавляет одну аллокацию на запрос (обёртка response writer). gRPC-перехватчик не добавляет ни одной.

## Миграция с v1

У v2 новый путь модуля (`/v2`), поэтому пользователей v1 изменения не затронут, пока они сами не перейдут.

| v1 | v2 |
|---|---|
| `import ".../go-metrics"` | `import ".../go-metrics/v2"` |
| Go 1.25 | Go 1.27.1 |
| глобальные метрики регистрируются при импорте | `m, err := metrics.New(...)` |
| `SetRegisterer(r)` / переменная `Registerer` | `metrics.WithRegisterer(r)` |
| `RequestDurationMetricHTTPMiddleware(h)` | `m.HTTPMiddleware()(h)` |
| `RequestDurationMetricUnaryServerInterceptor("svc")` | `m.UnaryServerInterceptor()` + `WithConstLabels(Labels{"service": "svc"})` |
| `NewGRPCConnectionMonitor(svc, d, from, to)` | `m.NewGRPCConnectionMonitor(conn, from, to, WithPingInterval(d))` |
| `monitor.Start(ctx)` | `err := monitor.Start(ctx)` |
| `metrics.NewCounter(opts)` | `m.NewCounter(opts)` (возвращает ошибку) |
| `Option` (конфиг) | `ConfigOption` |
| `Run` возвращает `context.Canceled` при остановке | возвращает `nil` |
| HTTP-лейблы `method, status_code` | `method, route, status_code` |
| `grpc_requests_total` / `grpc_request_duration_seconds` с `is_error` | `grpc_server_handled_total` / `grpc_server_handling_seconds` с `grpc_code` |
| бакеты по умолчанию 10ms … 5s | 1ms … 10s |

Дашборды и алерты, которые используют имена gRPC-метрик из v1 или лейбл `is_error`, нужно обновить.

## Разработка

```bash
make test    # go test -race -cover
make lint    # golangci-lint (конфиг в .golangci.yml)
make bench   # бенчмарки
make vuln    # govulncheck
make tags    # тег и push релиза из ./version
```

PR и issues приветствуются.

## Лицензия

[MIT](LICENSE) © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)
