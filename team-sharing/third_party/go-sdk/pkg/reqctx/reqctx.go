package reqctx

import "context"

// MetadataRequestID is the gRPC metadata key that carries the request id.
const MetadataRequestID = "x-request-id"

type (
	requestIDKey   struct{}
	principalIDKey struct{}
)

// WithRequestID returns ctx carrying the request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request id in ctx, or "" when absent.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithPrincipalID returns ctx carrying the principal id.
func WithPrincipalID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, principalIDKey{}, id)
}

// PrincipalID returns the principal id in ctx, or "" when absent.
func PrincipalID(ctx context.Context) string {
	id, _ := ctx.Value(principalIDKey{}).(string)
	return id
}
