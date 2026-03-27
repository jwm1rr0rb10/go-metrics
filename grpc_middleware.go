package metrics

import (
	"context"
	"strconv"
	"time"

	"google.golang.org/grpc"
)

var (
	grpcRequestsTotal = NewCounterVec(
		CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"service", "method", "is_error"},
	)

	grpcRequestDuration = NewHistogramVec(
		HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC request duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
		},
		[]string{"service", "method", "is_error"},
	)
)

func measureGRPCRequest(serviceName, method string, isErr bool, start time.Time) {
	isErrorStr := strconv.FormatBool(isErr)
	labels := []string{serviceName, method, isErrorStr}
	grpcRequestsTotal.WithLabelValues(labels...).Inc()
	grpcRequestDuration.WithLabelValues(labels...).Observe(time.Since(start).Seconds())
}

// RequestDurationMetricUnaryServerInterceptor tracks gRPC request duration and count.
func RequestDurationMetricUnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		start := time.Now()
		defer func() {
			measureGRPCRequest(serviceName, info.FullMethod, err != nil, start)
		}()
		return handler(ctx, req)
	}
}
