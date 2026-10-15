package auditv1_test

// Contract tests for the audit query filters and the audit action catalogue
// (openspec change q3-audit-log-contract, spec audit-query-contract).
//
// The proto is compiled with `buf build` (source info included) and inspected
// dynamically, so the tests do not depend on gitignored generated code.

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const auditProto = "platform/audit/v1/audit.proto"

const timestampFullName = "google.protobuf.Timestamp"

func loadFile(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	cmd := exec.Command("buf", "build", "-o", "-#format=binpb")
	cmd.Dir = "../../.." // packages/proto (buf module root)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed: %v\n%s", err, stderr.String())
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatalf("decode image: %v", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		t.Fatalf("build files: %v", err)
	}
	fd, err := files.FindFileByPath(auditProto)
	if err != nil {
		t.Fatalf("find %s: %v", auditProto, err)
	}
	return fd
}

func msg(t *testing.T, fd protoreflect.FileDescriptor, name protoreflect.Name) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(name)
	if md == nil {
		t.Fatalf("message %s missing", name)
	}
	return md
}

func field(t *testing.T, md protoreflect.MessageDescriptor, name protoreflect.Name) protoreflect.FieldDescriptor {
	t.Helper()
	f := md.Fields().ByName(name)
	if f == nil {
		t.Fatalf("%s.%s missing", md.Name(), name)
	}
	return f
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

func comment(fd protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	loc := fd.SourceLocations().ByDescriptor(d)
	return norm(loc.LeadingComments + " " + loc.TrailingComments)
}

func hasAll(t *testing.T, what, got string, words ...string) {
	t.Helper()
	lc := strings.ToLower(got)
	for _, w := range words {
		if !strings.Contains(lc, strings.ToLower(w)) {
			t.Errorf("%s comment missing %q; got %q", what, w, got)
		}
	}
}

func roundTrip(t *testing.T, md protoreflect.MessageDescriptor, m *dynamicpb.Message) *dynamicpb.Message {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatal(err)
	}
	return got
}

var newFilterFields = []protoreflect.Name{"target_id", "action", "created_after", "created_before"}

func TestQueryAuditLogRequest_FieldNumbers_Scenario_NewFilterFieldsExistWithFixedNumbers(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "QueryAuditLogRequest")

	for name, num := range map[protoreflect.Name]protoreflect.FieldNumber{"actor_id": 1, "target_type": 2} {
		f := field(t, req, name)
		if f.Number() != num || f.Kind() != protoreflect.StringKind || f.IsList() {
			t.Errorf("%s = number %d kind %v, want string = %d", name, f.Number(), f.Kind(), num)
		}
		if f.HasOptionalKeyword() {
			t.Errorf("%s must stay a plain proto3 string (not optional)", name)
		}
	}
	page := field(t, req, "page")
	if page.Number() != 3 || page.Kind() != protoreflect.MessageKind || page.Message().FullName() != "platform.common.v1.PageRequest" {
		t.Errorf("page changed: number %d kind %v", page.Number(), page.Kind())
	}

	for name, num := range map[protoreflect.Name]protoreflect.FieldNumber{"target_id": 4, "action": 5} {
		f := field(t, req, name)
		if f.Number() != num || f.Kind() != protoreflect.StringKind || f.IsList() {
			t.Errorf("%s = number %d kind %v, want string = %d", name, f.Number(), f.Kind(), num)
		}
		if !f.HasOptionalKeyword() {
			t.Errorf("%s must be proto3 optional", name)
		}
	}
	for name, num := range map[protoreflect.Name]protoreflect.FieldNumber{"created_after": 6, "created_before": 7} {
		f := field(t, req, name)
		if f.Number() != num || f.Kind() != protoreflect.MessageKind || f.IsList() ||
			string(f.Message().FullName()) != timestampFullName {
			t.Errorf("%s = number %d kind %v, want %s = %d", name, f.Number(), f.Kind(), timestampFullName, num)
		}
	}
	if req.Fields().Len() != 7 {
		t.Errorf("QueryAuditLogRequest has %d fields, want exactly 7", req.Fields().Len())
	}
}

