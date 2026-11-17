package store_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

// Task 1 (order-seller-list-paging). The repository API these tests pin:
//
//	type SellerOrdersCursor struct { CreatedAt time.Time; ID string }
//	type SellerOrdersFilter struct {
//	    Statuses []OrderStatus      // empty = every status; no UNSPECIFIED
//	    Query    string             // already trimmed by the caller; "" = no search
//	    Paged    bool               // false = legacy full list
//	    PageSize int32              // paged only: 0 -> 20, above 100 -> 100
//	    Cursor   *SellerOrdersCursor // paged only: rows strictly after (created_at, id) desc
//	}
//	type StatusCount struct { Status OrderStatus; Count int64 }
//	type SellerOrdersResult struct {
//	    Orders []Order               // items loaded
//	    Next   *SellerOrdersCursor   // (created_at, id) of the last order; nil unless more follow
//	    Total  int64                 // orders matching Statuses and Query
//	    Counts []StatusCount         // always 5 entries PENDING..CANCELLED, Statuses ignored, Query applied
//	}
//	OrderRepository.ListSellerOrders(ctx, sellerID string, f SellerOrdersFilter) (SellerOrdersResult, error)

type sellerRepoEnv struct {
	repo store.OrderRepository
	pool *pgxpool.Pool
}

func sellerRepoFactories() map[string]func(t *testing.T) sellerRepoEnv {
	return map[string]func(t *testing.T) sellerRepoEnv{
		"memory": func(*testing.T) sellerRepoEnv { return sellerRepoEnv{repo: fake.NewInMemoryOrderRepository()} },
		"postgres": func(t *testing.T) sellerRepoEnv {
			pool := pgPool(t)
			return sellerRepoEnv{repo: store.NewPostgresOrderRepository(pool, newOutbox(t, pool)), pool: pool}
		},
	}
}

func forEachSellerRepo(t *testing.T, fn func(t *testing.T, env sellerRepoEnv)) {
	t.Helper()
	for name, mk := range sellerRepoFactories() {
		t.Run(name, func(t *testing.T) { fn(t, mk(t)) })
	}
}

func seedSellerOrder(t *testing.T, repo store.OrderRepository, id, seller string, st store.OrderStatus, titles ...string) store.Order {
	t.Helper()
	o := store.Order{ID: id, BuyerID: "buyer", SellerID: seller, Status: st, TotalAmount: 1000}
	for i, title := range titles {
		o.Items = append(o.Items, store.OrderItem{ListingID: fmt.Sprintf("l-%s-%d", id, i), VariantID: fmt.Sprintf("v-%s-%d", id, i), Title: title, Quantity: 1, UnitPrice: 1000})
	}
	created, err := repo.CreateOrder(t.Context(), o)
	if err != nil {
		t.Fatalf("seed order %s: %v", id, err)
	}
	return created
}

func seedMany(t *testing.T, repo store.OrderRepository, seller string, n int, st func(i int) store.OrderStatus) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedSellerOrder(t, repo, fmt.Sprintf("%s-o%04d", seller, i), seller, st(i), fmt.Sprintf("item %d", i))
	}
}

func orderIDs(os []store.Order) []string {
	ids := make([]string, len(os))
	for i, o := range os {
		ids[i] = o.ID
	}
	sort.Strings(ids)
	return ids
}

func wantIDs(t *testing.T, got []store.Order, want ...string) {
	t.Helper()
	sort.Strings(want)
	g := orderIDs(got)
	if fmt.Sprint(g) != fmt.Sprint(want) {
		t.Fatalf("orders = %v, want %v", g, want)
	}
}

func wantCounts(t *testing.T, got []store.StatusCount, pending, paid, shipped, completed, cancelled int64) {
	t.Helper()
	want := []store.StatusCount{
		{Status: store.OrderStatusPending, Count: pending},
		{Status: store.OrderStatusPaid, Count: paid},
		{Status: store.OrderStatusShipped, Count: shipped},
		{Status: store.OrderStatusCompleted, Count: completed},
		{Status: store.OrderStatusCancelled, Count: cancelled},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("status counts = %v, want %v", got, want)
	}
}

func wantNewestFirst(t *testing.T, os []store.Order) {
	t.Helper()
	for i := 1; i < len(os); i++ {
		a, b := os[i-1], os[i]
		if b.CreatedAt.After(a.CreatedAt) || (b.CreatedAt.Equal(a.CreatedAt) && b.ID > a.ID) {
			t.Fatalf("order %d (%s %v) is newer than order %d (%s %v)", i, b.ID, b.CreatedAt, i-1, a.ID, a.CreatedAt)
		}
	}
}

