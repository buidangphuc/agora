package edge_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
)

func (f *bearerFixture) callSearch(t *testing.T, hdr map[string]string) *upstreamSpy {
	t.Helper()
	f.search.md = nil
	c := searchv1connect.NewSearchServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&searchv1.SearchListingsRequest{})
	for k, v := range hdr {
		req.Header().Set(k, v)
	}
	if _, err := c.SearchListings(context.Background(), req); err != nil {
		t.Fatalf("call: %v", err)
	}
	return f.search
}

func TestForwardedPrincipalSessionID(t *testing.T) {
	exp := time.Now().Add(time.Hour)

	t.Run("Verified token sid is forwarded as x-principal-sid", func(t *testing.T) {
		f := newBearerFixture(t)
		spy := f.callSearch(t, map[string]string{"Authorization": "Bearer " + mintSID(t, f.key, "kid-1", exp, "sid-verified")})
		if got := spy.md.Get("x-principal-sid"); len(got) != 1 || got[0] != "sid-verified" {
			t.Fatalf("x-principal-sid = %v, want [sid-verified]", got)
		}
	})

	t.Run("Client-sent x-principal-sid is replaced by the verified claim", func(t *testing.T) {
		f := newBearerFixture(t)
		spy := f.callSearch(t, map[string]string{
			"Authorization":   "Bearer " + mintSID(t, f.key, "kid-1", exp, "sid-verified"),
			"X-Principal-Sid": "sid-forged",
		})
		if got := spy.md.Get("x-principal-sid"); len(got) != 1 || got[0] != "sid-verified" {
			t.Fatalf("x-principal-sid = %v, want [sid-verified]", got)
		}
	})

	t.Run("Client-sent x-principal-sid is not forwarded when the token has no sid", func(t *testing.T) {
		f := newBearerFixture(t)
		spy := f.callSearch(t, map[string]string{
			"Authorization":   "Bearer " + mint(t, f.key, "kid-1", exp),
			"X-Principal-Sid": "sid-forged",
		})
		if got := spy.md.Get("x-principal-sid"); len(got) != 0 {
			t.Fatalf("x-principal-sid = %v, want none", got)
		}
	})

	t.Run("Anonymous caller cannot inject x-principal-sid", func(t *testing.T) {
		f := newBearerFixture(t)
		spy := f.callSearch(t, map[string]string{"X-Principal-Sid": "sid-forged"})
		if got := spy.md.Get("x-principal-sid"); len(got) != 0 {
			t.Fatalf("x-principal-sid = %v, want none", got)
		}
	})
}
