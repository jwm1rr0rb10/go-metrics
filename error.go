package metrics

import "github.com/jwm1rr0rb10/go-errors"

var (
	ErrEmptyHost = errors.New("host cannot be empty")
	ErrZeroPort  = errors.New("port cannot be zero")
)
