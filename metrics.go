package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DefaultBuckets are latency buckets (in seconds) tuned for low-latency services:
// they start at 1ms so that p50/p90 of fast endpoints are still visible.
var DefaultBuckets = []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// Metrics owns every collector exposed by this package. Create it once per
// process (or per registry) with New and share it between middlewares,
// interceptors and connection monitors. Nothing is registered at import time.
type Metrics struct {
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer

	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec

	grpcServerHandled  *prometheus.CounterVec
	grpcServerDuration *prometheus.HistogramVec
	grpcClientHandled  *prometheus.CounterVec
	grpcClientDuration *prometheus.HistogramVec

	grpcConnAvailability *prometheus.GaugeVec
}

type options struct {
	registerer     prometheus.Registerer
	gatherer       prometheus.Gatherer
	namespace      string
	constLabels    prometheus.Labels
	httpBuckets    []float64
	grpcBuckets    []float64
	nativeFactor   float64
	nativeMaxCount uint32
}

// Option configures Metrics.
type Option func(*options)

// WithRegisterer sets the registerer used for all collectors
// (default: prometheus.DefaultRegisterer). If it also implements
// prometheus.Gatherer (e.g. *prometheus.Registry) it is used as the gatherer too.
func WithRegisterer(r prometheus.Registerer) Option {
	return func(o *options) { o.registerer = r }
}

// WithGatherer sets the gatherer returned by Metrics.Gatherer. Use it when the
// registerer is wrapped (e.g. prometheus.WrapRegistererWith) and is not a Gatherer itself.
func WithGatherer(g prometheus.Gatherer) Option {
	return func(o *options) { o.gatherer = g }
}

// WithNamespace prefixes every metric name with "<namespace>_".
func WithNamespace(namespace string) Option {
	return func(o *options) { o.namespace = namespace }
}

// WithConstLabels adds constant labels (e.g. {"service": "auth"}) to every metric.
func WithConstLabels(labels prometheus.Labels) Option {
	return func(o *options) { o.constLabels = labels }
}

// WithHTTPBuckets sets the HTTP latency histogram buckets (default: DefaultBuckets).
func WithHTTPBuckets(buckets []float64) Option {
	return func(o *options) { o.httpBuckets = buckets }
}

// WithGRPCBuckets sets the gRPC latency histogram buckets (default: DefaultBuckets).
func WithGRPCBuckets(buckets []float64) Option {
	return func(o *options) { o.grpcBuckets = buckets }
}

// WithNativeHistograms enables Prometheus native (sparse) histograms in addition
// to the classic buckets. factor is the growth factor between buckets (1.1 is a
// good default), maxBuckets caps memory per series (e.g. 160).
// Requires Prometheus scraping with native histograms enabled.
func WithNativeHistograms(factor float64, maxBuckets uint32) Option {
	return func(o *options) {
		o.nativeFactor = factor
		o.nativeMaxCount = maxBuckets
	}
}

