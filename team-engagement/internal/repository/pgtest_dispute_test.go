package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

const disputeResolvedType = "platform.engagement.v1.DisputeResolved"

func disputeRow(id, order, returnID string, st DisputeStatus, at time.Time) Dispute {
	return Dispute{ID: id, OrderID: order, ClaimantID: "buyer-1", DefendantID: "seller-1",
		Reason: "not as described", ReturnID: returnID, Status: st, CreatedAt: at, UpdatedAt: at}
}

func resolvedFact(d Dispute) *engagementv1.DisputeResolved {
	return &engagementv1.DisputeResolved{
		DisputeId: d.ID, OrderId: d.OrderID, ReturnId: d.ReturnID,
		Status:     engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
		Ruling:     engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR,
		ResolverId: "admin-1",
	}
}

func disputeFactPayloads(t *testing.T, pool *pgxpool.Pool) []*engagementv1.DisputeResolved {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload FROM outbox WHERE type=$1 ORDER BY seq`, disputeResolvedType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []*engagementv1.DisputeResolved
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		var m engagementv1.DisputeResolved
		if err := proto.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, &m)
	}
	return out
}

func disputeCount(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM disputes WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Scenario: Dispute escalation: Concurrent creates leave one dispute
func TestCreateDisputeUniqueOpenPerReturnConcurrent(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.CreateDispute(ctx, Dispute{OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "x", ReturnID: "r-1"})
		}()
	}
	close(start)
	wg.Wait()

	ok, dup := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrOpenDisputeExists):
			dup++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 4 {
		t.Fatalf("ok=%d duplicate=%d, want 1 and 4", ok, dup)
	}
	if n := disputeCount(t, pool, `return_id=$1`, "r-1"); n != 1 {
		t.Fatalf("dispute rows for r-1 = %d, want 1", n)
	}
}

// Scenario: Dispute escalation: Second dispute on the same return is refused while one is open
func TestCreateDisputeRefusedWhileOpenOrInvestigating(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()
	in := Dispute{OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "x", ReturnID: "r-1"}
	d, err := repo.CreateDispute(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDispute(ctx, in); !errors.Is(err, ErrOpenDisputeExists) {
		t.Fatalf("OPEN duplicate err=%v, want ErrOpenDisputeExists", err)
	}
	d.Status = DisputeStatusInvestigating
	if _, err := repo.UpdateDispute(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDispute(ctx, in); !errors.Is(err, ErrOpenDisputeExists) {
		t.Fatalf("INVESTIGATING duplicate err=%v, want ErrOpenDisputeExists", err)
	}
	if n := disputeCount(t, pool, `return_id=$1`, "r-1"); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

// Scenario: Dispute escalation: A closed dispute lets the return be escalated again
func TestCreateDisputeAfterClosedDisputeOnReturn(t *testing.T) {
	for _, closed := range []DisputeStatus{DisputeStatusRejected, DisputeStatusResolved} {
		t.Run(string(closed), func(t *testing.T) {
			pool := testPool(t)
			repo := NewPostgresDisputeRepository(pool)
			ctx := callerCtx()
			in := Dispute{OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "x", ReturnID: "r-1"}
			d, err := repo.CreateDispute(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			d.Status = closed
			if _, err := repo.UpdateDispute(ctx, d); err != nil {
				t.Fatal(err)
			}
			again, err := repo.CreateDispute(ctx, in)
			if err != nil {
				t.Fatalf("create after %s: %v", closed, err)
			}
			if again.Status != DisputeStatusOpen || again.ID == d.ID {
				t.Fatalf("new dispute = %+v", again)
			}
			if n := disputeCount(t, pool, `return_id=$1`, "r-1"); n != 2 {
				t.Fatalf("rows for r-1 = %d, want 2", n)
			}
		})
	}
}

// Scenario: Dispute escalation: Dispute without return_id is unchanged
func TestCreateDisputeWithoutReturnIDNeverCollides(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	for i := 0; i < 3; i++ {
		if _, err := repo.CreateDispute(callerCtx(), Dispute{OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "x"}); err != nil {
			t.Fatalf("create %d without return_id: %v", i, err)
		}
	}
	if n := disputeCount(t, pool, `order_id=$1`, "o-1"); n != 3 {
		t.Fatalf("rows = %d, want 3", n)
	}
}

// Scenario: Dispute escalation: GetDispute shows the link and the ruling (storage round trip)
func TestDisputeReturnIDAndRulingRoundTrip(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()
	d, err := repo.CreateDispute(ctx, Dispute{OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "x", ReturnID: "r-9", Status: DisputeStatusInvestigating})
	if err != nil {
		t.Fatal(err)
	}
	d.Status = DisputeStatusResolved
	d.Ruling = engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR
	d.Resolution = "item matches listing"
	if _, err := repo.ResolveWithEvent(ctx, d, DisputeStatusInvestigating, resolvedFact(d)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetDispute(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReturnID != "r-9" || got.Ruling != engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR ||
		got.Status != DisputeStatusResolved || got.Resolution != "item matches listing" {
		t.Fatalf("stored = %+v", got)
	}
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool, pick func(base string) bool) {
	t.Helper()
	files, _ := filepath.Glob("../../migrations/*.up.sql")
	sort.Strings(files)
	for _, f := range files {
		if !pick(filepath.Base(f)) {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(b)); err != nil {
			t.Fatalf("migrate %s: %v", filepath.Base(f), err)
		}
	}
}

func disputeColumns0013(t *testing.T, pool *pgxpool.Pool) map[string]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT column_name FROM information_schema.columns WHERE table_name='disputes'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols[c] = true
	}
	return cols
}

// Scenario: Dispute escalation: Existing disputes survive the migration
func TestDisputeMigrationKeepsExistingRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	applyMigrations(t, pool, func(b string) bool { return !strings.HasPrefix(b, "0013_") })
	if _, err := pool.Exec(ctx, `INSERT INTO disputes (id, order_id, claimant_id, defendant_id, reason, status)
		VALUES ('old-1','o-old','buyer-1','seller-1','broken on arrival','INVESTIGATING')`); err != nil {
		t.Fatal(err)
	}

	up, err := os.ReadFile("../../migrations/0013_dispute_escalation.up.sql")
	if err != nil {
		t.Fatalf("0013 up migration missing: %v", err)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("0013 up: %v", err)
	}

	got, err := NewPostgresDisputeRepository(pool).GetDispute(callerCtx(), "old-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "old-1" || got.Status != DisputeStatusInvestigating || got.Reason != "broken on arrival" ||
		got.ReturnID != "" || got.Ruling != engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		t.Fatalf("migrated row = %+v", got)
	}

	defs := strings.ToLower(strings.Join(indexDefs(t, pool, "disputes"), "\n"))
	if !strings.Contains(defs, "unique") || !strings.Contains(defs, "(return_id)") || !strings.Contains(defs, "where") {
		t.Errorf("missing partial unique index on return_id; have:\n%s", defs)
	}
	if !strings.Contains(defs, "(status, created_at") {
		t.Errorf("missing (status, created_at) index; have:\n%s", defs)
	}

	before := disputeColumns0013(t, pool)
	if !before["return_id"] || !before["ruling"] {
		t.Fatalf("columns after up = %v", before)
	}

	down, err := os.ReadFile("../../migrations/0013_dispute_escalation.down.sql")
	if err != nil {
		t.Fatalf("0013 down migration missing: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("0013 down: %v", err)
	}
	after := disputeColumns0013(t, pool)
	if after["return_id"] || after["ruling"] {
		t.Fatalf("columns after down = %v, want return_id and ruling gone", after)
	}
	for c := range before {
		if c != "return_id" && c != "ruling" && !after[c] {
			t.Errorf("down migration dropped unrelated column %q", c)
		}
	}
	if n := disputeCount(t, pool, `id='old-1' AND status='INVESTIGATING'`); n != 1 {
		t.Fatalf("old row after down: count=%d, want 1", n)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("0013 up is not re-runnable after down: %v", err)
	}
}

func indexDefs(t *testing.T, pool *pgxpool.Pool, table string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT indexdef FROM pg_indexes WHERE tablename=$1`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func seedInvestigating(t *testing.T, repo *PostgresDisputeRepository) Dispute {
	t.Helper()
	d, err := repo.CreateDispute(callerCtx(), Dispute{OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "x", ReturnID: "r-1", Status: DisputeStatusInvestigating})
	if err != nil {
		t.Fatal(err)
	}
	d.Status = DisputeStatusResolved
	d.Ruling = engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR
	d.Resolution = "refund"
	return d
}

// Scenario: Dispute escalation: Concurrent resolutions apply once
func TestResolveDisputeCompareAndSetOnce(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	d := seedInvestigating(t, repo)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.ResolveWithEvent(callerCtx(), d, DisputeStatusInvestigating, resolvedFact(d))
		}()
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("successful resolutions = %d (errs %v), want 1", wins, errs)
	}
	facts := disputeFactPayloads(t, pool)
	if len(facts) != 1 || facts[0].GetDisputeId() != d.ID || facts[0].GetReturnId() != "r-1" ||
		facts[0].GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR {
		t.Fatalf("DisputeResolved rows = %v, want exactly one for %s", facts, d.ID)
	}
	got, _ := repo.GetDispute(callerCtx(), d.ID)
	if got.Status != DisputeStatusResolved || got.Ruling != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR {
		t.Fatalf("stored = %+v", got)
	}
}

