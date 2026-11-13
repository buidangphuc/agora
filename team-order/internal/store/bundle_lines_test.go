package store_test

import (
	"testing"

	"github.com/buidangphuc/team-order/internal/store"
)

// Interface fixed by these tests (order-bundle-checkout T1):
//   CartItem and OrderItem gain BundleID string and BundleDiscount int64.
//   PostgresCartRepository gains
//     AddBundleLines(ctx, userID string, lines []CartItem) ([]CartItem, error)
//       one transaction; each line is upserted on (user, listing, variant, bundle_id);
//       an existing line gets quantity += line.Quantity and its price/discount/title replaced.
//     DropBundleTag(ctx, userID, bundleID string) ([]CartItem, error)
//       clears BundleID and BundleDiscount on all of the user's lines of that bundle;
//       a line colliding with a plain line of the same listing/variant is merged
//       (quantities summed into the plain line, bundle line deleted). Returns the cart.

func bundleLine(user, listing, bundle string, qty int32, price, disc int64) store.CartItem {
	return store.CartItem{UserID: user, ListingID: listing, Quantity: qty, UnitPrice: price,
		Title: "t", SellerID: "s1", BundleID: bundle, BundleDiscount: disc}
}

func findLine(items []store.CartItem, listing, bundle string) (store.CartItem, bool) {
	for _, it := range items {
		if it.ListingID == listing && it.BundleID == bundle {
			return it, true
		}
	}
	return store.CartItem{}, false
}

