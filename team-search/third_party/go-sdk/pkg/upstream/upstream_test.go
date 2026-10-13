package upstream

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// withBufconn routes the client to an in-memory listener; test-only.
func withBufconn(lis *bufconn.Listener) Option {
	return func(o *options) {
		o.dialOptions = append(o.dialOptions, grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	}
}

// echoServer returns the metadata it received as response headers.
func echoServer(t *testing.T) *bufconn.Listener {
	t.Helper()
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, ss grpc.ServerStream) error {
		md, _ := metadata.FromIncomingContext(ss.Context())
		if err := ss.SetHeader(md); err != nil {
			return err
		}
		if err := ss.RecvMsg(new(wrapperspb.StringValue)); err != nil {
			return err
		}
		return ss.SendMsg(wrapperspb.String("ok"))
	}))
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis
}

func invoke(ctx context.Context, t *testing.T, opts ...Option) metadata.MD {
	t.Helper()
	conn, err := NewClient("passthrough:///bufnet", opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var hdr metadata.MD
	err = conn.Invoke(ctx, "/test.v1.Test/Echo", wrapperspb.String("x"), new(wrapperspb.StringValue), grpc.Header(&hdr))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return hdr
}

func buyerCtx() context.Context {
	ctx := reqctx.WithRequestID(context.Background(), "req-1")
	return auth.WithPrincipal(ctx, auth.Principal{ID: "buyer-9", Type: "user", Scopes: []string{"order.write"}})
}

func TestForwardsRequestIDAndPrincipal(t *testing.T) {
	hdr := invoke(buyerCtx(), t, withBufconn(echoServer(t)))

	for key, want := range map[string]string{
		reqctx.MetadataRequestID: "req-1",
		auth.MDPrincipalID:       "buyer-9",
		auth.MDPrincipalType:     "user",
		auth.MDPrincipalScopes:   "order.write",
	} {
		if got := hdr.Get(key); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want [%s]", key, got, want)
		}
	}
}

func TestAsServiceReplacesPrincipal(t *testing.T) {
	hdr := invoke(buyerCtx(), t, withBufconn(echoServer(t)), AsService("team-order", "listing.read"))

	for key, want := range map[string]string{
		reqctx.MetadataRequestID: "req-1",
		auth.MDPrincipalID:       "team-order",
		auth.MDPrincipalType:     "service",
		auth.MDPrincipalScopes:   "listing.read",
	} {
		if got := hdr.Get(key); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want [%s]", key, got, want)
		}
	}
	for _, vals := range hdr {
		for _, v := range vals {
			if v == "buyer-9" || v == "order.write" {
				t.Errorf("buyer identity leaked upstream: %v", hdr)
			}
		}
	}
}

func TestBoundedRespectsParentDeadline(t *testing.T) {
	deadline := func(ctx context.Context) time.Duration {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("no deadline")
		}
		return time.Until(dl)
	}

	// No parent deadline: d applies.
	c, cancel := Bounded(context.Background(), 2*time.Second)
	if got := deadline(c); got > 2*time.Second || got < time.Second {
		t.Errorf("no parent: deadline in %v, want about 2s", got)
	}
	cancel()

	// Parent 1s out, d 5s: parent minus the 500ms margin wins.
	parent, pc := context.WithTimeout(context.Background(), time.Second)
	defer pc()
	c, cancel = Bounded(parent, 5*time.Second)
	if got := deadline(c); got > 500*time.Millisecond {
		t.Errorf("parent 1s: deadline in %v, want at most 500ms", got)
	}
	cancel()

	// Parent inside the margin: already expired.
	tight, tc := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer tc()
	c, cancel = Bounded(tight, 5*time.Second)
	defer cancel()
	if c.Err() == nil && deadline(c) > 0 {
		t.Errorf("parent inside margin: want expired context, deadline in %v", deadline(c))
	}
}