// Scenario: Dispute escalation: Closed dispute stays closed
func TestResolveWithEventStalePreviousStatusWritesNothing(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	d := seedInvestigating(t, repo)
	if _, err := repo.ResolveWithEvent(callerCtx(), d, DisputeStatusInvestigating, resolvedFact(d)); err != nil {
		t.Fatal(err)
	}
	d.Ruling = engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR
	d.Resolution = "changed my mind"
	if _, err := repo.ResolveWithEvent(callerCtx(), d, DisputeStatusInvestigating, resolvedFact(d)); err == nil {
		t.Fatal("second resolution on a closed dispute succeeded, want error")
	}
	if n := len(disputeFactPayloads(t, pool)); n != 1 {
		t.Fatalf("DisputeResolved rows = %d, want 1", n)
	}
	got, _ := repo.GetDispute(callerCtx(), d.ID)
	if got.Ruling != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR || got.Resolution != "refund" {
		t.Fatalf("closed dispute was modified: %+v", got)
	}
}

// Scenario: Dispute escalation: A failed update leaves no event
func TestResolveDisputeUpdateFailureWritesNoFact(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := context.Background()
	d := seedInvestigating(t, repo)

	if _, err := pool.Exec(ctx, `
CREATE FUNCTION fail_dispute_update() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'injected update failure'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER fail_dispute_update BEFORE UPDATE ON disputes FOR EACH ROW EXECUTE FUNCTION fail_dispute_update()`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveWithEvent(callerCtx(), d, DisputeStatusInvestigating, resolvedFact(d)); err == nil {
		t.Fatal("ResolveWithEvent succeeded despite failing update")
	}
	if n := len(disputeFactPayloads(t, pool)); n != 0 {
		t.Fatalf("DisputeResolved rows after failed update = %d, want 0", n)
	}
	if got, _ := repo.GetDispute(callerCtx(), d.ID); got.Status != DisputeStatusInvestigating {
		t.Fatalf("status after failed update = %s, want INVESTIGATING", got.Status)
	}

	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_dispute_update ON disputes`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveWithEvent(callerCtx(), d, DisputeStatusInvestigating, resolvedFact(d)); err != nil {
		t.Fatalf("retry on healthy store: %v", err)
	}
	if n := len(disputeFactPayloads(t, pool)); n != 1 {
		t.Fatalf("DisputeResolved rows after retry = %d, want 1", n)
	}
}

func seedForList(t *testing.T, repo *PostgresDisputeRepository, n int, st func(i int) DisputeStatus, at func(i int) time.Time) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := repo.CreateDispute(callerCtx(), disputeRow(fmt.Sprintf("d-%03d", i), "o-1", "", st(i), at(i))); err != nil {
			t.Fatal(err)
		}
	}
}

var listBase = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// Scenario: Dispute escalation: Admin lists open disputes oldest first
func TestListDisputesByStatusOldestFirst(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()
	rows := []struct {
		id string
		st DisputeStatus
		at int
	}{
		{"d-c", DisputeStatusOpen, 30}, {"d-x", DisputeStatusResolved, 10}, {"d-a", DisputeStatusOpen, 10},
		{"d-y", DisputeStatusResolved, 20}, {"d-b", DisputeStatusOpen, 20},
	}
	for _, r := range rows {
		if _, err := repo.CreateDispute(ctx, disputeRow(r.id, "o-"+r.id, "", r.st, listBase.Add(time.Duration(r.at)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	got, next, err := repo.ListByStatus(ctx, DisputeStatusOpen, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "d-a" || got[1].ID != "d-b" || got[2].ID != "d-c" || next != "" {
		t.Fatalf("OPEN list = %v next=%q, want d-a,d-b,d-c and no cursor", ids(got), next)
	}
	all, _, err := repo.ListByStatus(ctx, "", 20, "")
	if err != nil || len(all) != 5 {
		t.Fatalf("unspecified status: len=%d err=%v, want 5", len(all), err)
	}
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.Before(all[i-1].CreatedAt) {
			t.Fatalf("not oldest first: %v", ids(all))
		}
	}
	none, next, err := repo.ListByStatus(ctx, DisputeStatusRejected, 20, "")
	if err != nil || len(none) != 0 || next != "" {
		t.Fatalf("empty status: %v next=%q err=%v", ids(none), next, err)
	}
}

func ids(ds []Dispute) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	return out
}

// Scenario: Dispute escalation: Pagination boundary
func TestListDisputesPagination(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()
	same := listBase
	seedForList(t, repo, 3, func(int) DisputeStatus { return DisputeStatusOpen }, func(int) time.Time { return same })

	p1, c1, err := repo.ListByStatus(ctx, DisputeStatusOpen, 2, "")
	if err != nil || len(p1) != 2 || c1 == "" {
		t.Fatalf("page1 len=%d cursor=%q err=%v, want 2 with cursor", len(p1), c1, err)
	}
	p2, c2, err := repo.ListByStatus(ctx, DisputeStatusOpen, 2, c1)
	if err != nil || len(p2) != 1 || c2 != "" {
		t.Fatalf("page2 len=%d cursor=%q err=%v, want 1 without cursor", len(p2), c2, err)
	}
	seen := map[string]bool{}
	for _, d := range append(p1, p2...) {
		if seen[d.ID] {
			t.Fatalf("dispute %s returned twice across pages", d.ID)
		}
		seen[d.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("pages covered %d disputes, want 3", len(seen))
	}

	exact, c3, err := repo.ListByStatus(ctx, DisputeStatusOpen, 3, "")
	if err != nil || len(exact) != 3 || c3 != "" {
		t.Fatalf("page_size == total: len=%d cursor=%q err=%v, want 3 and no cursor", len(exact), c3, err)
	}
}

// Scenario: Dispute escalation: Pagination boundary (default and clamp)
func TestListDisputesPageSizeDefaultAndClamp(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()
	seedForList(t, repo, 105, func(int) DisputeStatus { return DisputeStatusOpen },
		func(i int) time.Time { return listBase.Add(time.Duration(i) * time.Second) })

	def, next, err := repo.ListByStatus(ctx, DisputeStatusOpen, 0, "")
	if err != nil || len(def) != 20 || next == "" {
		t.Fatalf("default page: len=%d cursor=%q err=%v, want 20 with cursor", len(def), next, err)
	}
	big, next, err := repo.ListByStatus(ctx, DisputeStatusOpen, 500, "")
	if err != nil || len(big) != 100 || next == "" {
		t.Fatalf("page_size 500: len=%d cursor=%q err=%v, want 100 with cursor", len(big), next, err)
	}
	rest, next, err := repo.ListByStatus(ctx, DisputeStatusOpen, 500, next)
	if err != nil || len(rest) != 5 || next != "" {
		t.Fatalf("rest: len=%d cursor=%q err=%v, want 5 without cursor", len(rest), next, err)
	}
}

// Scenario: Dispute escalation: Buyer and seller list the order's disputes newest first
func TestListDisputesByOrderNewestFirst(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresDisputeRepository(pool)
	ctx := callerCtx()
	for i, o := range []string{"o-1", "o-2", "o-1", "o-1"} {
		if _, err := repo.CreateDispute(ctx, disputeRow(fmt.Sprintf("d-%d", i), o, "", DisputeStatusOpen, listBase.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListByOrder(ctx, "o-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "d-3" || got[1].ID != "d-2" || got[2].ID != "d-0" {
		t.Fatalf("ListByOrder = %v, want d-3,d-2,d-0", ids(got))
	}
	empty, err := repo.ListByOrder(ctx, "o-none")
	if err != nil || len(empty) != 0 {
		t.Fatalf("order without disputes: %v err=%v", ids(empty), err)
	}
}
