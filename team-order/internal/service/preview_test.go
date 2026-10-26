package service_test

// ---- order-multishop-checkout task 6: PreviewCheckout service ----
//
// Interface fixed by these tests (implementer provides, package service):
//   - func (s *OrderService) PreviewCheckout(ctx, buyerID, addressID string,
//     itemIDs []string, opts ...CheckoutOption) (*orderv1.PreviewCheckoutResponse, error)
//     Vouchers arrive through the existing WithVouchers option. The address is
//     resolved inside the service from its identity client: empty addressID = the
//     buyer's default; no addresses at all = shipping_quoted=false.
//   - var ErrAddressNotFound ("address not found") and ErrAddressUnavailable
//     (identity unreachable or no identity client), errors.Is-comparable.
//   - Existing errors reused: ErrEmptyCart, ErrSelfPurchase, ErrPriceChanged.
//   - Voucher holds are taken under "preview:<buyer id>:<random>" and released.
//   - The compile-only stub preview_teststub.go is replaced by the implementation.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type pvAddr struct {
	identityv1.AddressServiceClient
	addrs []*identityv1.Address
	err   error
}

func (a *pvAddr) ListAddresses(context.Context, *identityv1.ListAddressesRequest, ...grpc.CallOption) (*identityv1.ListAddressesResponse, error) {
	if a.err != nil {
		return nil, a.err
	}
	return &identityv1.ListAddressesResponse{Addresses: a.addrs}, nil
}

func pvHome() *identityv1.Address {
	return &identityv1.Address{Id: "addr_home", UserId: "buyer_1", RecipientName: "An", Phone: "0900000001", Street: "1 A St", Ward: "W1", District: "D1", City: "Hà Nội", IsDefault: true}
}

func pvOther() *identityv1.Address {
	return &identityv1.Address{Id: "addr_other", UserId: "buyer_1", RecipientName: "Binh", Phone: "0900000002", Street: "2 B St", Ward: "W2", District: "D2", City: "Đà Nẵng"}
}

type pvPromo struct {
	*msPromo
	cmu        sync.Mutex
	commits    int
	releaseErr error
}

func (p *pvPromo) CommitReservation(ctx context.Context, in *promotionv1.CommitReservationRequest, o ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	p.cmu.Lock()
	p.commits++
	p.cmu.Unlock()
	return p.msPromo.CommitReservation(ctx, in, o...)
}

func (p *pvPromo) ReleaseReservation(ctx context.Context, in *promotionv1.ReleaseReservationRequest, o ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	resp, err := p.msPromo.ReleaseReservation(ctx, in, o...)
	if p.releaseErr != nil {
		return nil, p.releaseErr
	}
	return resp, err
}

func (p *pvPromo) commitCount() int {
	p.cmu.Lock()
	defer p.cmu.Unlock()
	return p.commits
}

func (p *pvPromo) releasedIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.released...)
}

type pvRig struct {
	domain *upstreamtest.Domain
	orders *repository.InMemoryOrderRepository
	sagas  *recordingSagaRepo
	cart   *repository.InMemoryCartRepository
	promo  *pvPromo
	svc    *service.OrderService
}

type pvOpts struct {
	addrs      []*identityv1.Address
	addrErr    error
	buyer      string
	listings   map[string]*listingv1.Listing
	releaseErr error
	lines      []repository.CartItem
}

