package metrics

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name string
		opts []ConfigOption
		want error
	}{
		{"defaults", nil, nil},
		{"empty host", []ConfigOption{WithHost("")}, ErrEmptyHost},
		{"zero port", []ConfigOption{WithPort(0)}, ErrZeroPort},
		{"port too big", []ConfigOption{WithPort(70000)}, ErrInvalidPort},
		{"negative port", []ConfigOption{WithPort(-1)}, ErrInvalidPort},
		{"negative timeout", []ConfigOption{WithReadTimeout(-time.Second)}, ErrInvalidTimeout},
		{"bad path", []ConfigOption{WithPath("metrics")}, ErrInvalidPath},
		{"nil gatherer", []ConfigOption{WithServerGatherer(nil)}, ErrNilGatherer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewConfig(tt.opts...).Validate()
			if !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
			if _, err := NewServer(NewConfig(tt.opts...)); !errors.Is(err, tt.want) {
				t.Errorf("NewServer: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestConfig_Address(t *testing.T) {
	if got := NewConfig(WithHost("::1"), WithPort(9090)).Address(); got != "[::1]:9090" {
		t.Errorf("got %q", got)
	}
}

func TestServer_ServesCustomGatherer(t *testing.T) {
	m, _ := newTestMetrics(t)
	m.observeHTTP("GET", "/x", "200", time.Millisecond)

	srv, err := NewServer(NewConfig(WithServerGatherer(m.Gatherer()), WithPath("/m")))
	if err != nil {
		t.Fatal(err)
	}
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/m")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "http_requests_total") {
		t.Errorf("status %d, body missing metric:\n%s", resp.StatusCode, body)
	}

	if err := srv.Serve(ctx, listen(t)); !errors.Is(err, ErrServerStarted) {
		t.Errorf("second Serve: got %v", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("graceful shutdown returned %v", err)
	}
}

func TestServer_CloseWhileRunning(t *testing.T) {
	srv, err := NewServer(NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), listen(t)) }()

	// Close concurrently with Serve; must be race-free whichever runs first.
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil && !errors.Is(err, ErrServerClosed) {
		t.Errorf("got %v", err)
	}
	if err := srv.Serve(context.Background(), listen(t)); !errors.Is(err, ErrServerClosed) {
		t.Errorf("Serve after Close: got %v", err)
	}
}

func TestServer_RunListenError(t *testing.T) {
	ln := listen(t)
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	srv, err := NewServer(NewConfig(WithHost("127.0.0.1"), WithPort(port)))
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("expected listen error for busy port")
	}
}