func TestQueryAuditLogRequest_RoundTripEqualTimeBounds_Scenario_RoundTripOfEqualTimeBounds(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "QueryAuditLogRequest")
	after, before := field(t, req, "created_after"), field(t, req, "created_before")

	m := dynamicpb.NewMessage(req)
	ts := dynamicpb.NewMessage(after.Message())
	ts.Set(after.Message().Fields().ByName("seconds"), protoreflect.ValueOfInt64(1760000000))
	ts.Set(after.Message().Fields().ByName("nanos"), protoreflect.ValueOfInt32(500))
	m.Set(after, protoreflect.ValueOfMessage(ts))
	m.Set(before, protoreflect.ValueOfMessage(ts))

	got := roundTrip(t, req, m)
	for _, f := range []protoreflect.FieldDescriptor{after, before} {
		if !got.Has(f) {
			t.Fatalf("%s lost in round trip", f.Name())
		}
		if !proto.Equal(got.Get(f).Message().Interface(), ts) {
			t.Errorf("%s changed in round trip", f.Name())
		}
	}
	if !proto.Equal(got, m) {
		t.Error("request changed in round trip")
	}
}

func TestQueryAuditLogRequest_RoundTripEmptyActionLongTargetID_Scenario_RoundTripOfEmptyActionAndLongTargetId(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "QueryAuditLogRequest")
	action, targetID := field(t, req, "action"), field(t, req, "target_id")
	long := strings.Repeat("t", 256)

	m := dynamicpb.NewMessage(req)
	m.Set(action, protoreflect.ValueOfString(""))
	m.Set(targetID, protoreflect.ValueOfString(long))

	got := roundTrip(t, req, m)
	if !got.Has(action) {
		t.Error("empty action lost presence in round trip")
	}
	if got.Get(action).String() != "" {
		t.Error("action changed in round trip")
	}
	if got.Get(targetID).String() != long {
		t.Error("256-char target_id changed in round trip")
	}
}

func TestQueryAuditLogRequest_EmptyLeavesNewFieldsUnset_Scenario_UntouchedRequestLeavesNewFieldsUnset(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "QueryAuditLogRequest")
	got := roundTrip(t, req, dynamicpb.NewMessage(req))
	for _, n := range newFilterFields {
		if got.Has(field(t, req, n)) {
			t.Errorf("%s set on an untouched request", n)
		}
	}
}

func TestQueryAuditLogRequest_OldBytesParse_Scenario_OldClientRequestStillParses(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "QueryAuditLogRequest")

	// Bytes as an old client produced them: actor_id=1, target_type=2, page=3 (empty message).
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendString(b, "actor-1")
	b = protowire.AppendTag(b, 2, protowire.BytesType)
	b = protowire.AppendString(b, "listing")
	b = protowire.AppendTag(b, 3, protowire.BytesType)
	b = protowire.AppendBytes(b, nil)

	got := dynamicpb.NewMessage(req)
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("old bytes failed to parse: %v", err)
	}
	if got.Get(field(t, req, "actor_id")).String() != "actor-1" ||
		got.Get(field(t, req, "target_type")).String() != "listing" || !got.Has(field(t, req, "page")) {
		t.Error("old fields changed when parsed with the new descriptor")
	}
	for _, n := range newFilterFields {
		if got.Has(field(t, req, n)) {
			t.Errorf("%s set after parsing old bytes", n)
		}
	}
	if len(got.GetUnknown()) != 0 {
		t.Error("old bytes left unknown fields")
	}
}

