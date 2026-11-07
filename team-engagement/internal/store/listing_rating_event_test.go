package store_test

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/store"
)

const listingRatingChangedType = "platform.engagement.v1.ListingRatingChanged"

type ratingEvent struct {
	row factRow
	msg *engagementv1.ListingRatingChanged
}

func ratingEvents(t *testing.T, pool *pgxpool.Pool, listing string) []ratingEvent {
	t.Helper()
	var out []ratingEvent
	for _, f := range factsOfType(t, pool, listingRatingChangedType) {
		if f.aggregateID != listing {
			continue
		}
		m := &engagementv1.ListingRatingChanged{}
		if err := proto.Unmarshal(f.env.GetPayload(), m); err != nil {
			t.Fatalf("payload is not a ListingRatingChanged: %v", err)
		}
		out = append(out, ratingEvent{row: f, msg: m})
	}
	return out
}

func lastRatingEvent(t *testing.T, pool *pgxpool.Pool, listing string) *engagementv1.ListingRatingChanged {
	t.Helper()
	evs := ratingEvents(t, pool, listing)
	if len(evs) == 0 {
		t.Fatalf("no ListingRatingChanged row for %s", listing)
	}
	return evs[len(evs)-1].msg
}

func assertRating(t *testing.T, m *engagementv1.ListingRatingChanged, listing string, count int64, avg float64) {
	t.Helper()
	if m.GetListingId() != listing || m.GetReviewCount() != count || m.GetAverageRating() != avg {
		t.Fatalf("event = listing %q count %d avg %v, want %q %d %v", m.GetListingId(), m.GetReviewCount(), m.GetAverageRating(), listing, count, avg)
	}
}

func assertMatchesSummary(t *testing.T, repo *store.PostgresReviewRepository, m *engagementv1.ListingRatingChanged) {
	t.Helper()
	sum, err := repo.GetRatingSummary(callerCtx(), m.GetListingId())
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReviewCount != m.GetReviewCount() || sum.AverageRating != m.GetAverageRating() {
		t.Fatalf("GetRatingSummary = %d/%v, event = %d/%v", sum.ReviewCount, sum.AverageRating, m.GetReviewCount(), m.GetAverageRating())
	}
}

func newRatingRepo(t *testing.T) (*pgxpool.Pool, *store.PostgresReviewRepository) {
	t.Helper()
	pool := testPool(t)
	return pool, store.NewPostgresReviewRepository(pool, testOutbox(t, pool))
}

func addReview(t *testing.T, repo *store.PostgresReviewRepository, id, listing string, rating int32) {
	t.Helper()
	if _, err := repo.CreateReview(callerCtx(), store.Review{
		ID: id, ListingID: listing, UserID: "u-" + id, OrderID: "O-" + id, Rating: rating, VerifiedPurchase: true,
	}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

func hideReview(t *testing.T, repo *store.PostgresReviewRepository, id string) {
	t.Helper()
	if changed, err := repo.HideReview(callerCtx(), id, "admin-1", "reason"); err != nil || !changed {
		t.Fatalf("hide %s: changed=%v err=%v", id, changed, err)
	}
}

func raceAll(fns ...func()) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fn()
		}()
	}
	close(start)
	wg.Wait()
}

func TestCreateReviewWritesListingRatingChanged(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r1", "L1", 5)
	addReview(t, repo, "r2", "L1", 2)

	evs := ratingEvents(t, pool, "L1")
	if len(evs) != 2 {
		t.Fatalf("ListingRatingChanged rows = %d, want 2 (one per create)", len(evs))
	}
	assertRating(t, evs[0].msg, "L1", 1, 5)
	assertRating(t, evs[1].msg, "L1", 2, 3.5)
	assertMatchesSummary(t, repo, evs[1].msg)
	if evs[1].row.aggregateType != "Listing" {
		t.Fatalf("aggregate type = %q, want Listing", evs[1].row.aggregateType)
	}
	if evs[1].row.env.GetType() != listingRatingChangedType {
		t.Fatalf("envelope type = %q", evs[1].row.env.GetType())
	}
	if evs[1].msg.GetChangedAt() == nil {
		t.Fatal("changed_at is not set")
	}
}

