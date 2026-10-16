package promotionv1_test

// Tests for change q5-flash-stock-contract (capability flash-stock-contract).
// Stdlib only: the schema is read from `buf build -o -#format=json` (a
// FileDescriptorSet as JSON); a tiny schema-driven proto3 wire codec proves
// round-trip and old-reader compatibility without generated code.
// Comment text is whitespace-normalised so proto line wrapping is irrelevant.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
)

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
	Name           string    `json:"name"`
	Package        string    `json:"package"`
	MessageType    []msgDesc `json:"messageType"`
	Service        []svcDesc `json:"service"`
	SourceCodeInfo struct {
		Location []location `json:"location"`
	} `json:"sourceCodeInfo"`
}

const promoFile = "platform/promotion/v1/promotion.proto"
const pkgPrefix = ".platform.promotion.v1."

func loadFile(t *testing.T) fileDesc {
	t.Helper()
	cmd := exec.Command("buf", "build", "--path", promoFile, "-o", "-#format=json")
	cmd.Dir = "../../.."
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed (harness): %v\n%s", err, errb.String())
	}
	var set struct {
		File []fileDesc `json:"file"`
	}
	if err := json.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatalf("decode descriptor json: %v", err)
	}
	for _, f := range set.File {
		if f.Name == promoFile {
			return f
		}
	}
	t.Fatalf("%s not in descriptor set", promoFile)
	return fileDesc{}
}

func (f fileDesc) message(name string) (msgDesc, int, bool) {
	for i, m := range f.MessageType {
		if m.Name == name {
			return m, i, true
		}
	}
	return msgDesc{}, -1, false
}

func (f fileDesc) mustMessage(t *testing.T, name string) (msgDesc, int) {
	t.Helper()
	m, i, ok := f.message(name)
	if !ok {
		t.Fatalf("message %s%s missing from descriptor", pkgPrefix, name)
	}
	return m, i
}

func (m msgDesc) fieldIndex(name string) (fieldDesc, int, bool) {
	for i, fd := range m.Field {
		if fd.Name == name {
			return fd, i, true
		}
	}
	return fieldDesc{}, -1, false
}

func (f fileDesc) service(name string) (svcDesc, int, bool) {
	for i, s := range f.Service {
		if s.Name == name {
			return s, i, true
		}
	}
	return svcDesc{}, -1, false
}

// comment returns leading+trailing comment for an exact source path.
func (f fileDesc) comment(path ...int) (string, bool) {
	for _, l := range f.SourceCodeInfo.Location {
		if reflect.DeepEqual(l.Path, path) {
			return l.LeadingComments + l.TrailingComment, true
		}
	}
	return "", false
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// checkKeys returns an error naming every substring missing from the comment.
func checkKeys(comment string, ok bool, subs ...string) error {
	if !ok {
		return fmt.Errorf("no comment found on element")
	}
	c := norm(comment)
	var missing []string
	for _, s := range subs {
		if !strings.Contains(c, norm(s)) {
			missing = append(missing, fmt.Sprintf("%q", s))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("comment missing %s; got:\n%s", strings.Join(missing, ", "), c)
	}
	return nil
}

func (f fileDesc) rpcComment(t *testing.T, rpc string) (string, bool) {
	t.Helper()
	s, si, ok := f.service("FlashSaleService")
	if !ok {
		t.Fatalf("FlashSaleService missing")
	}
	for mi, m := range s.Method {
		if m.Name == rpc {
			return f.comment(6, si, 2, mi)
		}
	}
	t.Errorf("rpc FlashSaleService.%s missing", rpc)
	return "", false
}

func (f fileDesc) fieldComment(t *testing.T, msg, field string) (string, bool) {
	t.Helper()
	m, mi := f.mustMessage(t, msg)
	_, fi, ok := m.fieldIndex(field)
	if !ok {
		t.Errorf("field %s.%s missing", msg, field)
		return "", false
	}
	return f.comment(4, mi, 2, fi)
}

// ---- minimal schema-driven proto3 wire codec -------------------------------

func isVarint(t string) bool {
	switch t {
	case "TYPE_BOOL", "TYPE_INT32", "TYPE_INT64", "TYPE_UINT32", "TYPE_UINT64", "TYPE_ENUM":
		return true
	}
	return false
}

func encode(m msgDesc, vals map[string]any) []byte {
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
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3)
			switch x := v.(type) {
			case bool:
				if x {
					b = binary.AppendUvarint(b, 1)
				} else {
					b = binary.AppendUvarint(b, 0)
				}
			case int64:
				b = binary.AppendUvarint(b, uint64(x))
			}
		case fd.Type == "TYPE_STRING":
			s := v.(string)
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
			b = binary.AppendUvarint(b, uint64(len(s)))
			b = append(b, s...)
		}
	}
	return b
}

