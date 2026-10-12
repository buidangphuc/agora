package auth_test

import (
	"context"
	"reflect"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

func incoming(kv ...string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(kv...))
}

// run passes ctx through the unary interceptor and returns the handler's ctx.
func run(ctx context.Context, t *testing.T) context.Context {
	t.Helper()
	var got context.Context
	_, err := auth.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) { got = c; return nil, nil })
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	return got
}

func TestRequirePrincipalRejectsAnonymous(t *testing.T) {
	cases := map[string]context.Context{
		"no metadata":    run(context.Background(), t),
		"anonymous id":   run(incoming(auth.MDPrincipalID, "anonymous", auth.MDPrincipalType, "user"), t),
		"anonymous type": run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalType, "anonymous"), t),
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := auth.RequirePrincipal(ctx); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("code = %v, want Unauthenticated (err=%v)", status.Code(err), err)
			}
		})
	}

	p, err := auth.RequirePrincipal(run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalType, "user"), t))
	if err != nil || p.ID != "u1" {
		t.Fatalf("authenticated principal = %+v, err = %v", p, err)
	}
}

func TestRequireScopesExactMatchNoAdminBypass(t *testing.T) {
	ctx := run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalType, "user", auth.MDPrincipalScopes, "admin"), t)
	_, err := auth.RequireScopes(ctx, "order.admin")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}

	ctx = run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalScopes, "order.read,order.write"), t)
	if _, err := auth.RequireScopes(ctx, "order.read", "order.write"); err != nil {
		t.Fatalf("all scopes held: %v", err)
	}
	if _, err := auth.RequireScopes(ctx, "order.read", "order"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("partial match must be denied, got %v", err)
	}

	if _, err := auth.RequireScopes(context.Background(), "x"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no principal: code = %v, want Unauthenticated", status.Code(err))
	}
	anon := run(incoming(auth.MDPrincipalID, "anonymous", auth.MDPrincipalScopes, "x"), t)
	if _, err := auth.RequireScopes(anon, "x"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestStatusMessagesDoNotEchoMetadata(t *testing.T) {
	ctx := run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalScopes, "a"), t)
	_, err := auth.RequireScopes(ctx, "secret.scope")
	if got := status.Convert(err).Message(); got != "missing scope" {
		t.Fatalf("message = %q", got)
	}
	_, err = auth.RequirePrincipal(context.Background())
	if got := status.Convert(err).Message(); got != "authentication required" {
		t.Fatalf("message = %q", got)
	}
}

func TestUnaryInterceptorSetsPrincipalAndReqctx(t *testing.T) {
	ctx := run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalType, "service", auth.MDPrincipalScopes, "a"), t)
	p, ok := auth.FromContext(ctx)
	if !ok || p.ID != "u1" || p.Type != "service" || !p.HasScope("a") {
		t.Fatalf("principal = %+v ok=%v", p, ok)
	}
	if got := reqctx.PrincipalID(ctx); got != "u1" {
		t.Fatalf("reqctx principal = %q", got)
	}
}

func TestNoPrincipalHeaderLeavesContextUntouched(t *testing.T) {
	ctx := run(incoming(auth.MDPrincipalType, "user"), t)
	if _, ok := auth.FromContext(ctx); ok {
		t.Fatal("principal must not be set without x-principal-id")
	}
	if got := reqctx.PrincipalID(ctx); got != "" {
		t.Fatalf("reqctx principal = %q", got)
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeStream) Context() context.Context { return f.ctx }

func TestStreamInterceptorInjectsPrincipal(t *testing.T) {
	ss := fakeStream{ctx: incoming(auth.MDPrincipalID, "u9", auth.MDPrincipalScopes, "s")}
	var seen context.Context
	err := auth.StreamServerInterceptor()(nil, ss, &grpc.StreamServerInfo{},
		func(_ any, stream grpc.ServerStream) error { seen = stream.Context(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	p, ok := auth.FromContext(seen)
	if !ok || p.ID != "u9" || !p.HasScope("s") {
		t.Fatalf("principal = %+v ok=%v", p, ok)
	}
	if got := reqctx.PrincipalID(seen); got != "u9" {
		t.Fatalf("reqctx principal = %q", got)
	}
}

func TestScopesParsingTrimsAndDropsEmpty(t *testing.T) {
	ctx := run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalScopes, " a, ,b ,,c"), t)
	p, _ := auth.FromContext(ctx)
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(p.Scopes, want) {
		t.Fatalf("scopes = %q, want %q", p.Scopes, want)
	}
	ctx = run(incoming(auth.MDPrincipalID, "u1"), t)
	p, _ = auth.FromContext(ctx)
	if len(p.Scopes) != 0 {
		t.Fatalf("scopes = %q, want empty", p.Scopes)
	}
}

func TestWithPrincipalSetsReqctx(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "u2"})
	if got := reqctx.PrincipalID(ctx); got != "u2" {
		t.Fatalf("reqctx principal = %q", got)
	}
}
