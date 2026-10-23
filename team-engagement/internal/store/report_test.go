package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

// reportFixture hides the only differences between the two implementations:
// how rows with arbitrary timestamps and seller_listings rows are seeded.
type reportFixture struct {
	name        string
	repo        store.ReportRepository
	seed        func(t *testing.T, r store.Report)
	seedListing func(t *testing.T, sellerID, listingID string)
}

func newMemReportFixture() reportFixture {
	m := storetest.NewFakeReportRepository()
	return reportFixture{
		name:        "memory",
		repo:        m,
		seed:        func(_ *testing.T, r store.Report) { m.SeedReport(r) },
		seedListing: func(_ *testing.T, s, l string) { m.SeedSellerListing(s, l) },
	}
}

func newPGReportFixture(t *testing.T) reportFixture {
	pool := testPool(t)
	return reportFixture{
		name: "postgres",
		repo: store.NewPostgresReportRepository(pool),
		seed: func(t *testing.T, r store.Report) { seedReportRow(t, pool, r) },
		seedListing: func(t *testing.T, s, l string) {
			if _, err := pool.Exec(context.Background(),
				`INSERT INTO seller_listings (seller_id, listing_id) VALUES ($1,$2)`, s, l); err != nil {
				t.Fatal(err)
			}
		},
	}
}

func seedReportRow(t *testing.T, pool *pgxpool.Pool, r store.Report) {
	t.Helper()
	if r.TargetType == "" {
		r.TargetType = "LISTING"
	}
	if r.ReasonCode == "" {
		r.ReasonCode = "SPAM"
	}
	if r.Status == "" {
		r.Status = store.ReportStatusOpen
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO reports
		(id, target_type, target_id, reason_code, note, reporter_id, status, created_at, resolved_at, resolved_by, resolution_note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		r.ID, r.TargetType, r.TargetID, r.ReasonCode, r.Note, r.ReporterID, string(r.Status),
		r.CreatedAt, r.ResolvedAt, r.ResolvedBy, r.ResolutionNote); err != nil {
		t.Fatalf("seed report %s: %v", r.ID, err)
	}
}

// forEachReportRepo runs fn against the in-memory repo and, when
// TEST_DATABASE_URL is set, against Postgres (a skip is not a pass).
func forEachReportRepo(t *testing.T, fn func(t *testing.T, f reportFixture)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { fn(t, newMemReportFixture()) })
	t.Run("postgres", func(t *testing.T) { fn(t, newPGReportFixture(t)) })
}

func newReport(reporter, target string) store.Report {
	return store.Report{TargetType: "LISTING", TargetID: target, ReasonCode: "SPAM", Note: "n", ReporterID: reporter}
}

func mustCreate(t *testing.T, repo store.ReportRepository, r store.Report) store.Report {
	t.Helper()
	got, err := repo.CreateReport(context.Background(), r)
	if err != nil {
		t.Fatalf("CreateReport: %v", err)
	}
	return got
}

// Scenario: Listing reports: Concurrent duplicate reports yield one row
func TestCreateReportConcurrentDuplicatesOneRow(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		const n = 10
		ids := make([]string, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				r, err := f.repo.CreateReport(ctx, newReport("u1", "L1"))
				ids[i], errs[i] = r.ID, err
			}(i)
		}
		close(start)
		wg.Wait()
		for i := range ids {
			if errs[i] != nil {
				t.Fatalf("call %d: %v", i, errs[i])
			}
			if ids[i] == "" || ids[i] != ids[0] {
				t.Fatalf("call %d id %q, want %q for all", i, ids[i], ids[0])
			}
		}
		_, total, err := f.repo.ListReports(ctx, "", "", 0, 50)
		if err != nil || total != 1 {
			t.Fatalf("rows total=%d err=%v, want exactly 1", total, err)
		}
		got, err := f.repo.GetReport(ctx, ids[0])
		if err != nil || got.Status != store.ReportStatusOpen || got.ReporterID != "u1" || got.TargetID != "L1" || got.CreatedAt.IsZero() {
			t.Fatalf("stored report = %+v err=%v", got, err)
		}
	})
}