func mixedSeller(t *testing.T, repo store.OrderRepository, seller string) {
	t.Helper()
	for i := 0; i < 3; i++ {
		seedSellerOrder(t, repo, fmt.Sprintf("%s-paid%d", seller, i), seller, store.OrderStatusPaid, "Áo thun Đỏ")
	}
	for i := 0; i < 2; i++ {
		seedSellerOrder(t, repo, fmt.Sprintf("%s-ship%d", seller, i), seller, store.OrderStatusShipped, "Quần jean")
	}
	seedSellerOrder(t, repo, seller+"-canc0", seller, store.OrderStatusCancelled, "Quần jean")
}

func TestListSellerOrdersPaged_PagesCoverTheSellersOrdersExactlyOnce(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 250, func(int) store.OrderStatus { return store.OrderStatusPaid })
		var sizes []int
		seen := map[string]bool{}
		var last time.Time
		f := store.SellerOrdersFilter{Paged: true, PageSize: 100}
		for i := 0; i < 5; i++ {
			res, err := env.repo.ListSellerOrders(t.Context(), "sA", f)
			if err != nil {
				t.Fatal(err)
			}
			sizes = append(sizes, len(res.Orders))
			if res.Total != 250 {
				t.Fatalf("page %d total = %d, want 250", i, res.Total)
			}
			wantNewestFirst(t, res.Orders)
			for _, o := range res.Orders {
				if seen[o.ID] {
					t.Fatalf("order %s returned twice", o.ID)
				}
				seen[o.ID] = true
				if !last.IsZero() && o.CreatedAt.After(last) {
					t.Fatalf("created_at increased across pages at %s", o.ID)
				}
				last = o.CreatedAt
			}
			if res.Next == nil {
				break
			}
			f.Cursor = res.Next
		}
		if fmt.Sprint(sizes) != "[100 100 50]" {
			t.Fatalf("page sizes = %v, want [100 100 50]", sizes)
		}
		if len(seen) != 250 {
			t.Fatalf("distinct orders = %d, want 250", len(seen))
		}
	})
}

func TestListSellerOrdersPaged_NextIsEmptyOnTheLastFullPage(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 20, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, PageSize: 20})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 20 || res.Next != nil {
			t.Fatalf("got %d orders next=%v, want 20 and no next", len(res.Orders), res.Next)
		}
	})
}

func TestListSellerOrdersPaged_NextIsTheLastReturnedOrder(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 5, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, PageSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 2 || res.Next == nil {
			t.Fatalf("got %d orders next=%v", len(res.Orders), res.Next)
		}
		last := res.Orders[1]
		if res.Next.ID != last.ID || !res.Next.CreatedAt.Equal(last.CreatedAt) {
			t.Fatalf("next = %+v, want (%v, %s)", res.Next, last.CreatedAt, last.ID)
		}
	})
}

func TestListSellerOrdersPaged_EqualCreatedAtPagesByIDDescending(t *testing.T) {
	env := sellerRepoFactories()["postgres"](t)
	seedMany(t, env.repo, "sA", 7, func(int) store.OrderStatus { return store.OrderStatusPaid })
	if _, err := env.pool.Exec(t.Context(), `UPDATE orders SET created_at = '2026-01-01T00:00:00Z' WHERE seller_id = 'sA'`); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var order []string
	f := store.SellerOrdersFilter{Paged: true, PageSize: 3}
	for i := 0; i < 5; i++ {
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", f)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range res.Orders {
			if seen[o.ID] {
				t.Fatalf("order %s repeated", o.ID)
			}
			seen[o.ID] = true
			order = append(order, o.ID)
		}
		if res.Next == nil {
			break
		}
		f.Cursor = res.Next
	}
	if len(order) != 7 || !sort.SliceIsSorted(order, func(i, j int) bool { return order[i] > order[j] }) {
		t.Fatalf("order = %v, want 7 ids descending", order)
	}
}

func TestListSellerOrdersPaged_DefaultPageSizeIs20(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 30, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 20 || res.Next == nil || res.Total != 30 {
			t.Fatalf("got %d orders next=%v total=%d, want 20, set, 30", len(res.Orders), res.Next, res.Total)
		}
	})
}

func TestListSellerOrdersPaged_PageSizeAboveTheMaximumIsClamped(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 150, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, PageSize: 1000})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 100 || res.Next == nil {
			t.Fatalf("got %d orders next=%v, want 100 and a next cursor", len(res.Orders), res.Next)
		}
	})
}

func TestListSellerOrdersPaged_ACursorNeverReachesAnotherSellersOrders(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 30, func(int) store.OrderStatus { return store.OrderStatusPaid })
		seedMany(t, env.repo, "sB", 2, func(int) store.OrderStatus { return store.OrderStatusPaid })
		a, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, PageSize: 5})
		if err != nil || a.Next == nil {
			t.Fatalf("seller A page: next=%v err=%v", a.Next, err)
		}
		b, err := env.repo.ListSellerOrders(t.Context(), "sB", store.SellerOrdersFilter{Paged: true, PageSize: 5, Cursor: a.Next})
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range b.Orders {
			if o.SellerID != "sB" {
				t.Fatalf("order %s of %s returned to seller B", o.ID, o.SellerID)
			}
		}
	})
}

