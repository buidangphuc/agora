package rpc_test

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/rpc"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Wednesday; its UTC Monday is 2026-11-09.
var rpNow = time.Date(2026, 11, 11, 15, 30, 0, 0, time.UTC)

type reportsRepo struct {
	spyRepo
	retention    []query.RetentionCohortRow
	experiment   []query.ExperimentRow
	ai           []query.AiSurfaceRow
	reportErr    error
	retCalls     int
	expCalls     int
	aiCalls      int
	lastFrom     time.Time
	lastTo       time.Time
	lastWeeks    int
	lastNow      time.Time
	lastFlag     string
	lastWindowDy int
}

func (r *reportsRepo) RetentionCohorts(_ context.Context, from time.Time, weeks int, now time.Time) ([]query.RetentionCohortRow, error) {
	r.retCalls++
	r.lastFrom, r.lastWeeks, r.lastNow = from, weeks, now
	return r.retention, r.reportErr
}

func (r *reportsRepo) ExperimentReadout(_ context.Context, flag string, from, to time.Time, windowDays int) ([]query.ExperimentRow, error) {
	r.expCalls++
	r.lastFlag, r.lastFrom, r.lastTo, r.lastWindowDy = flag, from, to, windowDays
	return r.experiment, r.reportErr
}

func (r *reportsRepo) AiSurfaceReport(_ context.Context, from, to time.Time) ([]query.AiSurfaceRow, error) {
	r.aiCalls++
	r.lastFrom, r.lastTo = from, to
	return r.ai, r.reportErr
}

func (r *reportsRepo) total() int { return r.retCalls + r.expCalls + r.aiCalls }

func rpHandler(repo query.Repository) *rpc.Handler {
	return mustHandler(repo, rpc.WithClock(func() time.Time { return rpNow }))
}

var rpAdmin = func() context.Context { return asPrincipal("admin-1", "user", "admin") }

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (err=%v), want %v", got, err, want)
	}
}

// Scenario: Default and maximum weeks
func TestRetention_WeeksDefaultAndBoundary(t *testing.T) {
	cases := []struct {
		weeks    uint32
		wantN    int
		wantBack int
	}{{0, 12, 12 * 7}, {26, 26, 26 * 7}}
	monday := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	for _, c := range cases {
		repo := &reportsRepo{}
		_, err := rpHandler(repo).GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{Weeks: c.weeks})
		if err != nil {
			t.Fatalf("weeks=%d: %v", c.weeks, err)
		}
		if repo.retCalls != 1 || repo.lastWeeks != c.wantN {
			t.Errorf("weeks=%d: calls=%d weeks passed=%d, want 1 and %d", c.weeks, repo.retCalls, repo.lastWeeks, c.wantN)
		}
		if want := monday.AddDate(0, 0, -c.wantBack); !repo.lastFrom.Equal(want) {
			t.Errorf("weeks=%d: from=%v, want %v", c.weeks, repo.lastFrom, want)
		}
		if !repo.lastNow.Equal(rpNow) {
			t.Errorf("weeks=%d: now=%v, want the handler clock %v", c.weeks, repo.lastNow, rpNow)
		}
	}
}

// Scenario: Default and maximum weeks (an explicit from is truncated to its UTC Monday)
func TestRetention_WeeksDefaultAndBoundary_FromTruncatedToMonday(t *testing.T) {
	repo := &reportsRepo{}
	from := time.Date(2026, 10, 14, 13, 0, 0, 0, time.UTC)
	_, err := rpHandler(repo).GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{From: timestamppb.New(from), Weeks: 4})
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC); !repo.lastFrom.Equal(want) {
		t.Errorf("from = %v, want %v", repo.lastFrom, want)
	}
}

