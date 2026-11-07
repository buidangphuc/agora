package orderv1_test

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

// Contract tests for change discovery-seo-contract (capability discovery-seo-contract): order.proto.
// Baseline tables below are copied from origin/main at the time of the change.

const dsOrderProto = "platform/order/v1/order.proto"

var dsBaselineOrderEvents = map[string]map[string]string{
	"OrderPaidEvent": {
		"order_id": "1 string", "buyer_id": "2 string", "items": "3 repeated message:platform.order.v1.OrderLineItemFact",
		"total_amount": "4 int64", "currency": "5 string", "paid_at": "6 " + dsTS, "seller_id": "7 string", "ship_by": "8 " + dsTS,
	},
	"OrderCancelled": {
		"order_id": "1 string", "buyer_id": "2 string", "seller_id": "3 string", "previous_status": "4 enum:platform.order.v1.OrderStatus",
		"total_amount": "5 int64", "currency": "6 string", "cancelled_at": "7 " + dsTS, "reason_code": "8 enum:platform.order.v1.CancelReason",
	},
	"ReturnRefunded": {
		"return_id": "1 string", "order_id": "2 string", "buyer_id": "3 string", "seller_id": "4 string",
		"refund_amount": "5 int64", "currency": "6 string", "refunded_at": "7 " + dsTS,
	},
}

// Scenario: Message shape is fixed
func TestDiscovery_ListingSalesChangedShape(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	dsShape(t, fd, "ListingSalesChanged", map[string]string{
		"listing_id": "1 string", "sold_count": "2 int64", "changed_at": "3 " + dsTS,
	})
}

// Scenario: No personal or order fields
func TestDiscovery_ListingSalesChangedHasNoForbiddenFields(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	dsNoFields(t, dsMsg(t, fd, "ListingSalesChanged"), "buyer_id", "user_id", "seller_id", "order_id", "delta")
}

// Scenario: Round trip and absolute-value documentation
func TestDiscovery_ListingSalesChangedRoundTrip(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	md := dsMsg(t, fd, "ListingSalesChanged")
	m := dsNew(t, md, "listing_id", "l-1", "sold_count", int64(0), "changed_at", dsTimestamp(dsFld(t, md, "changed_at"), 1760000000))
	got := dsRoundEqual(t, m)
	if dsStr(got, "listing_id") != "l-1" || dsInt(got, "sold_count") != 0 {
		t.Errorf("round trip lost values: %v", got)
	}
	dsCheck(t, "ListingSalesChanged.sold_count", dsFieldComment(t, fd, "ListingSalesChanged", "sold_count"), "absolute", "never below 0")
}

// Scenario: Topic, key and envelope type are documented
func TestDiscovery_ListingSalesChangedTopicKeyTypeDocumented(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	dsCheck(t, "ListingSalesChanged", dsComment(fd, dsMsg(t, fd, "ListingSalesChanged")),
		"order.events", "key = listing_id", "platform.order.v1.ListingSalesChanged")
}

// Scenario: Existing order messages are untouched
func TestDiscovery_OrderEventsUnchanged(t *testing.T) {
	fd, _ := dsFile(t, dsOrderProto)
	for name, baseline := range dsBaselineOrderEvents {
		dsKeepsBaseline(t, fd, name, baseline)
	}
}

// ---- shared helpers (prefix ds; descriptors come from `buf build`, no generated code) ----

const dsTS = "message:google.protobuf.Timestamp"

func dsFile(t *testing.T, path string) (protoreflect.FileDescriptor, *protoregistry.Files) {
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
	fd, err := files.FindFileByPath(path)
	if err != nil {
		t.Fatalf("find %s: %v", path, err)
	}
	return fd, files
}

func dsMsg(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Fatalf("message %s missing from %s", name, fd.Path())
	}
	return md
}

func dsFld(t *testing.T, md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	t.Helper()
	f := md.Fields().ByName(protoreflect.Name(name))
	if f == nil {
		t.Fatalf("%s.%s missing", md.Name(), name)
	}
	return f
}

func dsKind(f protoreflect.FieldDescriptor) string {
	var s string
	switch {
	case f.IsMap():
		s = "map"
	case f.Kind() == protoreflect.MessageKind || f.Kind() == protoreflect.GroupKind:
		s = "message:" + string(f.Message().FullName())
	case f.Kind() == protoreflect.EnumKind:
		s = "enum:" + string(f.Enum().FullName())
	default:
		s = f.Kind().String()
	}
	if f.IsList() {
		s = "repeated " + s
	}
	if f.HasOptionalKeyword() {
		s = "optional " + s
	}
	return s
}

func dsFields(md protoreflect.MessageDescriptor) map[string]string {
	got := map[string]string{}
	for i := 0; i < md.Fields().Len(); i++ {
		f := md.Fields().Get(i)
		got[string(f.Name())] = fmt.Sprintf("%d %s", f.Number(), dsKind(f))
	}
	return got
}

