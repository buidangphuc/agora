package engagement_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"

	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func requireEvent(t *testing.T, ev auditx.Event, actor, action, targetType, targetID string, keys ...string) {
	t.Helper()
	if ev.ActorID != actor || ev.Action != action || ev.TargetType != targetType || ev.TargetID != targetID {
		t.Fatalf("event = %+v, want actor=%s action=%s target=%s/%s", ev, actor, action, targetType, targetID)
	}
	sort.Strings(keys)
	if got := sortedKeys(ev.Metadata); !reflect.DeepEqual(got, keys) {
		t.Fatalf("metadata keys = %v, want %v", got, keys)
	}
}

// Scenario: Engagement SDK: Audit is written before the state change
func TestGateWritesAuditBeforeStateChange(t *testing.T) {
	ctx := context.Background()

	t.Run("hide review", func(t *testing.T) {
		f := newHideFixture(ctx, t)
		if err := f.svc.HideReview(ctx, "adm-1", f.review.ID, "spam link"); err != nil {
			t.Fatal(err)
		}
		if got := f.log.calls; !reflect.DeepEqual(got, []string{"audit", "hide"}) {
			t.Fatalf("order = %v, want [audit hide]", got)
		}
		if len(f.audit.raw) != 1 {
			t.Fatalf("audit events = %d, want 1", len(f.audit.raw))
		}
		requireEvent(t, f.audit.raw[0], "adm-1", "review.hide", "review", f.review.ID, "reason", "listing_id")
		if f.audit.raw[0].Metadata["reason"] != "spam link" || f.audit.raw[0].Metadata["listing_id"] != "L-1" {
			t.Fatalf("metadata = %v", f.audit.raw[0].Metadata)
		}
	})

	t.Run("resolve dispute", func(t *testing.T) {
		f := newDisputeFixture()
		id := f.seed(ctx, t, store.DisputeStatusInvestigating, "r-1")
		if _, err := f.svc.ResolveDispute(ctx, "adm-1", id, store.DisputeStatusResolved, buyerFavour, "refund"); err != nil {
			t.Fatal(err)
		}
		if got := f.log.calls; !reflect.DeepEqual(got, []string{"audit", "resolve"}) {
			t.Fatalf("order = %v, want [audit resolve]", got)
		}
		if len(f.audit.events) != 1 {
			t.Fatalf("audit events = %d, want 1", len(f.audit.events))
		}
		requireEvent(t, f.audit.events[0], "adm-1", "dispute.resolve", "dispute", id,
			"resolution", "order_id", "status", "ruling", "return_id")
	})

	t.Run("close report", func(t *testing.T) {
		e := newRptEnv()
		e.seedOpen("rp-1", "u1", rptListingA)
		if _, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusActioned, "removed"); err != nil {
			t.Fatal(err)
		}
		if got := e.log.snapshot(); !reflect.DeepEqual(got, []string{"audit", "close"}) {
			t.Fatalf("order = %v, want [audit close]", got)
		}
		evs := e.audit.events()
		if len(evs) != 1 {
			t.Fatalf("audit events = %d, want 1", len(evs))
		}
		requireEvent(t, evs[0], "adm-1", "report.resolve", "report", "rp-1", "status", "listing_id", "resolution_note")
	})
}

// Scenario: Engagement SDK: Audit failure blocks hiding a review
func TestHideReviewAuditFailureFailsClosedOnGate(t *testing.T) {
	f := newHideFixture(t.Context(), t)
	f.audit.err = errors.New("team-audit down")
	err := f.svc.HideReview(context.Background(), "adm-1", f.review.ID, "spam")
	requireAuditUnavailable(t, err)
	if f.hiddenAt(t.Context(), t) != nil {
		t.Fatal("hidden_at set although the audit write failed")
	}
	for _, c := range f.log.calls {
		if c == "hide" {
			t.Fatalf("state change ran after a failed audit: %v", f.log.calls)
		}
	}
}

// Scenario: Engagement SDK: Audit failure blocks resolving a dispute
func TestResolveDisputeAuditFailureFailsClosedOnGate(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t.Context(), t, store.DisputeStatusInvestigating, "r-1")
	f.audit.err = errors.New("team-audit down")
	_, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, store.DisputeStatusResolved, buyerFavour, "x")
	requireAuditUnavailable(t, err)
	f.assertUntouched(t, id, store.DisputeStatusInvestigating)
	for _, c := range f.log.calls {
		if c == "update" || c == "resolve" {
			t.Fatalf("state change ran after a failed audit: %v", f.log.calls)
		}
	}
}

// Scenario: Engagement SDK: Audit failure blocks closing a report
func TestResolveReportAuditFailureFailsClosedOnGate(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	e.audit.err = errors.New("team-audit down")
	_, err := e.svc.ResolveReport(context.Background(), "adm-1", "rp-1", store.ReportStatusActioned, "n")
	requireAuditUnavailable(t, err)
	if e.repo.closes != 0 {
		t.Fatalf("CloseReport ran %d times after a failed audit", e.repo.closes)
	}
	got, _ := e.mem.GetReport(context.Background(), "rp-1")
	if got.Status != store.ReportStatusOpen {
		t.Fatalf("status = %s, want OPEN", got.Status)
	}
}

