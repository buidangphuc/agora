package promotionv1_test

// Tests for change seller-centre-contract (promotion half). Reuses the
// descriptor loader and helpers of flashstock_compat_test.go.

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
)

func scRPCs(t *testing.T, f fileDesc, svc string) map[string][2]string {
	t.Helper()
	s, _, ok := f.service(svc)
	if !ok {
		t.Fatalf("service %s missing", svc)
	}
	got := map[string][2]string{}
	for _, m := range s.Method {
		got[m.Name] = [2]string{strings.TrimPrefix(m.InputType, pkgPrefix), strings.TrimPrefix(m.OutputType, pkgPrefix)}
	}
	return got
}

func scMsgFields(t *testing.T, f fileDesc, name string) map[string]string {
	t.Helper()
	m, _, ok := f.message(name)
	if !ok {
		t.Errorf("message %s missing", name)
		return nil
	}
	got := map[string]string{}
	for _, fd := range m.Field {
		rep := ""
		if fd.Label == "LABEL_REPEATED" {
			rep = " repeated"
		}
		got[fd.Name] = fieldSig(fd) + rep
	}
	return got
}

func fieldSig(fd fieldDesc) string {
	s := itoa(fd.Number) + " " + fd.Type
	if fd.TypeName != "" {
		s += " " + fd.TypeName
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

const scPageReq = ".platform.common.v1.PageRequest"
const scPageResp = ".platform.common.v1.PageResponse"

// Scenario: Seller centre contract: Seller promotion RPCs exist
func TestSellerCentre_SellerPromotionRPCsExist(t *testing.T) {
	f := loadFile(t)
	fs := scRPCs(t, f, "FlashSaleService")
	vs := scRPCs(t, f, "VoucherService")
	for svc, want := range map[string]map[string][2]string{
		"FlashSaleService": {
			"ListMyCampaigns":  {"ListMyCampaignsRequest", "ListMyCampaignsResponse"},
			"CancelMyCampaign": {"CancelMyCampaignRequest", "CancelMyCampaignResponse"},
			"CreateCampaign":   {"CreateCampaignRequest", "CreateCampaignResponse"},
			"ListCampaigns":    {"ListCampaignsRequest", "ListCampaignsResponse"},
			"CancelCampaign":   {"CancelCampaignRequest", "CancelCampaignResponse"},
		},
		"VoucherService": {
			"ListMyShopVouchers":      {"ListMyShopVouchersRequest", "ListMyShopVouchersResponse"},
			"DeactivateMyShopVoucher": {"DeactivateMyShopVoucherRequest", "DeactivateMyShopVoucherResponse"},
			"DeactivateVoucher":       {"DeactivateVoucherRequest", "DeactivateVoucherResponse"},
			"ListMyVouchers":          {"ListMyVouchersRequest", "ListMyVouchersResponse"},
		},
	} {
		got := fs
		if svc == "VoucherService" {
			got = vs
		}
		for n, w := range want {
			if got[n] != w {
				t.Errorf("%s.%s = %v, want %v", svc, n, got[n], w)
			}
		}
	}
	P := pkgPrefix
	msgs := map[string]map[string]string{
		"ListMyCampaignsRequest":          {"statuses": "1 TYPE_ENUM " + P + "CampaignStatus repeated", "page": "2 TYPE_MESSAGE " + scPageReq},
		"ListMyCampaignsResponse":         {"campaigns": "1 TYPE_MESSAGE " + P + "FlashSaleCampaign repeated", "page": "2 TYPE_MESSAGE " + scPageResp},
		"CancelMyCampaignRequest":         {"campaign_id": "1 TYPE_STRING"},
		"CancelMyCampaignResponse":        {"campaign": "1 TYPE_MESSAGE " + P + "FlashSaleCampaign"},
		"ListMyShopVouchersRequest":       {"page": "1 TYPE_MESSAGE " + scPageReq},
		"ListMyShopVouchersResponse":      {"vouchers": "1 TYPE_MESSAGE " + P + "Voucher repeated", "page": "2 TYPE_MESSAGE " + scPageResp},
		"DeactivateMyShopVoucherRequest":  {"voucher_id": "1 TYPE_STRING"},
		"DeactivateMyShopVoucherResponse": {"voucher": "1 TYPE_MESSAGE " + P + "Voucher"},
	}
	for name, w := range msgs {
		if g := scMsgFields(t, f, name); g != nil && !reflect.DeepEqual(g, w) {
			t.Errorf("%s fields = %v, want exactly %v (no seller id field allowed)", name, g, w)
		}
	}
}

// Scenario: Seller centre contract: FlashSaleCampaign carries seller_id
func TestSellerCentre_FlashSaleCampaignCarriesSellerID(t *testing.T) {
	f := loadFile(t)
	want := map[string]string{
		"id": "1 TYPE_STRING", "listing_id": "2 TYPE_STRING", "variant_id": "3 TYPE_STRING",
		"sale_price": "4 TYPE_INT64", "stock_cap": "5 TYPE_INT64", "stock_sold": "6 TYPE_INT64",
		"starts_at": "7 TYPE_MESSAGE .google.protobuf.Timestamp", "ends_at": "8 TYPE_MESSAGE .google.protobuf.Timestamp",
		"status": "9 TYPE_ENUM " + pkgPrefix + "CampaignStatus", "cancelled_at": "10 TYPE_MESSAGE .google.protobuf.Timestamp",
		"seller_id": "11 TYPE_STRING",
	}
	if g := scMsgFields(t, f, "FlashSaleCampaign"); !reflect.DeepEqual(g, want) {
		t.Errorf("FlashSaleCampaign fields = %v, want exactly %v", g, want)
	}
}

var scCancelKeys = []string{"principal", "listing.write", "PERMISSION_DENIED", "FAILED_PRECONDITION", "SCHEDULED", "not audited"}

func (f fileDesc) svcRPCComment(t *testing.T, svc, rpc string) (string, bool) {
	t.Helper()
	s, si, ok := f.service(svc)
	if !ok {
		t.Fatalf("service %s missing", svc)
	}
	for mi, m := range s.Method {
		if m.Name == rpc {
			return f.comment(6, si, 2, mi)
		}
	}
	t.Errorf("rpc %s.%s missing", svc, rpc)
	return "", false
}

// Scenario: Seller centre contract: Seller promotion authz and preconditions are documented
func TestSellerCentre_SellerPromotionAuthzAndPreconditionsDocumented(t *testing.T) {
	f := loadFile(t)
	cases := []struct {
		svc, rpc string
		keys     []string
	}{
		{"FlashSaleService", "ListMyCampaigns", []string{"principal", "listing.write", "not audited"}},
		{"FlashSaleService", "CancelMyCampaign", scCancelKeys},
		{"VoucherService", "ListMyShopVouchers", []string{"principal", "listing.write"}},
		{"VoucherService", "DeactivateMyShopVoucher", []string{"principal", "listing.write", "PERMISSION_DENIED", "idempotent", "not audited"}},
	}
	for _, c := range cases {
		cm, ok := f.svcRPCComment(t, c.svc, c.rpc)
		if err := checkKeys(cm, ok, c.keys...); err != nil {
			t.Errorf("%s: %v", c.rpc, err)
		}
	}
	cm, ok := f.fieldComment(t, "FlashSaleCampaign", "seller_id")
	if err := checkKeys(cm, ok, "server-owned"); err != nil {
		t.Errorf("FlashSaleCampaign.seller_id: %v", err)
	}
}

// Scenario: Seller centre contract: Missing authz note is caught
func TestSellerCentre_MissingAuthzNoteIsCaught(t *testing.T) {
	fixture := "Cancels a campaign of the principal. Requires listing.write. FAILED_PRECONDITION unless SCHEDULED. Calls are not audited."
	if err := checkKeys(fixture, true, scCancelKeys...); err == nil {
		t.Fatal("helper accepted a CancelMyCampaign comment lacking PERMISSION_DENIED")
	} else if !strings.Contains(err.Error(), "PERMISSION_DENIED") {
		t.Fatalf("helper error does not name PERMISSION_DENIED: %v", err)
	}
	if err := checkKeys(fixture+" PERMISSION_DENIED", true, scCancelKeys...); err != nil {
		t.Fatalf("helper rejected a complete comment: %v", err)
	}
}

// ---- wire codec with nested and repeated fields -----------------------------

var scPageDesc = msgDesc{Name: "PageResponse", Field: []fieldDesc{
	{Name: "next_cursor", Number: 1, Type: "TYPE_STRING"}, {Name: "total", Number: 2, Type: "TYPE_INT64"}}}

func scEnc(m msgDesc, subs map[string]msgDesc, vals map[string]any) []byte {
	var b []byte
	for _, fd := range m.Field {
		v, ok := vals[fd.Name]
		if !ok {
			continue
		}
		var items []any
		if l, isList := v.([]any); isList {
			items = l
		} else {
			items = []any{v}
		}
		for _, it := range items {
			switch {
			case fd.Type == "TYPE_MESSAGE":
				raw := scEnc(subs[fd.Name], subs, it.(map[string]any))
				b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
				b = binary.AppendUvarint(b, uint64(len(raw)))
				b = append(b, raw...)
			case fd.Type == "TYPE_STRING":
				s := it.(string)
				b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
				b = binary.AppendUvarint(b, uint64(len(s)))
				b = append(b, s...)
			default:
				b = binary.AppendUvarint(b, uint64(fd.Number)<<3)
				b = binary.AppendUvarint(b, uint64(it.(int64)))
			}
		}
	}
	return b
}

func scDec(t *testing.T, m msgDesc, subs map[string]msgDesc, b []byte) map[string]any {
	t.Helper()
	by := map[int]fieldDesc{}
	out := map[string]any{}
	for _, fd := range m.Field {
		by[fd.Number] = fd
	}
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			t.Fatal("bad tag")
		}
		b = b[n:]
		fd, known := by[int(tag>>3)]
		switch tag & 7 {
		case 0:
			v, n := binary.Uvarint(b)
			b = b[n:]
			if known {
				out[fd.Name] = int64(v)
			}
		case 2:
			l, n := binary.Uvarint(b)
			raw := b[n : n+int(l)]
			b = b[n+int(l):]
			if !known {
				continue
			}
			var v any
			if fd.Type == "TYPE_MESSAGE" {
				v = scDec(t, subs[fd.Name], subs, raw)
			} else {
				v = string(raw)
			}
			if fd.Label == "LABEL_REPEATED" {
				l, _ := out[fd.Name].([]any)
				out[fd.Name] = append(l, v)
			} else {
				out[fd.Name] = v
			}
		default:
			t.Fatalf("unsupported wire type %d", tag&7)
		}
	}
	return out
}

