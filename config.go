package metrics

import (
	"errors"
	"fmt"
	"time"
)

const (
	defaultHost              = "0.0.0.0"
	defaultPort              = 8080
	defaultReadTimeout       = 10 * time.Second
	defaultWriteTimeout      = 10 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
)

var (
	ErrEmptyHost = errors.New("host cannot be empty")
	ErrZeroPort  = errors.New("port cannot be zero")
)

// Config defines the metrics server configuration.
type Config struct {
	address           string
	host              string
	port              int
	readTimeout       time.Duration
	writeTimeout      time.Duration
	readHeaderTimeout time.Duration
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.port == 0 {
		return ErrZeroPort
	}
	if c.host == "" {
		return ErrEmptyHost
	}
	return nil
}

// Option configures the Config.
type Option func(*Config)

// WithHost sets the server host (default: "0.0.0.0").
func WithHost(host string) Option {
	return func(c *Config) {
		c.host = host
	}
}

// WithPort sets the server port (default: 8080).
func WithPort(port int) Option {
	return func(c *Config) {
		c.port = port
	}
}

// WithReadTimeout sets the read timeout (default: 10s).
func WithReadTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.readTimeout = timeout
	}
}

// WithWriteTimeout sets the write timeout (default: 10s).
func WithWriteTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.writeTimeout = timeout
	}
}

// WithReadHeaderTimeout sets the read header timeout (default: 5s).
func WithReadHeaderTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.readHeaderTimeout = timeout
	}
}

// NewConfig creates a new Config with defaults and applies options.
func NewConfig(opts ...Option) *Config {
	config := &Config{
		host:              defaultHost,
		port:              defaultPort,
		readTimeout:       defaultReadTimeout,
		writeTimeout:      defaultWriteTimeout,
		readHeaderTimeout: defaultReadHeaderTimeout,
	}

	for _, opt := range opts {
		opt(config)
	}

	config.address = fmt.Sprintf("%s:%d", config.host, config.port)
	return config
}
