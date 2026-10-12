package grpcx_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type testSvc interface{}

// unary builds a method handler that decodes a StringValue and calls fn.
func unary(method string, fn func(ctx context.Context, in *wrapperspb.StringValue) (proto.Message, error)) grpc.MethodHandler {
	return func(_ any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		call := func(ctx context.Context, req any) (any, error) {
			return fn(ctx, req.(*wrapperspb.StringValue))
		}
		if ic == nil {
			return call(ctx, in)
		}
		return ic(ctx, in, &grpc.UnaryServerInfo{FullMethod: "/test.v1.Test/" + method}, call)
	}
}

func testDesc(methods map[string]grpc.MethodHandler) *grpc.ServiceDesc {
	d := &grpc.ServiceDesc{ServiceName: "test.v1.Test", HandlerType: (*testSvc)(nil)}
	for name, h := range methods {
		d.Methods = append(d.Methods, grpc.MethodDesc{MethodName: name, Handler: h})
	}
	return d
}

type env struct {
	conn *grpc.ClientConn
	logs *syncBuffer
}

// start serves methods on a bufconn listener behind grpcx.NewServer.
func start(t *testing.T, methods map[string]grpc.MethodHandler) env {
	t.Helper()
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	srv := grpcx.NewServer(grpcx.ServerOptions{
		Config: config.GRPC{ReflectionEnabled: true},
		Logger: logger,
		Health: healthx.New(time.Second, logger),
	})
	srv.RegisterService(testDesc(methods), testSvc(nil))

	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return env{conn: conn, logs: logs}
}

func call(ctx context.Context, conn *grpc.ClientConn, method, in string, opts ...grpc.CallOption) (string, error) {
	out := new(wrapperspb.StringValue)
	err := conn.Invoke(ctx, "/test.v1.Test/"+method, wrapperspb.String(in), out, opts...)
	return out.GetValue(), err
}

func echoRequestID(ctx context.Context, _ *wrapperspb.StringValue) (proto.Message, error) {
	return wrapperspb.String(reqctx.RequestID(ctx)), nil
}

func TestRecoveryKeepsServing(t *testing.T) {
	e := start(t, map[string]grpc.MethodHandler{
		"Boom": unary("Boom", func(context.Context, *wrapperspb.StringValue) (proto.Message, error) {
			panic("kaboom-secret")
		}),
		"Ping": unary("Ping", echoRequestID),
	})

	_, err := call(context.Background(), e.conn, "Boom", "")
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("Boom err = %v, want Internal %q", err, "internal error")
	}
	if _, err := call(context.Background(), e.conn, "Ping", ""); err != nil {
		t.Fatalf("call after panic: %v", err)
	}

	logs := e.logs.String()
	for _, want := range []string{`"level":"ERROR"`, "kaboom-secret", `"stack"`, `"method":"/test.v1.Test/Boom"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("panic log lacks %s in:\n%s", want, logs)
		}
	}
}

func TestRequestIDPropagates(t *testing.T) {
	e := start(t, map[string]grpc.MethodHandler{"Ping": unary("Ping", echoRequestID)})

	ctx := metadata.AppendToOutgoingContext(context.Background(), reqctx.MetadataRequestID, "abc")
	var hdr metadata.MD
	got, err := call(ctx, e.conn, "Ping", "", grpc.Header(&hdr))
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got != "abc" || hdr.Get(reqctx.MetadataRequestID)[0] != "abc" {
		t.Fatalf("handler id = %q, header = %v, want abc", got, hdr)
	}

	hdr = nil
	got, err = call(context.Background(), e.conn, "Ping", "", grpc.Header(&hdr))
	if err != nil {
		t.Fatalf("Ping without id: %v", err)
	}
	if got == "" || len(hdr.Get(reqctx.MetadataRequestID)) != 1 || hdr.Get(reqctx.MetadataRequestID)[0] != got {
		t.Fatalf("generated id = %q, header = %v", got, hdr)
	}
}

func TestLoggingRecordsCallWithoutPayload(t *testing.T) {
	e := start(t, map[string]grpc.MethodHandler{"Ping": unary("Ping", echoRequestID)})

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		reqctx.MetadataRequestID, "r1", "x-principal-id", "u1", "x-principal-scopes", "scope-secret")
	if _, err := call(ctx, e.conn, "Ping", "payload-secret"); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	logs := e.logs.String()
	for _, want := range []string{`"grpc.method":"Ping"`, `"grpc.code":"OK"`, `"request_id":"r1"`, `"principal_id":"u1"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %s in:\n%s", want, logs)
		}
	}
	for _, leak := range []string{"payload-secret", "scope-secret"} {
		if strings.Contains(logs, leak) {
			t.Errorf("log leaks %q:\n%s", leak, logs)
		}
	}
}

var (
	errNotFound = errors.New("thing not found")
	errTaken    = errors.New("name taken")
)