// New creates and registers all collectors. It returns an error instead of
// panicking if a collector conflicts with an already registered one.
func New(opts ...Option) (*Metrics, error) {
	o := options{
		registerer:  prometheus.DefaultRegisterer,
		httpBuckets: DefaultBuckets,
		grpcBuckets: DefaultBuckets,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.registerer == nil {
		return nil, ErrNilRegisterer
	}
	if o.nativeFactor != 0 && o.nativeFactor <= 1 {
		return nil, ErrInvalidNativeFactor
	}
	if o.gatherer == nil {
		if g, ok := o.registerer.(prometheus.Gatherer); ok {
			o.gatherer = g
		} else {
			o.gatherer = prometheus.DefaultGatherer
		}
	}

	histogram := func(name, help string, buckets []float64, labels []string) *prometheus.HistogramVec {
		return prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       o.namespace,
			Name:                            name,
			Help:                            help,
			ConstLabels:                     o.constLabels,
			Buckets:                         buckets,
			NativeHistogramBucketFactor:     o.nativeFactor,
			NativeHistogramMaxBucketNumber:  o.nativeMaxCount,
			NativeHistogramMinResetDuration: time.Hour,
		}, labels)
	}
	counter := func(name, help string, labels []string) *prometheus.CounterVec {
		return prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   o.namespace,
			Name:        name,
			Help:        help,
			ConstLabels: o.constLabels,
		}, labels)
	}

	httpLabels := []string{"method", "route", "status_code"}
	grpcLabels := []string{"grpc_type", "grpc_service", "grpc_method", "grpc_code"}

	m := &Metrics{
		registerer: o.registerer,
		gatherer:   o.gatherer,

		httpRequestsTotal: counter("http_requests_total",
			"Total number of HTTP requests handled by the server.", httpLabels),
		httpRequestDuration: histogram("http_request_duration_seconds",
			"HTTP request duration in seconds.", o.httpBuckets, httpLabels),

		grpcServerHandled: counter("grpc_server_handled_total",
			"Total number of RPCs completed on the server, regardless of success or failure.", grpcLabels),
		grpcServerDuration: histogram("grpc_server_handling_seconds",
			"Duration of RPCs handled by the server in seconds.", o.grpcBuckets, grpcLabels),
		grpcClientHandled: counter("grpc_client_handled_total",
			"Total number of RPCs completed by the client, regardless of success or failure.", grpcLabels),
		grpcClientDuration: histogram("grpc_client_handling_seconds",
			"Duration of RPCs made by the client in seconds, until the last message is received.", o.grpcBuckets, grpcLabels),

		grpcConnAvailability: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   o.namespace,
			Name:        "grpc_connection_availability",
			Help:        "Current availability of a downstream gRPC service (1 = serving, 0 = not serving).",
			ConstLabels: o.constLabels,
		}, []string{"from_service", "to_service"}),
	}

	collectors := []prometheus.Collector{
		m.httpRequestsTotal, m.httpRequestDuration,
		m.grpcServerHandled, m.grpcServerDuration,
		m.grpcClientHandled, m.grpcClientDuration,
		m.grpcConnAvailability,
	}
	for i, c := range collectors {
		if err := o.registerer.Register(c); err != nil {
			for _, registered := range collectors[:i] {
				o.registerer.Unregister(registered)
			}
			return nil, fmt.Errorf("register metrics collector: %w", err)
		}
	}

	return m, nil
}

// MustNew is like New but panics on error. Convenient for package-level init in main.
func MustNew(opts ...Option) *Metrics {
	m, err := New(opts...)
	if err != nil {
		panic(err)
	}
	return m
}

// Registerer returns the registerer the collectors were registered with.
func (m *Metrics) Registerer() prometheus.Registerer { return m.registerer }

// Gatherer returns the gatherer that exposes these collectors; pass it to the
// metrics Server via WithServerGatherer.
func (m *Metrics) Gatherer() prometheus.Gatherer { return m.gatherer }

// Unregister removes all collectors of m from its registerer.
func (m *Metrics) Unregister() {
	for _, c := range []prometheus.Collector{
		m.httpRequestsTotal, m.httpRequestDuration,
		m.grpcServerHandled, m.grpcServerDuration,
		m.grpcClientHandled, m.grpcClientDuration,
		m.grpcConnAvailability,
	} {
		m.registerer.Unregister(c)
	}
}

// NewCounter creates a Counter and registers it with m's registerer.
func (m *Metrics) NewCounter(opts CounterOpts) (Counter, error) {
	c := prometheus.NewCounter(opts)
	return c, m.registerer.Register(c)
}

// NewCounterVec creates a CounterVec and registers it with m's registerer.
func (m *Metrics) NewCounterVec(opts CounterOpts, labels []string) (*CounterVec, error) {
	c := prometheus.NewCounterVec(opts, labels)
	return c, m.registerer.Register(c)
}

// NewGauge creates a Gauge and registers it with m's registerer.
func (m *Metrics) NewGauge(opts GaugeOpts) (Gauge, error) {
	g := prometheus.NewGauge(opts)
	return g, m.registerer.Register(g)
}

// NewGaugeVec creates a GaugeVec and registers it with m's registerer.
func (m *Metrics) NewGaugeVec(opts GaugeOpts, labels []string) (*GaugeVec, error) {
	g := prometheus.NewGaugeVec(opts, labels)
	return g, m.registerer.Register(g)
}

// NewHistogram creates a Histogram and registers it with m's registerer.
func (m *Metrics) NewHistogram(opts HistogramOpts) (Histogram, error) {
	h := prometheus.NewHistogram(opts)
	return h, m.registerer.Register(h)
}

// NewHistogramVec creates a HistogramVec and registers it with m's registerer.
func (m *Metrics) NewHistogramVec(opts HistogramOpts, labels []string) (*HistogramVec, error) {
	h := prometheus.NewHistogramVec(opts, labels)
	return h, m.registerer.Register(h)
}