// Scenario: Listing reports: Concurrent duplicate reports yield one row (different reporters / targets stay separate)
func TestCreateReportDistinctPairsDistinctRows(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		a := mustCreate(t, f.repo, newReport("u1", "L1"))
		b := mustCreate(t, f.repo, newReport("u2", "L1"))
		c := mustCreate(t, f.repo, newReport("u1", "L2"))
		if a.ID == b.ID || a.ID == c.ID || b.ID == c.ID {
			t.Fatalf("ids not distinct: %s %s %s", a.ID, b.ID, c.ID)
		}
	})
}

// Scenario: Listing reports: A closed report does not block a new one
func TestCreateReportAfterCloseNewRow(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		first := mustCreate(t, f.repo, newReport("u1", "L1"))
		ok, err := f.repo.CloseReport(ctx, first.ID, "admin-1", store.ReportStatusDismissed, "not an issue")
		if err != nil || !ok {
			t.Fatalf("close: ok=%v err=%v", ok, err)
		}
		second := mustCreate(t, f.repo, newReport("u1", "L1"))
		if second.ID == first.ID || second.Status != store.ReportStatusOpen {
			t.Fatalf("second = %+v, want new OPEN row (first %s)", second, first.ID)
		}
		again := mustCreate(t, f.repo, newReport("u1", "L1"))
		if again.ID != second.ID {
			t.Fatalf("re-report while open returned %s, want existing %s", again.ID, second.ID)
		}
		old, _ := f.repo.GetReport(ctx, first.ID)
		if old.Status != store.ReportStatusDismissed {
			t.Fatalf("first report status %s, want DISMISSED", old.Status)
		}
	})
}

// Scenario: Listing reports: Open report cap per reporter (repository counts only OPEN rows of that reporter)
func TestCountOpenByReporter(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		var ids []string
		for i := 0; i < 3; i++ {
			ids = append(ids, mustCreate(t, f.repo, newReport("u1", fmt.Sprintf("L%d", i))).ID)
		}
		mustCreate(t, f.repo, newReport("u2", "L0"))
		if n, err := f.repo.CountOpenByReporter(ctx, "u1"); err != nil || n != 3 {
			t.Fatalf("u1 open = %d err=%v, want 3", n, err)
		}
		if _, err := f.repo.CloseReport(ctx, ids[0], "a", store.ReportStatusActioned, "done"); err != nil {
			t.Fatal(err)
		}
		if n, _ := f.repo.CountOpenByReporter(ctx, "u1"); n != 2 {
			t.Fatalf("u1 open after close = %d, want 2", n)
		}
		if n, _ := f.repo.CountOpenByReporter(ctx, "nobody"); n != 0 {
			t.Fatalf("unknown reporter open = %d, want 0", n)
		}
	})
}

// Scenario: Listing reports: Unknown report
func TestGetReportNotFound(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		_, err := f.repo.GetReport(context.Background(), "missing")
		if !errors.Is(err, store.ErrReportNotFound) {
			t.Fatalf("err = %v, want ErrReportNotFound", err)
		}
	})
}

// Scenario: Listing reports: Filter by target id
func TestListReportsFilterByTarget(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
		f.seed(t, store.Report{ID: "r-a1", TargetID: "A", ReporterID: "u1", Status: store.ReportStatusOpen, CreatedAt: base})
		f.seed(t, store.Report{ID: "r-b1", TargetID: "B", ReporterID: "u1", Status: store.ReportStatusOpen, CreatedAt: base.Add(time.Minute)})
		f.seed(t, store.Report{ID: "r-a2", TargetID: "A", ReporterID: "u2", Status: store.ReportStatusOpen, CreatedAt: base.Add(2 * time.Minute)})
		got, total, err := f.repo.ListReports(ctx, store.ReportStatusOpen, "A", 0, 50)
		if err != nil || total != 2 || len(got) != 2 {
			t.Fatalf("len=%d total=%d err=%v, want 2/2", len(got), total, err)
		}
		for _, r := range got {
			if r.TargetID != "A" {
				t.Fatalf("report %s for target %s leaked into filter A", r.ID, r.TargetID)
			}
		}
		none, total, err := f.repo.ListReports(ctx, store.ReportStatusOpen, "ZZ", 0, 50)
		if err != nil || total != 0 || len(none) != 0 {
			t.Fatalf("unknown target: len=%d total=%d err=%v", len(none), total, err)
		}
	})
}

