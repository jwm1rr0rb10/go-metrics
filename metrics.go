package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// SetRegisterer sets a custom Prometheus registerer.
func SetRegisterer(r prometheus.Registerer) {
	Registerer = r
}

// NewCounter creates a Counter metric.
func NewCounter(opts CounterOpts) Counter {
	return promauto.With(Registerer).NewCounter(opts)
}

// NewCounterVec creates a CounterVec metric.
func NewCounterVec(opts CounterOpts, labels []string) *CounterVec {
	return promauto.With(Registerer).NewCounterVec(opts, labels)
}

// NewGauge creates a Gauge metric.
func NewGauge(opts GaugeOpts) Gauge {
	return promauto.With(Registerer).NewGauge(opts)
}

// NewGaugeVec creates a GaugeVec metric.
func NewGaugeVec(opts GaugeOpts, labels []string) *GaugeVec {
	return promauto.With(Registerer).NewGaugeVec(opts, labels)
}

// NewHistogram creates a Histogram metric.
func NewHistogram(opts HistogramOpts) Histogram {
	return promauto.With(Registerer).NewHistogram(opts)
}

// NewHistogramVec creates a HistogramVec metric.
func NewHistogramVec(opts HistogramOpts, labels []string) *HistogramVec {
	return promauto.With(Registerer).NewHistogramVec(opts, labels)
}
