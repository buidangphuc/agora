package analyticsv1_test

// Tests for change ga4-consent-contract (capability tracking-consent).
// Stdlib only: the schema is read from `buf build -o -#format=json` (a
// FileDescriptorSet as JSON) and a tiny schema-driven proto3 wire codec proves
// forward/backward compatibility without any generated code or extra module.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"sort"
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

const analyticsFile = "platform/analytics/v1/analytics.proto"

func loadFile(t *testing.T) fileDesc {
	t.Helper()
	cmd := exec.Command("buf", "build", "--path", analyticsFile, "-o", "-#format=json")
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
		if f.Name == analyticsFile {
			return f
		}
	}
	t.Fatalf("%s not in descriptor set", analyticsFile)
	return fileDesc{}
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

// comment returns leading+trailing comment for an exact source path.
func (f fileDesc) comment(path ...int) (string, bool) {
	for _, l := range f.SourceCodeInfo.Location {
		if reflect.DeepEqual(l.Path, path) {
			return l.LeadingComments + l.TrailingComment, true
		}
	}
	return "", false
}

// ---- minimal schema-driven proto3 wire codec -------------------------------

func isVarint(t string) bool {
	switch t {
	case "TYPE_BOOL", "TYPE_INT32", "TYPE_INT64", "TYPE_UINT32", "TYPE_UINT64", "TYPE_ENUM":
		return true
	}
	return false
}

func (f fileDesc) byName(name string) (msgDesc, bool) {
	for _, m := range f.MessageType {
		if "."+f.Package+"."+m.Name == name {
			return m, true
		}
	}
	return msgDesc{}, false
}

// encode writes values (keyed by field name) in ascending field-number order.
func (f fileDesc) encode(m msgDesc, vals map[string]any) []byte {
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
			s := v.(string)
			b = binary.AppendUvarint(b, uint64(len(s)))
			b = append(b, s...)
		case fd.Type == "TYPE_MESSAGE":
			sub, _ := f.byName(fd.TypeName)
			raw := f.encode(sub, v.(map[string]any))
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
			b = binary.AppendUvarint(b, uint64(len(raw)))
			b = append(b, raw...)
		}
	}
	return b
}

