package analyticsv1_test

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	aiPkg      = "platform.analytics.v1."
	aiMsgPfx   = "message:" + aiPkg
	aiTSKind   = "message:google.protobuf.Timestamp"
	aiExposure = "EVENT_TYPE_EXPERIMENT_EXPOSURE"
)

var aiBaselineEventTypes = []string{
	"EVENT_TYPE_UNSPECIFIED", "EVENT_TYPE_VIEW", "EVENT_TYPE_CLICK", "EVENT_TYPE_ADD_TO_CART",
	"EVENT_TYPE_IMPRESSION", "EVENT_TYPE_REMOVE_FROM_CART", "EVENT_TYPE_BEGIN_CHECKOUT",
	"EVENT_TYPE_APPLY_PROMOTION", "EVENT_TYPE_SEARCH_FILTER", "EVENT_TYPE_FAVORITE",
	"EVENT_TYPE_SHARE", "EVENT_TYPE_VIEW_CART", "EVENT_TYPE_ADD_SHIPPING_INFO",
	"EVENT_TYPE_ADD_PAYMENT_INFO", "EVENT_TYPE_PURCHASE", "EVENT_TYPE_SIGN_UP", "EVENT_TYPE_LOGIN",
	"EVENT_TYPE_FOLLOW_SELLER", "EVENT_TYPE_REFERRAL_REDEEM", "EVENT_TYPE_DAILY_CHECK_IN",
	"EVENT_TYPE_ALERT_SUBSCRIBE", "EVENT_TYPE_AI_INTERACTION",
}

var aiBaselineForecast = map[string]string{
	"seller_id": "1 string", "listing_id": "2 string",
	"daily_forecasts":         "3 repeated message:" + aiPkg + "DailyForecast",
	"suggested_reorder_point": "4 double", "safety_stock": "5 double", "model_version": "6 string", "is_cold_start": "7 bool",
}

var aiBaselineMessages = map[string]map[string]string{
	"GetChannelFunnelRequest": {"seller_id": "1 string", "from": "2 " + aiTSKind, "to": "3 " + aiTSKind},
	"ChannelFunnelRow": {"source": "1 string", "medium": "2 string", "campaign": "3 string", "impressions": "4 int64",
		"views": "5 int64", "adds": "6 int64", "begin_checkouts": "7 int64", "purchases": "8 int64"},
	"GetChannelFunnelResponse": {"rows": "1 repeated " + aiMsgPfx + "ChannelFunnelRow"},
	"ConsentState":             {"analytics": "1 bool", "marketing": "2 bool", "personalization": "3 bool", "version": "4 int32"},
	"GetSellerFunnelRequest":   {"seller_id": "1 string", "from": "2 " + aiTSKind, "to": "3 " + aiTSKind},
	"GetSellerFunnelResponse": {"impressions": "1 int64", "views": "2 int64", "adds": "3 int64", "orders": "4 int64",
		"begin_checkouts": "5 int64", "purchases": "6 int64"},
	"DayRevenue":                 {"day": "1 string", "revenue": "2 int64", "order_count": "3 int64"},
	"TopSku":                     {"sku": "1 string", "listing_id": "2 string", "revenue": "3 int64", "units_sold": "4 int64"},
	"GetRevenueBreakdownRequest": {"seller_id": "1 string", "from": "2 " + aiTSKind, "to": "3 " + aiTSKind},
	"GetRevenueBreakdownResponse": {"days": "1 repeated " + aiMsgPfx + "DayRevenue",
		"top_skus": "2 repeated " + aiMsgPfx + "TopSku"},
	"DailyForecast": {"date": "1 string", "p10": "2 double", "p50": "3 double", "p90": "4 double"},
	"GetDemandForecastRequest": {"seller_id": "1 string", "listing_id": "2 string", "horizon_days": "3 int32",
		"lead_time_days": "4 int32", "service_level": "5 double"},
	"GetPlatformOrderSummaryRequest":  {"window": "1 message:google.protobuf.Duration"},
	"GetPlatformOrderSummaryResponse": {"order_count": "1 int64", "gmv": "2 int64"},
	"ListRecentOrdersRequest":         {"limit": "1 int32"},
	"RecentOrder":                     {"order_id": "1 string", "seller_id": "2 string", "total": "3 int64", "paid_at": "4 " + aiTSKind},
	"ListRecentOrdersResponse":        {"orders": "1 repeated " + aiMsgPfx + "RecentOrder"},
	"GetTrackingQualityReportRequest": {"window_hours": "1 uint32"},
	"TrackingTypeQuality": {"event_type": "1 string", "events": "2 int64", "visitors": "3 int64",
		"missing_listing_ratio": "4 double", "listing_scoped": "5 bool"},
	"GetTrackingQualityReportResponse": {"types": "1 repeated " + aiMsgPfx + "TrackingTypeQuality",
		"lag_p50_seconds": "2 double", "lag_p95_seconds": "3 double", "last_ingested_at": "4 " + aiTSKind,
		"decode_failures": "5 int64", "duplicates_skipped": "6 int64", "status": "7 string",
		"reasons": "8 repeated string", "window_hours": "9 uint32"},
	"GetRecommendationPerformanceRequest": dsBaselinePerfRequest,
	"GetSearchPerformanceRequest":         {"window_hours": "1 uint32"},
	"GetSearchPerformanceResponse": {"rows": "1 repeated " + aiMsgPfx + "RecommendationPerformanceRow",
		"window_hours": "2 uint32", "attribution_window_hours": "3 uint32"},
	"RecommendationPerformanceRow":         dsBaselinePerfRow,
	"PlacementFallbackShare":               {"placement_id": "1 string", "fallback_share": "2 double"},
	"GetRecommendationPerformanceResponse": dsBaselinePerfResponse,
	"GetDemandForecastResponse":            aiBaselineForecast,
}

