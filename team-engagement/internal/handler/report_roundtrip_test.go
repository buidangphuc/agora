package handler_test

// Round-trip guard for the vendored Report contract (openspec change
// engagement-listing-reports, task 6). Mirrors hidereview_roundtrip_test.go:
// the vendored proto must equal the platform-core source and the generated
// Report / enums must keep the contract wire shape.

import (
	"bytes"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

func TestReportContract_VendoredProtoMatchesSource_Scenario_ReportsVendoredCopyHonest(t *testing.T) {
	const rel = "platform/engagement/v1/engagement.proto"
	src, err := os.ReadFile("../../../platform-core/packages/proto/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../proto/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, got) {
		t.Error("team-engagement vendored engagement.proto differs from platform-core source")
	}
}

func TestReportContract_FieldNumbers_Scenario_ReportsVendoredCopyHonest(t *testing.T) {
	md := (&engagementv1.Report{}).ProtoReflect().Descriptor()
	want := map[protoreflect.Name]struct {
		num  protoreflect.FieldNumber
		kind protoreflect.Kind
	}{
		"id": {1, protoreflect.StringKind}, "target_type": {2, protoreflect.EnumKind},
		"target_id": {3, protoreflect.StringKind}, "reason_code": {4, protoreflect.StringKind},
		"note": {5, protoreflect.StringKind}, "reporter_id": {6, protoreflect.StringKind},
		"status": {7, protoreflect.EnumKind}, "created_at": {8, protoreflect.MessageKind},
		"resolved_at": {9, protoreflect.MessageKind}, "resolution_note": {10, protoreflect.StringKind},
	}
	if md.Fields().Len() != len(want) {
		t.Errorf("Report has %d fields, want %d", md.Fields().Len(), len(want))
	}
	for name, w := range want {
		f := md.Fields().ByName(name)
		if f == nil || f.Number() != w.num || f.Kind() != w.kind {
			t.Errorf("Report.%s mismatch: %v", name, f)
		}
	}
}

func TestReportContract_EnumValues_Scenario_ReportsVendoredCopyHonest(t *testing.T) {
	st := map[engagementv1.ReportStatus]int32{
		engagementv1.ReportStatus_REPORT_STATUS_UNSPECIFIED: 0,
		engagementv1.ReportStatus_REPORT_STATUS_OPEN:        1,
		engagementv1.ReportStatus_REPORT_STATUS_ACTIONED:    2,
		engagementv1.ReportStatus_REPORT_STATUS_DISMISSED:   3,
	}
	for k, v := range st {
		if int32(k) != v {
			t.Errorf("%s = %d, want %d", k, k, v)
		}
	}
	if engagementv1.ReportStatus(0).Descriptor().Values().Len() != 4 {
		t.Error("ReportStatus must have exactly 4 values")
	}
	if engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING != 1 ||
		engagementv1.ReportTargetType(0).Descriptor().Values().Len() != 2 {
		t.Error("ReportTargetType must be UNSPECIFIED=0, LISTING=1")
	}
}

func TestReportContract_RoundTrip_Scenario_ReportsVendoredCopyHonest(t *testing.T) {
	in := &engagementv1.Report{
		Id: "r1", TargetType: engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING,
		TargetId: "l1", ReasonCode: "spam", Note: "n", ReporterId: "u1",
		Status:         engagementv1.ReportStatus_REPORT_STATUS_DISMISSED,
		CreatedAt:      timestamppb.New(timestamppb.Now().AsTime()),
		ResolvedAt:     timestamppb.Now(),
		ResolutionNote: "ok",
	}
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	out := &engagementv1.Report{}
	if err := proto.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(in, out) {
		t.Error("Report changed in round trip")
	}
}

func TestReportContract_RPCs_Scenario_ReportsVendoredCopyHonest(t *testing.T) {
	svc := engagementv1.File_platform_engagement_v1_engagement_proto.Services().ByName("EngagementService")
	for name, io := range map[protoreflect.Name][2]protoreflect.Name{
		"CreateReport":  {"CreateReportRequest", "CreateReportResponse"},
		"ListReports":   {"ListReportsRequest", "ListReportsResponse"},
		"ResolveReport": {"ResolveReportRequest", "ResolveReportResponse"},
	} {
		m := svc.Methods().ByName(name)
		if m == nil || m.Input().Name() != io[0] || m.Output().Name() != io[1] {
			t.Errorf("%s signature mismatch", name)
		}
	}
}