// Scenario: Default and maximum weeks (ratios derived from counts)
func TestRetention_WeeksDefaultAndBoundary_RatiosFromCounts(t *testing.T) {
	repo := &reportsRepo{retention: []query.RetentionCohortRow{{
		CohortWeek: "2026-08-03", CohortSize: 4,
		D1Active: 2, D1Eligible: 4, D7Active: 1, D7Eligible: 4, D30Active: 0, D30Eligible: 0,
		WeeklyActive: []int64{4, 2}, RepeatBuyers: 1, RepeatEligible: 2,
	}}}
	resp, err := rpHandler(repo).GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetCohorts()) != 1 {
		t.Fatalf("cohorts = %v", resp.GetCohorts())
	}
	c := resp.GetCohorts()[0]
	if c.GetCohortWeek() != "2026-08-03" || c.GetCohortSize() != 4 {
		t.Errorf("cohort = %v", c)
	}
	if c.GetD1() != 0.5 || c.GetD7() != 0.25 || c.GetD30() != 0 || c.GetRepeatPurchase_30D() != 0.5 {
		t.Errorf("ratios d1=%v d7=%v d30=%v repeat=%v, want 0.5 0.25 0 0.5", c.GetD1(), c.GetD7(), c.GetD30(), c.GetRepeatPurchase_30D())
	}
	if wa := c.GetWeeklyActive(); len(wa) != 2 || wa[0] != 1 || wa[1] != 0.5 {
		t.Errorf("weekly_active = %v, want [1 0.5]", wa)
	}
}

// Scenario: Weeks above 26 is rejected
func TestRetention_WeeksAboveMaxRejected(t *testing.T) {
	repo := &reportsRepo{}
	_, err := rpHandler(repo).GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{Weeks: 27})
	wantCode(t, err, codes.InvalidArgument)
	if !strings.Contains(err.Error(), "weeks") {
		t.Errorf("message %q does not name weeks", err.Error())
	}
	if repo.total() != 0 {
		t.Errorf("repository queried %d times, want 0", repo.total())
	}
}

// Scenario: Non-admin cannot read retention / a readout / the AI report
func TestReports_NonAdminDenied(t *testing.T) {
	ctxs := map[string]context.Context{
		"seller":  asPrincipal("seller-1", "user", "seller"),
		"no-auth": context.Background(),
	}
	for name, ctx := range ctxs {
		repo := &reportsRepo{}
		h := rpHandler(repo)
		_, e1 := h.GetRetentionCohorts(ctx, &analyticsv1.GetRetentionCohortsRequest{Weeks: 99})
		_, e2 := h.GetExperimentReadout(ctx, &analyticsv1.GetExperimentReadoutRequest{Flag: ""})
		_, e3 := h.GetAiSurfaceReport(ctx, &analyticsv1.GetAiSurfaceReportRequest{
			From: timestamppb.New(rpNow), To: timestamppb.New(rpNow.Add(-time.Hour)),
		})
		for i, err := range []error{e1, e2, e3} {
			if status.Code(err) == codes.OK || status.Code(err) == codes.InvalidArgument || status.Code(err) == codes.Unimplemented {
				t.Errorf("%s rpc#%d: code = %v, want PermissionDenied/Unauthenticated before validation", name, i, status.Code(err))
			}
		}
		if repo.total() != 0 {
			t.Errorf("%s: repository queried %d times", name, repo.total())
		}
	}
	repo := &reportsRepo{}
	_, err := rpHandler(repo).GetRetentionCohorts(asPrincipal("seller-1", "user", "seller"), &analyticsv1.GetRetentionCohortsRequest{})
	wantCode(t, err, codes.PermissionDenied)
	_, err = rpHandler(repo).GetExperimentReadout(asPrincipal("seller-1", "user", "seller"), &analyticsv1.GetExperimentReadoutRequest{Flag: "f"})
	wantCode(t, err, codes.PermissionDenied)
	_, err = rpHandler(repo).GetAiSurfaceReport(asPrincipal("seller-1", "user", "seller"), &analyticsv1.GetAiSurfaceReportRequest{})
	wantCode(t, err, codes.PermissionDenied)
}

