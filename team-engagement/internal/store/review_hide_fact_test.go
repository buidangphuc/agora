package store_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/store"
)

const reviewHiddenType = "platform.engagement.v1.ReviewHidden"

func seedHideReview(t *testing.T, pool *pgxpool.Pool, id, listing string) *store.PostgresReviewRepository {
	t.Helper()
	repo := store.NewPostgresReviewRepository(pool, testOutbox(t, pool))
	if _, err := repo.CreateReview(callerCtx(), store.Review{ID: id, ListingID: listing, UserID: "u-" + id, OrderID: "O-" + id, Rating: 4, Comment: "nice", VerifiedPurchase: true}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func hiddenRows(t *testing.T, pool *pgxpool.Pool) (keys []string, payloads [][]byte) {
	t.Helper()
	for _, f := range factsOfType(t, pool, reviewHiddenType) {
		if f.aggregateType != "Listing" {
			t.Fatalf("ReviewHidden aggregate type = %q, want Listing", f.aggregateType)
		}
		keys = append(keys, f.aggregateID)
		payloads = append(payloads, f.env.GetPayload())
	}
	return
}

// Scenario: Review hidden event: effective hide enqueues one fact
func TestHideReviewEnqueuesReviewHidden(t *testing.T) {
	pool := testPool(t)
	repo := seedHideReview(t, pool, "r1", "L1")
	changed, err := repo.HideReview(callerCtx(), "r1", "admin-1", "spam link")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	keys, payloads := hiddenRows(t, pool)
	if len(keys) != 1 || keys[0] != "L1" {
		t.Fatalf("ReviewHidden rows keys=%v, want exactly [L1]", keys)
	}
	var got engagementv1.ReviewHidden
	if err := proto.Unmarshal(payloads[0], &got); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetReview(callerCtx(), "r1")
	if err != nil || stored.HiddenAt == nil {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	if got.GetReviewId() != "r1" || got.GetListingId() != "L1" {
		t.Fatalf("payload ids = %q/%q", got.GetReviewId(), got.GetListingId())
	}
	if got.GetHiddenAt() == nil || !got.GetHiddenAt().AsTime().Truncate(time.Microsecond).Equal(stored.HiddenAt.Truncate(time.Microsecond)) {
		t.Fatalf("payload hidden_at = %v, stored = %v", got.GetHiddenAt(), stored.HiddenAt)
	}
}

// Scenario: Review hidden event: payload carries no moderator data
func TestReviewHiddenFactOmitsAdminAndReason(t *testing.T) {
	pool := testPool(t)
	repo := seedHideReview(t, pool, "r1", "L1")
	if _, err := repo.HideReview(callerCtx(), "r1", "admin-1", "spam link"); err != nil {
		t.Fatal(err)
	}
	_, payloads := hiddenRows(t, pool)
	if len(payloads) != 1 {
		t.Fatalf("ReviewHidden rows = %d, want 1", len(payloads))
	}
	if bytes.Contains(payloads[0], []byte("admin-1")) || bytes.Contains(payloads[0], []byte("spam link")) {
		t.Fatal("payload leaks admin id or reason")
	}
	var m engagementv1.ReviewHidden
	if err := proto.Unmarshal(payloads[0], &m); err != nil {
		t.Fatal(err)
	}
	if len(m.ProtoReflect().GetUnknown()) != 0 {
		t.Fatal("payload has unknown fields")
	}
	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if n := fd.Number(); n < 1 || n > 3 {
			t.Fatalf("field %d set", n)
		}
		return true
	})
}

// Scenario: Review hidden event: fact and hide are atomic
func TestHideReviewRollsBackWhenEnqueueFails(t *testing.T) {
	pool := testPool(t)
	repo := seedHideReview(t, pool, "r1", "L1")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
CREATE FUNCTION fail_review_hidden() RETURNS trigger AS $$ BEGIN
  IF NEW.event_type = 'platform.engagement.v1.ReviewHidden' THEN RAISE EXCEPTION 'outbox down'; END IF;
  RETURN NEW; END $$ LANGUAGE plpgsql;
CREATE TRIGGER fail_review_hidden BEFORE INSERT ON engagement_outbox_events FOR EACH ROW EXECUTE FUNCTION fail_review_hidden();`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.HideReview(callerCtx(), "r1", "admin-1", "spam"); err == nil {
		t.Fatal("HideReview returned nil error although the enqueue failed")
	}
	var hidden, rows int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE hidden_at IS NOT NULL`).Scan(&hidden)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM engagement_outbox_events WHERE event_type=$1`, reviewHiddenType).Scan(&rows)
	if hidden != 0 || rows != 0 {
		t.Fatalf("hidden rows=%d outbox rows=%d, want 0/0", hidden, rows)
	}
}

// Scenario: Review hidden event: re-hide emits nothing new
func TestHideReviewTwiceEnqueuesOnce(t *testing.T) {
	pool := testPool(t)
	repo := seedHideReview(t, pool, "r1", "L1")
	if changed, err := repo.HideReview(callerCtx(), "r1", "admin-1", "spam link"); err != nil || !changed {
		t.Fatalf("first changed=%v err=%v", changed, err)
	}
	first, _ := repo.GetReview(callerCtx(), "r1")
	changed, err := repo.HideReview(callerCtx(), "r1", "admin-2", "other")
	if err != nil || changed {
		t.Fatalf("second changed=%v err=%v, want false,nil", changed, err)
	}
	again, _ := repo.GetReview(callerCtx(), "r1")
	if again.HiddenBy != "admin-1" || again.HideReason != "spam link" || !again.HiddenAt.Equal(*first.HiddenAt) {
		t.Fatalf("state changed on re-hide: %+v", again)
	}
	if keys, _ := hiddenRows(t, pool); len(keys) != 1 {
		t.Fatalf("ReviewHidden rows = %d, want 1", len(keys))
	}
}

// Scenario: Review hidden event: concurrent hides emit once
func TestHideReviewConcurrentEnqueuesOnce(t *testing.T) {
	pool := testPool(t)
	repo := seedHideReview(t, pool, "r1", "L1")
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, admin := range []string{"admin-1", "admin-2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.HideReview(callerCtx(), "r1", admin, "dup")
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent hide error: %v", err)
		}
	}
	if keys, _ := hiddenRows(t, pool); len(keys) != 1 {
		t.Fatalf("ReviewHidden rows = %d, want 1", len(keys))
	}
}

// Scenario: Review hidden event: unknown review emits nothing
func TestHideReviewUnknownEnqueuesNothing(t *testing.T) {
	pool := testPool(t)
	repo := store.NewPostgresReviewRepository(pool, testOutbox(t, pool))
	if _, err := repo.HideReview(callerCtx(), "missing", "admin-1", "x"); !errors.Is(err, store.ErrReviewNotFound) {
		t.Fatalf("err = %v, want ErrReviewNotFound", err)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM engagement_outbox_events`).Scan(&n)
	if n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: Engagement SDK: Hiding a review writes ReviewHidden without moderator data
