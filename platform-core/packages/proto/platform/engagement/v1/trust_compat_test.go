package engagementv1_test

// Contract tests for change q6-trust-contract (capability trust-contract): reports, dispute rulings, DisputeResolved.

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

const engProto = "platform/engagement/v1/engagement.proto"
const evProto = "platform/engagement/v1/events.proto"
const engPageReq = "message:platform.common.v1.PageRequest"
const engPageResp = "message:platform.common.v1.PageResponse"
const engTS = "message:google.protobuf.Timestamp"
const rptStatus = "enum:platform.engagement.v1.ReportStatus"
const dspStatus = "enum:platform.engagement.v1.DisputeStatus"
const dspRuling = "enum:platform.engagement.v1.DisputeRuling"

// methodsOf returns the service's methods; failing when the service is missing.
func engMethods(t *testing.T, fd protoreflect.FileDescriptor) map[string][2]string {
	t.Helper()
	sd := fd.Services().ByName("EngagementService")
	if sd == nil {
		t.Fatal("EngagementService missing")
	}
	got := map[string][2]string{}
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		got[string(m.Name())] = [2]string{string(m.Input().Name()), string(m.Output().Name())}
	}
	return got
}

func engWant(t *testing.T, fd protoreflect.FileDescriptor, want map[string][2]string) {
	t.Helper()
	got := engMethods(t, fd)
	for name, w := range want {
		if g, ok := got[name]; !ok {
			t.Errorf("EngagementService.%s missing", name)
		} else if g != w {
			t.Errorf("EngagementService.%s = %v, want %v", name, g, w)
		}
	}
	// pre-existing RPCs are unchanged
	for name, w := range map[string][2]string{
		"CreateReview":   {"CreateReviewRequest", "CreateReviewResponse"},
		"HideReview":     {"HideReviewRequest", "HideReviewResponse"},
		"CreateDispute":  {"CreateDisputeRequest", "CreateDisputeResponse"},
		"GetDispute":     {"GetDisputeRequest", "GetDisputeResponse"},
		"ResolveDispute": {"ResolveDisputeRequest", "ResolveDisputeResponse"},
		"AddFavorite":    {"AddFavoriteRequest", "AddFavoriteResponse"},
	} {
		if g := got[name]; g != w {
			t.Errorf("pre-existing EngagementService.%s = %v, want unchanged %v", name, g, w)
		}
	}
}

// Scenario: Trust contract: Report RPCs exist on EngagementService
func TestTrustContract_ReportRPCsExistOnEngagementService(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	engWant(t, fd, map[string][2]string{
		"CreateReport":  {"CreateReportRequest", "CreateReportResponse"},
		"ListReports":   {"ListReportsRequest", "ListReportsResponse"},
		"ResolveReport": {"ResolveReportRequest", "ResolveReportResponse"},
	})
}

// Scenario: Trust contract: Report messages have fixed numbers
func TestTrustContract_ReportMessagesHaveFixedNumbers(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	tcEnum(t, fd, "ReportTargetType", map[string]int{"REPORT_TARGET_TYPE_UNSPECIFIED": 0, "REPORT_TARGET_TYPE_LISTING": 1})
	tcEnum(t, fd, "ReportStatus", map[string]int{
		"REPORT_STATUS_UNSPECIFIED": 0, "REPORT_STATUS_OPEN": 1, "REPORT_STATUS_ACTIONED": 2, "REPORT_STATUS_DISMISSED": 3,
	})
	tcShape(t, fd, "Report", map[string]string{
		"id": "1 string", "target_type": "2 enum:platform.engagement.v1.ReportTargetType", "target_id": "3 string",
		"reason_code": "4 string", "note": "5 string", "reporter_id": "6 string", "status": "7 " + rptStatus,
		"created_at": "8 " + engTS, "resolved_at": "9 " + engTS, "resolution_note": "10 string",
	})
	tcShape(t, fd, "CreateReportRequest", map[string]string{
		"target_type": "1 enum:platform.engagement.v1.ReportTargetType", "target_id": "2 string",
		"reason_code": "3 string", "note": "4 string",
	})
	tcShape(t, fd, "CreateReportResponse", map[string]string{"report": "1 message:platform.engagement.v1.Report"})
	tcShape(t, fd, "ListReportsRequest", map[string]string{"status": "1 " + rptStatus, "target_id": "2 string", "page": "3 " + engPageReq})
	tcShape(t, fd, "ListReportsResponse", map[string]string{
		"reports": "1 repeated message:platform.engagement.v1.Report", "page": "2 " + engPageResp,
	})
	tcShape(t, fd, "ResolveReportRequest", map[string]string{"report_id": "1 string", "status": "2 " + rptStatus, "resolution_note": "3 string"})
	tcShape(t, fd, "ResolveReportResponse", map[string]string{"report": "1 message:platform.engagement.v1.Report"})
}

