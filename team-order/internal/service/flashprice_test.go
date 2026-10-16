package service_test

// ---- order-advertised-price tasks 5-8: checkout at the advertised flash price ----
//
// Interface fixed by these tests (implementer must provide, package service):
//   - var ErrPriceChanged, ErrPromotionUnavailable error (errors.Is-comparable).
//   - func FlashReservationID(orderID, listingID, variantID string) string
//     = orderID + ":" + listingID + ":" + variantID (empty variant -> trailing ":").
//   - type OrderFlashClient interface { GetActiveFlashSale, ReserveFlashSaleStock,
//     CommitFlashSaleStock, ReleaseFlashSaleStock } with the
//     promotionv1.FlashSaleServiceClient signatures (that client satisfies it).
//   - func WithOrderFlashSaleClient(c OrderFlashClient) OrderServiceOption
//     nil / typed-nil = "no client": every line is priced at the regular price and
//     no hold, commit or release call is ever made.
//   - CreateOrdersFromCart, before creating the saga: loads each selected line's
//     listing via the domain client GetListing (regular price = variant price when
//     > 0 else listing price), one GetActiveFlashSale per distinct listing, same
//     price rule as AddToCart. Any differing line -> every differing cart line is
//     re-priced via CartRepository.UpdateItemPrice (buyer scoped) and
//     ErrPriceChanged is returned; nothing is held. Flash lookup error: cart price
//     above regular -> ErrPriceChanged; equal -> proceeds; below -> ErrPromotionUnavailable
//     with the cart unchanged.
//   - Phase A (reserveGroup): after the stock reservation, one ReserveFlashSaleStock
//     per flash line (reservation_id = FlashReservationID(group order id, listing,
//     variant), campaign id, listing, variant, quantity = line quantity,
//     expected_sale_price = charged price). reserved=false -> ErrPriceChanged and
//     everything is released; transport error -> ErrPromotionUnavailable and
//     everything is released. The voucher gets the flash-priced subtotal.
//   - Phase B (runCheckout): CommitFlashSaleStock per flash hold BEFORE
//     commitDomainReservations; error or committed=false -> fail+compensate,
//     ErrPromotionUnavailable, no domain commit. failAndCompensate releases every
//     recorded flash hold (flash lines only), best effort.
//   - CancelOrder: after winning the claim, ReleaseFlashSaleStock for
//     FlashReservationID(order.ID, item.ListingID, item.VariantID) of EVERY item;
//     a release error never fails the cancel.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// pricedDomain is upstreamtest.Domain (stock + reservation lifecycle) whose
// GetListing also answers a price table.
type pricedDomain struct {
	*upstreamtest.Domain
	listings map[string]*listingv1.Listing
}

func (d *pricedDomain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	l, ok := d.listings[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "listing not found")
	}
	return &listingv1.GetListingResponse{Listing: l}, nil
}

// holdFake models team-promotion's FlashSaleService for one checkout. Events go to
// a log shared with the domain fake so cross-service ordering is observable.
type holdFake struct {
	promotionv1.FlashSaleServiceClient
	mu sync.Mutex

	sales  map[string]*promotionv1.GetActiveFlashSaleResponse
	getErr error

	reserveFn func(n int, req *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error)
	commitFn  func(req *promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error)
	releaseFn func(req *promotionv1.ReleaseFlashSaleStockRequest) error

	getCalls  []string
	reserved  []*promotionv1.ReserveFlashSaleStockRequest
	committed []string
	released  []string
	// releaseCtx records, per ReleaseFlashSaleStock call, the ctx error and whether a
	// deadline was set at the moment of the call.
	releaseCtxErrs []error
	events    *[]string
}

func (f *holdFake) log(e string) {
	if f.events != nil {
		*f.events = append(*f.events, e)
	}
}

func (f *holdFake) GetActiveFlashSale(_ context.Context, req *promotionv1.GetActiveFlashSaleRequest, _ ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, req.GetListingId())
	if f.getErr != nil {
		return nil, f.getErr
	}
	if r, ok := f.sales[req.GetListingId()]; ok {
		return r, nil
	}
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