func TestCreateReviewRatingEventFollowsReviewCreatedForSameListing(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r1", "L1", 4)
	addReview(t, repo, "x1", "L2", 1)

	var types []string
	for _, f := range factRows(t, pool) {
		if f.aggregateID == "L1" {
			types = append(types, f.eventType)
		}
	}
	want := []string{"platform.engagement.v1.ReviewCreated", listingRatingChangedType}
	if fmt.Sprint(types) != fmt.Sprint(want) {
		t.Fatalf("L1 facts = %v, want %v", types, want)
	}
	assertRating(t, lastRatingEvent(t, pool, "L2"), "L2", 1, 1)
}

func TestCreateUnverifiedReviewPublishesUnchangedSummary(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r1", "L1", 5)
	if _, err := repo.CreateReview(callerCtx(), store.Review{ID: "u1", ListingID: "L1", UserID: "u-u1", OrderID: "O-u1", Rating: 1}); err != nil {
		t.Fatal(err)
	}
	evs := ratingEvents(t, pool, "L1")
	if len(evs) != 2 {
		t.Fatalf("ListingRatingChanged rows = %d, want 2", len(evs))
	}
	assertRating(t, evs[1].msg, "L1", 1, 5)
}

func TestHideReviewWritesListingRatingChanged(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r5", "L1", 5)
	addReview(t, repo, "r1", "L1", 1)
	before := len(ratingEvents(t, pool, "L1"))

	hideReview(t, repo, "r1")

	evs := ratingEvents(t, pool, "L1")
	if len(evs) != before+1 {
		t.Fatalf("ListingRatingChanged rows = %d, want %d", len(evs), before+1)
	}
	last := evs[len(evs)-1]
	assertRating(t, last.msg, "L1", 1, 5)
	assertMatchesSummary(t, repo, last.msg)

	hiddenSeq, ratingSeq := -1, -1
	for i, f := range factRows(t, pool) {
		if f.eventType == reviewHiddenType && f.aggregateID == "L1" {
			hiddenSeq = i
		}
		if f.eventType == listingRatingChangedType && f.aggregateID == "L1" {
			ratingSeq = i
		}
	}
	if hiddenSeq < 0 || ratingSeq < hiddenSeq {
		t.Fatalf("rating event position %d must come after ReviewHidden position %d", ratingSeq, hiddenSeq)
	}
}

func TestHideUnverifiedReviewStillWritesEvent(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r5", "L1", 5)
	if _, err := repo.CreateReview(callerCtx(), store.Review{ID: "u1", ListingID: "L1", UserID: "u-u1", OrderID: "O-u1", Rating: 1}); err != nil {
		t.Fatal(err)
	}
	before := len(ratingEvents(t, pool, "L1"))

	hideReview(t, repo, "u1")

	evs := ratingEvents(t, pool, "L1")
	if len(evs) != before+1 {
		t.Fatalf("ListingRatingChanged rows = %d, want %d", len(evs), before+1)
	}
	assertRating(t, evs[len(evs)-1].msg, "L1", 1, 5)
}

func TestHideLastReviewPublishesZero(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r1", "L1", 4)

	hideReview(t, repo, "r1")

	evs := ratingEvents(t, pool, "L1")
	if len(evs) != 2 {
		t.Fatalf("ListingRatingChanged rows = %d, want 2 (create, hide)", len(evs))
	}
	assertRating(t, evs[1].msg, "L1", 0, 0)
	assertMatchesSummary(t, repo, evs[1].msg)
}

func TestNoOpWritesPublishNoRatingEvent(t *testing.T) {
	pool, repo := newRatingRepo(t)
	addReview(t, repo, "r1", "L1", 5)
	if n := len(ratingEvents(t, pool, "L1")); n != 1 {
		t.Fatalf("control: rating events after first create = %d, want 1", n)
	}

	_, err := repo.CreateReview(callerCtx(), store.Review{ID: "dup", ListingID: "L1", UserID: "u-r1", OrderID: "O-r1", Rating: 1, VerifiedPurchase: true})
	if err == nil {
		t.Fatal("duplicate review was accepted")
	}
	if n := len(ratingEvents(t, pool, "L1")); n != 1 {
		t.Fatalf("rating events after duplicate create = %d, want 1", n)
	}

	hideReview(t, repo, "r1")
	if n := len(ratingEvents(t, pool, "L1")); n != 2 {
		t.Fatalf("control: rating events after first hide = %d, want 2", n)
	}
	if changed, err := repo.HideReview(callerCtx(), "r1", "admin-2", "again"); err != nil || changed {
		t.Fatalf("re-hide changed=%v err=%v", changed, err)
	}
	if n := len(ratingEvents(t, pool, "L1")); n != 2 {
		t.Fatalf("rating events after re-hide = %d, want 2", n)
	}

	if _, err := repo.HideReview(callerCtx(), "missing", "admin-1", "x"); err == nil {
		t.Fatal("hiding an unknown review succeeded")
	}
	if n := len(factsOfType(t, pool, listingRatingChangedType)); n != 2 {
		t.Fatalf("rating events after unknown hide = %d, want 2", n)
	}
}