func TestPerCallServicePrincipal(t *testing.T) {
	conn, err := NewClient("passthrough:///bufnet", withBufconn(echoServer(t)))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	call := func(ctx context.Context) metadata.MD {
		t.Helper()
		var hdr metadata.MD
		if err := conn.Invoke(ctx, "/test.v1.Test/Echo", wrapperspb.String("x"), new(wrapperspb.StringValue), grpc.Header(&hdr)); err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		return hdr
	}
	want := func(hdr metadata.MD, id, typ, scopes string) {
		t.Helper()
		for key, w := range map[string]string{
			auth.MDPrincipalID: id, auth.MDPrincipalType: typ, auth.MDPrincipalScopes: scopes,
		} {
			if got := hdr.Get(key); len(got) != 1 || got[0] != w {
				t.Errorf("%s = %v, want [%s]", key, got, w)
			}
		}
	}

	want(call(WithServicePrincipal(buyerCtx(), "team-order", "listing.read")), "team-order", "service", "listing.read")
	want(call(buyerCtx()), "buyer-9", "user", "order.write")
}

func TestPerCallServicePrincipalBeatsAsService(t *testing.T) {
	ctx := WithServicePrincipal(buyerCtx(), "per-call", "a.read")
	hdr := invoke(ctx, t, withBufconn(echoServer(t)), AsService("conn-level", "b.read"))
	if got := hdr.Get(auth.MDPrincipalID); len(got) != 1 || got[0] != "per-call" {
		t.Errorf("principal id = %v, want [per-call]", got)
	}
}

func TestPerCallServicePrincipalIgnoresIncomingMetadata(t *testing.T) {
	// A caller-supplied incoming principal header must never become a per-call principal.
	ctx := metadata.NewIncomingContext(buyerCtx(), metadata.Pairs(auth.MDPrincipalID, "spoof", auth.MDPrincipalType, "service"))
	hdr := invoke(ctx, t, withBufconn(echoServer(t)))
	if got := hdr.Get(auth.MDPrincipalID); len(got) != 1 || got[0] != "buyer-9" {
		t.Errorf("principal id = %v, want [buyer-9]", got)
	}
}

func TestDialOptionsApplied(t *testing.T) {
	var seen []string
	icpt := grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		seen = append(seen, method)
		return invoker(ctx, method, req, reply, cc, opts...)
	})
	invoke(buyerCtx(), t, withBufconn(echoServer(t)), WithDialOptions(icpt))
	if len(seen) != 1 || seen[0] != "/test.v1.Test/Echo" {
		t.Errorf("interceptor saw %v, want one call to /test.v1.Test/Echo", seen)
	}
}

func TestDefaultsToRoundRobin(t *testing.T) {
	var hits [2]atomic.Int64
	listeners := map[string]*bufconn.Listener{}
	var addrs []resolver.Address
	for i := range hits {
		srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, ss grpc.ServerStream) error {
			hits[i].Add(1)
			if err := ss.RecvMsg(new(wrapperspb.StringValue)); err != nil {
				return err
			}
			return ss.SendMsg(wrapperspb.String("ok"))
		}))
		lis := bufconn.Listen(1 << 20)
		go func() { _ = srv.Serve(lis) }()
		t.Cleanup(srv.Stop)
		addr := "backend-" + string(rune('a'+i))
		listeners[addr] = lis
		addrs = append(addrs, resolver.Address{Addr: addr})
	}
	r := manual.NewBuilderWithScheme("rr")
	r.InitialState(resolver.State{Addresses: addrs})

	conn, err := NewClient("rr:///svc", WithDialOptions(
		grpc.WithResolvers(r),
		grpc.WithContextDialer(func(_ context.Context, addr string) (net.Conn, error) { return listeners[addr].Dial() }),
	))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for range 200 {
		if hits[0].Load() > 0 && hits[1].Load() > 0 {
			break
		}
		// WaitForReady rides out the window before both subchannels are connected.
		err := conn.Invoke(context.Background(), "/test.v1.Test/Echo", wrapperspb.String("x"), new(wrapperspb.StringValue), grpc.WaitForReady(true))
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
	}
	if hits[0].Load() == 0 || hits[1].Load() == 0 {
		t.Errorf("calls per backend = %d and %d, want both used (round_robin)", hits[0].Load(), hits[1].Load())
	}
}