func (f *holdFake) ReserveFlashSaleStock(_ context.Context, req *promotionv1.ReserveFlashSaleStockRequest, _ ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserved = append(f.reserved, req)
	f.log("flash-reserve")
	if f.reserveFn != nil {
		return f.reserveFn(len(f.reserved), req)
	}
	return &promotionv1.ReserveFlashSaleStockResponse{Reserved: true}, nil
}

func (f *holdFake) CommitFlashSaleStock(_ context.Context, req *promotionv1.CommitFlashSaleStockRequest, _ ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log("flash-commit")
	if f.commitFn != nil {
		return f.commitFn(req)
	}
	f.committed = append(f.committed, req.GetReservationId())
	return &promotionv1.CommitFlashSaleStockResponse{Committed: true}, nil
}

func (f *holdFake) ReleaseFlashSaleStock(ctx context.Context, req *promotionv1.ReleaseFlashSaleStockRequest, _ ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, req.GetReservationId())
	f.releaseCtxErrs = append(f.releaseCtxErrs, ctx.Err())
	f.log("flash-release")
	if f.releaseFn != nil {
		if err := f.releaseFn(req); err != nil {
			return nil, err
		}
	}
	return &promotionv1.ReleaseFlashSaleStockResponse{Released: true}, nil
}

func (f *holdFake) reserveIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.reserved {
		out = append(out, r.GetReservationId())
	}
	return out
}

func (f *holdFake) releasedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.released...)
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// subtotalPromo records the cart_subtotal each voucher reservation received.
type subtotalPromo struct {
	*fakePromotion
	subtotals []int64
}

func (p *subtotalPromo) ValidateAndReserve(ctx context.Context, in *promotionv1.ValidateAndReserveRequest, o ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	p.subtotals = append(p.subtotals, in.GetCartSubtotal())
	return p.fakePromotion.ValidateAndReserve(ctx, in, o...)
}

type flashRig struct {
	domain *pricedDomain
	orders *repository.InMemoryOrderRepository
	sagas  *recordingSagaRepo
	carts  *repository.InMemoryCartRepository
	flash  *holdFake
	promo  *subtotalPromo
	events []string
	svc    *service.OrderService
}

// newFlashRig: lst_1 regular 500000 and lst_2 regular 200000, both seller s1,
// stock 10 each. withFlash=false builds the service without a promotion client.
func newFlashRig(t *testing.T, withFlash bool) *flashRig {
	t.Helper()
	r := &flashRig{
		orders: repository.NewInMemoryOrderRepository(),
		sagas:  &recordingSagaRepo{SagaRepository: repository.NewInMemorySagaRepository()},
		carts:  repository.NewInMemoryCartRepository(),
		flash:  &holdFake{sales: map[string]*promotionv1.GetActiveFlashSaleResponse{}},
		promo:  &subtotalPromo{fakePromotion: newFakePromotion(1000)},
	}
	r.flash.events = &r.events
	r.domain = &pricedDomain{
		Domain: upstreamtest.NewDomain(map[string]int32{"lst_1": 10, "lst_2": 10}),
		listings: map[string]*listingv1.Listing{
			"lst_1": {Id: "lst_1", Price: 500000, SellerId: "s1", Stock: 10},
			"lst_2": {Id: "lst_2", Price: 200000, SellerId: "s1", Stock: 10},
		},
	}
	opts := []service.OrderServiceOption{service.WithSagaRepository(r.sagas), service.WithPromotionClient(r.promo)}
	if withFlash {
		opts = append(opts, service.WithOrderFlashSaleClient(r.flash))
	}
	r.svc = service.NewOrderService(r.orders, r.carts, nil, nil, r.domain, nil, nil, opts...)
	return r
}

// sale makes a listing-wide campaign "c1" active at price for the listing.
func (r *flashRig) sale(listing string, price int64) {
	r.flash.sales[listing] = activeSale("", price)
}

