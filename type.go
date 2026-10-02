package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metric types re-exported from Prometheus so callers need not import it for common cases.
type (
	// CounterOpts is an alias of prometheus.CounterOpts.
	CounterOpts = prometheus.CounterOpts
	// GaugeOpts is an alias of prometheus.GaugeOpts.
	GaugeOpts = prometheus.GaugeOpts
	// HistogramOpts is an alias of prometheus.HistogramOpts.
	HistogramOpts = prometheus.HistogramOpts
	// Counter is an alias of prometheus.Counter.
	Counter = prometheus.Counter
	// CounterVec is an alias of prometheus.CounterVec.
	CounterVec = prometheus.CounterVec
	// Gauge is an alias of prometheus.Gauge.
	Gauge = prometheus.Gauge
	// GaugeVec is an alias of prometheus.GaugeVec.
	GaugeVec = prometheus.GaugeVec
	// Histogram is an alias of prometheus.Histogram.
	Histogram = prometheus.Histogram
	// HistogramVec is an alias of prometheus.HistogramVec.
	HistogramVec = prometheus.HistogramVec
	// Labels is an alias of prometheus.Labels.
	Labels = prometheus.Labels
)
