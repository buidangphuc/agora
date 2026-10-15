package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
)

// Reviews created in the same instant must still page deterministically: the
// id is the tiebreak behind created_at DESC.
func TestPostgresListReviewsStableOrderOnTimestampTies(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ts := time.Now().Truncate(time.Microsecond)
	for i := 1; i <= 25; i++ {
		_, err := pool.Exec(ctx, `INSERT INTO reviews (id, listing_id, user_id, user_name, order_id, rating, comment, created_at)
			VALUES ($1,'l1','u','U','',5,'',$2)`, fmt.Sprintf("id-%02d", i), ts)
		if err != nil {
			t.Fatal(err)
		}
	}
	repo := NewPostgresReviewRepository(pool)
	var seen []string
	for off := 0; off < 25; off += 10 {
		revs, total, err := repo.ListReviews(ctx, "l1", 0, off, 10)
		if err != nil || total != 25 {
			t.Fatalf("offset %d: total=%d err=%v", off, total, err)
		}
		for _, r := range revs {
			seen = append(seen, r.ID)
		}
	}
	if len(seen) != 25 {
		t.Fatalf("saw %d reviews", len(seen))
	}
	for i, id := range seen {
		if want := fmt.Sprintf("id-%02d", 25-i); id != want {
			t.Fatalf("position %d = %s, want %s", i, id, want)
		}
	}
}

// ── engagement-review-integrity G1 ───────────────────────────────────

// reviewRepo is the repository surface the integrity rules apply to; both the
// Postgres and in-memory implementations must satisfy it.
type reviewRepo interface {
	CreateReview(ctx context.Context, r Review) (Review, error)
	ListReviews(ctx context.Context, listingID string, ratingFilter int32, offset, limit int) ([]Review, int64, error)
	GetRatingSummary(ctx context.Context, listingID string) (RatingSummary, error)
	GetShopRatingSummary(ctx context.Context, sellerID string) (ShopRatingSummary, error)
	MarkHelpful(ctx context.Context, reviewID, userID string) (int64, error)
	GetReview(ctx context.Context, id string) (Review, error)
	HideReview(ctx context.Context, id, adminID, reason string) (bool, error)
}

func migrationFiles(t *testing.T, suffix string) []string {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*." + suffix + ".sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no %s migrations found: %v", suffix, err)
	}
	sort.Strings(files)
	return files
}

// poolBefore0011 returns a fresh database migrated up to 0010 only.
func poolBefore0011(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	for _, f := range migrationFiles(t, "up") {
		if strings.HasPrefix(filepath.Base(f), "0011") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("migrate %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

func run0011(t *testing.T, pool *pgxpool.Pool, suffix string) {
	t.Helper()
	for _, f := range migrationFiles(t, suffix) {
		if !strings.HasPrefix(filepath.Base(f), "0011_review_integrity") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(b)); err != nil {
			t.Fatalf("run %s: %v", filepath.Base(f), err)
		}
		return
	}
	t.Fatalf("migration 0011_review_integrity.%s.sql missing", suffix)
}

func rawReview(t *testing.T, pool *pgxpool.Pool, id, listing, order string, verified bool, rating int, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO reviews
		(id, listing_id, user_id, user_name, order_id, rating, comment, verified_purchase, seller_id, created_at)
		VALUES ($1,$2,'u','U',$3,$4,'',$5,'S1',$6)`, id, listing, order, rating, verified, createdAt)
	if err != nil {
		t.Fatal(err)
	}
}

func verifiedOf(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var v bool
	if err := pool.QueryRow(context.Background(), `SELECT verified_purchase FROM reviews WHERE id=$1`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Scenario: Migration demotes later duplicates
func TestMigration0011DemotesDuplicateVerified(t *testing.T) {
	pool := poolBefore0011(t)
	base := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	// Inserted out of order so "earliest created_at" differs from insertion order.
	rawReview(t, pool, "dup-b", "L1", "O1", true, 5, base.Add(time.Minute))
	rawReview(t, pool, "dup-c", "L1", "O1", true, 4, base.Add(2*time.Minute))
	rawReview(t, pool, "dup-a", "L1", "O1", true, 3, base)
	rawReview(t, pool, "other", "L1", "O2", true, 5, base)
	// Equal created_at: the smaller id wins the tiebreak.
	rawReview(t, pool, "tie-2", "L2", "O3", true, 5, base)
	rawReview(t, pool, "tie-1", "L2", "O3", true, 5, base)

	run0011(t, pool, "up")

	for id, want := range map[string]bool{
		"dup-a": true, "dup-b": false, "dup-c": false, "other": true, "tie-1": true, "tie-2": false,
	} {
		if got := verifiedOf(t, pool, id); got != want {
			t.Errorf("review %s verified_purchase = %v, want %v", id, got, want)
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM reviews`).Scan(&n); err != nil || n != 6 {
		t.Fatalf("rows after migration = %d err=%v, want 6 (none deleted)", n, err)
	}
	var idx int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_indexes WHERE indexname='uq_reviews_order_listing_verified'`).Scan(&idx)
	if idx != 1 {
		t.Fatal("partial unique index uq_reviews_order_listing_verified not created")
	}
}

// Scenario: Migration leaves unverified rows alone
func TestMigration0011IgnoresUnverified(t *testing.T) {
	pool := poolBefore0011(t)
	base := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	rawReview(t, pool, "u1", "L1", "O1", false, 5, base)
	rawReview(t, pool, "u2", "L1", "O1", false, 4, base.Add(time.Second))
	rawReview(t, pool, "u3", "L1", "", false, 3, base)
	rawReview(t, pool, "u4", "L1", "", false, 2, base.Add(time.Second))

	run0011(t, pool, "up")

	for _, id := range []string{"u1", "u2", "u3", "u4"} {
		if verifiedOf(t, pool, id) {
			t.Errorf("review %s became verified", id)
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM reviews`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("rows = %d err=%v, want 4", n, err)
	}
}