func TestListSellerOrdersPaged_ASellerWithNoOrdersGetsAnEmptyPage(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 3, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "nobody", store.SellerOrdersFilter{Paged: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 0 || res.Total != 0 || res.Next != nil {
			t.Fatalf("got %d orders total=%d next=%v, want empty", len(res.Orders), res.Total, res.Next)
		}
		wantCounts(t, res.Counts, 0, 0, 0, 0, 0)
	})
}

func TestListSellerOrdersPaged_LoadsTheItemsOfEveryPageOrder(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "sA-one", "sA", store.OrderStatusPaid, "Alpha")
		seedSellerOrder(t, env.repo, "sA-two", "sA", store.OrderStatusPaid, "Beta", "Gamma")
		seedSellerOrder(t, env.repo, "sA-none", "sA", store.OrderStatusPaid)
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true})
		if err != nil {
			t.Fatal(err)
		}
		titles := map[string][]string{}
		for _, o := range res.Orders {
			for _, it := range o.Items {
				if it.OrderID != o.ID {
					t.Fatalf("item %s of order %s carries order id %s", it.ID, o.ID, it.OrderID)
				}
				titles[o.ID] = append(titles[o.ID], it.Title)
			}
		}
		sort.Strings(titles["sA-two"])
		if fmt.Sprint(titles) != "map[sA-one:[Alpha] sA-two:[Beta Gamma]]" {
			t.Fatalf("items by order = %v", titles)
		}
	})
}

func TestListSellerOrdersStatuses_OneStatusTab(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		mixedSeller(t, env.repo, "sA")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Statuses: []store.OrderStatus{store.OrderStatusPaid}})
		if err != nil {
			t.Fatal(err)
		}
		wantIDs(t, res.Orders, "sA-paid0", "sA-paid1", "sA-paid2")
		if res.Total != 3 {
			t.Fatalf("total = %d, want 3", res.Total)
		}
	})
}

func TestListSellerOrdersStatuses_SeveralStatusesAreOred(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		mixedSeller(t, env.repo, "sA")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Statuses: []store.OrderStatus{store.OrderStatusShipped, store.OrderStatusCancelled}})
		if err != nil {
			t.Fatal(err)
		}
		wantIDs(t, res.Orders, "sA-ship0", "sA-ship1", "sA-canc0")
		if res.Total != 3 {
			t.Fatalf("total = %d, want 3", res.Total)
		}
	})
}

func TestListSellerOrdersQuery_ItemTitleSubstring(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "sA-shirt", "sA", store.OrderStatusPaid, "Áo thun Đỏ")
		seedSellerOrder(t, env.repo, "sA-jeans", "sA", store.OrderStatusPaid, "Quần jean")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Query: "áo thun"})
		if err != nil {
			t.Fatal(err)
		}
		wantIDs(t, res.Orders, "sA-shirt")
		if res.Total != 1 {
			t.Fatalf("total = %d, want 1", res.Total)
		}
	})
}

func TestListSellerOrdersQuery_OrderIDPrefix(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "ABCD1234-0001", "sA", store.OrderStatusPaid, "x")
		seedSellerOrder(t, env.repo, "abcd1234-0002", "sA", store.OrderStatusPaid, "y")
		seedSellerOrder(t, env.repo, "zzzz9999-0003", "sA", store.OrderStatusPaid, "z")
		seedSellerOrder(t, env.repo, "xx-abcd1234-4", "sA", store.OrderStatusPaid, "w")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Query: "abcd1234"})
		if err != nil {
			t.Fatal(err)
		}
		wantIDs(t, res.Orders, "ABCD1234-0001", "abcd1234-0002")
	})
}

func TestListSellerOrdersQuery_CombinesWithStatusesUsingAnd(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "sA-paid", "sA", store.OrderStatusPaid, "Áo thun")
		seedSellerOrder(t, env.repo, "sA-ship", "sA", store.OrderStatusShipped, "Áo thun")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Query: "áo", Statuses: []store.OrderStatus{store.OrderStatusShipped}})
		if err != nil {
			t.Fatal(err)
		}
		wantIDs(t, res.Orders, "sA-ship")
		if res.Total != 1 {
			t.Fatalf("total = %d, want 1", res.Total)
		}
	})
}

