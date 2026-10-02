package metrics

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	// RouteUnmatched is the route label for requests no route matched (e.g. 404s).
	RouteUnmatched = "unmatched"
	// MethodOther is the method label for non-standard HTTP methods.
	MethodOther = "OTHER"
)

// statusCodes caches label values for valid status codes to avoid an
// allocation per request.
var statusCodes = func() [600]string {
	var codes [600]string
	for i := 100; i < len(codes); i++ {
		codes[i] = strconv.Itoa(i)
	}
	return codes
}()

func statusLabel(code int) string {
	if code >= 100 && code < len(statusCodes) {
		return statusCodes[code]
	}
	return strconv.Itoa(code)
}

// methodLabel maps the client-controlled method to a bounded set of values,
// so arbitrary methods cannot blow up series cardinality.
func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return MethodOther
	}
}

// RouteFunc returns a low-cardinality route label for a request, such as a
// route template ("/users/{id}"). It must never return the raw URL path.
// It is called after the wrapped handler has run. Return "" for unmatched requests.
type RouteFunc func(r *http.Request) string

type httpOptions struct {
	routeFunc RouteFunc
}

// HTTPOption configures the HTTP middleware.
type HTTPOption func(*httpOptions)

// WithRouteFunc sets how the route label is derived. The default uses
// r.Pattern, which http.ServeMux (Go 1.23+) fills in, so it works when the
// middleware wraps a ServeMux. For other routers pass a function that reads
// the matched template, e.g. chi.RouteContext(r.Context()).RoutePattern().
func WithRouteFunc(fn RouteFunc) HTTPOption {
	return func(o *httpOptions) { o.routeFunc = fn }
}

// WithRoute sets a fixed route label, for wrapping individual handlers.
func WithRoute(route string) HTTPOption {
	return WithRouteFunc(func(*http.Request) string { return route })
}

func patternRoute(r *http.Request) string { return r.Pattern }

// HTTPMiddleware returns middleware that records http_requests_total and
// http_request_duration_seconds with labels method, route and status_code.
//
// The wrapped ResponseWriter supports http.Flusher, io.ReaderFrom,
// http.Hijacker (when the original does) and http.ResponseController. Panics are recorded as 500 and re-raised unchanged.
func (m *Metrics) HTTPMiddleware(opts ...HTTPOption) func(http.Handler) http.Handler {
	o := httpOptions{routeFunc: patternRoute}
	for _, opt := range opts {
		opt(&o)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww, sw := wrapWriter(w)

			defer func() {
				p := recover()
				code := sw.code
				if code == 0 {
					if p != nil {
						code = http.StatusInternalServerError
					} else {
						code = http.StatusOK
					}
				}

				route := o.routeFunc(r)
				if route == "" {
					route = RouteUnmatched
				}

				m.observeHTTP(methodLabel(r.Method), route, statusLabel(code), time.Since(start))

				if p != nil {
					panic(p)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func (m *Metrics) observeHTTP(method, route, code string, d time.Duration) {
	m.httpRequestsTotal.WithLabelValues(method, route, code).Inc()
	m.httpRequestDuration.WithLabelValues(method, route, code).Observe(d.Seconds())
}

// statusWriter captures the first final status code sent to the client.
// It implements http.Flusher and io.ReaderFrom and exposes Unwrap, so
// http.ResponseController reaches every capability of the original writer.
type statusWriter struct {
	http.ResponseWriter
	code int
}

// hijackWriter is used when the original writer supports http.Hijacker
// (HTTP/1.x), so that type assertions keep reporting the real capabilities.
type hijackWriter struct {
	statusWriter
}

func wrapWriter(w http.ResponseWriter) (http.ResponseWriter, *statusWriter) {
	if _, ok := w.(http.Hijacker); ok {
		hw := &hijackWriter{statusWriter{ResponseWriter: w}}
		return hw, &hw.statusWriter
	}
	sw := &statusWriter{ResponseWriter: w}
	return sw, sw
}

func (w *statusWriter) markWritten() {
	if w.code == 0 {
		w.code = http.StatusOK
	}
}

func (w *statusWriter) WriteHeader(code int) {
	// 1xx are informational (except 101 Switching Protocols, which is final).
	if w.code == 0 && (code >= 200 || code == http.StatusSwitchingProtocols) {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.markWritten()
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) WriteString(s string) (int, error) {
	w.markWritten()
	return io.WriteString(w.ResponseWriter, s)
}

func (w *statusWriter) ReadFrom(src io.Reader) (int64, error) {
	w.markWritten()
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(writerOnly{w.ResponseWriter}, src)
}

func (w *statusWriter) Flush() {
	_ = w.FlushError()
}

// FlushError is used by http.ResponseController; it reports
// http.ErrNotSupported if the original writer cannot flush.
func (w *statusWriter) FlushError() error {
	w.markWritten()
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the original writer
// (SetReadDeadline, SetWriteDeadline, EnableFullDuplex, ...).
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *hijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && w.code == 0 {
		w.code = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

// writerOnly hides io.ReaderFrom to avoid infinite recursion in io.Copy.
type writerOnly struct {
	io.Writer
}