var aiBaselineRpcs = map[string][2]string{
	"GetSellerFunnel":              {"GetSellerFunnelRequest", "GetSellerFunnelResponse"},
	"GetRevenueBreakdown":          {"GetRevenueBreakdownRequest", "GetRevenueBreakdownResponse"},
	"GetDemandForecast":            {"GetDemandForecastRequest", "GetDemandForecastResponse"},
	"GetPlatformOrderSummary":      {"GetPlatformOrderSummaryRequest", "GetPlatformOrderSummaryResponse"},
	"ListRecentOrders":             {"ListRecentOrdersRequest", "ListRecentOrdersResponse"},
	"GetTrackingQualityReport":     {"GetTrackingQualityReportRequest", "GetTrackingQualityReportResponse"},
	"GetRecommendationPerformance": {"GetRecommendationPerformanceRequest", "GetRecommendationPerformanceResponse"},
	"GetChannelFunnel":             {"GetChannelFunnelRequest", "GetChannelFunnelResponse"},
	"GetSearchPerformance":         {"GetSearchPerformanceRequest", "GetSearchPerformanceResponse"},
}

var aiNewRpcs = map[string][2]string{
	"GetRetentionCohorts":  {"GetRetentionCohortsRequest", "GetRetentionCohortsResponse"},
	"GetExperimentReadout": {"GetExperimentReadoutRequest", "GetExperimentReadoutResponse"},
	"GetAiSurfaceReport":   {"GetAiSurfaceReportRequest", "GetAiSurfaceReportResponse"},
}

var aiNewMessages = map[string]map[string]string{
	"GetRetentionCohortsRequest": {"from": "1 " + aiTSKind, "weeks": "2 uint32"},
	"RetentionCohort": {"cohort_week": "1 string", "cohort_size": "2 int64", "d1": "3 double", "d7": "4 double",
		"d30": "5 double", "weekly_active": "6 repeated double", "repeat_purchase_30d": "7 double"},
	"GetRetentionCohortsResponse": {"cohorts": "1 repeated " + aiMsgPfx + "RetentionCohort"},
	"GetExperimentReadoutRequest": {"flag": "1 string", "from": "2 " + aiTSKind, "to": "3 " + aiTSKind},
	"ExperimentVariantRow": {"variant": "1 string", "exposed": "2 int64", "converted": "3 int64", "rate": "4 double",
		"ci_low": "5 double", "ci_high": "6 double"},
	"GetExperimentReadoutResponse": {"variants": "1 repeated " + aiMsgPfx + "ExperimentVariantRow", "window_days": "2 uint32"},
	"GetAiSurfaceReportRequest":    {"from": "1 " + aiTSKind, "to": "2 " + aiTSKind},
	"AiSurfaceRow": {"surface": "1 string", "model_version": "2 string", "shown": "3 int64", "accepted": "4 int64",
		"edited": "5 int64", "dismissed": "6 int64"},
	"GetAiSurfaceReportResponse": {"rows": "1 repeated " + aiMsgPfx + "AiSurfaceRow"},
}

