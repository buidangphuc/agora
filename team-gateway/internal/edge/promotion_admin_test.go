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

const (
	deactivateVoucherPath = "/platform.promotion.v1.VoucherService/DeactivateVoucher"
	updateVoucherPath     = "/platform.promotion.v1.VoucherService/UpdateVoucher"
)

type voucherAdminUpstream struct {
	promotionv1.VoucherServiceClient
	deactivateCalls atomic.Int64
	updateCalls     atomic.Int64
	err             error
	gotDeactivate   atomic.Pointer[promotionv1.DeactivateVoucherRequest]
	gotUpdate       atomic.Pointer[promotionv1.UpdateVoucherRequest]
	gotPID          atomic.Value
}

func (v *voucherAdminUpstream) record(ctx context.Context) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if p := md.Get("x-principal-id"); len(p) > 0 {
			v.gotPID.Store(p[0])
		}
	}
}

func (v *voucherAdminUpstream) DeactivateVoucher(ctx context.Context, in *promotionv1.DeactivateVoucherRequest, _ ...grpc.CallOption) (*promotionv1.DeactivateVoucherResponse, error) {
	v.deactivateCalls.Add(1)
	v.gotDeactivate.Store(in)
	v.record(ctx)
	if v.err != nil {
		return nil, v.err
	}
	return &promotionv1.DeactivateVoucherResponse{}, nil
}

func (v *voucherAdminUpstream) UpdateVoucher(ctx context.Context, in *promotionv1.UpdateVoucherRequest, _ ...grpc.CallOption) (*promotionv1.UpdateVoucherResponse, error) {
	v.updateCalls.Add(1)
	v.gotUpdate.Store(in)
	v.record(ctx)
	if v.err != nil {
		return nil, v.err
	}
	return &promotionv1.UpdateVoucherResponse{}, nil
}

func (v *voucherAdminUpstream) total() int64 { return v.deactivateCalls.Load() + v.updateCalls.Load() }

func callVoucherAdmin(t *testing.T, up *voucherAdminUpstream, path, body string, scopes []string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Voucher: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if scopes != nil {
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

var voucherAdminCalls = []struct{ name, path, body string }{
	{"DeactivateVoucher", deactivateVoucherPath, `{"voucher_id":"v-1"}`},
	{"UpdateVoucher", updateVoucherPath, `{"voucher_id":"v-1","quota":150}`},
}

// Scenario V7 / V7a: anonymous callers are UNAUTHENTICATED and promotion gets no call.
func TestV7_AnonymousCannotStopOrUpdate_NoUpstreamCall(t *testing.T) {
	for _, c := range voucherAdminCalls {
		up := &voucherAdminUpstream{}
		code, body := callVoucherAdmin(t, up, c.path, c.body, nil)
		if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
			t.Errorf("%s anonymous = %d %s, want 401 unauthenticated", c.name, code, body)
		}
		if n := up.total(); n != 0 {
			t.Errorf("%s anonymous reached promotion %d times, want 0", c.name, n)
		}
	}
}

// Scenario V6 / V7b / V17: non-admin tokens (buyer, seller) are PERMISSION_DENIED at the edge.
func TestV6_V7b_V17_NonAdminDeniedAtEdge_NoUpstreamCall(t *testing.T) {
	for _, c := range voucherAdminCalls {
		for _, who := range []struct {
			name   string
			scopes []string
		}{
			{"buyer", []string{"listing.read"}},
			{"seller", []string{"listing.read", "seller"}},
		} {
			up := &voucherAdminUpstream{}
			code, body := callVoucherAdmin(t, up, c.path, c.body, who.scopes)
			if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
				t.Errorf("%s %s = %d %s, want 403 permission_denied", who.name, c.name, code, body)
			}
			if n := up.total(); n != 0 {
				t.Errorf("%s %s reached promotion %d times, want 0", who.name, c.name, n)
			}
		}
	}
}

// Scenario V6 (admin side): an admin DeactivateVoucher is forwarded unaltered with the caller principal.
func TestV6_AdminDeactivateVoucherForwarded(t *testing.T) {
	up := &voucherAdminUpstream{}
	code, body := callVoucherAdmin(t, up, deactivateVoucherPath, `{"voucher_id":"v-1"}`, []string{"admin", "promotion.admin"})
	if code != http.StatusOK {
		t.Fatalf("admin DeactivateVoucher = %d %s, want 200 (not 501)", code, body)
	}
	if n := up.deactivateCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
	if g := up.gotDeactivate.Load(); g.GetVoucherId() != "v-1" {
		t.Fatalf("forwarded request altered: %v", g)
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Fatalf("principal id on upstream call = %q, want user-1", pid)
	}
}

// Scenario V17 (admin side): an admin UpdateVoucher is forwarded unaltered with the caller principal.
func TestV17_AdminUpdateVoucherForwarded(t *testing.T) {
	up := &voucherAdminUpstream{}
	code, body := callVoucherAdmin(t, up, updateVoucherPath, `{"voucher_id":"v-1","quota":150}`, []string{"admin", "promotion.admin"})
	if code != http.StatusOK {
		t.Fatalf("admin UpdateVoucher = %d %s, want 200 (not 501)", code, body)
	}
	if n := up.updateCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
	g := up.gotUpdate.Load()
	if g.GetVoucherId() != "v-1" || g.Quota == nil || g.GetQuota() != 150 || g.EndsAt != nil {
		t.Fatalf("forwarded request altered: %v", g)
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Fatalf("principal id on upstream call = %q, want user-1", pid)
	}
}

// Scenario V6/V17 write path: upstream failures are never retried and codes pass through.
func TestV6_V17_VoucherAdminWritesNotRetried(t *testing.T) {
	for _, c := range voucherAdminCalls {
		up := &voucherAdminUpstream{err: status.Error(codes.Unavailable, "promotion down")}
		code, body := callVoucherAdmin(t, up, c.path, c.body, []string{"admin", "promotion.admin"})
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d %s, want 503", c.name, code, body)
		}
		if n := up.total(); n != 1 {
			t.Errorf("%s upstream calls = %d, want exactly 1 (no retry)", c.name, n)
		}
	}
}

// Scenario V6/V17: domain errors from promotion reach the admin unchanged.
func TestV6_V17_VoucherAdminDomainErrorsPassThrough(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		http int
		text string
	}{
		{codes.NotFound, http.StatusNotFound, "not_found"},
		{codes.FailedPrecondition, http.StatusBadRequest, "failed_precondition"},
	} {
		for _, c := range voucherAdminCalls {
			up := &voucherAdminUpstream{err: status.Error(tc.code, "x")}
			code, body := callVoucherAdmin(t, up, c.path, c.body, []string{"admin", "promotion.admin"})
			if code != tc.http || !strings.Contains(body, tc.text) {
				t.Errorf("%s %v = %d %s, want %d %s", c.name, tc.code, code, body, tc.http, tc.text)
			}
		}
	}
}
