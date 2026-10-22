package orderv1_test

// Tests for change q11-checkout-contract (capability "Checkout contract").
// Reuses the descriptor model, pin helpers and wire codec of
// lifecycle_compat_test.go (same package). Comment text is whitespace-normalised
// so proto line wrapping is irrelevant.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	promoFile = "platform/promotion/v1/promotion.proto"
	identFile = "platform/identity/v1/identity.proto"
	promoPkg  = ".platform.promotion.v1."
	addressT  = ".platform.identity.v1.Address"
)

func loadCheckoutSet(t *testing.T) descSet {
	t.Helper()
	cmd := exec.Command("buf", "build", "--path", orderFile, "--path", paymentFile,
		"--path", promoFile, "--path", identFile, "-o", "-#format=json")
	cmd.Dir = "../../.."
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed (harness): %v\n%s", err, errb.String())
	}
	var s struct {
		File []fileDesc `json:"file"`
	}
	if err := json.Unmarshal(out.Bytes(), &s); err != nil {
		t.Fatalf("decode descriptor json: %v", err)
	}
	return descSet{files: s.File}
}

func normText(s string) string { return strings.Join(strings.Fields(s), " ") }

// checkNorm returns an error naming every substring missing from the comment.
func checkNorm(comment string, ok bool, subs ...string) error {
	if !ok {
		return fmt.Errorf("no comment found on element")
	}
	c := normText(comment)
	var missing []string
	for _, s := range subs {
		if !strings.Contains(c, normText(s)) {
			missing = append(missing, fmt.Sprintf("%q", s))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("comment missing %s; got:\n%s", strings.Join(missing, ", "), c)
	}
	return nil
}

func joinComments(parts ...func() (string, bool)) (string, bool) {
	var b strings.Builder
	all := true
	for _, p := range parts {
		c, ok := p()
		all = all && ok
		b.WriteString(c)
		b.WriteString(" ")
	}
	return b.String(), all
}

func repeated(m msgDesc, name string) bool {
	for _, fd := range m.Field {
		if fd.Name == name {
			return fd.Label == "LABEL_REPEATED"
		}
	}
	return false
}

func rep(name string, n int, typeName string) wf {
	return wf{name, n, "TYPE_MESSAGE", typeName}
}

var (
	shopVoucherT = orderPkg + "ShopVoucher"
	groupQuoteT  = orderPkg + "CheckoutGroupQuote"
	voucherQuote = orderPkg + "VoucherQuote"
	scopeT       = orderPkg + "CheckoutVoucherScope"
)

var shopVoucherF = []wf{str("seller_id", 1), str("code", 2)}

var previewReqF = []wf{
	str("address_id", 1), str("item_ids", 2), rep("shop_vouchers", 3, shopVoucherT), str("platform_voucher_code", 4),
}

var groupQuoteF = []wf{
	str("seller_id", 1), str("item_ids", 2), i64("items_subtotal", 3), i64("shipping_fee", 4),
	boolean("is_free_shipping", 5), str("shop_voucher_code", 6), i64("shop_discount_amount", 7),
	i64("platform_discount_amount", 8), i64("total_amount", 9), ts("estimated_delivery_at", 10),
}

var voucherQuoteF = []wf{
	str("code", 1), enm("scope", 2, scopeT), str("seller_id", 3), boolean("valid", 4),
	str("reason", 5), i64("discount_amount", 6),
}

var previewRespF = []wf{
	rep("groups", 1, groupQuoteT), rep("vouchers", 2, voucherQuote), i64("items_subtotal", 3),
	i64("shipping_fee", 4), i64("discount_amount", 5), i64("total_amount", 6),
	boolean("shipping_quoted", 7), msg("shipping_address", 8, addressT),
}

var createOrderF = []wf{
	str("address_id", 1), str("item_ids", 2), enm("payment_method", 3, payMethodT), str("voucher_code", 4),
	rep("shop_vouchers", 5, shopVoucherT), str("platform_voucher_code", 6), msg("confirmed_address", 7, addressT),
}

var shippingRespF = []wf{
	i64("shipping_fee", 1), boolean("is_free_shipping", 2), str("message", 3),
	i32("estimated_delivery_days", 4), ts("estimated_delivery_at", 5),
}

var orderQ11F = []wf{str("checkout_id", 25), str("platform_voucher_code", 26), i64("platform_discount_amount", 27)}

// Scenario: Checkout contract: PreviewCheckout exists on OrderService
func TestCheckoutContract_PreviewCheckoutExistsOnOrderService(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	svc, _ := f.service(t, "OrderService")
	got := map[string]methodDesc{}
	for _, m := range svc.Method {
		got[m.Name] = m
	}
	names := append(append([]string{}, existingOrderRPCs...), "ConfirmReceipt")
	for _, n := range names {
		m, ok := got[n]
		if !ok {
			t.Errorf("existing RPC %s missing", n)
			continue
		}
		if m.InputType != orderPkg+n+"Request" || m.OutputType != orderPkg+n+"Response" {
			t.Errorf("RPC %s = %s -> %s, want unchanged", n, m.InputType, m.OutputType)
		}
	}
	p, ok := got["PreviewCheckout"]
	if !ok {
		t.Fatal("OrderService.PreviewCheckout missing")
	}
	if p.InputType != orderPkg+"PreviewCheckoutRequest" || p.OutputType != orderPkg+"PreviewCheckoutResponse" {
		t.Errorf("PreviewCheckout = %s -> %s", p.InputType, p.OutputType)
	}
	if len(svc.Method) != len(names)+1 {
		t.Errorf("OrderService has %d RPCs, want exactly %d", len(svc.Method), len(names)+1)
	}
}

// Scenario: Checkout contract: Quote messages have fixed names, numbers and types
func TestCheckoutContract_QuoteMessagesHaveFixedNamesNumbersAndTypes(t *testing.T) {
	s := loadCheckoutSet(t)
	f := s.file(t, orderFile)
	cases := []struct {
		name string
		want []wf
		rep  []string
	}{
		{"ShopVoucher", shopVoucherF, nil},
		{"PreviewCheckoutRequest", previewReqF, []string{"item_ids", "shop_vouchers"}},
		{"CheckoutGroupQuote", groupQuoteF, []string{"item_ids"}},
		{"VoucherQuote", voucherQuoteF, nil},
		{"PreviewCheckoutResponse", previewRespF, []string{"groups", "vouchers"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := f.message(t, c.name)
			requireFields(t, m, c.want, 0)
			for _, fd := range m.Field {
				want := false
				for _, r := range c.rep {
					want = want || r == fd.Name
				}
				if got := fd.Label == "LABEL_REPEATED"; got != want {
					t.Errorf("%s.%s repeated=%v, want %v", c.name, fd.Name, got, want)
				}
			}
		})
	}
	e, _ := f.enum(t, "CheckoutVoucherScope")
	want := []string{
		"CHECKOUT_VOUCHER_SCOPE_PLATFORM=2", "CHECKOUT_VOUCHER_SCOPE_SHOP=1", "CHECKOUT_VOUCHER_SCOPE_UNSPECIFIED=0",
	}
	if got := enumPairs(e); !reflect.DeepEqual(got, want) {
		t.Errorf("CheckoutVoucherScope = %v, want %v", got, want)
	}
	raw, err := os.ReadFile("order.proto")
	if err != nil {
		t.Fatalf("read order.proto: %v", err)
	}
	if strings.Contains(string(raw), "promotion.proto") {
		t.Error("order.proto must not import promotion.proto")
	}
}

// Scenario: Checkout contract: Voucher response reports scope and shop (enum parity half)
func TestCheckoutContract_ScopeEnumMatchesPromotionVoucherScope(t *testing.T) {
	s := loadCheckoutSet(t)
	co, _ := s.file(t, orderFile).enum(t, "CheckoutVoucherScope")
	pe, _ := s.file(t, promoFile).enum(t, "VoucherScope")
	trim := func(e enumDesc, prefix string) []string {
		var o []string
		for _, v := range e.Value {
			o = append(o, fmt.Sprintf("%s=%d", strings.TrimPrefix(v.Name, prefix), v.Number))
		}
		return o
	}
	a, b := trim(co, "CHECKOUT_VOUCHER_SCOPE_"), trim(pe, "VOUCHER_SCOPE_")
	if !reflect.DeepEqual(a, b) {
		t.Errorf("CheckoutVoucherScope %v differs from VoucherScope %v", a, b)
	}
}

// Scenario: Checkout contract: Quote semantics are documented
func TestCheckoutContract_QuoteSemanticsAreDocumented(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	c, ok := rpcComment(t, f, "OrderService", "PreviewCheckout")
	if err := checkNorm(c, ok, "reserves nothing", "vouchers[]", "FAILED_PRECONDITION", "cart is empty",
		"CreateOrder places", "never logged"); err != nil {
		t.Errorf("rpc: %v", err)
	}
	c, ok = msgFieldComment(t, f, "PreviewCheckoutResponse", "groups")
	if err := checkNorm(c, ok, "sorted by seller_id", "one per order"); err != nil {
		t.Errorf("groups: %v", err)
	}
	c, ok = msgFieldComment(t, f, "CheckoutGroupQuote", "total_amount")
	if err := checkNorm(c, ok,
		"total_amount = items_subtotal + shipping_fee - shop_discount_amount - platform_discount_amount",
		"clamped at 0"); err != nil {
		t.Errorf("total_amount: %v", err)
	}
	c, ok = msgFieldComment(t, f, "CheckoutGroupQuote", "is_free_shipping")
	if err := checkNorm(c, ok, "per group"); err != nil {
		t.Errorf("is_free_shipping: %v", err)
	}
	c, ok = msgFieldComment(t, f, "CheckoutGroupQuote", "estimated_delivery_at")
	if err := checkNorm(c, ok, "unset when shipping_quoted is false"); err != nil {
		t.Errorf("estimated_delivery_at: %v", err)
	}
	c, ok = msgFieldComment(t, f, "PreviewCheckoutResponse", "shipping_address")
	if err := checkNorm(c, ok, "never logged"); err != nil {
		t.Errorf("shipping_address: %v", err)
	}
}

// Scenario: Checkout contract: Missing quote semantics are caught
func TestCheckoutContract_MissingQuoteSemanticsAreCaught(t *testing.T) {
	fixture := "Quotes a checkout. A rejected voucher is reported in vouchers[]."
	if err := checkNorm(fixture, true, "reserves nothing"); err == nil {
		t.Fatal("helper accepted a comment lacking 'reserves nothing'")
	}
	if err := checkNorm("", false, "reserves nothing"); err == nil {
		t.Fatal("helper accepted a missing comment")
	}
	if err := checkNorm("Quote\n  reserves   nothing.", true, "reserves nothing"); err != nil {
		t.Fatalf("helper rejected a wrapped comment containing the phrase: %v", err)
	}
}

// Scenario: Checkout contract: Quote of an empty selection is documented as an error
func TestCheckoutContract_QuoteOfEmptySelectionIsDocumentedAsError(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	c, ok := rpcComment(t, f, "OrderService", "PreviewCheckout")
	if err := checkNorm(c, ok, "item_ids", "FAILED_PRECONDITION", `"cart is empty"`, "not an empty groups list"); err != nil {
		t.Error(err)
	}
}

// Scenario: Checkout contract: Quote is documented as buyer-only
func TestCheckoutContract_QuoteIsDocumentedAsBuyerOnly(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	c, ok := rpcComment(t, f, "OrderService", "PreviewCheckout")
	if err := checkNorm(c, ok, "authenticated buyer", "own cart and addresses", "no buyer id",
		"seller", "service principal", "denied"); err != nil {
		t.Error(err)
	}
}

// ---- wire behaviour --------------------------------------------------------

func addressVals() map[string]any {
	return map[string]any{
		"id": "a-1", "user_id": "u-1", "recipient_name": "Nguyen Van A", "phone": "0900000000",
		"street": "1 Le Loi", "ward": "Ben Nghe", "district": "1", "city": "HCM", "is_default": true,
	}
}

func lenField(num int, raw []byte) []byte {
	b := binary.AppendUvarint(nil, uint64(num)<<3|2)
	b = binary.AppendUvarint(b, uint64(len(raw)))
	return append(b, raw...)
}

// Scenario: Checkout contract: Quote messages round-trip
func TestCheckoutContract_QuoteMessagesRoundTrip(t *testing.T) {
	s := loadCheckoutSet(t)
	f := s.file(t, orderFile)
	grp, _ := f.message(t, "CheckoutGroupQuote")
	vq, _ := f.message(t, "VoucherQuote")
	resp, _ := f.message(t, "PreviewCheckoutResponse")

	g1 := map[string]any{"seller_id": "s-a", "items_subtotal": 300000, "shipping_fee": 35000, "platform_discount_amount": 10000}
	g2 := map[string]any{"seller_id": "s-b", "items_subtotal": 300000, "shipping_fee": 35000, "platform_discount_amount": 20000}
	for _, g := range []map[string]any{g1, g2} {
		assertEqual(t, roundTrip(t, s, grp, g), g)
	}
	v := map[string]any{"scope": 2, "valid": false, "reason": "min spend not met"}
	got := roundTrip(t, s, vq, v)
	assertEqual(t, got, v)
	if got["seller_id"] != "" {
		t.Errorf("VoucherQuote.seller_id = %v, want empty", got["seller_id"])
	}

	top := map[string]any{"shipping_quoted": true, "shipping_address": addressVals()}
	raw := s.encode(resp, top)
	raw = append(raw, lenField(1, s.encode(grp, g1))...)
	raw = append(raw, lenField(1, s.encode(grp, g2))...)
	raw = append(raw, lenField(2, s.encode(vq, v))...)
	gotResp := s.decode(t, resp.Field, raw)
	if gotResp["shipping_quoted"] != true {
		t.Errorf("shipping_quoted = %v", gotResp["shipping_quoted"])
	}
	if !reflect.DeepEqual(gotResp["shipping_address"], addressVals()) {
		t.Errorf("shipping_address = %v, want %v", gotResp["shipping_address"], addressVals())
	}
}

// Scenario: Checkout contract: Maximum amounts round-trip
func TestCheckoutContract_MaximumAmountsRoundTrip(t *testing.T) {
	s := loadCheckoutSet(t)
	grp, _ := s.file(t, orderFile).message(t, "CheckoutGroupQuote")
	in := map[string]any{"items_subtotal": math.MaxInt64, "shipping_fee": math.MaxInt64, "total_amount": math.MaxInt64}
	assertEqual(t, roundTrip(t, s, grp, in), in)
}

// Scenario: Checkout contract: CreateOrderRequest fields have fixed numbers
func TestCheckoutContract_CreateOrderRequestFieldsHaveFixedNumbers(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	m, _ := f.message(t, "CreateOrderRequest")
	requireFields(t, m, createOrderF, 0)
	for _, fd := range m.Field {
		want := fd.Name == "item_ids" || fd.Name == "shop_vouchers"
		if got := fd.Label == "LABEL_REPEATED"; got != want {
			t.Errorf("CreateOrderRequest.%s repeated=%v, want %v", fd.Name, got, want)
		}
	}
}

// Scenario: Checkout contract: Old readers skip the new request fields
func TestCheckoutContract_OldReadersSkipTheNewRequestFields(t *testing.T) {
	s := loadCheckoutSet(t)
	f := s.file(t, orderFile)
	req, _ := f.message(t, "CreateOrderRequest")
	sv, _ := f.message(t, "ShopVoucher")
	in := map[string]any{
		"address_id": "a-1", "payment_method": 2, "voucher_code": "PLAT",
		"platform_voucher_code": "PLAT2", "confirmed_address": addressVals(),
	}
	raw := s.encode(req, in)
	raw = append(raw, lenField(5, s.encode(sv, map[string]any{"seller_id": "s-a", "code": "SHOP1"}))...)
	for _, n := range []string{"shop_vouchers", "platform_voucher_code", "confirmed_address"} {
		req.field(t, n)
	}
	got := s.decode(t, upTo(req.Field, 4), raw)
	want := map[string]any{"address_id": "a-1", "payment_method": 2, "voucher_code": "PLAT"}
	assertEqual(t, got, want)
	if _, leaked := got["platform_voucher_code"]; leaked {
		t.Error("old reader decoded field 6")
	}
}

// Scenario: Checkout contract: New CreateOrder fields are documented as buyer-only
func TestCheckoutContract_NewCreateOrderFieldsAreDocumentedAsBuyerOnly(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	for _, n := range []string{"shop_vouchers", "platform_voucher_code", "confirmed_address"} {
		c, ok := msgFieldComment(t, f, "CreateOrderRequest", n)
		if err := checkNorm(c, ok, "authenticated buyer", "own cart and own addresses", "no buyer id",
			"seller", "service principal", "PERMISSION_DENIED"); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}

// Scenario: Checkout contract: Voucher routing is documented
func TestCheckoutContract_VoucherRoutingIsDocumented(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	vc, ok := msgFieldComment(t, f, "CreateOrderRequest", "voucher_code")
	if err := checkNorm(vc, ok, "platform-voucher slot only", "INVALID_ARGUMENT",
		"platform_voucher_code", "shop_vouchers"); err != nil {
		t.Errorf("voucher_code: %v", err)
	}
	all, ok := joinComments(
		func() (string, bool) { return msgFieldComment(t, f, "CreateOrderRequest", "voucher_code") },
		func() (string, bool) { return msgFieldComment(t, f, "CreateOrderRequest", "shop_vouchers") },
		func() (string, bool) { return msgFieldComment(t, f, "CreateOrderRequest", "platform_voucher_code") },
	)
	if err := checkNorm(all, ok,
		"at most one shop voucher per seller_id", "seller's group", "min_spend", "whole selection",
		"integer division", "proportionally to items_subtotal", "largest group", "lowest seller_id",
		"stack", "clamped at 0", "platform:<checkout_id>",
		"FAILED_PRECONDITION", "promotion reason", "wrong slot", "INVALID_ARGUMENT", "nothing stays held"); err != nil {
		t.Error(err)
	}
}

// Scenario: Checkout contract: Address failures are documented as fail closed
func TestCheckoutContract_AddressFailuresAreDocumentedAsFailClosed(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	all, ok := joinComments(
		func() (string, bool) { return msgFieldComment(t, f, "CreateOrderRequest", "address_id") },
		func() (string, bool) { return msgFieldComment(t, f, "CreateOrderRequest", "confirmed_address") },
	)
	if err := checkNorm(all, ok, "empty address_id means the default address", "FAILED_PRECONDITION",
		`"address not found"`, "no first-address fallback", `"address changed"`,
		"recipient_name", "phone", "street", "ward", "district", "city",
		"UNAVAILABLE", "nothing is reserved"); err != nil {
		t.Error(err)
	}
	c, ok := msgFieldComment(t, f, "CreateOrderRequest", "confirmed_address")
	if err := checkNorm(c, ok, `"address changed"`); err != nil {
		t.Errorf("confirmed_address: %v", err)
	}
}

// Scenario: Checkout contract: Missing address-failure note is caught
func TestCheckoutContract_MissingAddressFailureNoteIsCaught(t *testing.T) {
	fixture := "Optional address the buyer confirmed; compared with identity's copy."
	if err := checkNorm(fixture, true, `"address changed"`); err == nil {
		t.Fatal("helper accepted a comment lacking \"address changed\"")
	}
	if err := checkNorm(`fails "address changed"`, true, `"address changed"`); err != nil {
		t.Fatalf("helper rejected a comment containing the phrase: %v", err)
	}
}

// Scenario: Checkout contract: Shipping response fields are fixed
func TestCheckoutContract_ShippingResponseFieldsAreFixed(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	m, _ := f.message(t, "CalculateShippingFeeResponse")
	requireFields(t, m, shippingRespF, 0)
	c, ok := msgFieldComment(t, f, "CalculateShippingFeeResponse", "estimated_delivery_at")
	if err := checkNorm(c, ok, "now + estimated_delivery_days"); err != nil {
		t.Error(err)
	}
}

// Scenario: Checkout contract: Order fields are fixed and 18-24 stay free
func TestCheckoutContract_OrderFieldsAreFixedAnd18To24StayFree(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	m, _ := f.message(t, "Order")
	want := append(append(append([]wf{}, orderExisting...), orderLifecycle...), orderQ11F...)
	requireFields(t, m, want, 0)
	var nums []int
	for _, fd := range m.Field {
		if fd.Number >= 18 && fd.Number <= 24 {
			nums = append(nums, fd.Number)
		}
	}
	sort.Ints(nums)
	if !reflect.DeepEqual(nums, []int{18, 19, 20, 21, 22, 23, 24}) {
		t.Errorf("Order fields 18-24 = %v, must stay exactly q10's", nums)
	}
	c, ok := msgFieldComment(t, f, "Order", "checkout_id")
	if err := checkNorm(c, ok, "every order of one checkout", "saga id"); err != nil {
		t.Errorf("checkout_id: %v", err)
	}
	c, ok = msgFieldComment(t, f, "Order", "platform_discount_amount")
	if err := checkNorm(c, ok, "part of discount_amount"); err != nil {
		t.Errorf("platform_discount_amount: %v", err)
	}
	c, ok = msgFieldComment(t, f, "Order", "voucher_code")
	if err := checkNorm(c, ok, "shop voucher"); err != nil {
		t.Errorf("voucher_code: %v", err)
	}
}

// Scenario: Checkout contract: Order round-trips and old readers skip the new fields
func TestCheckoutContract_OrderRoundTripsAndOldReadersSkipNewFields(t *testing.T) {
	s := loadCheckoutSet(t)
	m, _ := s.file(t, orderFile).message(t, "Order")
	in := orderBase()
	in["discount_amount"] = 30000
	in["platform_discount_amount"] = 10000
	in["checkout_id"] = "chk-1"
	in["platform_voucher_code"] = "PLAT10"
	assertEqual(t, roundTrip(t, s, m, in), in)
	old := s.decode(t, upTo(m.Field, 17), s.encode(m, in))
	assertEqual(t, old, orderBase2(in))
	for _, k := range []string{"checkout_id", "platform_voucher_code", "platform_discount_amount"} {
		if _, leaked := old[k]; leaked {
			t.Errorf("old reader decoded %s", k)
		}
	}
}

func orderBase2(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		switch k {
		case "checkout_id", "platform_voucher_code", "platform_discount_amount":
		default:
			out[k] = v
		}
	}
	return out
}

// Scenario: Checkout contract: Voucher response reports scope and shop
func TestCheckoutContract_VoucherResponseReportsScopeAndShop(t *testing.T) {
	s := loadCheckoutSet(t)
	pf := s.file(t, promoFile)
	m, _ := pf.message(t, "ValidateAndReserveResponse")
	requireFields(t, m, []wf{
		boolean("valid", 1), str("reason", 2), i64("discount_amount", 3), str("voucher_id", 4),
		enm("scope", 5, promoPkg+"VoucherScope"), str("seller_id", 6),
	}, 0)
	in := map[string]any{"valid": true, "discount_amount": 10000, "voucher_id": "v-1", "scope": 1, "seller_id": "s-a"}
	assertEqual(t, roundTrip(t, s, m, in), in)
	c, ok := msgFieldComment(t, pf, "ValidateAndReserveResponse", "seller_id")
	if err := checkNorm(c, ok, "empty for PLATFORM"); err != nil {
		t.Errorf("seller_id: %v", err)
	}
}

// Scenario: Checkout contract: Existing order messages are untouched
func TestCheckoutContract_ExistingOrderMessagesAreUntouched(t *testing.T) {
	f := loadCheckoutSet(t).file(t, orderFile)
	cases := []struct {
		name string
		want []wf
	}{
		{"OrderItem", []wf{
			str("id", 1), str("listing_id", 2), str("variant_id", 3), str("title", 4), str("variant_name", 5),
			i32("quantity", 6), i64("unit_price", 7), str("image_url", 8),
		}},
		{"GetOrderRequest", []wf{str("id", 1)}},
		{"ListBuyerOrdersRequest", []wf{enm("status_filter", 1, orderPkg+"OrderStatus")}},
		{"CancelOrderRequest", []wf{str("id", 1), str("reason", 2), enm("reason_code", 3, cancelReasonT)}},
		{"CalculateShippingFeeRequest", []wf{str("city", 1), i64("items_subtotal", 2)}},
		{"OrderPaidEvent", []wf{
			str("order_id", 1), str("buyer_id", 2), {"items", 3, "TYPE_MESSAGE", orderPkg + "OrderLineItemFact"},
			i64("total_amount", 4), str("currency", 5), ts("paid_at", 6), str("seller_id", 7), ts("ship_by", 8),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := f.message(t, c.name)
			requireFields(t, m, c.want, 0)
		})
	}
}