func TestStatusMapsKnownErrors(t *testing.T) {
	table := grpcx.ErrorTable{errNotFound: codes.NotFound, errTaken: codes.AlreadyExists}
	logger := slog.New(slog.NewJSONHandler(&syncBuffer{}, nil))

	err := grpcx.Status(context.Background(), logger, fmt.Errorf("lookup 7: %w", errNotFound), table)
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound || st.Message() != "lookup 7: thing not found" {
		t.Fatalf("got %v", err)
	}
	if err := grpcx.Status(context.Background(), logger, nil, table); err != nil {
		t.Fatalf("nil error mapped to %v", err)
	}
}

func TestStatusHidesUnknownErrors(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))

	err := grpcx.Status(context.Background(), logger, errors.New("pq: password=secret"), grpcx.ErrorTable{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v, want Internal %q", err, "internal error")
	}
	if !strings.Contains(logs.String(), "pq: password=secret") || !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("original error must be logged at error level, got %s", logs.String())
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func runServe(t *testing.T, srv *grpc.Server, c config.GRPC) (cancel func(), done <-chan error) {
	t.Helper()
	ctx, cancelCtx := context.WithCancel(context.Background())
	t.Cleanup(cancelCtx)
	ch := make(chan error, 1)
	go func() { ch <- grpcx.Serve(srv, c).Run(ctx) }()
	return cancelCtx, ch
}

func dialWhenUp(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestServeStopsGracefullyOnCancel(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&syncBuffer{}, nil))
	c := config.GRPC{Host: "127.0.0.1", Port: freePort(t), ShutdownGraceSeconds: 5}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: c, Logger: logger, Health: healthx.New(time.Second, logger)})
	srv.RegisterService(testDesc(map[string]grpc.MethodHandler{"Ping": unary("Ping", echoRequestID)}), testSvc(nil))

	cancel, done := runServe(t, srv, c)
	conn := dialWhenUp(t, c.Addr())
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if _, err := call(ctx, conn, "Ping", ""); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

func TestServeForcesStopAfterGrace(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&syncBuffer{}, nil))
	c := config.GRPC{Host: "127.0.0.1", Port: freePort(t), ShutdownGraceSeconds: 0.2}
	started := make(chan struct{})
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: c, Logger: logger, Health: healthx.New(time.Second, logger)})
	srv.RegisterService(testDesc(map[string]grpc.MethodHandler{
		"Hang": unary("Hang", func(ctx context.Context, _ *wrapperspb.StringValue) (proto.Message, error) {
			close(started)
			<-ctx.Done()
			return &emptypb.Empty{}, ctx.Err()
		}),
	}), testSvc(nil))

	cancel, done := runServe(t, srv, c)
	conn := dialWhenUp(t, c.Addr())
	go func() { _, _ = call(context.Background(), conn, "Hang", "") }()
	<-started

	begin := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not force-stop a hanging call")
	}
	if time.Since(begin) < 150*time.Millisecond {
		t.Fatal("Serve stopped before the grace period elapsed")
	}
}

func TestRecoveryLogCarriesRequestID(t *testing.T) {
	e := start(t, map[string]grpc.MethodHandler{
		"Boom": unary("Boom", func(context.Context, *wrapperspb.StringValue) (proto.Message, error) {
			panic("kaboom")
		}),
	})
	ctx := metadata.AppendToOutgoingContext(context.Background(), reqctx.MetadataRequestID, "rid-7")
	_, _ = call(ctx, e.conn, "Boom", "")

	logs := e.logs.String()
	if !strings.Contains(logs, "panic recovered") || !strings.Contains(logs, `"request_id":"rid-7"`) {
		t.Fatalf("recovery log lacks request_id:\n%s", logs)
	}
}

func TestStatusPassesThroughGRPCStatus(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	in := status.Error(codes.Unauthenticated, "authentication required")

	out := grpcx.Status(context.Background(), logger, in, grpcx.ErrorTable{})
	if out != in { //nolint:errorlint // identity is the point: the error must be returned unchanged
		t.Fatalf("got %v, want the same error", out)
	}
	// An Unknown status is not trusted: it is hidden like any other error.
	unk := grpcx.Status(context.Background(), logger, status.Error(codes.Unknown, "pq: secret"), grpcx.ErrorTable{})
	if st, _ := status.FromError(unk); st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("Unknown status leaked: %v", unk)
	}
}

func TestServeListenerServesAndStops(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&syncBuffer{}, nil))
	srv := grpcx.NewServer(grpcx.ServerOptions{Logger: logger, Health: healthx.New(time.Second, logger)})
	srv.RegisterService(testDesc(map[string]grpc.MethodHandler{"Ping": unary("Ping", echoRequestID)}), testSvc(nil))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- grpcx.ServeListener(srv, lis, 5*time.Second).Run(ctx) }()

	conn := dialWhenUp(t, lis.Addr().String())
	cctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if _, err := call(cctx, conn, "Ping", ""); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
}
