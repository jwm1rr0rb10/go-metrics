package metrics

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	defaultHost              = "0.0.0.0"
	defaultPort              = 8080
	defaultPath              = "/metrics"
	defaultReadTimeout       = 10 * time.Second
	defaultWriteTimeout      = 10 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
)

// Config defines the metrics server configuration.
type Config struct {
	host              string
	port              int
	path              string
	gatherer          prometheus.Gatherer
	readTimeout       time.Duration
	writeTimeout      time.Duration
	readHeaderTimeout time.Duration
	shutdownTimeout   time.Duration
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	switch {
	case c.host == "":
		return ErrEmptyHost
	case c.port == 0:
		return ErrZeroPort
	case c.port < 0 || c.port > 65535:
		return ErrInvalidPort
	case c.readTimeout < 0 || c.writeTimeout < 0 || c.readHeaderTimeout < 0 || c.shutdownTimeout < 0:
		return ErrInvalidTimeout
	case !strings.HasPrefix(c.path, "/"):
		return ErrInvalidPath
	case c.gatherer == nil:
		return ErrNilGatherer
	}
	return nil
}

// Address returns the configured listen address in host:port form.
func (c *Config) Address() string {
	return net.JoinHostPort(c.host, strconv.Itoa(c.port))
}

// ConfigOption configures the Config.
type ConfigOption func(*Config)

// WithHost sets the server host (default: "0.0.0.0").
func WithHost(host string) ConfigOption {
	return func(c *Config) { c.host = host }
}

// WithPort sets the server port (default: 8080).
func WithPort(port int) ConfigOption {
	return func(c *Config) { c.port = port }
}

// WithPath sets the path metrics are served on (default: "/metrics").
func WithPath(path string) ConfigOption {
	return func(c *Config) { c.path = path }
}

// WithServerGatherer sets the gatherer whose metrics are exposed
// (default: prometheus.DefaultGatherer). Pass Metrics.Gatherer() when using a custom registry.
func WithServerGatherer(g prometheus.Gatherer) ConfigOption {
	return func(c *Config) { c.gatherer = g }
}

// WithReadTimeout sets the read timeout (default: 10s).
func WithReadTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) { c.readTimeout = timeout }
}

// WithWriteTimeout sets the write timeout (default: 10s).
func WithWriteTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) { c.writeTimeout = timeout }
}

// WithReadHeaderTimeout sets the read header timeout (default: 5s).
func WithReadHeaderTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) { c.readHeaderTimeout = timeout }
}

// WithShutdownTimeout sets how long graceful shutdown may take (default: 10s).
func WithShutdownTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) { c.shutdownTimeout = timeout }
}

// NewConfig creates a new Config with defaults and applies options.
func NewConfig(opts ...ConfigOption) *Config {
	config := &Config{
		host:              defaultHost,
		port:              defaultPort,
		path:              defaultPath,
		gatherer:          prometheus.DefaultGatherer,
		readTimeout:       defaultReadTimeout,
		writeTimeout:      defaultWriteTimeout,
		readHeaderTimeout: defaultReadHeaderTimeout,
		shutdownTimeout:   defaultShutdownTimeout,
	}

	for _, opt := range opts {
		opt(config)
	}
	return config
}