// decode parses b with the supplied field set; unknown fields are skipped
// (proto3 semantics). Absent scalars read as their zero value; absent
// message fields are nil. Repeated/map fields are skipped for this test.
func (f fileDesc) decode(t *testing.T, fields []fieldDesc, b []byte) map[string]any {
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
					sub, ok := f.byName(fd.TypeName)
					if !ok {
						t.Fatalf("unknown message %s", fd.TypeName)
					}
					out[fd.Name] = f.decode(t, sub.Field, raw)
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

// ---- descriptor shape ------------------------------------------------------

func TestConsent_ConsentStateFieldsPresentWithStableNumbers(t *testing.T) {
	f := loadFile(t)
	m, _ := f.message(t, "ConsentState")
	got := map[string]string{}
	for _, fd := range m.Field {
		got[fd.Name] = fmt.Sprintf("%d %s", fd.Number, fd.Type)
	}
	want := map[string]string{
		"analytics":       "1 TYPE_BOOL",
		"marketing":       "2 TYPE_BOOL",
		"personalization": "3 TYPE_BOOL",
		"version":         "4 TYPE_INT32",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConsentState fields = %v, want exactly %v", got, want)
	}
}

func TestConsent_TrackingEventFields25And26(t *testing.T) {
	f := loadFile(t)
	m, _ := f.message(t, "TrackingEvent")
	g, _ := m.field(t, "ga_client_id")
	if g.Number != 25 || g.Type != "TYPE_STRING" {
		t.Errorf("ga_client_id = %d %s, want 25 TYPE_STRING", g.Number, g.Type)
	}
	c, _ := m.field(t, "consent")
	if c.Number != 26 || c.Type != "TYPE_MESSAGE" || c.TypeName != ".platform.analytics.v1.ConsentState" {
		t.Errorf("consent = %d %s %s, want 26 TYPE_MESSAGE ConsentState", c.Number, c.Type, c.TypeName)
	}
}

func TestConsent_ChannelFunnelMessagesPresent(t *testing.T) {
	f := loadFile(t)
	req, _ := f.message(t, "GetChannelFunnelRequest")
	for _, n := range []string{"seller_id", "from", "to"} {
		req.field(t, n)
	}
	if fd, _ := req.field(t, "seller_id"); fd.Type != "TYPE_STRING" {
		t.Errorf("seller_id type %s", fd.Type)
	}
	row, _ := f.message(t, "ChannelFunnelRow")
	for _, n := range []string{"source", "medium", "campaign"} {
		if fd, _ := row.field(t, n); fd.Type != "TYPE_STRING" {
			t.Errorf("%s type %s, want string", n, fd.Type)
		}
	}
	for _, n := range []string{"impressions", "views", "adds", "begin_checkouts", "purchases"} {
		if fd, _ := row.field(t, n); fd.Type != "TYPE_INT64" {
			t.Errorf("%s type %s, want int64", n, fd.Type)
		}
	}
	resp, _ := f.message(t, "GetChannelFunnelResponse")
	rows, _ := resp.field(t, "rows")
	if rows.Label != "LABEL_REPEATED" || rows.TypeName != ".platform.analytics.v1.ChannelFunnelRow" {
		t.Errorf("rows = %s %s, want repeated ChannelFunnelRow", rows.Label, rows.TypeName)
	}
	var found bool
	for _, s := range f.Service {
		if s.Name != "AnalyticsQueryService" {
			continue
		}
		for _, mm := range s.Method {
			if mm.Name == "GetChannelFunnel" {
				found = true
				if mm.InputType != ".platform.analytics.v1.GetChannelFunnelRequest" || mm.OutputType != ".platform.analytics.v1.GetChannelFunnelResponse" {
					t.Errorf("GetChannelFunnel types = %s -> %s", mm.InputType, mm.OutputType)
				}
			}
		}
	}
	if !found {
		t.Error("AnalyticsQueryService.GetChannelFunnel missing")
	}
}

// ---- wire compatibility ----------------------------------------------------

func legacyEvent() map[string]any {
	return map[string]any{
		"event_type": 3, "listing_id": "L-1", "session_id": "S-1", "anonymous_id": "A-1",
		"page_path": "/p", "position": 7, "value": 1999, "currency": "USD",
		"transaction_id": "T-9", "payment_type": "card",
	}
}

func TestConsent_NewFieldsRoundTrip(t *testing.T) {
	f := loadFile(t)
	te, _ := f.message(t, "TrackingEvent")
	te.field(t, "ga_client_id")
	te.field(t, "consent")
	cs, _ := f.message(t, "ConsentState")
	cs.field(t, "version")
	in := legacyEvent()
	in["ga_client_id"] = "123.456"
	in["consent"] = map[string]any{"analytics": true, "marketing": false, "personalization": true, "version": 1}
	got := f.decode(t, te.Field, f.encode(te, in))
	if got["ga_client_id"] != "123.456" {
		t.Errorf("ga_client_id = %v", got["ga_client_id"])
	}
	want := map[string]any{"analytics": true, "marketing": false, "personalization": true, "version": 1}
	if !reflect.DeepEqual(got["consent"], want) {
		t.Errorf("consent = %v, want %v", got["consent"], want)
	}
	for k, v := range legacyEvent() {
		if got[k] != v {
			t.Errorf("field %s = %v, want %v", k, got[k], v)
		}
	}
}

func TestConsent_OldPayloadStaysReadable(t *testing.T) {
	f := loadFile(t)
	te, _ := f.message(t, "TrackingEvent")
	te.field(t, "ga_client_id")
	te.field(t, "consent")
	old := f.encode(msgDesc{Field: upTo(te.Field, 24)}, legacyEvent())
	got := f.decode(t, te.Field, old)
	if got["ga_client_id"] != "" {
		t.Errorf("ga_client_id = %q, want empty", got["ga_client_id"])
	}
	if got["consent"] != nil {
		t.Errorf("consent = %v, want unset", got["consent"])
	}
	for k, v := range legacyEvent() {
		if got[k] != v {
			t.Errorf("field %s = %v, want %v", k, got[k], v)
		}
	}
}

func TestConsent_NewPayloadReadableByOldSchema(t *testing.T) {
	f := loadFile(t)
	te, _ := f.message(t, "TrackingEvent")
	te.field(t, "ga_client_id")
	te.field(t, "consent")
	in := legacyEvent()
	in["ga_client_id"] = "123.456"
	in["consent"] = map[string]any{"analytics": true, "version": 1}
	got := f.decode(t, upTo(te.Field, 24), f.encode(te, in))
	for k, v := range legacyEvent() {
		if got[k] != v {
			t.Errorf("field %s = %v, want %v", k, got[k], v)
		}
	}
}

func TestConsent_MissingConsentReadsAsDenied(t *testing.T) {
	f := loadFile(t)
	te, _ := f.message(t, "TrackingEvent")
	c, _ := te.field(t, "consent")
	cs, ok := f.byName(c.TypeName)
	if !ok {
		t.Fatalf("ConsentState missing")
	}
	ev := f.decode(t, te.Field, f.encode(msgDesc{Field: upTo(te.Field, 24)}, legacyEvent()))
	if ev["consent"] != nil {
		t.Fatalf("consent should be unset, got %v", ev["consent"])
	}
	// Reading through an unset message yields the default (empty) message.
	def := f.decode(t, cs.Field, nil)
	want := map[string]any{"analytics": false, "marketing": false, "personalization": false, "version": 0}
	if !reflect.DeepEqual(def, want) {
		t.Errorf("default consent = %v, want %v", def, want)
	}
}
