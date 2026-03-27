package metrics

import (
	"net/http"
	"strconv"
	"time"
)

var (
	httpRequestsTotal = NewCounterVec(
		CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "status_code"},
	)

	httpRequestDuration = NewHistogramVec(
		HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
		},
		[]string{"method", "status_code"},
	)
)

// measureHTTPRequest records both count and duration.
func measureHTTPRequest(method, statusCode string, start time.Time) {
	labels := []string{method, statusCode}
	httpRequestsTotal.WithLabelValues(labels...).Inc()
	httpRequestDuration.WithLabelValues(labels...).Observe(time.Since(start).Seconds())
}

// RequestDurationMetricHTTPMiddleware tracks HTTP request duration and count.
func RequestDurationMetricHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if rec := recover(); rec != nil {
				rw.status = http.StatusInternalServerError
			}
			measureHTTPRequest(r.Method, strconv.Itoa(rw.status), start)
		}()

		next.ServeHTTP(rw, r)
	})
}

// responseWriter captures the status code (works even if WriteHeader is never called).
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}
