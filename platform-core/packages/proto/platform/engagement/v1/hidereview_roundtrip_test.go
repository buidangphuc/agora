package engagementv1_test

// Contract tests for the HideReview RPC and the review rule comments
// (openspec change q2-review-integrity-contract, spec engagement-review-contract).
//
// The proto is compiled with `buf build` (source info included) and inspected
// dynamically, so the tests do not depend on gitignored generated code.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const engagementProto = "platform/engagement/v1/engagement.proto"

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
	fd, err := files.FindFileByPath(engagementProto)
	if err != nil {
		t.Fatalf("find %s: %v", engagementProto, err)
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

func hideReviewMethod(t *testing.T, fd protoreflect.FileDescriptor) protoreflect.MethodDescriptor {
	t.Helper()
	svc := fd.Services().ByName("EngagementService")
	if svc == nil {
		t.Fatal("EngagementService missing")
	}
	m := svc.Methods().ByName("HideReview")
	if m == nil {
		t.Fatal("EngagementService.HideReview missing")
	}
	return m
}

func TestHideReview_RPCSignature_Scenario_ServiceAndMessagesExist(t *testing.T) {
	fd := loadFile(t)
	m := hideReviewMethod(t, fd)
	if m.Input().Name() != "HideReviewRequest" || m.Output().Name() != "HideReviewResponse" {
		t.Errorf("signature = %s -> %s", m.Input().Name(), m.Output().Name())
	}
	if m.IsStreamingClient() || m.IsStreamingServer() {
		t.Error("HideReview must be unary")
	}
}

func TestHideReviewRequest_FieldNumbers_Scenarios_RequestFieldNumbersFixedAndNoActorField(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "HideReviewRequest")
	for name, num := range map[protoreflect.Name]protoreflect.FieldNumber{"review_id": 1, "reason": 2} {
		f := field(t, req, name)
		if f.Number() != num || f.Kind() != protoreflect.StringKind || f.IsList() {
			t.Errorf("%s = number %d kind %v, want string = %d", name, f.Number(), f.Kind(), num)
		}
	}
	if req.Fields().Len() != 2 {
		t.Errorf("HideReviewRequest has %d fields, want exactly 2 (no actor field)", req.Fields().Len())
	}
	if n := msg(t, fd, "HideReviewResponse").Fields().Len(); n != 0 {
		t.Errorf("HideReviewResponse has %d fields, want 0", n)
	}
}

func TestHideReviewRequest_RoundTripBoundaryReason_Scenario_RoundTripOfABoundaryReason(t *testing.T) {
	fd := loadFile(t)
	req := msg(t, fd, "HideReviewRequest")
	for name, reason := range map[string]string{
		"500 chars": strings.Repeat("a", 500),
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			m := dynamicpb.NewMessage(req)
			m.Set(field(t, req, "review_id"), protoreflect.ValueOfString("rev-1"))
			m.Set(field(t, req, "reason"), protoreflect.ValueOfString(reason))
			b, err := proto.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			got := dynamicpb.NewMessage(req)
			if err := proto.Unmarshal(b, got); err != nil {
				t.Fatal(err)
			}
			if got.Get(field(t, req, "reason")).String() != reason {
				t.Error("reason changed in round trip")
			}
			if got.Get(field(t, req, "review_id")).String() != "rev-1" {
				t.Error("review_id changed in round trip")
			}
		})
	}
}

func TestExistingMessages_WireShapeUnchanged_Scenario_ExistingMessagesKeepTheirWireShape(t *testing.T) {
	fd := loadFile(t)
	cases := []struct {
		msg   protoreflect.Name
		field protoreflect.Name
		num   protoreflect.FieldNumber
		kind  protoreflect.Kind
	}{
		{"CreateReviewRequest", "order_id", 4, protoreflect.StringKind},
		{"Review", "order_id", 5, protoreflect.StringKind},
		{"Review", "verified_purchase", 11, protoreflect.BoolKind},
		{"GetListingRatingSummaryResponse", "review_count", 3, protoreflect.Int64Kind},
		{"GetShopRatingSummaryResponse", "review_count", 3, protoreflect.Int64Kind},
	}
	for _, c := range cases {
		f := field(t, msg(t, fd, c.msg), c.field)
		if f.Number() != c.num || f.Kind() != c.kind {
			t.Errorf("%s.%s = number %d kind %v, want %d %v", c.msg, c.field, f.Number(), f.Kind(), c.num, c.kind)
		}
	}
	// No field added or removed on the unchanged messages.
	counts := map[protoreflect.Name]int{"Review": 11}
	for n, want := range counts {
		if got := msg(t, fd, n).Fields().Len(); got != want {
			t.Errorf("%s has %d fields, want %d", n, got, want)
		}
	}
}

func TestRuleComments_Scenario_RuleCommentsArePresent(t *testing.T) {
	fd := loadFile(t)
	cases := []struct {
		msg, field protoreflect.Name
		text       string
	}{
		{"CreateReviewRequest", "order_id", "required: a COMPLETED order of the caller containing listing_id"},
		{"Review", "verified_purchase", "true for every review created after q2; false = legacy, excluded from ratings"},
		{"GetListingRatingSummaryResponse", "review_count", "verified, non-hidden reviews"},
		{"GetShopRatingSummaryResponse", "review_count", "verified, non-hidden reviews"},
	}
	for _, c := range cases {
		f := field(t, msg(t, fd, c.msg), c.field)
		loc := fd.SourceLocations().ByDescriptor(f)
		got := norm(loc.LeadingComments + " " + loc.TrailingComments)
		if !strings.Contains(got, c.text) {
			t.Errorf("%s.%s comment missing %q; got %q", c.msg, c.field, c.text, got)
		}
	}
}

func TestHideReview_DocComment_Scenarios_ErrorContractAndDistinctAuthzCodes(t *testing.T) {
	fd := loadFile(t)
	m := hideReviewMethod(t, fd)
	loc := fd.SourceLocations().ByDescriptor(m)
	c := norm(loc.LeadingComments)
	if c == "" {
		t.Fatal("HideReview has no leading doc comment")
	}
	lc := strings.ToLower(c)
	for _, w := range []string{
		"admin-only", "Unauthenticated", "PermissionDenied", "NotFound", "InvalidArgument",
		"idempotent", "Unavailable", "Internal", "anonymous", "non-admin", "review_id", "500",
		"audit", "visible",
	} {
		if !strings.Contains(lc, strings.ToLower(w)) {
			t.Errorf("HideReview comment missing %q; got %q", w, c)
		}
	}
	// Distinct codes: Unauthenticated must be tied to anonymous, PermissionDenied to non-admin.
	if i, j := strings.Index(c, "Unauthenticated"), strings.Index(c, "PermissionDenied"); i < 0 || j < 0 || i == j {
		t.Error("Unauthenticated and PermissionDenied must both appear, distinctly")
	}
}

func TestConsumerCopies_Scenarios_CopiesMatchTheSource(t *testing.T) {
	src, err := os.ReadFile(engagementProto_path("../../.."))
	if err != nil {
		t.Fatal(err)
	}
	root := "../../../../../.." // repo root
	for _, team := range []string{"team-engagement", "team-gateway", "team-frontend"} {
		p := filepath.Join(root, team, "proto", engagementProto)
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", team, err)
			continue
		}
		if !bytes.Equal(src, got) {
			t.Errorf("%s copy of engagement.proto differs from platform-core source", team)
		}
	}
}

func engagementProto_path(modRoot string) string { return filepath.Join(modRoot, engagementProto) }
