package order

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

type routeVoucher struct {
	scope    promotionv1.VoucherScope
	sellerID string
	discount int64
	minSpend int64
}

type routePromo struct {
	mu       sync.Mutex
	vouchers map[string]routeVoucher
	reserves []*promotionv1.ValidateAndReserveRequest
	released []string
	failWith error
}

func newRoutePromo(v map[string]routeVoucher) *routePromo { return &routePromo{vouchers: v} }

func (p *routePromo) ValidateAndReserve(_ context.Context, in *promotionv1.ValidateAndReserveRequest, _ ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failWith != nil {
		return nil, p.failWith
	}
	p.reserves = append(p.reserves, in)
	v, ok := p.vouchers[in.GetCode()]
	if !ok {
		return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: "voucher not found"}, nil
	}
	if v.scope == promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP && v.sellerID != in.GetSellerId() {
		return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: "voucher belongs to another shop"}, nil
	}
	if in.GetCartSubtotal() < v.minSpend {
		return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: "below minimum spend"}, nil
	}
	return &promotionv1.ValidateAndReserveResponse{Valid: true, DiscountAmount: v.discount, VoucherId: "v_" + in.GetCode(), Scope: v.scope, SellerId: v.sellerID}, nil
}

func (p *routePromo) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func (p *routePromo) ReleaseReservation(_ context.Context, in *promotionv1.ReleaseReservationRequest, _ ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.released = append(p.released, in.GetReservationId())
	return &promotionv1.ReleaseReservationResponse{Released: true}, nil
}

func (p *routePromo) calls() []*promotionv1.ValidateAndReserveRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*promotionv1.ValidateAndReserveRequest(nil), p.reserves...)
}

func (p *routePromo) wasReleased(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.released {
		if r == id {
			return true
		}
	}
	return false
}

func routeSvc(t testing.TB, p PromotionClient) *OrderService {
	return internalSvc(t, WithPromotionClient(p))
}

func twoGroups(a, b int64) []quoteGroupInput {
	return []quoteGroupInput{{sellerID: "A", itemsSubtotal: a}, {sellerID: "B", itemsSubtotal: b}}
}

func routeInput(slots voucherSlots, groups []quoteGroupInput) voucherResolveInput {
	return voucherResolveInput{
		buyerID:               "buyer_1",
		groups:                groups,
		slots:                 slots,
		platformReservationID: "platform:chk_1",
		shopReservationID:     func(seller string) string { return "shop:chk_1:" + seller },
	}
}

func TestShopSlotAcceptsShopScope_MultishopCheckout(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{"SHOPA": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, sellerID: "A", discount: 30000}})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{shops: []shopVoucherSlot{{sellerID: "A", code: "SHOPA"}}}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: shop voucher for a group in the selection: err=%v", err)
	}
	o := out.shops["A"]
	if !o.valid || o.discount != 30000 || o.scope != orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_SHOP || o.reservationID != "shop:chk_1:A" {
		t.Errorf("Multishop checkout: shop voucher for a group in the selection: outcome=%+v", o)
	}
	calls := p.calls()
	if len(calls) != 1 || calls[0].GetSellerId() != "A" || calls[0].GetCartSubtotal() != 300000 || calls[0].GetReservationId() != "shop:chk_1:A" {
		t.Errorf("Multishop checkout: shop voucher is reserved on its own group: calls=%v", calls)
	}
}

func TestShopSlotRefusesPlatformScope_MultishopCheckout_APlatformCodeInAShopSlotIsRefused(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{"PLAT": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, discount: 60000}})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{shops: []shopVoucherSlot{{sellerID: "A", code: "PLAT"}}}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: A platform code in a shop slot is refused: err=%v", err)
	}
	o := out.shops["A"]
	if o.valid || o.reason == "" || o.discount != 0 {
		t.Errorf("Multishop checkout: A platform code in a shop slot is refused: outcome=%+v", o)
	}
	if !p.wasReleased("shop:chk_1:A") {
		t.Errorf("Multishop checkout: A platform code in a shop slot is refused: the hold placed for the wrong scope must be released, released=%v", p.released)
	}
}

func TestShopVoucherSellerNotInSelectionRefused_MultishopCheckout(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{"SHOPZ": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, sellerID: "Z", discount: 30000}})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{shops: []shopVoucherSlot{{sellerID: "Z", code: "SHOPZ"}}}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: A shop voucher for a shop outside the selection is refused: err=%v", err)
	}
	o := out.shops["Z"]
	if o.valid || o.reason != "voucher does not apply to this checkout" {
		t.Errorf("Multishop checkout: A shop voucher for a shop outside the selection is refused: outcome=%+v", o)
	}
	if n := len(p.calls()); n != 0 {
		t.Errorf("Multishop checkout: A shop voucher for a shop outside the selection is refused: no promotion call expected, got %d", n)
	}
}

