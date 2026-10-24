package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-domain/internal/repository"
)

func modWriter(pool *pgxpool.Pool) *repository.PgTxWriter {
	return repository.NewPgTxWriter(pool, repository.NewOutboxStore(pool))
}

func modEv(id string) repository.EnqueueFn {
	return func(l repository.Listing) (repository.OutboxRow, error) {
		return repository.OutboxRow{
			EventID: uuid.NewString(), AggregateType: "Listing", AggregateID: id,
			EventType: "platform.listing.v1.ListingChanged", Payload: []byte{0x01},
		}, nil
	}
}

func outboxCount(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}

// seedStatus creates a listing and forces its status/submitted_at/flags via SQL.
func seedStatus(t *testing.T, repo *repository.PostgresListingRepository, pool *pgxpool.Pool,
	id, status string, submitted time.Time, flags []string) {
	t.Helper()
	seedPGListing(t, repo, id, 1)
	if flags == nil {
		flags = []string{}
	}
	var sub any
	if !submitted.IsZero() {
		sub = submitted
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE listings SET status = $2, submitted_at = $3, screen_flags = $4 WHERE id = $1`,
		id, status, sub, flags); err != nil {
		t.Fatalf("force status %s: %v", id, err)
	}
}

func drain(t *testing.T, repo *repository.PostgresListingRepository, size int32) (pages [][]string) {
	t.Helper()
	cursor := ""
	for i := 0; i < 20; i++ {
		p, err := repo.ListModerationQueue(context.Background(), cursor, size)
		if err != nil {
			t.Fatalf("ListModerationQueue: %v", err)
		}
		var ids []string
		for _, it := range p.Items {
			ids = append(ids, it.ID)
		}
		pages = append(pages, ids)
		if p.NextCursor == "" {
			return pages
		}
		cursor = p.NextCursor
	}
	t.Fatal("queue pagination did not terminate")
	return nil
}

// Scenario: Status ownership: Seller write racing a moderation decision is refused
func TestUpdateTxConditionalOnLoadedStatus(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := context.Background()
	w := modWriter(pool)
	seedStatus(t, repo, pool, "L1", "pending_review", time.Now().UTC(), nil)

	loaded, err := repo.Get(ctx, "L1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatal("Get must populate UpdatedAt from listings.updated_at")
	}

	// Admin BAN commits between the seller's read and the seller's transaction.
	if _, err := w.ModerateTx(ctx, "L1", "pending_review", loaded.UpdatedAt, repository.ModerationUpdate{
		ToStatus: "banned", ReasonCode: "PROHIBITED", Note: "n", DecidedAt: time.Now().UTC(),
	}, modEv("L1")); err != nil {
		t.Fatalf("ModerateTx ban: %v", err)
	}
	before := outboxCount(t, pool, "L1")

	stale := loaded
	stale.Title = "seller edit"
	stale.Status = "pending_review"
	stale.LoadedStatus = loaded.Status
	stale.LoadedUpdatedAt = loaded.UpdatedAt
	_, err = w.UpdateTx(ctx, stale, modEv("L1"))
	if !errors.Is(err, repository.ErrStatusChanged) {
		t.Fatalf("stale seller UpdateTx err = %v, want ErrStatusChanged", err)
	}
	got, _ := repo.Get(ctx, "L1")
	if got.Status != "banned" || got.Title == "seller edit" {
		t.Fatalf("stored = status %q title %q, want banned and untouched title", got.Status, got.Title)
	}
	if after := outboxCount(t, pool, "L1"); after != before {
		t.Fatalf("outbox rows %d -> %d, want no row for the refused seller write", before, after)
	}
}

// Scenario: Status ownership: Seller write racing a moderation decision is refused
// (control: an unraced conditional seller write still succeeds and writes one row)
func TestUpdateTxConditionalOnLoadedStatusFreshSucceeds(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := context.Background()
	w := modWriter(pool)
	seedStatus(t, repo, pool, "L2", "draft", time.Time{}, nil)
	loaded, err := repo.Get(ctx, "L2")
	if err != nil {
		t.Fatal(err)
	}
	loaded.Title = "edited"
	loaded.LoadedStatus = loaded.Status
	loaded.LoadedUpdatedAt = loaded.UpdatedAt
	out, err := w.UpdateTx(ctx, loaded, modEv("L2"))
	if err != nil {
		t.Fatalf("fresh UpdateTx: %v", err)
	}
	if out.Title != "edited" || outboxCount(t, pool, "L2") != 1 {
		t.Fatalf("title=%q outbox=%d, want edited and 1 row", out.Title, outboxCount(t, pool, "L2"))
	}
	// Reusing the now-stale updated_at is refused: one change applies once.
	again := loaded
	again.Title = "second"
	if _, err := w.UpdateTx(ctx, again, modEv("L2")); !errors.Is(err, repository.ErrStatusChanged) {
		t.Fatalf("stale reuse err = %v, want ErrStatusChanged", err)
	}
}

// Scenario: Moderation: Concurrent decision applies once
func TestModerateTxConditionalOnLoadedStatus(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := context.Background()
	w := modWriter(pool)
	seedStatus(t, repo, pool, "L3", "pending_review", time.Now().UTC(), []string{"kw"})
	loaded, err := repo.Get(ctx, "L3")
	if err != nil {
		t.Fatal(err)
	}

	decisions := []repository.ModerationUpdate{
		{ToStatus: "published", ReasonCode: "OK", DecidedAt: time.Now().UTC()},
		{ToStatus: "rejected", ReasonCode: "NO", Note: "x", DecidedAt: time.Now().UTC()},
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range decisions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = w.ModerateTx(ctx, "L3", "pending_review", loaded.UpdatedAt, decisions[i], modEv("L3"))
		}(i)
	}
	wg.Wait()

	winner := -1
	for i, e := range errs {
		switch {
		case e == nil:
			if winner != -1 {
				t.Fatal("both decisions succeeded")
			}
			winner = i
		case !errors.Is(e, repository.ErrStatusChanged):
			t.Fatalf("loser err = %v, want ErrStatusChanged", e)
		}
	}
	if winner == -1 {
		t.Fatal("no decision succeeded")
	}
	got, _ := repo.Get(ctx, "L3")
	if got.Status != decisions[winner].ToStatus {
		t.Fatalf("status = %q, want winner's %q", got.Status, decisions[winner].ToStatus)
	}
	if got.ModerationReasonCode != decisions[winner].ReasonCode || got.ModerationDecidedAt.IsZero() {
		t.Fatalf("moderation columns = %q / %v, want winner's reason and a decided_at", got.ModerationReasonCode, got.ModerationDecidedAt)
	}
	if n := outboxCount(t, pool, "L3"); n != 1 {
		t.Fatalf("outbox rows = %d, want exactly 1", n)
	}
}

// Scenario: Moderation: Concurrent decision applies once (wrong from-status is refused too)
func TestModerateTxRefusesWrongFromStatus(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := context.Background()
	w := modWriter(pool)
	seedStatus(t, repo, pool, "L4", "published", time.Time{}, nil)
	loaded, _ := repo.Get(ctx, "L4")
	_, err := w.ModerateTx(ctx, "L4", "pending_review", loaded.UpdatedAt,
		repository.ModerationUpdate{ToStatus: "banned", ReasonCode: "X", DecidedAt: time.Now().UTC()}, modEv("L4"))
	if !errors.Is(err, repository.ErrStatusChanged) {
		t.Fatalf("err = %v, want ErrStatusChanged", err)
	}
	if got, _ := repo.Get(ctx, "L4"); got.Status != "published" || outboxCount(t, pool, "L4") != 0 {
		t.Fatalf("status %q outbox %d, want published and 0", got.Status, outboxCount(t, pool, "L4"))
	}
}

// Scenario: Queue: Oldest submission first
func TestListModerationQueueOrderedBySubmission(t *testing.T) {
	repo, pool := newPGRepo(t)
	base := time.Now().UTC().Truncate(time.Second)
	// Inserted out of order; flags must round-trip.
	seedStatus(t, repo, pool, "q-t3", "pending_review", base.Add(3*time.Minute), []string{"f3"})
	seedStatus(t, repo, pool, "q-t1", "pending_review", base.Add(1*time.Minute), []string{"f1", "f1b"})
	seedStatus(t, repo, pool, "q-t2", "pending_review", base.Add(2*time.Minute), []string{"f2"})

	p, err := repo.ListModerationQueue(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	var ids []string
	for _, it := range p.Items {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, ",") != "q-t1,q-t2,q-t3" {
		t.Fatalf("order = %v, want q-t1,q-t2,q-t3", ids)
	}
	if len(p.Items) == 3 {
		if got := strings.Join(p.Items[0].ScreenFlags, "|"); got != "f1|f1b" {
			t.Fatalf("flags = %q, want f1|f1b", got)
		}
		if !p.Items[0].SubmittedAt.Equal(base.Add(time.Minute)) {
			t.Fatalf("submitted_at = %v, want %v", p.Items[0].SubmittedAt, base.Add(time.Minute))
		}
	}
}

// Scenario: Queue: Oldest submission first (id tie-break on equal submitted_at)
func TestListModerationQueueTieBreaksOnID(t *testing.T) {
	repo, pool := newPGRepo(t)
	ts := time.Now().UTC().Truncate(time.Second)
	for _, id := range []string{"tie-c", "tie-a", "tie-b"} {
		seedStatus(t, repo, pool, id, "pending_review", ts, nil)
	}
	pages := drain(t, repo, 2)
	var all []string
	for _, p := range pages {
		all = append(all, p...)
	}
	if strings.Join(all, ",") != "tie-a,tie-b,tie-c" {
		t.Fatalf("order across pages = %v, want tie-a,tie-b,tie-c", all)
	}
}

// Scenario: Queue: Only pending listings appear
func TestListModerationQueueOnlyPending(t *testing.T) {
	repo, pool := newPGRepo(t)
	now := time.Now().UTC()
	for _, st := range []string{"draft", "published", "rejected", "banned", "held", "pending_review"} {
		seedStatus(t, repo, pool, "s-"+st, st, now, nil)
	}
	p, err := repo.ListModerationQueue(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].ID != "s-pending_review" || p.Items[0].Status != "pending_review" {
		t.Fatalf("items = %+v, want exactly s-pending_review", p.Items)
	}
}

// Scenario: Queue: Empty queue (repository level: empty page, empty cursor)
func TestListModerationQueueEmptyStore(t *testing.T) {
	repo, _ := newPGRepo(t)
	p, err := repo.ListModerationQueue(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if len(p.Items) != 0 || p.NextCursor != "" {
		t.Fatalf("page = %d items cursor %q, want empty", len(p.Items), p.NextCursor)
	}
}

// Scenario: Queue: Pagination boundary
func TestListModerationQueueKeysetPages(t *testing.T) {
	repo, pool := newPGRepo(t)
	base := time.Now().UTC().Truncate(time.Second)
	for i := 1; i <= 5; i++ {
		seedStatus(t, repo, pool, fmt.Sprintf("p-%d", i), "pending_review", base.Add(time.Duration(i)*time.Minute), nil)
	}
	first, err := repo.ListModerationQueue(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if first.NextCursor == "" {
		t.Fatal("first page must carry a next_cursor")
	}
	pages := drain(t, repo, 2)
	sizes := make([]int, len(pages))
	var seen []string
	for i, p := range pages {
		sizes[i] = len(p)
		seen = append(seen, p...)
	}
	if fmt.Sprint(sizes) != "[2 2 1]" {
		t.Fatalf("page sizes = %v, want [2 2 1]", sizes)
	}
	if strings.Join(seen, ",") != "p-1,p-2,p-3,p-4,p-5" {
		t.Fatalf("seen = %v, want p-1..p-5 each once in order", seen)
	}
	last, _ := repo.ListModerationQueue(context.Background(), "", 5)
	if last.NextCursor != "" || len(last.Items) != 5 {
		t.Fatalf("exact-fit page: %d items cursor %q, want 5 and empty cursor", len(last.Items), last.NextCursor)
	}
}

// Scenario: Queue: Page latency at 10k pending
func TestListModerationQueueIndexAndLatency(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO listings (id, title, price, currency, status, stock)
		SELECT 'bulk-' || lpad(g::text, 6, '0'), 't', 0, 'VND', 'pending_review', 1 FROM generate_series(1, 10000) g`); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE listings SET submitted_at = now() - (random() * interval '30 days'), screen_flags = '{}'
		WHERE id LIKE 'bulk-%'`); err != nil {
		t.Fatalf("bulk submitted_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE listings`); err != nil {
		t.Fatal(err)
	}

	// A partial index on (submitted_at, id) restricted to pending_review exists.
	var idxName, idxDef string
	err := pool.QueryRow(ctx, `SELECT indexname, indexdef FROM pg_indexes
		WHERE tablename = 'listings' AND schemaname = current_schema()
		  AND indexdef ILIKE '%submitted_at%' AND indexdef ILIKE '%pending_review%'`).Scan(&idxName, &idxDef)
	if err != nil {
		t.Fatalf("no partial submitted_at index on pending_review: %v", err)
	}
	if !strings.Contains(strings.ReplaceAll(idxDef, `"`, ""), "submitted_at, id") {
		t.Fatalf("index %q = %s, want columns (submitted_at, id)", idxName, idxDef)
	}

	// The queue query shape plans onto that index.
	rows, err := pool.Query(ctx, `EXPLAIN SELECT id FROM listings WHERE status = 'pending_review' ORDER BY submitted_at, id LIMIT 51`)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	var plan []string
	for rows.Next() {
		var line string
		_ = rows.Scan(&line)
		plan = append(plan, line)
	}
	rows.Close()
	if !strings.Contains(strings.Join(plan, "\n"), idxName) {
		t.Fatalf("plan does not use %s:\n%s", idxName, strings.Join(plan, "\n"))
	}

	// p95 over 20 page requests of 50 <= 200ms.
	var durs []time.Duration
	for i := 0; i < 20; i++ {
		start := time.Now()
		p, err := repo.ListModerationQueue(ctx, "", 50)
		if err != nil {
			t.Fatalf("ListModerationQueue: %v", err)
		}
		durs = append(durs, time.Since(start))
		if len(p.Items) != 50 || p.NextCursor == "" {
			t.Fatalf("page = %d items cursor %q, want 50 and a cursor", len(p.Items), p.NextCursor)
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	if p95 := durs[18]; p95 > 200*time.Millisecond {
		t.Fatalf("p95 = %v, want <= 200ms", p95)
	}
}
