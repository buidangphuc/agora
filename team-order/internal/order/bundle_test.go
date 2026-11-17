package order_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// Interface fixed by these tests (implementer must provide):
//   - func (s *CartService) AddBundleToCart(ctx, userID, bundleID string, quantity int32)
//     ([]store.CartItem, int64, error): same return shape as AddToCart (items, net subtotal).
//   - upstream.DomainClient gains GetBundle(ctx, *listingv1.GetBundleRequest, ...grpc.CallOption)
//     (*listingv1.GetBundleResponse, error); a bundle unknown to team-domain surfaces as a
//     gRPC NotFound error from the client.
//   - store.CartRepository / store.OrderRepository carry BundleID and BundleDiscount on lines.
//   - GetCart's subtotal is net of bundle discounts.
//   - All bundle behaviour below runs against the real Postgres repositories (T1 store API).

type bundleDomain struct {
	*upstreamtest.Domain
	mu        sync.Mutex
	listings  map[string]*listingv1.Listing
	bundles   map[string]*listingv1.Bundle
	getErr    map[string]error
	bundleErr error
}

func (d *bundleDomain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.getErr[req.GetId()]; err != nil {
		return nil, err
	}
	l, ok := d.listings[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "listing not found")
	}
	cp := proto.Clone(l).(*listingv1.Listing)
	return &listingv1.GetListingResponse{Listing: cp}, nil
}

func (d *bundleDomain) GetBundle(_ context.Context, req *listingv1.GetBundleRequest, _ ...grpc.CallOption) (*listingv1.GetBundleResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bundleErr != nil {
		return nil, d.bundleErr
	}
	b, ok := d.bundles[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "bundle not found")
	}
	cp := proto.Clone(b).(*listingv1.Bundle)
	return &listingv1.GetBundleResponse{Bundle: cp}, nil
}

func (d *bundleDomain) setListing(id string, mut func(*listingv1.Listing)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	mut(d.listings[id])
}

func (d *bundleDomain) setBundle(id string, mut func(*listingv1.Bundle)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	mut(d.bundles[id])
}

func pubListing(id, seller string, price int64, stock int32) *listingv1.Listing {
	return &listingv1.Listing{Id: id, Title: id, Price: price, SellerId: seller, Stock: stock, Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED}
}

type bundleRig struct {
	pg     *pgOutbox
	carts  *store.PostgresCartRepository
	orders *store.PostgresOrderRepository
	domain *bundleDomain
	cart   *order.CartService
	svc    *order.OrderService
	logs   *bytes.Buffer
}

