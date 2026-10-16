package aiv1_test

// Contract tests for change q6-trust-contract (capability trust-contract): AIService.ScreenListing.

import (
	"bytes"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const aiProto = "platform/ai/v1/ai.proto"

// Scenario: Trust contract: ScreenListing exists on AIService
func TestTrustContract_ScreenListingExistsOnAIService(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	tcRPCs(t, fd, "AIService", map[string][2]string{
		"ShoppingAssistant": {"ShoppingAssistantRequest", "ShoppingAssistantResponse"},
		"MagicListing":      {"MagicListingRequest", "MagicListingResponse"},
		"ChatCopilot":       {"ChatCopilotRequest", "ChatCopilotResponse"},
		"SummarizeReviews":  {"SummarizeReviewsRequest", "SummarizeReviewsResponse"},
		"ClassifyTags":      {"ClassifyTagsRequest", "ClassifyTagsResponse"},
		"ScreenListing":     {"ScreenListingRequest", "ScreenListingResponse"},
	})
}

// Scenario: Trust contract: Screen messages have fixed numbers
func TestTrustContract_ScreenMessagesHaveFixedNumbers(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	tcEnum(t, fd, "ScreenVerdict", map[string]int{
		"SCREEN_VERDICT_UNSPECIFIED": 0, "SCREEN_VERDICT_ALLOW": 1, "SCREEN_VERDICT_REVIEW": 2,
	})
	tcShape(t, fd, "ScreenListingRequest", map[string]string{
		"listing_id": "1 string", "title": "2 string", "description": "3 string", "category_id": "4 string",
	})
	tcShape(t, fd, "ScreenListingResponse", map[string]string{
		"verdict": "1 enum:platform.ai.v1.ScreenVerdict", "labels": "2 repeated string", "backend": "3 string",
	})
}

// Scenario: Trust contract: Screen response round-trips
func TestTrustContract_ScreenResponseRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	resp := tcMsg(t, fd, "ScreenListingResponse")
	m := tcNew(t, resp, "verdict", 2, "labels", []string{"weapon", "keyword:x"}, "backend", "mock")
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, resp, "verdict")).Enum() != 2 || got.Get(tcFld(t, resp, "labels")).List().Len() != 2 ||
		got.Get(tcFld(t, resp, "backend")).String() != "mock" {
		t.Errorf("response fields changed in round trip")
	}
	req := tcMsg(t, fd, "ScreenListingRequest")
	desc := strings.Repeat("d", 5000)
	r := tcNew(t, req, "title", "t", "description", desc, "category_id", "c")
	gotReq := tcRoundEqual(t, r)
	if gotReq.Get(tcFld(t, req, "listing_id")).String() != "" {
		t.Errorf("empty listing_id not preserved")
	}
	if gotReq.Get(tcFld(t, req, "description")).String() != desc {
		t.Errorf("5,000-character description changed in round trip")
	}
}

// Scenario: Trust contract: ScreenListing is documented as internal
func TestTrustContract_ScreenListingIsDocumentedAsInternal(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	c := tcRPCComment(t, fd, "AIService", "ScreenListing")
	tcCheck(t, "ScreenListing", c, "scope ai.classify", "gateway does not route", "PERMISSION_DENIED",
		"UNAUTHENTICATED", "mock", "llm_router", "mock backend returns ALLOW", "REVIEW", "error or timeout",
		"never UNSPECIFIED", "listing_id is empty on create")
}

// Scenario: Trust contract: Missing internal note is caught
func TestTrustContract_MissingInternalNoteIsCaught(t *testing.T) {
	if err := tcKeys("Screens a listing for prohibited content.", "ai.classify"); err == nil {
		t.Fatal("helper accepted a ScreenListing comment lacking ai.classify")
	}
	if err := tcKeys("scope ai.classify", "ai.classify"); err != nil {
		t.Fatalf("helper rejected a comment containing ai.classify: %v", err)
	}
}

// ---- shared helpers (prefix tc; descriptors come from `buf build`, no generated code) ----

func tcFile(t *testing.T, path string) (protoreflect.FileDescriptor, *protoregistry.Files) {
	t.Helper()
	cmd := exec.Command("buf", "build", "-o", "-#format=binpb")
	cmd.Dir = "../../.." // packages/proto (buf module root)
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

func tcMsg(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Fatalf("message %s missing from %s", name, fd.Path())
	}
	return md
}

func tcFld(t *testing.T, md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	t.Helper()
	f := md.Fields().ByName(protoreflect.Name(name))
	if f == nil {
		t.Fatalf("%s.%s missing", md.Name(), name)
	}
	return f
}

func tcKind(f protoreflect.FieldDescriptor) string {
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
	return s
}

// tcShape asserts the message has exactly the wanted fields: name -> "<number> <kind>".
// A wanted value may list alternatives separated by "|" (each "<number> <kind>").
func tcShape(t *testing.T, fd protoreflect.FileDescriptor, msgName string, want map[string]string) {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(msgName))
	if md == nil {
		t.Errorf("message %s missing", msgName)
		return
	}
	got := map[string]string{}
	for i := 0; i < md.Fields().Len(); i++ {
		f := md.Fields().Get(i)
		got[string(f.Name())] = fmt.Sprintf("%d %s", f.Number(), tcKind(f))
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s.%s missing (want %s)", msgName, name, w)
			continue
		}
		match := false
		for _, alt := range strings.Split(w, "|") {
			if g == alt {
				match = true
			}
		}
		if !match {
			t.Errorf("%s.%s = %q, want %q", msgName, name, g, w)
		}
	}
	for name, g := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s has unexpected field %s = %q", msgName, name, g)
		}
	}
}