func TestListSellerOrdersQuery_LikeWildcardsMatchLiterally(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "sA-pct", "sA", store.OrderStatusPaid, "Giảm 50% áo")
		seedSellerOrder(t, env.repo, "sA-500", "sA", store.OrderStatusPaid, "Giảm 500 áo")
		seedSellerOrder(t, env.repo, "sA-us", "sA", store.OrderStatusPaid, "size a_b")
		seedSellerOrder(t, env.repo, "sA-ax", "sA", store.OrderStatusPaid, "size axb")
		seedSellerOrder(t, env.repo, "sA-bs", "sA", store.OrderStatusPaid, `path c\d`)
		seedSellerOrder(t, env.repo, "sA-cd", "sA", store.OrderStatusPaid, "path cd")
		for _, tc := range []struct {
			query string
			want  []string
		}{
			{"50%", []string{"sA-pct"}},
			{"a_b", []string{"sA-us"}},
			{`c\d`, []string{"sA-bs"}},
			{"%", []string{"sA-pct"}},
			{"_", []string{"sA-us"}},
		} {
			res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Query: tc.query})
			if err != nil {
				t.Fatalf("query %q: %v", tc.query, err)
			}
			wantIDs(t, res.Orders, tc.want...)
		}
	})
}

func TestListSellerOrdersQuery_NeverMatchesAnotherSellersOrders(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "sA-secret", "sA", store.OrderStatusPaid, "Unique needle title")
		for _, q := range []string{"Unique needle title", "sA-secret"} {
			res, err := env.repo.ListSellerOrders(t.Context(), "sB", store.SellerOrdersFilter{Paged: true, Query: q})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Orders) != 0 || res.Total != 0 {
				t.Fatalf("query %q gave seller B %d orders total %d", q, len(res.Orders), res.Total)
			}
		}
	})
}

func TestListSellerOrdersCounts_IgnoreTheSelectedTab(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		mixedSeller(t, env.repo, "sA")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Statuses: []store.OrderStatus{store.OrderStatusPaid}})
		if err != nil {
			t.Fatal(err)
		}
		wantCounts(t, res.Counts, 0, 3, 2, 0, 1)
	})
}

func TestListSellerOrdersCounts_FollowTheQuery(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedSellerOrder(t, env.repo, "sA-1", "sA", store.OrderStatusPaid, "Áo thun")
		seedSellerOrder(t, env.repo, "sA-2", "sA", store.OrderStatusShipped, "Áo khoác")
		seedSellerOrder(t, env.repo, "sA-3", "sA", store.OrderStatusPaid, "Quần jean")
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Paged: true, Query: "áo", Statuses: []store.OrderStatus{store.OrderStatusPaid}})
		if err != nil {
			t.Fatal(err)
		}
		wantCounts(t, res.Counts, 0, 1, 1, 0, 0)
	})
}

func TestListSellerOrdersCounts_NeverIncludeAnotherSellersOrders(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 3, func(int) store.OrderStatus { return store.OrderStatusPaid })
		seedMany(t, env.repo, "sB", 2, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "sB", store.SellerOrdersFilter{Paged: true})
		if err != nil {
			t.Fatal(err)
		}
		wantCounts(t, res.Counts, 0, 2, 0, 0, 0)
	})
}

func TestListSellerOrdersLegacy_UnpagedCallReturnsTheFullList(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		seedMany(t, env.repo, "sA", 250, func(i int) store.OrderStatus {
			if i < 120 {
				return store.OrderStatusPaid
			}
			return store.OrderStatusShipped
		})
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{Statuses: []store.OrderStatus{store.OrderStatusPaid}})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 120 || res.Next != nil || res.Total != 120 {
			t.Fatalf("got %d orders next=%v total=%d, want 120, none, 120", len(res.Orders), res.Next, res.Total)
		}
		wantNewestFirst(t, res.Orders)
		wantCounts(t, res.Counts, 0, 120, 130, 0, 0)
	})
}

func TestListSellerOrdersLegacy_EmptyFilterReturnsEveryOwnOrder(t *testing.T) {
	forEachSellerRepo(t, func(t *testing.T, env sellerRepoEnv) {
		mixedSeller(t, env.repo, "sA")
		seedMany(t, env.repo, "sB", 2, func(int) store.OrderStatus { return store.OrderStatusPaid })
		res, err := env.repo.ListSellerOrders(t.Context(), "sA", store.SellerOrdersFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Orders) != 6 || res.Next != nil || res.Total != 6 {
			t.Fatalf("got %d orders next=%v total=%d, want 6, none, 6", len(res.Orders), res.Next, res.Total)
		}
		wantNewestFirst(t, res.Orders)
		for _, o := range res.Orders {
			if o.SellerID != "sA" || len(o.Items) == 0 {
				t.Fatalf("order %s seller %s items %d", o.ID, o.SellerID, len(o.Items))
			}
		}
	})
}