// Scenario: Warehouse without the capability is unavailable
func TestReports_UnsupportedIsUnavailable(t *testing.T) {
	bare := &spyRepo{}
	h := rpHandler(bare)
	r1, e1 := h.GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{})
	r2, e2 := h.GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: "f"})
	r3, e3 := h.GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{})
	wantCode(t, e1, codes.Unavailable)
	wantCode(t, e2, codes.Unavailable)
	wantCode(t, e3, codes.Unavailable)
	if len(r1.GetCohorts()) != 0 || len(r2.GetVariants()) != 0 || len(r3.GetRows()) != 0 {
		t.Errorf("responses carry data on failure: %v %v %v", r1, r2, r3)
	}

	failing := &reportsRepo{reportErr: query.ErrUnsupported}
	h = rpHandler(failing)
	_, e1 = h.GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{})
	_, e2 = h.GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: "f"})
	_, e3 = h.GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{})
	wantCode(t, e1, codes.Unavailable)
	wantCode(t, e2, codes.Unavailable)
	wantCode(t, e3, codes.Unavailable)
}

func wilsonVia(t *testing.T, exposed, converted int64) *analyticsv1.ExperimentVariantRow {
	t.Helper()
	repo := &reportsRepo{experiment: []query.ExperimentRow{{Variant: "a", Exposed: exposed, Converted: converted}}}
	resp, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: "f"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetVariants()) != 1 {
		t.Fatalf("variants = %v", resp.GetVariants())
	}
	return resp.GetVariants()[0]
}

func near4(got, want float64) bool { return math.Abs(got-want) < 0.00005 }

// Scenario: Wilson interval for a known sample
func TestWilson_KnownValues(t *testing.T) {
	v := wilsonVia(t, 100, 20)
	if v.GetExposed() != 100 || v.GetConverted() != 20 || !near4(v.GetRate(), 0.2) {
		t.Errorf("row = %v", v)
	}
	if !near4(v.GetCiLow(), 0.1334) || !near4(v.GetCiHigh(), 0.2888) {
		t.Errorf("ci = [%.6f, %.6f], want [0.1334, 0.2888]", v.GetCiLow(), v.GetCiHigh())
	}
}

// Scenario: Wilson interval boundaries
func TestWilson_ZeroAndFullBoundaries(t *testing.T) {
	z := wilsonVia(t, 0, 0)
	if z.GetRate() != 0 || z.GetCiLow() != 0 || z.GetCiHigh() != 0 {
		t.Errorf("zero exposed = %v, want rate and bounds 0", z)
	}
	f := wilsonVia(t, 10, 10)
	if f.GetRate() != 1 || math.Abs(f.GetCiHigh()-1) > 1e-9 || !(f.GetCiLow() < 1 && f.GetCiLow() > 0) {
		t.Errorf("full = %v, want rate 1, ci_high 1, 0 < ci_low < 1", f)
	}
	n := wilsonVia(t, 10, 0)
	if n.GetCiLow() != 0 || !(n.GetCiHigh() > 0 && n.GetCiHigh() < 1) {
		t.Errorf("none converted = %v, want ci_low 0", n)
	}
}

// Scenario: Wilson interval for a known sample (window_days and variant order passthrough)
func TestWilson_KnownValues_WindowDaysIsSeven(t *testing.T) {
	repo := &reportsRepo{}
	resp, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: "f"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetWindowDays() != 7 || repo.lastWindowDy != 7 || len(resp.GetVariants()) != 0 {
		t.Errorf("window_days=%d passed=%d variants=%v, want 7, 7, none", resp.GetWindowDays(), repo.lastWindowDy, resp.GetVariants())
	}
	if repo.lastFlag != "f" {
		t.Errorf("flag = %q", repo.lastFlag)
	}
}