func (r *flashRig) addLine(t *testing.T, buyer, listing string, qty int32, price int64) {
	t.Helper()
	if _, err := r.carts.AddItem(context.Background(), repository.CartItem{
		UserID: buyer, ListingID: listing, Quantity: qty, UnitPrice: price, SellerID: "s1", Title: listing,
	}); err != nil {
		t.Fatal(err)
	}
}

func (r *flashRig) cartPrice(t *testing.T, buyer, listing string) int64 {
	t.Helper()
	items, err := r.carts.GetCart(context.Background(), buyer)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ListingID == listing {
			return it.UnitPrice
		}
	}
	t.Fatalf("no cart line for %s", listing)
	return 0
}

func (r *flashRig) checkout(buyer, voucher string, opts ...service.CheckoutOption) ([]repository.Order, error) {
	return r.svc.CreateOrdersFromCart(context.Background(), buyer, addr(), nil, 1, voucher, opts...)
}

func (r *flashRig) orderCount(t *testing.T, buyer string) int {
	t.Helper()
	os, err := r.orders.ListBuyerOrders(context.Background(), buyer, 0)
	if err != nil {
		t.Fatal(err)
	}
	return len(os)
}

// assertNothingHeld: no stock reservation, voucher hold, flash hold or saga exists.
func (r *flashRig) assertNothingHeld(t *testing.T) {
	t.Helper()
	if n := r.domain.Reservations(); n != 0 {
		t.Errorf("domain reservations = %d, want 0", n)
	}
	if n := r.promo.reserveCount(); n != 0 {
		t.Errorf("voucher reservations = %d, want 0", n)
	}
	if ids := r.flash.reserveIDs(); len(ids) != 0 {
		t.Errorf("flash holds = %v, want none", ids)
	}
	if ids := r.sagas.sagaIDs(); len(ids) != 0 {
		t.Errorf("sagas created = %v, want none (pre-check runs before the saga)", ids)
	}
}

// assertAllDomainReleased: every domain reservation of the attempt is released.
func (r *flashRig) assertAllDomainReleased(t *testing.T, want int) {
	t.Helper()
	if n := r.domain.Reservations(); n != want {
		t.Fatalf("domain reservations = %d, want %d", n, want)
	}
	for _, l := range []string{"lst_1", "lst_2"} {
		if got := r.domain.Stock(l); got != 10 {
			t.Errorf("stock %s = %d, want 10 (released)", l, got)
		}
	}
}

func (r *flashRig) assertCompensated(t *testing.T) {
	t.Helper()
	id := r.sagas.lastSagaID()
	if id == "" {
		t.Fatal("no saga was created")
	}
	sg, err := r.sagas.GetSaga(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if sg.Status != repository.SagaStatusCompensated {
		t.Fatalf("saga status = %v, want COMPENSATED", sg.Status)
	}
}

func TestFlashReservationIDShape(t *testing.T) {
	if got := service.FlashReservationID("o1", "l1", "v1"); got != "o1:l1:v1" {
		t.Errorf("with variant = %q, want o1:l1:v1", got)
	}
	if got := service.FlashReservationID("o1", "l1", ""); got != "o1:l1:" {
		t.Errorf("without variant = %q, want o1:l1:", got)
	}
}

// ---------------------------------------------------------------- pre-check

func TestCheckoutEndedSaleRefusedAndRepriced(t *testing.T) {
	r := newFlashRig(t, true) // no active campaign any more
	r.addLine(t, "b1", "lst_1", 1, 350000)

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 500000 {
		t.Errorf("cart unit_price = %d, want re-priced 500000", got)
	}
	r.assertNothingHeld(t)
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

func TestCheckoutSaleStartedRefusedAndRepriced(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 500000)

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 350000 {
		t.Errorf("cart unit_price = %d, want 350000", got)
	}
	r.assertNothingHeld(t)
}

func TestCheckoutRetryAfterRepriceSucceeds(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 500000)

	if _, err := r.checkout("b1", ""); !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("first call err = %v, want ErrPriceChanged", err)
	}
	orders, err := r.checkout("b1", "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("retry: %v (%d orders)", err, len(orders))
	}
	if got := orders[0].Items[0].UnitPrice; got != 350000 {
		t.Errorf("item unit_price = %d, want the re-priced 350000", got)
	}
}