// newBundleRig: seller s1 owns lst_a (100000), lst_b (50000), lst_c1..c3 (10000 each); bundle
// bnd_ab = {lst_a, lst_b} at 120000; bnd_c = {lst_c1, lst_c2, lst_c3} at 29999.
func newBundleRig(t *testing.T) *bundleRig {
	t.Helper()
	pg := newPGOutbox(t.Context(), t)
	r := &bundleRig{pg: pg, carts: store.NewPostgresCartRepository(pg.pool), orders: pg.orders(), logs: &bytes.Buffer{}}
	stock := map[string]int32{"lst_a": 100, "lst_b": 100, "lst_c1": 50, "lst_c2": 50, "lst_c3": 50}
	r.domain = &bundleDomain{
		Domain: upstreamtest.NewDomain(stock),
		listings: map[string]*listingv1.Listing{
			"lst_a":  pubListing("lst_a", "s1", 100000, 100),
			"lst_b":  pubListing("lst_b", "s1", 50000, 100),
			"lst_c1": pubListing("lst_c1", "s1", 10000, 50),
			"lst_c2": pubListing("lst_c2", "s1", 10000, 50),
			"lst_c3": pubListing("lst_c3", "s1", 10000, 50),
		},
		bundles: map[string]*listingv1.Bundle{
			"bnd_ab": {Id: "bnd_ab", SellerId: "s1", Title: "AB", ListingIds: []string{"lst_a", "lst_b"}, BundlePrice: 120000},
			"bnd_c":  {Id: "bnd_c", SellerId: "s1", Title: "C", ListingIds: []string{"lst_c1", "lst_c2", "lst_c3"}, BundlePrice: 29999},
		},
		getErr: map[string]error{},
	}
	logger := slog.New(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r.cart = newCart(t, r.carts, r.orders, r.domain, logger)
	r.svc = newSvc(t, r.orders, r.carts, nil, nil, r.domain, &pvAddr{addrs: []*identityv1.Address{pvHome()}}, logger, order.WithSagaRepository(pg.sagas()))
	return r
}

func byListing(items []store.CartItem) map[string]store.CartItem {
	m := map[string]store.CartItem{}
	for _, it := range items {
		if it.BundleID == "" {
			m[it.ListingID+"|plain"] = it
		} else {
			m[it.ListingID+"|"+it.BundleID] = it
		}
	}
	return m
}

func (r *bundleRig) cartOf(t *testing.T, user string) []store.CartItem {
	t.Helper()
	items, err := r.carts.GetCart(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func (r *bundleRig) mustAddBundle(t *testing.T, user, bundle string, qty int32) []store.CartItem {
	t.Helper()
	items, _, err := r.cart.AddBundleToCart(context.Background(), user, bundle, qty)
	if err != nil {
		t.Fatalf("AddBundleToCart(%s x%d): %v", bundle, qty, err)
	}
	return items
}

func assertNoBundleTag(t *testing.T, items []store.CartItem) {
	t.Helper()
	for _, it := range items {
		if it.BundleID != "" || it.BundleDiscount != 0 {
			t.Errorf("line %s still tagged: bundle=%q discount=%d", it.ListingID, it.BundleID, it.BundleDiscount)
		}
	}
}

func discountSum(items []store.CartItem) int64 {
	var s int64
	for _, it := range items {
		s += it.BundleDiscount
	}
	return s
}

// ---------------------------------------------------------------- add

// Scenario: Adding a bundle creates one tagged line per listing
func TestBundleAddCreatesOneTaggedLinePerListing(t *testing.T) {
	r := newBundleRig(t)
	items, subtotal, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("lines = %d, want 2", len(items))
	}
	m := byListing(items)
	a, b := m["lst_a|bnd_ab"], m["lst_b|bnd_ab"]
	if a.BundleDiscount != 20000 || b.BundleDiscount != 10000 {
		t.Errorf("discounts = %d/%d, want 20000/10000", a.BundleDiscount, b.BundleDiscount)
	}
	if a.Quantity != 1 || b.Quantity != 1 || a.UnitPrice != 100000 || b.UnitPrice != 50000 {
		t.Errorf("lines not at listing price x1: %+v %+v", a, b)
	}
	if a.UserID != "buyer_1" || a.SellerID != "s1" {
		t.Errorf("line owner/seller = %q/%q", a.UserID, a.SellerID)
	}
	if subtotal != 120000 {
		t.Errorf("subtotal = %d, want 120000 (net of the bundle discount)", subtotal)
	}
}

// Scenario: The discount is allocated exactly with the remainder on the last line
func TestBundleAddAllocatesRemainderOnLastLine(t *testing.T) {
	r := newBundleRig(t)
	items := r.mustAddBundle(t, "buyer_1", "bnd_c", 1)
	m := byListing(items)
	// per combo discount = 30000-29999 = 1; floor(1*10000/30000)=0 on the first two lines.
	if d1, d2, d3 := m["lst_c1|bnd_c"].BundleDiscount, m["lst_c2|bnd_c"].BundleDiscount, m["lst_c3|bnd_c"].BundleDiscount; d1 != 0 || d2 != 0 || d3 != 1 {
		t.Errorf("discounts = %d/%d/%d, want 0/0/1 (remainder on the last listing)", d1, d2, d3)
	}
}

// Scenario: The discount is allocated exactly with the remainder on the last line
func TestBundleAddQuantityThreeSumsExactly(t *testing.T) {
	r := newBundleRig(t)
	r.domain.setBundle("bnd_c", func(b *listingv1.Bundle) { b.BundlePrice = 20000 })
	items, subtotal, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_c", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := discountSum(items); got != 30000 {
		t.Errorf("discount sum = %d, want 30000", got)
	}
	for _, it := range items {
		if it.Quantity != 3 {
			t.Errorf("line %s quantity = %d, want 3", it.ListingID, it.Quantity)
		}
	}
	if subtotal != 3*30000-30000 {
		t.Errorf("subtotal = %d, want 60000", subtotal)
	}
}

// Scenario: The discount is allocated exactly with the remainder on the last line
func TestBundleAddUnevenAllocationAlwaysSumsToTotal(t *testing.T) {
	r := newBundleRig(t)
	// 150000 -> 120001: discount 29999 per combo, q=7 -> 209993 over prices 100000/50000.
	r.domain.setBundle("bnd_ab", func(b *listingv1.Bundle) { b.BundlePrice = 120001 })
	items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 7)
	if got := discountSum(items); got != 29999*7 {
		t.Errorf("discount sum = %d, want %d", got, 29999*7)
	}
	m := byListing(items)
	if first := m["lst_a|bnd_ab"].BundleDiscount; first != 209993*100000/150000 {
		t.Errorf("first line = %d, want floor share %d", first, 209993*100000/150000)
	}
}