// decode parses b with the supplied field set; unknown fields are skipped.
func decode(t *testing.T, fields []fieldDesc, b []byte) map[string]any {
	t.Helper()
	byNum := map[int]fieldDesc{}
	out := map[string]any{}
	for _, fd := range fields {
		byNum[fd.Number] = fd
		switch {
		case fd.Type == "TYPE_BOOL":
			out[fd.Name] = false
		case isVarint(fd.Type):
			out[fd.Name] = int64(0)
		case fd.Type == "TYPE_STRING":
			out[fd.Name] = ""
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
			if known {
				if fd.Type == "TYPE_BOOL" {
					out[fd.Name] = v != 0
				} else {
					out[fd.Name] = int64(v)
				}
			}
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || int(l) > len(b)-n {
				t.Fatalf("bad length")
			}
			raw := b[n : n+int(l)]
			b = b[n+int(l):]
			if known && fd.Type == "TYPE_STRING" {
				out[fd.Name] = string(raw)
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

// ---- descriptor shape ------------------------------------------------------

var holdRPCs = map[string][2]string{
	"ReserveFlashSaleStock": {"ReserveFlashSaleStockRequest", "ReserveFlashSaleStockResponse"},
	"CommitFlashSaleStock":  {"CommitFlashSaleStockRequest", "CommitFlashSaleStockResponse"},
	"ReleaseFlashSaleStock": {"ReleaseFlashSaleStockRequest", "ReleaseFlashSaleStockResponse"},
}

var existingRPCs = map[string][2]string{
	"CreateCampaign":      {"CreateCampaignRequest", "CreateCampaignResponse"},
	"GetActiveFlashSale":  {"GetActiveFlashSaleRequest", "GetActiveFlashSaleResponse"},
	"ListActiveCampaigns": {"ListActiveCampaignsRequest", "ListActiveCampaignsResponse"},
	"GetFlashSaleStock":   {"GetFlashSaleStockRequest", "GetFlashSaleStockResponse"},
}

// Scenario: Flash stock contract: Hold RPCs exist on FlashSaleService
func TestFlashStock_HoldRPCsExistOnFlashSaleService(t *testing.T) {
	f := loadFile(t)
	s, _, ok := f.service("FlashSaleService")
	if !ok {
		t.Fatal("FlashSaleService missing")
	}
	got := map[string][2]string{}
	for _, m := range s.Method {
		got[m.Name] = [2]string{strings.TrimPrefix(m.InputType, pkgPrefix), strings.TrimPrefix(m.OutputType, pkgPrefix)}
	}
	want := map[string][2]string{}
	for k, v := range existingRPCs {
		want[k] = v
	}
	for k, v := range holdRPCs {
		want[k] = v
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FlashSaleService methods = %v, want exactly %v", got, want)
	}
}

func fields(m msgDesc) map[string]string {
	got := map[string]string{}
	for _, fd := range m.Field {
		got[fd.Name] = fmt.Sprintf("%d %s", fd.Number, fd.Type)
	}
	return got
}

// Scenario: Flash stock contract: Message fields have fixed names and numbers
// (also: Flash stock contract: Wire-breaking edit is rejected, descriptor half)
func TestFlashStock_MessageFieldsHaveFixedNamesAndNumbers(t *testing.T) {
	f := loadFile(t)
	want := map[string]map[string]string{
		"ReserveFlashSaleStockRequest": {
			"reservation_id": "1 TYPE_STRING", "campaign_id": "2 TYPE_STRING", "listing_id": "3 TYPE_STRING",
			"variant_id": "4 TYPE_STRING", "quantity": "5 TYPE_INT64", "expected_sale_price": "6 TYPE_INT64",
		},
		"ReserveFlashSaleStockResponse": {"reserved": "1 TYPE_BOOL", "reason": "2 TYPE_STRING"},
		"CommitFlashSaleStockRequest":   {"reservation_id": "1 TYPE_STRING"},
		"CommitFlashSaleStockResponse":  {"committed": "1 TYPE_BOOL"},
		"ReleaseFlashSaleStockRequest":  {"reservation_id": "1 TYPE_STRING"},
		"ReleaseFlashSaleStockResponse": {"released": "1 TYPE_BOOL"},
	}
	for name, w := range want {
		m, _, ok := f.message(name)
		if !ok {
			t.Errorf("message %s missing", name)
			continue
		}
		if g := fields(m); !reflect.DeepEqual(g, w) {
			t.Errorf("%s fields = %v, want exactly %v", name, g, w)
		}
	}
}

// Scenario: Flash stock contract: Existing messages are untouched
func TestFlashStock_ExistingMessagesAreUntouched(t *testing.T) {
	f := loadFile(t)
	want := map[string]map[string]string{
		"FlashSaleCampaign": {
			"id": "1 TYPE_STRING", "listing_id": "2 TYPE_STRING", "variant_id": "3 TYPE_STRING",
			"sale_price": "4 TYPE_INT64", "stock_cap": "5 TYPE_INT64", "stock_sold": "6 TYPE_INT64",
			"starts_at": "7 TYPE_MESSAGE", "ends_at": "8 TYPE_MESSAGE",
		},
		"GetActiveFlashSaleRequest":  {"listing_id": "1 TYPE_STRING"},
		"GetActiveFlashSaleResponse": {"active": "1 TYPE_BOOL", "campaign": "2 TYPE_MESSAGE"},
		"GetFlashSaleStockRequest":   {"campaign_id": "1 TYPE_STRING"},
		"GetFlashSaleStockResponse":  {"remaining": "1 TYPE_INT64", "stock_cap": "2 TYPE_INT64"},
	}
	for name, w := range want {
		m, _, ok := f.message(name)
		if !ok {
			t.Errorf("message %s missing", name)
			continue
		}
		if g := fields(m); !reflect.DeepEqual(g, w) {
			t.Errorf("%s fields = %v, want %v", name, g, w)
		}
	}
}

// ---- wire behaviour --------------------------------------------------------

func reserveReq(t *testing.T) (fileDesc, msgDesc) {
	t.Helper()
	f := loadFile(t)
	m, _ := f.mustMessage(t, "ReserveFlashSaleStockRequest")
	for _, n := range []string{"reservation_id", "campaign_id", "listing_id", "variant_id", "quantity", "expected_sale_price"} {
		if _, _, ok := m.fieldIndex(n); !ok {
			t.Fatalf("ReserveFlashSaleStockRequest.%s missing", n)
		}
	}
	return f, m
}

// Scenario: Flash stock contract: Hold messages round-trip
func TestFlashStock_HoldMessagesRoundTrip(t *testing.T) {
	f, req := reserveReq(t)
	in := map[string]any{
		"reservation_id": "ord-1:lst-1:", "campaign_id": "c-1", "listing_id": "lst-1",
		"variant_id": "", "quantity": int64(2), "expected_sale_price": int64(199000),
	}
	got := decode(t, req.Field, encode(req, in))
	if !reflect.DeepEqual(got, in) {
		t.Errorf("request round-trip = %v, want %v", got, in)
	}
	resp, _ := f.mustMessage(t, "ReserveFlashSaleStockResponse")
	rin := map[string]any{"reserved": false, "reason": "insufficient_stock"}
	rgot := decode(t, resp.Field, encode(resp, rin))
	if !reflect.DeepEqual(rgot, rin) {
		t.Errorf("response round-trip = %v, want %v", rgot, rin)
	}
}

// Scenario: Flash stock contract: Maximum quantity and price round-trip
func TestFlashStock_MaximumQuantityAndPriceRoundTrip(t *testing.T) {
	_, req := reserveReq(t)
	const max = int64(9223372036854775807)
	in := map[string]any{"reservation_id": "o:l:", "quantity": max, "expected_sale_price": max}
	got := decode(t, req.Field, encode(req, in))
	if got["quantity"] != max || got["expected_sale_price"] != max {
		t.Errorf("max int64 round-trip = %v / %v, want %d", got["quantity"], got["expected_sale_price"], max)
	}
}

// Scenario: Flash stock contract: Old readers skip the new request fields
func TestFlashStock_OldReadersSkipNewRequestFields(t *testing.T) {
	_, req := reserveReq(t)
	in := map[string]any{
		"reservation_id": "ord-1:lst-1:v", "campaign_id": "c-1", "listing_id": "lst-1",
		"variant_id": "v", "quantity": int64(2), "expected_sale_price": int64(199000),
	}
	got := decode(t, upTo(req.Field, 3), encode(req, in))
	want := map[string]any{"reservation_id": "ord-1:lst-1:v", "campaign_id": "c-1", "listing_id": "lst-1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("old-schema decode = %v, want %v", got, want)
	}
}

// ---- comment text ----------------------------------------------------------

// every hold RPC must carry the service-only and permission notes.
var serviceOnlyKeys = []string{"SERVICE-ONLY"}
var deniedKeys = []string{"promotion.reserve", "PERMISSION_DENIED", "UNAUTHENTICATED", "gateway"}

// Scenario: Flash stock contract: Service-only is documented
func TestFlashStock_ServiceOnlyIsDocumented(t *testing.T) {
	f := loadFile(t)
	for rpc := range holdRPCs {
		c, ok := f.rpcComment(t, rpc)
		if err := checkKeys(c, ok, serviceOnlyKeys...); err != nil {
			t.Errorf("%s: %v", rpc, err)
		}
	}
	c, ok := f.rpcComment(t, "ReserveFlashSaleStock")
	if err := checkKeys(c, ok, "Idempotent on reservation_id"); err != nil {
		t.Errorf("Reserve: %v", err)
	}
	c, ok = f.rpcComment(t, "ReleaseFlashSaleStock")
	if err := checkKeys(c, ok, "returns committed units", "stock_sold -= quantity", "unknown", "released=false"); err != nil {
		t.Errorf("Release: %v", err)
	}
	c, ok = f.rpcComment(t, "CommitFlashSaleStock")
	if err := checkKeys(c, ok, "released or expired", "committed=false"); err != nil {
		t.Errorf("Commit: %v", err)
	}
}

// Scenario: Flash stock contract: Missing service-only note is caught
func TestFlashStock_MissingServiceOnlyNoteIsCaught(t *testing.T) {
	fixture := "Reserves stock for a flash sale line. Idempotent on reservation_id."
	if err := checkKeys(fixture, true, serviceOnlyKeys...); err == nil {
		t.Fatal("helper accepted a comment lacking SERVICE-ONLY")
	}
	if err := checkKeys("", false, serviceOnlyKeys...); err == nil {
		t.Fatal("helper accepted a missing comment")
	}
	if err := checkKeys("SERVICE-ONLY: x", true, serviceOnlyKeys...); err != nil {
		t.Fatalf("helper rejected a comment containing SERVICE-ONLY: %v", err)
	}
}

// Scenario: Flash stock contract: Non-service callers are documented as denied
func TestFlashStock_NonServiceCallersAreDocumentedAsDenied(t *testing.T) {
	f := loadFile(t)
	for rpc := range holdRPCs {
		c, ok := f.rpcComment(t, rpc)
		if err := checkKeys(c, ok, deniedKeys...); err != nil {
			t.Errorf("%s: %v", rpc, err)
		}
	}
}

// Scenario: Flash stock contract: Hold lifecycle is documented
func TestFlashStock_HoldLifecycleIsDocumented(t *testing.T) {
	f := loadFile(t)
	c, ok := f.rpcComment(t, "ReserveFlashSaleStock")
	if err := checkKeys(c, ok, "15 minutes"); err != nil {
		t.Errorf("Reserve: %v", err)
	}
	c, ok = f.rpcComment(t, "CommitFlashSaleStock")
	if err := checkKeys(c, ok, "Phase B", "committed=false", "released or expired"); err != nil {
		t.Errorf("Commit: %v", err)
	}
	c, ok = f.rpcComment(t, "ReleaseFlashSaleStock")
	if err := checkKeys(c, ok, "stock_sold -= quantity", "already released", "expired", "released=true", "changes no counter", "released=false"); err != nil {
		t.Errorf("Release: %v", err)
	}
}

// Scenario: Flash stock contract: Reserve replay after release
func TestFlashStock_ReserveReplayAfterRelease(t *testing.T) {
	f := loadFile(t)
	c, ok := f.rpcComment(t, "ReserveFlashSaleStock")
	err := checkKeys(c, ok,
		"A replay returns the stored outcome and ignores the other request fields.",
		"reserved=false", "reason=not_active", "released or expired", "fresh order ids")
	if err != nil {
		t.Error(err)
	}
}

// Scenario: Flash stock contract: Reasons are documented as a closed set
func TestFlashStock_ReasonsAreDocumentedAsClosedSet(t *testing.T) {
	f := loadFile(t)
	c, ok := f.rpcComment(t, "ReserveFlashSaleStock")
	err := checkKeys(c, ok, "not_active", "price_changed", "insufficient_stock",
		"expected_sale_price", "cap - sold - holds < quantity", "closed",
		"not_active, price_changed, insufficient_stock", "first failure")
	if err != nil {
		t.Errorf("Reserve rpc: %v", err)
	}
	c, ok = f.fieldComment(t, "ReserveFlashSaleStockResponse", "reason")
	if err := checkKeys(c, ok, "not_active", "price_changed", "insufficient_stock", "closed"); err != nil {
		t.Errorf("reason field: %v", err)
	}
}

// Scenario: Flash stock contract: Invalid input is not a reason
func TestFlashStock_InvalidInputIsNotAReason(t *testing.T) {
	f := loadFile(t)
	c, ok := f.rpcComment(t, "ReserveFlashSaleStock")
	err := checkKeys(c, ok, "INVALID_ARGUMENT", "reservation_id", "campaign_id", "listing_id",
		"200 bytes", "quantity <= 0", "empty variant_id is valid")
	if err != nil {
		t.Error(err)
	}
}

// Scenario: Flash stock contract: Reservation id shape is documented
func TestFlashStock_ReservationIdShapeIsDocumented(t *testing.T) {
	f := loadFile(t)
	c, ok := f.fieldComment(t, "ReserveFlashSaleStockRequest", "reservation_id")
	err := checkKeys(c, ok, "<order_id>:<listing_id>:<variant_id>", "<order_id>:<listing_id>:", "200 bytes")
	if err != nil {
		t.Error(err)
	}
}

// Scenario: Flash stock contract: Remaining meter definition is documented
func TestFlashStock_RemainingMeterDefinitionIsDocumented(t *testing.T) {
	f := loadFile(t)
	c, ok := f.fieldComment(t, "GetFlashSaleStockResponse", "remaining")
	err := checkKeys(c, ok, "stock_cap - stock_sold - active (unexpired, uncommitted) holds")
	if err != nil {
		t.Error(err)
	}
}