func TestCheckoutChargesFlashPrice(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 2, 350000)

	orders, err := r.checkout("b1", "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	o := orders[0]
	if o.Items[0].UnitPrice != 350000 {
		t.Errorf("item unit_price = %d, want 350000", o.Items[0].UnitPrice)
	}
	if o.ItemsSubtotal != 700000 {
		t.Errorf("items_subtotal = %d, want 700000", o.ItemsSubtotal)
	}
	if want := o.ItemsSubtotal + o.ShippingFee - o.DiscountAmount; o.TotalAmount != want {
		t.Errorf("total_amount = %d, want items+shipping-discount = %d", o.TotalAmount, want)
	}
}

func TestCheckoutOneChangedLineRefusesAll(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000) // unchanged
	r.addLine(t, "b1", "lst_2", 1, 150000) // flash ended: regular is 200000

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	if got := r.cartPrice(t, "b1", "lst_2"); got != 200000 {
		t.Errorf("changed line = %d, want 200000", got)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 350000 {
		t.Errorf("unchanged line = %d, want 350000", got)
	}
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
	r.assertNothingHeld(t)
}

func TestCheckoutPriceChangedFreesIdempotencyKey(t *testing.T) {
	r := newFlashRig(t, true)
	r.addLine(t, "b1", "lst_1", 1, 350000) // ended sale

	if _, err := r.checkout("b1", "", service.WithIdempotencyKey("key-1")); !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("first call err = %v, want ErrPriceChanged", err)
	}
	orders, err := r.checkout("b1", "", service.WithIdempotencyKey("key-1"))
	if err != nil || len(orders) != 1 {
		t.Fatalf("retry with the same key must run: %v", err)
	}
	if orders[0].Items[0].UnitPrice != 500000 {
		t.Errorf("retry unit_price = %d, want 500000", orders[0].Items[0].UnitPrice)
	}
}

func TestCheckoutRepriceScopedToBuyer(t *testing.T) {
	r := newFlashRig(t, true)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.addLine(t, "b2", "lst_1", 1, 350000)

	if _, err := r.checkout("b1", ""); !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 500000 {
		t.Errorf("b1 line = %d, want 500000", got)
	}
	if got := r.cartPrice(t, "b2", "lst_1"); got != 350000 {
		t.Errorf("b2 line = %d, want untouched 350000", got)
	}
}

func TestCheckoutPromotionDownFlashLineUnavailable(t *testing.T) {
	r := newFlashRig(t, true)
	r.flash.getErr = status.Error(codes.Unavailable, "promotion down")
	r.addLine(t, "b1", "lst_1", 1, 350000)

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPromotionUnavailable) {
		t.Fatalf("err = %v, want ErrPromotionUnavailable", err)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 350000 {
		t.Errorf("cart line = %d, want unchanged 350000", got)
	}
	r.assertNothingHeld(t)
}

func TestCheckoutPromotionDownRegularLineProceeds(t *testing.T) {
	r := newFlashRig(t, true)
	r.flash.getErr = status.Error(codes.Unavailable, "promotion down")
	r.addLine(t, "b1", "lst_1", 1, 500000)

	orders, err := r.checkout("b1", "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	if orders[0].Items[0].UnitPrice != 500000 {
		t.Errorf("unit_price = %d, want 500000", orders[0].Items[0].UnitPrice)
	}
	if ids := r.flash.reserveIDs(); len(ids) != 0 {
		t.Errorf("regular line must make no hold call: %v", ids)
	}
}

func TestCheckoutPromotionDownCartAboveRegularPriceChanged(t *testing.T) {
	r := newFlashRig(t, true)
	r.flash.getErr = status.Error(codes.Unavailable, "promotion down")
	r.addLine(t, "b1", "lst_1", 1, 600000) // regular is 500000

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 500000 {
		t.Errorf("cart line = %d, want 500000", got)
	}
}