// Scenario: Trust contract: Report round-trips
func TestTrustContract_ReportRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	rep := tcMsg(t, fd, "Report")
	note := strings.Repeat("r", 500)
	m := tcNew(t, rep, "id", "r-1", "target_type", 1, "target_id", "3f2b8c1e-0000-4000-8000-000000000001",
		"reason_code", "scam", "note", note, "reporter_id", "u-1", "status", 2,
		"created_at", tcTS(tcFld(t, rep, "created_at"), 1760000000), "resolved_at", tcTS(tcFld(t, rep, "resolved_at"), 1760000900))
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, rep, "note")).String() != note || got.Get(tcFld(t, rep, "status")).Enum() != 2 {
		t.Errorf("note or status changed in round trip")
	}
	req := tcMsg(t, fd, "CreateReportRequest")
	gotReq := tcRoundEqual(t, tcNew(t, req, "target_type", 1, "target_id", "x", "reason_code", "other"))
	if gotReq.Get(tcFld(t, req, "note")).String() != "" {
		t.Errorf("empty note not preserved")
	}
}

// Scenario: Trust contract: CreateReport rules are documented
func TestTrustContract_CreateReportRulesAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	c := tcRPCComment(t, fd, "EngagementService", "CreateReport")
	tcCheck(t, "CreateReport", c, "UNAUTHENTICATED", "anonymous", "INVALID_ARGUMENT", "target type UNSPECIFIED",
		"not a UUID", "prohibited_item | counterfeit | scam | misleading | inappropriate | other", "note over 500",
		"FAILED_PRECONDITION", "caller owns the listing", "RESOURCE_EXHAUSTED", "20 OPEN reports",
		"idempotent per (reporter, target) while OPEN", "returns the existing report", "not checked for existence")
}

// Scenario: Trust contract: Report admin RPCs are documented
func TestTrustContract_ReportAdminRPCsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	list := tcRPCComment(t, fd, "EngagementService", "ListReports")
	res := tcRPCComment(t, fd, "EngagementService", "ResolveReport")
	tcCheck(t, "ListReports", list, "admin-only", "oldest first", "not audited", "UNSPECIFIED status means all")
	tcCheck(t, "ResolveReport", res, "admin-only", "report.resolve", "first", "UNAVAILABLE", "no change",
		"ACTIONED or DISMISSED", "INVALID_ARGUMENT", "resolution_note", "1 to 500 characters after trim",
		"NOT_FOUND", "FAILED_PRECONDITION", "already closed")
}

// Scenario: Trust contract: Reporter identity is documented as admin-only
func TestTrustContract_ReporterIdentityIsDocumentedAsAdminOnly(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	tcCheck(t, "Report.reporter_id", tcFieldComment(t, fd, "Report", "reporter_id"),
		"returned to the reporter and to admin only", "never to the reported seller")
}

// Scenario: Trust contract: Missing audit-first note is caught
func TestTrustContract_MissingAuditFirstNoteIsCaught(t *testing.T) {
	if err := tcKeys("Closes a report as ACTIONED or DISMISSED.", "report.resolve"); err == nil {
		t.Fatal("helper accepted a ResolveReport comment lacking report.resolve")
	}
	if err := tcKeys("audit report.resolve first", "report.resolve"); err != nil {
		t.Fatalf("helper rejected a comment containing report.resolve: %v", err)
	}
}

// Scenario: Trust contract: Dispute RPCs exist
func TestTrustContract_DisputeRPCsExist(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	engWant(t, fd, map[string][2]string{
		"ListOrderDisputes": {"ListOrderDisputesRequest", "ListOrderDisputesResponse"},
		"ListDisputes":      {"ListDisputesRequest", "ListDisputesResponse"},
	})
}

