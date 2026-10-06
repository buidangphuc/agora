package interceptor

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestInterceptorTreatsGatewayAnonymousAsAnonymous(t *testing.T) {
	cases := []struct {
		name string
		md   metadata.MD
		want Principal
	}{
		{"user", metadata.Pairs("x-principal-id", "u-1", "x-principal-type", "user"), Principal{UserID: "u-1"}},
		{"gateway anonymous", metadata.Pairs("x-principal-id", "anonymous", "x-principal-type", "anonymous"), Principal{Anonymous: true}},
		{"anonymous type with an id", metadata.Pairs("x-principal-id", "u-1", "x-principal-type", "anonymous"), Principal{Anonymous: true}},
		{"anonymous id without a type", metadata.Pairs("x-principal-id", "anonymous"), Principal{Anonymous: true}},
		{"no metadata", metadata.MD{}, Principal{Anonymous: true}},
	}
	for _, c := range cases {
		got := FromContext(interceptorWithPrincipal(metadata.NewIncomingContext(context.Background(), c.md)))
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