func TestCheckoutNilPromotionFlashLineRepriced(t *testing.T) {
	r := newFlashRig(t, false)
	r.addLine(t, "b1", "lst_1", 1, 350000)

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	if got := r.cartPrice(t, "b1", "lst_1"); got != 500000 {
		t.Errorf("cart line = %d, want 500000", got)
	}
	r.assertNothingHeld(t)
}

// ----------------------------------------------------------------- Phase A

func TestCheckoutFlashHoldOnePerLineFullQuantity(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 3, 350000)

	orders, err := r.checkout("b1", "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	if len(r.flash.reserved) != 1 {
		t.Fatalf("ReserveFlashSaleStock calls = %d, want exactly 1", len(r.flash.reserved))
	}
	got := r.flash.reserved[0]
	if got.GetQuantity() != 3 || got.GetExpectedSalePrice() != 350000 {
		t.Errorf("quantity=%d expected_sale_price=%d, want 3 and 350000", got.GetQuantity(), got.GetExpectedSalePrice())
	}
	if got.GetCampaignId() != "c1" || got.GetListingId() != "lst_1" || got.GetVariantId() != "" {
		t.Errorf("campaign/listing/variant = %q/%q/%q", got.GetCampaignId(), got.GetListingId(), got.GetVariantId())
	}
	if want := service.FlashReservationID(orders[0].ID, "lst_1", ""); got.GetReservationId() != want {
		t.Errorf("reservation_id = %q, want %q", got.GetReservationId(), want)
	}
}

func TestCheckoutRegularLineNoFlashHold(t *testing.T) {
	r := newFlashRig(t, true)
	r.addLine(t, "b1", "lst_1", 1, 500000)

	if _, err := r.checkout("b1", ""); err != nil {
		t.Fatal(err)
	}
	if ids := r.flash.reserveIDs(); len(ids) != 0 {
		t.Errorf("flash holds = %v, want none", ids)
	}
}

