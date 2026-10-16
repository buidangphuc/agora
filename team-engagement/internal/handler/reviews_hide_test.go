package handler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

// ── shared fakes for the review handler tests (also used by engagement_test.go) ──

// reviewVerifier is a fake team-order: eligible only for "buyer|order|listing".
type reviewVerifier struct {
	orders map[string]string // key -> seller
	err    error
	calls  int
}

func (v *reviewVerifier) VerifyPurchase(_ context.Context, buyerID, listingID, orderID string) (bool, string, error) {
	v.calls++
	if v.err != nil {
		return false, "", v.err
	}
	seller, ok := v.orders[buyerID+"|"+orderID+"|"+listingID]
	return ok, seller, nil
}

type auditEvent struct{ actor, review, listing, reason string }

// auditSpy is a fake team-audit client: records events, can be forced to fail.
type auditSpy struct {
	events []auditEvent
	err    error
}

func (a *auditSpy) WriteHideEvent(_ context.Context, actorID, reviewID, listingID, reason string) error {
	if a.err != nil {
		return a.err
	}
	a.events = append(a.events, auditEvent{actorID, reviewID, listingID, reason})
	return nil
}

// failingHideRepo fails only the hide update.
type failingHideRepo struct {
	*repository.InMemoryReviewRepository
	err error
}

func (r *failingHideRepo) HideReview(ctx context.Context, id, adminID, reason string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return r.InMemoryReviewRepository.HideReview(ctx, id, adminID, reason)
}

type reviewFixture struct {
	h        *handler.EngagementHandler
	repo     *failingHideRepo
	verifier *reviewVerifier
	audit    *auditSpy
	logs     *bytes.Buffer
}

// newReviewFixture builds a handler whose logs (default slog and the service
// logger) go to fx.logs, so tests can assert what is never logged.
func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	repo := &failingHideRepo{InMemoryReviewRepository: repository.NewInMemoryReviewRepository()}
	verifier := &reviewVerifier{orders: map[string]string{}}
	audit := &auditSpy{}
	svc := service.NewReviewService(repo, verifier, logger).WithAudit(audit)
	h := handler.NewEngagementHandler(repository.NewInMemoryRepository(), svc, nil, nil, nil)
	return &reviewFixture{h: h, repo: repo, verifier: verifier, audit: audit, logs: logs}
}

