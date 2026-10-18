package orderv1_test

// Tests for change q10-order-lifecycle-contract (capability "Order lifecycle contract").
// Same approach as analytics/v1/loopevents_compat_test.go: the schema comes
// from `buf build -o -#format=json` (a FileDescriptorSet as JSON) and a small
// schema-driven proto3 wire codec proves round-trips and forward compatibility
// without generated code. Order field numbers 25 and above belong to
// q11-checkout-coherence and are deliberately never asserted here.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	orderFile   = "platform/order/v1/order.proto"
	paymentFile = "platform/payment/v1/payment.proto"
	orderPkg    = ".platform.order.v1."
)

// ---- descriptor model ------------------------------------------------------

type fieldDesc struct {
	Name     string `json:"name"`
	Number   int    `json:"number"`
	Type     string `json:"type"`
	TypeName string `json:"typeName"`
	Label    string `json:"label"`
}

type msgDesc struct {
	Name  string      `json:"name"`
	Field []fieldDesc `json:"field"`
}

type enumValueDesc struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type enumDesc struct {
	Name  string          `json:"name"`
	Value []enumValueDesc `json:"value"`
}

type methodDesc struct {
	Name       string `json:"name"`
	InputType  string `json:"inputType"`
	OutputType string `json:"outputType"`
}

type svcDesc struct {
	Name   string       `json:"name"`
	Method []methodDesc `json:"method"`
}

type location struct {
	Path            []int  `json:"path"`
	LeadingComments string `json:"leadingComments"`
	TrailingComment string `json:"trailingComments"`
}

type fileDesc struct {
	Name           string     `json:"name"`
	Package        string     `json:"package"`
	MessageType    []msgDesc  `json:"messageType"`
	EnumType       []enumDesc `json:"enumType"`
	Service        []svcDesc  `json:"service"`
	SourceCodeInfo struct {
		Location []location `json:"location"`
	} `json:"sourceCodeInfo"`
}

type descSet struct {
	files []fileDesc
}

