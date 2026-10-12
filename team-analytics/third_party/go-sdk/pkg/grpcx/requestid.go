package grpcx

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// requestIDFrom returns the incoming x-request-id, or a new uuid when the
// caller sent none.
func requestIDFrom(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(reqctx.MetadataRequestID); len(v) > 0 && v[0] != "" {
			return v[0]
		}
	}
	return uuid.NewString()
}

func requestIDHeader(id string) metadata.MD {
	return metadata.Pairs(reqctx.MetadataRequestID, id)
}

func requestIDUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := requestIDFrom(ctx)
		ctx = reqctx.WithRequestID(ctx, id)
		// Headers are best effort: a failure here must not fail the call.
		_ = grpc.SetHeader(ctx, requestIDHeader(id))
		return handler(ctx, req)
	}
}

func requestIDStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		id := requestIDFrom(ss.Context())
		_ = ss.SetHeader(requestIDHeader(id))
		return handler(srv, &ctxStream{ServerStream: ss, ctx: reqctx.WithRequestID(ss.Context(), id)})
	}
}

// ctxStream overrides Context so the handler sees the enriched context.
type ctxStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *ctxStream) Context() context.Context { return s.ctx }