func TestCheckoutRefusedFlashHoldPriceChangedReleasesAll(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.sale("lst_2", 150000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.addLine(t, "b1", "lst_2", 1, 150000)
	refused := ""
	r.flash.reserveFn = func(n int, req *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error) {
		if n == 2 {
			refused = req.GetListingId()
			return &promotionv1.ReserveFlashSaleStockResponse{Reserved: false, Reason: "price_changed"}, nil
		}
		return &promotionv1.ReserveFlashSaleStockResponse{Reserved: true}, nil
	}

	_, err := r.checkout("b1", "SAVE")
	if !errors.Is(err, service.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	ids := r.flash.reserveIDs()
	if len(ids) != 2 {
		t.Fatalf("hold calls = %v, want 2", ids)
	}
	if !contains(r.flash.releasedIDs(), ids[0]) {
		t.Errorf("first flash hold %q not released: %v", ids[0], r.flash.releasedIDs())
	}
	if len(r.promo.releasedIDs) != 1 {
		t.Errorf("voucher hold releases = %v, want 1", r.promo.releasedIDs)
	}
	r.assertAllDomainReleased(t, 2)
	regular := map[string]int64{"lst_1": 500000, "lst_2": 200000}
	if got := r.cartPrice(t, "b1", refused); got != regular[refused] {
		t.Errorf("refused line %s = %d, want regular %d", refused, got, regular[refused])
	}
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

func TestCheckoutVoucherReservedOnFlashSubtotal(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 2, 350000)

	if _, err := r.checkout("b1", "SAVE"); err != nil {
		t.Fatal(err)
	}
	if len(r.promo.subtotals) != 1 || r.promo.subtotals[0] != 700000 {
		t.Errorf("ValidateAndReserve cart_subtotal = %v, want [700000]", r.promo.subtotals)
	}
}

func TestCheckoutFlashHoldTransportErrorUnavailable(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.sale("lst_2", 150000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.addLine(t, "b1", "lst_2", 1, 150000)
	r.flash.reserveFn = func(n int, _ *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error) {
		if n == 2 {
			return nil, status.Error(codes.Unavailable, "promotion down")
		}
		return &promotionv1.ReserveFlashSaleStockResponse{Reserved: true}, nil
	}

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPromotionUnavailable) {
		t.Fatalf("err = %v, want ErrPromotionUnavailable", err)
	}
	ids := r.flash.reserveIDs()
	if len(ids) != 2 || !contains(r.flash.releasedIDs(), ids[0]) {
		t.Errorf("first hold must be released: holds=%v released=%v", ids, r.flash.releasedIDs())
	}
	r.assertAllDomainReleased(t, 2)
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

// ----------------------------------------------------------------- Phase B

func (r *flashRig) hookDomainCommit(commitErr error) {
	r.domain.CommitErr = func(string) error {
		r.events = append(r.events, "domain-commit")
		return commitErr
	}
}

func TestCheckoutCommitsFlashHoldInPhaseB(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.hookDomainCommit(nil)
	ordersAtFlashCommit := -1
	r.flash.commitFn = func(req *promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error) {
		ordersAtFlashCommit = r.orderCount(t, "b1")
		r.flash.committed = append(r.flash.committed, req.GetReservationId())
		return &promotionv1.CommitFlashSaleStockResponse{Committed: true}, nil
	}

	orders, err := r.checkout("b1", "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("checkout: %v", err)
	}
	want := service.FlashReservationID(orders[0].ID, "lst_1", "")
	if len(r.flash.committed) != 1 || r.flash.committed[0] != want {
		t.Errorf("committed = %v, want [%s]", r.flash.committed, want)
	}
	fc, dc := -1, -1
	for i, e := range r.events {
		switch e {
		case "flash-commit":
			fc = i
		case "domain-commit":
			dc = i
		}
	}
	if fc < 0 || dc < 0 || fc > dc {
		t.Errorf("flash commit must precede domain commit: events=%v", r.events)
	}
	if ordersAtFlashCommit != 0 {
		t.Errorf("order existed at flash commit time (%d)", ordersAtFlashCommit)
	}
}

func TestCheckoutFlashCommitErrorCompensates(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.flash.commitFn = func(*promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error) {
		return nil, status.Error(codes.Unavailable, "promotion down")
	}

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPromotionUnavailable) {
		t.Fatalf("err = %v, want ErrPromotionUnavailable", err)
	}
	if r.domain.Calls.Commit != 0 {
		t.Errorf("domain commit calls = %d, want 0", r.domain.Calls.Commit)
	}
	ids := r.flash.reserveIDs()
	if len(ids) != 1 || !contains(r.flash.releasedIDs(), ids[0]) {
		t.Errorf("flash hold must be released: holds=%v released=%v", ids, r.flash.releasedIDs())
	}
	r.assertAllDomainReleased(t, 1)
	r.assertCompensated(t)
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

func TestCheckoutDomainCommitFailureReleasesCommittedFlashHold(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.hookDomainCommit(status.Error(codes.Unavailable, "team-domain down"))

	if _, err := r.checkout("b1", ""); err == nil {
		t.Fatal("checkout must fail when the domain commit fails")
	}
	if len(r.flash.committed) != 1 {
		t.Fatalf("flash commits = %v, want 1", r.flash.committed)
	}
	if got := r.flash.releasedIDs(); len(got) != 1 || got[0] != r.flash.committed[0] {
		t.Errorf("released = %v, want the committed hold %v", got, r.flash.committed)
	}
	r.assertCompensated(t)
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

func TestCheckoutFlashCommitFalseCompensates(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.flash.commitFn = func(*promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error) {
		return &promotionv1.CommitFlashSaleStockResponse{Committed: false}, nil
	}

	_, err := r.checkout("b1", "")
	if !errors.Is(err, service.ErrPromotionUnavailable) {
		t.Fatalf("err = %v, want ErrPromotionUnavailable", err)
	}
	if r.domain.Calls.Commit != 0 {
		t.Errorf("domain commit calls = %d, want 0", r.domain.Calls.Commit)
	}
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

func TestCheckoutRegularLineNoFlashCommit(t *testing.T) {
	r := newFlashRig(t, true)
	r.addLine(t, "b1", "lst_1", 1, 500000)

	if _, err := r.checkout("b1", ""); err != nil {
		t.Fatal(err)
	}
	for _, e := range r.events {
		if e == "flash-commit" {
			t.Fatalf("regular-only checkout made a commit call: %v", r.events)
		}
	}
}

func TestCheckoutRetryUsesFreshFlashReservationIDs(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.flash.reserveFn = func(n int, _ *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error) {
		if n == 1 {
			return nil, status.Error(codes.Unavailable, "promotion blip")
		}
		return &promotionv1.ReserveFlashSaleStockResponse{Reserved: true}, nil
	}

	if _, err := r.checkout("b1", ""); !errors.Is(err, service.ErrPromotionUnavailable) {
		t.Fatalf("first attempt err = %v, want ErrPromotionUnavailable", err)
	}
	if _, err := r.checkout("b1", ""); err != nil {
		t.Fatalf("retry: %v", err)
	}
	ids := r.flash.reserveIDs()
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Errorf("reservation ids = %v, want two distinct ids", ids)
	}
}

// ------------------------------------------------------------ compensation

func TestFailAndCompensateReleasesFlashHolds(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.hookDomainCommit(status.Error(codes.Unavailable, "team-domain down"))

	if _, err := r.checkout("b1", ""); err == nil {
		t.Fatal("checkout must fail")
	}
	ids := r.flash.reserveIDs()
	if got := r.flash.releasedIDs(); len(ids) != 1 || len(got) != 1 || got[0] != ids[0] {
		t.Errorf("released = %v, want exactly the hold %v", got, ids)
	}
	r.assertCompensated(t)
}

func TestFailAndCompensateReleasesOnlyFlashLines(t *testing.T) {
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 1, 350000)
	r.addLine(t, "b1", "lst_2", 1, 200000) // regular
	r.hookDomainCommit(status.Error(codes.Unavailable, "team-domain down"))

	if _, err := r.checkout("b1", ""); err == nil {
		t.Fatal("checkout must fail")
	}
	ids := r.flash.reserveIDs()
	if len(ids) != 1 {
		t.Fatalf("holds = %v, want 1 (flash line only)", ids)
	}
	if got := r.flash.releasedIDs(); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("released = %v, want only %v", got, ids)
	}
}

// ------------------------------------------------------------------ cancel

// placeFlashOrder checks out one flash line (lst_1 x2 at 350000) and optionally a
// regular line (lst_2) and returns the single order.
func placeFlashOrder(t *testing.T, withRegular bool) (*flashRig, repository.Order) {
	t.Helper()
	r := newFlashRig(t, true)
	r.sale("lst_1", 350000)
	r.addLine(t, "b1", "lst_1", 2, 350000)
	if withRegular {
		r.addLine(t, "b1", "lst_2", 1, 200000)
	}
	orders, err := r.checkout("b1", "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("setup checkout: %v", err)
	}
	if len(r.flash.releasedIDs()) != 0 {
		t.Fatalf("a successful checkout must release nothing: %v", r.flash.releasedIDs())
	}
	return r, orders[0]
}

func TestCancelOrderReleasesFlashHold(t *testing.T) {
	r, o := placeFlashOrder(t, false)
	hold := r.flash.reserveIDs()[0]

	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled {
		t.Fatalf("cancel: %v %+v", err, res)
	}
	if got := r.flash.releasedIDs(); len(got) != 1 || got[0] != hold {
		t.Errorf("released = %v, want [%s]", got, hold)
	}
}

func TestCancelOrderReleasesEveryItem(t *testing.T) {
	r, o := placeFlashOrder(t, true)

	if _, err := r.svc.CancelOrder(context.Background(), o.ID); err != nil {
		t.Fatal(err)
	}
	got := r.flash.releasedIDs()
	want := []string{service.FlashReservationID(o.ID, "lst_1", ""), service.FlashReservationID(o.ID, "lst_2", "")}
	if len(got) != 2 || !contains(got, want[0]) || !contains(got, want[1]) {
		t.Errorf("released = %v, want one call per item %v", got, want)
	}
}

func TestCancelOrderFlashReleaseErrorStillCancels(t *testing.T) {
	r, o := placeFlashOrder(t, false)
	r.flash.releaseFn = func(*promotionv1.ReleaseFlashSaleStockRequest) error {
		return status.Error(codes.Unavailable, "promotion down")
	}

	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled {
		t.Fatalf("a flash release error must not fail the cancel: %v %+v", err, res)
	}
	got, _ := r.orders.GetOrder(context.Background(), o.ID)
	if got.Status != repository.OrderStatusCancelled {
		t.Errorf("order status = %v, want Cancelled", got.Status)
	}
}

func TestCancelOrderRefusedNoFlashRelease(t *testing.T) {
	r, o := placeFlashOrder(t, false)
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), o.ID, repository.OrderStatusShipped,
		[]repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}

	if _, err := r.svc.CancelOrder(context.Background(), o.ID); !errors.Is(err, service.ErrInvalidStatus) {
		t.Fatalf("err = %v, want ErrInvalidStatus", err)
	}
	if got := r.flash.releasedIDs(); len(got) != 0 {
		t.Errorf("a refused cancel must release nothing: %v", got)
	}
}

