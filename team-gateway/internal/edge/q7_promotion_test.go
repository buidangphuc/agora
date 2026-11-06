package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const listMyVouchersPath = "/platform.promotion.v1.VoucherService/ListMyVouchers"

type q7MyVouchers struct {
	promotionv1.VoucherServiceClient
	calls     atomic.Int64
	updates   atomic.Int64
	err       error
	principal atomic.Value
	ptype     atomic.Value
}

func (s *q7MyVouchers) ListMyVouchers(ctx context.Context, _ *promotionv1.ListMyVouchersRequest, _ ...grpc.CallOption) (*promotionv1.ListMyVouchersResponse, error) {
	s.calls.Add(1)
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if v := md.Get("x-principal-id"); len(v) > 0 {
			s.principal.Store(v[0])
		}
		if v := md.Get("x-principal-type"); len(v) > 0 {
			s.ptype.Store(v[0])
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return &promotionv1.ListMyVouchersResponse{Vouchers: []*promotionv1.Voucher{{Code: "MINE"}}}, nil
}

func (s *q7MyVouchers) UpdateVoucher(context.Context, *promotionv1.UpdateVoucherRequest, ...grpc.CallOption) (*promotionv1.UpdateVoucherResponse, error) {
	s.updates.Add(1)
	return &promotionv1.UpdateVoucherResponse{}, nil
}

func callMyVouchers(t *testing.T, up *q7MyVouchers, path string, scopes []string, signedIn bool) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Voucher: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	if signedIn {
		req.Header.Set("Authorization", "Bearer "+f.token(t, scopes...))
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Scenario: An anonymous caller is refused
func TestListMyVouchersAnonymous(t *testing.T) {
	up := &q7MyVouchers{}
	code, body := callMyVouchers(t, up, listMyVouchersPath, nil, false)
	if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Fatalf("anonymous ListMyVouchers = %d %s, want 401 unauthenticated", code, body)
	}
	if n := up.calls.Load(); n != 0 {
		t.Fatalf("team-promotion received %d calls for an anonymous caller, want 0", n)
	}
}

// Scenario: Any signed-in principal needs no scope
func TestAdminProceduresSetUnchanged(t *testing.T) {
	for _, scopes := range [][]string{nil, {"listing.read"}, {"listing.read", "seller"}} {
		up := &q7MyVouchers{}
		code, body := callMyVouchers(t, up, listMyVouchersPath, scopes, true)
		if code != http.StatusOK || !strings.Contains(body, "MINE") {
			t.Fatalf("ListMyVouchers with scopes %v = %d %s, want 200 with the upstream list", scopes, code, body)
		}
		if n := up.calls.Load(); n != 1 {
			t.Fatalf("scopes %v: upstream calls = %d, want 1", scopes, n)
		}
		if pid, _ := up.principal.Load().(string); pid != "user-1" {
			t.Fatalf("scopes %v: x-principal-id on the upstream call = %q, want user-1", scopes, pid)
		}
	}

	up := &q7MyVouchers{}
	code, body := callMyVouchers(t, up, updateVoucherPath, []string{"listing.read"}, true)
	if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") || up.updates.Load() != 0 {
		t.Fatalf("buyer UpdateVoucher = %d %s (upstream calls %d), want 403 permission_denied and no upstream call", code, body, up.updates.Load())
	}
}

// Scenario: Any signed-in principal needs no scope (upstream failures keep their code)
func TestListMyVouchersUpstreamCodePassesThrough(t *testing.T) {
	up := &q7MyVouchers{err: status.Error(codes.Unavailable, "promotion down")}
	code, body := callMyVouchers(t, up, listMyVouchersPath, []string{"listing.read"}, true)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("ListMyVouchers with promotion down = %d %s, want 503", code, body)
	}
}

// Scenario: HasCompletedOrder is not routed at the gateway
func TestHasCompletedOrderNotRouted(t *testing.T) {
	f := newCockpitFixture(t)
	ord := &q7HasCompletedOrder{}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Order: ord, Voucher: &q7MyVouchers{}}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform.order.v1.OrderService/HasCompletedOrder", strings.NewReader(`{"buyer_id":"user-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token(t, "listing.read"))
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusNotImplemented && res.StatusCode != http.StatusNotFound {
		t.Fatalf("HasCompletedOrder with a buyer token = %d %s, want 501 or 404", res.StatusCode, b)
	}
	if n := ord.calls.Load(); n != 0 {
		t.Fatalf("team-order received %d HasCompletedOrder calls, want 0", n)
	}
}
