package metrics

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gRPC call types used as the grpc_type label value.
const (
	grpcTypeUnary        = "unary"
	grpcTypeClientStream = "client_stream"
	grpcTypeServerStream = "server_stream"
	grpcTypeBidiStream   = "bidi_stream"
)

func streamType(clientStreams, serverStreams bool) string {
	switch {
	case clientStreams && serverStreams:
		return grpcTypeBidiStream
	case clientStreams:
		return grpcTypeClientStream
	case serverStreams:
		return grpcTypeServerStream
	default:
		return grpcTypeUnary
	}
}

// splitMethod splits "/package.Service/Method" into service and method without allocating.
func splitMethod(fullMethod string) (service, method string) {
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	if i := strings.LastIndexByte(fullMethod, '/'); i >= 0 {
		return fullMethod[:i], fullMethod[i+1:]
	}
	return "unknown", fullMethod
}

func (m *Metrics) observeGRPCServer(typ, fullMethod string, code codes.Code, d time.Duration) {
	service, method := splitMethod(fullMethod)
	c := code.String()
	m.grpcServerHandled.WithLabelValues(typ, service, method, c).Inc()
	m.grpcServerDuration.WithLabelValues(typ, service, method, c).Observe(d.Seconds())
}

func (m *Metrics) observeGRPCClient(typ, fullMethod string, code codes.Code, d time.Duration) {
	service, method := splitMethod(fullMethod)
	c := code.String()
	m.grpcClientHandled.WithLabelValues(typ, service, method, c).Inc()
	m.grpcClientDuration.WithLabelValues(typ, service, method, c).Observe(d.Seconds())
}

// UnaryServerInterceptor records grpc_server_handled_total and
// grpc_server_handling_seconds for unary RPCs. A panicking handler is recorded
// as codes.Internal and the panic is re-raised unchanged.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				m.observeGRPCServer(grpcTypeUnary, info.FullMethod, codes.Internal, time.Since(start))
				panic(p)
			}
			m.observeGRPCServer(grpcTypeUnary, info.FullMethod, status.Code(err), time.Since(start))
		}()
		return handler(ctx, req)
	}
}

// StreamServerInterceptor records grpc_server_handled_total and
// grpc_server_handling_seconds for streaming RPCs.
func (m *Metrics) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		start := time.Now()
		typ := streamType(info.IsClientStream, info.IsServerStream)
		defer func() {
			if p := recover(); p != nil {
				m.observeGRPCServer(typ, info.FullMethod, codes.Internal, time.Since(start))
				panic(p)
			}
			m.observeGRPCServer(typ, info.FullMethod, status.Code(err), time.Since(start))
		}()
		return handler(srv, ss)
	}
}

// UnaryClientInterceptor records grpc_client_handled_total and
// grpc_client_handling_seconds for outgoing unary RPCs.
func (m *Metrics) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		m.observeGRPCClient(grpcTypeUnary, method, status.Code(err), time.Since(start))
		return err
	}
}

// StreamClientInterceptor records grpc_client_handled_total and
// grpc_client_handling_seconds for outgoing streaming RPCs. The RPC is
// considered finished when RecvMsg returns io.EOF or an error (or, for
// client-streaming RPCs, after the single response is received). Streams the
// caller abandons without reading to the end are not recorded.
func (m *Metrics) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		start := time.Now()
		typ := streamType(desc.ClientStreams, desc.ServerStreams)
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			m.observeGRPCClient(typ, method, status.Code(err), time.Since(start))
			return nil, err
		}
		return &monitoredClientStream{
			ClientStream:  cs,
			serverStreams: desc.ServerStreams,
			finish: func(err error) {
				m.observeGRPCClient(typ, method, status.Code(err), time.Since(start))
			},
		}, nil
	}
}

type monitoredClientStream struct {
	grpc.ClientStream
	serverStreams bool
	once          sync.Once
	finish        func(error)
}

func (s *monitoredClientStream) RecvMsg(msg any) error {
	err := s.ClientStream.RecvMsg(msg)
	switch {
	case errors.Is(err, io.EOF):
		s.once.Do(func() { s.finish(nil) })
	case err != nil:
		s.once.Do(func() { s.finish(err) })
	case !s.serverStreams:
		s.once.Do(func() { s.finish(nil) })
	}
	return err
}