// seed stores a review directly in the repository.
func (f *reviewFixture) seed(t *testing.T, listing, seller string, rating int32, verified bool) repository.Review {
	t.Helper()
	rev, err := f.repo.InMemoryReviewRepository.CreateReview(context.Background(), repository.Review{
		ListingID: listing, SellerID: seller, UserID: "u-seed",
		OrderID: fmt.Sprintf("o-%s-%d-%v-%d", listing, rating, verified, f.count(listing)), Rating: rating,
		VerifiedPurchase: verified,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func (f *reviewFixture) count(listing string) int64 {
	_, n, _ := f.repo.InMemoryReviewRepository.ListReviews(context.Background(), listing, 0, 0, 1000)
	return n
}

func (f *reviewFixture) listIDs(t *testing.T, ctx context.Context, listing string) []string {
	t.Helper()
	res, err := f.h.ListReviews(ctx, &engagementv1.ListReviewsRequest{ListingId: listing, Page: &commonv1.PageRequest{PageSize: 100}})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	var ids []string
	for _, r := range res.GetReviews() {
		ids = append(ids, r.GetId())
	}
	return ids
}

func (f *reviewFixture) hide(ctx context.Context, id, reason string) (*engagementv1.HideReviewResponse, error) {
	return f.h.HideReview(ctx, &engagementv1.HideReviewRequest{ReviewId: id, Reason: reason})
}

func (f *reviewFixture) stillVisible(t *testing.T, id string) {
	t.Helper()
	got, err := f.repo.GetReview(context.Background(), id)
	if err != nil || got.HiddenAt != nil {
		t.Fatalf("review must stay visible: %+v %v", got, err)
	}
}

func adminCtxAs(id string) context.Context {
	return principalCtx(id, commonv1.PrincipalType_PRINCIPAL_TYPE_USER, append(append([]string{}, userScopes...), "admin")...)
}

// ── HideReview ──

func TestHideReviewHidesFromList(t *testing.T) {
	f := newReviewFixture(t)
	spam := f.seed(t, "L-1", "s-1", 1, true)
	keep := f.seed(t, "L-1", "s-1", 5, true)

	if _, err := f.hide(adminCtx(), spam.ID, "spam link"); err != nil {
		t.Fatalf("HideReview: %v", err)
	}
	for name, ctx := range map[string]context.Context{"anonymous": anonCtx(), "buyer": userCtx("buyer-1"), "no principal": context.Background()} {
		ids := f.listIDs(t, ctx, "L-1")
		if len(ids) != 1 || ids[0] != keep.ID {
			t.Fatalf("%s sees %v, want only %s", name, ids, keep.ID)
		}
	}
}

func TestHideReviewInvisibleToAdminList(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	if _, err := f.hide(adminCtx(), r.ID, "spam"); err != nil {
		t.Fatal(err)
	}
	if ids := f.listIDs(t, adminCtx(), "L-1"); len(ids) != 0 {
		t.Fatalf("admin still sees hidden review: %v", ids)
	}
}

func TestHideReviewUpdatesSummaries(t *testing.T) {
	f := newReviewFixture(t)
	f.seed(t, "L-1", "s-1", 5, true)
	f.seed(t, "L-1", "s-1", 3, true)
	bad := f.seed(t, "L-1", "s-1", 1, true)
	if _, err := f.hide(adminCtx(), bad.ID, "abuse"); err != nil {
		t.Fatal(err)
	}
	ls, err := f.h.GetListingRatingSummary(context.Background(), &engagementv1.GetListingRatingSummaryRequest{ListingId: "L-1"})
	if err != nil || ls.GetAverageRating() != 4.0 || ls.GetReviewCount() != 2 {
		t.Fatalf("listing summary = %v err=%v, want avg 4.0 count 2", ls, err)
	}
	ss, err := f.h.GetShopRatingSummary(context.Background(), &engagementv1.GetShopRatingSummaryRequest{SellerId: "s-1"})
	if err != nil || ss.GetAverageRating() != 4.0 || ss.GetReviewCount() != 2 {
		t.Fatalf("shop summary = %v err=%v, want avg 4.0 count 2", ss, err)
	}
}

func TestHideReviewAuditEventFields(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	if _, err := f.hide(adminCtxAs("adm-1"), r.ID, "spam link"); err != nil {
		t.Fatal(err)
	}
	want := auditEvent{actor: "adm-1", review: r.ID, listing: "L-1", reason: "spam link"}
	if len(f.audit.events) != 1 || f.audit.events[0] != want {
		t.Fatalf("audit events = %+v, want exactly %+v", f.audit.events, want)
	}
	got, _ := f.repo.GetReview(context.Background(), r.ID)
	if got.HiddenBy != "adm-1" {
		t.Fatalf("hidden_by = %q, want the principal id adm-1", got.HiddenBy)
	}
}

func TestHideReviewNotFound(t *testing.T) {
	f := newReviewFixture(t)
	_, err := f.hide(adminCtx(), "no-such-review", "spam")
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
	if len(f.audit.events) != 0 {
		t.Fatal("audit event written for unknown review")
	}
}

func TestHideReviewEmptyReasonInvalidArgument(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	for _, reason := range []string{"", "   ", "\t\n"} {
		if _, err := f.hide(adminCtx(), r.ID, reason); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("reason %q: want InvalidArgument, got %v", reason, err)
		}
	}
	if len(f.audit.events) != 0 {
		t.Fatal("audit event written for invalid reason")
	}
	f.stillVisible(t, r.ID)
}

func TestHideReviewReasonLengthBoundary(t *testing.T) {
	f := newReviewFixture(t)
	ok := f.seed(t, "L-1", "s-1", 1, true)
	tooLong := f.seed(t, "L-1", "s-1", 2, true)
	multi := f.seed(t, "L-1", "s-1", 3, true)
	multiLong := f.seed(t, "L-1", "s-1", 4, true)
	padded := f.seed(t, "L-1", "s-1", 5, true)

	cases := []struct {
		id     string
		reason string
		want   codes.Code
	}{
		{ok.ID, strings.Repeat("a", 500), codes.OK},
		{tooLong.ID, strings.Repeat("a", 501), codes.InvalidArgument},
		{multi.ID, strings.Repeat("ạ", 500), codes.OK}, // 1500 bytes, 500 runes
		{multiLong.ID, strings.Repeat("ạ", 501), codes.InvalidArgument},
		{padded.ID, "  " + strings.Repeat("a", 500) + "  ", codes.OK}, // counted after trim
	}
	for i, c := range cases {
		if _, err := f.hide(adminCtx(), c.id, c.reason); status.Code(err) != c.want {
			t.Fatalf("case %d: want %v, got %v", i, c.want, err)
		}
	}
	f.stillVisible(t, tooLong.ID)
	f.stillVisible(t, multiLong.ID)
}

func TestHideReviewReasonTrimmed(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	if _, err := f.hide(adminCtx(), r.ID, "  spam  "); err != nil {
		t.Fatal(err)
	}
	got, _ := f.repo.GetReview(context.Background(), r.ID)
	if got.HideReason != "spam" || len(f.audit.events) != 1 || f.audit.events[0].reason != "spam" {
		t.Fatalf("stored %q audit %+v, want both \"spam\"", got.HideReason, f.audit.events)
	}
}

func TestHideReviewEmptyIDInvalidArgument(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.hide(adminCtx(), "", "spam"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
}

func TestHideReviewNonAdminPermissionDenied(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	for name, ctx := range map[string]context.Context{
		"buyer":                 userCtx("buyer-1"),
		"seller":                principalCtx("s-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, append(append([]string{}, userScopes...), "listing.write")...),
		"service without admin": principalCtx("service-x", commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, "engagement:write", "engagement:read"),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := f.hide(ctx, r.ID, "spam")
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("want PermissionDenied, got %v", err)
			}
			if res != nil {
				t.Fatalf("nothing may be returned on rejection, got %v", res)
			}
			if len(f.audit.events) != 0 {
				t.Fatal("audit event written for a non-admin")
			}
			f.stillVisible(t, r.ID)
		})
	}
}

func TestHideReviewNonAdminInvalidReasonPermissionDenied(t *testing.T) {
	f := newReviewFixture(t)
	_, err := f.hide(userCtx("buyer-1"), "unknown-review", "")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("authorization must precede validation: want PermissionDenied, got %v", err)
	}
	if len(f.audit.events) != 0 {
		t.Fatal("audit event written")
	}
}