// pvNew: cart of buyer_1 = ci_a (sa, 2 x 1000), ci_b (sb, 1 x 2000), ci_c (sb, 1 x 1000).
func pvNew(t *testing.T, o pvOpts) *pvRig {
	t.Helper()
	buyer := o.buyer
	if buyer == "" {
		buyer = "buyer_1"
	}
	r := &pvRig{
		domain: upstreamtest.NewDomain(map[string]int32{"lst_a": 10, "lst_b": 10, "lst_c": 10}),
		orders: repository.NewInMemoryOrderRepository(),
		sagas:  &recordingSagaRepo{SagaRepository: repository.NewInMemorySagaRepository()},
		cart:   repository.NewInMemoryCartRepository(),
		promo:  &pvPromo{msPromo: newMsPromo(msCatalog()), releaseErr: o.releaseErr},
	}
	lines := o.lines
	if lines == nil {
		lines = []repository.CartItem{
			{ID: "ci_a", ListingID: "lst_a", Quantity: 2, SellerID: "sa", UnitPrice: 1000},
			{ID: "ci_b", ListingID: "lst_b", Quantity: 1, SellerID: "sb", UnitPrice: 2000},
			{ID: "ci_c", ListingID: "lst_c", Quantity: 1, SellerID: "sb", UnitPrice: 1000},
		}
	}
	for _, l := range lines {
		l.UserID = buyer
		if _, err := r.cart.AddItem(context.Background(), l); err != nil {
			t.Fatal(err)
		}
	}
	inner := r.sagas.SagaRepository.(*repository.InMemorySagaRepository)
	opts := []service.OrderServiceOption{
		service.WithSagaRepository(r.sagas),
		service.WithOrderPlacer(repository.NewInMemoryOrderPlacer(r.orders, inner)),
		service.WithPromotionClient(r.promo),
	}
	ac := &pvAddr{addrs: o.addrs, err: o.addrErr}
	if o.listings != nil {
		r.svc = service.NewOrderService(r.orders, r.cart, nil, nil, &pricedDomain{Domain: r.domain, listings: o.listings}, ac, nil, opts...)
	} else {
		r.svc = service.NewOrderService(r.orders, r.cart, nil, nil, r.domain, ac, nil, opts...)
	}
	return r
}

func (r *pvRig) cartSnapshot(t *testing.T, buyer string) map[string]repository.CartItem {
	t.Helper()
	items, err := r.cart.GetCart(context.Background(), buyer)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]repository.CartItem{}
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

func pvGroup(t *testing.T, resp *orderv1.PreviewCheckoutResponse, seller string) *orderv1.CheckoutGroupQuote {
	t.Helper()
	for _, g := range resp.GetGroups() {
		if g.GetSellerId() == seller {
			return g
		}
	}
	t.Fatalf("no group for seller %s in %v", seller, resp.GetGroups())
	return nil
}

func TestPreviewCheckoutReservesNothing_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{pvHome()}})
	before := r.cartSnapshot(t, "buyer_1")

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil,
		service.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetGroups()) != 2 || resp.GetGroups()[0].GetSellerId() != "sa" || resp.GetGroups()[1].GetSellerId() != "sb" {
		t.Fatalf("want two groups sorted by seller, got %v", resp.GetGroups())
	}
	sa, sb := pvGroup(t, resp, "sa"), pvGroup(t, resp, "sb")
	if sa.GetItemsSubtotal() != 2000 || sa.GetShippingFee() != 20000 || sa.GetShopDiscountAmount() != 500 || sa.GetTotalAmount() != 21500 || sa.GetShopVoucherCode() != "SHOPA" {
		t.Fatalf("group sa = %v", sa)
	}
	if sb.GetItemsSubtotal() != 3000 || sb.GetShippingFee() != 20000 || sb.GetShopDiscountAmount() != 0 || sb.GetTotalAmount() != 23000 {
		t.Fatalf("group sb = %v", sb)
	}
	if resp.GetTotalAmount() != 44500 || resp.GetItemsSubtotal() != 5000 || resp.GetShippingFee() != 40000 || resp.GetDiscountAmount() != 500 {
		t.Fatalf("totals = %v", resp)
	}
	if len(resp.GetVouchers()) != 1 || !resp.GetVouchers()[0].GetValid() || resp.GetVouchers()[0].GetCode() != "SHOPA" || resp.GetVouchers()[0].GetDiscountAmount() != 500 {
		t.Fatalf("vouchers = %v", resp.GetVouchers())
	}

	if n := r.domain.Reservations(); n != 0 {
		t.Fatalf("no stock reservation may exist, got %d", n)
	}
	if ids := r.sagas.sagaIDs(); len(ids) != 0 {
		t.Fatalf("no saga may exist, got %v", ids)
	}
	if os, _ := r.orders.ListBuyerOrders(context.Background(), "buyer_1", 0); len(os) != 0 {
		t.Fatalf("no order may exist, got %d", len(os))
	}
	if held := r.promo.heldIDs(); len(held) != 0 {
		t.Fatalf("no voucher hold may survive, got %v", held)
	}
	after := r.cartSnapshot(t, "buyer_1")
	if len(after) != len(before) {
		t.Fatalf("cart changed: %v -> %v", before, after)
	}
	for id, b := range before {
		if after[id].UnitPrice != b.UnitPrice || after[id].Quantity != b.Quantity {
			t.Fatalf("cart line %s changed: %+v -> %+v", id, b, after[id])
		}
	}
}