// Scenario: A bundle line stays separate from a plain line of the same listing.
func TestBundleLineStaysSeparateFromPlainLine(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresCartRepository(pool)
	ctx := t.Context()
	u := uid("u")
	if _, err := repo.AddItem(ctx, store.CartItem{UserID: u, ListingID: "X", Quantity: 1, UnitPrice: 500, Title: "t", SellerID: "s1"}); err != nil {
		t.Fatal(err)
	}
	items, err := repo.AddBundleLines(ctx, u, []store.CartItem{
		bundleLine(u, "X", "B1", 2, 500, 100),
		bundleLine(u, "Y", "B1", 2, 300, 60),
	})
	if err != nil {
		t.Fatalf("AddBundleLines: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("lines = %d, want 3", len(items))
	}
	plain, ok := findLine(items, "X", "")
	if !ok || plain.Quantity != 1 || plain.UnitPrice != 500 || plain.BundleDiscount != 0 {
		t.Fatalf("plain line changed: %+v", plain)
	}
	b, ok := findLine(items, "X", "B1")
	if !ok || b.Quantity != 2 || b.BundleDiscount != 100 {
		t.Fatalf("bundle line wrong: %+v", b)
	}
}

// Scenario: Adding the same bundle again increases every line (store part).
func TestBundleReAddIncreasesEveryLine(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresCartRepository(pool)
	ctx := t.Context()
	u := uid("u")
	if _, err := repo.AddBundleLines(ctx, u, []store.CartItem{
		bundleLine(u, "X", "B1", 1, 500, 50), bundleLine(u, "Y", "B1", 1, 300, 30),
	}); err != nil {
		t.Fatal(err)
	}
	items, err := repo.AddBundleLines(ctx, u, []store.CartItem{
		bundleLine(u, "X", "B1", 2, 500, 150), bundleLine(u, "Y", "B1", 2, 300, 90),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("lines = %d, want 2", len(items))
	}
	x, _ := findLine(items, "X", "B1")
	y, _ := findLine(items, "Y", "B1")
	if x.Quantity != 3 || y.Quantity != 3 {
		t.Fatalf("quantities = %d,%d want 3,3", x.Quantity, y.Quantity)
	}
	if x.BundleDiscount != 150 || y.BundleDiscount != 90 {
		t.Fatalf("discounts = %d,%d want recomputed 150,90", x.BundleDiscount, y.BundleDiscount)
	}
}

// Scenario: A dropped line merges into the plain line of the same listing.
func TestBundleDropTagMergesIntoPlainLine(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresCartRepository(pool)
	ctx := t.Context()
	u := uid("u")
	if _, err := repo.AddItem(ctx, store.CartItem{UserID: u, ListingID: "X", Quantity: 1, UnitPrice: 500, Title: "t", SellerID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddBundleLines(ctx, u, []store.CartItem{
		bundleLine(u, "X", "B1", 2, 500, 100), bundleLine(u, "Y", "B1", 2, 300, 60),
	}); err != nil {
		t.Fatal(err)
	}
	items, err := repo.DropBundleTag(ctx, u, "B1")
	if err != nil {
		t.Fatalf("DropBundleTag: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("lines = %d, want 2", len(items))
	}
	x, ok := findLine(items, "X", "")
	if !ok || x.Quantity != 3 || x.BundleDiscount != 0 {
		t.Fatalf("merged X wrong: %+v", x)
	}
	y, ok := findLine(items, "Y", "")
	if !ok || y.Quantity != 2 || y.BundleDiscount != 0 {
		t.Fatalf("untagged Y wrong: %+v", y)
	}
	for _, it := range items {
		if it.BundleID != "" {
			t.Fatalf("tag remains: %+v", it)
		}
	}
}

// Scenario: another buyer's bundle line is never touched by a tag drop.
func TestBundleDropTagOnlyAffectsCaller(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresCartRepository(pool)
	ctx := t.Context()
	a, b := uid("a"), uid("b")
	for _, u := range []string{a, b} {
		if _, err := repo.AddBundleLines(ctx, u, []store.CartItem{
			bundleLine(u, "X", "B1", 1, 500, 50), bundleLine(u, "Y", "B1", 1, 300, 30),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.DropBundleTag(ctx, b, "B1"); err != nil {
		t.Fatal(err)
	}
	cart, err := repo.GetCart(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if x, ok := findLine(cart, "X", "B1"); !ok || x.BundleDiscount != 50 {
		t.Fatalf("buyer a's bundle line changed: %+v", cart)
	}
}

// Scenario: A plain cart is priced as before (plain AddItem merges as before, no bundle data).
func TestBundlePlainAddItemUnchanged(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresCartRepository(pool)
	ctx := t.Context()
	u := uid("u")
	for i := 0; i < 2; i++ {
		if _, err := repo.AddItem(ctx, store.CartItem{UserID: u, ListingID: "X", Quantity: 1, UnitPrice: 500, Title: "t", SellerID: "s1"}); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := repo.GetCart(ctx, u)
	if len(items) != 1 || items[0].Quantity != 2 || items[0].BundleID != "" || items[0].BundleDiscount != 0 {
		t.Fatalf("plain cart changed: %+v", items)
	}
}

// Scenario: the order seller sees the bundle on order lines (store part): bundle columns round-trip.
func TestBundleOrderItemColumnsRoundTrip(t *testing.T) {
	pool := pgPool(t)
	repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool))
	ctx := t.Context()
	created, err := repo.CreateOrder(ctx, store.Order{
		BuyerID: uid("b"), SellerID: "s1", TotalAmount: 900,
		Items: []store.OrderItem{
			{ListingID: "X", Title: "t", Quantity: 2, UnitPrice: 500, BundleID: "B1", BundleDiscount: 100},
			{ListingID: "Z", Title: "t", Quantity: 1, UnitPrice: 300},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetOrder(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var tagged, plain int
	for _, it := range got.Items {
		switch it.ListingID {
		case "X":
			if it.BundleID != "B1" || it.BundleDiscount != 100 {
				t.Fatalf("bundle item lost columns: %+v", it)
			}
			tagged++
		case "Z":
			if it.BundleID != "" || it.BundleDiscount != 0 {
				t.Fatalf("plain item got bundle data: %+v", it)
			}
			plain++
		}
	}
	if tagged != 1 || plain != 1 {
		t.Fatalf("items = %+v", got.Items)
	}
}
