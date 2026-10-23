package engagement_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
	"github.com/buidangphuc/team-engagement/migrations"
)

// fakeVerifier is a stand-in for internal/upstream.OrderClient (true external
// boundary). An order is eligible only for the exact (buyer, order, listing).
type fakeVerifier struct {
	orders map[string]string // "buyer|order|listing" -> seller
	err    error
	calls  int
}

func eligible(seller string, keys ...string) *fakeVerifier {
	v := &fakeVerifier{orders: map[string]string{}}
	for _, k := range keys {
		v.orders[k] = seller
	}
	return v
}

func (f *fakeVerifier) VerifyPurchase(_ context.Context, buyerID, listingID, orderID string) (bool, string, error) {
	f.calls++
	if f.err != nil {
		return false, "", f.err
	}
	seller, ok := f.orders[buyerID+"|"+orderID+"|"+listingID]
	return ok, seller, nil
}

func total(t *testing.T, repo store.ReviewRepository, listing string) int64 {
	t.Helper()
	_, n, err := repo.ListReviews(context.Background(), listing, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateReviewVerifiedStoresVerified(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	verifier := eligible("seller-1", "buyer-1|order-1|listing-1")
	svc := engagement.NewReviewService(repo, verifier, quietGate(), nil)

	rev, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "Buyer", "order-1", 5, "Tuyệt vời",
		[]string{"https://cdn/x.jpg", "https://cdn/y.jpg"})
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if !rev.VerifiedPurchase || rev.SellerID != "seller-1" || len(rev.MediaURLs) != 2 || rev.OrderID != "order-1" {
		t.Fatalf("unexpected review %+v", rev)
	}
	got, err := repo.GetReview(ctx, rev.ID)
	if err != nil || !got.VerifiedPurchase {
		t.Fatalf("stored review not verified: %+v %v", got, err)
	}
	if verifier.calls != 1 {
		t.Fatalf("expected verifier called once, got %d", verifier.calls)
	}
}

func TestCreateReviewEmptyOrderIDRejected(t *testing.T) {
	repo := storetest.NewFakeReviewRepository()
	verifier := eligible("s", "buyer-1||listing-1")
	svc := engagement.NewReviewService(repo, verifier, quietGate(), nil)
	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "", 5, "", nil)
	if !errors.Is(err, engagement.ErrEmptyOrderID) {
		t.Fatalf("want ErrEmptyOrderID, got %v", err)
	}
	if verifier.calls != 0 || total(t, repo, "listing-1") != 0 {
		t.Fatalf("empty order must not call verifier or store (calls=%d)", verifier.calls)
	}
}

func TestCreateReviewIneligibleOrderFailedPrecondition(t *testing.T) {
	repo := storetest.NewFakeReviewRepository()
	svc := engagement.NewReviewService(repo, eligible("s"), quietGate(), nil) // nothing eligible
	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, engagement.ErrOrderNotEligible) {
		t.Fatalf("want ErrOrderNotEligible, got %v", err)
	}
	if total(t, repo, "listing-1") != 0 {
		t.Fatal("ineligible review stored")
	}
}

func TestCreateReviewVerifierErrorUnavailable(t *testing.T) {
	repo := storetest.NewFakeReviewRepository()
	verifier := &fakeVerifier{err: errors.New("order unavailable")}
	svc := engagement.NewReviewService(repo, verifier, quietGate(), nil)

	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, engagement.ErrOrderUnavailable) {
		t.Fatalf("want ErrOrderUnavailable, got %v", err)
	}
	if errors.Is(err, engagement.ErrOrderNotEligible) {
		t.Fatal("outage must not read as ineligible")
	}
	if total(t, repo, "listing-1") != 0 {
		t.Fatal("review stored despite verifier error")
	}

	pool := migratedPool(t)
	pgRepo := store.NewPostgresReviewRepository(pool, mustFactStore(t, pool))
	pgSvc := engagement.NewReviewService(pgRepo, verifier, quietGate(), nil)
	_, err = pgSvc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, engagement.ErrOrderUnavailable) {
		t.Fatalf("postgres: want ErrOrderUnavailable, got %v", err)
	}
	var rows, facts int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM reviews`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM engagement_outbox_events WHERE event_type = 'platform.engagement.v1.ReviewCreated'`).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || facts != 0 {
		t.Fatalf("reviews = %d, ReviewCreated rows = %d, want 0 and 0", rows, facts)
	}
}