// Scenario: Adding the same bundle again increases every line
func TestBundleAddSameBundleAgainIncreasesEveryLine(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	items, subtotal, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("lines = %d, want 2", len(items))
	}
	for _, it := range items {
		if it.Quantity != 3 || it.BundleID != "bnd_ab" {
			t.Errorf("line %s = qty %d bundle %q, want qty 3 bundle bnd_ab", it.ListingID, it.Quantity, it.BundleID)
		}
	}
	if got := discountSum(items); got != 90000 {
		t.Errorf("discount sum = %d, want 90000", got)
	}
	if subtotal != 3*150000-90000 {
		t.Errorf("subtotal = %d, want 360000", subtotal)
	}
}

// Scenario: A bundle line stays separate from a plain line of the same listing
func TestBundleAddKeepsPlainLineSeparate(t *testing.T) {
	r := newBundleRig(t)
	if _, _, err := r.cart.AddToCart(context.Background(), "buyer_1", "lst_a", "", 2); err != nil {
		t.Fatal(err)
	}
	items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	m := byListing(items)
	if len(items) != 3 {
		t.Fatalf("lines = %d, want 3", len(items))
	}
	if p := m["lst_a|plain"]; p.Quantity != 2 || p.UnitPrice != 100000 || p.BundleDiscount != 0 {
		t.Errorf("plain line changed: %+v", p)
	}
	if b := m["lst_a|bnd_ab"]; b.Quantity != 1 || b.BundleDiscount != 20000 {
		t.Errorf("bundle line wrong: %+v", b)
	}
}

// Scenario: A quantity out of range is rejected
func TestBundleAddQuantityOutOfRangeRejected(t *testing.T) {
	r := newBundleRig(t)
	for _, q := range []int32{0, -1, 100} {
		if _, _, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", q); err == nil {
			t.Errorf("quantity %d: want error", q)
		}
	}
	if items := r.cartOf(t, "buyer_1"); len(items) != 0 {
		t.Fatalf("cart changed by rejected adds: %+v", items)
	}
	// boundaries 1 and 99 are accepted.
	if items := r.mustAddBundle(t, "buyer_2", "bnd_ab", 99); len(items) != 2 || items[0].Quantity != 99 {
		t.Errorf("quantity 99 not accepted: %+v", items)
	}
	if items := r.mustAddBundle(t, "buyer_3", "bnd_ab", 1); len(items) != 2 {
		t.Errorf("quantity 1 not accepted: %+v", items)
	}
}

// Scenario: An unknown bundle is rejected
func TestBundleAddUnknownBundleRejected(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	before := r.cartOf(t, "buyer_1")
	_, _, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_missing", 1)
	if err == nil {
		t.Fatal("want error for an unknown bundle")
	}
	if status.Code(err) != codes.NotFound && !errors.Is(err, store.ErrCartItemNotFound) && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("err = %v, want a not-found error", err)
	}
	if after := r.cartOf(t, "buyer_1"); len(after) != len(before) || discountSum(after) != discountSum(before) {
		t.Fatalf("cart changed: %+v -> %+v", before, after)
	}
}

