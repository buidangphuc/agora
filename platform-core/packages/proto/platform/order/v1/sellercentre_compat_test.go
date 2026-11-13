package orderv1_test

// Tests for change seller-centre-contract (order/v1 part, tasks 2.2). Same
// approach as lifecycle_compat_test.go: schema from `buf build` JSON and the
// schema-driven wire codec from that file. Only the field numbers listed in
// the spec are pinned; nothing above them is asserted.

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

const (
	scPageReqT  = ".platform.common.v1.PageRequest"
	scPageRespT = ".platform.common.v1.PageResponse"
	scStatusT   = orderPkg + "OrderStatus"
	scCountT    = orderPkg + "OrderStatusCount"
)

func scRepeated(name string, n int, typ, typeName string) wf {
	return wf{name, n, typ, typeName}
}

func scRequireRepeated(t *testing.T, m msgDesc, name string) {
	t.Helper()
	if !repeated(m, name) {
		t.Errorf("%s.%s must be repeated", m.Name, name)
	}
}

func scVarintField(num int, v uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(num)<<3), v)
}

// Scenario: Seller centre contract: AddBundleToCart RPC exists
func TestSellerCentreContract_AddBundleToCartRPCExists(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	svc, _ := f.service(t, "CartService")
	got := map[string]methodDesc{}
	for _, m := range svc.Method {
		got[m.Name] = m
	}
	for _, name := range []string{"GetCart", "AddToCart", "UpdateCartItem", "RemoveFromCart", "ClearCart", "Reorder"} {
		m, ok := got[name]
		if !ok {
			t.Errorf("existing RPC %s missing", name)
			continue
		}
		if m.InputType != orderPkg+name+"Request" || m.OutputType != orderPkg+name+"Response" {
			t.Errorf("RPC %s = %s -> %s, want unchanged", name, m.InputType, m.OutputType)
		}
	}
	ab, ok := got["AddBundleToCart"]
	if !ok {
		t.Fatalf("CartService.AddBundleToCart missing")
	}
	if ab.InputType != orderPkg+"AddBundleToCartRequest" || ab.OutputType != orderPkg+"AddBundleToCartResponse" {
		t.Errorf("AddBundleToCart = %s -> %s", ab.InputType, ab.OutputType)
	}
	req, _ := f.message(t, "AddBundleToCartRequest")
	requireFields(t, req, []wf{str("bundle_id", 1), i32("quantity", 2)}, 0)
	resp, _ := f.message(t, "AddBundleToCartResponse")
	requireFields(t, resp, []wf{msg("cart", 1, orderPkg+"Cart")}, 0)
}

// Scenario: Seller centre contract: Bundle fields have fixed numbers
func TestSellerCentreContract_BundleFieldsHaveFixedNumbers(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	ci, _ := f.message(t, "CartItem")
	requireFields(t, ci, []wf{
		str("id", 1), str("listing_id", 2), str("variant_id", 3), i32("quantity", 4),
		i64("unit_price", 5), str("title", 6), str("variant_name", 7), str("image_url", 8),
		str("seller_id", 9), str("bundle_id", 10), i64("bundle_discount", 11),
	}, 11)
	c, _ := f.message(t, "Cart")
	requireFields(t, c, []wf{
		str("user_id", 1), rep("items", 2, orderPkg+"CartItem"), i64("subtotal", 3),
		i64("bundle_discount_total", 4),
	}, 4)
	oi, _ := f.message(t, "OrderItem")
	requireFields(t, oi, []wf{
		str("id", 1), str("listing_id", 2), str("variant_id", 3), str("title", 4),
		str("variant_name", 5), i32("quantity", 6), i64("unit_price", 7), str("image_url", 8),
		str("bundle_id", 9), i64("bundle_discount", 10),
	}, 10)
}

// Scenario: Seller centre contract: AddBundleToCart failures are documented
func TestSellerCentreContract_AddBundleToCartFailuresAreDocumented(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	c, ok := rpcComment(t, f, "CartService", "AddBundleToCart")
	if err := checkNorm(c, ok, "one cart line per bundle listing", "bundle_id", "FAILED_PRECONDITION",
		"unpublished", "out of stock", "not owned by the bundle seller", "variant",
		"NOT_FOUND", "INVALID_ARGUMENT", "quantity", "below 1"); err != nil {
		t.Error(err)
	}
}