// Scenario: Missing or oversized flag is rejected
func TestExperiment_FlagValidation(t *testing.T) {
	for _, flag := range []string{"", strings.Repeat("a", 65)} {
		repo := &reportsRepo{}
		_, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: flag})
		wantCode(t, err, codes.InvalidArgument)
		if !strings.Contains(err.Error(), "flag") {
			t.Errorf("message %q does not name flag", err.Error())
		}
		if repo.total() != 0 {
			t.Errorf("repository queried for flag len %d", len(flag))
		}
	}
	repo := &reportsRepo{}
	if _, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: strings.Repeat("a", 64)}); err != nil {
		t.Errorf("64-char flag rejected: %v", err)
	}
}

// Scenario: Inverted or oversized window is rejected
func TestExperiment_WindowValidation(t *testing.T) {
	to := rpNow
	cases := map[string][2]time.Time{
		"inverted":  {to, to.Add(-time.Hour)},
		"367 days":  {to.AddDate(0, 0, -367), to},
		"367 days+": {to.Add(-367*24*time.Hour - time.Second), to},
	}
	for name, w := range cases {
		repo := &reportsRepo{}
		_, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{
			Flag: "f", From: timestamppb.New(w[0]), To: timestamppb.New(w[1]),
		})
		wantCode(t, err, codes.InvalidArgument)
		if repo.total() != 0 {
			t.Errorf("%s: repository queried", name)
		}
	}
}

// Scenario: Window of exactly 366 days is accepted
func TestExperiment_WindowBoundaryAccepted(t *testing.T) {
	repo := &reportsRepo{}
	from := rpNow.Add(-366 * 24 * time.Hour)
	_, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{
		Flag: "f", From: timestamppb.New(from), To: timestamppb.New(rpNow),
	})
	if err != nil {
		t.Fatalf("366-day window rejected: %v", err)
	}
	if !repo.lastFrom.Equal(from) || !repo.lastTo.Equal(rpNow) {
		t.Errorf("window passed = [%v, %v]", repo.lastFrom, repo.lastTo)
	}
}

// Scenario: Window of exactly 366 days is accepted (unset bounds default to the last 30 days)
func TestExperiment_WindowBoundaryAccepted_DefaultsToLast30Days(t *testing.T) {
	repo := &reportsRepo{}
	if _, err := rpHandler(repo).GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: "f"}); err != nil {
		t.Fatal(err)
	}
	if !repo.lastTo.Equal(rpNow) || !repo.lastFrom.Equal(rpNow.Add(-30*24*time.Hour)) {
		t.Errorf("window = [%v, %v], want [now-30d, now]", repo.lastFrom, repo.lastTo)
	}
	ai := &reportsRepo{}
	if _, err := rpHandler(ai).GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{}); err != nil {
		t.Fatal(err)
	}
	if !ai.lastTo.Equal(rpNow) || !ai.lastFrom.Equal(rpNow.Add(-30*24*time.Hour)) {
		t.Errorf("ai window = [%v, %v], want [now-30d, now]", ai.lastFrom, ai.lastTo)
	}
}

// Scenario: Inverted window is rejected
func TestAiSurface_WindowValidation(t *testing.T) {
	repo := &reportsRepo{}
	h := rpHandler(repo)
	_, err := h.GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{
		From: timestamppb.New(rpNow), To: timestamppb.New(rpNow.Add(-time.Second)),
	})
	wantCode(t, err, codes.InvalidArgument)
	_, err = h.GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{
		From: timestamppb.New(rpNow.AddDate(0, 0, -367)), To: timestamppb.New(rpNow),
	})
	wantCode(t, err, codes.InvalidArgument)
	if repo.total() != 0 {
		t.Errorf("repository queried")
	}
}