func TestPreviewCheckoutSelectsItemIDs_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{pvHome()}})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", []string{"ci_b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetGroups()) != 1 {
		t.Fatalf("want one group, got %v", resp.GetGroups())
	}
	g := resp.GetGroups()[0]
	if g.GetSellerId() != "sb" || len(g.GetItemIds()) != 1 || g.GetItemIds()[0] != "ci_b" || g.GetItemsSubtotal() != 2000 {
		t.Fatalf("group = %v", g)
	}
	if resp.GetItemsSubtotal() != 2000 {
		t.Fatalf("selection subtotal = %d, want 2000", resp.GetItemsSubtotal())
	}
}

func TestPreviewCheckoutEmptySelectionRefused_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{pvHome()}})
	_, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", []string{"nope"})
	if !errors.Is(err, service.ErrEmptyCart) {
		t.Fatalf("want ErrEmptyCart, got %v", err)
	}
}

func TestPreviewCheckoutNoAddressNotQuoted_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{
		addrs: nil,
		lines: []repository.CartItem{{ID: "ci_x", ListingID: "lst_a", Quantity: 1, SellerID: "sa", UnitPrice: 300000}},
	})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetShippingQuoted() || resp.GetShippingFee() != 0 || resp.GetTotalAmount() != 300000 {
		t.Fatalf("quoted=%v fee=%d total=%d", resp.GetShippingQuoted(), resp.GetShippingFee(), resp.GetTotalAmount())
	}
	if len(resp.GetGroups()) != 1 {
		t.Fatalf("groups = %v", resp.GetGroups())
	}
	g := resp.GetGroups()[0]
	if g.GetShippingFee() != 0 || g.GetEstimatedDeliveryAt() != nil || g.GetTotalAmount() != 300000 {
		t.Fatalf("group = %v", g)
	}
	if resp.GetShippingAddress() != nil {
		t.Fatalf("no address to echo, got %v", resp.GetShippingAddress())
	}
}

func TestPreviewCheckoutNoDefaultAddressFailedPrecondition_MultishopCheckout(t *testing.T) {
	a, b := pvOther(), pvOther()
	b.Id = "addr_other2"
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{a, b}})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if !errors.Is(err, service.ErrAddressNotFound) {
		t.Fatalf("want ErrAddressNotFound, got resp=%v err=%v", resp, err)
	}
	if !strings.Contains(err.Error(), "address not found") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestPreviewCheckoutUnknownAddressIDRefused_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{pvHome()}})
	_, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "addr_not_mine", nil)
	if !errors.Is(err, service.ErrAddressNotFound) {
		t.Fatalf("want ErrAddressNotFound, got %v", err)
	}
}

func TestPreviewCheckoutIdentityUnavailable_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrErr: status.Error(codes.Unavailable, "identity down")})

	_, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil,
		service.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}}, "PLAT"))
	if !errors.Is(err, service.ErrAddressUnavailable) {
		t.Fatalf("want ErrAddressUnavailable, got %v", err)
	}
	if held := r.promo.heldIDs(); len(held) != 0 {
		t.Fatalf("no voucher hold may be left, got %v", held)
	}
	if n := len(r.promo.reserveCalls()); n != 0 {
		t.Fatalf("fail closed before touching promotion, got %d reserve calls", n)
	}
}

func TestPreviewCheckoutEchoesShippingAddress_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{pvOther(), pvHome()}})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetShippingQuoted() {
		t.Fatal("shipping must be quoted")
	}
	if !proto.Equal(resp.GetShippingAddress(), pvHome()) {
		t.Fatalf("shipping_address = %v, want the default address", resp.GetShippingAddress())
	}
	for _, g := range resp.GetGroups() {
		if g.GetEstimatedDeliveryAt() == nil || g.GetShippingFee() != 20000 {
			t.Fatalf("group %s eta=%v fee=%d", g.GetSellerId(), g.GetEstimatedDeliveryAt(), g.GetShippingFee())
		}
	}

	resp, err = r.svc.PreviewCheckout(context.Background(), "buyer_1", "addr_other", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(resp.GetShippingAddress(), pvOther()) {
		t.Fatalf("explicit address_id not honoured: %v", resp.GetShippingAddress())
	}
	if resp.GetGroups()[0].GetShippingFee() != 35000 {
		t.Fatalf("Da Nang fee = %d, want 35000", resp.GetGroups()[0].GetShippingFee())
	}
}