// Scenario: Listing reports: Admin lists open reports oldest first (repository order, status filter and paging)
func TestListReportsOldestFirstStatusAndPaging(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
		// Inserted newest first; ids tie on one timestamp to pin the (created_at, id) order.
		f.seed(t, store.Report{ID: "r-3", TargetID: "A", ReporterID: "u3", CreatedAt: base.Add(2 * time.Minute)})
		f.seed(t, store.Report{ID: "r-2b", TargetID: "A", ReporterID: "u2b", CreatedAt: base.Add(time.Minute)})
		f.seed(t, store.Report{ID: "r-2a", TargetID: "A", ReporterID: "u2a", CreatedAt: base.Add(time.Minute)})
		f.seed(t, store.Report{ID: "r-1", TargetID: "A", ReporterID: "u1", CreatedAt: base})
		closedAt := base.Add(time.Hour)
		f.seed(t, store.Report{ID: "r-c", TargetID: "A", ReporterID: "u9", Status: store.ReportStatusDismissed,
			CreatedAt: base.Add(-time.Minute), ResolvedAt: &closedAt, ResolvedBy: "adm", ResolutionNote: "x"})

		got, total, err := f.repo.ListReports(ctx, store.ReportStatusOpen, "", 0, 50)
		if err != nil || total != 4 {
			t.Fatalf("open total=%d err=%v, want 4", total, err)
		}
		want := []string{"r-1", "r-2a", "r-2b", "r-3"}
		for i, r := range got {
			if r.ID != want[i] {
				t.Fatalf("order[%d] = %s, want %s (all %v)", i, r.ID, want[i], got)
			}
		}
		page, total, err := f.repo.ListReports(ctx, store.ReportStatusOpen, "", 1, 2)
		if err != nil || total != 4 || len(page) != 2 || page[0].ID != "r-2a" || page[1].ID != "r-2b" {
			t.Fatalf("page = %v total=%d err=%v", page, total, err)
		}
		closed, total, _ := f.repo.ListReports(ctx, store.ReportStatusDismissed, "", 0, 50)
		if total != 1 || len(closed) != 1 || closed[0].ID != "r-c" || closed[0].ResolvedAt == nil || closed[0].ResolvedBy != "adm" {
			t.Fatalf("dismissed = %+v total=%d", closed, total)
		}
		anyStatus, total, _ := f.repo.ListReports(ctx, "", "", 0, 50)
		if total != 5 || len(anyStatus) != 5 {
			t.Fatalf("any status total=%d len=%d, want 5", total, len(anyStatus))
		}
	})
}

// Scenario: Listing reports: Empty result
func TestListReportsEmpty(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		got, total, err := f.repo.ListReports(context.Background(), store.ReportStatusOpen, "", 0, 50)
		if err != nil || total != 0 || len(got) != 0 {
			t.Fatalf("len=%d total=%d err=%v, want empty", len(got), total, err)
		}
	})
}

// Scenario: Listing reports: Two admins close the same report at once
func TestCloseReportCompareAndSetOneWinner(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		rep := mustCreate(t, f.repo, newReport("u1", "L1"))
		type res struct {
			ok  bool
			err error
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		out := make([]res, 2)
		admins := []string{"adm-1", "adm-2"}
		statuses := []store.ReportStatus{store.ReportStatusActioned, store.ReportStatusDismissed}
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				ok, err := f.repo.CloseReport(ctx, rep.ID, admins[i], statuses[i], "note-"+admins[i])
				out[i] = res{ok, err}
			}(i)
		}
		close(start)
		wg.Wait()
		winner := -1
		for i, r := range out {
			if r.err != nil {
				t.Fatalf("admin %d: %v", i, r.err)
			}
			if r.ok {
				if winner != -1 {
					t.Fatal("both admins reported a state change")
				}
				winner = i
			}
		}
		if winner == -1 {
			t.Fatal("no admin won")
		}
		got, err := f.repo.GetReport(ctx, rep.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != statuses[winner] || got.ResolvedBy != admins[winner] ||
			got.ResolutionNote != "note-"+admins[winner] || got.ResolvedAt == nil {
			t.Fatalf("stored = %+v, want winner %s with %s", got, admins[winner], statuses[winner])
		}
	})
}