func TestConcurrentReviewsConvergeOnFinalRating(t *testing.T) {
	pool, repo := newRatingRepo(t)
	for i := 0; i < 8; i++ {
		listing := fmt.Sprintf("L%d", i)
		raceAll(
			func() { addReview(t, repo, listing+"-a", listing, 5) },
			func() { addReview(t, repo, listing+"-b", listing, 1) },
		)
		last := lastRatingEvent(t, pool, listing)
		assertRating(t, last, listing, 2, 3.0)
		assertMatchesSummary(t, repo, last)
	}
}

func TestConcurrentHidesConvergeOnFinalRating(t *testing.T) {
	pool, repo := newRatingRepo(t)
	for i := 0; i < 8; i++ {
		listing := fmt.Sprintf("L%d", i)
		addReview(t, repo, listing+"-5", listing, 5)
		addReview(t, repo, listing+"-3", listing, 3)
		addReview(t, repo, listing+"-1", listing, 1)
		raceAll(
			func() { hideReview(t, repo, listing+"-3") },
			func() { hideReview(t, repo, listing+"-1") },
		)
		last := lastRatingEvent(t, pool, listing)
		assertRating(t, last, listing, 1, 5)
		assertMatchesSummary(t, repo, last)
	}
}

func TestConcurrentCreateAndHideConvergeOnFinalRating(t *testing.T) {
	pool, repo := newRatingRepo(t)
	for i := 0; i < 8; i++ {
		listing := fmt.Sprintf("L%d", i)
		addReview(t, repo, listing+"-5", listing, 5)
		addReview(t, repo, listing+"-1", listing, 1)
		raceAll(
			func() { addReview(t, repo, listing+"-3", listing, 3) },
			func() { hideReview(t, repo, listing+"-1") },
		)
		last := lastRatingEvent(t, pool, listing)
		assertRating(t, last, listing, 2, 4.0)
		assertMatchesSummary(t, repo, last)
	}
}

func TestListingRatingChangedEnvelopeCarriesNoPersonalData(t *testing.T) {
	pool, repo := newRatingRepo(t)
	if _, err := repo.CreateReview(callerCtx(), store.Review{
		ID: "review-id-7f3a", ListingID: "L1", UserID: "reviewer-9c2b", UserName: "Reviewer Name", OrderID: "O-1",
		Rating: 4, Comment: "rất tốt", VerifiedPurchase: true, SellerID: "seller-4d1e",
	}); err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.HideReview(adminCtx("admin-id-5e8f"), "review-id-7f3a", "admin-id-5e8f", "lý do vi phạm"); err != nil || !changed {
		t.Fatalf("hide changed=%v err=%v", changed, err)
	}

	evs := ratingEvents(t, pool, "L1")
	if len(evs) != 2 {
		t.Fatalf("ListingRatingChanged rows = %d, want 2", len(evs))
	}
	for i, ev := range evs {
		if ev.row.env.GetPrincipal() != nil {
			t.Fatalf("event %d envelope carries principal %v", i, ev.row.env.GetPrincipal())
		}
		raw, err := proto.Marshal(ev.row.env)
		if err != nil {
			t.Fatal(err)
		}
		for _, leak := range []string{"reviewer-9c2b", "Reviewer Name", "admin-id-5e8f", "review-id-7f3a", "seller-4d1e", "rất tốt", "lý do vi phạm", "buyer-1"} {
			if bytes.Contains(raw, []byte(leak)) {
				t.Fatalf("event %d envelope contains %q", i, leak)
			}
		}
	}
}
