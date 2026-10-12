package app_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/app"
	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/config"
	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{} // closed when Run returned; err is then set
	err    error
}

// freePort asks the kernel for an unused port, then releases it for the
// service to bind. There is a small window where another process can take it;
// for a race-free alternative build the server yourself and use
// grpcx.ServeListener on a 127.0.0.1:0 listener. GRPC_PORT has no default and
// 0 is not a usable value, so app.Run needs a concrete port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func settings(port int) config.Settings {
	return config.Settings{
		Runtime: sdkconfig.Runtime{Env: "test", LogLevel: "error", LogJSON: true},
		GRPC:    sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGraceSeconds: 5},
		// Nothing listens here; ENV=test tolerates that, so these tests run without Postgres.
		Postgres: sdkconfig.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable"},
	}
}

// startApp runs app.Run on a free loopback port with an unreachable database (ENV=test).
func startApp(t *testing.T) *running {
	t.Helper()
	port := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{})}
	go func() { r.err = app.Run(ctx, settings(port)); close(r.done) }()

	var err error
	r.conn, err = grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.conn.Close()
		cancel()
		select {
		case <-r.done:
		case <-time.After(10 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func greet(ctx context.Context, conn *grpc.ClientConn, name string) (string, error) {
	out := new(wrapperspb.StringValue)
	err := conn.Invoke(ctx, "/hello.v1.Hello/Greet", wrapperspb.String(name), out, grpc.WaitForReady(true))
	return out.GetValue(), err
}

func TestShutdownDrainsHealthFirst(t *testing.T) {
	r := startApp(t)

	watchCtx, stopWatch := context.WithTimeout(context.Background(), 15*time.Second)
	defer stopWatch()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, %v, want SERVING", first.GetStatus(), err)
	}

	r.cancel()

	// The open Watch stream keeps the graceful stop pending, so the connection
	// is still up when the drain reaches the watcher.
	next, err := stream.Recv()
	if err != nil {
		t.Fatalf("connection closed before NOT_SERVING was observed: %v", err)
	}
	if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("status after cancel = %v, want NOT_SERVING", next.GetStatus())
	}
	select {
	case <-r.done:
		t.Fatalf("app returned (%v) before the watcher released the server", r.err)
	default:
	}

	stopWatch()
	select {
	case <-r.done:
		if r.err != nil {
			t.Fatalf("Run = %v, want nil", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("app did not stop after the watcher left")
	}
}

func TestGreetRejectsMissingPrincipal(t *testing.T) {
	r := startApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := greet(ctx, r.conn, "Ann"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no principal: err = %v, want Unauthenticated", err)
	}

	authed := metadata.AppendToOutgoingContext(ctx, "x-principal-id", "u1", "x-principal-type", "user")
	got, err := greet(authed, r.conn, "Ann")
	if err != nil || got != "hello, Ann (caller u1)" {
		t.Fatalf("with principal: %q, %v", got, err)
	}

	if _, err := greet(authed, r.conn, " "); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty name: err = %v, want InvalidArgument", err)
	}
}

func TestBoomIsRecovered(t *testing.T) {
	r := startApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := r.conn.Invoke(ctx, "/hello.v1.Hello/Boom", &emptypb.Empty{}, &emptypb.Empty{}, grpc.WaitForReady(true))
	if status.Code(err) != codes.Internal {
		t.Fatalf("Boom err = %v, want Internal", err)
	}
	authed := metadata.AppendToOutgoingContext(ctx, "x-principal-id", "u1")
	if _, err := greet(authed, r.conn, "Ann"); err != nil {
		t.Fatalf("call after panic: %v", err)
	}
}

func TestBootFailsWhenDatabaseUnreachableOutsideLocal(t *testing.T) {
	s := settings(freePort(t))
	s.Env = "prod"
	s.Postgres = sdkconfig.Postgres{URL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable"}

	if err := app.Run(context.Background(), s); err == nil {
		t.Fatal("Run succeeded with an unreachable database in ENV=prod")
	}
}
