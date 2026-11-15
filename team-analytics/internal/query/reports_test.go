package query_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

func day(m time.Month, d, h, mi int) time.Time {
	return time.Date(2026, m, d, h, mi, 0, 0, time.UTC)
}

var (
	aug = func(d, h int) time.Time { return day(time.August, d, h, 0) }
	sep = func(d, h int) time.Time { return day(time.September, d, h, 0) }
)

func ev(id, typ, user string, at time.Time) *warehouse.TrackingRecord {
	return sev(id, typ, user, "", "", "", "", at)
}

func anonEv(id, typ, anon string, at time.Time, props map[string]string) *warehouse.TrackingRecord {
	return &warehouse.TrackingRecord{EventID: id, EventType: typ, AnonymousID: anon, OccurredAt: at, Properties: props}
}

func retention(t *testing.T, repo *query.DuckDBRepository, from time.Time, weeks int, now time.Time) []query.RetentionCohortRow {
	t.Helper()
	got, err := repo.RetentionCohorts(context.Background(), from, weeks, now)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func signUps(prefix string, n int, at time.Time) []*warehouse.TrackingRecord {
	var out []*warehouse.TrackingRecord
	for i := 1; i <= n; i++ {
		out = append(out, ev(fmt.Sprintf("%s-su%d", prefix, i), "sign_up", fmt.Sprintf("%s%d", prefix, i), at))
	}
	return out
}

func TestRetention_AnchorIsFirstSignUpWeek(t *testing.T) {
	recs := signUps("u", 3, aug(5, 10))
	recs = append(recs, ev("u1-su-late", "sign_up", "u1", aug(19, 10)))
	repo := searchRepo(t, recs)
	for _, weeks := range []int{1, 3} {
		got := retention(t, repo, aug(3, 0), weeks, sep(14, 12))
		if len(got) != 1 || got[0].CohortWeek != "2026-08-03" || got[0].CohortSize != 3 {
			t.Fatalf("weeks=%d: got %+v, want one cohort 2026-08-03 size 3", weeks, got)
		}
	}
}

func TestRetention_DayNIsExactDay(t *testing.T) {
	recs := signUps("u", 4, aug(5, 10))
	recs = append(recs,
		ev("e1", "view", "u1", aug(6, 23)),
		ev("e2", "view", "u2", aug(6, 1)),
		ev("e3", "view", "u3", aug(12, 8)),
		ev("e4", "view", "u4", aug(13, 8)),
		ev("e5", "view", "u1", aug(14, 8)),
	)
	repo := searchRepo(t, recs)
	got := retention(t, repo, aug(3, 0), 1, sep(14, 12))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	c := got[0]
	if c.CohortSize != 4 || c.D1Active != 2 || c.D1Eligible != 4 || c.D7Active != 1 || c.D7Eligible != 4 || c.D30Active != 0 || c.D30Eligible != 4 {
		t.Fatalf("got %+v, want d1 2/4 d7 1/4 d30 0/4", c)
	}
}

func TestRetention_WeeklyActiveOnlyElapsedWeeks(t *testing.T) {
	recs := signUps("u", 4, aug(5, 10))
	recs = append(recs, ev("e1", "view", "u1", aug(12, 8)), ev("e2", "view", "u2", aug(16, 8)))
	repo := searchRepo(t, recs)
	got := retention(t, repo, aug(3, 0), 1, aug(24, 12))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if want := []int64{4, 2, 0}; !reflect.DeepEqual(got[0].WeeklyActive, want) {
		t.Fatalf("WeeklyActive = %v, want %v", got[0].WeeklyActive, want)
	}
}

func TestRetention_RepeatPurchaseWithin30Days(t *testing.T) {
	recs := signUps("u", 3, aug(5, 10))
	repo := searchRepo(t, recs,
		spaid("b1a", "u1", "L", aug(6, 9)),
		spaid("b1b", "u1", "L", aug(16, 9)),
		spaid("b2a", "u2", "L", aug(6, 9)),
		spaid("b2b", "u2", "L", sep(15, 9)),
	)
	got := retention(t, repo, aug(3, 0), 1, sep(21, 12))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[0].CohortSize != 3 || got[0].RepeatEligible != 2 || got[0].RepeatBuyers != 1 {
		t.Fatalf("got %+v, want repeat 1/2", got[0])
	}
}

func TestRetention_YoungCohortReportsZero(t *testing.T) {
	recs := []*warehouse.TrackingRecord{
		ev("s1", "sign_up", "y1", sep(11, 10)),
		ev("s2", "sign_up", "y2", sep(13, 22)),
		ev("e1", "view", "y1", sep(12, 9)),
	}
	repo := searchRepo(t, recs, spaid("p1", "y1", "L", sep(11, 11)))
	got := retention(t, repo, sep(7, 0), 1, sep(14, 12))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	c := got[0]
	if c.CohortSize != 2 || c.D1Eligible != 1 || c.D1Active != 1 ||
		c.D7Eligible != 0 || c.D7Active != 0 || c.D30Eligible != 0 || c.D30Active != 0 ||
		c.RepeatEligible != 0 || c.RepeatBuyers != 0 {
		t.Fatalf("got %+v", c)
	}
}

func TestRetention_NoSignUpExcluded(t *testing.T) {
	recs := signUps("u", 2, aug(5, 10))
	recs = append(recs,
		ev("x1", "view", "ghost", aug(6, 10)),
		anonEv("a1", "sign_up", "anon-only", aug(5, 11), nil),
	)
	repo := searchRepo(t, recs,
		spaid("g1", "ghost", "L", aug(6, 9)),
		spaid("g2", "ghost", "L", aug(7, 9)),
	)
	got := retention(t, repo, aug(3, 0), 1, sep(14, 12))
	if len(got) != 1 || got[0].CohortSize != 2 || got[0].RepeatEligible != 0 || got[0].RepeatBuyers != 0 {
		t.Fatalf("got %+v, want one cohort of 2 with no repeat eligibility", got)
	}
}

func TestRetention_EmptyWarehouseReturnsEmptyList(t *testing.T) {
	repo := searchRepo(t, nil)
	if got := retention(t, repo, aug(3, 0), 12, sep(14, 12)); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func exp(id, user, flag, variant string, at time.Time) *warehouse.TrackingRecord {
	r := ev(id, "experiment_exposure", user, at)
	r.Properties = map[string]string{"flag": flag, "variant": variant}
	return r
}

func readout(t *testing.T, repo *query.DuckDBRepository, flag string, from, to time.Time) map[string]query.ExperimentRow {
	t.Helper()
	got, err := repo.ExperimentReadout(context.Background(), flag, from, to, 7)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]query.ExperimentRow{}
	for _, r := range got {
		if _, dup := by[r.Variant]; dup {
			t.Fatalf("duplicate variant row: %+v", got)
		}
		by[r.Variant] = r
	}
	return by
}

var expT = sep(1, 12)

func expWindow() (time.Time, time.Time) { return expT.Add(-24 * time.Hour), expT.Add(24 * time.Hour) }

func TestExperiment_FirstExposureWins(t *testing.T) {
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		exp("e2", "u1", "F", "b", expT.Add(time.Hour)),
		exp("e1", "u1", "F", "a", expT),
	})
	from, to := expWindow()
	by := readout(t, repo, "F", from, to)
	if len(by) != 1 || by["a"].Exposed != 1 {
		t.Fatalf("got %+v, want only a with exposed 1", by)
	}
}