func TestCreateReviewRatingBounds(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := engagement.NewReviewService(repo,
		eligible("seller-1", "buyer-1|order-1|listing-1", "buyer-1|order-1|listing-2"), quietGate(), nil)

	for _, bad := range []int32{0, 6} {
		if _, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "B", "order-1", bad, "", nil); !errors.Is(err, engagement.ErrInvalidRating) {
			t.Fatalf("rating %d: want ErrInvalidRating, got %v", bad, err)
		}
	}
	if total(t, repo, "listing-1") != 0 {
		t.Fatal("invalid rating stored a row")
	}
	if _, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "B", "order-1", 1, "", nil); err != nil {
		t.Fatalf("rating 1: %v", err)
	}
	if _, err := svc.CreateReview(ctx, "listing-2", "buyer-1", "B", "order-1", 5, "", nil); err != nil {
		t.Fatalf("rating 5: %v", err)
	}
}

func TestCreateReviewSameOrderTwoListings(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := engagement.NewReviewService(repo,
		eligible("seller-1", "buyer-1|order-1|listing-1", "buyer-1|order-1|listing-2"), quietGate(), nil)
	for _, l := range []string{"listing-1", "listing-2"} {
		if _, err := svc.CreateReview(ctx, l, "buyer-1", "B", "order-1", 4, "", nil); err != nil {
			t.Fatalf("%s: %v", l, err)
		}
	}
}

func TestCreateReviewDuplicateOrderListingRefused(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := engagement.NewReviewService(repo, eligible("seller-1", "buyer-1|order-1|listing-1"), quietGate(), nil)
	if _, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "B", "order-1", 4, "", nil); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, store.ErrDuplicateReview) {
		t.Fatalf("want ErrDuplicateReview, got %v", err)
	}
	if total(t, repo, "listing-1") != 1 {
		t.Fatal("duplicate stored a second row")
	}
}

// Every refusal leaves the store (and therefore the transactional outbox) untouched.
func TestCreateReviewRefusedWritesNoOutbox(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		verifier engagement.OrderVerifier
		order    string
		rating   int32
		want     error
	}{
		{"unavailable", &fakeVerifier{err: errors.New("down")}, "order-1", 5, engagement.ErrOrderUnavailable},
		{"ineligible", eligible("s"), "order-1", 5, engagement.ErrOrderNotEligible},
		{"invalid rating", eligible("s", "buyer-1|order-1|listing-1"), "order-1", 9, engagement.ErrInvalidRating},
		{"empty order", eligible("s", "buyer-1||listing-1"), "", 5, engagement.ErrEmptyOrderID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := storetest.NewFakeReviewRepository()
			svc := engagement.NewReviewService(repo, c.verifier, quietGate(), nil)
			if _, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "B", c.order, c.rating, "", nil); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
			if total(t, repo, "listing-1") != 0 {
				t.Fatal("refused review was stored")
			}
		})
	}
}

// ── HideReview ──

type hideEvent struct{ actor, review, listing, reason string }

type callLog struct{ calls []string }

type spyAudit struct {
	log    *callLog
	events []hideEvent
	raw    []auditx.Event
	err    error
}

func (a *spyAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	a.log.calls = append(a.log.calls, "audit")
	if a.err != nil {
		return a.err
	}
	a.raw = append(a.raw, ev)
	a.events = append(a.events, hideEvent{ev.ActorID, ev.TargetID, ev.Metadata["listing_id"], ev.Metadata["reason"]})
	return nil
}

// spyRepo records the hide call order and can fail the hide update.
type spyRepo struct {
	*storetest.FakeReviewRepository
	log     *callLog
	hideErr error
}

