package interceptor_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-audit/internal/interceptor"
)

func resolve(t *testing.T, md metadata.MD) (interceptor.Principal, bool) {
	t.Helper()
	ctx := context.Background()
	if md != nil {
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	var (
		p  interceptor.Principal
		ok bool
	)
	_, err := interceptor.Unary()(ctx, nil, &grpc.UnaryServerInfo{}, func(c context.Context, _ any) (any, error) {
		p, ok = interceptor.PrincipalFromContext(c)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, ok
}

func TestPrincipalResolution(t *testing.T) {
	p, ok := resolve(t, metadata.Pairs("x-principal-id", "u1", "x-principal-type", "user", "x-principal-scopes", "a, admin"))
	if !ok || p.ID != "u1" || p.Anonymous() || p.IsService() || !p.HasScope("admin") || !p.HasScope("a") {
		t.Fatalf("user: %+v ok=%v", p, ok)
	}
	p, _ = resolve(t, metadata.Pairs("x-principal-id", "svc", "x-principal-type", "service"))
	if !p.IsService() {
		t.Fatalf("service: %+v", p)
	}
	for name, md := range map[string]metadata.MD{
		"gateway anonymous": metadata.Pairs("x-principal-id", "anonymous", "x-principal-type", "anonymous"),
		"anonymous type":    metadata.Pairs("x-principal-id", "u1", "x-principal-type", "anonymous"),
	} {
		p, ok := resolve(t, md)
		if !ok || !p.Anonymous() {
			t.Fatalf("%s: %+v ok=%v", name, p, ok)
		}
		if _, err := interceptor.RequirePrincipal(interceptor.ContextWithPrincipal(context.Background(), p)); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: RequirePrincipal got %v", name, err)
		}
	}
	if _, ok := resolve(t, nil); ok {
		t.Fatal("no metadata must yield no principal")
	}
	if _, err := interceptor.RequireAdmin(context.Background()); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("RequireAdmin without principal: %v", err)
	}
}

func TestRequireService(t *testing.T) {
	mk := func(p interceptor.Principal) context.Context {
		return interceptor.ContextWithPrincipal(context.Background(), p)
	}
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"none", context.Background(), codes.Unauthenticated},
		{"anonymous", mk(interceptor.Principal{ID: "anonymous", Type: "anonymous"}), codes.Unauthenticated},
		{"admin user", mk(interceptor.Principal{ID: "a", Type: "user", Scopes: []string{"admin", "audit.write"}}), codes.PermissionDenied},
		{"service no scope", mk(interceptor.Principal{ID: "s", Type: "service"}), codes.PermissionDenied},
		{"service ok", mk(interceptor.Principal{ID: "s", Type: "service", Scopes: []string{"audit.write"}}), codes.OK},
	}
	for _, c := range cases {
		if _, err := interceptor.RequireService(c.ctx, interceptor.ScopeAuditWrite); status.Code(err) != c.want {
			t.Fatalf("%s: got %v want %v", c.name, status.Code(err), c.want)
		}
	}
}