func TestHideReviewAnonymousUnauthenticated(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	for name, ctx := range map[string]context.Context{"no principal": context.Background(), "anonymous principal": anonCtx()} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.hide(ctx, r.ID, "spam"); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("want Unauthenticated, got %v", err)
			}
			if len(f.audit.events) != 0 {
				t.Fatal("audit event written")
			}
			f.stillVisible(t, r.ID)
		})
	}
}

func TestHideReviewIdempotentOK(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	if _, err := f.hide(adminCtxAs("adm-1"), r.ID, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.hide(adminCtxAs("adm-2"), r.ID, "second"); err != nil {
		t.Fatalf("re-hide must succeed, got %v", err)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(f.audit.events))
	}
}

func TestHideReviewUpstreamAndStorageErrorCodes(t *testing.T) {
	t.Run("audit down is Unavailable and review stays visible", func(t *testing.T) {
		f := newReviewFixture(t)
		r := f.seed(t, "L-1", "s-1", 1, true)
		f.audit.err = errors.New("audit down")
		if _, err := f.hide(adminCtx(), r.ID, "spam"); status.Code(err) != codes.Unavailable {
			t.Fatalf("want Unavailable, got %v", err)
		}
		f.stillVisible(t, r.ID)
	})
	t.Run("no audit upstream is Unavailable", func(t *testing.T) {
		f := newReviewFixture(t)
		r := f.seed(t, "L-1", "s-1", 1, true)
		svc := service.NewReviewService(f.repo, nil, nil) // never wired with audit
		h := handler.NewEngagementHandler(repository.NewInMemoryRepository(), svc, nil, nil, nil)
		_, err := h.HideReview(adminCtx(), &engagementv1.HideReviewRequest{ReviewId: r.ID, Reason: "spam"})
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("want Unavailable, got %v", err)
		}
		f.stillVisible(t, r.ID)
	})
	t.Run("hide update failure is Internal then retry succeeds", func(t *testing.T) {
		f := newReviewFixture(t)
		r := f.seed(t, "L-1", "s-1", 1, true)
		f.repo.err = errors.New("db down: secret-dsn")
		_, err := f.hide(adminCtx(), r.ID, "spam")
		if status.Code(err) != codes.Internal {
			t.Fatalf("want Internal, got %v", err)
		}
		if strings.Contains(err.Error(), "secret-dsn") {
			t.Fatalf("storage detail leaked to the client: %v", err)
		}
		f.repo.err = nil
		if _, err := f.hide(adminCtx(), r.ID, "spam"); err != nil {
			t.Fatalf("retry: %v", err)
		}
		if len(f.audit.events) != 2 {
			t.Fatalf("want 2 audit events, got %d", len(f.audit.events))
		}
	})
}

func TestHideReviewReasonNeverLogged(t *testing.T) {
	const secret = "sensitive-moderation-reason-7f3a"
	f := newReviewFixture(t)
	ok := f.seed(t, "L-1", "s-1", 1, true)
	failing := f.seed(t, "L-1", "s-1", 2, true)

	if _, err := f.hide(adminCtx(), ok.ID, secret); err != nil {
		t.Fatal(err)
	}
	f.audit.err = errors.New("audit down")
	_, _ = f.hide(adminCtx(), failing.ID, secret)
	f.audit.err = nil
	f.repo.err = errors.New("db down")
	_, _ = f.hide(adminCtx(), failing.ID, secret)

	if strings.Contains(f.logs.String(), secret) {
		t.Fatalf("hide reason leaked into logs:\n%s", f.logs.String())
	}
}

func TestMarkHelpfulHiddenReviewNotFound(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	if _, err := f.hide(adminCtx(), r.ID, "spam"); err != nil {
		t.Fatal(err)
	}
	_, err := f.h.MarkReviewHelpful(userCtx("buyer-1"), &engagementv1.MarkReviewHelpfulRequest{ReviewId: r.ID})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
	got, _ := f.repo.GetReview(context.Background(), r.ID)
	if got.HelpfulCount != 0 {
		t.Fatalf("helpful_count changed to %d", got.HelpfulCount)
	}
}
