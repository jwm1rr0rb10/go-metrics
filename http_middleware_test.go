package metrics

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func httpCount(m *Metrics, method, route, code string) float64 {
	return testutil.ToFloat64(m.httpRequestsTotal.WithLabelValues(method, route, code))
}

func TestHTTPMiddleware_RouteFromServeMuxPattern(t *testing.T) {
	m, _ := newTestMetrics(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	h := m.HTTPMiddleware()(mux)

	for _, path := range []string{"/users/1", "/users/2", "/nope"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	if got := httpCount(m, "GET", "GET /users/{id}", "200"); got != 2 {
		t.Errorf("route pattern: got %v, want 2", got)
	}
	if got := httpCount(m, "GET", RouteUnmatched, "404"); got != 1 {
		t.Errorf("unmatched: got %v, want 1", got)
	}
	if n := testutil.CollectAndCount(m.httpRequestsTotal); n != 2 {
		t.Errorf("raw paths must not create series: got %d series", n)
	}
}

func TestHTTPMiddleware_WithRoute(t *testing.T) {
	m, _ := newTestMetrics(t)
	h := m.HTTPMiddleware(WithRoute("/static"))(http.NotFoundHandler())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/static/a.css", nil))

	if got := httpCount(m, "GET", "/static", "404"); got != 1 {
		t.Errorf("got %v, want 1", got)
	}
}

func TestHTTPMiddleware_UnknownMethodIsBounded(t *testing.T) {
	m, _ := newTestMetrics(t)
	h := m.HTTPMiddleware(WithRoute("/"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	for _, method := range []string{"RANDOM1", "RANDOM2", "get"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/", nil))
	}

	if got := httpCount(m, MethodOther, "/", "200"); got != 3 {
		t.Errorf("got %v, want 3", got)
	}
}

func TestHTTPMiddleware_StatusCode(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"implicit 200", func(http.ResponseWriter, *http.Request) {}, "200"},
		{"write without header", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }, "200"},
		{"first header wins", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusInternalServerError)
		}, "201"},
		{"informational ignored", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusNoContent)
		}, "204"},
		{"panic after header", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			panic("boom")
		}, "202"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestMetrics(t)
			h := m.HTTPMiddleware(WithRoute("/"))(tt.handler)
			func() {
				defer func() { _ = recover() }()
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			}()
			if got := httpCount(m, "GET", "/", tt.want); got != 1 {
				t.Errorf("status %s: got %v, want 1", tt.want, got)
			}
		})
	}
}

func TestHTTPMiddleware_PanicIsRecordedAndReraised(t *testing.T) {
	m, _ := newTestMetrics(t)
	h := m.HTTPMiddleware(WithRoute("/"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	if recovered != http.ErrAbortHandler { //nolint:errorlint // panic value identity is what is being tested
		t.Fatalf("panic value must be re-raised unchanged, got %v", recovered)
	}
	if got := httpCount(m, "GET", "/", "500"); got != 1 {
		t.Errorf("got %v, want 1", got)
	}
}

func TestHTTPMiddleware_PreservesOptionalInterfaces(t *testing.T) {
	m, _ := newTestMetrics(t)

	t.Run("flusher", func(t *testing.T) {
		h := m.HTTPMiddleware(WithRoute("/sse"))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, ok := w.(http.Flusher); !ok {
				t.Error("wrapped writer does not implement http.Flusher")
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("ResponseController.Flush: %v", err)
			}
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/sse", nil))
		if got := httpCount(m, "GET", "/sse", "200"); got != 1 {
			t.Errorf("got %v, want 1", got)
		}
	})

	t.Run("hijacker", func(t *testing.T) {
		srv := httptest.NewServer(m.HTTPMiddleware(WithRoute("/ws"))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("wrapped writer does not implement http.Hijacker")
				return
			}
			conn, rw, err := hj.Hijack()
			if err != nil {
				t.Errorf("Hijack: %v", err)
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n\r\n")
			_ = rw.Flush()
		})))
		defer srv.Close()

		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = bufio.NewReader(resp.Body).ReadString('\n')
		resp.Body.Close()

		waitFor(t, "hijacked request metric", func() bool {
			return httpCount(m, "GET", "/ws", "101") == 1
		})
	})
}

func BenchmarkHTTPMiddleware(b *testing.B) {
	m, _ := newTestMetrics(b)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := m.HTTPMiddleware()(mux)
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.ServeHTTP(w, req)
	}
}

func TestHTTPMiddleware_HijackerOnlyWhenSupported(t *testing.T) {
	m, _ := newTestMetrics(t)
	h := m.HTTPMiddleware(WithRoute("/"))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// httptest.ResponseRecorder cannot hijack, so neither may the wrapper.
		if _, ok := w.(http.Hijacker); ok {
			t.Error("wrapper must not claim http.Hijacker support")
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