// Scenario: Down migration removes the new schema
func TestMigration0011Down(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	rawReview(t, pool, "keep-1", "L1", "O1", true, 5, time.Now())
	rawReview(t, pool, "keep-2", "L1", "O2", false, 3, time.Now())

	run0011(t, pool, "down")

	var cols, idx, rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name='reviews' AND column_name IN ('hidden_at','hidden_by','hide_reason')`).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname='uq_reviews_order_listing_verified'`).Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reviews`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if cols != 0 || idx != 0 || rows != 2 {
		t.Fatalf("after down: hidden columns=%d index=%d rows=%d, want 0/0/2", cols, idx, rows)
	}
}

// Scenario: Duplicate review is refused
func TestPGCreateReviewDuplicateOrderListing(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	repo := NewPostgresReviewRepository(pool)
	first := Review{ListingID: "L1", UserID: "b1", OrderID: "O1", Rating: 5, VerifiedPurchase: true, SellerID: "S1"}
	if _, err := repo.CreateReview(ctx, first); err != nil {
		t.Fatal(err)
	}
	_, err := repo.CreateReview(ctx, first)
	if !errors.Is(err, ErrDuplicateReview) {
		t.Fatalf("second create err = %v, want ErrDuplicateReview", err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM reviews WHERE order_id='O1' AND listing_id='L1'`).Scan(&n)
	if n != 1 {
		t.Fatalf("rows for pair = %d, want 1", n)
	}
	// Same order, different listing is allowed.
	other := first
	other.ListingID = "L2"
	if _, err := repo.CreateReview(ctx, other); err != nil {
		t.Fatalf("same order, other listing: %v", err)
	}
	// Unverified rows are outside the partial index.
	legacy := Review{ListingID: "L1", UserID: "b2", OrderID: "O1", Rating: 2}
	if _, err := repo.CreateReview(ctx, legacy); err != nil {
		t.Fatalf("unverified row must not collide: %v", err)
	}
}

// Scenario: Duplicate review is refused (only the review index maps to the sentinel)
func TestPGCreateReviewOtherUniqueViolationIsNotDuplicate(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	repo := NewPostgresReviewRepository(pool)
	if _, err := repo.CreateReview(ctx, Review{ID: "same-id", ListingID: "L1", UserID: "b1", OrderID: "O1", Rating: 5, VerifiedPurchase: true}); err != nil {
		t.Fatal(err)
	}
	_, err := repo.CreateReview(ctx, Review{ID: "same-id", ListingID: "L9", UserID: "b1", OrderID: "O9", Rating: 5, VerifiedPurchase: true})
	if err == nil || errors.Is(err, ErrDuplicateReview) {
		t.Fatalf("primary key violation err = %v, want a non-duplicate-review error", err)
	}
}

