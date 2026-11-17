package rpc_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

func sellerOrdersRig(t *testing.T, logger *slog.Logger) (*rpc.OrderHandler, *fake.InMemoryOrderRepository) {
	t.Helper()
	repo := fake.NewInMemoryOrderRepository()
	return newHandler(t, newSvc(t, repo, nil, nil, nil, nil, nil, logger), nil, logger), repo
}

func seedSO(t *testing.T, repo *fake.InMemoryOrderRepository, seller string, st store.OrderStatus, title string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := repo.CreateOrder(context.Background(), store.Order{
			BuyerID: "buyer", SellerID: seller, Status: st,
			Items: []store.OrderItem{{ListingID: "l", Title: title, Quantity: 1, UnitPrice: 1}},
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func sellerCtx(id string) context.Context { return incomingPrincipalCtx(id, "seller") }

func countOf(res *orderv1.ListSellerOrdersResponse, st orderv1.OrderStatus) int64 {
	for _, c := range res.GetStatusCounts() {
		if c.GetStatus() == st {
			return c.GetCount()
		}
	}
	return -1
}

// Scenario: Legacy call without any field
func TestSellerOrdersHandler_LegacyCallWithoutAnyFieldReturnsAllOrdersAndNoPage(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 6)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 6 {
		t.Fatalf("orders = %d, want 6", len(res.GetOrders()))
	}
	if res.GetPage().GetNextCursor() != "" {
		t.Fatalf("next_cursor must be empty")
	}
	if res.GetPage() == nil || res.GetPage().GetTotal() != 6 {
		t.Fatalf("page = %v, want total 6", res.GetPage())
	}
	if len(res.GetStatusCounts()) != 5 {
		t.Fatalf("status_counts = %d entries, want 5", len(res.GetStatusCounts()))
	}
}

// Scenario: Legacy call returns the full list (status_filter only, more than the max page)
func TestSellerOrdersHandler_LegacyStatusFilterReturnsFullListBeyondMaxPage(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 120)
	seedSO(t, repo, "sA", store.OrderStatusShipped, "x", 30)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{StatusFilter: orderv1.OrderStatus_ORDER_STATUS_PAID})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 120 || res.GetPage().GetTotal() != 120 || res.GetPage().GetNextCursor() != "" {
		t.Fatalf("orders=%d page=%v, want 120 total 120 no cursor", len(res.GetOrders()), res.GetPage())
	}
}

// Scenario: statuses wins over status_filter
func TestSellerOrdersHandler_StatusesWinsOverStatusFilter(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 3)
	seedSO(t, repo, "sA", store.OrderStatusShipped, "x", 2)
	seedSO(t, repo, "sA", store.OrderStatusCancelled, "x", 1)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{
		StatusFilter: orderv1.OrderStatus_ORDER_STATUS_CANCELLED,
		Statuses:     []orderv1.OrderStatus{orderv1.OrderStatus_ORDER_STATUS_PAID},
		Page:         &commonv1.PageRequest{},
	})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 3 || res.GetPage().GetTotal() != 3 {
		t.Fatalf("orders=%d total=%d, want 3 3", len(res.GetOrders()), res.GetPage().GetTotal())
	}
	for _, o := range res.GetOrders() {
		if o.GetStatus() != orderv1.OrderStatus_ORDER_STATUS_PAID {
			t.Fatalf("status %v, want PAID", o.GetStatus())
		}
	}
	if countOf(res, orderv1.OrderStatus_ORDER_STATUS_PAID) != 3 || countOf(res, orderv1.OrderStatus_ORDER_STATUS_SHIPPED) != 2 || countOf(res, orderv1.OrderStatus_ORDER_STATUS_CANCELLED) != 1 {
		t.Fatalf("counts must ignore the tab: %v", res.GetStatusCounts())
	}
}

