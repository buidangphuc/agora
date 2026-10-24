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

type scopeFixture struct {
	vc       promotionv1.VoucherServiceClient
	vouchers store.VoucherRepository
	holds    store.ReservationRepository
}

func startScopeServer(t *testing.T, wrap func(store.VoucherRepository) store.VoucherRepository) scopeFixture {
	t.Helper()
	var vouchers store.VoucherRepository = storefake.NewInMemoryVoucherRepository()
	if wrap != nil {
		vouchers = wrap(vouchers)
	}
	r := startRig(t, rigOpts{vouchers: vouchers})
	return scopeFixture{vc: r.vc, vouchers: vouchers, holds: r.reservations}
}

func (f scopeFixture) seed(t *testing.T, code string, scope promotionv1.VoucherScope, seller string) {
	t.Helper()
	now := time.Now()
	if _, err := f.vouchers.Create(t.Context(), store.Voucher{
		Code: code, Scope: int32(scope), SellerID: seller,
		DiscountType: int32(promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT), DiscountValue: 10,
		Quota: 5, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, voucherRow); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func (f scopeFixture) hasHold(t *testing.T, id string) bool {
	t.Helper()
	_, err := f.holds.GetByReservationID(t.Context(), id)
	return err == nil
}

// Scenario: The order service receives the scope
func TestValidateAndReserveHandlerEchoesScope(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "SHOPV", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, "seller-1")
	resp, err := f.vc.ValidateAndReserve(orderSvcCtx(t), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "SHOPV", BuyerId: "b1", CartSubtotal: 1000, SellerId: "seller-1",
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if !resp.GetValid() {
		t.Fatalf("valid=false reason=%q", resp.GetReason())
	}
	if resp.GetScope() != promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP || resp.GetSellerId() != "seller-1" {
		t.Errorf("scope=%v seller=%q, want SHOP/seller-1", resp.GetScope(), resp.GetSellerId())
	}
}

// Scenario: The order service receives the scope (rejection path keeps scope)
func TestValidateAndReserveHandlerEchoesScopeOnRejection(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "SHOPV", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, "seller-1")
	resp, err := f.vc.ValidateAndReserve(orderSvcCtx(t), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "SHOPV", BuyerId: "b1", CartSubtotal: 1000, SellerId: "seller-2",
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if resp.GetValid() || resp.GetScope() != promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP || resp.GetSellerId() != "seller-1" {
		t.Errorf("valid=%v scope=%v seller=%q, want false/SHOP/seller-1", resp.GetValid(), resp.GetScope(), resp.GetSellerId())
	}
}

// Scenario: A buyer preview receives the scope
func TestValidateAndReservePreviewEchoesScope(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	resp, err := f.vc.ValidateAndReserve(authCtx(t, "buyer-1", "listing.read"), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "preview:buyer-1:abc", Code: "PLAT", BuyerId: "someone-else", CartSubtotal: 1000,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !resp.GetValid() || resp.GetScope() != promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM || resp.GetSellerId() != "" {
		t.Errorf("valid=%v scope=%v seller=%q, want true/PLATFORM/empty", resp.GetValid(), resp.GetScope(), resp.GetSellerId())
	}
	hold, err := f.holds.GetByReservationID(t.Context(), "preview:buyer-1:abc")
	if err != nil {
		t.Fatalf("hold missing: %v", err)
	}
	if hold.BuyerID != "buyer-1" {
		t.Errorf("hold buyer = %q, want principal buyer-1", hold.BuyerID)
	}
}

// Scenario: A buyer outside the preview namespace is denied
func TestValidateAndReservePreviewNamespaceDenied(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	for _, id := range []string{"preview:buyer-2:abc", "order-1"} {
		_, err := f.vc.ValidateAndReserve(authCtx(t, "buyer-1", "listing.read"), &promotionv1.ValidateAndReserveRequest{
			ReservationId: id, Code: "PLAT", BuyerId: "buyer-1", CartSubtotal: 1000,
		})
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: got %v, want PermissionDenied", id, code(err))
		}
		if f.hasHold(t, id) {
			t.Errorf("%s: hold was created", id)
		}
	}
}

// Scenario: A service without the reserve scope is denied
func TestValidateAndReserveServiceWithoutScopeDenied(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	_, err := f.vc.ValidateAndReserve(principalCtx(t, "service-x", "service", "listing.read"), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "PLAT", BuyerId: "b1", CartSubtotal: 1000,
	})
	if code(err) != codes.PermissionDenied {
		t.Errorf("got %v, want PermissionDenied", code(err))
	}
	if f.hasHold(t, "o1") {
		t.Error("hold was created")
	}
}

// Scenario: An anonymous caller is unauthenticated
func TestValidateAndReserveAnonymousUnauthenticated(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	_, err := f.vc.ValidateAndReserve(principalCtx(t, "anonymous", "anonymous"), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "PLAT", BuyerId: "b1", CartSubtotal: 1000,
	})
	if code(err) != codes.Unauthenticated {
		t.Errorf("got %v, want Unauthenticated", code(err))
	}
	if f.hasHold(t, "o1") {
		t.Error("hold was created")
	}
}

// Scenario: An empty reservation id is rejected
func TestValidateAndReserveEmptyReservationIDInvalid(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	_, err := f.vc.ValidateAndReserve(orderSvcCtx(t), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "", Code: "PLAT", BuyerId: "b1", CartSubtotal: 1000,
	})
	if code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", code(err))
	}
	if f.hasHold(t, "") {
		t.Error("hold was created")
	}
}

type failingVouchers struct{ store.VoucherRepository }

var errVoucherStore = errors.New("pq: connection refused to 10.0.0.5:5432")

func (failingVouchers) GetByCode(context.Context, string) (store.Voucher, error) {
	return store.Voucher{}, errVoucherStore
}

func (failingVouchers) GetByID(context.Context, string) (store.Voucher, error) {
	return store.Voucher{}, errVoucherStore
}

// Scenario: A store failure is a generic internal error
func TestValidateAndReserveStoreFailureInternal(t *testing.T) {
	f := startScopeServer(t, func(r store.VoucherRepository) store.VoucherRepository {
		return failingVouchers{r}
	})
	_, err := f.vc.ValidateAndReserve(orderSvcCtx(t), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "ANY", BuyerId: "b1", CartSubtotal: 1000,
	})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" || strings.Contains(st.Message(), "pq") {
		t.Errorf("code=%v message=%q, want Internal / %q", st.Code(), st.Message(), "internal error")
	}
}
