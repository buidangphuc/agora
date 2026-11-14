package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-order/internal/store"
)

const erasedMask = "Đã xóa"

type erasureRig struct {
	t    *testing.T
	pool *pgxpool.Pool
	repo *store.PostgresOrderRepository
}

func newErasureRig(t *testing.T) *erasureRig {
	t.Helper()
	pool := pgPool(t)
	return &erasureRig{t: t, pool: pool, repo: store.NewPostgresOrderRepository(pool, newOutbox(t, pool))}
}

func (r *erasureRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.pool.Exec(context.Background(), q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *erasureRig) addOrder(id, buyer, seller string, status store.OrderStatus, total int64) {
	r.t.Helper()
	addr, _ := json.Marshal(map[string]any{
		"id": "addr-" + id, "user_id": buyer, "recipient_name": "Nguyễn Văn A", "phone": "0901234567",
		"street": "12 Lê Lợi", "ward": "Bến Nghé", "district": "Quận 1", "city": "Hồ Chí Minh", "is_default": true,
	})
	r.exec(`INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, shipping_address) VALUES ($1,$2,$3,$4,$5,$6::jsonb)`,
		id, buyer, seller, int32(status), total, string(addr))
	r.exec(`INSERT INTO order_items (id, order_id, listing_id, title, quantity, unit_price) VALUES ($1,$2,'l-1','Áo thun',2,50000)`, "it-"+id, id)
	r.exec(`INSERT INTO shipments (id, order_id, carrier, tracking_code) VALUES ($1,$2,'SPX',$3)`, "sh-"+id, id, "trk-"+id)
}

func (r *erasureRig) addReturn(id, orderID, buyer, seller string, status store.ReturnStatus) {
	r.t.Helper()
	r.exec(`INSERT INTO order_returns (id, order_id, buyer_id, seller_id, reason, refund_amount, status) VALUES ($1,$2,$3,$4,'Hàng bị vỡ',100000,$5)`,
		id, orderID, buyer, seller, int32(status))
}

func (r *erasureRig) addCart(id, user, listing string) {
	r.t.Helper()
	r.exec(`INSERT INTO cart_items (id, user_id, listing_id, unit_price, title, seller_id) VALUES ($1,$2,$3,1000,'Áo','s-x')`, id, user, listing)
}

func (r *erasureRig) text(q string, args ...any) string {
	r.t.Helper()
	var s string
	if err := r.pool.QueryRow(context.Background(), q, args...).Scan(&s); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	return s
}

func (r *erasureRig) count(q string, args ...any) int {
	r.t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	return n
}

func (r *erasureRig) addrField(orderID, field string) string {
	return r.text(`SELECT shipping_address->>$2 FROM orders WHERE id = $1`, orderID, field)
}

func (r *erasureRig) keptSnapshot() string {
	return r.text(`SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (
		SELECT concat_ws(',', o.id, o.buyer_id, o.seller_id, o.status, o.total_amount, o.currency, o.tracking_number,
			(o.shipping_address - 'recipient_name' - 'phone' - 'street' - 'ward' - 'district' - 'city')::text) AS x FROM orders o
		UNION ALL SELECT concat_ws(',', i.id, i.order_id, i.listing_id, i.title, i.quantity, i.unit_price) FROM order_items i
		UNION ALL SELECT concat_ws(',', s.id, s.order_id, s.tracking_code, s.status) FROM shipments s
		UNION ALL SELECT concat_ws(',', t.id, t.order_id, t.buyer_id, t.seller_id, t.refund_amount, t.status) FROM order_returns t
	) q`)
}

func (r *erasureRig) fullSnapshot() string {
	return r.text(`SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (
		SELECT row_to_json(o)::text AS x FROM orders o
		UNION ALL SELECT row_to_json(t)::text FROM order_returns t
		UNION ALL SELECT row_to_json(c)::text FROM cart_items c
	) q`)
}

func (r *erasureRig) userSnapshot(user string) string {
	return r.text(`SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (
		SELECT row_to_json(o)::text AS x FROM orders o WHERE buyer_id = $1
		UNION ALL SELECT row_to_json(t)::text FROM order_returns t WHERE buyer_id = $1
		UNION ALL SELECT row_to_json(c)::text FROM cart_items c WHERE user_id = $1
	) q`, user)
}

func (r *erasureRig) assertMasked(orderID string) {
	r.t.Helper()
	for _, f := range []string{"recipient_name", "phone", "street", "ward", "district", "city"} {
		if got := r.addrField(orderID, f); got != erasedMask {
			r.t.Fatalf("order %s %s = %q, want %q", orderID, f, got, erasedMask)
		}
	}
}

func (r *erasureRig) assertUnmasked(orderID string) {
	r.t.Helper()
	if got := r.addrField(orderID, "recipient_name"); got != "Nguyễn Văn A" {
		r.t.Fatalf("order %s recipient_name = %q, want unchanged", orderID, got)
	}
	if got := r.addrField(orderID, "phone"); got != "0901234567" {
		r.t.Fatalf("order %s phone = %q, want unchanged", orderID, got)
	}
	if got := r.addrField(orderID, "street"); got != "12 Lê Lợi" {
		r.t.Fatalf("order %s street = %q, want unchanged", orderID, got)
	}
}

func TestEraseUser(t *testing.T) {
	ctx := context.Background()

	t.Run("ORD-1 buyer contact data is masked and accounting data is kept", func(t *testing.T) {
		r := newErasureRig(t)
		for i := 1; i <= 3; i++ {
			r.addOrder(fmt.Sprintf("o-%d", i), "u-1", "s-1", store.OrderStatusPaid, int64(100000*i))
		}
		r.addReturn("ret-1", "o-1", "u-1", "s-1", store.ReturnStatusPending)
		r.addCart("c-1", "u-1", "l-1")
		r.addCart("c-2", "u-1", "l-2")
		kept := r.keptSnapshot()

		if err := r.repo.EraseUser(ctx, "u-1"); err != nil {
			t.Fatalf("EraseUser: %v", err)
		}

		for i := 1; i <= 3; i++ {
			r.assertMasked(fmt.Sprintf("o-%d", i))
		}
		if got := r.text(`SELECT reason FROM order_returns WHERE id = 'ret-1'`); got != erasedMask {
			t.Fatalf("return reason = %q, want %q", got, erasedMask)
		}
		if n := r.count(`SELECT count(*) FROM cart_items WHERE user_id = 'u-1'`); n != 0 {
			t.Fatalf("cart items = %d, want 0", n)
		}
		if after := r.keptSnapshot(); after != kept {
			t.Fatalf("accounting data changed\nbefore: %s\nafter:  %s", kept, after)
		}
	})

	t.Run("ORD-2 another user's data is untouched", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-u", "u-1", "s-1", store.OrderStatusPaid, 1000)
		r.addOrder("o-w", "w-1", "s-1", store.OrderStatusPaid, 2000)
		r.addReturn("ret-w", "o-w", "w-1", "s-1", store.ReturnStatusPending)
		r.addCart("c-w", "w-1", "l-1")
		before := r.userSnapshot("w-1")

		if err := r.repo.EraseUser(ctx, "u-1"); err != nil {
			t.Fatalf("EraseUser: %v", err)
		}

		r.assertMasked("o-u")
		r.assertUnmasked("o-w")
		if after := r.userSnapshot("w-1"); after != before {
			t.Fatalf("other user's rows changed\nbefore: %s\nafter:  %s", before, after)
		}
	})

	t.Run("ORD-4 a user with no data is a no-op", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-w", "w-1", "s-1", store.OrderStatusPaid, 2000)
		r.addReturn("ret-w", "o-w", "w-1", "s-1", store.ReturnStatusPending)
		r.addCart("c-w", "w-1", "l-1")
		before := r.fullSnapshot()

		if err := r.repo.EraseUser(ctx, "nobody"); err != nil {
			t.Fatalf("EraseUser: %v", err)
		}
		if after := r.fullSnapshot(); after != before {
			t.Fatalf("rows changed for a user with no data\nbefore: %s\nafter:  %s", before, after)
		}
	})

	t.Run("ORD-5 a seller's orders keep the buyers' addresses", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-b", "b-1", "u-1", store.OrderStatusPaid, 5000)
		r.addReturn("ret-b", "o-b", "b-1", "u-1", store.ReturnStatusApproved)
		before := r.fullSnapshot()

		if err := r.repo.EraseUser(ctx, "u-1"); err != nil {
			t.Fatalf("EraseUser: %v", err)
		}

		r.assertUnmasked("o-b")
		if got := r.text(`SELECT seller_id FROM orders WHERE id = 'o-b'`); got != "u-1" {
			t.Fatalf("seller_id = %q, want u-1", got)
		}
		if after := r.fullSnapshot(); after != before {
			t.Fatalf("buyer's rows changed when the seller was erased\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func TestEraseUserIdempotent(t *testing.T) {
	r := newErasureRig(t)
	r.addOrder("o-1", "u-1", "s-1", store.OrderStatusPaid, 1000)
	r.addReturn("ret-1", "o-1", "u-1", "s-1", store.ReturnStatusPending)
	r.addCart("c-1", "u-1", "l-1")
	ctx := context.Background()

	if err := r.repo.EraseUser(ctx, "u-1"); err != nil {
		t.Fatalf("first EraseUser: %v", err)
	}
	first := r.fullSnapshot()
	for i := 0; i < 2; i++ {
		if err := r.repo.EraseUser(ctx, "u-1"); err != nil {
			t.Fatalf("repeat EraseUser %d: %v", i, err)
		}
	}
	if again := r.fullSnapshot(); again != first {
		t.Fatalf("ORD-3 repeated erasure changed rows\nfirst: %s\nagain: %s", first, again)
	}
}

func TestEraseUserRollsBack(t *testing.T) {
	r := newErasureRig(t)
	ctx := context.Background()
	r.addOrder("o-1", "u-1", "s-1", store.OrderStatusPaid, 1000)
	r.addReturn("ret-1", "o-1", "u-1", "s-1", store.ReturnStatusPending)
	r.addCart("c-1", "u-1", "l-1")

	r.exec(`CREATE FUNCTION fail_cart_erase() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected commit failure'; END $$`)
	r.exec(`CREATE CONSTRAINT TRIGGER fail_cart_erase AFTER DELETE ON cart_items DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_cart_erase()`)

	if err := r.repo.EraseUser(ctx, "u-1"); err == nil {
		t.Fatal("ORD-7 EraseUser succeeded despite the injected failure")
	}
	r.assertUnmasked("o-1")
	if got := r.text(`SELECT reason FROM order_returns WHERE id = 'ret-1'`); got != "Hàng bị vỡ" {
		t.Fatalf("ORD-7 return reason = %q after rollback, want unchanged", got)
	}
	if n := r.count(`SELECT count(*) FROM cart_items WHERE user_id = 'u-1'`); n != 1 {
		t.Fatalf("ORD-7 cart items = %d after rollback, want 1", n)
	}

	r.exec(`DROP TRIGGER fail_cart_erase ON cart_items`)
	if err := r.repo.EraseUser(ctx, "u-1"); err != nil {
		t.Fatalf("ORD-7 retry after recovery: %v", err)
	}
	r.assertMasked("o-1")
	if n := r.count(`SELECT count(*) FROM cart_items WHERE user_id = 'u-1'`); n != 0 {
		t.Fatalf("ORD-7 cart items = %d after retry, want 0", n)
	}
}

func TestUserObligationsCounts(t *testing.T) {
	ctx := context.Background()

	t.Run("OBL-1 open orders and returns are counted", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-1", "u-1", "s-1", store.OrderStatusPending, 1)
		r.addOrder("o-2", "u-1", "s-1", store.OrderStatusPending, 1)
		r.addOrder("o-3", "u-1", "s-1", store.OrderStatusShipped, 1)
		r.addOrder("o-4", "b-1", "u-1", store.OrderStatusPaid, 1)
		r.addReturn("ret-1", "o-1", "u-1", "s-1", store.ReturnStatusPending)
		got, err := r.repo.UserObligations(ctx, "u-1")
		if err != nil {
			t.Fatalf("UserObligations: %v", err)
		}
		if got.OpenBuyerOrders != 3 || got.OpenSellerOrders != 1 || got.OpenReturns != 1 {
			t.Fatalf("got %+v, want buyer 3 seller 1 returns 1", got)
		}
	})

	t.Run("OBL-2 closed orders and returns are not counted", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-1", "u-1", "s-1", store.OrderStatusCompleted, 1)
		r.addOrder("o-2", "u-1", "s-1", store.OrderStatusCancelled, 1)
		r.addOrder("o-3", "b-1", "u-1", store.OrderStatusCompleted, 1)
		r.addOrder("o-4", "b-1", "u-1", store.OrderStatusCancelled, 1)
		r.addReturn("ret-1", "o-1", "u-1", "s-1", store.ReturnStatusRejected)
		r.addReturn("ret-2", "o-2", "u-1", "s-1", store.ReturnStatusRefunded)
		got, err := r.repo.UserObligations(ctx, "u-1")
		if err != nil {
			t.Fatalf("UserObligations: %v", err)
		}
		if got.OpenBuyerOrders != 0 || got.OpenSellerOrders != 0 || got.OpenReturns != 0 {
			t.Fatalf("got %+v, want all zero", got)
		}
	})

	t.Run("OBL-3 a user with no data reads zeros", func(t *testing.T) {
		r := newErasureRig(t)
		got, err := r.repo.UserObligations(ctx, "nobody")
		if err != nil {
			t.Fatalf("UserObligations: %v", err)
		}
		if got != (store.UserObligations{}) {
			t.Fatalf("got %+v, want zero value", got)
		}
	})

	t.Run("OBL-4 a return counts for both parties", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-1", "u-1", "v-1", store.OrderStatusCompleted, 1)
		r.addReturn("ret-1", "o-1", "u-1", "v-1", store.ReturnStatusApproved)
		for _, id := range []string{"u-1", "v-1"} {
			got, err := r.repo.UserObligations(ctx, id)
			if err != nil {
				t.Fatalf("UserObligations(%s): %v", id, err)
			}
			if got.OpenReturns != 1 {
				t.Fatalf("%s open_returns = %d, want 1", id, got.OpenReturns)
			}
		}
	})

	t.Run("OBL-5 another user's orders are not counted", func(t *testing.T) {
		r := newErasureRig(t)
		r.addOrder("o-u", "u-1", "s-1", store.OrderStatusPaid, 1)
		for i := 1; i <= 4; i++ {
			r.addOrder(fmt.Sprintf("o-w%d", i), "w-1", "s-1", store.OrderStatusPending, 1)
		}
		got, err := r.repo.UserObligations(ctx, "u-1")
		if err != nil {
			t.Fatalf("UserObligations: %v", err)
		}
		if got.OpenBuyerOrders != 1 {
			t.Fatalf("open_buyer_orders = %d, want 1", got.OpenBuyerOrders)
		}
	})
}