// Scenario: Listing reports: Closing again with the same status is refused (repository level: second close changes nothing)
func TestCloseReportSecondCloseChangesNothing(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		rep := mustCreate(t, f.repo, newReport("u1", "L1"))
		if ok, err := f.repo.CloseReport(ctx, rep.ID, "adm-1", store.ReportStatusActioned, "first"); err != nil || !ok {
			t.Fatalf("first close ok=%v err=%v", ok, err)
		}
		for _, st := range []store.ReportStatus{store.ReportStatusActioned, store.ReportStatusDismissed} {
			ok, err := f.repo.CloseReport(ctx, rep.ID, "adm-2", st, "second")
			if err != nil || ok {
				t.Fatalf("second close(%s) ok=%v err=%v, want false,nil", st, ok, err)
			}
		}
		got, _ := f.repo.GetReport(ctx, rep.ID)
		if got.Status != store.ReportStatusActioned || got.ResolvedBy != "adm-1" || got.ResolutionNote != "first" {
			t.Fatalf("stored = %+v, want first close preserved", got)
		}
	})
}

// Scenario: Listing reports: Unknown report
func TestCloseReportUnknown(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ok, err := f.repo.CloseReport(context.Background(), "missing", "adm", store.ReportStatusActioned, "n")
		if ok || !errors.Is(err, store.ErrReportNotFound) {
			t.Fatalf("ok=%v err=%v, want false, ErrReportNotFound", ok, err)
		}
	})
}

// Scenario: Listing reports: Seller reports own listing (repository provides the ownership lookup)
func TestReportHasSellerListing(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		f.seedListing(t, "seller-1", "L1")
		cases := []struct {
			seller, listing string
			want            bool
		}{
			{"seller-1", "L1", true},
			{"seller-2", "L1", false},
			{"seller-1", "L2", false},
			{"", "", false},
		}
		for _, c := range cases {
			got, err := f.repo.HasSellerListing(ctx, c.seller, c.listing)
			if err != nil || got != c.want {
				t.Errorf("HasSellerListing(%q,%q) = %v err=%v, want %v", c.seller, c.listing, got, err, c.want)
			}
		}
	})
}

func sweepClock() (now, cutoff time.Time) {
	now = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	return now, now.Add(-730 * 24 * time.Hour)
}

func closedReport(id string, resolvedAt time.Time) store.Report {
	return store.Report{ID: id, TargetID: "L1", ReasonCode: "SPAM", Note: "my phone 0900", ReporterID: "u-" + id,
		Status: store.ReportStatusActioned, CreatedAt: resolvedAt.Add(-time.Hour), ResolvedAt: &resolvedAt,
		ResolvedBy: "adm-1", ResolutionNote: "removed"}
}

// Scenario: Listing reports: Old closed report is pseudonymised
func TestSweepPseudonymisesOldClosedReports(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		now, cutoff := sweepClock()
		resolved := now.Add(-731 * 24 * time.Hour).Truncate(time.Microsecond)
		old := closedReport("old", resolved)
		old.ReporterID = "u-1"
		f.seed(t, old)
		n, err := f.repo.SweepClosedBefore(ctx, cutoff, 500)
		if err != nil || n != 1 {
			t.Fatalf("sweep n=%d err=%v, want 1", n, err)
		}
		got, err := f.repo.GetReport(ctx, "old")
		if err != nil {
			t.Fatal(err)
		}
		if got.ReporterID != "redacted" || got.Note != "" {
			t.Fatalf("reporter=%q note=%q, want redacted and empty", got.ReporterID, got.Note)
		}
		if got.Status != store.ReportStatusActioned || got.TargetID != "L1" || got.ReasonCode != "SPAM" ||
			got.ResolutionNote != "removed" || got.ResolvedAt == nil || !got.ResolvedAt.Equal(resolved) {
			t.Fatalf("non-personal fields changed: %+v", got)
		}
	})
}

