package order_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

type msVoucher struct {
	scope    promotionv1.VoucherScope
	sellerID string
	discount int64
}

type msPromo struct {
	mu       sync.Mutex
	catalog  map[string]msVoucher
	requests []*promotionv1.ValidateAndReserveRequest
	active   map[string]bool
	released []string
}

func newMsPromo(c map[string]msVoucher) *msPromo {
	return &msPromo{catalog: c, active: map[string]bool{}}
}

func (p *msPromo) ValidateAndReserve(_ context.Context, in *promotionv1.ValidateAndReserveRequest, _ ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, in)
	v, ok := p.catalog[in.GetCode()]
	if !ok {
		return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: "voucher not found"}, nil
	}
	if v.scope == promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP && v.sellerID != in.GetSellerId() {
		return &promotionv1.ValidateAndReserveResponse{Valid: false, Reason: "voucher belongs to another shop"}, nil
	}
	p.active[in.GetReservationId()] = true
	return &promotionv1.ValidateAndReserveResponse{Valid: true, DiscountAmount: v.discount, VoucherId: "v_" + in.GetCode(), Scope: v.scope, SellerId: v.sellerID}, nil
}

func (p *msPromo) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func (p *msPromo) ReleaseReservation(_ context.Context, in *promotionv1.ReleaseReservationRequest, _ ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, in.GetReservationId())
	p.released = append(p.released, in.GetReservationId())
	return &promotionv1.ReleaseReservationResponse{Released: true}, nil
}

func (p *msPromo) heldIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var ids []string
	for id := range p.active {
		ids = append(ids, id)
	}
	return ids
}

func (p *msPromo) wasReleased(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.released {
		if r == id {
			return true
		}
	}
	return false
}

func (p *msPromo) reserveCalls() []*promotionv1.ValidateAndReserveRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*promotionv1.ValidateAndReserveRequest(nil), p.requests...)
}

func msCatalog() map[string]msVoucher {
	return map[string]msVoucher{
		"SHOPA": {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, sellerID: "sa", discount: 500},
		"PLAT":  {scope: promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, discount: 1000},
	}
}

// twoSellerRig groups: sa = 2 x 1000 = 2000, sb = 1 x 2000 = 2000, Ha Noi shipping 20000 each.
func TestRunCheckoutPlacesQuotedFigures_MultishopCheckout(t *testing.T) {
	promo := newMsPromo(msCatalog())
	r := twoSellerRig(t, 5, order.WithPromotionClient(promo))

	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "",
		order.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}}, "PLAT"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SellerID != "sa" || got[1].SellerID != "sb" {
		t.Fatalf("want one order per seller sorted, got %+v", got)
	}
	want := map[string]struct{ fee, discount, platform, total int64 }{
		"sa": {20000, 1000, 500, 21000},
		"sb": {20000, 500, 500, 21500},
	}
	checkoutID := r.sagas.lastSagaID()
	for _, o := range got {
		w := want[o.SellerID]
		if o.ShippingFee != w.fee || o.DiscountAmount != w.discount || o.PlatformDiscountAmount != w.platform || o.TotalAmount != w.total {
			t.Fatalf("seller %s: fee=%d discount=%d platform=%d total=%d, want %+v", o.SellerID, o.ShippingFee, o.DiscountAmount, o.PlatformDiscountAmount, o.TotalAmount, w)
		}
		if o.PlatformVoucherCode != "PLAT" {
			t.Fatalf("seller %s platform_voucher_code=%q", o.SellerID, o.PlatformVoucherCode)
		}
		if o.CheckoutID == "" || o.CheckoutID != checkoutID {
			t.Fatalf("seller %s checkout_id=%q, want saga id %q", o.SellerID, o.CheckoutID, checkoutID)
		}
	}
	var platformCalls, shopCalls int
	for _, c := range promo.reserveCalls() {
		switch c.GetCode() {
		case "PLAT":
			platformCalls++
			if c.GetReservationId() != "platform:"+checkoutID || c.GetSellerId() != "" || c.GetCartSubtotal() != 4000 {
				t.Fatalf("platform reserve = %+v", c)
			}
		case "SHOPA":
			shopCalls++
			if c.GetSellerId() != "sa" || c.GetCartSubtotal() != 2000 {
				t.Fatalf("shop reserve = %+v", c)
			}
		}
	}
	if platformCalls != 1 || shopCalls != 1 {
		t.Fatalf("want one platform and one shop reserve, got %d and %d", platformCalls, shopCalls)
	}
	if held := promo.heldIDs(); len(held) != 2 {
		t.Fatalf("both holds must stay for the placed orders, got %v", held)
	}
}

func TestRunCheckoutNoVoucherUnchanged_MultishopCheckout(t *testing.T) {
	promo := newMsPromo(msCatalog())
	r := twoSellerRig(t, 5, order.WithPromotionClient(promo))

	got, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 orders, got %d", len(got))
	}
	for _, o := range got {
		if o.DiscountAmount != 0 || o.PlatformVoucherCode != "" || o.PlatformDiscountAmount != 0 {
			t.Fatalf("seller %s: discount=%d platform=%q/%d", o.SellerID, o.DiscountAmount, o.PlatformVoucherCode, o.PlatformDiscountAmount)
		}
		if o.TotalAmount != o.ItemsSubtotal+o.ShippingFee || o.ItemsSubtotal != 2000 {
			t.Fatalf("seller %s: total=%d items=%d fee=%d", o.SellerID, o.TotalAmount, o.ItemsSubtotal, o.ShippingFee)
		}
		if o.CheckoutID == "" || o.CheckoutID != r.sagas.lastSagaID() {
			t.Fatalf("seller %s checkout_id=%q", o.SellerID, o.CheckoutID)
		}
	}
	if n := len(promo.reserveCalls()); n != 0 {
		t.Fatalf("promotion must not be called without vouchers, got %d", n)
	}
}