func TestCancelPaidOrderReleasesCommittedFlashHold(t *testing.T) {
	r, o := placeFlashOrder(t, false)
	hold := r.flash.reserveIDs()[0]
	if _, err := r.orders.UpdateOrderStatusFrom(context.Background(), o.ID, repository.OrderStatusPaid,
		[]repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}

	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled {
		t.Fatalf("cancel paid: %v %+v", err, res)
	}
	if got := r.flash.releasedIDs(); len(got) != 1 || got[0] != hold {
		t.Errorf("released = %v, want [%s]", got, hold)
	}
}

// Fail closed: a listing the domain cannot describe (nil Listing, no error) means
// the regular price is unknown, so the checkout is refused with a plain error.
func TestCheckoutNilListingFailsClosed(t *testing.T) {
	r := newFlashRig(t, true)
	r.domain.listings["lst_nil"] = nil
	r.addLine(t, "b1", "lst_nil", 1, 350000)

	_, err := r.checkout("b1", "")
	if err == nil {
		t.Fatal("checkout must fail when the listing is nil")
	}
	if errors.Is(err, service.ErrPriceChanged) || errors.Is(err, service.ErrPromotionUnavailable) {
		t.Fatalf("err = %v, want a plain error (not price changed / promotion unavailable)", err)
	}
	if got := r.cartPrice(t, "b1", "lst_nil"); got != 350000 {
		t.Errorf("cart line = %d, want unchanged 350000", got)
	}
	r.assertNothingHeld(t)
	if r.orderCount(t, "b1") != 0 {
		t.Error("no order may exist")
	}
}

// The flash release gets its own live context: when the stock release consumes the
// shared inline deadline (a stopped team-domain), every item's flash release is
// still made, on a context that has not expired. Takes about the 2s inline budget.
func TestCancelOrderFlashReleaseHasOwnTimeout(t *testing.T) {
	r, o := placeFlashOrder(t, true)
	r.domain.ReleaseHang = true // set after checkout: blocks until the inline deadline

	res, err := r.svc.CancelOrder(context.Background(), o.ID)
	if err != nil || res.Status != repository.OrderStatusCancelled {
		t.Fatalf("cancel: %v %+v", err, res)
	}
	r.flash.mu.Lock()
	defer r.flash.mu.Unlock()
	if len(r.flash.released) != 2 {
		t.Fatalf("flash releases = %v, want one per item", r.flash.released)
	}
	for i, cerr := range r.flash.releaseCtxErrs {
		if cerr != nil {
			t.Errorf("flash release %d ran on an expired context: %v", i, cerr)
		}
	}
}
