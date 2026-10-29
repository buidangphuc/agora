package store_test

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

// Task 2 (order-multishop-checkout). The repository API these tests pin:
//   - Order gains CheckoutID (string), PlatformVoucherCode (string), PlatformDiscountAmount (int64).
//   - ListOrdersByCheckout(ctx, checkoutID string) ([]Order, error) on the Postgres and
//     in-memory order repositories: every order carrying that checkout_id, none for an
//     unknown or empty id.
//   - migration 0010 adds the three columns, defaulting to '' / '' / 0 for existing rows.

type checkoutRepo interface {
	store.OrderRepository
	ListOrdersByCheckout(ctx context.Context, checkoutID string) ([]store.Order, error)
}

func checkoutRepoFactories() map[string]func(t *testing.T) checkoutRepo {
	return map[string]func(t *testing.T) checkoutRepo{
		"memory": func(t *testing.T) checkoutRepo { return fake.NewInMemoryOrderRepository() },
		"postgres": func(t *testing.T) checkoutRepo {
			pool := pgPool(t)
			return store.NewPostgresOrderRepository(pool, newOutbox(t, pool))
		},
	}
}

func checkoutOrder(buyer, seller, checkoutID, platformCode string, platformDiscount int64) store.Order {
	return store.Order{
		BuyerID:                buyer,
		SellerID:               seller,
		TotalAmount:            90000,
		ItemsSubtotal:          100000,
		DiscountAmount:         10000,
		CheckoutID:             checkoutID,
		PlatformVoucherCode:    platformCode,
		PlatformDiscountAmount: platformDiscount,
		Items: []store.OrderItem{
			{ListingID: uid("lst"), Title: "item", Quantity: 1, UnitPrice: 100000},
		},
	}
}

func checkoutIDs(orders []store.Order) []string {
	out := make([]string, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.ID)
	}
	sort.Strings(out)
	return out
}

func TestListOrdersByCheckout(t *testing.T) {
	for name, mk := range checkoutRepoFactories() {
		t.Run("Multishop checkout: The order repository returns checkout siblings/"+name, func(t *testing.T) {
			ctx := context.Background()
			repo := mk(t)
			buyer := uid("buyer")
			checkoutID := uid("chk")
			otherCheckout := uid("chk")

			var want []string
			for i := 0; i < 3; i++ {
				o, err := repo.CreateOrder(ctx, checkoutOrder(buyer, uid("seller"), checkoutID, "PLAT10", int64(1000*(i+1))))
				if err != nil {
					t.Fatalf("create sibling: %v", err)
				}
				want = append(want, o.ID)
			}
			if _, err := repo.CreateOrder(ctx, checkoutOrder(buyer, uid("seller"), otherCheckout, "", 0)); err != nil {
				t.Fatalf("create other-checkout order: %v", err)
			}
			if _, err := repo.CreateOrder(ctx, checkoutOrder(buyer, uid("seller"), "", "", 0)); err != nil {
				t.Fatalf("create legacy order: %v", err)
			}
			sort.Strings(want)

			got, err := repo.ListOrdersByCheckout(ctx, checkoutID)
			if err != nil {
				t.Fatalf("ListOrdersByCheckout: %v", err)
			}
			gotIDs := checkoutIDs(got)
			if len(gotIDs) != 3 {
				t.Fatalf("siblings = %v, want %v", gotIDs, want)
			}
			for i := range want {
				if gotIDs[i] != want[i] {
					t.Fatalf("siblings = %v, want %v", gotIDs, want)
				}
			}
			for _, o := range got {
				if o.CheckoutID != checkoutID || o.PlatformVoucherCode != "PLAT10" || o.PlatformDiscountAmount == 0 {
					t.Fatalf("sibling lost checkout fields: %+v", o)
				}
				if len(o.Items) != 1 {
					t.Fatalf("sibling %s items = %d, want 1", o.ID, len(o.Items))
				}
			}

			none, err := repo.ListOrdersByCheckout(ctx, uid("unknown"))
			if err != nil {
				t.Fatalf("ListOrdersByCheckout unknown: %v", err)
			}
			if len(none) != 0 {
				t.Fatalf("unknown checkout returned %d orders, want 0", len(none))
			}
			empty, err := repo.ListOrdersByCheckout(ctx, "")
			if err != nil {
				t.Fatalf("ListOrdersByCheckout empty: %v", err)
			}
			if len(empty) != 0 {
				t.Fatalf("empty checkout id returned %d orders, want 0", len(empty))
			}
		})
	}
}

