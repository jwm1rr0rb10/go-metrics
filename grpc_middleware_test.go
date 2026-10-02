package metrics

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const healthService = "grpc.health.v1.Health"

// startHealthServer starts an in-memory gRPC server exposing the standard
// health service, instrumented with m, and returns a client connection
// instrumented with m as well.
func startHealthServer(t *testing.T, m *Metrics) (*health.Server, *grpc.ClientConn) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)

	var serverOpts []grpc.ServerOption
	var clientOpts []grpc.DialOption
	if m != nil {
		serverOpts = append(serverOpts,
			grpc.ChainUnaryInterceptor(m.UnaryServerInterceptor()),
			grpc.ChainStreamInterceptor(m.StreamServerInterceptor()),
		)
		clientOpts = append(clientOpts,
			grpc.WithChainUnaryInterceptor(m.UnaryClientInterceptor()),
			grpc.WithChainStreamInterceptor(m.StreamClientInterceptor()),
		)
	}

	srv := grpc.NewServer(serverOpts...)
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet", append(clientOpts,
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return hs, conn
}

func serverCount(m *Metrics, typ, method string, code codes.Code) float64 {
	return testutil.ToFloat64(m.grpcServerHandled.WithLabelValues(typ, healthService, method, code.String()))
}

func clientCount(m *Metrics, typ, method string, code codes.Code) float64 {
	return testutil.ToFloat64(m.grpcClientHandled.WithLabelValues(typ, healthService, method, code.String()))
}

func TestGRPC_Unary(t *testing.T) {
	m, _ := newTestMetrics(t)
	_, conn := startHealthServer(t, m)
	client := grpc_health_v1.NewHealthClient(conn)
	ctx := context.Background()

	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	_, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	for _, code := range []codes.Code{codes.OK, codes.NotFound} {
		if got := serverCount(m, grpcTypeUnary, "Check", code); got != 1 {
			t.Errorf("server %s: got %v, want 1", code, got)
		}
		if got := clientCount(m, grpcTypeUnary, "Check", code); got != 1 {
			t.Errorf("client %s: got %v, want 1", code, got)
		}
	}
}

func TestGRPC_ServerStream(t *testing.T) {
	m, _ := newTestMetrics(t)
	_, conn := startHealthServer(t, m)
	client := grpc_health_v1.NewHealthClient(conn)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	if got := clientCount(m, grpcTypeServerStream, "Watch", codes.OK); got != 0 {
		t.Fatal("server stream must not be recorded before it ends")
	}

	cancel()
	if _, err := stream.Recv(); status.Code(err) != codes.Canceled {
		t.Fatalf("expected Canceled, got %v", err)
	}

	if got := clientCount(m, grpcTypeServerStream, "Watch", codes.Canceled); got != 1 {
		t.Errorf("client: got %v, want 1", got)
	}
	waitFor(t, "server stream metric", func() bool {
		return testutil.CollectAndCount(m.grpcServerHandled) == 1
	})
}

func TestGRPC_ServerInterceptorPanic(t *testing.T) {
	m, _ := newTestMetrics(t)
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Boom"}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = m.UnaryServerInterceptor()(context.Background(), nil, info, func(context.Context, any) (any, error) {
			panic("boom")
		})
	}()

	if recovered != "boom" {
		t.Fatalf("panic must be re-raised, got %v", recovered)
	}
	got := testutil.ToFloat64(m.grpcServerHandled.WithLabelValues(grpcTypeUnary, "pkg.Svc", "Boom", codes.Internal.String()))
	if got != 1 {
		t.Errorf("got %v, want 1", got)
	}
}

// fakeClientStream returns the queued errors from RecvMsg.
type fakeClientStream struct {
	grpc.ClientStream
	recv []error
}

func (f *fakeClientStream) RecvMsg(any) error {
	err := f.recv[0]
	f.recv = f.recv[1:]
	return err
}

func TestGRPC_ClientStreamFinish(t *testing.T) {
	tests := []struct {
		name string
		desc grpc.StreamDesc
		recv []error
		typ  string
		code codes.Code
	}{
		{"client stream finishes on response", grpc.StreamDesc{ClientStreams: true}, []error{nil}, grpcTypeClientStream, codes.OK},
		{"bidi finishes on EOF", grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, []error{nil, nil, io.EOF}, grpcTypeBidiStream, codes.OK},
		{"server stream error", grpc.StreamDesc{ServerStreams: true}, []error{nil, status.Error(codes.Unavailable, "x")}, grpcTypeServerStream, codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestMetrics(t)
			fake := &fakeClientStream{recv: tt.recv}
			streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
				return fake, nil
			}
			cs, err := m.StreamClientInterceptor()(context.Background(), &tt.desc, nil, "/pkg.Svc/M", streamer)
			if err != nil {
				t.Fatal(err)
			}
			for range tt.recv {
				_ = cs.RecvMsg(nil)
			}
			if n := testutil.CollectAndCount(m.grpcClientHandled); n != 1 {
				t.Fatalf("expected exactly one series, got %d", n)
			}
			got := testutil.ToFloat64(m.grpcClientHandled.WithLabelValues(tt.typ, "pkg.Svc", "M", tt.code.String()))
			if got != 1 {
				t.Errorf("got %v, want 1", got)
			}
		})
	}
}

func TestSplitMethod(t *testing.T) {
	tests := []struct{ in, service, method string }{
		{"/pkg.Svc/Method", "pkg.Svc", "Method"},
		{"/a.b.c.Svc/M", "a.b.c.Svc", "M"},
		{"bad", "unknown", "bad"},
	}
	for _, tt := range tests {
		s, m := splitMethod(tt.in)
		if s != tt.service || m != tt.method {
			t.Errorf("splitMethod(%q) = %q, %q", tt.in, s, m)
		}
	}
}

func BenchmarkUnaryServerInterceptor(b *testing.B) {
	m, _ := newTestMetrics(b)
	interceptor := m.UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	handler := func(context.Context, any) (any, error) { return nil, nil }
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = interceptor(ctx, nil, info, handler)
	}
}