func aiEnum(t *testing.T, fd protoreflect.FileDescriptor) protoreflect.EnumDescriptor {
	t.Helper()
	e := fd.Enums().ByName("EventType")
	if e == nil {
		t.Fatalf("enum EventType missing")
	}
	return e
}

func TestAiData_ExposureEventTypeIsTwentyTwo(t *testing.T) { // scenario: Exposure value present at 22
	fd, _ := dsFile(t, dsAnalyticsProto)
	e := aiEnum(t, fd)
	v := e.Values().ByName(aiExposure)
	if v == nil {
		t.Fatalf("%s missing from EventType", aiExposure)
	}
	if v.Number() != 22 {
		t.Errorf("%s = %d, want 22", aiExposure, v.Number())
	}
	if e.Values().Len() != 23 {
		t.Errorf("EventType has %d values, want 23", e.Values().Len())
	}
}

func TestAiData_ExistingEventTypesUnchanged(t *testing.T) { // scenario: Existing event types keep their numbers
	fd, _ := dsFile(t, dsAnalyticsProto)
	e := aiEnum(t, fd)
	for n, name := range aiBaselineEventTypes {
		v := e.Values().ByNumber(protoreflect.EnumNumber(n))
		if v == nil || string(v.Name()) != name {
			t.Errorf("EventType %d: got %v, want %s", n, v, name)
		}
	}
}

func TestAiData_ExposureDocumented(t *testing.T) { // scenario: Exposure properties are documented as allowlisted
	fd, _ := dsFile(t, dsAnalyticsProto)
	v := aiEnum(t, fd).Values().ByName(aiExposure)
	if v == nil {
		t.Fatalf("%s missing from EventType", aiExposure)
	}
	dsCheck(t, aiExposure, dsComment(fd, v), "feature-flag", "variant", "properties", "flag", "no free text")
}

func TestAiData_OldSchemaParsesExposure(t *testing.T) { // scenario: Old consumer meets the new event type
	fd, files := dsFile(t, dsAnalyticsProto)
	te := dsMsg(t, fd, "TrackingEvent")
	cur := dynamicpb.NewMessage(te)
	cur.Set(dsFld(t, te, "event_type"), protoreflect.ValueOfEnum(22))
	dsSet(t, cur, "listing_id", "l-1")
	dsSet(t, cur, "session_id", "s-1")
	old := dsOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		for _, e := range p.EnumType {
			if e.GetName() == "EventType" {
				var vs []*descriptorpb.EnumValueDescriptorProto
				for _, v := range e.Value {
					if v.GetNumber() != 22 {
						vs = append(vs, v)
					}
				}
				e.Value = vs
			}
		}
	})
	got := dsParse(t, cur, dsMsg(t, old, "TrackingEvent"))
	f := got.Descriptor().Fields().ByName("event_type")
	if n := got.Get(f).Enum(); n != 22 {
		t.Errorf("old schema reads event_type %d, want 22", n)
	}
	if old.Enums().ByName("EventType").Values().ByNumber(22) != nil {
		t.Errorf("old schema unexpectedly defines 22")
	}
	if dsStr(got, "listing_id") != "l-1" || dsStr(got, "session_id") != "s-1" {
		t.Errorf("other fields changed: %v", got)
	}
}

func TestAiData_NoUndefinedEventNumbers(t *testing.T) { // scenario: Undefined event number is not a defined value
	fd, _ := dsFile(t, dsAnalyticsProto)
	if v := aiEnum(t, fd).Values().ByNumber(23); v != nil {
		t.Errorf("EventType defines 23 (%s) without a spec change", v.Name())
	}
}

func TestAiData_ForecastGeneratedAtShape(t *testing.T) { // scenario: generated_at present at 8
	fd, _ := dsFile(t, dsAnalyticsProto)
	want := map[string]string{"generated_at": "8 " + aiTSKind}
	for k, v := range aiBaselineForecast {
		want[k] = v
	}
	dsShape(t, fd, "GetDemandForecastResponse", want)
}