// Scenario: Seller centre contract: Bundle fields are documented server-owned
func TestSellerCentreContract_BundleFieldsAreDocumentedServerOwned(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	for _, msgName := range []string{"CartItem", "OrderItem"} {
		c, ok := msgFieldComment(t, f, msgName, "bundle_discount")
		if err := checkNorm(c, ok, "server-owned", "pro-rata"); err != nil {
			t.Errorf("%s.bundle_discount: %v", msgName, err)
		}
	}
}

func scCartItem(discount int) map[string]any {
	return map[string]any{
		"id": "ci-1", "listing_id": "l-1", "variant_id": "", "quantity": 2, "unit_price": 100000,
		"title": "Item", "seller_id": "s-1", "bundle_id": "b1", "bundle_discount": discount,
	}
}

// Scenario: Seller centre contract: Cart with bundle lines round-trips
func TestSellerCentreContract_CartWithBundleLinesRoundTrips(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	ci, _ := f.message(t, "CartItem")
	cart, _ := f.message(t, "Cart")
	oi, _ := f.message(t, "OrderItem")
	scRequireRepeated(t, cart, "items")

	in := map[string]any{"user_id": "u-1", "subtotal": 400000, "bundle_discount_total": 6667}
	raw := s.encode(cart, in)
	for _, d := range []int{3333, 3334} {
		it := scCartItem(d)
		assertEqual(t, roundTrip(t, s, ci, it), it)
		raw = append(raw, lenField(2, s.encode(ci, it))...)
	}
	assertEqual(t, s.decode(t, cart.Field, raw), in)

	plain := map[string]any{"user_id": "u-1", "subtotal": 100}
	gotCart := roundTrip(t, s, cart, plain)
	assertEqual(t, gotCart, map[string]any{"user_id": "u-1", "subtotal": 100, "bundle_discount_total": 0})
	gotItem := roundTrip(t, s, ci, map[string]any{"id": "ci-2", "bundle_id": ""})
	assertEqual(t, gotItem, map[string]any{"bundle_id": "", "bundle_discount": 0})

	order := map[string]any{"id": "oi-1", "bundle_id": "b1", "bundle_discount": math.MaxInt64}
	assertEqual(t, roundTrip(t, s, oi, order), order)
}

// Scenario: Seller centre contract: Old readers skip bundle fields
func TestSellerCentreContract_OldReadersSkipBundleFields(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	ci, _ := f.message(t, "CartItem")
	cart, _ := f.message(t, "Cart")
	oi, _ := f.message(t, "OrderItem")
	for _, c := range []struct {
		m   msgDesc
		in  map[string]any
		max int
		key map[string]any
	}{
		{ci, scCartItem(3333), 9, map[string]any{"id": "ci-1", "quantity": 2, "unit_price": 100000, "seller_id": "s-1"}},
		{cart, map[string]any{"user_id": "u-1", "subtotal": 400000, "bundle_discount_total": 6667}, 3,
			map[string]any{"user_id": "u-1", "subtotal": 400000}},
		{oi, map[string]any{"id": "oi-1", "quantity": 1, "bundle_id": "b1", "bundle_discount": 5}, 8,
			map[string]any{"id": "oi-1", "quantity": 1}},
	} {
		for k := range c.in {
			c.m.field(t, k)
		}
		raw := s.encode(c.m, c.in)
		old := s.decode(t, upTo(c.m.Field, c.max), raw)
		assertEqual(t, old, c.key)
		if _, leaked := old["bundle_id"]; leaked {
			t.Errorf("%s: old schema must not know bundle_id", c.m.Name)
		}
	}
}