func TestExperiment_ConversionWindowSevenDays(t *testing.T) {
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		exp("e1", "u1", "F", "a", expT),
		exp("e2", "u2", "F", "a", expT),
	},
		spaid("p1", "u1", "L", expT.Add(6*24*time.Hour)),
		spaid("p2", "u2", "L", expT.Add(8*24*time.Hour)),
	)
	from, to := expWindow()
	by := readout(t, repo, "F", from, to)
	if r := by["a"]; len(by) != 1 || r.Exposed != 2 || r.Converted != 1 {
		t.Fatalf("got %+v, want a exposed 2 converted 1", by)
	}
}

func TestExperiment_ConversionWindowSevenDaysBoundaryInclusive(t *testing.T) {
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		exp("e1", "u1", "F", "a", expT),
		exp("e2", "u2", "F", "a", expT),
	},
		spaid("p1", "u1", "L", expT.Add(7*24*time.Hour)),
		spaid("p2", "u2", "L", expT.Add(7*24*time.Hour+time.Second)),
	)
	from, to := expWindow()
	if r := readout(t, repo, "F", from, to)["a"]; r.Exposed != 2 || r.Converted != 1 {
		t.Fatalf("got %+v, want exposed 2 converted 1", r)
	}
}

func TestExperiment_OrderBeforeExposureDoesNotConvert(t *testing.T) {
	repo := searchRepo(t, []*warehouse.TrackingRecord{exp("e1", "u1", "F", "a", expT)},
		spaid("p1", "u1", "L", expT.Add(-24*time.Hour)))
	from, to := expWindow()
	if r := readout(t, repo, "F", from, to)["a"]; r.Exposed != 1 || r.Converted != 0 {
		t.Fatalf("got %+v, want exposed 1 converted 0", r)
	}
}

func TestExperiment_UnassignedAndOtherFlagsExcluded(t *testing.T) {
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		exp("e1", "u1", "F", "unassigned", expT),
		exp("e2", "u2", "G", "a", expT),
		exp("e3", "u3", "F", "a", expT),
	})
	from, to := expWindow()
	by := readout(t, repo, "F", from, to)
	if r := by["a"]; len(by) != 1 || r.Exposed != 1 {
		t.Fatalf("got %+v, want only a with exposed 1", by)
	}
}