func TestAiData_GeneratedAtPresenceRoundTrip(t *testing.T) { // scenario: Unset run time on a cold start is distinct from a set one
	fd, _ := dsFile(t, dsAnalyticsProto)
	md := dsMsg(t, fd, "GetDemandForecastResponse")
	gf := dsFld(t, md, "generated_at")
	cold := dsNew(t, md, "seller_id", "s")
	cold.Set(dsFld(t, md, "is_cold_start"), protoreflect.ValueOfBool(true))
	warm := dsNew(t, md, "seller_id", "s", "generated_at", dsTimestamp(gf, 1700000000))
	gotCold := dsRoundEqual(t, cold)
	gotWarm := dsRoundEqual(t, warm)
	if gotCold.Has(gf) {
		t.Errorf("cold start reads generated_at present")
	}
	if !gotWarm.Has(gf) {
		t.Fatalf("warm response lost generated_at")
	}
	if s := gotWarm.Get(gf).Message().Get(gf.Message().Fields().ByName("seconds")).Int(); s != 1700000000 {
		t.Errorf("generated_at seconds = %d", s)
	}
	bc, _ := proto.Marshal(cold)
	bw, _ := proto.Marshal(warm)
	if bytes.Equal(bc, bw) {
		t.Errorf("cold and warm responses serialise identically")
	}
}

func TestAiData_GeneratedAtDocumented(t *testing.T) { // scenario: Cold-start semantics are documented
	fd, _ := dsFile(t, dsAnalyticsProto)
	dsCheck(t, "GetDemandForecastResponse.generated_at", dsFieldComment(t, fd, "GetDemandForecastResponse", "generated_at"),
		"run time", "unset when is_cold_start is true")
}

func TestAiData_ForecastExistingFieldsUnchanged(t *testing.T) { // scenario: Forecast fields 1 to 7 are not renumbered
	fd, _ := dsFile(t, dsAnalyticsProto)
	dsKeepsBaseline(t, fd, "GetDemandForecastResponse", aiBaselineForecast)
}

func aiRpc(t *testing.T, fd protoreflect.FileDescriptor, name string, io [2]string) protoreflect.MethodDescriptor {
	t.Helper()
	m := dsMethod(t, fd, name)
	if string(m.Input().Name()) != io[0] || string(m.Output().Name()) != io[1] {
		t.Errorf("%s: %s -> %s, want %s -> %s", name, m.Input().Name(), m.Output().Name(), io[0], io[1])
	}
	if m.IsStreamingClient() || m.IsStreamingServer() {
		t.Errorf("%s must not stream", name)
	}
	return m
}

func TestAiData_ReportRpcsShape(t *testing.T) { // scenario: Three RPCs present with their message types
	fd, _ := dsFile(t, dsAnalyticsProto)
	for name, io := range aiNewRpcs {
		aiRpc(t, fd, name, io)
	}
}

func TestAiData_ReportMessagesShape(t *testing.T) { // scenario: Message fields sit at the specified numbers
	fd, _ := dsFile(t, dsAnalyticsProto)
	for name, want := range aiNewMessages {
		dsShape(t, fd, name, want)
	}
}

func TestAiData_ExistingQuerySurfaceUntouched(t *testing.T) { // scenario: Existing query surface is untouched
	fd, _ := dsFile(t, dsAnalyticsProto)
	for name, io := range aiBaselineRpcs {
		aiRpc(t, fd, name, io)
	}
	for name, base := range aiBaselineMessages {
		dsKeepsBaseline(t, fd, name, base)
	}
	sd := fd.Services().ByName("AnalyticsQueryService")
	if got, want := sd.Methods().Len(), len(aiBaselineRpcs)+len(aiNewRpcs); got != want {
		t.Errorf("AnalyticsQueryService has %d rpcs, want %d", got, want)
	}
}

func TestAiData_ReportRpcsDocumentedAdminOnly(t *testing.T) { // scenario: Admin-only and aggregate-only are documented
	fd, _ := dsFile(t, dsAnalyticsProto)
	for name := range aiNewRpcs {
		dsCheck(t, "rpc "+name, dsComment(fd, dsMethod(t, fd, name)), `scope "admin"`, "no ids")
	}
}

