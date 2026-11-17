package paymentv1_test

// Tests for change monetization-contract, capabilities "Fee and commission
// ledger types", "Order settlement lookup RPC" and "Seller fee charge RPC".
// Baselines are pinned from origin/main.

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	monPaymentProto = "platform/payment/v1/payment.proto"
	monTS           = "message:google.protobuf.Timestamp"
	monPkg          = "platform.payment.v1."
)

var monBaselineMessages = map[string]map[string]string{
	"PaymentSettled":              {"payment_id": "1 string", "order_id": "2 string", "buyer_id": "3 string", "status": "4 enum:" + monPkg + "PaymentStatus", "occurred_at": "5 string"},
	"PaymentTransaction":          {"id": "1 string", "order_id": "2 string", "buyer_id": "3 string", "amount": "4 int64", "currency": "5 string", "method": "6 enum:" + monPkg + "PaymentMethod", "status": "7 enum:" + monPkg + "PaymentStatus", "provider_reference": "8 string", "created_at": "9 " + monTS, "updated_at": "10 " + monTS, "refunded_amount": "11 int64", "refunds": "12 repeated message:" + monPkg + "PaymentRefund"},
	"PaymentRefund":               {"id": "1 string", "source": "2 enum:" + monPkg + "PaymentRefundSource", "source_id": "3 string", "requested_amount": "4 int64", "amount": "5 int64", "reason": "6 string", "created_at": "7 " + monTS},
	"SellerWallet":                {"id": "1 string", "seller_id": "2 string", "balance": "3 int64", "currency": "4 string", "updated_at": "5 " + monTS},
	"PayoutRequest":               {"id": "1 string", "seller_id": "2 string", "amount": "3 int64", "bank_code": "4 string", "account_number": "5 string", "account_name": "6 string", "status": "7 enum:" + monPkg + "PayoutStatus", "created_at": "8 " + monTS},
	"WalletTransaction":           {"id": "1 string", "wallet_id": "2 string", "amount": "3 int64", "type": "4 enum:" + monPkg + "WalletTransactionType", "reference_id": "5 string", "created_at": "6 " + monTS},
	"WalletEntry":                 {"id": "1 string", "seller_id": "2 string", "type": "3 string", "amount": "4 int64", "status": "5 string", "created_at": "6 " + monTS, "reference_id": "7 string"},
	"GetWalletBalanceRequest":     {"seller_id": "1 string"},
	"GetWalletBalanceResponse":    {"balance": "1 int64"},
	"ListLedgerEntriesRequest":    {"seller_id": "1 string", "page": "2 message:platform.common.v1.PageRequest"},
	"ListLedgerEntriesResponse":   {"entries": "1 repeated message:" + monPkg + "WalletEntry", "page": "2 message:platform.common.v1.PageResponse"},
	"RequestWalletPayoutRequest":  {"seller_id": "1 string", "amount": "2 int64"},
	"RequestWalletPayoutResponse": {"entry": "1 message:" + monPkg + "WalletEntry"},
	"CreatePaymentRequest":        {"order_id": "1 string", "method": "2 enum:" + monPkg + "PaymentMethod"},
	"CreatePaymentResponse":       {"transaction": "1 message:" + monPkg + "PaymentTransaction", "payment_url": "2 string"},
	"GetPaymentRequest":           {"id": "1 string", "order_id": "2 string"},
	"GetPaymentResponse":          {"transaction": "1 message:" + monPkg + "PaymentTransaction"},
	"ProcessMockPaymentRequest":   {"transaction_id": "1 string", "simulate_success": "2 bool"},
	"ProcessMockPaymentResponse":  {"transaction": "1 message:" + monPkg + "PaymentTransaction", "success": "2 bool", "message": "3 string"},
	"GetSellerWalletRequest":      {"seller_id": "1 string"},
	"GetSellerWalletResponse":     {"wallet": "1 message:" + monPkg + "SellerWallet"},
	"RequestPayoutRequest":        {"seller_id": "1 string", "amount": "2 int64", "bank_code": "3 string", "account_number": "4 string", "account_name": "5 string"},
	"RequestPayoutResponse":       {"payout": "1 message:" + monPkg + "PayoutRequest"},
	"ListPayoutHistoryRequest":    {"seller_id": "1 string"},
	"ListPayoutHistoryResponse":   {"payouts": "1 repeated message:" + monPkg + "PayoutRequest"},
	"RefundPaymentRequest":        {"payment_id": "1 string", "amount": "2 int64", "reason": "3 string", "refund_id": "4 string"},
	"RefundPaymentResponse":       {"transaction": "1 message:" + monPkg + "PaymentTransaction", "success": "2 bool", "message": "3 string"},
}

