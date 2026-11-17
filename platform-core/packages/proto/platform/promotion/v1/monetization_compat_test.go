package promotionv1_test

// Tests for change monetization-contract, capabilities "Plan terms and
// subscription reads", "Ad campaign counters and budget status", "Sponsored slot
// serving contract" and "Ad click and campaign management RPCs". Baselines are
// pinned from origin/main.

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	monPromoProto = "platform/promotion/v1/promotion.proto"
	monPromoPkg   = "platform.promotion.v1."
	monPageReq    = "message:platform.common.v1.PageRequest"
	monPageResp   = "message:platform.common.v1.PageResponse"
)

var monPromoBaselineMessages = map[string]map[string]string{
	"Plan":                          {"id": "1 string", "name": "2 enum:platform.promotion.v1.PlanTier", "price": "3 int64", "features": "4 repeated string"},
	"SellerSubscription":            {"id": "1 string", "seller_id": "2 string", "plan_id": "3 string", "tier": "4 enum:platform.promotion.v1.PlanTier", "status": "5 enum:platform.promotion.v1.SubscriptionStatus", "starts_at": "6 message:google.protobuf.Timestamp", "ends_at": "7 message:google.protobuf.Timestamp"},
	"ListPlansRequest":              {},
	"ListPlansResponse":             {"plans": "1 repeated message:platform.promotion.v1.Plan"},
	"SubscribeRequest":              {"plan_id": "1 string"},
	"SubscribeResponse":             {"subscription": "1 message:platform.promotion.v1.SellerSubscription"},
	"GetEntitlementsRequest":        {"seller_id": "1 string"},
	"GetEntitlementsResponse":       {"plan": "1 enum:platform.promotion.v1.PlanTier", "limits": "2 repeated message:platform.promotion.v1.GetEntitlementsResponse.LimitsEntry"},
	"ListMyVouchersRequest":         {},
	"ListMyVouchersResponse":        {"vouchers": "1 repeated message:platform.promotion.v1.Voucher"},
	"Voucher":                       {"id": "1 string", "code": "2 string", "scope": "3 enum:platform.promotion.v1.VoucherScope", "seller_id": "4 string", "discount_type": "5 enum:platform.promotion.v1.DiscountType", "discount_value": "6 int64", "min_spend": "7 int64", "max_discount": "8 int64", "quota": "9 int64", "used": "10 int64", "starts_at": "11 message:google.protobuf.Timestamp", "ends_at": "12 message:google.protobuf.Timestamp", "active": "13 bool", "deactivated_at": "14 message:google.protobuf.Timestamp", "owner_user_id": "15 string", "kind": "16 enum:platform.promotion.v1.VoucherKind", "per_user_limit": "17 int32", "new_user_only": "18 bool"},
	"DeactivateVoucherRequest":      {"voucher_id": "1 string"},
	"DeactivateVoucherResponse":     {"voucher": "1 message:platform.promotion.v1.Voucher"},
	"UpdateVoucherRequest":          {"voucher_id": "1 string", "ends_at": "2 optional message:google.protobuf.Timestamp", "quota": "3 optional int64", "per_user_limit": "4 optional int32", "new_user_only": "5 optional bool"},
	"UpdateVoucherResponse":         {"voucher": "1 message:platform.promotion.v1.Voucher"},
	"CreateVoucherRequest":          {"code": "1 string", "scope": "2 enum:platform.promotion.v1.VoucherScope", "discount_type": "3 enum:platform.promotion.v1.DiscountType", "discount_value": "4 int64", "min_spend": "5 int64", "max_discount": "6 int64", "quota": "7 int64", "starts_at": "8 message:google.protobuf.Timestamp", "ends_at": "9 message:google.protobuf.Timestamp", "per_user_limit": "10 int32", "new_user_only": "11 bool"},
	"CreateVoucherResponse":         {"voucher": "1 message:platform.promotion.v1.Voucher"},
	"GetVoucherRequest":             {"code": "1 string"},
	"GetVoucherResponse":            {"voucher": "1 message:platform.promotion.v1.Voucher"},
	"ListVouchersRequest":           {"seller_id": "1 string", "page": "2 message:platform.common.v1.PageRequest"},
	"ListVouchersResponse":          {"vouchers": "1 repeated message:platform.promotion.v1.Voucher", "page": "2 message:platform.common.v1.PageResponse"},
	"ValidateAndReserveRequest":     {"reservation_id": "1 string", "code": "2 string", "buyer_id": "3 string", "cart_subtotal": "4 int64", "seller_id": "5 string"},
	"ValidateAndReserveResponse":    {"valid": "1 bool", "reason": "2 string", "discount_amount": "3 int64", "voucher_id": "4 string", "scope": "5 enum:platform.promotion.v1.VoucherScope", "seller_id": "6 string"},
	"CommitReservationRequest":      {"reservation_id": "1 string"},
	"CommitReservationResponse":     {"committed": "1 bool"},
	"ReleaseReservationRequest":     {"reservation_id": "1 string"},
	"ReleaseReservationResponse":    {"released": "1 bool"},
	"FlashSaleCampaign":             {"id": "1 string", "listing_id": "2 string", "variant_id": "3 string", "sale_price": "4 int64", "stock_cap": "5 int64", "stock_sold": "6 int64", "starts_at": "7 message:google.protobuf.Timestamp", "ends_at": "8 message:google.protobuf.Timestamp", "status": "9 enum:platform.promotion.v1.CampaignStatus", "cancelled_at": "10 message:google.protobuf.Timestamp"},
	"CreateCampaignRequest":         {"listing_id": "1 string", "variant_id": "2 string", "sale_price": "3 int64", "stock_cap": "4 int64", "starts_at": "5 message:google.protobuf.Timestamp", "ends_at": "6 message:google.protobuf.Timestamp", "idempotency_key": "7 string"},
	"CreateCampaignResponse":        {"campaign": "1 message:platform.promotion.v1.FlashSaleCampaign"},
	"GetActiveFlashSaleRequest":     {"listing_id": "1 string"},
	"GetActiveFlashSaleResponse":    {"active": "1 bool", "campaign": "2 message:platform.promotion.v1.FlashSaleCampaign"},
	"ListActiveCampaignsRequest":    {"page": "1 message:platform.common.v1.PageRequest"},
	"ListActiveCampaignsResponse":   {"campaigns": "1 repeated message:platform.promotion.v1.FlashSaleCampaign", "page": "2 message:platform.common.v1.PageResponse", "flash_sale_disabled": "3 bool"},
	"ListCampaignsRequest":          {"statuses": "1 repeated enum:platform.promotion.v1.CampaignStatus", "page": "2 message:platform.common.v1.PageRequest"},
	"ListCampaignsResponse":         {"campaigns": "1 repeated message:platform.promotion.v1.FlashSaleCampaign", "page": "2 message:platform.common.v1.PageResponse", "flash_sale_disabled": "3 bool"},
	"CancelCampaignRequest":         {"campaign_id": "1 string"},
	"CancelCampaignResponse":        {"campaign": "1 message:platform.promotion.v1.FlashSaleCampaign"},
	"UpdateCampaignRequest":         {"campaign_id": "1 string", "sale_price": "2 int64", "stock_cap": "3 int64", "starts_at": "4 message:google.protobuf.Timestamp", "ends_at": "5 message:google.protobuf.Timestamp"},
	"UpdateCampaignResponse":        {"campaign": "1 message:platform.promotion.v1.FlashSaleCampaign"},
	"GetFlashSaleStockRequest":      {"campaign_id": "1 string"},
	"GetFlashSaleStockResponse":     {"remaining": "1 int64", "stock_cap": "2 int64"},
	"ReserveFlashSaleStockRequest":  {"reservation_id": "1 string", "campaign_id": "2 string", "listing_id": "3 string", "variant_id": "4 string", "quantity": "5 int64", "expected_sale_price": "6 int64"},
	"ReserveFlashSaleStockResponse": {"reserved": "1 bool", "reason": "2 string"},
	"CommitFlashSaleStockRequest":   {"reservation_id": "1 string"},
	"CommitFlashSaleStockResponse":  {"committed": "1 bool"},
	"ReleaseFlashSaleStockRequest":  {"reservation_id": "1 string"},
	"ReleaseFlashSaleStockResponse": {"released": "1 bool"},
	"AdCampaign":                    {"id": "1 string", "seller_id": "2 string", "listing_id": "3 string", "budget": "4 int64", "bid": "5 int64", "status": "6 enum:platform.promotion.v1.AdCampaignStatus", "created_at": "7 message:google.protobuf.Timestamp"},
	"CreateAdCampaignRequest":       {"listing_id": "1 string", "budget": "2 int64", "bid": "3 int64"},
	"CreateAdCampaignResponse":      {"campaign": "1 message:platform.promotion.v1.AdCampaign"},
	"ListSponsoredSlotsRequest":     {"context_str": "1 string"},
	"ListSponsoredSlotsResponse":    {"listing_ids": "1 repeated string"},
	"VoucherChanged":                {"voucher": "1 message:platform.promotion.v1.Voucher"},
	"FlashSaleChanged":              {"campaign": "1 message:platform.promotion.v1.FlashSaleCampaign"},
}
var monPromoBaselineMethods = map[string]map[string][2]string{
	"VoucherService": {
		"CreateVoucher":      {"CreateVoucherRequest", "CreateVoucherResponse"},
		"GetVoucher":         {"GetVoucherRequest", "GetVoucherResponse"},
		"ListVouchers":       {"ListVouchersRequest", "ListVouchersResponse"},
		"DeactivateVoucher":  {"DeactivateVoucherRequest", "DeactivateVoucherResponse"},
		"UpdateVoucher":      {"UpdateVoucherRequest", "UpdateVoucherResponse"},
		"ValidateAndReserve": {"ValidateAndReserveRequest", "ValidateAndReserveResponse"},
		"CommitReservation":  {"CommitReservationRequest", "CommitReservationResponse"},
		"ReleaseReservation": {"ReleaseReservationRequest", "ReleaseReservationResponse"},
		"ListMyVouchers":     {"ListMyVouchersRequest", "ListMyVouchersResponse"},
	},
	"FlashSaleService": {
		"CreateCampaign":        {"CreateCampaignRequest", "CreateCampaignResponse"},
		"GetActiveFlashSale":    {"GetActiveFlashSaleRequest", "GetActiveFlashSaleResponse"},
		"ListActiveCampaigns":   {"ListActiveCampaignsRequest", "ListActiveCampaignsResponse"},
		"ListCampaigns":         {"ListCampaignsRequest", "ListCampaignsResponse"},
		"CancelCampaign":        {"CancelCampaignRequest", "CancelCampaignResponse"},
		"UpdateCampaign":        {"UpdateCampaignRequest", "UpdateCampaignResponse"},
		"GetFlashSaleStock":     {"GetFlashSaleStockRequest", "GetFlashSaleStockResponse"},
		"ReserveFlashSaleStock": {"ReserveFlashSaleStockRequest", "ReserveFlashSaleStockResponse"},
		"CommitFlashSaleStock":  {"CommitFlashSaleStockRequest", "CommitFlashSaleStockResponse"},
		"ReleaseFlashSaleStock": {"ReleaseFlashSaleStockRequest", "ReleaseFlashSaleStockResponse"},
	},
	"SubscriptionService": {
		"ListPlans":       {"ListPlansRequest", "ListPlansResponse"},
		"Subscribe":       {"SubscribeRequest", "SubscribeResponse"},
		"GetEntitlements": {"GetEntitlementsRequest", "GetEntitlementsResponse"},
	},
	"SponsoredService": {
		"CreateAdCampaign":   {"CreateAdCampaignRequest", "CreateAdCampaignResponse"},
		"ListSponsoredSlots": {"ListSponsoredSlotsRequest", "ListSponsoredSlotsResponse"},
	},
}
var monPromoBaselineEnums = map[string]map[string]int{
	"DiscountType":       {"DISCOUNT_TYPE_UNSPECIFIED": 0, "DISCOUNT_TYPE_PERCENT": 1, "DISCOUNT_TYPE_FIXED": 2},
	"CampaignStatus":     {"CAMPAIGN_STATUS_UNSPECIFIED": 0, "CAMPAIGN_STATUS_SCHEDULED": 1, "CAMPAIGN_STATUS_ACTIVE": 2, "CAMPAIGN_STATUS_ENDED": 3, "CAMPAIGN_STATUS_CANCELLED": 4},
	"VoucherScope":       {"VOUCHER_SCOPE_UNSPECIFIED": 0, "VOUCHER_SCOPE_SHOP": 1, "VOUCHER_SCOPE_PLATFORM": 2},
	"PlanTier":           {"PLAN_TIER_UNSPECIFIED": 0, "PLAN_TIER_FREE": 1, "PLAN_TIER_PRO": 2, "PLAN_TIER_PREMIUM": 3},
	"SubscriptionStatus": {"SUBSCRIPTION_STATUS_UNSPECIFIED": 0, "SUBSCRIPTION_STATUS_ACTIVE": 1, "SUBSCRIPTION_STATUS_EXPIRED": 2, "SUBSCRIPTION_STATUS_CANCELLED": 3},
	"VoucherKind":        {"VOUCHER_KIND_UNSPECIFIED": 0, "VOUCHER_KIND_STANDARD": 1, "VOUCHER_KIND_NEW_USER": 2},
	"AdCampaignStatus":   {"AD_CAMPAIGN_STATUS_UNSPECIFIED": 0, "AD_CAMPAIGN_STATUS_ACTIVE": 1, "AD_CAMPAIGN_STATUS_PAUSED": 2, "AD_CAMPAIGN_STATUS_ENDED": 3},
}

