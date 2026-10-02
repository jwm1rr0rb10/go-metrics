package metrics

import "github.com/jwm1rr0rb10/go-errors"

// Errors returned by this package; compare with errors.Is.
var (
	ErrEmptyHost           = errors.New("host cannot be empty")
	ErrZeroPort            = errors.New("port cannot be zero")
	ErrInvalidPort         = errors.New("port must be in range 1-65535")
	ErrInvalidTimeout      = errors.New("timeouts cannot be negative")
	ErrInvalidPath         = errors.New("metrics path must start with '/'")
	ErrNilRegisterer       = errors.New("registerer cannot be nil")
	ErrNilGatherer         = errors.New("gatherer cannot be nil")
	ErrInvalidNativeFactor = errors.New("native histogram bucket factor must be greater than 1")
	ErrServerStarted       = errors.New("metrics server already started")
	ErrServerClosed        = errors.New("metrics server closed")
	ErrMonitorStarted      = errors.New("connection monitor already started")
	ErrInvalidInterval     = errors.New("ping interval and check timeout must be positive")
	ErrNilConnection       = errors.New("grpc connection cannot be nil")
)
