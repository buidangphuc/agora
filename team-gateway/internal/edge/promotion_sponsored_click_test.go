package edge_test

import (
	"context"
	"crypto/rsa"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"

	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/promotion/v1/promotionv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/revocation"
)

type spySponsored struct {
	promotionv1.SponsoredServiceClient
	spy   *upstreamSpy
	token string
}

func (s *spySponsored) RecordAdClick(ctx context.Context, in *promotionv1.RecordAdClickRequest, _ ...grpc.CallOption) (*promotionv1.RecordAdClickResponse, error) {
	s.spy.record(ctx)
	s.token = in.GetImpressionToken()
	return &promotionv1.RecordAdClickResponse{ListingId: "lst-1", Charged: true}, nil
}

type sponsoredClickFixture struct {
	srv *httptest.Server
	spy *upstreamSpy
	up  *spySponsored
	key *rsa.PrivateKey
}

func newSponsoredClickFixture(t *testing.T) *sponsoredClickFixture {
	t.Helper()
	bf := newBearerFixture(t)
	spy := &upstreamSpy{}
	up := &spySponsored{spy: spy}
	e := edge.NewEdge(bf.verifier, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000).
		WithRevocations(revocation.NewDenylist())
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.DiscardHandler))...)
	mux := http.NewServeMux()
	p, h := promotionv1connect.NewSponsoredServiceHandler(edge.NewSponsoredForwarder(up, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &sponsoredClickFixture{srv: srv, spy: spy, up: up, key: bf.key}
}

func (f *sponsoredClickFixture) click(auth string) (*promotionv1.RecordAdClickResponse, error) {
	c := promotionv1connect.NewSponsoredServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&promotionv1.RecordAdClickRequest{ImpressionToken: "tok-1"})
	if auth != "" {
		req.Header().Set("Authorization", auth)
	}
	resp, err := c.RecordAdClick(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestRecordAdClickForwardsAnonymous(t *testing.T) {
	f := newSponsoredClickFixture(t)
	resp, err := f.click("")
	if err != nil {
		t.Fatalf("anonymous RecordAdClick: %v", err)
	}
	if !resp.GetCharged() || resp.GetListingId() != "lst-1" {
		t.Errorf("response = %v, want upstream answer passed through", resp)
	}
	if f.spy.calls != 1 || f.up.token != "tok-1" {
		t.Fatalf("upstream calls=%d token=%q, want 1 and tok-1", f.spy.calls, f.up.token)
	}
	if v := f.spy.md.Get("x-principal-id"); len(v) != 1 || v[0] != "anonymous" {
		t.Errorf("principal id = %v, want anonymous", v)
	}
}

func TestRecordAdClickForwardsPrincipal(t *testing.T) {
	f := newSponsoredClickFixture(t)
	auth := "Bearer " + mint(t, f.key, "kid-1", time.Now().Add(time.Hour))
	if _, err := f.click(auth); err != nil {
		t.Fatalf("buyer RecordAdClick: %v", err)
	}
	if f.spy.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", f.spy.calls)
	}
	if v := f.spy.md.Get("x-principal-id"); len(v) != 1 || v[0] != "user-1" {
		t.Errorf("principal id = %v, want user-1", v)
	}
}

func TestRecordAdClickRejectsBadBearer(t *testing.T) {
	f := newSponsoredClickFixture(t)
	_, err := f.click("Bearer garbage")
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v (%v), want Unauthenticated", connect.CodeOf(err), err)
	}
	if f.spy.calls != 0 {
		t.Errorf("malformed bearer must not reach team-promotion, got %d calls", f.spy.calls)
	}
}