func monFile(t *testing.T) (protoreflect.FileDescriptor, *protoregistry.Files) {
	t.Helper()
	cmd := exec.Command("buf", "build", "-o", "-#format=binpb")
	cmd.Dir = "../../.."
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed (harness): %v\n%s", err, stderr.String())
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatalf("decode image: %v", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		t.Fatalf("build files: %v", err)
	}
	fd, err := files.FindFileByPath(monPromoProto)
	if err != nil {
		t.Fatalf("find promotion.proto: %v", err)
	}
	return fd, files
}

func monMsg(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Fatalf("message %s missing", name)
	}
	return md
}

func monKind(f protoreflect.FieldDescriptor) string {
	var s string
	switch {
	case f.Kind() == protoreflect.MessageKind:
		s = "message:" + string(f.Message().FullName())
	case f.Kind() == protoreflect.EnumKind:
		s = "enum:" + string(f.Enum().FullName())
	default:
		s = f.Kind().String()
	}
	if f.IsList() || f.IsMap() {
		s = "repeated " + s
	} else if f.HasOptionalKeyword() {
		s = "optional " + s
	}
	return s
}

func monFields(md protoreflect.MessageDescriptor) map[string]string {
	got := map[string]string{}
	for i := 0; i < md.Fields().Len(); i++ {
		f := md.Fields().Get(i)
		got[string(f.Name())] = fmt.Sprintf("%d %s", f.Number(), monKind(f))
	}
	return got
}