func TestPlatformVoucherMinSpendOnSelectionSubtotal_MultishopCheckout(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{"PLAT": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, discount: 60000, minSpend: 500000}})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{platformCode: "PLAT"}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: Platform min spend is checked on the whole selection: err=%v", err)
	}
	if !out.platform.valid || out.platform.discount != 60000 || out.platform.scope != orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_PLATFORM || out.platform.reservationID != "platform:chk_1" {
		t.Errorf("Multishop checkout: Platform min spend is checked on the whole selection: outcome=%+v", out.platform)
	}
	calls := p.calls()
	if len(calls) != 1 {
		t.Fatalf("Multishop checkout: Platform min spend is checked on the whole selection: want exactly 1 call, got %d", len(calls))
	}
	if calls[0].GetCartSubtotal() != 600000 || calls[0].GetSellerId() != "" || calls[0].GetReservationId() != "platform:chk_1" {
		t.Errorf("Multishop checkout: Platform min spend is checked on the whole selection: request=%v", calls[0])
	}
}

func TestPlatformSlotRefusesShopScope_MultishopCheckout_AShopCodeInThePlatformSlotIsRefused(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{"SHOPB": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, sellerID: "", discount: 30000}})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{platformCode: "SHOPB"}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: A shop code in the platform slot is refused: err=%v", err)
	}
	if out.platform.valid || out.platform.reason == "" || out.platform.discount != 0 {
		t.Errorf("Multishop checkout: A shop code in the platform slot is refused: outcome=%+v", out.platform)
	}
	if !p.wasReleased("platform:chk_1") {
		t.Errorf("Multishop checkout: A shop code in the platform slot is refused: the wrong-scope hold must be released, released=%v", p.released)
	}
}

func TestLegacyVoucherCodePlatformScopeRouted_MultishopCheckout(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{"PLAT": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, discount: 60000}})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{legacyCode: "PLAT"}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: A legacy platform code becomes the platform voucher: err=%v", err)
	}
	if !out.platform.valid || out.platform.code != "PLAT" || out.platform.discount != 60000 || out.platform.reservationID != "platform:chk_1" {
		t.Errorf("Multishop checkout: A legacy platform code becomes the platform voucher: outcome=%+v", out.platform)
	}
	calls := p.calls()
	if len(calls) != 1 || calls[0].GetSellerId() != "" || calls[0].GetCartSubtotal() != 600000 {
		t.Errorf("Multishop checkout: A legacy platform code becomes the platform voucher: calls=%v", calls)
	}
	if len(out.shops) != 0 {
		t.Errorf("Multishop checkout: A legacy platform code becomes the platform voucher: no shop outcome expected, got %v", out.shops)
	}
}

func TestLegacyVoucherCodeShopScopeRefused_MultishopCheckout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seller string
	}{{"shop in selection", "B"}, {"shop outside selection", "Z"}} {
		t.Run(tc.name, func(t *testing.T) {
			p := newRoutePromo(map[string]routeVoucher{"SHOPX": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, sellerID: tc.seller, discount: 30000}})
			out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
				voucherSlots{legacyCode: "SHOPX"}, twoGroups(300000, 300000)))
			if err != nil {
				t.Fatalf("Multishop checkout: A legacy shop code is refused as a platform voucher: err=%v", err)
			}
			if out.platform.valid || out.platform.reason == "" || out.platform.discount != 0 {
				t.Errorf("Multishop checkout: A legacy shop code is refused as a platform voucher: outcome=%+v", out.platform)
			}
			if len(out.shops) != 0 {
				t.Errorf("Multishop checkout: A legacy shop code is refused as a platform voucher: no shop discount expected, got %v", out.shops)
			}
			if n := len(p.calls()); n != 1 {
				t.Errorf("Multishop checkout: A legacy shop code is refused as a platform voucher: exactly one ValidateAndReserve (no seller probe), got %d", n)
			}
		})
	}
}

func TestLegacyVoucherCodeUnknownRefused_MultishopCheckout(t *testing.T) {
	p := newRoutePromo(map[string]routeVoucher{})
	out, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(
		voucherSlots{legacyCode: "NOPE"}, twoGroups(300000, 300000)))
	if err != nil {
		t.Fatalf("Multishop checkout: An unknown legacy code is refused: err=%v", err)
	}
	if out.platform.valid || out.platform.reason != "voucher not found" || out.platform.discount != 0 {
		t.Errorf("Multishop checkout: An unknown legacy code is refused: outcome=%+v", out.platform)
	}
	if n := len(p.calls()); n != 1 {
		t.Errorf("Multishop checkout: An unknown legacy code is refused: want 1 call, got %d", n)
	}
}

func TestVoucherTransportErrorUnavailable_MultishopCheckout(t *testing.T) {
	for _, tc := range []struct {
		name  string
		slots voucherSlots
	}{
		{"platform slot", voucherSlots{platformCode: "PLAT"}},
		{"shop slot", voucherSlots{shops: []shopVoucherSlot{{sellerID: "A", code: "SHOPA"}}}},
		{"legacy slot", voucherSlots{legacyCode: "PLAT"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newRoutePromo(nil)
			p.failWith = errors.New("connection refused")
			_, err := routeSvc(t, p).resolveVouchers(context.Background(), routeInput(tc.slots, twoGroups(300000, 300000)))
			if !errors.Is(err, ErrPromotionUnavailable) {
				t.Errorf("Multishop checkout: A promotion outage blocks a voucher checkout: want ErrPromotionUnavailable, got %v", err)
			}
			if errors.Is(err, ErrVoucherRejected) {
				t.Errorf("Multishop checkout: A promotion outage blocks a voucher checkout: an outage is not a voucher refusal")
			}
		})
	}
}