func loadSet(t *testing.T) descSet {
	t.Helper()
	cmd := exec.Command("buf", "build", "--path", orderFile, "--path", paymentFile, "-o", "-#format=json")
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

func (s descSet) file(t *testing.T, name string) fileDesc {
	t.Helper()
	for _, f := range s.files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s not in descriptor set", name)
	return fileDesc{}
}

// byFullName resolves ".pkg.Message" across every file in the set.
func (s descSet) byFullName(name string) (msgDesc, bool) {
	for _, f := range s.files {
		for _, m := range f.MessageType {
			if "."+f.Package+"."+m.Name == name {
				return m, true
			}
		}
	}
	return msgDesc{}, false
}

func (f fileDesc) message(t *testing.T, name string) (msgDesc, int) {
	t.Helper()
	for i, m := range f.MessageType {
		if m.Name == name {
			return m, i
		}
	}
	t.Fatalf("message %s.%s missing from descriptor", f.Package, name)
	return msgDesc{}, -1
}

func (f fileDesc) enum(t *testing.T, name string) (enumDesc, int) {
	t.Helper()
	for i, e := range f.EnumType {
		if e.Name == name {
			return e, i
		}
	}
	t.Fatalf("enum %s.%s missing from descriptor", f.Package, name)
	return enumDesc{}, -1
}

func (f fileDesc) service(t *testing.T, name string) (svcDesc, int) {
	t.Helper()
	for i, s := range f.Service {
		if s.Name == name {
			return s, i
		}
	}
	t.Fatalf("service %s.%s missing from descriptor", f.Package, name)
	return svcDesc{}, -1
}

func (m msgDesc) field(t *testing.T, name string) (fieldDesc, int) {
	t.Helper()
	for i, fd := range m.Field {
		if fd.Name == name {
			return fd, i
		}
	}
	t.Fatalf("field %s.%s missing", m.Name, name)
	return fieldDesc{}, -1
}

func (e enumDesc) number(name string) (int, bool) {
	for _, v := range e.Value {
		if v.Name == name {
			return v.Number, true
		}
	}
	return 0, false
}

func (f fileDesc) comment(path ...int) (string, bool) {
	for _, l := range f.SourceCodeInfo.Location {
		if reflect.DeepEqual(l.Path, path) {
			return l.LeadingComments + l.TrailingComment, true
		}
	}
	return "", false
}

// ---- pinned shapes ---------------------------------------------------------

type wf struct {
	name     string
	num      int
	typ      string
	typeName string
}

func str(name string, n int) wf     { return wf{name, n, "TYPE_STRING", ""} }
func i64(name string, n int) wf     { return wf{name, n, "TYPE_INT64", ""} }
func i32(name string, n int) wf     { return wf{name, n, "TYPE_INT32", ""} }
func boolean(name string, n int) wf { return wf{name, n, "TYPE_BOOL", ""} }
func ts(name string, n int) wf      { return wf{name, n, "TYPE_MESSAGE", ".google.protobuf.Timestamp"} }
func enm(name string, n int, e string) wf {
	return wf{name, n, "TYPE_ENUM", e}
}
func msg(name string, n int, m string) wf { return wf{name, n, "TYPE_MESSAGE", m} }

// diffFields reports every difference between the message's fields numbered
// <= max (max<=0: all fields) and want, comparing name, number, type and type name.
func diffFields(m msgDesc, want []wf, max int) []string {
	got := map[int]wf{}
	for _, fd := range m.Field {
		if max > 0 && fd.Number > max {
			continue
		}
		got[fd.Number] = wf{fd.Name, fd.Number, fd.Type, fd.TypeName}
	}
	exp := map[int]wf{}
	for _, w := range want {
		exp[w.num] = w
	}
	var d []string
	var nums []int
	for n := range exp {
		nums = append(nums, n)
	}
	for n := range got {
		if _, ok := exp[n]; !ok {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	for _, n := range nums {
		g, gok := got[n]
		e, eok := exp[n]
		switch {
		case !gok:
			d = append(d, fmt.Sprintf("%s.%d (%s %s) missing", m.Name, n, e.name, e.typ))
		case !eok:
			d = append(d, fmt.Sprintf("%s.%d unexpected field %s", m.Name, n, g.name))
		case g != e:
			d = append(d, fmt.Sprintf("%s.%d = %+v, want %+v", m.Name, n, g, e))
		}
	}
	return d
}

func requireFields(t *testing.T, m msgDesc, want []wf, max int) {
	t.Helper()
	for _, d := range diffFields(m, want, max) {
		t.Error(d)
	}
}

var (
	cancelReasonT = orderPkg + "CancelReason"
	completedByT  = orderPkg + "CompletedBy"
	orderT        = orderPkg + "Order"
	payMethodT    = ".platform.payment.v1.PaymentMethod"
)

// Order fields 1-17 exist today and must not move.
var orderExisting = []wf{
	str("id", 1), str("buyer_id", 2), str("seller_id", 3),
	enm("status", 4, orderPkg+"OrderStatus"), i64("total_amount", 5), str("currency", 6),
	msg("shipping_address", 7, ".platform.identity.v1.Address"),
	{"items", 8, "TYPE_MESSAGE", orderPkg + "OrderItem"},
	str("tracking_number", 9), ts("created_at", 10), ts("updated_at", 11),
	i64("shipping_fee", 12), enm("payment_method", 13, payMethodT),
	i64("items_subtotal", 14), i64("discount_amount", 15), str("voucher_code", 16),
	ts("paid_at", 17),
}

var orderLifecycle = []wf{
	ts("payment_due_at", 18), ts("ship_by", 19), ts("shipped_at", 20),
	ts("completed_at", 21), ts("return_window_ends_at", 22),
	enm("cancel_reason", 23, cancelReasonT), ts("auto_complete_at", 24),
}

var orderPlacedF = []wf{
	str("order_id", 1), str("buyer_id", 2), str("seller_id", 3),
	enm("payment_method", 4, payMethodT), i64("total_amount", 5), str("currency", 6),
	ts("placed_at", 7), ts("payment_due_at", 8), ts("ship_by", 9),
}

var orderReminderF = []wf{
	str("order_id", 1), str("buyer_id", 2), i64("total_amount", 3), str("currency", 4),
	ts("payment_due_at", 5),
}

var orderCompletedF = []wf{
	str("order_id", 1), str("buyer_id", 2), str("seller_id", 3), i64("total_amount", 4),
	str("currency", 5), enm("completed_by", 6, completedByT), ts("completed_at", 7),
	ts("return_window_ends_at", 8), boolean("paid_online", 9),
}

var returnRequestedF = []wf{
	str("return_id", 1), str("order_id", 2), str("buyer_id", 3), str("seller_id", 4),
	i64("refund_amount", 5), str("currency", 6), ts("requested_at", 7),
}

// ---- minimal schema-driven proto3 wire codec -------------------------------

func isVarint(t string) bool {
	switch t {
	case "TYPE_BOOL", "TYPE_INT32", "TYPE_INT64", "TYPE_UINT32", "TYPE_UINT64", "TYPE_ENUM":
		return true
	}
	return false
}

func (s descSet) encode(m msgDesc, vals map[string]any) []byte {
	fs := append([]fieldDesc(nil), m.Field...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].Number < fs[j].Number })
	var b []byte
	for _, fd := range fs {
		v, ok := vals[fd.Name]
		if !ok {
			continue
		}
		switch {
		case isVarint(fd.Type):
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|0)
			switch x := v.(type) {
			case bool:
				if x {
					b = binary.AppendUvarint(b, 1)
				} else {
					b = binary.AppendUvarint(b, 0)
				}
			case int:
				b = binary.AppendUvarint(b, uint64(int64(x)))
			}
		case fd.Type == "TYPE_STRING":
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
			str := v.(string)
			b = binary.AppendUvarint(b, uint64(len(str)))
			b = append(b, str...)
		case fd.Type == "TYPE_MESSAGE":
			sub, _ := s.byFullName(fd.TypeName)
			raw := s.encode(sub, v.(map[string]any))
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
			b = binary.AppendUvarint(b, uint64(len(raw)))
			b = append(b, raw...)
		}
	}
	return b
}