// Scenario: Engagement SDK: State change failure after audit is retryable
func TestGateActionErrorIsReturnedAndRetryWritesNewEvent(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("db down")

	t.Run("hide review", func(t *testing.T) {
		f := newHideFixture(ctx, t)
		f.repo.hideErr = boom
		err := f.svc.HideReview(ctx, "adm-1", f.review.ID, "spam")
		if !errors.Is(err, boom) || status.Code(err) == codes.Unavailable {
			t.Fatalf("first call err = %v, want the store error unchanged", err)
		}
		f.repo.hideErr = nil
		if err := f.svc.HideReview(ctx, "adm-1", f.review.ID, "spam"); err != nil {
			t.Fatalf("retry: %v", err)
		}
		if f.hiddenAt(ctx, t) == nil {
			t.Fatal("retry did not hide the review")
		}
		if len(f.audit.raw) != 2 {
			t.Fatalf("audit events = %d, want 2", len(f.audit.raw))
		}
	})

	t.Run("resolve dispute", func(t *testing.T) {
		f := newDisputeFixture()
		id := f.seed(ctx, t, store.DisputeStatusInvestigating, "r-1")
		f.repo.resolveErr = boom
		_, err := f.svc.ResolveDispute(ctx, "adm-1", id, store.DisputeStatusResolved, buyerFavour, "x")
		if !errors.Is(err, boom) || status.Code(err) == codes.Unavailable {
			t.Fatalf("first call err = %v, want the store error unchanged", err)
		}
		f.repo.resolveErr = nil
		if _, err := f.svc.ResolveDispute(ctx, "adm-1", id, store.DisputeStatusResolved, buyerFavour, "x"); err != nil {
			t.Fatalf("retry: %v", err)
		}
		if len(f.audit.events) != 2 || len(f.repo.facts) != 1 {
			t.Fatalf("audit events = %d (want 2), facts = %d (want 1)", len(f.audit.events), len(f.repo.facts))
		}
	})

	t.Run("close report", func(t *testing.T) {
		e := newRptEnv()
		e.seedOpen("rp-1", "u1", rptListingA)
		e.repo.closeErrs = []error{boom}
		_, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusActioned, "n")
		if !errors.Is(err, boom) || status.Code(err) == codes.Unavailable {
			t.Fatalf("first call err = %v, want the store error unchanged", err)
		}
		if _, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusActioned, "n"); err != nil {
			t.Fatalf("retry: %v", err)
		}
		evs := e.audit.events()
		if len(evs) != 2 || evs[0].TargetID != "rp-1" || evs[1].TargetID != "rp-1" {
			t.Fatalf("audit events = %+v, want 2 for rp-1", evs)
		}
	})
}

type blockingWriter struct{ calls int }

func (b *blockingWriter) WriteAuditEvent(ctx context.Context, _ auditx.Event) error {
	b.calls++
	<-ctx.Done()
	return ctx.Err()
}

// Scenario: Engagement SDK: Audit failure blocks resolving a dispute (silent team-audit)
func TestResolveDisputeAuditTimeoutUnavailable(t *testing.T) {
	log := &disputeCallLog{}
	repo := &disputeSpyRepo{FakeDisputeRepository: storetest.NewFakeDisputeRepository(), log: log}
	w := &blockingWriter{}
	svc := engagement.NewDisputeService(repo, auditx.NewGate(w, nil), nil)
	d, err := repo.CreateDispute(context.Background(), store.Dispute{
		OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "r",
		Status: store.DisputeStatusInvestigating, ReturnID: "r-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = svc.ResolveDispute(context.Background(), "adm-1", d.ID, store.DisputeStatusResolved, buyerFavour, "x")
	requireAuditUnavailable(t, err)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %v, want within 2 s plus 1 s", elapsed)
	}
	if w.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", w.calls)
	}
	got, _ := repo.GetDispute(context.Background(), d.ID)
	if got.Status != store.DisputeStatusInvestigating || len(log.calls) != 0 {
		t.Fatalf("state changed despite timeout: %+v calls=%v", got, log.calls)
	}
}

// Scenario: Engagement SDK: Audit failure blocks hiding a review (validation happens before the gate)
func TestGateNotReachedWhenValidationFails(t *testing.T) {
	ctx := context.Background()
	f := newHideFixture(ctx, t)
	if err := f.svc.HideReview(ctx, "adm-1", f.review.ID, "   "); !errors.Is(err, engagement.ErrEmptyReason) {
		t.Fatalf("hide with blank reason: %v", err)
	}
	if err := f.svc.HideReview(ctx, "adm-1", "", "spam"); !errors.Is(err, engagement.ErrEmptyReviewID) {
		t.Fatalf("hide with empty id: %v", err)
	}
	if err := f.svc.HideReview(ctx, "adm-1", "missing", "spam"); !errors.Is(err, store.ErrReviewNotFound) {
		t.Fatalf("hide unknown review: %v", err)
	}
	if len(f.audit.raw) != 0 {
		t.Fatalf("audit written for refused calls: %+v", f.audit.raw)
	}
}