func TestPreviewCheckoutPriceChangedRepricesCart_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{
		addrs: []*identityv1.Address{pvHome()},
		listings: map[string]*listingv1.Listing{
			"lst_a": {Id: "lst_a", Price: 1500},
			"lst_b": {Id: "lst_b", Price: 2000},
			"lst_c": {Id: "lst_c", Price: 1000},
		},
	})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("want ErrPriceChanged, got resp=%v err=%v", resp, err)
	}
	if got := r.cartSnapshot(t, "buyer_1")["ci_a"].UnitPrice; got != 1500 {
		t.Fatalf("ci_a unit price = %d, want re-priced 1500", got)
	}
	if n := r.domain.Reservations(); n != 0 {
		t.Fatalf("nothing may be reserved, got %d", n)
	}
}

func TestPreviewCheckoutSelfPurchaseRefused_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{
		buyer: "sa",
		addrs: []*identityv1.Address{pvHome()},
	})

	_, err := r.svc.PreviewCheckout(context.Background(), "sa", "", nil)
	if !errors.Is(err, service.ErrSelfPurchase) {
		t.Fatalf("want ErrSelfPurchase, got %v", err)
	}
}

func TestPreviewCheckoutReleasesPreviewHoldsNeverCommits_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{addrs: []*identityv1.Address{pvHome()}})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil,
		service.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}}, "PLAT"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetVouchers()) != 2 {
		t.Fatalf("want two voucher quotes, got %v", resp.GetVouchers())
	}
	for _, v := range resp.GetVouchers() {
		if !v.GetValid() {
			t.Fatalf("voucher %s invalid: %s", v.GetCode(), v.GetReason())
		}
	}

	calls := r.promo.reserveCalls()
	if len(calls) != 2 {
		t.Fatalf("want two ValidateAndReserve calls, got %d", len(calls))
	}
	seen := map[string]bool{}
	for _, c := range calls {
		id := c.GetReservationId()
		if !strings.HasPrefix(id, "preview:buyer_1:") || len(id) <= len("preview:buyer_1:") {
			t.Fatalf("reservation id %q is not in the preview namespace", id)
		}
		if seen[id] {
			t.Fatalf("reservation id %q reused", id)
		}
		seen[id] = true
	}
	released := r.promo.releasedIDs()
	if len(released) != 2 {
		t.Fatalf("want two ReleaseReservation calls, got %v", released)
	}
	for _, id := range released {
		if !seen[id] {
			t.Fatalf("released %q was never reserved", id)
		}
	}
	if n := r.promo.commitCount(); n != 0 {
		t.Fatalf("preview must never commit, got %d commits", n)
	}
	if held := r.promo.heldIDs(); len(held) != 0 {
		t.Fatalf("holds left: %v", held)
	}
}

func TestPreviewCheckoutReleaseErrorIgnored_MultishopCheckout(t *testing.T) {
	r := pvNew(t, pvOpts{
		addrs:      []*identityv1.Address{pvHome()},
		releaseErr: status.Error(codes.Unavailable, "promotion down"),
	})

	resp, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil,
		service.WithVouchers([]*orderv1.ShopVoucher{{SellerId: "sa", Code: "SHOPA"}}, ""))
	if err != nil {
		t.Fatalf("a release failure must not fail the preview: %v", err)
	}
	if resp.GetTotalAmount() != 44500 || len(resp.GetVouchers()) != 1 || !resp.GetVouchers()[0].GetValid() {
		t.Fatalf("quote lost: total=%d vouchers=%v", resp.GetTotalAmount(), resp.GetVouchers())
	}
	if len(r.promo.releasedIDs()) != 1 {
		t.Fatalf("release must still be attempted, got %v", r.promo.releasedIDs())
	}
}
