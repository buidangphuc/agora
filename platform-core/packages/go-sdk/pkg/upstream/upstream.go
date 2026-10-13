package upstream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

const (
	// DefaultTimeout bounds one unary call when WithTimeout is not given.
	DefaultTimeout = 2 * time.Second

	// deadlineMargin is what a bounded call leaves of the parent deadline for
	// the caller's own remaining work after the call gives up.
	deadlineMargin = 500 * time.Millisecond

	servicePrincipalType = "service"
)

type options struct {
	timeout     time.Duration
	service     *auth.Principal // nil: forward the incoming principal
	dialOptions []grpc.DialOption
}

// Option configures Dial.
type Option func(*options)

// AsService makes every call present the service principal id with scopes,
// replacing whatever principal the incoming request carried.
func AsService(id string, scopes ...string) Option {
	return func(o *options) {
		o.service = &auth.Principal{ID: id, Type: servicePrincipalType, Scopes: scopes}
	}
}

// servicePrincipalKey is unexported so only WithServicePrincipal can set it and
// nothing read from incoming metadata can.
type servicePrincipalKey struct{}

// WithServicePrincipal returns a context whose upstream calls present the service
// principal id with scopes. It wins over a connection-level AsService, which in
// turn wins over forwarding the incoming principal.
func WithServicePrincipal(ctx context.Context, id string, scopes ...string) context.Context {
	return context.WithValue(ctx, servicePrincipalKey{}, auth.Principal{ID: id, Type: servicePrincipalType, Scopes: scopes})
}

// WithTimeout sets the per-call bound for unary calls (default DefaultTimeout).
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithDialOptions appends opts after the SDK's own dial options, so they can
// add to or override the defaults (extra interceptors, service config, ...).
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(o *options) { o.dialOptions = append(o.dialOptions, opts...) }
}

// Dial creates a lazily connecting plaintext gRPC client with otelgrpc, the
// request id forwarded, and the principal forwarded or replaced (AsService).
func Dial(addr string, opts ...Option) (*grpc.ClientConn, error) {
	o := options{timeout: DefaultTimeout}
	for _, opt := range opts {
		opt(&o)
	}
	dial := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(o.unary()),
		grpc.WithChainStreamInterceptor(o.stream()),
	}, o.dialOptions...)
	conn, err := grpc.NewClient(addr, dial...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, nil
}

// Bounded derives the context for one upstream call: its deadline is the
// earlier of d from now and the parent's deadline minus a 500ms margin. When
// the parent is already inside the margin the context is already expired and
// the call fails at once. Run follow-up work on the parent ctx, not this one.
func Bounded(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl) - deadlineMargin; rem < d {
			d = max(rem, 0)
		}
	}
	return context.WithTimeout(ctx, d)
}

func (o options) unary() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, cancel := Bounded(o.outgoing(ctx), o.timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (o options) stream() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(o.outgoing(ctx), desc, cc, method, opts...)
	}
}

// outgoing returns ctx with the request id and principal set on the outgoing
// metadata, keeping any other metadata the caller attached.
func (o options) outgoing(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	if id := reqctx.RequestID(ctx); id != "" {
		md.Set(reqctx.MetadataRequestID, id)
	}
	p, ok := o.principal(ctx)
	if ok {
		md.Set(auth.MDPrincipalID, p.ID)
		md.Set(auth.MDPrincipalType, p.Type)
		if len(p.Scopes) > 0 {
			md.Set(auth.MDPrincipalScopes, strings.Join(p.Scopes, ","))
		} else {
			md.Delete(auth.MDPrincipalScopes)
		}
	} else {
		md.Delete(auth.MDPrincipalID)
		md.Delete(auth.MDPrincipalType)
		md.Delete(auth.MDPrincipalScopes)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func (o options) principal(ctx context.Context) (auth.Principal, bool) {
	if p, ok := ctx.Value(servicePrincipalKey{}).(auth.Principal); ok {
		return p, true
	}
	if o.service != nil {
		return *o.service, true
	}
	p, ok := auth.FromContext(ctx)
	if !ok || p.Anonymous() {
		return auth.Principal{}, false
	}
	return p, true
}