func (r *spyRepo) HideReview(ctx context.Context, id, adminID, reason string) (bool, error) {
	r.log.calls = append(r.log.calls, "hide")
	if r.hideErr != nil {
		return false, r.hideErr
	}
	return r.FakeReviewRepository.HideReview(ctx, id, adminID, reason)
}

type hideFixture struct {
	svc    *engagement.ReviewService
	repo   *spyRepo
	audit  *spyAudit
	log    *callLog
	review store.Review
}

func newHideFixture(t *testing.T) *hideFixture {
	t.Helper()
	log := &callLog{}
	mem := storetest.NewFakeReviewRepository()
	rev, err := mem.CreateReview(context.Background(), store.Review{
		ListingID: "L-1", UserID: "buyer-1", OrderID: "order-1", Rating: 1,
		VerifiedPurchase: true, SellerID: "seller-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &spyRepo{FakeReviewRepository: mem, log: log}
	audit := &spyAudit{log: log}
	return &hideFixture{svc: engagement.NewReviewService(repo, &fakeVerifier{}, auditx.NewGate(audit, nil), nil), repo: repo, audit: audit, log: log, review: rev}
}

func (f *hideFixture) hiddenAt(t *testing.T) *time.Time {
	t.Helper()
	got, err := f.repo.GetReview(context.Background(), f.review.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got.HiddenAt
}

func TestHideReviewAuditBeforeHide(t *testing.T) {
	f := newHideFixture(t)
	if err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "spam link"); err != nil {
		t.Fatalf("HideReview: %v", err)
	}
	if got := f.log.calls; len(got) != 2 || got[0] != "audit" || got[1] != "hide" {
		t.Fatalf("call order = %v, want [audit hide]", got)
	}
	want := hideEvent{"adm-1", f.review.ID, "L-1", "spam link"}
	if len(f.audit.events) != 1 || f.audit.events[0] != want {
		t.Fatalf("audit events = %+v, want %+v", f.audit.events, want)
	}
	got, _ := f.repo.GetReview(context.Background(), f.review.ID)
	if got.HiddenAt == nil || got.HiddenBy != "adm-1" || got.HideReason != "spam link" {
		t.Fatalf("review not hidden correctly: %+v", got)
	}
}

func TestHideReviewAuditFailureFailsClosed(t *testing.T) {
	f := newHideFixture(t)
	f.audit.err = errors.New("audit down")
	err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "spam")
	requireAuditUnavailable(t, err)
	if f.hiddenAt(t) != nil {
		t.Fatal("review hidden although audit failed")
	}
	if n := total(t, f.repo, "L-1"); n != 1 {
		t.Fatalf("review no longer listed: total=%d", n)
	}
	for _, c := range f.log.calls {
		if c == "hide" {
			t.Fatal("hide must not run after a failed audit")
		}
	}
}

func TestHideReviewUpdateFailureInternalThenRetry(t *testing.T) {
	f := newHideFixture(t)
	boom := errors.New("db down")
	f.repo.hideErr = boom
	err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "spam")
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("want the raw repository error (mapped to Internal by the handler), got %v", err)
	}
	if status.Code(err) == codes.Unavailable {
		t.Fatalf("update failure must not look like an audit outage: %v", err)
	}
	for _, s := range []error{engagement.ErrEmptyReason, engagement.ErrReasonTooLong, engagement.ErrEmptyReviewID, store.ErrReviewNotFound} {
		if errors.Is(err, s) {
			t.Fatalf("update failure must not be classified as %v", s)
		}
	}
	if f.hiddenAt(t) != nil {
		t.Fatal("review hidden although update failed")
	}

	f.repo.hideErr = nil
	if err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "spam"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if f.hiddenAt(t) == nil {
		t.Fatal("retry did not hide the review")
	}
	if len(f.audit.events) != 2 {
		t.Fatalf("want 2 audit events (failed attempt + retry), got %d", len(f.audit.events))
	}
}

func TestHideReviewIdempotentNoSecondAudit(t *testing.T) {
	f := newHideFixture(t)
	if err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "first"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.HideReview(context.Background(), "adm-2", f.review.ID, "second"); err != nil {
		t.Fatalf("re-hide must be OK, got %v", err)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(f.audit.events))
	}
	got, _ := f.repo.GetReview(context.Background(), f.review.ID)
	if got.HiddenBy != "adm-1" || got.HideReason != "first" {
		t.Fatalf("first hide state must be kept, got %+v", got)
	}
}

