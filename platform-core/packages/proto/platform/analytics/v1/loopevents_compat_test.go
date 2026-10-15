package analyticsv1_test

// Tests for change q4-loop-events-contract (capability loop-events-contract).
// Same approach as consent_compat_test.go: the schema comes from
// `buf build -o -#format=json`; wire checks use the shared schema-driven codec
// (fileDesc.encode/decode) plus a tiny map<string,string> codec for
// TrackingEvent.properties (the shared codec skips repeated/map fields).

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// The enum helpers read the same descriptor JSON as loadFile.
type enumValueDesc struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type enumDesc struct {
	Name  string          `json:"name"`
	Value []enumValueDesc `json:"value"`
}

// loopEnum returns the EventType descriptor and its index in the file.
func loopEnum(t *testing.T) (enumDesc, int) {
	t.Helper()
	return loadEventTypeEnum(t)
}

func (e enumDesc) number(name string) (int, bool) {
	for _, v := range e.Value {
		if v.Name == name {
			return v.Number, true
		}
	}
	return 0, false
}

func (e enumDesc) valueIndex(num int) int {
	for i, v := range e.Value {
		if v.Number == num {
			return i
		}
	}
	return -1
}

// pinned is the full 0-21 contract: names and numbers.
var pinned = []string{
	"EVENT_TYPE_UNSPECIFIED", "EVENT_TYPE_VIEW", "EVENT_TYPE_CLICK", "EVENT_TYPE_ADD_TO_CART",
	"EVENT_TYPE_IMPRESSION", "EVENT_TYPE_REMOVE_FROM_CART", "EVENT_TYPE_BEGIN_CHECKOUT",
	"EVENT_TYPE_APPLY_PROMOTION", "EVENT_TYPE_SEARCH_FILTER", "EVENT_TYPE_FAVORITE",
	"EVENT_TYPE_SHARE", "EVENT_TYPE_VIEW_CART", "EVENT_TYPE_ADD_SHIPPING_INFO",
	"EVENT_TYPE_ADD_PAYMENT_INFO", "EVENT_TYPE_PURCHASE",
	"EVENT_TYPE_SIGN_UP", "EVENT_TYPE_LOGIN", "EVENT_TYPE_FOLLOW_SELLER",
	"EVENT_TYPE_REFERRAL_REDEEM", "EVENT_TYPE_DAILY_CHECK_IN", "EVENT_TYPE_ALERT_SUBSCRIBE",
	"EVENT_TYPE_AI_INTERACTION",
}

const firstNew, lastNew = 15, 21

// ---- map<string,string> properties codec -----------------------------------

func propsField(t *testing.T, te msgDesc) fieldDesc {
	t.Helper()
	p, _ := te.field(t, "properties")
	return p
}

func appendProps(b []byte, num int, props map[string]string) []byte {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	// deterministic order
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		var e []byte
		e = binary.AppendUvarint(e, 1<<3|2)
		e = binary.AppendUvarint(e, uint64(len(k)))
		e = append(e, k...)
		e = binary.AppendUvarint(e, 2<<3|2)
		e = binary.AppendUvarint(e, uint64(len(props[k])))
		e = append(e, props[k]...)
		b = binary.AppendUvarint(b, uint64(num)<<3|2)
		b = binary.AppendUvarint(b, uint64(len(e)))
		b = append(b, e...)
	}
	return b
}

// readProps extracts the map entries of field num from b.
func readProps(t *testing.T, num int, b []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			t.Fatalf("bad tag")
		}
		b = b[n:]
		switch tag & 7 {
		case 0:
			_, n := binary.Uvarint(b)
			b = b[n:]
		case 2:
			l, n := binary.Uvarint(b)
			raw := b[n : n+int(l)]
			b = b[n+int(l):]
			if int(tag>>3) != num {
				continue
			}
			var k, v string
			for len(raw) > 0 {
				et, n := binary.Uvarint(raw)
				raw = raw[n:]
				el, n := binary.Uvarint(raw)
				s := string(raw[n : n+int(el)])
				raw = raw[n+int(el):]
				if et>>3 == 1 {
					k = s
				} else {
					v = s
				}
			}
			out[k] = v
		default:
			t.Fatalf("unexpected wire type %d", tag&7)
		}
	}
	return out
}