func monShape(t *testing.T, fd protoreflect.FileDescriptor, name string, want map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Errorf("message %s missing", name)
		return
	}
	got := monFields(md)
	for n, w := range want {
		if g, ok := got[n]; !ok {
			t.Errorf("%s.%s missing (want %s)", name, n, w)
		} else if g != w {
			t.Errorf("%s.%s = %q, want %q", name, n, g, w)
		}
	}
	for n, g := range got {
		if _, ok := want[n]; !ok {
			t.Errorf("%s has unexpected field %s = %q", name, n, g)
		}
	}
}

func monMerge(base map[string]string, extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func monNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

func monComment(fd protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	loc := fd.SourceLocations().ByDescriptor(d)
	return monNorm(loc.LeadingComments + " " + loc.TrailingComments)
}

func monFieldComment(t *testing.T, fd protoreflect.FileDescriptor, msg, field string) string {
	t.Helper()
	f := monMsg(t, fd, msg).Fields().ByName(protoreflect.Name(field))
	if f == nil {
		t.Errorf("%s.%s missing", msg, field)
		return ""
	}
	return monComment(fd, f)
}

func monMethod(t *testing.T, fd protoreflect.FileDescriptor, svcName, name string) protoreflect.MethodDescriptor {
	t.Helper()
	svc := fd.Services().ByName(protoreflect.Name(svcName))
	if svc == nil {
		t.Fatalf("%s missing", svcName)
	}
	m := svc.Methods().ByName(protoreflect.Name(name))
	if m == nil {
		t.Fatalf("rpc %s.%s missing", svcName, name)
	}
	return m
}

func monRpcComment(t *testing.T, fd protoreflect.FileDescriptor, svcName, name string) string {
	t.Helper()
	return monComment(fd, monMethod(t, fd, svcName, name))
}

func monHas(t *testing.T, what, comment string, keys ...string) {
	t.Helper()
	c := strings.ToLower(monNorm(comment))
	if c == "" {
		t.Errorf("%s: no comment found", what)
		return
	}
	for _, k := range keys {
		if !strings.Contains(c, strings.ToLower(monNorm(k))) {
			t.Errorf("%s: comment missing %q; got: %s", what, k, c)
		}
	}
}

func monRpc(t *testing.T, fd protoreflect.FileDescriptor, svcName, name, in, out string) {
	t.Helper()
	svc := fd.Services().ByName(protoreflect.Name(svcName))
	if svc == nil {
		t.Errorf("%s missing", svcName)
		return
	}
	m := svc.Methods().ByName(protoreflect.Name(name))
	if m == nil {
		t.Errorf("rpc %s.%s missing", svcName, name)
		return
	}
	if string(m.Input().Name()) != in || string(m.Output().Name()) != out {
		t.Errorf("rpc %s.%s = %s -> %s, want %s -> %s", svcName, name, m.Input().Name(), m.Output().Name(), in, out)
	}
}

func monFieldNames(md protoreflect.MessageDescriptor) []string {
	var n []string
	for i := 0; i < md.Fields().Len(); i++ {
		n = append(n, string(md.Fields().Get(i).Name()))
	}
	sort.Strings(n)
	return n
}

func monOldFile(t *testing.T, fd protoreflect.FileDescriptor, files *protoregistry.Files, strip func(p *descriptorpb.FileDescriptorProto)) protoreflect.FileDescriptor {
	t.Helper()
	p := protodesc.ToFileDescriptorProto(fd)
	strip(p)
	old, err := protodesc.NewFile(p, files)
	if err != nil {
		t.Fatalf("build old schema: %v", err)
	}
	return old
}

func TestMonetization_PlanTermShape(t *testing.T) {
	fd, _ := monFile(t)
	ts := "message:google.protobuf.Timestamp"
	monShape(t, fd, "Plan", monMerge(monPromoBaselineMessages["Plan"], map[string]string{"term_days": "5 int32"}))
	monShape(t, fd, "SellerSubscription", monMerge(monPromoBaselineMessages["SellerSubscription"], map[string]string{
		"current_period_end": "8 " + ts, "auto_renew": "9 bool",
	}))
}

func TestMonetization_SubscriptionRpcShape(t *testing.T) {
	fd, _ := monFile(t)
	monRpc(t, fd, "SubscriptionService", "GetMySubscription", "GetMySubscriptionRequest", "GetMySubscriptionResponse")
	monRpc(t, fd, "SubscriptionService", "CancelSubscription", "CancelSubscriptionRequest", "CancelSubscriptionResponse")
	sub := "1 message:" + monPromoPkg + "SellerSubscription"
	monShape(t, fd, "GetMySubscriptionRequest", map[string]string{})
	monShape(t, fd, "CancelSubscriptionRequest", map[string]string{})
	monShape(t, fd, "GetMySubscriptionResponse", map[string]string{"subscription": sub})
	monShape(t, fd, "CancelSubscriptionResponse", map[string]string{"subscription": sub})
}

func TestMonetization_SubscriptionSemanticsDocumented(t *testing.T) {
	fd, _ := monFile(t)
	monHas(t, "Plan.price", monFieldComment(t, fd, "Plan", "price"), "integer VND", "per term")
	monHas(t, "GetMySubscriptionResponse.subscription", monFieldComment(t, fd, "GetMySubscriptionResponse", "subscription"), "FREE seller", "no subscription")
	monHas(t, "CancelSubscription", monRpcComment(t, fd, "SubscriptionService", "CancelSubscription"), "auto-renew", "current_period_end")
}

func TestMonetization_SubscriptionRequestsCarryNoSellerId(t *testing.T) {
	fd, _ := monFile(t)
	for _, n := range []string{"GetMySubscriptionRequest", "CancelSubscriptionRequest"} {
		if got := monFieldNames(monMsg(t, fd, n)); len(got) != 0 {
			t.Errorf("%s has fields %v, want none", n, got)
		}
	}
}

func TestMonetization_AdCampaignShape(t *testing.T) {
	fd, _ := monFile(t)
	e := fd.Enums().ByName("AdCampaignStatus")
	if e == nil {
		t.Fatal("AdCampaignStatus missing")
	}
	want := map[string]int{"AD_CAMPAIGN_STATUS_PAUSED_BUDGET": 4}
	for k, v := range monPromoBaselineEnums["AdCampaignStatus"] {
		want[k] = v
	}
	if e.Values().Len() != 5 {
		t.Errorf("AdCampaignStatus has %d values, want 5", e.Values().Len())
	}
	for n, num := range want {
		v := e.Values().ByName(protoreflect.Name(n))
		if v == nil {
			t.Errorf("%s missing", n)
		} else if int(v.Number()) != num {
			t.Errorf("%s = %d, want %d", n, v.Number(), num)
		}
	}
	monShape(t, fd, "AdCampaign", monMerge(monPromoBaselineMessages["AdCampaign"], map[string]string{
		"spent": "8 int64", "impressions": "9 int64", "clicks": "10 int64", "category_id": "11 string",
	}))
}

func TestMonetization_OldSchemaParsesPausedBudget(t *testing.T) {
	fd, files := monFile(t)
	cur := monMsg(t, fd, "AdCampaign")
	if fd.Enums().ByName("AdCampaignStatus").Values().ByNumber(4) == nil {
		t.Fatal("new schema does not define AdCampaignStatus 4")
	}
	old := monOldFile(t, fd, files, func(p *descriptorpb.FileDescriptorProto) {
		for _, e := range p.EnumType {
			if e.GetName() == "AdCampaignStatus" {
				var keep []*descriptorpb.EnumValueDescriptorProto
				for _, v := range e.Value {
					if v.GetNumber() <= 3 {
						keep = append(keep, v)
					}
				}
				e.Value = keep
			}
		}
	})
	m := dynamicpb.NewMessage(cur)
	m.Set(cur.Fields().ByName("id"), protoreflect.ValueOfString("c-1"))
	m.Set(cur.Fields().ByName("budget"), protoreflect.ValueOfInt64(100000))
	m.Set(cur.Fields().ByName("bid"), protoreflect.ValueOfInt64(500))
	m.Set(cur.Fields().ByName("status"), protoreflect.ValueOfEnum(4))
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	oldMd := old.Messages().ByName("AdCampaign")
	got := dynamicpb.NewMessage(oldMd)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("old schema rejects status 4: %v", err)
	}
	sf := oldMd.Fields().ByName("status")
	if got.Get(sf).Enum() != 4 || sf.Enum().Values().ByNumber(4) != nil {
		t.Errorf("status = %d, want 4 with no defined name", got.Get(sf).Enum())
	}
	if got.Get(oldMd.Fields().ByName("id")).String() != "c-1" || got.Get(oldMd.Fields().ByName("budget")).Int() != 100000 || got.Get(oldMd.Fields().ByName("bid")).Int() != 500 {
		t.Errorf("other fields changed: %v", got)
	}
}