// Scenario: Listing reports: Retention boundary
func TestSweepRetentionBoundary(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		now, cutoff := sweepClock()
		f.seed(t, closedReport("inside", now.Add(-730*24*time.Hour+time.Minute)))  // 729d23h59m ago
		f.seed(t, closedReport("outside", now.Add(-730*24*time.Hour-time.Minute))) // 730d + 1m ago
		n, err := f.repo.SweepClosedBefore(ctx, cutoff, 500)
		if err != nil || n != 1 {
			t.Fatalf("sweep n=%d err=%v, want 1", n, err)
		}
		in, _ := f.repo.GetReport(ctx, "inside")
		out, _ := f.repo.GetReport(ctx, "outside")
		if in.ReporterID != "u-inside" || in.Note != "my phone 0900" {
			t.Fatalf("inside the window was touched: %+v", in)
		}
		if out.ReporterID != "redacted" || out.Note != "" {
			t.Fatalf("outside the window not pseudonymised: %+v", out)
		}
	})
}

// Scenario: Listing reports: Open reports are never swept
func TestSweepSkipsOpenReports(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		now, cutoff := sweepClock()
		f.seed(t, store.Report{ID: "open-old", TargetID: "L1", ReporterID: "u-open", Note: "keep me",
			Status: store.ReportStatusOpen, CreatedAt: now.Add(-800 * 24 * time.Hour)})
		n, err := f.repo.SweepClosedBefore(ctx, cutoff, 500)
		if err != nil || n != 0 {
			t.Fatalf("sweep n=%d err=%v, want 0", n, err)
		}
		got, _ := f.repo.GetReport(ctx, "open-old")
		if got.ReporterID != "u-open" || got.Note != "keep me" {
			t.Fatalf("open report altered: %+v", got)
		}
	})
}

// Scenario: Listing reports: Sweep is idempotent and batched
func TestSweepIdempotentAndBatched(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		now, cutoff := sweepClock()
		old := now.Add(-731 * 24 * time.Hour)
		for i := 0; i < 1200; i++ {
			f.seed(t, closedReport(fmt.Sprintf("r-%04d", i), old))
		}
		var batches []int64
		for i := 0; i < 10; i++ {
			n, err := f.repo.SweepClosedBefore(ctx, cutoff, 500)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
			batches = append(batches, n)
		}
		if fmt.Sprint(batches) != "[500 500 200]" {
			t.Fatalf("batches = %v, want [500 500 200]", batches)
		}
		if n, err := f.repo.SweepClosedBefore(ctx, cutoff, 500); err != nil || n != 0 {
			t.Fatalf("second run n=%d err=%v, want 0", n, err)
		}
		for _, id := range []string{"r-0000", "r-0599", "r-1199"} {
			got, _ := f.repo.GetReport(ctx, id)
			if got.ReporterID != "redacted" || got.Note != "" {
				t.Fatalf("%s not pseudonymised: %+v", id, got)
			}
		}
	})
}

// Scenario: Listing reports: Sweep is idempotent and batched (batch 1 stays committed, next run finishes the rest)
func TestSweepPartialProgressSurvivesAndResumes(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		now, cutoff := sweepClock()
		for i := 0; i < 7; i++ {
			f.seed(t, closedReport(fmt.Sprintf("p-%d", i), now.Add(-731*24*time.Hour)))
		}
		if n, err := f.repo.SweepClosedBefore(ctx, cutoff, 3); err != nil || n != 3 {
			t.Fatalf("first batch n=%d err=%v, want 3", n, err)
		}
		redacted := func() int {
			c := 0
			for i := 0; i < 7; i++ {
				if r, _ := f.repo.GetReport(ctx, fmt.Sprintf("p-%d", i)); r.ReporterID == "redacted" {
					c++
				}
			}
			return c
		}
		if got := redacted(); got != 3 {
			t.Fatalf("after batch 1 redacted=%d, want 3", got)
		}
		var total int64 = 3
		for i := 0; i < 5; i++ {
			n, err := f.repo.SweepClosedBefore(ctx, cutoff, 3)
			if err != nil {
				t.Fatal(err)
			}
			total += n
		}
		if total != 7 || redacted() != 7 {
			t.Fatalf("total=%d redacted=%d, want 7/7", total, redacted())
		}
	})
}

// Scenario: Listing reports: Pseudonymised reporter can report again (a redacted closed row does not block a real-id OPEN row)
func TestCreateReportAfterSweepStoresRealReporter(t *testing.T) {
	forEachReportRepo(t, func(t *testing.T, f reportFixture) {
		ctx := context.Background()
		now, cutoff := sweepClock()
		f.seed(t, closedReport("old", now.Add(-731*24*time.Hour)))
		if _, err := f.repo.SweepClosedBefore(ctx, cutoff, 500); err != nil {
			t.Fatal(err)
		}
		got := mustCreate(t, f.repo, newReport("u-old", "L1"))
		if got.ID == "old" || got.Status != store.ReportStatusOpen || got.ReporterID != "u-old" {
			t.Fatalf("new report = %+v", got)
		}
	})
}