// decode parses b with the supplied field set; unknown fields are skipped.
// Absent scalars read as zero values, absent messages as nil. Repeated fields
// are skipped (the lifecycle checks do not need them).
func (s descSet) decode(t *testing.T, fields []fieldDesc, b []byte) map[string]any {
	t.Helper()
	byNum := map[int]fieldDesc{}
	out := map[string]any{}
	for _, fd := range fields {
		byNum[fd.Number] = fd
		switch {
		case fd.Label == "LABEL_REPEATED":
		case fd.Type == "TYPE_BOOL":
			out[fd.Name] = false
		case isVarint(fd.Type):
			out[fd.Name] = 0
		case fd.Type == "TYPE_STRING":
			out[fd.Name] = ""
		case fd.Type == "TYPE_MESSAGE":
			out[fd.Name] = nil
		}
	}
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			t.Fatalf("bad tag")
		}
		b = b[n:]
		num, wt := int(tag>>3), int(tag&7)
		fd, known := byNum[num]
		switch wt {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				t.Fatalf("bad varint")
			}
			b = b[n:]
			if known && fd.Label != "LABEL_REPEATED" {
				if fd.Type == "TYPE_BOOL" {
					out[fd.Name] = v != 0
				} else {
					out[fd.Name] = int(int64(v))
				}
			}
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || int(l) > len(b)-n {
				t.Fatalf("bad length")
			}
			raw := b[n : n+int(l)]
			b = b[n+int(l):]
			if known && fd.Label != "LABEL_REPEATED" {
				if fd.Type == "TYPE_STRING" {
					out[fd.Name] = string(raw)
				} else if fd.Type == "TYPE_MESSAGE" {
					sub, ok := s.byFullName(fd.TypeName)
					if !ok {
						t.Fatalf("unknown message %s", fd.TypeName)
					}
					out[fd.Name] = s.decode(t, sub.Field, raw)
				}
			}
		case 1:
			b = b[8:]
		case 5:
			b = b[4:]
		default:
			t.Fatalf("unsupported wire type %d", wt)
		}
	}
	return out
}

func upTo(fs []fieldDesc, max int) []fieldDesc {
	var o []fieldDesc
	for _, fd := range fs {
		if fd.Number <= max {
			o = append(o, fd)
		}
	}
	return o
}