// Scenario: Trust contract: Dispute fields have fixed numbers
func TestTrustContract_DisputeFieldsHaveFixedNumbers(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	tcEnum(t, fd, "DisputeRuling", map[string]int{
		"DISPUTE_RULING_UNSPECIFIED": 0, "DISPUTE_RULING_BUYER_FAVOUR": 1, "DISPUTE_RULING_SELLER_FAVOUR": 2,
	})
	tcShape(t, fd, "Dispute", map[string]string{
		"id": "1 string", "order_id": "2 string", "claimant_id": "3 string", "defendant_id": "4 string",
		"reason": "5 string", "evidence_urls": "6 repeated string", "status": "7 " + dspStatus,
		"resolution": "8 string", "created_at": "9 " + engTS, "updated_at": "10 " + engTS,
		"return_id": "11 string", "ruling": "12 " + dspRuling,
	})
	tcShape(t, fd, "CreateDisputeRequest", map[string]string{
		"order_id": "1 string", "defendant_id": "2 string", "reason": "3 string",
		"evidence_urls": "4 repeated string", "return_id": "5 string",
	})
	tcShape(t, fd, "ResolveDisputeRequest", map[string]string{
		"dispute_id": "1 string", "status": "2 " + dspStatus, "resolution": "3 string", "ruling": "4 " + dspRuling,
	})
	tcShape(t, fd, "ListOrderDisputesRequest", map[string]string{"order_id": "1 string"})
	tcShape(t, fd, "ListOrderDisputesResponse", map[string]string{"disputes": "1 repeated message:platform.engagement.v1.Dispute"})
	tcShape(t, fd, "ListDisputesRequest", map[string]string{"status": "1 " + dspStatus, "page": "2 " + engPageReq})
	tcShape(t, fd, "ListDisputesResponse", map[string]string{
		"disputes": "1 repeated message:platform.engagement.v1.Dispute", "page": "2 " + engPageResp,
	})
}

// Scenario: Trust contract: Dispute with ruling round-trips
func TestTrustContract_DisputeWithRulingRoundTrips(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	d := tcMsg(t, fd, "Dispute")
	m := tcNew(t, d, "id", "d-1", "order_id", "o-1", "return_id", "ret-1", "status", 3, "ruling", 1)
	got := tcRoundEqual(t, m)
	if got.Get(tcFld(t, d, "return_id")).String() != "ret-1" || got.Get(tcFld(t, d, "ruling")).Enum() != 1 {
		t.Errorf("return_id or ruling changed in round trip")
	}
	bare := tcRound(t, tcNew(t, d, "id", "d-2", "status", 1))
	if bare.Get(tcFld(t, d, "return_id")).String() != "" || bare.Get(tcFld(t, d, "ruling")).Enum() != 0 {
		t.Errorf("a Dispute without fields 11 and 12 must parse with empty return_id and ruling UNSPECIFIED")
	}
	rq := tcMsg(t, fd, "ResolveDisputeRequest")
	gotRq := tcRoundEqual(t, tcNew(t, rq, "dispute_id", "d-1", "status", 3, "ruling", 2))
	if gotRq.Get(tcFld(t, rq, "ruling")).Enum() != 2 {
		t.Errorf("ruling SELLER_FAVOUR changed in round trip")
	}
}

// Scenario: Trust contract: Old readers skip the dispute additions
func TestTrustContract_OldReadersSkipTheDisputeAdditions(t *testing.T) {
	fd, files := tcFile(t, engProto)
	m := tcNew(t, tcMsg(t, fd, "Dispute"), "id", "d-1", "order_id", "o-1", "return_id", "ret-1", "status", 3, "ruling", 1)
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropFields(p, "Dispute", func(n int32) bool { return n <= 10 })
	})
	oldMsg := old.Messages().ByName("Dispute")
	got := tcParseWith(t, m, oldMsg)
	if got.Get(oldMsg.Fields().ByName("id")).String() != "d-1" || got.Get(oldMsg.Fields().ByName("status")).Enum() != 3 {
		t.Errorf("old reader lost known fields")
	}
	if len(got.GetUnknown()) == 0 {
		t.Errorf("fields 11 and 12 should be skipped into unknown fields")
	}
}

