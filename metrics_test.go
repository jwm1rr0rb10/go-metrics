package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestMetrics(t testing.TB, opts ...Option) (*Metrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m, err := New(append([]Option{WithRegisterer(reg)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return m, reg
}

// waitFor polls cond until it returns true or the deadline expires.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNew_DuplicateRegistrationReturnsError(t *testing.T) {
	m, reg := newTestMetrics(t)

	if _, err := New(WithRegisterer(reg)); err == nil {
		t.Fatal("expected error on duplicate registration")
	}

	// A failed New must not leave partial registrations behind.
	m.Unregister()
	if _, err := New(WithRegisterer(reg)); err != nil {
		t.Fatalf("New after Unregister: %v", err)
	}
}

func TestNew_IndependentRegistries(t *testing.T) {
	newTestMetrics(t)
	newTestMetrics(t)
}

func TestNew_Validation(t *testing.T) {
	if _, err := New(WithRegisterer(nil)); !errors.Is(err, ErrNilRegisterer) {
		t.Errorf("nil registerer: got %v", err)
	}
	reg := prometheus.NewRegistry()
	if _, err := New(WithRegisterer(reg), WithNativeHistograms(1, 160)); !errors.Is(err, ErrInvalidNativeFactor) {
		t.Errorf("native factor 1: got %v", err)
	}
}

func TestNew_GathererFromRegistry(t *testing.T) {
	m, reg := newTestMetrics(t)
	if m.Gatherer() != prometheus.Gatherer(reg) {
		t.Error("expected registry to be used as gatherer")
	}
}

func TestNew_NamespaceAndConstLabels(t *testing.T) {
	m, reg := newTestMetrics(t, WithNamespace("app"), WithConstLabels(Labels{"service": "auth"}))
	m.observeHTTP("GET", "/x", "200", time.Millisecond)

	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range families {
		if f.GetName() != "app_http_requests_total" {
			continue
		}
		found = true
		var hasService bool
		for _, l := range f.GetMetric()[0].GetLabel() {
			if l.GetName() == "service" && l.GetValue() == "auth" {
				hasService = true
			}
		}
		if !hasService {
			t.Error("const label service=auth missing")
		}
	}
	if !found {
		t.Error("app_http_requests_total not found")
	}
}

func TestNew_NativeHistograms(t *testing.T) {
	m, _ := newTestMetrics(t, WithNativeHistograms(1.1, 160))
	m.observeHTTP("GET", "/x", "200", time.Millisecond)
	if n := testutil.CollectAndCount(m.httpRequestDuration); n != 1 {
		t.Errorf("expected 1 series, got %d", n)
	}
}

func TestMetrics_CustomCollectors(t *testing.T) {
	m, reg := newTestMetrics(t)

	c, err := m.NewCounter(CounterOpts{Name: "custom_total", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}
	c.Inc()
	if _, err := m.NewCounter(CounterOpts{Name: "custom_total", Help: "h"}); err == nil {
		t.Error("expected duplicate registration error")
	}
	if n, err := testutil.GatherAndCount(reg, "custom_total"); err != nil || n != 1 {
		t.Errorf("custom_total: n=%d err=%v", n, err)
	}
}