func TestHideReviewReasonTrimmedBeforeStoring(t *testing.T) {
	f := newHideFixture(t)
	if err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "  spam \n"); err != nil {
		t.Fatal(err)
	}
	got, _ := f.repo.GetReview(context.Background(), f.review.ID)
	if got.HideReason != "spam" || f.audit.events[0].reason != "spam" {
		t.Fatalf("reason not trimmed: stored %q audit %q", got.HideReason, f.audit.events[0].reason)
	}
}

func TestHideReviewReasonValidation(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		want   error
	}{
		{"empty", "", engagement.ErrEmptyReason},
		{"whitespace", "   \t", engagement.ErrEmptyReason},
		{"500 ascii ok", strings.Repeat("a", 500), nil},
		{"500 runes multibyte ok", strings.Repeat("ạ", 500), nil},
		{"500 runes padded with spaces ok", "  " + strings.Repeat("a", 500) + "  ", nil},
		{"501 ascii", strings.Repeat("a", 501), engagement.ErrReasonTooLong},
		{"501 runes multibyte", strings.Repeat("ạ", 501), engagement.ErrReasonTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newHideFixture(t)
			err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, c.reason)
			if !errors.Is(err, c.want) || (c.want == nil && err != nil) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
			if c.want != nil && (len(f.audit.events) != 0 || f.hiddenAt(t) != nil) {
				t.Fatal("rejected reason must not audit or hide")
			}
		})
	}
}

func TestHideReviewUnknownAndEmptyID(t *testing.T) {
	f := newHideFixture(t)
	if err := f.svc.HideReview(context.Background(), "adm-1", "nope", "spam"); !errors.Is(err, store.ErrReviewNotFound) {
		t.Fatalf("want ErrReviewNotFound, got %v", err)
	}
	if err := f.svc.HideReview(context.Background(), "adm-1", "", "spam"); !errors.Is(err, engagement.ErrEmptyReviewID) {
		t.Fatalf("want ErrEmptyReviewID, got %v", err)
	}
	if len(f.audit.events) != 0 {
		t.Fatal("no audit event may be written")
	}
}

func TestMarkHelpfulIdempotentPerUser(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := engagement.NewReviewService(repo, &fakeVerifier{}, quietGate(), nil)

	rev, err := repo.CreateReview(ctx, store.Review{ListingID: "listing-1", UserID: "buyer-1", Rating: 5})
	if err != nil {
		t.Fatalf("seed review: %v", err)
	}

	// Same user voting twice counts once.
	c1, err := svc.MarkHelpful(ctx, rev.ID, "user-a")
	if err != nil {
		t.Fatalf("MarkHelpful a1: %v", err)
	}
	if c1 != 1 {
		t.Fatalf("expected count 1, got %d", c1)
	}
	c2, err := svc.MarkHelpful(ctx, rev.ID, "user-a")
	if err != nil {
		t.Fatalf("MarkHelpful a2: %v", err)
	}
	if c2 != 1 {
		t.Fatalf("expected idempotent count 1, got %d", c2)
	}

	// A different user increments.
	c3, err := svc.MarkHelpful(ctx, rev.ID, "user-b")
	if err != nil {
		t.Fatalf("MarkHelpful b: %v", err)
	}
	if c3 != 2 {
		t.Fatalf("expected count 2, got %d", c3)
	}

	// Unknown review → not found.
	if _, err := svc.MarkHelpful(ctx, "no-such-review", "user-a"); !errors.Is(err, store.ErrReviewNotFound) {
		t.Fatalf("expected ErrReviewNotFound, got %v", err)
	}
}