func TestExperiment_AnonymousNeverConverts(t *testing.T) {
	r := anonEv("e1", "experiment_exposure", "anon-x", expT, map[string]string{"flag": "F", "variant": "a"})
	repo := searchRepo(t, []*warehouse.TrackingRecord{r}, spaid("p1", "anon-x", "L", expT.Add(time.Hour)))
	from, to := expWindow()
	if got := readout(t, repo, "F", from, to)["a"]; got.Exposed != 1 || got.Converted != 0 {
		t.Fatalf("got %+v, want exposed 1 converted 0", got)
	}
}

func TestExperiment_ExposureOutsideWindowAndUnknownFlag(t *testing.T) {
	repo := searchRepo(t, []*warehouse.TrackingRecord{exp("e1", "u1", "F", "a", expT.Add(-48*time.Hour))})
	from, to := expWindow()
	if by := readout(t, repo, "F", from, to); len(by) != 0 {
		t.Fatalf("got %+v, want none", by)
	}
	if by := readout(t, repo, "nope", expT.Add(-72*time.Hour), to); len(by) != 0 {
		t.Fatalf("unknown flag got %+v, want none", by)
	}
}

func aiEv(id, surface, action, model string, at time.Time) *warehouse.TrackingRecord {
	r := ev(id, "ai_interaction", "u1", at)
	r.ModelVersion = model
	r.Properties = map[string]string{"surface": surface, "action": action}
	return r
}

func aiRows(t *testing.T, repo *query.DuckDBRepository, from, to time.Time) []query.AiSurfaceRow {
	t.Helper()
	got, err := repo.AiSurfaceReport(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func aiBatch(prefix, surface, model string, shown, acc, edited, dis int, at time.Time) []*warehouse.TrackingRecord {
	var out []*warehouse.TrackingRecord
	n := 0
	add := func(action string, c int) {
		for i := 0; i < c; i++ {
			n++
			out = append(out, aiEv(fmt.Sprintf("%s-%d", prefix, n), surface, action, model, at))
		}
	}
	add("shown", shown)
	add("accepted", acc)
	add("edited", edited)
	add("dismissed", dis)
	return out
}

func TestAiSurface_CountsPerSurfaceAndVersion(t *testing.T) {
	at := sep(1, 10)
	recs := aiBatch("a", "assistant", "mock:v1|hybrid", 5, 2, 1, 1, at)
	recs = append(recs, aiBatch("c", "chat_copilot", "rules-v1", 3, 0, 0, 0, at)...)
	repo := searchRepo(t, recs)
	got := aiRows(t, repo, sep(1, 0), sep(2, 0))
	want := []query.AiSurfaceRow{
		{Surface: "assistant", ModelVersion: "mock:v1|hybrid", Shown: 5, Accepted: 2, Edited: 1, Dismissed: 1},
		{Surface: "chat_copilot", ModelVersion: "rules-v1", Shown: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAiSurface_WindowBoundaryInclusive(t *testing.T) {
	from, to := sep(1, 0), sep(2, 0)
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		aiEv("a1", "assistant", "shown", "m", from.Add(-time.Second)),
		aiEv("a2", "assistant", "shown", "m", to),
	})
	got := aiRows(t, repo, from, to)
	if len(got) != 1 || got[0].Shown != 1 {
		t.Fatalf("got %+v, want one row with shown 1", got)
	}
}

func TestAiSurface_EmptyModelVersionOwnRow(t *testing.T) {
	at := sep(1, 10)
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		aiEv("a1", "assistant", "shown", "", at),
		aiEv("a2", "assistant", "shown", "x", at),
		aiEv("a3", "assistant", "shown", "x", at),
	})
	got := aiRows(t, repo, sep(1, 0), sep(2, 0))
	want := []query.AiSurfaceRow{
		{Surface: "assistant", ModelVersion: "", Shown: 1},
		{Surface: "assistant", ModelVersion: "x", Shown: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAiSurface_UnknownActionIgnored(t *testing.T) {
	at := sep(1, 10)
	repo := searchRepo(t, []*warehouse.TrackingRecord{
		aiEv("a1", "assistant", "shown", "m", at),
		aiEv("a2", "assistant", "hovered", "m", at),
		aiEv("a3", "ghost", "hovered", "m", at),
	})
	got := aiRows(t, repo, sep(1, 0), sep(2, 0))
	want := []query.AiSurfaceRow{{Surface: "assistant", ModelVersion: "m", Shown: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestAiSurface_EmptyWindowReturnsNoRows(t *testing.T) {
	repo := searchRepo(t, nil)
	if got := aiRows(t, repo, sep(1, 0), sep(2, 0)); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}