func TestMonetization_AdCampaignSemanticsDocumented(t *testing.T) {
	fd, _ := monFile(t)
	monHas(t, "AdCampaign.budget", monFieldComment(t, fd, "AdCampaign", "budget"), "integer VND")
	monHas(t, "AdCampaign.bid", monFieldComment(t, fd, "AdCampaign", "bid"), "integer VND", "500", "INVALID_ARGUMENT")
	monHas(t, "AdCampaign.spent", monFieldComment(t, fd, "AdCampaign", "spent"), "integer VND")
	v := fd.Enums().ByName("AdCampaignStatus").Values().ByName("AD_CAMPAIGN_STATUS_PAUSED_BUDGET")
	if v == nil {
		t.Fatal("AD_CAMPAIGN_STATUS_PAUSED_BUDGET missing")
	}
	monHas(t, "PAUSED_BUDGET", monComment(fd, v), "remaining budget", "below the bid")
}

func TestMonetization_SponsoredSlotsShape(t *testing.T) {
	fd, _ := monFile(t)
	monShape(t, fd, "ListSponsoredSlotsRequest", map[string]string{
		"context_str": "1 string", "placement": "2 string", "category_id": "3 string", "limit": "4 int32",
	})
	monShape(t, fd, "SponsoredSlot", map[string]string{
		"listing_id": "1 string", "campaign_id": "2 string", "impression_token": "3 string",
	})
	monShape(t, fd, "ListSponsoredSlotsResponse", map[string]string{
		"listing_ids": "1 repeated string", "slots": "2 repeated message:" + monPromoPkg + "SponsoredSlot",
	})
}

