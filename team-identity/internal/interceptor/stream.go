package interceptor

import (
	"context"

	"google.golang.org/grpc"
)

// wrappedStream overrides Context() so an augmented context propagates to a
// streaming handler (used by the request-id interceptor).
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