func TestAiData_ReportSemanticsDocumented(t *testing.T) { // scenario: Window and interval semantics are documented
	fd, _ := dsFile(t, dsAnalyticsProto)
	dsCheck(t, "GetRetentionCohortsRequest.weeks", dsFieldComment(t, fd, "GetRetentionCohortsRequest", "weeks"),
		"1..26", "0 = 12", "INVALID_ARGUMENT")
	dsCheck(t, "RetentionCohort", dsComment(fd, dsMsg(t, fd, "RetentionCohort")), "UTC Monday", "0..1")
	dsCheck(t, "ExperimentVariantRow", dsComment(fd, dsMsg(t, fd, "ExperimentVariantRow")), "95% Wilson", "0..1")
	dsCheck(t, "GetExperimentReadoutResponse.window_days", dsFieldComment(t, fd, "GetExperimentReadoutResponse", "window_days"),
		"paid order", "7 days", "first exposure")
}

func TestAiData_EmptyReportsRoundTrip(t *testing.T) { // scenario: Empty report is a valid response
	fd, _ := dsFile(t, dsAnalyticsProto)
	for name, list := range map[string]string{
		"GetRetentionCohortsResponse":  "cohorts",
		"GetExperimentReadoutResponse": "variants",
		"GetAiSurfaceReportResponse":   "rows",
	} {
		md := dsMsg(t, fd, name)
		got := dsRoundEqual(t, dynamicpb.NewMessage(md))
		if n := got.Get(dsFld(t, md, list)).List().Len(); n != 0 {
			t.Errorf("%s.%s has %d entries, want 0", name, list, n)
		}
	}
}

func TestAiData_WireCompatible(t *testing.T) { // scenario: Wire compatible both ways
	fd, files := dsFile(t, dsAnalyticsProto)
	old := dsOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		dsDropFields(p, "GetDemandForecastResponse", func(n int32) bool { return n <= 7 })
	})
	oldMd := dsMsg(t, old, "GetDemandForecastResponse")
	newMd := dsMsg(t, fd, "GetDemandForecastResponse")
	oldMsg := dsNew(t, oldMd, "seller_id", "s", "listing_id", "l", "suggested_reorder_point", 4.5, "safety_stock", 1.5, "model_version", "v1")
	fromOld := dsParse(t, oldMsg, newMd)
	if dsStr(fromOld, "seller_id") != "s" || dsFloat(fromOld, "safety_stock") != 1.5 || dsStr(fromOld, "model_version") != "v1" {
		t.Errorf("new schema misread old bytes: %v", fromOld)
	}
	gf := dsFld(t, newMd, "generated_at")
	newMsg := dsNew(t, newMd, "seller_id", "s", "listing_id", "l", "suggested_reorder_point", 4.5,
		"safety_stock", 1.5, "model_version", "v1", "generated_at", dsTimestamp(gf, 1700000000))
	fromNew := dsParse(t, newMsg, oldMd)
	if dsStr(fromNew, "seller_id") != "s" || dsFloat(fromNew, "suggested_reorder_point") != 4.5 || dsStr(fromNew, "model_version") != "v1" {
		t.Errorf("old schema misread new bytes: %v", fromNew)
	}
	sd := dsMsg(t, fd, "GetSellerFunnelResponse")
	osd := dsMsg(t, old, "GetSellerFunnelResponse")
	sm := dsNew(t, osd, "impressions", int64(9), "purchases", int64(2))
	got := dsParse(t, sm, sd)
	if dsInt(got, "impressions") != 9 || dsInt(got, "purchases") != 2 {
		t.Errorf("funnel bytes changed: %v", got)
	}
}

func TestAiData_ReportsCarryNoPersonalData(t *testing.T) { // scenario: The report messages carry no personal data
	fd, _ := dsFile(t, dsAnalyticsProto)
	for name := range aiNewMessages {
		md := dsMsg(t, fd, name)
		dsNoFields(t, md, "user_id", "anonymous_id", "session_id", "email", "phone", "ip")
		for i := 0; i < md.Fields().Len(); i++ {
			n := string(md.Fields().Get(i).Name())
			if strings.Contains(n, "email") || strings.Contains(n, "phone") {
				t.Errorf("%s.%s looks personal", name, n)
			}
		}
	}
}