func TestMonetization_SlotBoundAndTokenDocumented(t *testing.T) {
	fd, _ := monFile(t)
	monHas(t, "limit", monFieldComment(t, fd, "ListSponsoredSlotsRequest", "limit"), "1..4", "INVALID_ARGUMENT")
	monHas(t, "impression_token", monFieldComment(t, fd, "SponsoredSlot", "impression_token"), "opaque", "single-use", "30 minutes")
}

func TestMonetization_OldSponsoredSlotsParse(t *testing.T) {
	fd, _ := monFile(t)
	req := monMsg(t, fd, "ListSponsoredSlotsRequest")
	var rb []byte
	rb = protowire.AppendTag(rb, 1, protowire.BytesType)
	rb = protowire.AppendString(rb, "search:shoes")
	gr := dynamicpb.NewMessage(req)
	if err := proto.Unmarshal(rb, gr); err != nil {
		t.Fatalf("old request rejected: %v", err)
	}
	if gr.Get(req.Fields().ByName("context_str")).String() != "search:shoes" {
		t.Errorf("context_str changed: %v", gr)
	}
	for _, n := range []string{"placement", "category_id"} {
		f := req.Fields().ByName(protoreflect.Name(n))
		if f == nil {
			t.Errorf("%s missing from schema", n)
		} else if gr.Get(f).String() != "" {
			t.Errorf("%s = %q, want zero", n, gr.Get(f).String())
		}
	}
	if f := req.Fields().ByName("limit"); f == nil {
		t.Errorf("limit missing from schema")
	} else if gr.Get(f).Int() != 0 {
		t.Errorf("limit = %d, want 0", gr.Get(f).Int())
	}

	resp := monMsg(t, fd, "ListSponsoredSlotsResponse")
	var pb []byte
	for _, id := range []string{"l-1", "l-2"} {
		pb = protowire.AppendTag(pb, 1, protowire.BytesType)
		pb = protowire.AppendString(pb, id)
	}
	gp := dynamicpb.NewMessage(resp)
	if err := proto.Unmarshal(pb, gp); err != nil {
		t.Fatalf("old response rejected: %v", err)
	}
	ids := gp.Get(resp.Fields().ByName("listing_ids")).List()
	if ids.Len() != 2 || ids.Get(0).String() != "l-1" || ids.Get(1).String() != "l-2" {
		t.Errorf("listing_ids changed: %v", gp)
	}
	sf := resp.Fields().ByName("slots")
	if sf == nil {
		t.Fatal("slots missing from schema")
	}
	if gp.Get(sf).List().Len() != 0 {
		t.Errorf("slots non-empty for an old response")
	}
}