var monBaselineMethods = map[string][2]string{
	"CreatePayment": {"CreatePaymentRequest", "CreatePaymentResponse"}, "GetPayment": {"GetPaymentRequest", "GetPaymentResponse"},
	"ProcessMockPayment": {"ProcessMockPaymentRequest", "ProcessMockPaymentResponse"}, "GetSellerWallet": {"GetSellerWalletRequest", "GetSellerWalletResponse"},
	"RequestPayout": {"RequestPayoutRequest", "RequestPayoutResponse"}, "ListPayoutHistory": {"ListPayoutHistoryRequest", "ListPayoutHistoryResponse"},
	"RefundPayment": {"RefundPaymentRequest", "RefundPaymentResponse"}, "GetWalletBalance": {"GetWalletBalanceRequest", "GetWalletBalanceResponse"},
	"ListLedgerEntries": {"ListLedgerEntriesRequest", "ListLedgerEntriesResponse"}, "RequestWalletPayout": {"RequestWalletPayoutRequest", "RequestWalletPayoutResponse"},
}

var monBaselineWalletTypes = map[string]int{
	"WALLET_TRANSACTION_TYPE_UNSPECIFIED": 0, "WALLET_TRANSACTION_TYPE_ORDER_SETTLEMENT": 1,
	"WALLET_TRANSACTION_TYPE_PAYOUT": 2, "WALLET_TRANSACTION_TYPE_REFUND_DEDUCTION": 3,
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
	fd, err := files.FindFileByPath(monPaymentProto)
	if err != nil {
		t.Fatalf("find payment.proto: %v", err)
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
	if f.IsList() {
		s = "repeated " + s
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

func monMethod(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MethodDescriptor {
	t.Helper()
	svc := fd.Services().ByName("PaymentService")
	if svc == nil {
		t.Fatal("PaymentService missing")
	}
	m := svc.Methods().ByName(protoreflect.Name(name))
	if m == nil {
		t.Fatalf("rpc %s missing", name)
	}
	return m
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

func TestMonetization_WalletTypesShape(t *testing.T) {
	fd, _ := monFile(t)
	e := fd.Enums().ByName("WalletTransactionType")
	if e == nil {
		t.Fatal("WalletTransactionType missing")
	}
	want := map[string]int{
		"WALLET_TRANSACTION_TYPE_COMMISSION": 4, "WALLET_TRANSACTION_TYPE_COMMISSION_REVERSAL": 5,
		"WALLET_TRANSACTION_TYPE_SUBSCRIPTION_FEE": 6, "WALLET_TRANSACTION_TYPE_AD_SPEND": 7,
	}
	for k, v := range monBaselineWalletTypes {
		want[k] = v
	}
	if e.Values().Len() != 8 {
		t.Errorf("WalletTransactionType has %d values, want 8", e.Values().Len())
	}
	for n, num := range want {
		v := e.Values().ByName(protoreflect.Name(n))
		if v == nil {
			t.Errorf("%s missing", n)
		} else if int(v.Number()) != num {
			t.Errorf("%s = %d, want %d", n, v.Number(), num)
		}
	}
}

func TestMonetization_OldSchemaParsesNewType(t *testing.T) {
	fd, files := monFile(t)
	cur := monMsg(t, fd, "WalletTransaction")
	if fd.Enums().ByName("WalletTransactionType").Values().ByNumber(4) == nil {
		t.Fatal("new schema does not define WalletTransactionType 4")
	}
	p := protodesc.ToFileDescriptorProto(fd)
	for _, e := range p.EnumType {
		if e.GetName() == "WalletTransactionType" {
			var keep []*descriptorpb.EnumValueDescriptorProto
			for _, v := range e.Value {
				if v.GetNumber() <= 3 {
					keep = append(keep, v)
				}
			}
			e.Value = keep
		}
	}
	oldFile, err := protodesc.NewFile(p, files)
	if err != nil {
		t.Fatalf("build old schema: %v", err)
	}
	m := dynamicpb.NewMessage(cur)
	m.Set(cur.Fields().ByName("id"), protoreflect.ValueOfString("w-1"))
	m.Set(cur.Fields().ByName("amount"), protoreflect.ValueOfInt64(5000))
	m.Set(cur.Fields().ByName("type"), protoreflect.ValueOfEnum(4))
	m.Set(cur.Fields().ByName("reference_id"), protoreflect.ValueOfString("o-1"))
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	oldMd := oldFile.Messages().ByName("WalletTransaction")
	got := dynamicpb.NewMessage(oldMd)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("old schema rejects type 4: %v", err)
	}
	tf := oldMd.Fields().ByName("type")
	if got.Get(tf).Enum() != 4 || tf.Enum().Values().ByNumber(4) != nil {
		t.Errorf("type = %d, want 4 with no defined name", got.Get(tf).Enum())
	}
	if got.Get(oldMd.Fields().ByName("id")).String() != "w-1" || got.Get(oldMd.Fields().ByName("amount")).Int() != 5000 || got.Get(oldMd.Fields().ByName("reference_id")).String() != "o-1" {
		t.Errorf("other fields changed: %v", got)
	}
}

func TestMonetization_AdSpendDocumentedReserved(t *testing.T) {
	fd, _ := monFile(t)
	v := fd.Enums().ByName("WalletTransactionType").Values().ByName("WALLET_TRANSACTION_TYPE_AD_SPEND")
	if v == nil {
		t.Fatal("WALLET_TRANSACTION_TYPE_AD_SPEND missing")
	}
	monHas(t, "AD_SPEND", monComment(fd, v), "reserved", "not written in v1", "campaign counter")
}

func TestMonetization_GetOrderSettlementShape(t *testing.T) {
	fd, _ := monFile(t)
	m := monMethod(t, fd, "GetOrderSettlement")
	if m.Input().Name() != "GetOrderSettlementRequest" || m.Output().Name() != "GetOrderSettlementResponse" {
		t.Errorf("GetOrderSettlement types = %s -> %s", m.Input().Name(), m.Output().Name())
	}
	monShape(t, fd, "GetOrderSettlementRequest", map[string]string{"order_id": "1 string"})
	monShape(t, fd, "GetOrderSettlementResponse", map[string]string{
		"order_id": "1 string", "credited": "2 int64", "commission": "3 int64", "commission_reversed": "4 int64",
		"refund_deducted": "5 int64", "net": "6 int64", "commission_rate_bps": "7 int32",
	})
}

func TestMonetization_SettlementSemanticsDocumented(t *testing.T) {
	fd, _ := monFile(t)
	rpc := monComment(fd, monMethod(t, fd, "GetOrderSettlement"))
	net := monFieldComment(t, fd, "GetOrderSettlementResponse", "net")
	formula := "credited - commission + commission_reversed - refund_deducted"
	monHas(t, "GetOrderSettlement rpc", rpc, formula, "seller", "admin", "NOT_FOUND", "positive integer VND")
	monHas(t, "net", net, formula)
}

func TestMonetization_SettlementNotFoundDocumented(t *testing.T) {
	fd, _ := monFile(t)
	rpc := monComment(fd, monMethod(t, fd, "GetOrderSettlement"))
	monHas(t, "GetOrderSettlement rpc", rpc, "NOT_FOUND", "another seller", "unsettled", "does not reveal")
}

func TestMonetization_NoFloatMoney(t *testing.T) {
	fd, _ := monFile(t)
	for _, name := range []string{"GetOrderSettlementRequest", "GetOrderSettlementResponse", "ChargeSellerFeeRequest", "ChargeSellerFeeResponse"} {
		md := monMsg(t, fd, name)
		for i := 0; i < md.Fields().Len(); i++ {
			k := md.Fields().Get(i).Kind()
			if k == protoreflect.DoubleKind || k == protoreflect.FloatKind {
				t.Errorf("%s.%s is %s", name, md.Fields().Get(i).Name(), k)
			}
		}
	}
	md := monMsg(t, fd, "GetOrderSettlementResponse")
	for _, n := range []string{"credited", "commission", "commission_reversed", "refund_deducted", "net"} {
		f := md.Fields().ByName(protoreflect.Name(n))
		if f == nil || f.Kind() != protoreflect.Int64Kind {
			t.Errorf("GetOrderSettlementResponse.%s is not int64", n)
		}
	}
}

func TestMonetization_ChargeSellerFeeShape(t *testing.T) {
	fd, _ := monFile(t)
	m := monMethod(t, fd, "ChargeSellerFee")
	if m.Input().Name() != "ChargeSellerFeeRequest" || m.Output().Name() != "ChargeSellerFeeResponse" {
		t.Errorf("ChargeSellerFee types = %s -> %s", m.Input().Name(), m.Output().Name())
	}
	e := fd.Enums().ByName("SellerFeeKind")
	if e == nil {
		t.Fatal("SellerFeeKind missing")
	}
	want := map[string]int{"SELLER_FEE_KIND_UNSPECIFIED": 0, "SELLER_FEE_KIND_SUBSCRIPTION": 1, "SELLER_FEE_KIND_AD_SPEND": 2}
	if e.Values().Len() != 3 {
		t.Errorf("SellerFeeKind has %d values, want 3", e.Values().Len())
	}
	for n, num := range want {
		if v := e.Values().ByName(protoreflect.Name(n)); v == nil || int(v.Number()) != num {
			t.Errorf("%s want %d", n, num)
		}
	}
	monShape(t, fd, "ChargeSellerFeeRequest", map[string]string{
		"seller_id": "1 string", "amount": "2 int64", "kind": "3 enum:" + monPkg + "SellerFeeKind", "reference_id": "4 string",
	})
	monShape(t, fd, "ChargeSellerFeeResponse", map[string]string{"entry_id": "1 string", "created": "2 bool"})
}

func TestMonetization_ChargeSellerFeeSemanticsDocumented(t *testing.T) {
	fd, _ := monFile(t)
	rpc := monComment(fd, monMethod(t, fd, "ChargeSellerFee"))
	monHas(t, "ChargeSellerFee rpc", rpc, "created = false", "nothing booked", "withdrawable balance", "FAILED_PRECONDITION")
	monHas(t, "amount", monFieldComment(t, fd, "ChargeSellerFeeRequest", "amount"), "integer VND", "greater than 0", "INVALID_ARGUMENT")
	monHas(t, "reference_id", monFieldComment(t, fd, "ChargeSellerFeeRequest", "reference_id"), "idempotency key")
	monHas(t, "created", monFieldComment(t, fd, "ChargeSellerFeeResponse", "created"), "replay", "false")
}

func TestMonetization_ChargeSellerFeeInternalOnlyDocumented(t *testing.T) {
	fd, _ := monFile(t)
	rpc := monComment(fd, monMethod(t, fd, "ChargeSellerFee"))
	monHas(t, "ChargeSellerFee rpc", rpc, "service-to-service only", "not exposed by team-gateway")
	v := fd.Enums().ByName("SellerFeeKind").Values().ByName("SELLER_FEE_KIND_AD_SPEND")
	if v == nil {
		t.Fatal("SELLER_FEE_KIND_AD_SPEND missing")
	}
	monHas(t, "SELLER_FEE_KIND_AD_SPEND", monComment(fd, v), "reserved", "rejected in v1")
}

func TestMonetization_ChargeSellerFeeNoUserScopeDocumented(t *testing.T) {
	fd, _ := monFile(t)
	rpc := monComment(fd, monMethod(t, fd, "ChargeSellerFee"))
	monHas(t, "ChargeSellerFee rpc", rpc, "no end-user principal or scope may call it", "another seller's wallet")
}

func TestMonetization_ExistingPaymentSurfaceUntouched(t *testing.T) {
	fd, _ := monFile(t)
	svc := fd.Services().ByName("PaymentService")
	if svc == nil {
		t.Fatal("PaymentService missing")
	}
	for n, io := range monBaselineMethods {
		m := svc.Methods().ByName(protoreflect.Name(n))
		if m == nil {
			t.Errorf("rpc %s removed", n)
			continue
		}
		if string(m.Input().Name()) != io[0] || string(m.Output().Name()) != io[1] {
			t.Errorf("rpc %s = %s -> %s, baseline %v", n, m.Input().Name(), m.Output().Name(), io)
		}
	}
	for name, base := range monBaselineMessages {
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
	if k := fd.Messages().ByName("WalletEntry").Fields().ByName("type").Kind(); k != protoreflect.StringKind {
		t.Errorf("WalletEntry.type kind = %s, want string", k)
	}
}
