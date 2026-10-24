package rpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

type holdFixture struct {
	fc        promotionv1.FlashSaleServiceClient
	campaigns *storefake.InMemoryFlashSaleRepository
	rig       *rig
}

func startHoldServer(t *testing.T, wrap func(*storefake.InMemoryFlashHoldRepository) store.FlashHoldRepository) holdFixture {
	t.Helper()
	r := startRig(t, rigOpts{holds: wrap})
	return holdFixture{fc: r.fc, campaigns: r.campaigns, rig: r}
}

func seedCampaign(t *testing.T, r *rig, variant string, stockCap int64) store.FlashSaleCampaign {
	t.Helper()
	c, err := r.campaigns.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: "l1", VariantID: variant, SalePrice: 90000, StockCap: stockCap,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour),
	}, campaignRow)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return c
}

func (f holdFixture) seed(t *testing.T, variant string, stockCap int64) store.FlashSaleCampaign {
	t.Helper()
	return seedCampaign(t, f.rig, variant, stockCap)
}

func (f holdFixture) remaining(t *testing.T, c store.FlashSaleCampaign) int64 {
	t.Helper()
	r, err := f.fc.GetFlashSaleStock(authCtx(t, "buyer-1", "listing.read"), &promotionv1.GetFlashSaleStockRequest{CampaignId: c.ID})
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	return r.GetRemaining()
}

func reserveReq(id string, c store.FlashSaleCampaign, qty int64) *promotionv1.ReserveFlashSaleStockRequest {
	return &promotionv1.ReserveFlashSaleStockRequest{
		ReservationId: id, CampaignId: c.ID, ListingId: c.ListingID, VariantId: c.VariantID,
		Quantity: qty, ExpectedSalePrice: c.SalePrice,
	}
}

func orderSvc(t *testing.T) context.Context {
	t.Helper()
	return orderSvcCtx(t)
}

// Scenarios: A buyer cannot reserve flash stock; A user carrying the service scope is still
// refused; A service without the scope is refused; An anonymous caller is unauthenticated.
func TestFlashHoldRPCsAreServiceOnly(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	if r, err := f.fc.ReserveFlashSaleStock(orderSvc(t), reserveReq("o1", c, 2)); err != nil || !r.GetReserved() {
		t.Fatalf("service reserve: %v %+v", err, r)
	}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", principalCtx(t, "anonymous", "anonymous"), codes.Unauthenticated},
		{"no principal", t.Context(), codes.Unauthenticated},
		{"buyer", authCtx(t, "buyer-1"), codes.PermissionDenied},
		{"user with promotion.reserve and admin", authCtx(t, "buyer-1", "promotion.reserve", "admin"), codes.PermissionDenied},
		{"service without scope", principalCtx(t, "service-other", "service", "listing.read"), codes.PermissionDenied},
	} {
		if _, err := f.fc.ReserveFlashSaleStock(tc.ctx, reserveReq("x-"+tc.name, c, 1)); code(err) != tc.want {
			t.Errorf("reserve %s: got %v, want %v", tc.name, code(err), tc.want)
		}
		if _, err := f.fc.CommitFlashSaleStock(tc.ctx, &promotionv1.CommitFlashSaleStockRequest{ReservationId: "o1"}); code(err) != tc.want {
			t.Errorf("commit %s: got %v, want %v", tc.name, code(err), tc.want)
		}
		if _, err := f.fc.ReleaseFlashSaleStock(tc.ctx, &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: "o1"}); code(err) != tc.want {
			t.Errorf("release %s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
	// Refused calls changed nothing: the hold of 2 is still there and still commits.
	if got := f.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
	if r, err := f.fc.CommitFlashSaleStock(orderSvc(t), &promotionv1.CommitFlashSaleStockRequest{ReservationId: "o1"}); err != nil || !r.GetCommitted() {
		t.Fatalf("service commit: %v %+v", err, r)
	}
	if r, err := f.fc.ReleaseFlashSaleStock(orderSvc(t), &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: "o1"}); err != nil || !r.GetReleased() {
		t.Fatalf("service release: %v %+v", err, r)
	}
}

