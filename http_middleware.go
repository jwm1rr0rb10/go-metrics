package metrics

import (
	"net/http"
	"strconv"
	"time"
)

var httpRequestDuration = NewHistogramVec(
	HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1},
	},
	[]string{"method", "status_code"},
)

// measureHTTPRequestDuration records the duration of HTTP requests.
func measureHTTPRequestDuration(method, statusCode string, start time.Time) {
	httpRequestDuration.
		WithLabelValues(method, statusCode).
		Observe(time.Since(start).Seconds())
}

// RequestDurationMetricHTTPMiddleware tracks HTTP request duration.
func RequestDurationMetricHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{w, http.StatusOK}
		defer func() {
			measureHTTPRequestDuration(r.Method, strconv.Itoa(rw.status), start)
		}()
		next.ServeHTTP(rw, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