func stamp(sec int) map[string]any { return map[string]any{"seconds": sec, "nanos": 0} }

// roundTrip encodes in with m, decodes it with all of m's fields and returns
// the decoded map. It first requires every key of in to be a field of m, so a
// missing field fails loudly instead of being silently dropped by the codec.
func roundTrip(t *testing.T, s descSet, m msgDesc, in map[string]any) map[string]any {
	t.Helper()
	for k := range in {
		m.field(t, k)
	}
	return s.decode(t, m.Field, s.encode(m, in))
}

func assertEqual(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("field %s = %v, want %v", k, got[k], v)
		}
	}
}

// ---- RPC list and request/response shapes ----------------------------------

var existingOrderRPCs = []string{
	"CreateOrder", "GetOrder", "ListBuyerOrders", "ListSellerOrders", "UpdateOrderStatus",
	"CancelOrder", "CalculateShippingFee", "GetSagaState", "ForceFailSaga",
	"CreateReturnRequest", "GetReturnRequest", "UpdateReturnStatus", "ListOrderReturns",
	"CreateShipment", "GetShipmentTracking",
}

func TestLifecycleContract_ConfirmReceiptRPCExists(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	svc, _ := f.service(t, "OrderService")
	got := map[string]methodDesc{}
	for _, m := range svc.Method {
		got[m.Name] = m
	}
	for _, name := range existingOrderRPCs {
		m, ok := got[name]
		if !ok {
			t.Errorf("existing RPC %s missing", name)
			continue
		}
		if m.InputType != orderPkg+name+"Request" || m.OutputType != orderPkg+name+"Response" {
			t.Errorf("RPC %s = %s -> %s, want unchanged request/response", name, m.InputType, m.OutputType)
		}
	}
	cr, ok := got["ConfirmReceipt"]
	if !ok {
		t.Fatalf("OrderService.ConfirmReceipt missing")
	}
	if cr.InputType != orderPkg+"ConfirmReceiptRequest" || cr.OutputType != orderPkg+"ConfirmReceiptResponse" {
		t.Errorf("ConfirmReceipt = %s -> %s", cr.InputType, cr.OutputType)
	}
	req, _ := f.message(t, "ConfirmReceiptRequest")
	requireFields(t, req, []wf{str("id", 1)}, 0)
	resp, _ := f.message(t, "ConfirmReceiptResponse")
	requireFields(t, resp, []wf{msg("order", 1, orderT)}, 0)
}

// ---- enums -----------------------------------------------------------------

func enumPairs(e enumDesc) []string {
	var o []string
	for _, v := range e.Value {
		o = append(o, fmt.Sprintf("%s=%d", v.Name, v.Number))
	}
	sort.Strings(o)
	return o
}

func TestLifecycleContract_EnumsHaveFixedNamesAndNumbers(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	cases := map[string][]string{
		"CancelReason": {
			"CANCEL_REASON_UNSPECIFIED=0", "CANCEL_REASON_CHANGED_MIND=1", "CANCEL_REASON_WRONG_ITEM=2",
			"CANCEL_REASON_DAMAGED=3", "CANCEL_REASON_DELIVERY_DELAY=4", "CANCEL_REASON_OTHER=5",
			"CANCEL_REASON_PAYMENT_EXPIRED=6", "CANCEL_REASON_ADMIN_FORCE_FAIL=7",
		},
		"CompletedBy": {
			"COMPLETED_BY_UNSPECIFIED=0", "COMPLETED_BY_BUYER=1", "COMPLETED_BY_SELLER=2", "COMPLETED_BY_SYSTEM=3",
		},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			e, _ := f.enum(t, name)
			sort.Strings(want)
			if got := enumPairs(e); !reflect.DeepEqual(got, want) {
				t.Errorf("%s = %v, want exactly %v", name, got, want)
			}
		})
	}
}

func TestLifecycleContract_CancelOrderRequestCarriesAReasonCode(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	m, _ := f.message(t, "CancelOrderRequest")
	requireFields(t, m, []wf{str("id", 1), str("reason", 2), enm("reason_code", 3, cancelReasonT)}, 0)
}

// ---- Order, OrderReturn and event field additions --------------------------