func encodeEvent(f fileDesc, te msgDesc, typ int, props map[string]string) []byte {
	b := f.encode(te, map[string]any{"event_type": typ, "listing_id": "L-1", "session_id": "S-1"})
	return appendProps(b, 9, props)
}

// ---- enum shape ------------------------------------------------------------

func TestLoopEvents_LoopEventTypesExistWithFixedNumbers(t *testing.T) {
	e, _ := loopEnum(t)
	for i := firstNew; i <= lastNew; i++ {
		n, ok := e.number(pinned[i])
		if !ok {
			t.Errorf("%s missing from EventType", pinned[i])
			continue
		}
		if n != i {
			t.Errorf("%s = %d, want %d", pinned[i], n, i)
		}
	}
	if len(e.Value) != len(pinned) {
		t.Errorf("EventType has %d values, want exactly %d", len(e.Value), len(pinned))
	}
}

func TestLoopEvents_ExistingValuesKeepTheirNumbers(t *testing.T) {
	e, _ := loopEnum(t)
	for i := 0; i < firstNew; i++ {
		n, ok := e.number(pinned[i])
		if !ok || n != i {
			t.Errorf("%s = %d (present=%v), want %d", pinned[i], n, ok, i)
		}
	}
	for name, want := range map[string]int{"EVENT_TYPE_SHARE": 10, "EVENT_TYPE_FAVORITE": 9, "EVENT_TYPE_PURCHASE": 14} {
		if n, _ := e.number(name); n != want {
			t.Errorf("%s = %d, want %d", name, n, want)
		}
	}
}

// ---- wire behaviour --------------------------------------------------------

func TestLoopEvents_EachNewTypeRoundTrips(t *testing.T) {
	f := loadFile(t)
	e, _ := loopEnum(t)
	te, _ := f.message(t, "TrackingEvent")
	pf := propsField(t, te)
	props := map[string]string{"surface": "assistant", "action": "shown"}
	for i := firstNew; i <= lastNew; i++ {
		t.Run(pinned[i], func(t *testing.T) {
			num, ok := e.number(pinned[i])
			if !ok {
				t.Fatalf("%s missing from EventType", pinned[i])
			}
			b := encodeEvent(f, te, num, props)
			got := f.decode(t, te.Field, b)
			if got["event_type"] != i {
				t.Errorf("event_type = %v, want %d", got["event_type"], i)
			}
			if gp := readProps(t, pf.Number, b); !reflect.DeepEqual(gp, props) {
				t.Errorf("properties = %v, want %v", gp, props)
			}
		})
	}
}

func TestLoopEvents_MaxValueRoundTripsWithEmptyProperties(t *testing.T) {
	f := loadFile(t)
	e, _ := loopEnum(t)
	te, _ := f.message(t, "TrackingEvent")
	num, ok := e.number("EVENT_TYPE_AI_INTERACTION")
	if !ok {
		t.Fatalf("EVENT_TYPE_AI_INTERACTION missing from EventType")
	}
	max := 0
	for _, v := range e.Value {
		if v.Number > max {
			max = v.Number
		}
	}
	if num != 21 || max != 21 {
		t.Fatalf("AI_INTERACTION = %d, highest value = %d, want both 21", num, max)
	}
	b := encodeEvent(f, te, num, nil)
	got := f.decode(t, te.Field, b)
	if got["event_type"] != 21 {
		t.Errorf("event_type = %v, want 21", got["event_type"])
	}
	if gp := readProps(t, propsField(t, te).Number, b); len(gp) != 0 {
		t.Errorf("properties = %v, want none", gp)
	}
}