func TestOrderCheckoutFieldsRoundTrip(t *testing.T) {
	for name, mk := range checkoutRepoFactories() {
		t.Run("Multishop checkout: The order repository returns checkout siblings (fields persist)/"+name, func(t *testing.T) {
			ctx := context.Background()
			repo := mk(t)
			chk := uid("chk")
			created, err := repo.CreateOrder(ctx, checkoutOrder(uid("buyer"), uid("seller"), chk, "PLAT10", 2500))
			if err != nil {
				t.Fatal(err)
			}
			got, err := repo.GetOrder(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.CheckoutID != chk || got.PlatformVoucherCode != "PLAT10" || got.PlatformDiscountAmount != 2500 {
				t.Fatalf("GetOrder checkout fields = (%q, %q, %d)", got.CheckoutID, got.PlatformVoucherCode, got.PlatformDiscountAmount)
			}
			buyerOrders, err := repo.ListBuyerOrders(ctx, created.BuyerID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(buyerOrders) != 1 || buyerOrders[0].CheckoutID != chk || buyerOrders[0].PlatformDiscountAmount != 2500 {
				t.Fatalf("ListBuyerOrders lost checkout fields: %+v", buyerOrders)
			}
			sellerOrders, err := repo.ListSellerOrders(ctx, created.SellerID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(sellerOrders) != 1 || sellerOrders[0].PlatformVoucherCode != "PLAT10" {
				t.Fatalf("ListSellerOrders lost checkout fields: %+v", sellerOrders)
			}
		})
	}
}

func TestOrderLegacyRowDefaultsNewColumns(t *testing.T) {
	t.Run("Multishop checkout: Order rows stay readable before the migration data exists", func(t *testing.T) {
		ctx := context.Background()
		pool := pgPool(t)
		repo := store.NewPostgresOrderRepository(pool, newOutbox(t, pool))

		id := uid("legacy")
		addr, _ := json.Marshal(store.Address{})
		now := time.Now()
		const q = `INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, currency, shipping_address, tracking_number, created_at, updated_at, shipping_fee, items_subtotal, payment_method, voucher_code, discount_amount)
			VALUES ($1, $2, $3, 1, 50000, 'VND', $4, '', $5, $5, 0, 50000, 0, '', 0)`
		if _, err := pool.Exec(ctx, q, id, uid("buyer"), uid("seller"), addr, now); err != nil {
			t.Fatalf("insert legacy row without new columns: %v", err)
		}

		got, err := repo.GetOrder(ctx, id)
		if err != nil {
			t.Fatalf("GetOrder legacy row: %v", err)
		}
		if got.CheckoutID != "" || got.PlatformVoucherCode != "" || got.PlatformDiscountAmount != 0 {
			t.Fatalf("legacy defaults = (%q, %q, %d), want empty/empty/0", got.CheckoutID, got.PlatformVoucherCode, got.PlatformDiscountAmount)
		}
		list, err := repo.ListBuyerOrders(ctx, got.BuyerID, 0)
		if err != nil || len(list) != 1 {
			t.Fatalf("ListBuyerOrders legacy = %v, %v", list, err)
		}
		if siblings, err := repo.ListOrdersByCheckout(ctx, ""); err != nil || len(siblings) != 0 {
			t.Fatalf("legacy row must not match an empty checkout id: %v, %v", siblings, err)
		}
	})
}