func assertAbortedAndNothingHeld(t *testing.T, r *checkoutRig, promo *msPromo, err error) {
	t.Helper()
	if !errors.Is(err, order.ErrVoucherRejected) {
		t.Fatalf("want ErrVoucherRejected, got %v", err)
	}
	if n := len(r.buyerOrders(t)); n != 0 {
		t.Fatalf("no order may exist, got %d", n)
	}
	if r.domain.Stock("lst_a") != 10 || r.domain.Stock("lst_b") != 5 {
		t.Fatalf("stock holds must be released, a=%d b=%d", r.domain.Stock("lst_a"), r.domain.Stock("lst_b"))
	}
	if r.cart.removeCalled {
		t.Fatal("the cart must stay intact")
	}
	if held := promo.heldIDs(); len(held) != 0 {
		t.Fatalf("voucher holds left behind: %v", held)
	}
	if st := r.sagaStatus(t); st != store.SagaStatusCompensated {
		t.Fatalf("saga status %v, want COMPENSATED", st)
	}
}

func TestCreateOrderShopVoucherWrongShopAbortsAndReleases_MultishopCheckout(t *testing.T) {
	promo := newMsPromo(msCatalog())
	r := twoSellerRig(t, 5, order.WithPromotionClient(promo))

	_, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "",
		order.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}, {SellerId: "sb", Code: "SHOPA"}}, "PLAT"))
	if err == nil || !strings.Contains(err.Error(), "voucher belongs to another shop") {
		t.Fatalf("want promotion's reason in the error, got %v", err)
	}
	assertAbortedAndNothingHeld(t, r, promo, err)
	if len(promo.reserveCalls()) < 2 {
		t.Fatal("the valid holds must have been taken first, otherwise this test proves nothing")
	}
}

func TestCreateOrderPlatformVoucherRefusedAbortsAndReleases_MultishopCheckout(t *testing.T) {
	promo := newMsPromo(msCatalog())
	r := twoSellerRig(t, 5, order.WithPromotionClient(promo))

	_, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "",
		order.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}}, "NOPE"))
	if err == nil || !strings.Contains(err.Error(), "voucher not found") {
		t.Fatalf("want promotion's reason in the error, got %v", err)
	}
	assertAbortedAndNothingHeld(t, r, promo, err)
	var shopHeld bool
	for _, c := range promo.reserveCalls() {
		if c.GetCode() == "SHOPA" {
			shopHeld = promo.wasReleased(c.GetReservationId())
		}
	}
	if !shopHeld {
		t.Fatal("the shop voucher hold taken before the refusal must have been released")
	}
}

func TestFailAndCompensateReleasesAllVoucherHolds_MultishopCheckout(t *testing.T) {
	promo := newMsPromo(msCatalog())
	promo.catalog["SHOPB"] = msVoucher{scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, sellerID: "sb", discount: 300}
	orders := fake.NewInMemoryOrderRepository()
	inner := fake.NewInMemorySagaRepository()
	r := twoSellerRig(t, 5)
	r.orders = orders
	r.sagas = &recordingSagaRepo{SagaRepository: inner}
	r.svc = newSvc(t, orders, r.cart, nil, nil, r.domain, nil, nil,
		order.WithSagaRepository(r.sagas),
		order.WithPromotionClient(promo),
		order.WithOrderPlacer(faultyPlacer{inner: fake.NewInMemoryOrderPlacer(orders, inner), mode: "before"}))

	_, err := r.svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "",
		order.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}, {SellerId: "sb", Code: "SHOPB"}}, "PLAT"))
	if err == nil {
		t.Fatal("placement fails, so the checkout must fail")
	}
	if n := len(r.buyerOrders(t)); n != 0 {
		t.Fatalf("no order may exist, got %d", n)
	}
	if held := promo.heldIDs(); len(held) != 0 {
		t.Fatalf("voucher holds left behind: %v", held)
	}
	reserved := map[string]bool{}
	for _, c := range promo.reserveCalls() {
		reserved[c.GetReservationId()] = true
	}
	if len(reserved) != 3 {
		t.Fatalf("want 3 distinct holds (two shop, one platform), got %v", reserved)
	}
	platformID := "platform:" + r.sagas.lastSagaID()
	if !reserved[platformID] || !promo.wasReleased(platformID) {
		t.Fatalf("platform hold %q must be reserved and released", platformID)
	}
	for id := range reserved {
		if !promo.wasReleased(id) {
			t.Fatalf("hold %q was not released", id)
		}
	}
	if r.domain.Stock("lst_a") != 10 || r.domain.Stock("lst_b") != 5 {
		t.Fatalf("stock holds must be released, a=%d b=%d", r.domain.Stock("lst_a"), r.domain.Stock("lst_b"))
	}
}