// Scenario: A bundle with an unavailable listing is refused
func TestBundleAddUnavailableListingRefusedAllOrNone(t *testing.T) {
	cases := map[string]func(r *bundleRig){
		"unpublished": func(r *bundleRig) {
			r.domain.setListing("lst_b", func(l *listingv1.Listing) { l.Status = listingv1.ListingStatus_LISTING_STATUS_DRAFT })
		},
		"banned": func(r *bundleRig) {
			r.domain.setListing("lst_b", func(l *listingv1.Listing) { l.Status = listingv1.ListingStatus_LISTING_STATUS_BANNED })
		},
		"out of stock for the quantity": func(r *bundleRig) {
			r.domain.setListing("lst_b", func(l *listingv1.Listing) { l.Stock = 1 })
		},
		"not owned by the bundle seller": func(r *bundleRig) {
			r.domain.setListing("lst_b", func(l *listingv1.Listing) { l.SellerId = "s2" })
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := newBundleRig(t)
			mutate(r)
			if _, _, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", 2); err == nil {
				t.Fatal("want error")
			}
			if items := r.cartOf(t, "buyer_1"); len(items) != 0 {
				t.Fatalf("no line may be added, got %+v", items)
			}
		})
	}
}

// Scenario: A bundle with an unavailable listing is refused
func TestBundleAddDownDomainFailsWithoutWriting(t *testing.T) {
	r := newBundleRig(t)
	r.domain.getErr["lst_b"] = status.Error(codes.Unavailable, "domain down")
	if _, _, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", 1); err == nil {
		t.Fatal("want error")
	}
	if items := r.cartOf(t, "buyer_1"); len(items) != 0 {
		t.Fatalf("no line may be added, got %+v", items)
	}
}

// Scenario: A bundle with a variant listing is refused
func TestBundleAddVariantListingRefused(t *testing.T) {
	r := newBundleRig(t)
	r.domain.setListing("lst_b", func(l *listingv1.Listing) {
		l.Variants = []*listingv1.Variant{{Id: "v1", Name: "Red", Price: 50000, Stock: 5}}
	})
	if _, _, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", 1); err == nil {
		t.Fatal("want error")
	}
	if items := r.cartOf(t, "buyer_1"); len(items) != 0 {
		t.Fatalf("no line may be added, got %+v", items)
	}
}

// Scenario: A bundle priced at or above the sum of its listings is refused
func TestBundleAddPriceNotBelowSumRefused(t *testing.T) {
	for name, price := range map[string]int64{"equal": 150000, "above": 150001} {
		t.Run(name, func(t *testing.T) {
			r := newBundleRig(t)
			r.domain.setBundle("bnd_ab", func(b *listingv1.Bundle) { b.BundlePrice = price })
			if _, _, err := r.cart.AddBundleToCart(context.Background(), "buyer_1", "bnd_ab", 1); err == nil {
				t.Fatal("want error")
			}
			if items := r.cartOf(t, "buyer_1"); len(items) != 0 {
				t.Fatalf("no line may be added, got %+v", items)
			}
		})
	}
}

// Scenario: The cart of another buyer is never touched
func TestBundleAddNeverTouchesAnotherBuyersCart(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_A", "bnd_ab", 1)
	items, _, err := r.cart.GetCart(context.Background(), "buyer_B")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("buyer B cart = %+v, want empty", items)
	}
	if got := len(r.cartOf(t, "buyer_A")); got != 2 {
		t.Fatalf("buyer A lines = %d, want 2", got)
	}
}

// ---------------------------------------------------------------- tag drop

// Scenario: Changing one line quantity drops the tag from all lines
func TestBundleUpdateQuantityDropsTagFromAllLines(t *testing.T) {
	for name, newQty := range map[string]int32{"different quantity": 4, "same quantity": 1} {
		t.Run(name, func(t *testing.T) {
			r := newBundleRig(t)
			items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
			line := byListing(items)["lst_a|bnd_ab"]
			got, subtotal, err := r.cart.UpdateCartItem(context.Background(), "buyer_1", line.ID, newQty)
			if err != nil {
				t.Fatal(err)
			}
			assertNoBundleTag(t, got)
			assertNoBundleTag(t, r.cartOf(t, "buyer_1"))
			want := int64(newQty)*100000 + 50000
			if subtotal != want {
				t.Errorf("subtotal = %d, want %d at regular prices", subtotal, want)
			}
		})
	}
}