func TestQueryAuditLog_DocComments_Scenarios_FilterSemanticsAndNonAdminAccessDocumented(t *testing.T) {
	fd := loadFile(t)
	svc := fd.Services().ByName("AuditService")
	if svc == nil {
		t.Fatal("AuditService missing")
	}
	m := svc.Methods().ByName("QueryAuditLog")
	if m == nil {
		t.Fatal("AuditService.QueryAuditLog missing")
	}
	c := comment(fd, m)
	hasAll(t, "QueryAuditLog RPC", c,
		"admin-only", "AND", "InvalidArgument", "created_before", "created_after",
		"newest first", "PermissionDenied", "Unauthenticated", "non-admin", "anonymous")

	req := msg(t, fd, "QueryAuditLogRequest")
	for name, words := range map[protoreflect.Name][]string{
		"target_id":      {"exact match"},
		"action":         {"exact match"},
		"created_after":  {"inclusive"},
		"created_before": {"exclusive"},
	} {
		hasAll(t, string(name), comment(fd, field(t, req, name)), words...)
	}
}

var catalogue = []string{
	"review.hide", "kyc.review", "dispute.resolve", "listing.admin_update", "listing.admin_delete",
	"order.force_fail", "order.admin_status_update", "return.admin_status_update",
	"shipment.admin_create", "payment.admin_refund", "voucher.platform_create",
	"flash_sale.admin_create", "ad_campaign.admin_create",
}

func TestWriteAuditEventRequest_Comment_Scenarios_NamingRuleCatalogueAndAdminPathWording(t *testing.T) {
	fd := loadFile(t)
	c := comment(fd, msg(t, fd, "WriteAuditEventRequest"))
	if c == "" {
		t.Fatal("WriteAuditEventRequest has no doc comment")
	}
	for _, s := range append([]string{"<entity>.<verb>"}, catalogue...) {
		if !strings.Contains(c, s) {
			t.Errorf("WriteAuditEventRequest comment missing %q; got %q", s, c)
		}
	}
	// Uniform admin-path wording and non-admin exclusion.
	hasAll(t, "WriteAuditEventRequest", c,
		"admin", "order.admin", "even when", "owns the resource", "never audited", "permission denied")
}

func TestWriteAuditEvent_WireUnchanged_Scenario_WriteWireIsUnchanged(t *testing.T) {
	fd := loadFile(t)
	type fld struct {
		name protoreflect.Name
		num  protoreflect.FieldNumber
		kind protoreflect.Kind
	}
	check := func(m protoreflect.Name, want []fld) {
		md := msg(t, fd, m)
		for _, w := range want {
			f := field(t, md, w.name)
			if f.Number() != w.num || f.Kind() != w.kind {
				t.Errorf("%s.%s = number %d kind %v, want %d %v", m, w.name, f.Number(), f.Kind(), w.num, w.kind)
			}
		}
		if md.Fields().Len() != len(want) {
			t.Errorf("%s has %d fields, want %d", m, md.Fields().Len(), len(want))
		}
	}
	check("WriteAuditEventRequest", []fld{
		{"actor_id", 1, protoreflect.StringKind}, {"action", 2, protoreflect.StringKind},
		{"target_type", 3, protoreflect.StringKind}, {"target_id", 4, protoreflect.StringKind},
		{"metadata", 5, protoreflect.MessageKind},
	})
	check("WriteAuditEventResponse", nil)
	check("AuditEvent", []fld{
		{"id", 1, protoreflect.StringKind}, {"actor_id", 2, protoreflect.StringKind},
		{"action", 3, protoreflect.StringKind}, {"target_type", 4, protoreflect.StringKind},
		{"target_id", 5, protoreflect.StringKind}, {"metadata", 6, protoreflect.MessageKind},
		{"created_at", 7, protoreflect.MessageKind},
	})
	for _, m := range []protoreflect.Name{"WriteAuditEventRequest", "AuditEvent"} {
		if f := field(t, msg(t, fd, m), "metadata"); !f.IsMap() {
			t.Errorf("%s.metadata must stay a map", m)
		}
	}
}
