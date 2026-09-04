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

const mdRequestID = "x-request-id"

type requestIDCtxKey struct{}

func OTelStatsHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDCtxKey{}).(string)
	return id, ok
}

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

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(b[:])
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
