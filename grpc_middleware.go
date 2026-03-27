package metrics

import (
	"context"
	"strconv"
	"time"

	"google.golang.org/grpc"
)

var grpcRequestDuration = NewHistogramVec(
	HistogramOpts{
		Name:    "grpc_request_duration_seconds",
		Help:    "gRPC request duration in seconds",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1},
	},
	[]string{"service", "method", "is_error"},
)

// measureGRPCRequestDuration records the duration of gRPC requests.
func measureGRPCRequestDuration(serviceName, method string, isErr bool, start time.Time) {
	grpcRequestDuration.
		WithLabelValues(serviceName, method, strconv.FormatBool(isErr)).
		Observe(time.Since(start).Seconds())
}

// RequestDurationMetricUnaryServerInterceptor tracks gRPC request duration.
func RequestDurationMetricUnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		start := time.Now()
		defer func() {
			measureGRPCRequestDuration(serviceName, info.FullMethod, err != nil, start)
		}()
		return handler(ctx, req)
	}
}
