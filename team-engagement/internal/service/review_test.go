package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
	"github.com/buidangphuc/team-engagement/internal/upstream"
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
	svc := service.NewReviewService(repo, verifier, nil)

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
	svc := service.NewReviewService(repo, verifier, nil)
	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "", 5, "", nil)
	if !errors.Is(err, service.ErrEmptyOrderID) {
		t.Fatalf("want ErrEmptyOrderID, got %v", err)
	}
	if verifier.calls != 0 || total(t, repo, "listing-1") != 0 {
		t.Fatalf("empty order must not call verifier or store (calls=%d)", verifier.calls)
	}
}

func TestCreateReviewIneligibleOrderFailedPrecondition(t *testing.T) {
	repo := storetest.NewFakeReviewRepository()
	svc := service.NewReviewService(repo, eligible("s"), nil) // nothing eligible
	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, service.ErrOrderNotEligible) {
		t.Fatalf("want ErrOrderNotEligible, got %v", err)
	}
	if total(t, repo, "listing-1") != 0 {
		t.Fatal("ineligible review stored")
	}
}

func TestCreateReviewVerifierErrorUnavailable(t *testing.T) {
	repo := storetest.NewFakeReviewRepository()
	verifier := &fakeVerifier{err: errors.New("order unavailable")}
	svc := service.NewReviewService(repo, verifier, nil)

	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, service.ErrOrderUnavailable) {
		t.Fatalf("want ErrOrderUnavailable, got %v", err)
	}
	if errors.Is(err, service.ErrOrderNotEligible) {
		t.Fatal("outage must not read as ineligible")
	}
	if total(t, repo, "listing-1") != 0 {
		t.Fatal("review stored despite verifier error")
	}
}

func TestCreateReviewNilVerifierUnavailable(t *testing.T) {
	repo := storetest.NewFakeReviewRepository()
	svc := service.NewReviewService(repo, nil, nil)
	_, err := svc.CreateReview(context.Background(), "listing-1", "buyer-1", "B", "order-1", 5, "", nil)
	if !errors.Is(err, service.ErrOrderUnavailable) {
		t.Fatalf("want ErrOrderUnavailable, got %v", err)
	}
	if total(t, repo, "listing-1") != 0 {
		t.Fatal("review stored with nil verifier")
	}
}

