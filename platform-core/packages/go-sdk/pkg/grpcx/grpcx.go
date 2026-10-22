package grpcx

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

const internalMessage = "internal error"

// ServerOptions configures NewServer.
type ServerOptions struct {
	Config config.GRPC
	// Logger receives call logs and recovered panics. Nil means slog.Default().
	Logger *slog.Logger
	// Health is required; its gRPC health server is registered on the server.
	Health *healthx.Health
	// Extra options are appended after the SDK's own.
	Extra []grpc.ServerOption
}

// NewServer returns a gRPC server with the otelgrpc stats handler, the chain
// request id, recovery, logging and auth (outermost first), the health service
// and reflection.
func NewServer(o ServerOptions) *grpc.Server {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logOpts := []logging.Option{
		logging.WithLogOnEvents(logging.FinishCall),
		logging.WithFieldsFromContext(func(ctx context.Context) logging.Fields {
			if id := reqctx.RequestID(ctx); id != "" {
				return logging.Fields{"request_id", id}
			}
			return nil
		}),
	}
	opts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			requestIDUnary(),
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandlerContext(recoverPanic(logger))),
			logging.UnaryServerInterceptor(slogLogger(logger), logOpts...),
			auth.UnaryServerInterceptor(),
			principalLogUnary(),
		),
		grpc.ChainStreamInterceptor(
			requestIDStream(),
			recovery.StreamServerInterceptor(recovery.WithRecoveryHandlerContext(recoverPanic(logger))),
			logging.StreamServerInterceptor(slogLogger(logger), logOpts...),
			auth.StreamServerInterceptor(),
			principalLogStream(),
		),
	}
	srv := grpc.NewServer(append(opts, o.Extra...)...)

	grpc_health_v1.RegisterHealthServer(srv, o.Health.Server())
	reflection.Register(srv)
	return srv
}

// slogLogger adapts a slog.Logger to the go-grpc-middleware logging interface.
func slogLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(level), msg, fields...)
	})
}

// recoverPanic logs the panic with its stack and hides it from the caller.
func recoverPanic(l *slog.Logger) recovery.RecoveryHandlerFuncContext {
	return func(ctx context.Context, p any) error {
		l.ErrorContext(ctx, "panic recovered",
			"method", grpcMethod(ctx), "request_id", reqctx.RequestID(ctx), "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		return status.Error(codes.Internal, internalMessage)
	}
}

// principalLogUnary copies the principal id that auth resolved into the
// logging interceptor's fields. Logging runs before auth in the chain, so its
// own context never sees the principal.
func principalLogUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		addPrincipalField(ctx)
		return handler(ctx, req)
	}
}

func principalLogStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		addPrincipalField(ss.Context())
		return handler(srv, ss)
	}
}

func addPrincipalField(ctx context.Context) {
	if id := reqctx.PrincipalID(ctx); id != "" {
		logging.AddFields(ctx, logging.Fields{"principal_id", id})
	}
}

func grpcMethod(ctx context.Context) string {
	m, _ := grpc.Method(ctx)
	return m
}

// Serve returns a component that listens on c.Addr() and serves srv. When its
// context ends it calls GracefulStop and, if that has not finished within
// c.ShutdownGrace, Stop. Under lifecycle.Run the components get
// Options.ShutdownGrace minus Options.CloseGrace, so keep c.ShutdownGrace within
// that or the closers (database pool, for one) can run while RPCs still drain.
func Serve(srv *grpc.Server, c config.GRPC) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		lis, err := net.Listen("tcp", c.Addr())
		if err != nil {
			return fmt.Errorf("grpcx: listen %s: %w", c.Addr(), err)
		}
		return ServeListener(srv, lis, c.ShutdownGrace).Run(ctx)
	})
}

// ServeListener is Serve on a caller-provided listener, for tests that need an
// ephemeral port (listen on 127.0.0.1:0 and read lis.Addr()).
func ServeListener(srv *grpc.Server, lis net.Listener, grace time.Duration) lifecycle.Component {
	return lifecycle.Func(func(ctx context.Context) error {
		served := make(chan error, 1)
		go func() { served <- srv.Serve(lis) }()

		select {
		case err := <-served:
			return err // the server stopped by itself
		case <-ctx.Done():
		}

		stopped := make(chan struct{})
		go func() { srv.GracefulStop(); close(stopped) }()
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-stopped:
		case <-timer.C:
			srv.Stop()
		}
		return <-served
	})
}