func TestLoopEvents_OldBytesAndUnknownValueStillParse(t *testing.T) {
	f := loadFile(t)
	e, _ := loopEnum(t)
	te, _ := f.message(t, "TrackingEvent")
	purchase, ok := e.number("EVENT_TYPE_PURCHASE")
	if !ok {
		t.Fatalf("EVENT_TYPE_PURCHASE missing")
	}
	// Bytes produced before this change: no field above 24, PURCHASE type.
	in := legacyEvent()
	in["event_type"] = purchase
	old := f.encode(msgDesc{Field: upTo(te.Field, 24)}, in)
	got := f.decode(t, te.Field, old)
	for k, v := range in {
		if got[k] != v {
			t.Errorf("field %s = %v, want %v", k, got[k], v)
		}
	}
	// Unknown value 22 is not defined yet must survive as a number.
	if e.valueIndex(22) != -1 {
		t.Fatalf("value 22 must stay undefined in this change")
	}
	unk := f.decode(t, te.Field, encodeEvent(f, te, 22, nil))
	if unk["event_type"] != 22 {
		t.Errorf("unknown event_type = %v, want 22 preserved", unk["event_type"])
	}
	if unk["listing_id"] != "L-1" {
		t.Errorf("listing_id = %v, want L-1", unk["listing_id"])
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

func enumComment(t *testing.T, name string) (string, bool) {
	t.Helper()
	f := loadFile(t)
	e, ei := loopEnum(t)
	for vi, v := range e.Value {
		if v.Name == name {
			return f.comment(5, ei, 2, vi)
		}
	}
	t.Fatalf("%s missing from EventType", name)
	return "", false
}

func TestLoopEventsComments_PropertyKeysAreDocumented(t *testing.T) {
	cases := []struct {
		name string
		keys []string
	}{
		{"EVENT_TYPE_SIGN_UP", []string{"method"}},
		{"EVENT_TYPE_LOGIN", []string{"method"}},
		{"EVENT_TYPE_FOLLOW_SELLER", []string{"seller_id"}},
		{"EVENT_TYPE_DAILY_CHECK_IN", []string{"streak"}},
		{"EVENT_TYPE_ALERT_SUBSCRIBE", []string{"alert_type", "listing_id"}},
		{"EVENT_TYPE_AI_INTERACTION", []string{"surface", "action"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cm, ok := enumComment(t, c.name)
			if err := checkKeys(cm, ok, c.keys...); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestLoopEventsComments_ReferralRedeemStatesNoPropertiesAndCodeNeverSent(t *testing.T) {
	cm, ok := enumComment(t, "EVENT_TYPE_REFERRAL_REDEEM")
	if err := checkKeys(cm, ok, "No properties", "never sent"); err != nil {
		t.Error(err)
	}
}

func TestLoopEventsComments_AIInteractionForbidsFreeText(t *testing.T) {
	cm, ok := enumComment(t, "EVENT_TYPE_AI_INTERACTION")
	if err := checkKeys(cm, ok, "exactly surface, action", "allowlisted", "no free text"); err != nil {
		t.Error(err)
	}
}

func TestLoopEventsComments_MissingKeyCommentIsCaught(t *testing.T) {
	fixture := "Daily loyalty check-in completed."
	if err := checkKeys(fixture, true, "streak"); err == nil {
		t.Fatal("helper accepted a value-19 comment lacking streak")
	}
	if err := checkKeys("", false, "streak"); err == nil {
		t.Fatal("helper accepted a missing comment")
	}
	if err := checkKeys("properties: streak (optional).", true, "streak"); err != nil {
		t.Fatalf("helper rejected a comment containing streak: %v", err)
	}
}

// loadEventTypeEnum decodes the enum list from the same buf descriptor JSON.
func loadEventTypeEnum(t *testing.T) (enumDesc, int) {
	t.Helper()
	var set struct {
		File []struct {
			Name     string     `json:"name"`
			EnumType []enumDesc `json:"enumType"`
		} `json:"file"`
	}
	loadDescriptorJSON(t, &set)
	for _, f := range set.File {
		if f.Name != analyticsFile {
			continue
		}
		for i, e := range f.EnumType {
			if e.Name == "EventType" {
				return e, i
			}
		}
	}
	t.Fatalf("EventType enum missing from %s", analyticsFile)
	return enumDesc{}, -1
}

func loadDescriptorJSON(t *testing.T, into any) {
	t.Helper()
	cmd := exec.Command("buf", "build", "--path", analyticsFile, "-o", "-#format=json")
	cmd.Dir = "../../.."
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed (harness): %v\n%s", err, errb.String())
	}
	if err := json.Unmarshal(out.Bytes(), into); err != nil {
		t.Fatalf("decode descriptor json: %v", err)
	}
}