// Scenario: Listing reports: In-memory repository matches Postgres
func TestReportRepositoryParity(t *testing.T) {
	pg := newPGReportFixture(t)
	mem := newMemReportFixture()
	run := func(f reportFixture) []string {
		ctx := context.Background()
		var log []string
		rec := func(format string, a ...any) { log = append(log, fmt.Sprintf(format, a...)) }
		norm := func(r store.Report, idx map[string]string) string {
			return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|resolved=%v", idx[r.ID], r.TargetType, r.TargetID,
				r.ReasonCode, r.Note, r.ReporterID, r.Status, r.ResolvedBy, r.ResolutionNote, r.ResolvedAt != nil)
		}
		names := map[string]string{}
		create := func(label string, r store.Report) {
			got, err := f.repo.CreateReport(ctx, r)
			if _, seen := names[got.ID]; !seen && err == nil {
				names[got.ID] = fmt.Sprintf("#%d", len(names)+1)
			}
			rec("create %s err=%v -> %s", label, err != nil, norm(got, names))
		}
		create("a", newReport("u1", "L1"))
		create("a-dup", newReport("u1", "L1"))
		create("b", newReport("u2", "L1"))
		create("c", newReport("u1", "L2"))
		n, err := f.repo.CountOpenByReporter(ctx, "u1")
		rec("count u1=%d err=%v", n, err != nil)

		reps, total, err := f.repo.ListReports(ctx, store.ReportStatusOpen, "L1", 0, 10)
		rec("list L1 total=%d err=%v", total, err != nil)
		for _, r := range reps {
			rec("  %s", norm(r, names))
		}
		if len(reps) == 0 {
			t.Fatalf("%s: no open reports for L1", f.name)
		}
		first := reps[0]
		ok1, e1 := f.repo.CloseReport(ctx, first.ID, "adm-1", store.ReportStatusDismissed, "nope")
		ok2, e2 := f.repo.CloseReport(ctx, first.ID, "adm-2", store.ReportStatusActioned, "late")
		ok3, e3 := f.repo.CloseReport(ctx, "missing", "adm-1", store.ReportStatusActioned, "x")
		rec("close ok1=%v e1=%v ok2=%v e2=%v ok3=%v e3=%v", ok1, e1 != nil, ok2, e2 != nil, ok3, errors.Is(e3, store.ErrReportNotFound))
		create("a-again", newReport("u1", "L1"))
		all, total, _ := f.repo.ListReports(ctx, "", "", 0, 10)
		rec("list all total=%d", total)
		for _, r := range all {
			rec("  %s", norm(r, names))
		}
		pageOpen, total, _ := f.repo.ListReports(ctx, store.ReportStatusOpen, "", 1, 1)
		rec("list open page total=%d len=%d", total, len(pageOpen))
		_, err = f.repo.GetReport(ctx, "missing")
		rec("get missing notfound=%v", errors.Is(err, store.ErrReportNotFound))
		f.seedListing(t, "s1", "L1")
		h1, _ := f.repo.HasSellerListing(ctx, "s1", "L1")
		h2, _ := f.repo.HasSellerListing(ctx, "s1", "L2")
		rec("owns L1=%v L2=%v", h1, h2)
		sw, err := f.repo.SweepClosedBefore(ctx, time.Now().Add(time.Hour), 500)
		rec("sweep n=%d err=%v", sw, err != nil)
		swept, _ := f.repo.GetReport(ctx, first.ID)
		rec("after sweep %s", norm(swept, names))
		return log
	}
	m, p := run(mem), run(pg)
	if len(m) != len(p) {
		t.Fatalf("log length memory=%d postgres=%d\nmem=%v\npg=%v", len(m), len(p), m, p)
	}
	for i := range m {
		if m[i] != p[i] {
			t.Errorf("line %d differs\n memory:   %s\n postgres: %s", i, m[i], p[i])
		}
	}
	if len(m) == 0 {
		t.Fatal("empty log")
	}
}