func TestGetShopRatingSummaryAcrossListings(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	verifier := eligible("seller-1",
		"b1|o1|listing-1", "b2|o2|listing-1", "b3|o3|listing-2")
	svc := engagement.NewReviewService(repo, verifier, quietGate(), nil)

	// Two listings, same seller, ratings 5,3 (listing-1) and 4 (listing-2).
	if _, err := svc.CreateReview(ctx, "listing-1", "b1", "B1", "o1", 5, "", nil); err != nil {
		t.Fatalf("review1: %v", err)
	}
	if _, err := svc.CreateReview(ctx, "listing-1", "b2", "B2", "o2", 3, "", nil); err != nil {
		t.Fatalf("review2: %v", err)
	}
	if _, err := svc.CreateReview(ctx, "listing-2", "b3", "B3", "o3", 4, "", nil); err != nil {
		t.Fatalf("review3: %v", err)
	}
	// A review for a different seller must not leak in.
	other := eligible("seller-2", "b9|o9|listing-9")
	otherSvc := engagement.NewReviewService(repo, other, quietGate(), nil)
	if _, err := otherSvc.CreateReview(ctx, "listing-9", "b9", "B9", "o9", 1, "", nil); err != nil {
		t.Fatalf("review-other: %v", err)
	}

	sum, err := svc.GetShopRatingSummary(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetShopRatingSummary: %v", err)
	}
	if sum.ReviewCount != 3 {
		t.Fatalf("expected 3 reviews for seller-1, got %d", sum.ReviewCount)
	}
	if sum.AverageRating != 4.0 { // (5+3+4)/3
		t.Fatalf("expected avg 4.0, got %v", sum.AverageRating)
	}
	if sum.Breakdown.Star5 != 1 || sum.Breakdown.Star4 != 1 || sum.Breakdown.Star3 != 1 {
		t.Fatalf("unexpected breakdown: %+v", sum.Breakdown)
	}

	// Validation
	if _, err := svc.GetShopRatingSummary(ctx, ""); !errors.Is(err, engagement.ErrEmptySeller) {
		t.Fatalf("expected ErrEmptySeller, got %v", err)
	}
}

func reviewHiddenCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM engagement_outbox_events WHERE event_type = 'platform.engagement.v1.ReviewHidden'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Scenario: Review hidden event: audit failure emits nothing
func TestHideReviewAuditFailureNoFact(t *testing.T) {
	pool := migratedPool(t)
	ctx := auth.WithPrincipal(context.Background(),
		auth.Principal{ID: "adm-1", Type: auth.TypeUser, Scopes: []string{"engagement:admin"}})
	repo := store.NewPostgresReviewRepository(pool, mustFactStore(t, pool))
	rev, err := repo.CreateReview(ctx, store.Review{ID: "r1", ListingID: "L1", UserID: "u1", OrderID: "O1", Rating: 2, VerifiedPurchase: true})
	if err != nil {
		t.Fatal(err)
	}
	failing := engagement.NewReviewService(repo, &fakeVerifier{},
		auditx.NewGate(&spyAudit{log: &callLog{}, err: errors.New("audit down")}, nil), nil)
	err = failing.HideReview(ctx, "adm-1", rev.ID, "spam")
	requireAuditUnavailable(t, err)
	got, err := repo.GetReview(ctx, rev.ID)
	if err != nil || got.HiddenAt != nil {
		t.Fatalf("review must stay visible: %+v %v", got, err)
	}
	if n := reviewHiddenCount(t, pool); n != 0 {
		t.Fatalf("ReviewHidden outbox rows = %d, want 0", n)
	}
	// Control: with a working audit the same review hides and emits exactly one fact,
	// so the zero count above is not a vacuous result of a broken outbox query.
	ok := engagement.NewReviewService(repo, &fakeVerifier{}, auditx.NewGate(&spyAudit{log: &callLog{}}, nil), nil)
	if err := ok.HideReview(ctx, "adm-1", rev.ID, "spam"); err != nil {
		t.Fatal(err)
	}
	if n := reviewHiddenCount(t, pool); n != 1 {
		t.Fatalf("control: ReviewHidden outbox rows = %d, want 1", n)
	}
}

func mustFactStore(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	st, err := outbox.NewStore(pool, "engagement_outbox_events")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func quietGate() *auditx.Gate {
	return auditx.NewGate(&spyAudit{log: &callLog{}}, nil)
}