// Scenario: Removing one line drops the tag from the others
func TestBundleRemoveLineDropsTagFromOthers(t *testing.T) {
	r := newBundleRig(t)
	items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 2)
	line := byListing(items)["lst_a|bnd_ab"]
	got, subtotal, err := r.cart.RemoveFromCart(context.Background(), "buyer_1", line.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ListingID != "lst_b" || got[0].Quantity != 2 {
		t.Fatalf("remaining = %+v, want lst_b x2 only", got)
	}
	assertNoBundleTag(t, got)
	if subtotal != 100000 {
		t.Errorf("subtotal = %d, want 100000", subtotal)
	}
}

// Scenario: A dropped line merges into the plain line of the same listing
func TestBundleDroppedLineMergesIntoPlainLine(t *testing.T) {
	r := newBundleRig(t)
	if _, _, err := r.cart.AddToCart(context.Background(), "buyer_1", "lst_a", "", 1); err != nil {
		t.Fatal(err)
	}
	items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 2)
	other := byListing(items)["lst_b|bnd_ab"]
	got, _, err := r.cart.UpdateCartItem(context.Background(), "buyer_1", other.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	var a []store.CartItem
	for _, it := range got {
		if it.ListingID == "lst_a" {
			a = append(a, it)
		}
	}
	if len(a) != 1 || a[0].Quantity != 3 || a[0].BundleID != "" || a[0].BundleDiscount != 0 {
		t.Fatalf("lst_a lines = %+v, want one plain line x3", a)
	}
	assertNoBundleTag(t, got)
}

// Scenario: Another buyer's bundle line cannot be changed
func TestBundleAnotherBuyersLineCannotBeChanged(t *testing.T) {
	r := newBundleRig(t)
	items := r.mustAddBundle(t, "buyer_A", "bnd_ab", 1)
	line := byListing(items)["lst_a|bnd_ab"]
	if _, _, err := r.cart.UpdateCartItem(context.Background(), "buyer_B", line.ID, 9); !errors.Is(err, store.ErrCartItemNotFound) {
		t.Errorf("update by B: err = %v, want ErrCartItemNotFound", err)
	}
	if _, _, err := r.cart.RemoveFromCart(context.Background(), "buyer_B", line.ID); !errors.Is(err, store.ErrCartItemNotFound) {
		t.Errorf("remove by B: err = %v, want ErrCartItemNotFound", err)
	}
	after := r.cartOf(t, "buyer_A")
	if len(after) != 2 || discountSum(after) != 30000 {
		t.Fatalf("buyer A bundle changed: %+v", after)
	}
}

// ---------------------------------------------------------------- checkout

func (r *bundleRig) checkout(buyer string, ids ...string) ([]store.Order, error) {
	return r.svc.CreateOrdersFromCart(context.Background(), buyer, addr(), ids, 1, "")
}