// Scenario: Inverted window is rejected (valid report maps rows)
func TestAiSurface_WindowValidation_ValidWindowMapsRows(t *testing.T) {
	repo := &reportsRepo{ai: []query.AiSurfaceRow{{Surface: "assistant", ModelVersion: "", Shown: 5, Accepted: 2, Edited: 1, Dismissed: 1}}}
	resp, err := rpHandler(repo).GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{
		From: timestamppb.New(rpNow.Add(-time.Hour)), To: timestamppb.New(rpNow),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := resp.GetRows()
	if len(r) != 1 || r[0].GetSurface() != "assistant" || r[0].GetModelVersion() != "" ||
		r[0].GetShown() != 5 || r[0].GetAccepted() != 2 || r[0].GetEdited() != 1 || r[0].GetDismissed() != 1 {
		t.Errorf("rows = %v", r)
	}
}

// Scenario: Responses carry no identifiers
func TestReports_NoIdentifiersLeak(t *testing.T) {
	ctx := context.Background()
	w := cfWarehouse(t)
	secrets := []string{"user-secret-7731", "anon-secret-9042", "order-secret-5518"}
	at := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	recs := []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "sign_up", PrincipalID: secrets[0], PrincipalType: "user", OccurredAt: at},
		{EventID: "e2", EventType: "view", PrincipalID: secrets[0], PrincipalType: "user", OccurredAt: at.Add(24 * time.Hour)},
		{
			EventID: "e3", EventType: "experiment_exposure", PrincipalID: secrets[0], PrincipalType: "user", OccurredAt: at.Add(time.Hour),
			Properties: map[string]string{"flag": "home-ranker", "variant": "a"},
		},
		{
			EventID: "e4", EventType: "experiment_exposure", AnonymousID: secrets[1], OccurredAt: at.Add(time.Hour),
			Properties: map[string]string{"flag": "home-ranker", "variant": "b"},
		},
		{
			EventID: "e5", EventType: "ai_interaction", AnonymousID: secrets[1], ModelVersion: "mock:v1", OccurredAt: at,
			Properties: map[string]string{"surface": "assistant", "action": "shown"},
		},
	}
	if err := w.Write(ctx, recs); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{{
		EventID: "o1", OrderID: secrets[2], ListingID: "l1", SellerID: "s1", Quantity: 1, UnitPrice: 100, Currency: "VND",
		OccurredAt: at.Add(2 * time.Hour), Status: "PAID", BuyerID: secrets[0],
	}}); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	h, err := rpc.NewHandler(query.NewDuckDBRepository(w.DB()), slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		rpc.WithClock(func() time.Time { return rpNow }))
	if err != nil {
		t.Fatal(err)
	}
	from, to := timestamppb.New(at.Add(-24*time.Hour)), timestamppb.New(at.Add(30*24*time.Hour))
	r1, e1 := h.GetRetentionCohorts(rpAdmin(), &analyticsv1.GetRetentionCohortsRequest{From: from, Weeks: 26})
	r2, e2 := h.GetExperimentReadout(rpAdmin(), &analyticsv1.GetExperimentReadoutRequest{Flag: "home-ranker", From: from, To: to})
	r3, e3 := h.GetAiSurfaceReport(rpAdmin(), &analyticsv1.GetAiSurfaceReportRequest{From: from, To: to})
	for i, e := range []error{e1, e2, e3} {
		if e != nil {
			t.Fatalf("report %d: %v", i, e)
		}
	}
	if len(r1.GetCohorts()) == 0 || len(r2.GetVariants()) != 2 || len(r3.GetRows()) != 1 {
		t.Fatalf("fixture not reported: cohorts=%v variants=%v rows=%v", r1.GetCohorts(), r2.GetVariants(), r3.GetRows())
	}
	var all strings.Builder
	for _, b := range [][]byte{mustJSON(t, r1), mustJSON(t, r2), mustJSON(t, r3)} {
		all.Write(b)
	}
	all.WriteString(logs.String())
	for _, s := range secrets {
		if strings.Contains(all.String(), s) {
			t.Errorf("identifier %q leaked into responses or logs", s)
		}
	}
}

func mustJSON(t *testing.T, m interface {
	ProtoReflect() protoreflect.Message
},
) []byte {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