// Scenario: Pages cover the seller's orders exactly once (handler maps page and cursor)
func TestSellerOrdersHandler_PagesCoverTheSellersOrdersExactlyOnce(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 25)
	seen := map[string]bool{}
	cursor := ""
	var sizes []int
	for i := 0; i < 5; i++ {
		res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{Cursor: cursor, PageSize: 10}})
		if err != nil {
			t.Fatalf("ListSellerOrders: %v", err)
		}
		sizes = append(sizes, len(res.GetOrders()))
		if res.GetPage().GetTotal() != 25 {
			t.Fatalf("total = %d, want 25", res.GetPage().GetTotal())
		}
		for _, o := range res.GetOrders() {
			if seen[o.GetId()] {
				t.Fatalf("order %s repeated", o.GetId())
			}
			seen[o.GetId()] = true
		}
		cursor = res.GetPage().GetNextCursor()
		if cursor == "" {
			break
		}
		if _, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(cursor, "=")); err != nil {
			t.Fatalf("cursor %q is not base64url: %v", cursor, err)
		}
	}
	if len(seen) != 25 || len(sizes) != 3 || sizes[0] != 10 || sizes[1] != 10 || sizes[2] != 5 {
		t.Fatalf("seen=%d sizes=%v, want 25 and [10 10 5]", len(seen), sizes)
	}
}

// Scenario: Default page size is 20
func TestSellerOrdersHandler_SetPageWithZeroSizeDefaultsToTwenty(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 30)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 20 || res.GetPage().GetNextCursor() == "" {
		t.Fatalf("orders=%d cursor=%q, want 20 and non-empty", len(res.GetOrders()), res.GetPage().GetNextCursor())
	}
}

// Scenario: Seller orders: Page size above the maximum is clamped
func TestSellerOrdersHandler_PageSizeAboveMaximumIsClamped(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 150)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{PageSize: 1000}})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 100 || res.GetPage().GetNextCursor() == "" {
		t.Fatalf("orders=%d cursor=%q, want 100 and non-empty", len(res.GetOrders()), res.GetPage().GetNextCursor())
	}
}

// Scenario: A negative page size is rejected
func TestSellerOrdersHandler_NegativePageSizeIsRejected(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 3)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{PageSize: -1}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if len(res.GetOrders()) != 0 {
		t.Fatalf("no order may be returned")
	}
}

// Scenario: An unknown cursor is rejected
func TestSellerOrdersHandler_UnknownCursorIsRejected(t *testing.T) {
	for _, c := range []string{"not-a-cursor", "!!!", "bm90LWpzb24"} {
		h, repo := sellerOrdersRig(t, nil)
		seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 3)
		res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{Cursor: c}})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("cursor %q: want InvalidArgument, got %v", c, err)
		}
		if len(res.GetOrders()) != 0 {
			t.Fatalf("no order may be returned")
		}
	}
}

// Scenario: A cursor never reaches another seller's orders
func TestSellerOrdersHandler_CursorNeverReachesAnotherSellersOrders(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 30)
	seedSO(t, repo, "sB", store.OrderStatusPaid, "x", 2)
	a, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{PageSize: 5}})
	if err != nil || a.GetPage().GetNextCursor() == "" {
		t.Fatalf("seller A page: %v cursor=%q", err, a.GetPage().GetNextCursor())
	}
	b, err := h.ListSellerOrders(sellerCtx("sB"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{PageSize: 5, Cursor: a.GetPage().GetNextCursor()}})
	if err != nil {
		t.Fatalf("seller B: %v", err)
	}
	for _, o := range b.GetOrders() {
		if o.GetSellerId() != "sB" {
			t.Fatalf("order of %s leaked to seller B", o.GetSellerId())
		}
	}
}

// Scenario: Seller orders: An unspecified status is rejected
// Scenario: A rejected request returns no counts
func TestSellerOrdersHandler_UnspecifiedStatusIsRejectedWithNoCounts(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 3)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{
		Statuses: []orderv1.OrderStatus{orderv1.OrderStatus_ORDER_STATUS_UNSPECIFIED},
		Page:     &commonv1.PageRequest{},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if len(res.GetOrders()) != 0 || len(res.GetStatusCounts()) != 0 {
		t.Fatalf("rejected request must carry no orders or counts")
	}
}

// Scenario: Seller orders: A query of exactly 100 runes is accepted, 101 is rejected
func TestSellerOrdersHandler_QueryOfExactly100RunesIsAccepted101IsRejected(t *testing.T) {
	h, _ := sellerOrdersRig(t, nil)
	ok, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Query: strings.Repeat("đ", 100), Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("100 runes: %v", err)
	}
	if len(ok.GetOrders()) != 0 {
		t.Fatalf("want 0 orders")
	}
	_, err = h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Query: strings.Repeat("đ", 101), Page: &commonv1.PageRequest{}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("101 runes: want InvalidArgument, got %v", err)
	}
}