func TestLifecycleContract_OrderLifecycleFieldsHaveFixedNumbers(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	m, _ := f.message(t, "Order")
	// Only 1-24 are pinned; numbers 25+ belong to q11-checkout-coherence.
	want := append(append([]wf{}, orderExisting...), orderLifecycle...)
	requireFields(t, m, want, 24)
}

func TestLifecycleContract_ReturnAndEventAdditionsAreFixed(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	ret, _ := f.message(t, "OrderReturn")
	requireFields(t, ret, []wf{
		str("id", 1), str("order_id", 2), str("buyer_id", 3), str("seller_id", 4), str("reason", 5),
		i64("refund_amount", 6), enm("status", 7, orderPkg+"ReturnStatus"),
		ts("created_at", 8), ts("updated_at", 9), boolean("can_escalate", 10),
	}, 0)
	can, _ := f.message(t, "OrderCancelled")
	requireFields(t, can, []wf{
		str("order_id", 1), str("buyer_id", 2), str("seller_id", 3),
		enm("previous_status", 4, orderPkg+"OrderStatus"), i64("total_amount", 5), str("currency", 6),
		ts("cancelled_at", 7), enm("reason_code", 8, cancelReasonT),
	}, 0)
	paid, _ := f.message(t, "OrderPaidEvent")
	requireFields(t, paid, []wf{
		str("order_id", 1), str("buyer_id", 2),
		{"items", 3, "TYPE_MESSAGE", orderPkg + "OrderLineItemFact"},
		i64("total_amount", 4), str("currency", 5), ts("paid_at", 6),
		str("seller_id", 7), ts("ship_by", 8),
	}, 0)
}

func TestLifecycleContract_NewEventMessagesHaveFixedFields(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	cases := []struct {
		name string
		want []wf
	}{
		{"OrderPlaced", orderPlacedF},
		{"OrderPaymentReminder", orderReminderF},
		{"OrderCompleted", orderCompletedF},
		{"ReturnRequested", returnRequestedF},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := f.message(t, c.name)
			requireFields(t, m, c.want, 0)
		})
	}
}

// ---- wire behaviour --------------------------------------------------------

func orderBase() map[string]any {
	return map[string]any{
		"id": "O-1", "buyer_id": "B-1", "seller_id": "S-1", "status": 3, "total_amount": 1999000,
		"currency": "VND", "tracking_number": "TRK-1", "created_at": stamp(1760000000),
		"updated_at": stamp(1760000100), "shipping_fee": 30000, "payment_method": 2,
		"items_subtotal": 1969000, "discount_amount": 0, "voucher_code": "V-1", "paid_at": stamp(1760000050),
	}
}

func orderWithLifecycle() map[string]any {
	in := orderBase()
	in["payment_due_at"] = stamp(1760659200)   // 2026-10-17T00:00:00Z
	in["cancel_reason"] = 6                    // CANCEL_REASON_PAYMENT_EXPIRED
	in["auto_complete_at"] = stamp(1761264000) // 2026-10-24T00:00:00Z
	in["ship_by"] = stamp(1760200000)
	in["shipped_at"] = stamp(1760300000)
	in["completed_at"] = stamp(1760400000)
	in["return_window_ends_at"] = stamp(1761700000)
	return in
}

func TestLifecycleContract_OrderWithLifecycleFieldsRoundTrips(t *testing.T) {
	s := loadSet(t)
	m, _ := s.file(t, orderFile).message(t, "Order")
	in := orderWithLifecycle()
	got := roundTrip(t, s, m, in)
	assertEqual(t, got, in)

	t.Run("without lifecycle fields all seven are unset", func(t *testing.T) {
		for _, w := range orderLifecycle {
			m.field(t, w.name)
		}
		got := roundTrip(t, s, m, orderBase())
		assertEqual(t, got, orderBase())
		for _, w := range orderLifecycle {
			switch w.typ {
			case "TYPE_MESSAGE":
				if got[w.name] != nil {
					t.Errorf("%s = %v, want unset", w.name, got[w.name])
				}
			default:
				if got[w.name] != 0 {
					t.Errorf("%s = %v, want 0", w.name, got[w.name])
				}
			}
		}
	})
}

