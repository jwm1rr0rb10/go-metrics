# Metrics

**Лёгкая и удобная библиотека для Prometheus-метрик в Go-сервисах**

Библиотека предоставляет готовые инструменты для сбора и экспорта метрик:
- HTTP-сервер для `/metrics`
- Middleware для HTTP-запросов
- Unary-интерцептор для gRPC
- Мониторинг доступности gRPC-соединений через health checks
- Удобные обёртки над `prometheus/client_golang`

## Возможности

- Полностью настраиваемый HTTP-сервер метрик (хост, порт, таймауты)
- Автоматический сбор длительности HTTP-запросов (`http_request_duration_seconds`)
- Автоматический сбор длительности gRPC-запросов (`grpc_request_duration_seconds`)
- Gauge-метрика доступности gRPC-соединений (`grpc_connection_availability`)
- Хелперы для создания Counter / Gauge / Histogram / Vec-метрик
- Валидация конфигурации и разумные значения по умолчанию
- Поддержка кастомного `prometheus.Registerer`

## Установка

```bash
go get github.com/jwm1rr0rb10/go-metrics  # замените на путь к вашему репозиторию
```

---

Библиотека зависит от:

- `github.com/prometheus/client_golang`
- `github.com/julienschmidt/httprouter`
- `google.golang.org/grpc + google.golang.org/grpc/health/grpc_health_v1`

После добавления модуля выполните `go mod tidy`.

---

## Быстрый старт
## 1. Запуск сервера метрик

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

	log.Printf("Metrics available at http://%s/metrics", cfg.Address()) // или вручную

	// ... ваш основной код ...

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

// Оборачиваем middleware
handler := metrics.RequestDurationMetricHTTPMiddleware(mux)
http.ListenAndServe(":8080", handler)
```

Метрика: `http_request_duration_seconds` (histogram) с лейблами `method`, `status_code`.

---

## 3. gRPC Interceptor

```go
grpcServer := grpc.NewServer(
	grpc.UnaryInterceptor(metrics.RequestDurationMetricUnaryServerInterceptor("auth_service")),
)
```
Метрика: `grpc_request_duration_seconds` (histogram) с лейблами `service`, `method`, `is_error`

---

## 4. Мониторинг gRPC-соединения

```go
type myClient struct {
	conn grpc.ClientConnInterface
}

func (c *myClient) Connection() grpc.ClientConnInterface { return c.conn }

monitor := metrics.NewGRPCConnectionMonitor(
	&myClient{conn: externalConn},
	5*time.Second,      // интервал пинга
	"my_service",       // from_service
	"payment_service",  // to_service
)

monitor.Start(ctx)

// При завершении работы
defer monitor.Close()
```

Метрика: `grpc_connection_availability` (gauge 0/1) с лейблами `from_service`, `to_service`.

---

### Конфигурация

```go
cfg := metrics.NewConfig(
	metrics.WithHost("127.0.0.1"),
	metrics.WithPort(9090),
	metrics.WithReadTimeout(30 * time.Second),
	metrics.WithWriteTimeout(30 * time.Second),
	metrics.WithReadHeaderTimeout(5 * time.Second),
)
```
Значения по умолчанию:

- host: 0.0.0.0
- port: 8080
- read timeout: 10s
- write timeout: 10s
- read header timeout: 5s

---

### Валидация
`cfg.Validate()` (вызывается автоматически в `NewServer`).

---

Кастомные метрики

```go
counter := metrics.NewCounter(prometheus.CounterOpts{
	Name: "my_custom_counter",
	Help: "Количество чего-то",
})

gaugeVec := metrics.NewGaugeVec(prometheus.GaugeOpts{
	Name: "my_gauge",
	Help: "Значение чего-то",
}, []string{"label1", "label2"})

// И т.д. для Histogram, CounterVec и т.п.
```

Можно сменить регистр:

```go
metrics.SetRegisterer(myCustomRegisterer)
```

---

## Экспортируемые метрики
| Метрика                       | Тип       | Лейблы                      | Описание                        |
|:------------------------------|:----------|:----------------------------|:--------------------------------|
| http_request_duration_seconds | Histogram | "method, status_code"       | Длительность HTTP-запросов      |
| grpc_request_duration_seconds | Histogram | "service, method, is_error" | Длительность gRPC-запросов      |
| grpc_connection_availability  | Gauge     | "from_service, to_service"  | Доступность gRPC-соединения (1/0)|

---

### Тестирование
В репозитории есть `metrics_test.go` с тестами на валидацию конфига и жизненный цикл сервера.

---

### Пример полного использования
Смотрите тесты и файлы `config.go`, `handler.go`, `http_middleware.go`, `grpc_middleware.go`, `metrics_grpc_availability.go`.

---

Лицензия

[MIT License](https://github.com/jwm1rr0rb10/go-metrics/blob/main/LICENSE) – © Raman Zaitsau [@jwm1rrr0rb10](https://github.com/jwm1rr0rb10)


🙌 Contributions

PRs and suggestions welcome! Feel free to fork, open issues, or create a pull request 🚀