// Scenario: Seller centre contract: ListSellerOrders fields have fixed numbers
func TestSellerCentreContract_ListSellerOrdersFieldsHaveFixedNumbers(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	req, _ := f.message(t, "ListSellerOrdersRequest")
	requireFields(t, req, []wf{
		enm("status_filter", 1, scStatusT), msg("page", 2, scPageReqT), str("query", 3),
		enm("statuses", 4, scStatusT),
	}, 4)
	scRequireRepeated(t, req, "statuses")
	resp, _ := f.message(t, "ListSellerOrdersResponse")
	requireFields(t, resp, []wf{
		rep("orders", 1, orderPkg+"Order"), msg("page", 2, scPageRespT), rep("status_counts", 3, scCountT),
	}, 3)
	scRequireRepeated(t, resp, "orders")
	scRequireRepeated(t, resp, "status_counts")
	osc, _ := f.message(t, "OrderStatusCount")
	requireFields(t, osc, []wf{enm("status", 1, scStatusT), i64("count", 2)}, 0)
}

// Scenario: Seller centre contract: ListSellerOrders semantics are documented
func TestSellerCentreContract_ListSellerOrdersSemanticsAreDocumented(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	c, ok := joinComments(
		func() (string, bool) { return msgComment(t, f, "ListSellerOrdersRequest") },
		func() (string, bool) { return msgFieldComment(t, f, "ListSellerOrdersRequest", "page") },
		func() (string, bool) { return msgFieldComment(t, f, "ListSellerOrdersRequest", "query") },
		func() (string, bool) { return msgFieldComment(t, f, "ListSellerOrdersRequest", "statuses") },
		func() (string, bool) { return msgFieldComment(t, f, "ListSellerOrdersResponse", "page") },
		func() (string, bool) { return msgFieldComment(t, f, "ListSellerOrdersResponse", "status_counts") },
	)
	if err := checkNorm(c, ok, "unset", "full list", "100", "wins over", "status_counts",
		"query applied", "INVALID_ARGUMENT", "principal"); err != nil {
		t.Error(err)
	}
}

// Scenario: Seller centre contract: Unpaged request keeps the old wire form
func TestSellerCentreContract_UnpagedRequestKeepsTheOldWireForm(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	req, _ := f.message(t, "ListSellerOrdersRequest")
	resp, _ := f.message(t, "ListSellerOrdersResponse")
	order, _ := f.message(t, "Order")
	requireFields(t, req, []wf{enm("status_filter", 1, scStatusT)}, 1)

	raw := s.encode(req, map[string]any{"status_filter": 3})
	if want := scVarintField(1, 3); !reflect.DeepEqual(raw, want) {
		t.Errorf("request bytes = %x, want %x", raw, want)
	}
	got := s.decode(t, req.Field, raw)
	assertEqual(t, got, map[string]any{"status_filter": 3, "page": nil, "query": ""})

	o := s.encode(order, map[string]any{"id": "O-1", "seller_id": "S-1"})
	body := lenField(1, o)
	gotResp := s.decode(t, resp.Field, body)
	if gotResp["page"] != nil {
		t.Errorf("response page = %v, want unset", gotResp["page"])
	}
	if _, has := gotResp["status_counts"]; has {
		t.Errorf("response status_counts must stay a repeated (skipped) field")
	}
}

// Scenario: Seller centre contract: Paged response round-trips
func TestSellerCentreContract_PagedResponseRoundTrips(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	resp, _ := f.message(t, "ListSellerOrdersResponse")
	osc, _ := f.message(t, "OrderStatusCount")
	page, _ := s.byFullName(scPageRespT)

	pg := map[string]any{"next_cursor": "c2", "total": -1}
	counts := []map[string]any{
		{"status": 1, "count": 12}, {"status": 3, "count": 5000}, {"status": 5, "count": math.MaxInt64},
	}
	raw := lenField(2, s.encode(page, pg))
	for _, c := range counts {
		assertEqual(t, roundTrip(t, s, osc, c), c)
		raw = append(raw, lenField(3, s.encode(osc, c))...)
	}
	scRequireRepeated(t, resp, "status_counts")
	got := s.decode(t, resp.Field, raw)
	if !reflect.DeepEqual(got["page"], pg) {
		t.Errorf("page = %v, want %v", got["page"], pg)
	}
}
