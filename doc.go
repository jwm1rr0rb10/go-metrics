// Package metrics provides Prometheus instrumentation for Go services:
// HTTP middleware, gRPC server and client interceptors, a downstream gRPC
// availability monitor and a /metrics HTTP server.
//
// Nothing is registered at import time. Create a *Metrics with New, choosing
// the registerer, namespace, constant labels and histogram buckets, and share
// it across the process:
//
//	m, err := metrics.New(
//		metrics.WithRegisterer(reg),
//		metrics.WithNamespace("myapp"),
//	)
//	handler := m.HTTPMiddleware()(mux)
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(m.UnaryServerInterceptor()))
//
// All label values are bounded: HTTP methods are normalized, routes come
// from route templates (never raw paths) and gRPC calls are labeled by
// status code, so client input cannot inflate series cardinality.
package metrics