func dsShape(t *testing.T, fd protoreflect.FileDescriptor, msgName string, want map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(msgName))
	if md == nil {
		t.Errorf("message %s missing", msgName)
		return
	}
	got := dsFields(md)
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s.%s missing (want %s)", msgName, name, w)
		} else if g != w {
			t.Errorf("%s.%s = %q, want %q", msgName, name, g, w)
		}
	}
	for name, g := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s has unexpected field %s = %q", msgName, name, g)
		}
	}
}

func dsKeepsBaseline(t *testing.T, fd protoreflect.FileDescriptor, msgName string, baseline map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(msgName))
	if md == nil {
		t.Errorf("message %s missing", msgName)
		return
	}
	got := dsFields(md)
	for name, w := range baseline {
		if g, ok := got[name]; !ok {
			t.Errorf("%s.%s removed or renamed (baseline %s)", msgName, name, w)
		} else if g != w {
			t.Errorf("%s.%s = %q, baseline %q", msgName, name, g, w)
		}
	}
}

func dsNoFields(t *testing.T, md protoreflect.MessageDescriptor, forbidden ...string) {
	t.Helper()
	for _, name := range forbidden {
		if md.Fields().ByName(protoreflect.Name(name)) != nil {
			t.Errorf("%s must not carry %s", md.Name(), name)
		}
	}
}

func dsNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

func dsComment(fd protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	loc := fd.SourceLocations().ByDescriptor(d)
	return dsNorm(loc.LeadingComments + " " + loc.TrailingComments)
}

func dsFieldComment(t *testing.T, fd protoreflect.FileDescriptor, msg, field string) string {
	t.Helper()
	return dsComment(fd, dsFld(t, dsMsg(t, fd, msg), field))
}

func dsCheck(t *testing.T, what, comment string, keys ...string) {
	t.Helper()
	c := dsNorm(comment)
	if c == "" {
		t.Errorf("%s: no comment found on element", what)
		return
	}
	var missing []string
	for _, k := range keys {
		if !strings.Contains(c, dsNorm(k)) {
			missing = append(missing, fmt.Sprintf("%q", k))
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s: comment missing %s; got:\n%s", what, strings.Join(missing, ", "), c)
	}
}

func dsOld(t *testing.T, files *protoregistry.Files, fd protoreflect.FileDescriptor, mutate func(*descriptorpb.FileDescriptorProto)) protoreflect.FileDescriptor {
	t.Helper()
	p := protodesc.ToFileDescriptorProto(fd)
	mutate(p)
	old, err := protodesc.NewFile(p, files)
	if err != nil {
		t.Fatalf("build old-schema file: %v", err)
	}
	return old
}

func dsDropFields(p *descriptorpb.FileDescriptorProto, msg string, keep func(num int32) bool) {
	for _, m := range p.MessageType {
		if m.GetName() != msg {
			continue
		}
		var fs []*descriptorpb.FieldDescriptorProto
		for _, f := range m.Field {
			if keep(f.GetNumber()) {
				fs = append(fs, f)
			}
		}
		m.Field = fs
	}
}

func dsSet(t *testing.T, m *dynamicpb.Message, name string, v any) {
	t.Helper()
	f := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		t.Fatalf("no field %s on %s", name, m.Descriptor().Name())
	}
	switch x := v.(type) {
	case string:
		m.Set(f, protoreflect.ValueOfString(x))
	case int64:
		m.Set(f, protoreflect.ValueOfInt64(x))
	case int32:
		m.Set(f, protoreflect.ValueOfInt32(x))
	case float64:
		m.Set(f, protoreflect.ValueOfFloat64(x))
	case float32:
		m.Set(f, protoreflect.ValueOfFloat32(x))
	case uint32:
		m.Set(f, protoreflect.ValueOfUint32(x))
	case []string:
		l := m.Mutable(f).List()
		for _, s := range x {
			l.Append(protoreflect.ValueOfString(s))
		}
	case *dynamicpb.Message:
		m.Set(f, protoreflect.ValueOfMessage(x))
	default:
		t.Fatalf("unsupported value type %T", v)
	}
}

func dsNew(t *testing.T, md protoreflect.MessageDescriptor, kv ...any) *dynamicpb.Message {
	t.Helper()
	m := dynamicpb.NewMessage(md)
	for i := 0; i < len(kv); i += 2 {
		dsSet(t, m, kv[i].(string), kv[i+1])
	}
	return m
}

func dsTimestamp(f protoreflect.FieldDescriptor, seconds int64) *dynamicpb.Message {
	ts := dynamicpb.NewMessage(f.Message())
	ts.Set(f.Message().Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	return ts
}

func dsParse(t *testing.T, m proto.Message, into protoreflect.MessageDescriptor) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(into)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("parse as %s: %v", into.FullName(), err)
	}
	return got
}

func dsRoundEqual(t *testing.T, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	got := dsParse(t, m, m.Descriptor())
	if !proto.Equal(got, m) {
		t.Errorf("%s changed in round trip:\n got %v\nwant %v", m.Descriptor().Name(), got, m)
	}
	return got
}

func dsStr(m protoreflect.Message, name string) string {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).String()
}

func dsInt(m protoreflect.Message, name string) int64 {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).Int()
}

func dsFloat(m protoreflect.Message, name string) float64 {
	return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).Float()
}