func tcEnum(t *testing.T, fd protoreflect.FileDescriptor, name string, want map[string]int) {
	t.Helper()
	ed := fd.Enums().ByName(protoreflect.Name(name))
	if ed == nil {
		t.Errorf("enum %s missing", name)
		return
	}
	got := map[string]int{}
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		got[string(v.Name())] = int(v.Number())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enum %s = %v, want exactly %v", name, got, want)
	}
}

// tcRPCs asserts the service has exactly the wanted methods: name -> {request, response} short names.
func tcRPCs(t *testing.T, fd protoreflect.FileDescriptor, svc string, want map[string][2]string) {
	t.Helper()
	sd := fd.Services().ByName(protoreflect.Name(svc))
	if sd == nil {
		t.Errorf("service %s missing", svc)
		return
	}
	got := map[string][2]string{}
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		got[string(m.Name())] = [2]string{string(m.Input().Name()), string(m.Output().Name())}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s methods = %v, want exactly %v", svc, got, want)
	}
}

func tcNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

func tcComment(fd protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	loc := fd.SourceLocations().ByDescriptor(d)
	return tcNorm(loc.LeadingComments + " " + loc.TrailingComments)
}

func tcRPCComment(t *testing.T, fd protoreflect.FileDescriptor, svc, rpc string) string {
	t.Helper()
	sd := fd.Services().ByName(protoreflect.Name(svc))
	if sd == nil {
		t.Errorf("service %s missing", svc)
		return ""
	}
	m := sd.Methods().ByName(protoreflect.Name(rpc))
	if m == nil {
		t.Errorf("rpc %s.%s missing", svc, rpc)
		return ""
	}
	return tcComment(fd, m)
}

func tcFieldComment(t *testing.T, fd protoreflect.FileDescriptor, msg, field string) string {
	t.Helper()
	return tcComment(fd, tcFld(t, tcMsg(t, fd, msg), field))
}

// tcKeys returns an error naming every key (whitespace-normalised substring) missing from the comment.
func tcKeys(comment string, keys ...string) error {
	c := tcNorm(comment)
	if c == "" {
		return fmt.Errorf("no comment found on element")
	}
	var missing []string
	for _, k := range keys {
		if !strings.Contains(c, tcNorm(k)) {
			missing = append(missing, fmt.Sprintf("%q", k))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("comment missing %s; got:\n%s", strings.Join(missing, ", "), c)
	}
	return nil
}

func tcCheck(t *testing.T, what, comment string, keys ...string) {
	t.Helper()
	if err := tcKeys(comment, keys...); err != nil {
		t.Errorf("%s: %v", what, err)
	}
}

// tcOld builds a copy of the file as an old reader knew it (mutate edits the descriptor proto).
func tcOld(t *testing.T, files *protoregistry.Files, fd protoreflect.FileDescriptor, mutate func(*descriptorpb.FileDescriptorProto)) protoreflect.FileDescriptor {
	t.Helper()
	p := protodesc.ToFileDescriptorProto(fd)
	mutate(p)
	old, err := protodesc.NewFile(p, files)
	if err != nil {
		t.Fatalf("build old-schema file: %v", err)
	}
	return old
}

func tcDropFields(p *descriptorpb.FileDescriptorProto, msg string, keep func(num int32) bool) {
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

func tcDropEnumValues(p *descriptorpb.FileDescriptorProto, enum string, keep func(num int32) bool) {
	for _, e := range p.EnumType {
		if e.GetName() != enum {
			continue
		}
		var vs []*descriptorpb.EnumValueDescriptorProto
		for _, v := range e.Value {
			if keep(v.GetNumber()) {
				vs = append(vs, v)
			}
		}
		e.Value = vs
	}
}

func tcNew(t *testing.T, md protoreflect.MessageDescriptor, kv ...any) *dynamicpb.Message {
	t.Helper()
	m := dynamicpb.NewMessage(md)
	for i := 0; i < len(kv); i += 2 {
		f := md.Fields().ByName(protoreflect.Name(kv[i].(string)))
		if f == nil {
			t.Fatalf("no field %s on %s", kv[i].(string), md.Name())
		}
		switch v := kv[i+1].(type) {
		case string:
			m.Set(f, protoreflect.ValueOfString(v))
		case int:
			switch f.Kind() {
			case protoreflect.EnumKind:
				m.Set(f, protoreflect.ValueOfEnum(protoreflect.EnumNumber(v)))
			case protoreflect.Int32Kind:
				m.Set(f, protoreflect.ValueOfInt32(int32(v)))
			default:
				m.Set(f, protoreflect.ValueOfInt64(int64(v)))
			}
		case []string:
			l := m.Mutable(f).List()
			for _, s := range v {
				l.Append(protoreflect.ValueOfString(s))
			}
		case *dynamicpb.Message:
			m.Set(f, protoreflect.ValueOfMessage(v))
		default:
			panic("unsupported value type")
		}
	}
	return m
}

// tcTS builds a Timestamp for the given message field.
func tcTS(f protoreflect.FieldDescriptor, seconds int64) *dynamicpb.Message {
	ts := dynamicpb.NewMessage(f.Message())
	ts.Set(f.Message().Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	return ts
}

// tcRound serialises m and parses it back into a fresh message of the same descriptor.
func tcRound(t *testing.T, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(m.Descriptor())
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatal(err)
	}
	return got
}

func tcRoundEqual(t *testing.T, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	got := tcRound(t, m)
	if !proto.Equal(got, m) {
		t.Errorf("%s changed in round trip:\n got %v\nwant %v", m.Descriptor().Name(), got, m)
	}
	return got
}

// tcParseWith serialises m (new schema) and parses it with the old message descriptor.
func tcParseWith(t *testing.T, m *dynamicpb.Message, old protoreflect.MessageDescriptor) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(old)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("old reader failed to parse: %v", err)
	}
	return got
}
