package orderv1_test

// Tests for change monetization-contract, capability "Order paid event carries
// its price breakdown". Baseline for fields 1 to 8 is pinned from origin/main.

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

var monOrderPaidBaseline = map[string]string{
	"order_id": "1 string", "buyer_id": "2 string", "items": "3 repeated message:platform.order.v1.OrderLineItemFact",
	"total_amount": "4 int64", "currency": "5 string", "paid_at": "6 message:google.protobuf.Timestamp",
	"seller_id": "7 string", "ship_by": "8 message:google.protobuf.Timestamp",
}

var monBreakdownFields = []string{"items_subtotal", "shipping_fee", "shop_discount_amount", "platform_discount_amount"}

func monHas(t *testing.T, what, comment string, keys ...string) {
	t.Helper()
	c := strings.ToLower(dsNorm(comment))
	if c == "" {
		t.Errorf("%s: no comment found", what)
		return
	}
	for _, k := range keys {
		if !strings.Contains(c, strings.ToLower(dsNorm(k))) {
			t.Errorf("%s: comment missing %q; got: %s", what, k, c)
		}
	}
}

func TestMonetization_OrderPaidBreakdownShape(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	want := map[string]string{
		"items_subtotal": "9 optional int64", "shipping_fee": "10 optional int64",
		"shop_discount_amount": "11 optional int64", "platform_discount_amount": "12 optional int64",
	}
	for k, v := range monOrderPaidBaseline {
		want[k] = v
	}
	dsShape(t, fd, "OrderPaidEvent", want)
}

func TestMonetization_BreakdownPresenceRoundTrip(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	md := dsMsg(t, fd, "OrderPaidEvent")
	f := dsFld(t, md, "shop_discount_amount")
	zero := dynamicpb.NewMessage(md)
	zero.Set(f, protoreflect.ValueOfInt64(0))
	unset := dynamicpb.NewMessage(md)
	zb, err := proto.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	ub, err := proto.Marshal(unset)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(zb, ub) {
		t.Errorf("explicit zero and unset serialise to the same bytes: %x", zb)
	}
	gz := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(zb, gz); err != nil {
		t.Fatal(err)
	}
	if !gz.Has(f) || gz.Get(f).Int() != 0 {
		t.Errorf("explicit zero: presence=%v value=%d, want true/0", gz.Has(f), gz.Get(f).Int())
	}
	gu := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(ub, gu); err != nil {
		t.Fatal(err)
	}
	if gu.Has(f) {
		t.Errorf("unset field reads as present")
	}
}

func TestMonetization_OldOrderPaidParses(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	md := dsMsg(t, fd, "OrderPaidEvent")
	var old []byte
	old = protowire.AppendTag(old, 1, protowire.BytesType)
	old = protowire.AppendString(old, "o-1")
	old = protowire.AppendTag(old, 4, protowire.VarintType)
	old = protowire.AppendVarint(old, 125000)
	old = protowire.AppendTag(old, 5, protowire.BytesType)
	old = protowire.AppendString(old, "VND")
	old = protowire.AppendTag(old, 7, protowire.BytesType)
	old = protowire.AppendString(old, "s-1")
	got := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(old, got); err != nil {
		t.Fatalf("old bytes rejected: %v", err)
	}
	r := got.ProtoReflect()
	if dsStr(r, "order_id") != "o-1" || dsInt(r, "total_amount") != 125000 || dsStr(r, "currency") != "VND" || dsStr(r, "seller_id") != "s-1" {
		t.Errorf("fields 1 to 8 changed: %v", got)
	}
	for _, n := range monBreakdownFields {
		f := md.Fields().ByName(protoreflect.Name(n))
		if f == nil {
			t.Errorf("%s missing from schema", n)
			continue
		}
		if got.Has(f) {
			t.Errorf("%s reads as present for an old event", n)
		}
	}
}

func TestMonetization_BreakdownDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	var all []string
	for _, n := range monBreakdownFields {
		c := dsFieldComment(t, fd, "OrderPaidEvent", n)
		monHas(t, n, c, "integer VND")
		all = append(all, c)
	}
	monHas(t, "shop_discount_amount", dsFieldComment(t, fd, "OrderPaidEvent", "shop_discount_amount"), "seller-funded voucher share")
	monHas(t, "platform_discount_amount", dsFieldComment(t, fd, "OrderPaidEvent", "platform_discount_amount"), "platform voucher share")
	monHas(t, "absent breakdown", strings.Join(all, " "), "absent", "published without")
}