// Scenarios: Reserve rejects malformed input; Commit rejects an empty reservation id;
// Release rejects an empty reservation id.
func TestFlashHoldRPCsRejectInvalidInput(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	mut := func(fn func(*promotionv1.ReserveFlashSaleStockRequest)) *promotionv1.ReserveFlashSaleStockRequest {
		r := reserveReq("r1", c, 1)
		fn(r)
		return r
	}
	for name, req := range map[string]*promotionv1.ReserveFlashSaleStockRequest{
		"empty reservation id": mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.ReservationId = "" }),
		"empty campaign id":    mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.CampaignId = "" }),
		"empty listing id":     mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.ListingId = "" }),
		"quantity 0":           mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.Quantity = 0 }),
		"quantity -1":          mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.Quantity = -1 }),
	} {
		resp, err := f.fc.ReserveFlashSaleStock(orderSvc(t), req)
		if code(err) != codes.InvalidArgument {
			t.Errorf("reserve %s: got %v, want InvalidArgument", name, code(err))
		}
		if resp.GetReason() != "" {
			t.Errorf("reserve %s: reason %q, want none", name, resp.GetReason())
		}
	}
	if got := f.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5 (no hold created)", got)
	}
	for name, id := range map[string]string{"empty": "", "201 bytes": strings.Repeat("a", 201)} {
		if _, err := f.fc.CommitFlashSaleStock(orderSvc(t), &promotionv1.CommitFlashSaleStockRequest{ReservationId: id}); code(err) != codes.InvalidArgument {
			t.Errorf("commit %s: got %v, want InvalidArgument", name, code(err))
		}
		if _, err := f.fc.ReleaseFlashSaleStock(orderSvc(t), &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: id}); code(err) != codes.InvalidArgument {
			t.Errorf("release %s: got %v, want InvalidArgument", name, code(err))
		}
	}
}

// Scenario: A reservation id of 200 bytes is accepted and 201 is rejected.
func TestFlashHoldReservationIDLengthBoundary(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	r, err := f.fc.ReserveFlashSaleStock(orderSvc(t), reserveReq(strings.Repeat("a", 200), c, 1))
	if err != nil || !r.GetReserved() || r.GetReason() != "" {
		t.Fatalf("200 bytes: %v %+v, want reserved with empty reason", err, r)
	}
	if _, err := f.fc.ReserveFlashSaleStock(orderSvc(t), reserveReq(strings.Repeat("a", 201), c, 1)); code(err) != codes.InvalidArgument {
		t.Fatalf("201 bytes: got %v, want InvalidArgument", code(err))
	}
}

// Scenario: An empty variant id is valid.
func TestFlashHoldEmptyVariantAccepted(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	req := reserveReq("r1", c, 1)
	req.VariantId = ""
	if r, err := f.fc.ReserveFlashSaleStock(orderSvc(t), req); err != nil || !r.GetReserved() {
		t.Fatalf("%v %+v, want reserved=true", err, r)
	}
}

// Scenario: Stock of an unknown campaign is not found.
func TestGetFlashSaleStockUnknownNotFound(t *testing.T) {
	f := startHoldServer(t, nil)
	_, err := f.fc.GetFlashSaleStock(authCtx(t, "buyer-1", "listing.read"), &promotionv1.GetFlashSaleStockRequest{CampaignId: "nope"})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", code(err))
	}
}

// failingHolds makes every hold-store call fail with a revealing error.
type failingHolds struct {
	*storefake.InMemoryFlashHoldRepository
}

var errHoldStore = errors.New("pq: connection refused to 10.0.0.5:5432")

func (failingHolds) Reserve(context.Context, store.FlashHold, time.Time) (store.FlashHold, bool, error) {
	return store.FlashHold{}, false, errHoldStore
}

func (failingHolds) Get(context.Context, string) (store.FlashHold, error) {
	return store.FlashHold{}, errHoldStore
}

func (failingHolds) Commit(context.Context, string, time.Time) (bool, error) {
	return false, errHoldStore
}

func (failingHolds) Release(context.Context, string, time.Time) (bool, error) {
	return false, errHoldStore
}

func (failingHolds) ActiveHeld(context.Context, string, time.Time) (int64, error) {
	return 0, errHoldStore
}

// Scenario: A store failure is a generic internal error.
func TestFlashHoldStoreFailureIsInternal(t *testing.T) {
	f := startHoldServer(t, func(m *storefake.InMemoryFlashHoldRepository) store.FlashHoldRepository {
		return failingHolds{m}
	})
	c := f.seed(t, "", 5)
	check := func(op string, err error) {
		t.Helper()
		st, _ := status.FromError(err)
		if st.Code() != codes.Internal || st.Message() != "internal error" || strings.Contains(st.Message(), "pq") {
			t.Errorf("%s: code=%v message=%q, want Internal / %q", op, st.Code(), st.Message(), "internal error")
		}
	}
	_, err := f.fc.ReserveFlashSaleStock(orderSvc(t), reserveReq("r1", c, 1))
	check("reserve", err)
	_, err = f.fc.CommitFlashSaleStock(orderSvc(t), &promotionv1.CommitFlashSaleStockRequest{ReservationId: "r1"})
	check("commit", err)
	_, err = f.fc.ReleaseFlashSaleStock(orderSvc(t), &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: "r1"})
	check("release", err)
}