func scMust(t *testing.T, f fileDesc, name string) msgDesc {
	t.Helper()
	m, _, ok := f.message(name)
	if !ok {
		t.Fatalf("message %s missing", name)
	}
	return m
}

// Scenario: Seller centre contract: Campaign and voucher lists round-trip
func TestSellerCentre_CampaignAndVoucherListsRoundTrip(t *testing.T) {
	f := loadFile(t)
	camp := scMust(t, f, "FlashSaleCampaign")
	voucher := scMust(t, f, "Voucher")
	resp := scMust(t, f, "ListMyCampaignsResponse")
	vresp := scMust(t, f, "ListMyShopVouchersResponse")
	const max = int64(9223372036854775807)
	c1 := map[string]any{"id": "c1", "listing_id": "l1", "seller_id": "s1", "stock_sold": max, "stock_cap": int64(10)}
	c2 := map[string]any{"id": "c2", "listing_id": "l2"}
	in := map[string]any{"campaigns": []any{c1, c2}, "page": map[string]any{"next_cursor": "c1", "total": int64(2)}}
	subs := map[string]msgDesc{"campaigns": camp, "page": scPageDesc}
	got := scDec(t, resp, subs, scEnc(resp, subs, in))
	if !reflect.DeepEqual(got, in) {
		t.Errorf("campaigns round-trip = %v, want %v", got, in)
	}
	if _, has := got["campaigns"].([]any)[1].(map[string]any)["seller_id"]; has {
		t.Errorf("campaign without seller_id parsed with seller_id set")
	}
	vin := map[string]any{"vouchers": []any{map[string]any{"id": "v1", "code": "SAVE10", "seller_id": "s1", "quota": int64(5)}},
		"page": map[string]any{"next_cursor": "c1"}}
	vsubs := map[string]msgDesc{"vouchers": voucher, "page": scPageDesc}
	vgot := scDec(t, vresp, vsubs, scEnc(vresp, vsubs, vin))
	if !reflect.DeepEqual(vgot, vin) {
		t.Errorf("vouchers round-trip = %v, want %v", vgot, vin)
	}
}

// Scenario: Seller centre contract: Old readers skip seller_id
func TestSellerCentre_OldReadersSkipSellerID(t *testing.T) {
	f := loadFile(t)
	camp := scMust(t, f, "FlashSaleCampaign")
	if _, _, ok := camp.fieldIndex("seller_id"); !ok {
		t.Fatal("FlashSaleCampaign.seller_id missing")
	}
	in := map[string]any{"id": "c1", "listing_id": "l1", "variant_id": "v1", "sale_price": int64(900),
		"stock_cap": int64(10), "stock_sold": int64(3), "seller_id": "s1"}
	got := scDec(t, msgDesc{Field: upTo(camp.Field, 10)}, nil, scEnc(camp, nil, in))
	want := map[string]any{"id": "c1", "listing_id": "l1", "variant_id": "v1", "sale_price": int64(900),
		"stock_cap": int64(10), "stock_sold": int64(3)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("old-schema decode = %v, want %v", got, want)
	}
}