// Scenario: Trust contract: DisputeResolved message is fixed
func TestTrustContract_DisputeResolvedMessageIsFixed(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcShape(t, fd, "DisputeResolved", map[string]string{
		"dispute_id": "1 string", "order_id": "2 string", "return_id": "3 string",
		"status": "4 " + dspStatus, "ruling": "5 " + dspRuling, "resolver_id": "6 string", "resolved_at": "7 " + engTS,
	})
	for _, name := range []string{"FavoriteAdded", "FavoriteRemoved", "SellerFollowed", "SellerUnfollowed", "ReviewCreated"} {
		if fd.Messages().ByName(protoreflect.Name(name)) == nil {
			t.Errorf("existing event message %s missing", name)
		}
	}
	dr := tcMsg(t, fd, "DisputeResolved")
	m := tcNew(t, dr, "dispute_id", "d-1", "order_id", "o-1", "return_id", "ret-1", "status", 3, "ruling", 1,
		"resolver_id", "admin-1", "resolved_at", tcTS(tcFld(t, dr, "resolved_at"), 1760000000))
	tcRoundEqual(t, m)
}

// Scenario: Trust contract: Dispute escalation errors are documented
func TestTrustContract_DisputeEscalationErrorsAreDocumented(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	c := tcRPCComment(t, fd, "EngagementService", "CreateDispute")
	tcCheck(t, "CreateDispute", c, "return_id", "claimant must be the order's buyer", "defendant is the order's seller",
		"INVALID_ARGUMENT", "FAILED_PRECONDITION", "can_escalate", "ALREADY_EXISTS", "OPEN or INVESTIGATING",
		"while can_escalate is true", "NOT_FOUND", "UNAVAILABLE", "team-order")
}

// Scenario: Trust contract: Dispute state machine is documented
func TestTrustContract_DisputeStateMachineIsDocumented(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	c := tcRPCComment(t, fd, "EngagementService", "ResolveDispute")
	tcCheck(t, "ResolveDispute", c, "OPEN to INVESTIGATING or REJECTED",
		"INVESTIGATING to INVESTIGATING (note update), RESOLVED or REJECTED",
		"OPEN to RESOLVED", "FAILED_PRECONDITION", "out of RESOLVED or REJECTED",
		"ruling is required for RESOLVED on a return-linked dispute", "INVALID_ARGUMENT",
		"ruling sent with any status other than RESOLVED", "dispute.resolve", "first",
		"exactly one DisputeResolved", "RESOLVED or REJECTED")
}

// Scenario: Trust contract: Dispute listing authz is documented
func TestTrustContract_DisputeListingAuthzIsDocumented(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	one := tcRPCComment(t, fd, "EngagementService", "ListOrderDisputes")
	all := tcRPCComment(t, fd, "EngagementService", "ListDisputes")
	tcCheck(t, "ListOrderDisputes", one, "order's buyer", "order's seller", "admin", "PERMISSION_DENIED",
		"INVALID_ARGUMENT", "empty order_id", "newest first")
	tcCheck(t, "ListDisputes", all, "admin-only", "oldest first", "UNSPECIFIED status means all", "not audited")
}

// Scenario: Trust contract: Non-party callers are denied dispute reads
func TestTrustContract_NonPartyCallersAreDeniedDisputeReads(t *testing.T) {
	fd, _ := tcFile(t, engProto)
	one := tcRPCComment(t, fd, "EngagementService", "ListOrderDisputes")
	all := tcRPCComment(t, fd, "EngagementService", "ListDisputes")
	tcCheck(t, "ListOrderDisputes", one, "neither the order's buyer, the order's seller nor an admin gets PERMISSION_DENIED")
	tcCheck(t, "ListDisputes", all, "any non-admin gets PERMISSION_DENIED")
}

// Scenario: Trust contract: DisputeResolved delivery is documented
func TestTrustContract_DisputeResolvedDeliveryIsDocumented(t *testing.T) {
	fd, _ := tcFile(t, evProto)
	tcCheck(t, "DisputeResolved", tcComment(fd, tcMsg(t, fd, "DisputeResolved")),
		"engagement.events", "key = dispute id", "platform.engagement.v1.DisputeResolved", "no free text")
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