func TestMonetization_NoBuyerIdentifiers(t *testing.T) {
	fd, _ := monFile(t)
	if got, want := monFieldNames(monMsg(t, fd, "SponsoredSlot")), []string{"campaign_id", "impression_token", "listing_id"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("SponsoredSlot fields = %v, want %v", got, want)
	}
	if got := monFieldNames(monMsg(t, fd, "RecordAdClickRequest")); strings.Join(got, ",") != "impression_token" {
		t.Errorf("RecordAdClickRequest fields = %v, want [impression_token]", got)
	}
}

func TestMonetization_AdRpcsShape(t *testing.T) {
	fd, _ := monFile(t)
	for _, n := range []string{"ListMyAdCampaigns", "PauseAdCampaign", "ResumeAdCampaign", "RecordAdClick"} {
		monRpc(t, fd, "SponsoredService", n, n+"Request", n+"Response")
	}
	camp := "message:" + monPromoPkg + "AdCampaign"
	monShape(t, fd, "ListMyAdCampaignsRequest", map[string]string{"page": "1 " + monPageReq})
	monShape(t, fd, "ListMyAdCampaignsResponse", map[string]string{"campaigns": "1 repeated " + camp, "page": "2 " + monPageResp})
	monShape(t, fd, "PauseAdCampaignRequest", map[string]string{"campaign_id": "1 string"})
	monShape(t, fd, "PauseAdCampaignResponse", map[string]string{"campaign": "1 " + camp})
	monShape(t, fd, "ResumeAdCampaignRequest", map[string]string{"campaign_id": "1 string"})
	monShape(t, fd, "ResumeAdCampaignResponse", map[string]string{"campaign": "1 " + camp})
	monShape(t, fd, "RecordAdClickRequest", map[string]string{"impression_token": "1 string"})
	monShape(t, fd, "RecordAdClickResponse", map[string]string{"listing_id": "1 string", "charged": "2 bool"})
}

