package interceptor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

// mdRequestID is the human-facing correlation id bridged from the platform's
// HTTP convention onto gRPC metadata. Distinct from W3C `traceparent`, which
// otel propagates automatically.
const mdRequestID = "x-request-id"

// requestIDCtxKey carries the request id on the context (unexported: no collisions).
type requestIDCtxKey struct{}

// OTelStatsHandler returns the otelgrpc server stats handler. A single
// stats.Handler covers unary AND streaming and reads/writes W3C `traceparent`.
//
// ADR-0004: tracing is W3C trace-context via OpenTelemetry; the exporter is
// swappable (OTLP/gRPC, stdout, or a no-op when OTEL_ENABLED=false) without
// touching handler code.
func OTelStatsHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

// RequestIDFromContext returns the correlation id for this call, if present.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDCtxKey{}).(string)
	return id, ok
}

// RequestIDUnaryInterceptor bridges `x-request-id`: it reads the incoming value,
// generates one when absent, puts it on the context, and logs the call. Pair it
// with OTelStatsHandler() (the stats handler owns `traceparent`; this owns the
// human-facing id).
func RequestIDUnaryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx, reqID := withRequestID(ctx)
		logger.InfoContext(ctx, "grpc.request",
			slog.String("method", info.FullMethod),
			slog.String(mdRequestID, reqID),
		)
		return handler(ctx, req)
	}
}

// RequestIDStreamInterceptor is the streaming counterpart.
func RequestIDStreamInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, reqID := withRequestID(ss.Context())
		logger.InfoContext(ctx, "grpc.stream",
			slog.String("method", info.FullMethod),
			slog.String(mdRequestID, reqID),
		)
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// withRequestID resolves (or mints) the correlation id and returns a context
// carrying it.
func withRequestID(ctx context.Context) (context.Context, string) {
	reqID := incomingRequestID(ctx)
	if reqID == "" {
		reqID = newRequestID()
	}
	return context.WithValue(ctx, requestIDCtxKey{}, reqID), reqID
}

func incomingRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if vals := md.Get(mdRequestID); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// newRequestID returns a random 128-bit hex id. Falls back to a fixed marker if
// the system RNG is unavailable (should never happen in practice).
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(b[:])
}