// Scenario: Seller orders: A query of exactly 100 runes is accepted, 101 is rejected (length counted after trim)
func TestSellerOrdersHandler_QueryLengthIsCountedAfterTrim(t *testing.T) {
	h, _ := sellerOrdersRig(t, nil)
	_, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Query: "  " + strings.Repeat("a", 100) + "  ", Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("padded 100 runes: %v", err)
	}
}

// Scenario: Seller orders: A blank query is ignored
func TestSellerOrdersHandler_BlankQueryIsIgnored(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 4)
	blank, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Query: "   ", Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("blank: %v", err)
	}
	empty, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(blank.GetOrders()) != 4 || len(blank.GetOrders()) != len(empty.GetOrders()) ||
		blank.GetPage().GetTotal() != empty.GetPage().GetTotal() || len(blank.GetStatusCounts()) != len(empty.GetStatusCounts()) {
		t.Fatalf("blank=%v empty=%v", blank, empty)
	}
}

// Scenario: Seller orders: No match returns an empty page
func TestSellerOrdersHandler_NoMatchReturnsAnEmptyPage(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 4)
	res, err := h.ListSellerOrders(sellerCtx("sA"), &orderv1.ListSellerOrdersRequest{Query: "zzzz-no-such-title", Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 0 || res.GetPage() == nil || res.GetPage().GetTotal() != 0 || res.GetPage().GetNextCursor() != "" {
		t.Fatalf("res = %v", res)
	}
}

// Scenario: A seller with no orders gets an empty page
func TestSellerOrdersHandler_SellerWithNoOrdersGetsFiveZeroCounts(t *testing.T) {
	h, _ := sellerOrdersRig(t, nil)
	res, err := h.ListSellerOrders(sellerCtx("nobody"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 0 || len(res.GetStatusCounts()) != 5 {
		t.Fatalf("orders=%d counts=%v", len(res.GetOrders()), res.GetStatusCounts())
	}
	for _, c := range res.GetStatusCounts() {
		if c.GetCount() != 0 {
			t.Fatalf("count %v, want 0", c)
		}
	}
}

// Scenario: An unauthenticated caller is denied
func TestSellerOrdersHandler_UnauthenticatedCallerIsDenied(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 3)
	res, err := h.ListSellerOrders(context.Background(), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{}})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
	if len(res.GetOrders()) != 0 {
		t.Fatalf("no order may be returned")
	}
}

// Scenario: Another seller's orders are never returned
// Scenario: Counts never include another seller's orders
func TestSellerOrdersHandler_AnotherSellersOrdersAreNeverReturnedOrCounted(t *testing.T) {
	h, repo := sellerOrdersRig(t, nil)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "Ao thun doc nhat", 3)
	seedSO(t, repo, "sB", store.OrderStatusPaid, "other", 2)
	res, err := h.ListSellerOrders(sellerCtx("sB"), &orderv1.ListSellerOrdersRequest{Query: "Ao thun doc nhat", Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if len(res.GetOrders()) != 0 || res.GetPage().GetTotal() != 0 {
		t.Fatalf("leak: %v", res)
	}
	all, err := h.ListSellerOrders(sellerCtx("sB"), &orderv1.ListSellerOrdersRequest{Page: &commonv1.PageRequest{}})
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if countOf(all, orderv1.OrderStatus_ORDER_STATUS_PAID) != 2 {
		t.Fatalf("PAID count = %d, want 2", countOf(all, orderv1.OrderStatus_ORDER_STATUS_PAID))
	}
}

// Scenario: Seller orders: The query text is not logged
func TestSellerOrdersHandler_QueryTextIsNotLogged(t *testing.T) {
	const needle = "secret-needle-title"
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h, repo := sellerOrdersRig(t, logger)
	seedSO(t, repo, "sA", store.OrderStatusPaid, "x", 2)
	for _, req := range []*orderv1.ListSellerOrdersRequest{
		{Query: needle, Page: &commonv1.PageRequest{}},
		{Query: needle, Page: &commonv1.PageRequest{PageSize: -1}},
		{Query: needle, Page: &commonv1.PageRequest{Cursor: "not-a-cursor"}},
		{Query: needle, Statuses: []orderv1.OrderStatus{orderv1.OrderStatus_ORDER_STATUS_UNSPECIFIED}},
		{Query: strings.Repeat(needle, 10)},
	} {
		_, _ = h.ListSellerOrders(sellerCtx("sA"), req)
	}
	if strings.Contains(buf.String(), needle) {
		t.Fatalf("query text leaked into logs: %s", buf.String())
	}
}