func TestCreateReviewRatingBounds(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := service.NewReviewService(repo,
		eligible("seller-1", "buyer-1|order-1|listing-1", "buyer-1|order-1|listing-2"), nil)

	for _, bad := range []int32{0, 6} {
		if _, err := svc.CreateReview(ctx, "listing-1", "buyer-1", "B", "order-1", bad, "", nil); !errors.Is(err, service.ErrInvalidRating) {
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
	svc := service.NewReviewService(repo,
		eligible("seller-1", "buyer-1|order-1|listing-1", "buyer-1|order-1|listing-2"), nil)
	for _, l := range []string{"listing-1", "listing-2"} {
		if _, err := svc.CreateReview(ctx, l, "buyer-1", "B", "order-1", 4, "", nil); err != nil {
			t.Fatalf("%s: %v", l, err)
		}
	}
}

func TestCreateReviewDuplicateOrderListingRefused(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := service.NewReviewService(repo, eligible("seller-1", "buyer-1|order-1|listing-1"), nil)
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
		verifier service.OrderVerifier
		order    string
		rating   int32
		want     error
	}{
		{"unavailable", &fakeVerifier{err: errors.New("down")}, "order-1", 5, service.ErrOrderUnavailable},
		{"nil verifier", nil, "order-1", 5, service.ErrOrderUnavailable},
		{"ineligible", eligible("s"), "order-1", 5, service.ErrOrderNotEligible},
		{"invalid rating", eligible("s", "buyer-1|order-1|listing-1"), "order-1", 9, service.ErrInvalidRating},
		{"empty order", eligible("s", "buyer-1||listing-1"), "", 5, service.ErrEmptyOrderID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := storetest.NewFakeReviewRepository()
			svc := service.NewReviewService(repo, c.verifier, nil)
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
	err    error
}

func (a *spyAudit) WriteHideEvent(_ context.Context, actorID, reviewID, listingID, reason string) error {
	a.log.calls = append(a.log.calls, "audit")
	if a.err != nil {
		return a.err
	}
	a.events = append(a.events, hideEvent{actorID, reviewID, listingID, reason})
	return nil
}

// WriteEvent satisfies the extended AuditWriter; review tests never use it.
func (a *spyAudit) WriteEvent(context.Context, upstream.AuditEvent) error { return nil }

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
	svc    *service.ReviewService
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
	return &hideFixture{svc: service.NewReviewService(repo, nil, nil).WithAudit(audit), repo: repo, audit: audit, log: log, review: rev}
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
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("want ErrAuditUnavailable, got %v", err)
	}
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

func TestHideReviewNilAuditClientUnavailable(t *testing.T) {
	f := newHideFixture(t)
	for name, svc := range map[string]*service.ReviewService{
		"never wired": service.NewReviewService(f.repo, nil, nil),
		"nil writer":  service.NewReviewService(f.repo, nil, nil).WithAudit(nil),
	} {
		t.Run(name, func(t *testing.T) {
			err := svc.HideReview(context.Background(), "adm-1", f.review.ID, "spam")
			if !errors.Is(err, service.ErrAuditUnavailable) {
				t.Fatalf("want ErrAuditUnavailable, got %v", err)
			}
			if f.hiddenAt(t) != nil {
				t.Fatal("review hidden without an audit upstream")
			}
		})
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
	for _, s := range []error{service.ErrAuditUnavailable, service.ErrEmptyReason, service.ErrReasonTooLong, service.ErrEmptyReviewID, store.ErrReviewNotFound} {
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
		{"empty", "", service.ErrEmptyReason},
		{"whitespace", "   \t", service.ErrEmptyReason},
		{"500 ascii ok", strings.Repeat("a", 500), nil},
		{"500 runes multibyte ok", strings.Repeat("ạ", 500), nil},
		{"500 runes padded with spaces ok", "  " + strings.Repeat("a", 500) + "  ", nil},
		{"501 ascii", strings.Repeat("a", 501), service.ErrReasonTooLong},
		{"501 runes multibyte", strings.Repeat("ạ", 501), service.ErrReasonTooLong},
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
	if err := f.svc.HideReview(context.Background(), "adm-1", "", "spam"); !errors.Is(err, service.ErrEmptyReviewID) {
		t.Fatalf("want ErrEmptyReviewID, got %v", err)
	}
	if len(f.audit.events) != 0 {
		t.Fatal("no audit event may be written")
	}
}

func TestMarkHelpfulIdempotentPerUser(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeReviewRepository()
	svc := service.NewReviewService(repo, nil, nil)

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
	svc := service.NewReviewService(repo, verifier, nil)

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
	otherSvc := service.NewReviewService(repo, other, nil)
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
	if _, err := svc.GetShopRatingSummary(ctx, ""); !errors.Is(err, service.ErrEmptySeller) {
		t.Fatalf("expected ErrEmptySeller, got %v", err)
	}
}

// pgSchemaPool returns a pool on a private, freshly migrated schema of the
// throwaway TEST_DATABASE_URL database (skipped when unset). A private schema
// keeps parallel package test binaries from dropping each other's tables.
func pgSchemaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("hide_%s_%d", "svc", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	files, _ := filepath.Glob("../../migrations/*.up.sql")
	sort.Strings(files)
	for _, f := range files {
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
	pool := pgSchemaPool(t)
	ctx := interceptor.ContextWithPrincipal(context.Background(),
		&commonv1.Principal{Id: "adm-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: []string{"admin"}})
	repo := store.NewPostgresReviewRepository(pool, mustFactStore(t, pool))
	rev, err := repo.CreateReview(ctx, store.Review{ID: "r1", ListingID: "L1", UserID: "u1", OrderID: "O1", Rating: 2, VerifiedPurchase: true})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]*service.ReviewService{
		"audit write fails": service.NewReviewService(repo, nil, nil).WithAudit(&spyAudit{log: &callLog{}, err: errors.New("audit down")}),
		"no audit upstream": service.NewReviewService(repo, nil, nil),
	}
	for name, svc := range cases {
		t.Run(name, func(t *testing.T) {
			err := svc.HideReview(ctx, "adm-1", rev.ID, "spam")
			if !errors.Is(err, service.ErrAuditUnavailable) {
				t.Fatalf("want ErrAuditUnavailable, got %v", err)
			}
			got, err := repo.GetReview(ctx, rev.ID)
			if err != nil || got.HiddenAt != nil {
				t.Fatalf("review must stay visible: %+v %v", got, err)
			}
			if n := reviewHiddenCount(t, pool); n != 0 {
				t.Fatalf("ReviewHidden outbox rows = %d, want 0", n)
			}
		})
	}
	// Control: with a working audit the same review hides and emits exactly one fact,
	// so the zero counts above are not a vacuous result of a broken outbox query.
	ok := service.NewReviewService(repo, nil, nil).WithAudit(&spyAudit{log: &callLog{}})
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