func TestLifecycleContract_OldReadersSkipTheNewOrderFields(t *testing.T) {
	s := loadSet(t)
	m, _ := s.file(t, orderFile).message(t, "Order")
	in := orderWithLifecycle()
	for k := range in {
		m.field(t, k) // fail loudly if a lifecycle field is absent
	}
	bytesNew := s.encode(m, in)
	got := s.decode(t, upTo(m.Field, 17), bytesNew)
	assertEqual(t, got, orderBase())
	for _, w := range orderLifecycle {
		if _, ok := got[w.name]; ok {
			t.Errorf("old schema decoded %s", w.name)
		}
	}
}

func TestLifecycleContract_UnknownCancelReasonValueParses(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	m, _ := f.message(t, "Order")
	m.field(t, "cancel_reason")
	e, _ := f.enum(t, "CancelReason")
	for _, v := range e.Value {
		if v.Number == 99 {
			t.Fatalf("value 99 must stay undefined")
		}
	}
	in := orderBase()
	in["cancel_reason"] = 99
	got := roundTrip(t, s, m, in)
	if got["cancel_reason"] != 99 {
		t.Errorf("cancel_reason = %v, want 99 preserved", got["cancel_reason"])
	}
	if got["id"] != "O-1" {
		t.Errorf("id = %v, want O-1", got["id"])
	}
}

func TestLifecycleContract_CompletedEventRoundTrips(t *testing.T) {
	s := loadSet(t)
	m, _ := s.file(t, orderFile).message(t, "OrderCompleted")
	in := map[string]any{
		"order_id": "O-1", "buyer_id": "B-1", "seller_id": "S-1",
		"total_amount": math.MaxInt64, "currency": "VND", "completed_by": 3, // COMPLETED_BY_SYSTEM
		"completed_at": stamp(1760400000), "return_window_ends_at": stamp(1761700000), "paid_online": true,
	}
	got := roundTrip(t, s, m, in)
	assertEqual(t, got, in)
	if got["total_amount"] != math.MaxInt64 {
		t.Errorf("total_amount = %v, want 9223372036854775807", got["total_amount"])
	}
}

func TestLifecycleContract_CODPlacedEventRoundTripsWithoutADeadline(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	m, _ := f.message(t, "OrderPlaced")
	in := map[string]any{
		"order_id": "O-1", "buyer_id": "B-1", "seller_id": "S-1",
		"payment_method": 1, // PAYMENT_METHOD_COD
		"total_amount":   250000, "currency": "VND", "placed_at": stamp(1760000000),
		"ship_by": stamp(1760172800),
	}
	got := roundTrip(t, s, m, in)
	assertEqual(t, got, in)
	if got["payment_due_at"] != nil {
		t.Errorf("payment_due_at = %v, want unset for COD", got["payment_due_at"])
	}
}

func TestLifecycleContract_ReminderAndReturnEventsRoundTrip(t *testing.T) {
	s := loadSet(t)
	f := s.file(t, orderFile)
	rem, _ := f.message(t, "OrderPaymentReminder")
	inR := map[string]any{
		"order_id": "O-1", "buyer_id": "B-1", "total_amount": 1999000, "currency": "VND",
		"payment_due_at": stamp(1760659200),
	}
	assertEqual(t, roundTrip(t, s, rem, inR), inR)

	ret, _ := f.message(t, "ReturnRequested")
	inQ := map[string]any{
		"return_id": "R-1", "order_id": "O-1", "buyer_id": "B-1", "seller_id": "S-1",
		"refund_amount": 500000, "currency": "VND", "requested_at": stamp(1760500000),
	}
	assertEqual(t, roundTrip(t, s, ret, inQ), inQ)
}

// ---- PAYMENT_STATUS_CANCELLED ----------------------------------------------