func TestHideReviewEnqueuesReviewHiddenOnce(t *testing.T) {
	pool := testPool(t)
	repo := seedHideReview(t, pool, "R1", "L1")

	changed, err := repo.HideReview(callerCtx(), "R1", "admin-1", "spam link")
	if err != nil || !changed {
		t.Fatalf("hide: changed=%v err=%v", changed, err)
	}
	rows := factsOfType(t, pool, reviewHiddenType)
	if len(rows) != 1 {
		t.Fatalf("ReviewHidden rows = %d, want 1", len(rows))
	}
	if rows[0].aggregateType != "Listing" || rows[0].aggregateID != "L1" {
		t.Fatalf("row key = %s/%s, want Listing/L1", rows[0].aggregateType, rows[0].aggregateID)
	}
	if rows[0].env.GetType() != reviewHiddenType {
		t.Fatalf("envelope type = %q", rows[0].env.GetType())
	}
	payload := rows[0].env.GetPayload()
	var got engagementv1.ReviewHidden
	if err := proto.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetReviewId() != "R1" || got.GetListingId() != "L1" || got.GetHiddenAt() == nil {
		t.Fatalf("payload = %v, want review_id R1, listing_id L1 and hidden_at", &got)
	}
	for _, leak := range []string{"admin-1", "spam link", "u-R1", "nice"} {
		if bytes.Contains(payload, []byte(leak)) {
			t.Fatalf("payload leaks %q", leak)
		}
	}

	if changed, err := repo.HideReview(callerCtx(), "R1", "admin-2", "again"); err != nil || changed {
		t.Fatalf("second hide: changed=%v err=%v, want false,nil", changed, err)
	}
	if n := len(factsOfType(t, pool, reviewHiddenType)); n != 1 {
		t.Fatalf("ReviewHidden rows after second hide = %d, want 1", n)
	}
}
