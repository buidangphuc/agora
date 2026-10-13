package reqctx

import "context"

// MetadataRequestID is the gRPC metadata key that carries the request id.
const MetadataRequestID = "x-request-id"

// maxRequestIDLen bounds an accepted request id.
const maxRequestIDLen = 128

// ValidRequestID reports whether id is 1..128 characters of [A-Za-z0-9._-].
// A request id that arrives over the wire (gRPC metadata, a Kafka header) ends
// up in logs and response headers, so callers accept it only when this holds.
func ValidRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := range len(id) {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

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
