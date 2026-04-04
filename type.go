package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Registerer allows custom metric registration.
	Registerer = prometheus.DefaultRegisterer
)

// Metric types (aliases from Prometheus).
type (
	CounterOpts   = prometheus.CounterOpts
	GaugeOpts     = prometheus.GaugeOpts
	HistogramOpts = prometheus.HistogramOpts
	Counter       = prometheus.Counter
	CounterVec    = prometheus.CounterVec
	Gauge         = prometheus.Gauge
	GaugeVec      = prometheus.GaugeVec
	Histogram     = prometheus.Histogram
	HistogramVec  = prometheus.HistogramVec
)