// Scenario: Concurrent duplicates yield one row
func TestPGCreateReviewConcurrentDuplicates(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	repo := NewPostgresReviewRepository(pool)
	const n = 10
	var wg sync.WaitGroup
	var ok, dup, other atomic.Int32
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.CreateReview(ctx, Review{ListingID: "L1", UserID: "b1", OrderID: "O1", Rating: 5, VerifiedPurchase: true, SellerID: "S1"})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrDuplicateReview):
				dup.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 1 || dup.Load() != n-1 || other.Load() != 0 {
		t.Fatalf("ok=%d dup=%d other=%d, want 1/%d/0", ok.Load(), dup.Load(), other.Load(), n-1)
	}
	var rows, events int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM reviews`).Scan(&rows)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE type LIKE '%ReviewCreated'`).Scan(&events)
	if rows != 1 || events != 1 {
		t.Fatalf("rows=%d outbox ReviewCreated=%d, want 1/1", rows, events)
	}
}

// Scenario: Stored review emits one event
func TestOutboxReviewCreated(t *testing.T) {
	pool := testPool(t)
	ctx := callerCtx()
	repo := NewPostgresReviewRepository(pool)
	rev, err := repo.CreateReview(ctx, Review{ListingID: "L1", UserID: "b1", OrderID: "O1", Rating: 4, VerifiedPurchase: true, SellerID: "S7"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateReview(ctx, Review{ListingID: "L1", UserID: "b1", OrderID: "O1", Rating: 4, VerifiedPurchase: true, SellerID: "S7"}); !errors.Is(err, ErrDuplicateReview) {
		t.Fatalf("duplicate err = %v", err)
	}
	var payloads [][]byte
	rows, err := pool.Query(ctx, `SELECT payload FROM outbox WHERE type LIKE '%ReviewCreated'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p []byte
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, p)
	}
	if len(payloads) != 1 {
		t.Fatalf("ReviewCreated rows = %d, want exactly 1 (refused duplicate emits nothing)", len(payloads))
	}
	var rc engagementv1.ReviewCreated
	if err := proto.Unmarshal(payloads[0], &rc); err != nil {
		t.Fatal(err)
	}
	if rc.GetReviewId() != rev.ID || rc.GetListingId() != "L1" || rc.GetSellerId() != "S7" || rc.GetRating() != 4 {
		t.Fatalf("event = %v", &rc)
	}
}

type fixtureReview struct {
	id, listing, order string
	rating             int32
	verified, hidden   bool
}

// seedMixed loads verified, unverified and hidden reviews across two listings
// of seller S1, plus one review of another seller.
func seedMixed(t *testing.T, repo reviewRepo) {
	t.Helper()
	ctx := callerCtx()
	for _, f := range []fixtureReview{
		{"v5", "L1", "O1", 5, true, false},
		{"v3", "L1", "O2", 3, true, false},
		{"u1", "L1", "", 1, false, false},
		{"h5", "L1", "O3", 5, true, true},
		{"v4", "L2", "O4", 4, true, false},
		{"h1", "L2", "O5", 1, true, true},
		{"u2", "L2", "", 2, false, false},
	} {
		if _, err := repo.CreateReview(ctx, Review{ID: f.id, ListingID: f.listing, UserID: "b-" + f.id, OrderID: f.order,
			Rating: f.rating, VerifiedPurchase: f.verified, SellerID: "S1"}); err != nil {
			t.Fatalf("create %s: %v", f.id, err)
		}
		if f.hidden {
			if changed, err := repo.HideReview(ctx, f.id, "admin-1", "spam"); err != nil || !changed {
				t.Fatalf("hide %s: changed=%v err=%v", f.id, changed, err)
			}
		}
	}
	if _, err := repo.CreateReview(ctx, Review{ID: "x5", ListingID: "L9", UserID: "bx", OrderID: "OX", Rating: 5, VerifiedPurchase: true, SellerID: "S2"}); err != nil {
		t.Fatal(err)
	}
}

func idsOf(revs []Review) []string {
	var out []string
	for _, r := range revs {
		out = append(out, r.ID)
	}
	sort.Strings(out)
	return out
}

func newReviewRepos(t *testing.T) map[string]reviewRepo {
	return map[string]reviewRepo{
		"postgres": NewPostgresReviewRepository(testPool(t)),
		"memory":   NewInMemoryReviewRepository(),
	}
}

// Scenario: Listing summary ignores unverified reviews
func TestPGRatingSummaryVerifiedOnly(t *testing.T) {
	ctx := callerCtx()
	for name, repo := range newReviewRepos(t) {
		t.Run(name, func(t *testing.T) {
			for _, r := range []Review{
				{ID: "a", ListingID: "L1", UserID: "u1", OrderID: "O1", Rating: 5, VerifiedPurchase: true},
				{ID: "b", ListingID: "L1", UserID: "u2", OrderID: "O2", Rating: 3, VerifiedPurchase: true},
				{ID: "c", ListingID: "L1", UserID: "u3", Rating: 1},
			} {
				if _, err := repo.CreateReview(ctx, r); err != nil {
					t.Fatal(err)
				}
			}
			s, err := repo.GetRatingSummary(ctx, "L1")
			if err != nil {
				t.Fatal(err)
			}
			if s.AverageRating != 4.0 || s.ReviewCount != 2 || s.Breakdown != (RatingBreakdown{Star5: 1, Star3: 1}) {
				t.Fatalf("summary = %+v", s)
			}
		})
	}
}

// Scenario: Shop summary applies the same rule
func TestPGShopSummaryVerifiedNonHidden(t *testing.T) {
	ctx := callerCtx()
	for name, repo := range newReviewRepos(t) {
		t.Run(name, func(t *testing.T) {
			seedMixed(t, repo)
			// S1: verified non-hidden are v5, v3, v4 -> 3 reviews, avg 4.0.
			s, err := repo.GetShopRatingSummary(ctx, "S1")
			if err != nil {
				t.Fatal(err)
			}
			if s.ReviewCount != 3 || s.AverageRating != 4.0 || s.Breakdown != (RatingBreakdown{Star5: 1, Star4: 1, Star3: 1}) {
				t.Fatalf("shop summary = %+v", s)
			}
		})
	}
}

// Scenario: Listing with no verified reviews
func TestRatingSummaryNoVerifiedIsZero(t *testing.T) {
	ctx := callerCtx()
	for name, repo := range newReviewRepos(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := repo.CreateReview(ctx, Review{ID: "only", ListingID: "L1", UserID: "u", Rating: 1, SellerID: "S1"}); err != nil {
				t.Fatal(err)
			}
			s, err := repo.GetRatingSummary(ctx, "L1")
			if err != nil {
				t.Fatal(err)
			}
			if s.ReviewCount != 0 || s.AverageRating != 0 || s.Breakdown != (RatingBreakdown{}) {
				t.Fatalf("summary = %+v", s)
			}
			sh, err := repo.GetShopRatingSummary(ctx, "S1")
			if err != nil || sh.ReviewCount != 0 || sh.AverageRating != 0 {
				t.Fatalf("shop summary = %+v err=%v", sh, err)
			}
		})
	}
}

// Scenario: Hidden review is excluded from the list and the total
func TestPGListReviewsExcludesHidden(t *testing.T) {
	ctx := callerCtx()
	for name, repo := range newReviewRepos(t) {
		t.Run(name, func(t *testing.T) {
			for _, r := range []Review{
				{ID: "A", ListingID: "L1", UserID: "u1", OrderID: "O1", Rating: 5, VerifiedPurchase: true},
				{ID: "B", ListingID: "L1", UserID: "u2", OrderID: "O2", Rating: 5, VerifiedPurchase: true},
				{ID: "C", ListingID: "L1", UserID: "u3", OrderID: "O3", Rating: 3, VerifiedPurchase: true},
			} {
				if _, err := repo.CreateReview(ctx, r); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := repo.HideReview(ctx, "B", "admin-1", "spam"); err != nil {
				t.Fatal(err)
			}
			all, total, err := repo.ListReviews(ctx, "L1", 0, 0, 10)
			if err != nil || total != 2 || !reflect.DeepEqual(idsOf(all), []string{"A", "C"}) {
				t.Fatalf("unfiltered ids=%v total=%d err=%v", idsOf(all), total, err)
			}
			five, total5, err := repo.ListReviews(ctx, "L1", 5, 0, 10)
			if err != nil || total5 != 1 || !reflect.DeepEqual(idsOf(five), []string{"A"}) {
				t.Fatalf("filter-5 ids=%v total=%d err=%v", idsOf(five), total5, err)
			}
		})
	}
}

// Scenario: In-memory repository matches Postgres
func TestReviewRepoParity(t *testing.T) {
	ctx := callerCtx()
	repos := newReviewRepos(t)
	for _, repo := range repos {
		seedMixed(t, repo)
	}
	pg, mem := repos["postgres"], repos["memory"]
	for _, listing := range []string{"L1", "L2", "L9"} {
		for _, filter := range []int32{0, 1, 5} {
			pl, pt, perr := pg.ListReviews(ctx, listing, filter, 0, 50)
			ml, mt, merr := mem.ListReviews(ctx, listing, filter, 0, 50)
			if perr != nil || merr != nil || pt != mt || !reflect.DeepEqual(idsOf(pl), idsOf(ml)) {
				t.Errorf("list %s filter %d: pg=%v/%d/%v mem=%v/%d/%v", listing, filter, idsOf(pl), pt, perr, idsOf(ml), mt, merr)
			}
		}
		ps, _ := pg.GetRatingSummary(ctx, listing)
		ms, _ := mem.GetRatingSummary(ctx, listing)
		if ps != ms {
			t.Errorf("listing summary %s: pg=%+v mem=%+v", listing, ps, ms)
		}
	}
	for _, seller := range []string{"S1", "S2", "S-none"} {
		ps, _ := pg.GetShopRatingSummary(ctx, seller)
		ms, _ := mem.GetShopRatingSummary(ctx, seller)
		if ps != ms {
			t.Errorf("shop summary %s: pg=%+v mem=%+v", seller, ps, ms)
		}
	}
	// Duplicate refusal parity.
	dup := Review{ListingID: "L9", UserID: "bx", OrderID: "OX", Rating: 5, VerifiedPurchase: true, SellerID: "S2"}
	for name, repo := range repos {
		if _, err := repo.CreateReview(ctx, dup); !errors.Is(err, ErrDuplicateReview) {
			t.Errorf("%s duplicate err = %v, want ErrDuplicateReview", name, err)
		}
	}
}

// Scenario: Hide a review end to end (repository level: state and idempotency)
func TestReviewRepoHideReviewIdempotent(t *testing.T) {
	ctx := callerCtx()
	for name, repo := range newReviewRepos(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := repo.CreateReview(ctx, Review{ID: "r1", ListingID: "L1", UserID: "u", OrderID: "O1", Rating: 5, VerifiedPurchase: true}); err != nil {
				t.Fatal(err)
			}
			before, err := repo.GetReview(ctx, "r1")
			if err != nil || before.HiddenAt != nil || before.HiddenBy != "" || before.HideReason != "" {
				t.Fatalf("fresh review = %+v err=%v", before, err)
			}
			changed, err := repo.HideReview(ctx, "r1", "admin-1", "spam link")
			if err != nil || !changed {
				t.Fatalf("first hide changed=%v err=%v", changed, err)
			}
			got, err := repo.GetReview(ctx, "r1")
			if err != nil || got.HiddenAt == nil || got.HiddenBy != "admin-1" || got.HideReason != "spam link" {
				t.Fatalf("hidden review = %+v err=%v", got, err)
			}
			firstAt := *got.HiddenAt
			changed, err = repo.HideReview(ctx, "r1", "admin-2", "other reason")
			if err != nil || changed {
				t.Fatalf("second hide changed=%v err=%v, want false,nil", changed, err)
			}
			again, _ := repo.GetReview(ctx, "r1")
			if again.HiddenBy != "admin-1" || again.HideReason != "spam link" || !again.HiddenAt.Equal(firstAt) {
				t.Fatalf("second hide overwrote state: %+v", again)
			}
			if _, err := repo.HideReview(ctx, "missing", "admin-1", "x"); !errors.Is(err, ErrReviewNotFound) {
				t.Fatalf("hide unknown err = %v, want ErrReviewNotFound", err)
			}
			if _, err := repo.GetReview(ctx, "missing"); !errors.Is(err, ErrReviewNotFound) {
				t.Fatalf("get unknown err = %v, want ErrReviewNotFound", err)
			}
		})
	}
}

// Scenario: Other review RPCs treat a hidden review as missing
func TestReviewRepoMarkHelpfulHiddenIsNotFound(t *testing.T) {
	ctx := callerCtx()
	for name, repo := range newReviewRepos(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := repo.CreateReview(ctx, Review{ID: "r1", ListingID: "L1", UserID: "u", OrderID: "O1", Rating: 5, VerifiedPurchase: true}); err != nil {
				t.Fatal(err)
			}
			if n, err := repo.MarkHelpful(ctx, "r1", "voter-1"); err != nil || n != 1 {
				t.Fatalf("visible MarkHelpful n=%d err=%v", n, err)
			}
			if _, err := repo.HideReview(ctx, "r1", "admin-1", "spam"); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.MarkHelpful(ctx, "r1", "voter-2"); !errors.Is(err, ErrReviewNotFound) {
				t.Fatalf("hidden MarkHelpful err = %v, want ErrReviewNotFound", err)
			}
			got, _ := repo.GetReview(ctx, "r1")
			if got.HelpfulCount != 1 {
				t.Fatalf("helpful_count = %d, hidden vote must not count", got.HelpfulCount)
			}
		})
	}
}