func TestLifecycleContract_PaymentStatusCancelledExists(t *testing.T) {
	f := loadSet(t).file(t, paymentFile)
	e, ei := f.enum(t, "PaymentStatus")
	want := []string{
		"PAYMENT_STATUS_UNSPECIFIED=0", "PAYMENT_STATUS_PENDING=1", "PAYMENT_STATUS_PAID=2",
		"PAYMENT_STATUS_FAILED=3", "PAYMENT_STATUS_REFUNDED=4", "PAYMENT_STATUS_PARTIALLY_REFUNDED=5",
	}
	got := enumPairs(e)
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			t.Errorf("existing value %s changed or missing; got %v", w, got)
		}
	}
	n, ok := e.number("PAYMENT_STATUS_CANCELLED")
	if !ok || n != 6 {
		t.Fatalf("PAYMENT_STATUS_CANCELLED = %d (present=%v), want 6", n, ok)
	}
	vi := -1
	for i, v := range e.Value {
		if v.Name == "PAYMENT_STATUS_CANCELLED" {
			vi = i
		}
	}
	cm, has := f.comment(5, ei, 2, vi)
	if err := checkKeys(cm, has, "ProcessMockPayment refuses it"); err != nil {
		t.Error(err)
	}
}

// ---- comment text ----------------------------------------------------------