func (r *bundleRig) assertNothingReserved(t *testing.T, buyer string) {
	t.Helper()
	if n := r.domain.Reservations(); n != 0 {
		t.Errorf("domain reservations = %d, want 0", n)
	}
	os, err := r.orders.ListBuyerOrders(context.Background(), buyer, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(os) != 0 {
		t.Errorf("orders created = %d, want 0", len(os))
	}
}

// Scenario: The order total reflects the bundle price
// Scenario: The seller's order shows the bundle on its lines
func TestBundleCheckoutTotalAndItems(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)

	prev, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if prev.GetItemsSubtotal() != 120000 {
		t.Errorf("preview items subtotal = %d, want 120000", prev.GetItemsSubtotal())
	}

	orders, err := r.checkout("buyer_1")
	if err != nil || len(orders) != 1 {
		t.Fatalf("checkout: %v %d", err, len(orders))
	}
	got, err := r.orders.GetOrder(context.Background(), orders[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ItemsSubtotal != 120000 || got.TotalAmount != got.ItemsSubtotal+got.ShippingFee {
		t.Errorf("order subtotal/total/fee = %d/%d/%d, want items 120000 and total = items + fee", got.ItemsSubtotal, got.TotalAmount, got.ShippingFee)
	}
	if len(got.Items) != 2 {
		t.Fatalf("order items = %d, want 2", len(got.Items))
	}
	var sum int64
	for _, it := range got.Items {
		if it.BundleID != "bnd_ab" {
			t.Errorf("item %s bundle = %q, want bnd_ab", it.ListingID, it.BundleID)
		}
		sum += it.BundleDiscount
	}
	if sum != 30000 {
		t.Errorf("order item discounts sum = %d, want 30000", sum)
	}
	if left := r.cartOf(t, "buyer_1"); len(left) != 0 {
		t.Errorf("cart not emptied after checkout: %+v", left)
	}
}

// Scenario: A bundle that became invalid is untagged before payment
func TestBundleInvalidAtPreviewIsUntaggedBeforePayment(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	r.domain.setListing("lst_b", func(l *listingv1.Listing) { l.Status = listingv1.ListingStatus_LISTING_STATUS_DRAFT })

	_, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if !errors.Is(err, order.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	items := r.cartOf(t, "buyer_1")
	if len(items) != 2 {
		t.Fatalf("cart lines = %d, want 2", len(items))
	}
	assertNoBundleTag(t, items)
	r.assertNothingReserved(t, "buyer_1")
}

// Scenario: A bundle that became invalid is untagged before payment
func TestBundleUnpublishedListingHiddenByDomainIsUntaggedAtPreview(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	r.domain.mu.Lock()
	r.domain.getErr = map[string]error{"lst_b": status.Error(codes.NotFound, "not_found")}
	r.domain.mu.Unlock()

	_, err := r.svc.PreviewCheckout(context.Background(), "buyer_1", "", nil)
	if !errors.Is(err, order.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	items := r.cartOf(t, "buyer_1")
	if len(items) != 2 {
		t.Fatalf("cart lines = %d, want 2", len(items))
	}
	assertNoBundleTag(t, items)
	r.assertNothingReserved(t, "buyer_1")
}

// Scenario: A bundle that became invalid is untagged before payment
func TestBundleInvalidAtCreateOrderIsUntagged(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	r.domain.bundleErr = status.Error(codes.NotFound, "bundle deleted")

	_, err := r.checkout("buyer_1")
	if !errors.Is(err, order.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	assertNoBundleTag(t, r.cartOf(t, "buyer_1"))
	r.assertNothingReserved(t, "buyer_1")
}

// Scenario: A changed bundle price is untagged at checkout
func TestBundleChangedPriceIsUntaggedAtCheckout(t *testing.T) {
	cases := map[string]func(r *bundleRig){
		"bundle price edited": func(r *bundleRig) {
			r.domain.setBundle("bnd_ab", func(b *listingv1.Bundle) { b.BundlePrice = 110000 })
		},
		"listing price edited": func(r *bundleRig) {
			r.domain.setListing("lst_a", func(l *listingv1.Listing) { l.Price = 90000 })
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := newBundleRig(t)
			r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
			mutate(r)
			_, err := r.checkout("buyer_1")
			if !errors.Is(err, order.ErrPriceChanged) {
				t.Fatalf("err = %v, want ErrPriceChanged", err)
			}
			assertNoBundleTag(t, r.cartOf(t, "buyer_1"))
			r.assertNothingReserved(t, "buyer_1")
		})
	}
}

// Scenario: Checking out only part of a bundle drops the discount
func TestBundlePartialSelectionDropsDiscount(t *testing.T) {
	r := newBundleRig(t)
	items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	line := byListing(items)["lst_a|bnd_ab"]
	_, err := r.checkout("buyer_1", line.ID)
	if !errors.Is(err, order.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	after := r.cartOf(t, "buyer_1")
	if len(after) != 2 {
		t.Fatalf("cart lines = %d, want 2", len(after))
	}
	assertNoBundleTag(t, after)
	r.assertNothingReserved(t, "buyer_1")
}

// Scenario: Checking out only part of a bundle drops the discount
func TestBundleUnequalQuantitiesDropDiscount(t *testing.T) {
	r := newBundleRig(t)
	items := r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	line := byListing(items)["lst_a|bnd_ab"]
	if _, err := r.carts.UpdateItem(context.Background(), "buyer_1", line.ID, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := r.checkout("buyer_1"); !errors.Is(err, order.ErrPriceChanged) {
		t.Fatalf("err = %v, want ErrPriceChanged", err)
	}
	assertNoBundleTag(t, r.cartOf(t, "buyer_1"))
	r.assertNothingReserved(t, "buyer_1")
}

// Scenario: A seller cannot check out their own bundle
func TestBundleSellerCannotCheckOutOwnBundle(t *testing.T) {
	r := newBundleRig(t)
	r.mustAddBundle(t, "s1", "bnd_ab", 1)
	if _, err := r.checkout("s1"); !errors.Is(err, order.ErrSelfPurchase) {
		t.Fatalf("err = %v, want ErrSelfPurchase", err)
	}
	r.assertNothingReserved(t, "s1")
}

// Scenario: A return refunds the allocated bundle price
func TestBundleReturnRefundsOrderTotal(t *testing.T) {
	r := newBundleRig(t)
	returns := r.pg.returns()
	svc := newSvc(t, r.orders, r.carts, returns, nil, r.domain, nil, nil, order.WithSagaRepository(r.pg.sagas()))
	r.mustAddBundle(t, "buyer_1", "bnd_ab", 1)
	orders, err := svc.CreateOrdersFromCart(context.Background(), "buyer_1", addr(), nil, 1, "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("checkout: %v %d", err, len(orders))
	}
	ctx := context.Background()
	id := orders[0].ID
	if _, err := r.orders.UpdateOrderStatusFrom(ctx, id, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.orders.UpdateOrderStatusFrom(ctx, id, store.OrderStatusShipped, []store.OrderStatus{store.OrderStatusPaid}, "T"); err != nil {
		t.Fatal(err)
	}
	total := orders[0].TotalAmount
	if total != 120000+orders[0].ShippingFee {
		t.Fatalf("order total = %d, want bundle price plus shipping", total)
	}
	ret, err := svc.CreateReturnRequest(ctx, "buyer_1", id, "r", 0)
	if err != nil || ret.RefundAmount != total {
		t.Fatalf("refund = %+v %v, want %d", ret, err, total)
	}
	if _, err := svc.CreateReturnRequest(ctx, "buyer_1", id, "r", total+1); !errors.Is(err, order.ErrInvalidRefundAmount) {
		t.Fatalf("above total: err = %v, want ErrInvalidRefundAmount", err)
	}
}

// ---------------------------------------------------------------- logs

// Scenario: No raw bundle or buyer ids in logs (task 1.7: all scenarios)
func TestBundleFlowsLogNoRawBuyerOrBundleIDs(t *testing.T) {
	r := newBundleRig(t)
	const buyer, bundle = "buyer_secret_77", "bnd_secret_99"
	r.domain.mu.Lock()
	r.domain.bundles[bundle] = &listingv1.Bundle{Id: bundle, SellerId: "s1", ListingIds: []string{"lst_a", "lst_b"}, BundlePrice: 120000}
	r.domain.mu.Unlock()
	ctx := context.Background()

	r.mustAddBundle(t, buyer, bundle, 1)
	_, _, _ = r.cart.AddBundleToCart(ctx, buyer, "bnd_missing", 1)
	r.domain.getErr["lst_b"] = status.Error(codes.Unavailable, "down")
	_, _, _ = r.cart.AddBundleToCart(ctx, buyer, bundle, 1)
	delete(r.domain.getErr, "lst_b")
	r.domain.setBundle(bundle, func(b *listingv1.Bundle) { b.BundlePrice = 100000 })
	_, _ = r.checkout(buyer)
	_, _ = r.svc.PreviewCheckout(ctx, buyer, "", nil)

	out := r.logs.String()
	for _, secret := range []string{buyer, bundle} {
		if strings.Contains(out, secret) {
			t.Errorf("logs contain raw id %q:\n%s", secret, out)
		}
	}
}