func TestMonetization_CampaignRulesDocumented(t *testing.T) {
	fd, _ := monFile(t)
	for _, n := range []string{"ListMyAdCampaigns", "PauseAdCampaign", "ResumeAdCampaign"} {
		monHas(t, n, monRpcComment(t, fd, "SponsoredService", n), "own campaigns")
	}
	monHas(t, "PauseAdCampaign", monRpcComment(t, fd, "SponsoredService", "PauseAdCampaign"), "NOT_FOUND")
	monHas(t, "ResumeAdCampaign", monRpcComment(t, fd, "SponsoredService", "ResumeAdCampaign"), "NOT_FOUND", "FAILED_PRECONDITION", "budget", "bid")
}

func TestMonetization_ClickRulesDocumented(t *testing.T) {
	fd, _ := monFile(t)
	monHas(t, "RecordAdClick", monRpcComment(t, fd, "SponsoredService", "RecordAdClick"), "anonymous", "at most once per token")
	monHas(t, "RecordAdClickResponse.charged", monFieldComment(t, fd, "RecordAdClickResponse", "charged"), "false", "replayed", "expired", "own seller")
}

func TestMonetization_ExistingPromotionSurfaceUntouched(t *testing.T) {
	fd, _ := monFile(t)
	for svc, methods := range monPromoBaselineMethods {
		for n, io := range methods {
			monRpc(t, fd, svc, n, io[0], io[1])
		}
	}
	for e, vals := range monPromoBaselineEnums {
		ed := fd.Enums().ByName(protoreflect.Name(e))
		if ed == nil {
			t.Errorf("enum %s removed", e)
			continue
		}
		for n, num := range vals {
			v := ed.Values().ByName(protoreflect.Name(n))
			if v == nil || int(v.Number()) != num {
				t.Errorf("enum value %s.%s changed or removed (baseline %d)", e, n, num)
			}
		}
	}
	for name, base := range monPromoBaselineMessages {
		md := fd.Messages().ByName(protoreflect.Name(name))
		if md == nil {
			t.Errorf("message %s removed", name)
			continue
		}
		got := monFields(md)
		for f, w := range base {
			if g, ok := got[f]; !ok {
				t.Errorf("%s.%s removed or renamed (baseline %s)", name, f, w)
			} else if g != w {
				t.Errorf("%s.%s = %q, baseline %q", name, f, g, w)
			}
		}
	}
}