// checkKeys returns an error naming every substring missing from the comment.
func checkKeys(comment string, ok bool, subs ...string) error {
	if !ok {
		return fmt.Errorf("no comment found on element")
	}
	var missing []string
	for _, s := range subs {
		if !strings.Contains(comment, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("comment missing %q; got:\n%s", missing, comment)
	}
	return nil
}

func msgFieldComment(t *testing.T, f fileDesc, msgName, field string) (string, bool) {
	t.Helper()
	m, mi := f.message(t, msgName)
	_, fi := m.field(t, field)
	return f.comment(4, mi, 2, fi)
}

func msgComment(t *testing.T, f fileDesc, msgName string) (string, bool) {
	t.Helper()
	_, mi := f.message(t, msgName)
	return f.comment(4, mi)
}

func enumValueComment(t *testing.T, f fileDesc, enumName, value string) (string, bool) {
	t.Helper()
	e, ei := f.enum(t, enumName)
	for vi, v := range e.Value {
		if v.Name == value {
			return f.comment(5, ei, 2, vi)
		}
	}
	t.Fatalf("%s missing from %s", value, enumName)
	return "", false
}

func rpcComment(t *testing.T, f fileDesc, svc, rpc string) (string, bool) {
	t.Helper()
	s, si := f.service(t, svc)
	for mi, m := range s.Method {
		if m.Name == rpc {
			return f.comment(6, si, 2, mi)
		}
	}
	t.Fatalf("rpc %s.%s missing", svc, rpc)
	return "", false
}

func TestLifecycleContract_ConfirmReceiptAuthzAndPreconditionsAreDocumented(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	cm, ok := rpcComment(t, f, "OrderService", "ConfirmReceipt")
	if err := checkKeys(cm, ok, "buyer of the order only", "SHIPPED", "COMPLETED",
		"return is PENDING or APPROVED", "PERMISSION_DENIED", "FAILED_PRECONDITION"); err != nil {
		t.Error(err)
	}
}

func TestLifecycleContract_SystemOnlyCancelReasonsAreDocumentedAsRefusedForBuyers(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	cm, ok := msgFieldComment(t, f, "CancelOrderRequest", "reason_code")
	if err := checkKeys(cm, ok, "PAYMENT_EXPIRED", "ADMIN_FORCE_FAIL", "PERMISSION_DENIED"); err != nil {
		t.Errorf("reason_code: %v", err)
	}
	cm, ok = enumValueComment(t, f, "CancelReason", "CANCEL_REASON_PAYMENT_EXPIRED")
	if err := checkKeys(cm, ok, "system only", "PERMISSION_DENIED"); err != nil {
		t.Errorf("PAYMENT_EXPIRED: %v", err)
	}
	cm, ok = enumValueComment(t, f, "CancelReason", "CANCEL_REASON_ADMIN_FORCE_FAIL")
	if err := checkKeys(cm, ok, "ForceFailSaga only", "PERMISSION_DENIED"); err != nil {
		t.Errorf("ADMIN_FORCE_FAIL: %v", err)
	}
}

func TestLifecycleContract_MissingAuthzNoteIsCaught(t *testing.T) {
	fixture := "Buyer of the order only. SHIPPED to COMPLETED; FAILED_PRECONDITION otherwise."
	if err := checkKeys(fixture, true, "PERMISSION_DENIED"); err == nil {
		t.Fatal("helper accepted a ConfirmReceipt comment lacking PERMISSION_DENIED")
	}
	if err := checkKeys("", false, "PERMISSION_DENIED"); err == nil {
		t.Fatal("helper accepted a missing comment")
	}
	if err := checkKeys("PERMISSION_DENIED for any other caller.", true, "PERMISSION_DENIED"); err != nil {
		t.Fatalf("helper rejected a comment containing PERMISSION_DENIED: %v", err)
	}
}

func TestLifecycleContract_DerivationRulesAreDocumented(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	cases := []struct {
		msg, field string
		keys       []string
	}{
		{"Order", "payment_due_at", []string{"created_at + 24h", "unset for COD", "before the deploy"}},
		{"Order", "ship_by", []string{"2 days"}},
		{"Order", "return_window_ends_at", []string{"15 days"}},
		{"Order", "auto_complete_at", []string{"7 days"}},
		{"OrderReturn", "can_escalate", []string{"server-computed", "REJECTED", "SHIPPED"}},
	}
	for _, c := range cases {
		t.Run(c.msg+"."+c.field, func(t *testing.T) {
			cm, ok := msgFieldComment(t, f, c.msg, c.field)
			if err := checkKeys(cm, ok, c.keys...); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestLifecycleContract_EventSemanticsAreDocumented(t *testing.T) {
	f := loadSet(t).file(t, orderFile)
	rule := []string{"outbox", "order.events", "EventEnvelope.type"}
	msgs := []struct {
		name string
		keys []string
	}{
		{"OrderPlaced", append([]string{"every placed order"}, rule...)},
		{"OrderPaymentReminder", append([]string{"once per order"}, rule...)},
		{"OrderCompleted", append([]string{"once per order"}, rule...)},
		{"ReturnRequested", append([]string{"once per created return"}, rule...)},
	}
	for _, c := range msgs {
		t.Run(c.name, func(t *testing.T) {
			cm, ok := msgComment(t, f, c.name)
			if err := checkKeys(cm, ok, c.keys...); err != nil {
				t.Error(err)
			}
		})
	}
	t.Run("OrderPlaced.payment_due_at", func(t *testing.T) {
		cm, ok := msgFieldComment(t, f, "OrderPlaced", "payment_due_at")
		if err := checkKeys(cm, ok, "unset for COD"); err != nil {
			t.Error(err)
		}
	})
	t.Run("OrderPlaced.ship_by", func(t *testing.T) {
		cm, ok := msgFieldComment(t, f, "OrderPlaced", "ship_by")
		if err := checkKeys(cm, ok, "COD only"); err != nil {
			t.Error(err)
		}
	})
}

// ---- wire-breaking edits are caught ----------------------------------------

func TestLifecycleContract_WireBreakingEditIsRejected(t *testing.T) {
	// A renumbered, retyped or removed new field must make the pin helper fail.
	base := msgDesc{Name: "OrderCompleted"}
	for _, w := range orderCompletedF {
		base.Field = append(base.Field, fieldDesc{Name: w.name, Number: w.num, Type: w.typ, TypeName: w.typeName})
	}
	if d := diffFields(base, orderCompletedF, 0); len(d) != 0 {
		t.Fatalf("pin helper flags an exact match: %v", d)
	}
	renumbered := msgDesc{Name: base.Name, Field: append([]fieldDesc(nil), base.Field...)}
	renumbered.Field[5].Number = 16
	if d := diffFields(renumbered, orderCompletedF, 0); len(d) == 0 {
		t.Error("helper accepted a renumbered field")
	}
	retyped := msgDesc{Name: base.Name, Field: append([]fieldDesc(nil), base.Field...)}
	retyped.Field[3].Type = "TYPE_INT32"
	if d := diffFields(retyped, orderCompletedF, 0); len(d) == 0 {
		t.Error("helper accepted a retyped field")
	}
	removed := msgDesc{Name: base.Name, Field: base.Field[:len(base.Field)-1]}
	if d := diffFields(removed, orderCompletedF, 0); len(d) == 0 {
		t.Error("helper accepted a removed field")
	}
}
