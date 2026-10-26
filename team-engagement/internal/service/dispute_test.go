package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

func TestDisputeService(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryDisputeRepository()
	svc := service.NewDisputeService(repo, nil).WithAudit(&disputeAuditFake{log: &disputeCallLog{}})

	t.Run("CreateDispute Validations", func(t *testing.T) {
		// Empty order ID
		_, err := svc.CreateDispute(ctx, "", "buyer-1", "seller-1", "", "Hỏng hàng", nil)
		if err != service.ErrEmptyOrderID {
			t.Fatalf("expected ErrEmptyOrderID, got %v", err)
		}

		// Empty claimant ID
		_, err = svc.CreateDispute(ctx, "order-1", "", "seller-1", "", "Hỏng hàng", nil)
		if err != service.ErrEmptyClaimantID {
			t.Fatalf("expected ErrEmptyClaimantID, got %v", err)
		}

		// Empty defendant ID
		_, err = svc.CreateDispute(ctx, "order-1", "buyer-1", "", "", "Hỏng hàng", nil)
		if err != service.ErrEmptyDefendantID {
			t.Fatalf("expected ErrEmptyDefendantID, got %v", err)
		}

		// Claimant and Defendant same
		_, err = svc.CreateDispute(ctx, "order-1", "user-1", "user-1", "", "Hỏng hàng", nil)
		if err != service.ErrSameClaimantAndDef {
			t.Fatalf("expected ErrSameClaimantAndDef, got %v", err)
		}

		// Empty reason
		_, err = svc.CreateDispute(ctx, "order-1", "buyer-1", "seller-1", "", "   ", nil)
		if err != service.ErrEmptyReason {
			t.Fatalf("expected ErrEmptyReason, got %v", err)
		}
	})

	t.Run("Lifecycle: Create -> Get -> Resolve", func(t *testing.T) {
		disp, err := svc.CreateDispute(ctx, "order-101", "buyer-101", "seller-101", "", "Sản phẩm bị trầy xước nặng", []string{"https://img.com/scratch.png"})
		if err != nil {
			t.Fatalf("unexpected error creating dispute: %v", err)
		}
		if disp.ID == "" {
			t.Fatal("expected dispute ID")
		}
		if disp.Status != repository.DisputeStatusOpen {
			t.Fatalf("expected OPEN status, got %s", disp.Status)
		}

		// Get dispute validation
		_, err = svc.GetDispute(ctx, "")
		if err != service.ErrEmptyDisputeID {
			t.Fatalf("expected ErrEmptyDisputeID, got %v", err)
		}

		// Get dispute
		fetched, err := svc.GetDispute(ctx, disp.ID)
		if err != nil {
			t.Fatalf("unexpected error getting dispute: %v", err)
		}
		if fetched.Reason != "Sản phẩm bị trầy xước nặng" {
			t.Fatalf("unexpected reason: %s", fetched.Reason)
		}

		// Resolve dispute validation
		_, err = svc.ResolveDispute(ctx, "adm-1", "", repository.DisputeStatusResolved, 0, "Xong")
		if err != service.ErrEmptyDisputeID {
			t.Fatalf("expected ErrEmptyDisputeID, got %v", err)
		}

		_, err = svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatus("INVALID_STATUS"), 0, "Xong")
		if err != service.ErrInvalidDisputeStatus {
			t.Fatalf("expected ErrInvalidDisputeStatus, got %v", err)
		}

		// Resolve dispute successfully
		resolved, err := svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatusInvestigating, 0, "Hoàn tiền 50% cho người mua")
		if err != nil {
			t.Fatalf("investigating: %v", err)
		}
		resolved, err = svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatusResolved, 0, "Hoàn tiền 50% cho người mua")
		if err != nil {
			t.Fatalf("unexpected error resolving dispute: %v", err)
		}
		if resolved.Status != repository.DisputeStatusResolved {
			t.Fatalf("expected RESOLVED status, got %s", resolved.Status)
		}
		if resolved.Resolution != "Hoàn tiền 50% cho người mua" {
			t.Fatalf("unexpected resolution: %s", resolved.Resolution)
		}

		// Try resolving already closed dispute -> ErrDisputeClosed
		_, err = svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatusRejected, 0, "Từ chối sau khi đã đóng")
		if !errors.Is(err, service.ErrInvalidTransition) && !errors.Is(err, service.ErrDisputeClosed) {
			t.Fatalf("expected ErrDisputeClosed, got %v", err)
		}
	})
}

// ── Dispute escalation (engagement-dispute-escalation task 3) ──

type disputeCallLog struct{ calls []string }

// disputeAuditFake is the true external boundary (team-audit).
type disputeAuditFake struct {
	log    *disputeCallLog
	events []upstream.AuditEvent
	err    error
}

func (a *disputeAuditFake) WriteHideEvent(context.Context, string, string, string, string) error {
	return nil
}

func (a *disputeAuditFake) WriteEvent(_ context.Context, ev upstream.AuditEvent) error {
	a.log.calls = append(a.log.calls, "audit")
	if a.err != nil {
		return a.err
	}
	a.events = append(a.events, ev)
	return nil
}

// disputeSpyRepo records call order and the facts handed to ResolveWithEvent;
// facts are only kept when the wrapped repository accepted the update.
type disputeSpyRepo struct {
	*repository.InMemoryDisputeRepository
	log        *disputeCallLog
	updateErr  error
	resolveErr error
	facts      []*engagementv1.DisputeResolved
}

func (r *disputeSpyRepo) UpdateDispute(ctx context.Context, d repository.Dispute) (repository.Dispute, error) {
	r.log.calls = append(r.log.calls, "update")
	if r.updateErr != nil {
		return repository.Dispute{}, r.updateErr
	}
	return r.InMemoryDisputeRepository.UpdateDispute(ctx, d)
}

func (r *disputeSpyRepo) ResolveWithEvent(ctx context.Context, d repository.Dispute, prev repository.DisputeStatus, fact proto.Message) (repository.Dispute, error) {
	r.log.calls = append(r.log.calls, "resolve")
	if r.resolveErr != nil {
		return repository.Dispute{}, r.resolveErr
	}
	got, err := r.InMemoryDisputeRepository.ResolveWithEvent(ctx, d, prev, fact)
	if err == nil {
		if f, ok := fact.(*engagementv1.DisputeResolved); ok {
			r.facts = append(r.facts, f)
		}
	}
	return got, err
}

type disputeFixture struct {
	svc   *service.DisputeService
	repo  *disputeSpyRepo
	audit *disputeAuditFake
	log   *disputeCallLog
}

func newDisputeFixture() *disputeFixture {
	log := &disputeCallLog{}
	repo := &disputeSpyRepo{InMemoryDisputeRepository: repository.NewInMemoryDisputeRepository(), log: log}
	audit := &disputeAuditFake{log: log}
	return &disputeFixture{svc: service.NewDisputeService(repo, nil).WithAudit(audit), repo: repo, audit: audit, log: log}
}

func (f *disputeFixture) seed(t *testing.T, status repository.DisputeStatus, returnID string) string {
	t.Helper()
	d, err := f.repo.InMemoryDisputeRepository.CreateDispute(context.Background(), repository.Dispute{
		OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "r",
		Status: status, ReturnID: returnID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d.ID
}

func (f *disputeFixture) get(t *testing.T, id string) repository.Dispute {
	t.Helper()
	d, err := f.repo.InMemoryDisputeRepository.GetDispute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *disputeFixture) assertUntouched(t *testing.T, id string, status repository.DisputeStatus) {
	t.Helper()
	got := f.get(t, id)
	if got.Status != status || got.Resolution != "" || got.Ruling != engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		t.Fatalf("dispute must be unchanged: %+v", got)
	}
	if len(f.audit.events) != 0 || len(f.repo.facts) != 0 {
		t.Fatalf("no audit event / fact expected, got %d / %d", len(f.audit.events), len(f.repo.facts))
	}
}

const (
	buyerFavour  = engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR
	sellerFavour = engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR
)

// Scenario: Dispute escalation: Open dispute moves to investigating
func TestResolveDisputeOpenToInvestigating(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusOpen, "r-1")
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusInvestigating, 0, "looking into it")
	if err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if got.Status != repository.DisputeStatusInvestigating || got.Resolution != "looking into it" {
		t.Fatalf("got %+v", got)
	}
	if st := f.get(t, id); st.Status != repository.DisputeStatusInvestigating || st.Resolution != "looking into it" {
		t.Fatalf("persisted %+v", st)
	}
	if len(f.repo.facts) != 0 {
		t.Fatalf("no DisputeResolved fact for a non-closing move, got %d", len(f.repo.facts))
	}
}

// Scenario: Dispute escalation: Investigating note can be updated
func TestResolveDisputeInvestigatingNoteUpdate(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusInvestigating, 0, "  new note ")
	if err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if got.Status != repository.DisputeStatusInvestigating || got.Resolution != "new note" {
		t.Fatalf("got %+v", got)
	}
	if st := f.get(t, id); st.Status != repository.DisputeStatusInvestigating || st.Resolution != "new note" {
		t.Fatalf("persisted %+v", st)
	}
}

// Scenario: Dispute escalation: Dispute without a return resolves without a ruling
func TestResolveDisputeWithoutReturnNoRulingNeeded(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "")
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, 0, "settled")
	if err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if got.Status != repository.DisputeStatusResolved {
		t.Fatalf("status = %s", got.Status)
	}
	if len(f.repo.facts) != 1 || f.repo.facts[0].GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		t.Fatalf("facts = %v", f.repo.facts)
	}
}

// Scenario: Dispute escalation: Audit is written before the update
func TestResolveDisputeAuditBeforeUpdateAndEvent(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, buyerFavour, " refund buyer "); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if got := f.log.calls; len(got) != 2 || got[0] != "audit" || got[1] != "resolve" {
		t.Fatalf("call order = %v, want [audit resolve]", got)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(f.audit.events))
	}
	ev := f.audit.events[0]
	if ev.ActorID != "adm-1" || ev.Action != "dispute.resolve" || ev.TargetType != "dispute" || ev.TargetID != id {
		t.Errorf("unexpected event %+v", ev)
	}
	want := map[string]string{"resolution": "refund buyer", "order_id": "o-1", "status": "resolved", "ruling": "buyer_favour", "return_id": "r-1"}
	if len(ev.Metadata) != len(want) {
		t.Errorf("metadata = %v, want %v", ev.Metadata, want)
	}
	for k, v := range want {
		if ev.Metadata[k] != v {
			t.Errorf("metadata[%s] = %q, want %q", k, ev.Metadata[k], v)
		}
	}
	if len(f.repo.facts) != 1 {
		t.Fatalf("want 1 fact, got %d", len(f.repo.facts))
	}
	fact := f.repo.facts[0]
	if fact.GetDisputeId() != id || fact.GetOrderId() != "o-1" || fact.GetReturnId() != "r-1" ||
		fact.GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED ||
		fact.GetRuling() != buyerFavour || fact.GetResolverId() != "adm-1" {
		t.Errorf("fact = %v", fact)
	}
	if fact.GetResolvedAt() == nil || time.Since(fact.GetResolvedAt().AsTime()) > time.Minute {
		t.Errorf("resolved_at = %v", fact.GetResolvedAt())
	}
}

// Scenario: Dispute escalation: Seller-favour ruling is announced
func TestResolveDisputeSellerFavourFact(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, sellerFavour, "no fault")
	if err != nil || got.Ruling != sellerFavour {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(f.repo.facts) != 1 || f.repo.facts[0].GetRuling() != sellerFavour {
		t.Fatalf("facts = %v", f.repo.facts)
	}
	if f.audit.events[0].Metadata["ruling"] != "seller_favour" {
		t.Errorf("metadata = %v", f.audit.events[0].Metadata)
	}
}

// Scenario: Dispute escalation: Rejected dispute carries no ruling
func TestResolveDisputeRejectedFactNoRuling(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusRejected, 0, "no merit"); err != nil {
		t.Fatal(err)
	}
	if len(f.repo.facts) != 1 || f.repo.facts[0].GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED ||
		f.repo.facts[0].GetRuling() != engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED {
		t.Fatalf("facts = %v", f.repo.facts)
	}
	if f.audit.events[0].Metadata["ruling"] != "" {
		t.Errorf("ruling metadata = %q", f.audit.events[0].Metadata["ruling"])
	}
}

// Scenario: Dispute escalation: Open dispute can be rejected as spam
func TestResolveDisputeOpenToRejectedService(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusOpen, "r-1")
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusRejected, 0, "spam")
	if err != nil || got.Status != repository.DisputeStatusRejected {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(f.audit.events) != 1 || len(f.repo.facts) != 1 {
		t.Fatalf("events=%d facts=%d", len(f.audit.events), len(f.repo.facts))
	}
}

// Scenario: Dispute escalation: Audit down blocks the ruling
func TestResolveDisputeAuditDownNoEvent(t *testing.T) {
	for name, mk := range map[string]func(f *disputeFixture) *service.DisputeService{
		"audit error": func(f *disputeFixture) *service.DisputeService { f.audit.err = errors.New("audit down"); return f.svc },
		"timeout":     func(f *disputeFixture) *service.DisputeService { f.audit.err = context.DeadlineExceeded; return f.svc },
		"never wired": func(f *disputeFixture) *service.DisputeService { return service.NewDisputeService(f.repo, nil) },
		"nil writer": func(f *disputeFixture) *service.DisputeService {
			return service.NewDisputeService(f.repo, nil).WithAudit(nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newDisputeFixture()
			id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
			svc := mk(f)
			_, err := svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, buyerFavour, "x")
			if !errors.Is(err, service.ErrAuditUnavailable) {
				t.Fatalf("want ErrAuditUnavailable, got %v", err)
			}
			f.assertUntouched(t, id, repository.DisputeStatusInvestigating)
			for _, c := range f.log.calls {
				if c == "update" || c == "resolve" {
					t.Fatalf("repository written despite audit failure: %v", f.log.calls)
				}
			}
		})
	}
}

// Scenario: Dispute escalation: A failed update leaves no event
func TestResolveDisputeUpdateFailureNoFactThenRetry(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
	f.repo.resolveErr = errors.New("db down")
	_, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, buyerFavour, "x")
	if err == nil || errors.Is(err, service.ErrAuditUnavailable) || errors.Is(err, service.ErrInvalidTransition) {
		t.Fatalf("want a plain internal error, got %v", err)
	}
	if len(f.repo.facts) != 0 || f.get(t, id).Status != repository.DisputeStatusInvestigating {
		t.Fatalf("failed update must leave dispute and facts untouched")
	}
	f.repo.resolveErr = nil
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, buyerFavour, "x")
	if err != nil || got.Status != repository.DisputeStatusResolved {
		t.Fatalf("retry = %+v, %v", got, err)
	}
	if len(f.repo.facts) != 1 {
		t.Fatalf("want exactly 1 fact after retry, got %d", len(f.repo.facts))
	}
}

// Scenario: Dispute escalation: Open dispute cannot be resolved directly
func TestResolveDisputeOpenToResolvedRefusedService(t *testing.T) {
	for _, ruling := range []engagementv1.DisputeRuling{0, buyerFavour} {
		f := newDisputeFixture()
		id := f.seed(t, repository.DisputeStatusOpen, "r-1")
		_, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, ruling, "x")
		if !errors.Is(err, service.ErrInvalidTransition) {
			t.Fatalf("ruling %v: want ErrInvalidTransition, got %v", ruling, err)
		}
		f.assertUntouched(t, id, repository.DisputeStatusOpen)
	}
}

// Scenario: Dispute escalation: Closed dispute stays closed
func TestResolveDisputeClosedStaysClosedService(t *testing.T) {
	for _, closed := range []repository.DisputeStatus{repository.DisputeStatusResolved, repository.DisputeStatusRejected} {
		for _, to := range []repository.DisputeStatus{repository.DisputeStatusInvestigating, repository.DisputeStatusResolved, repository.DisputeStatusRejected} {
			f := newDisputeFixture()
			id := f.seed(t, closed, "")
			_, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, to, 0, "x")
			if !errors.Is(err, service.ErrInvalidTransition) && !errors.Is(err, service.ErrDisputeClosed) {
				t.Fatalf("%s->%s: want invalid transition, got %v", closed, to, err)
			}
			f.assertUntouched(t, id, closed)
		}
	}
}

// Scenario: Dispute escalation: Return-linked resolution needs a ruling
func TestResolveReturnLinkedWithoutRulingService(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "r-1")
	_, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusResolved, 0, "x")
	if !errors.Is(err, service.ErrRulingRequired) {
		t.Fatalf("want ErrRulingRequired, got %v", err)
	}
	f.assertUntouched(t, id, repository.DisputeStatusInvestigating)
}

// Scenario: Dispute escalation: Ruling on a non-resolving status is invalid
func TestResolveRulingWithNonResolvedStatusService(t *testing.T) {
	cases := []struct {
		from, to repository.DisputeStatus
	}{
		{repository.DisputeStatusOpen, repository.DisputeStatusInvestigating},
		{repository.DisputeStatusInvestigating, repository.DisputeStatusRejected},
	}
	for _, c := range cases {
		f := newDisputeFixture()
		id := f.seed(t, c.from, "r-1")
		_, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, c.to, buyerFavour, "x")
		if !errors.Is(err, service.ErrRulingNotAllowed) {
			t.Fatalf("%s->%s: want ErrRulingNotAllowed, got %v", c.from, c.to, err)
		}
		f.assertUntouched(t, id, c.from)
	}
}

// Scenario: Dispute escalation: Open dispute moves to investigating (unknown id and invalid inputs)
func TestResolveDisputeInputValidationService(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "")
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", "", repository.DisputeStatusResolved, 0, "x"); !errors.Is(err, service.ErrEmptyDisputeID) {
		t.Fatalf("empty id: %v", err)
	}
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusOpen, 0, "x"); !errors.Is(err, service.ErrInvalidDisputeStatus) {
		t.Fatalf("OPEN target: %v", err)
	}
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", "missing", repository.DisputeStatusResolved, 0, "x"); !errors.Is(err, repository.ErrDisputeNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	f.assertUntouched(t, id, repository.DisputeStatusInvestigating)
}

// Scenario: Dispute escalation: Resolution note is trimmed and empty note audited as empty
func TestResolveDisputeNoteTrimmedAndEmptyAuditedService(t *testing.T) {
	f := newDisputeFixture()
	id := f.seed(t, repository.DisputeStatusInvestigating, "")
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", id, repository.DisputeStatusInvestigating, 0, ""); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.audit.events[0].Metadata["resolution"]; !ok || v != "" {
		t.Fatalf("resolution = %q present=%v", v, ok)
	}
	if v, ok := f.audit.events[0].Metadata["return_id"]; !ok || v != "" {
		t.Fatalf("return_id = %q present=%v", v, ok)
	}
}

// Scenario: Dispute escalation: GetDispute shows the link and the ruling (service half: create carries return_id)
func TestCreateDisputeCarriesReturnID(t *testing.T) {
	f := newDisputeFixture()
	d, err := f.svc.CreateDispute(context.Background(), "o-1", "buyer-1", "seller-1", " r-9 ", "broken", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.ReturnID != "r-9" || d.Status != repository.DisputeStatusOpen {
		t.Fatalf("got %+v", d)
	}
	if st := f.get(t, d.ID); st.ReturnID != "r-9" {
		t.Fatalf("persisted %+v", st)
	}
}

// Scenario: Dispute escalation: Second dispute on the same return is refused while one is open
func TestCreateDisputeSecondOpenForReturnRefusedService(t *testing.T) {
	f := newDisputeFixture()
	if _, err := f.svc.CreateDispute(context.Background(), "o-1", "buyer-1", "seller-1", "r-1", "a", nil); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CreateDispute(context.Background(), "o-1", "buyer-1", "seller-1", "r-1", "b", nil)
	if !errors.Is(err, repository.ErrOpenDisputeExists) {
		t.Fatalf("want ErrOpenDisputeExists, got %v", err)
	}
}

// Scenario: Dispute escalation: Buyer and seller list the order's disputes newest first
func TestListOrderDisputesNewestFirst(t *testing.T) {
	f := newDisputeFixture()
	ctx := context.Background()
	var ids []string
	for i := 0; i < 3; i++ {
		d, err := f.svc.CreateDispute(ctx, "o-1", "buyer-1", "seller-1", "", "r", nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
		time.Sleep(2 * time.Millisecond)
	}
	if _, err := f.svc.CreateDispute(ctx, "o-2", "buyer-1", "seller-1", "", "other", nil); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.ListOrderDisputes(ctx, "o-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != ids[2] || got[2].ID != ids[0] {
		t.Fatalf("got %+v, want newest first %v", got, ids)
	}
	empty, err := f.svc.ListOrderDisputes(ctx, "o-none")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v, %v", empty, err)
	}
	if _, err := f.svc.ListOrderDisputes(ctx, "  "); !errors.Is(err, service.ErrEmptyOrderID) {
		t.Fatalf("blank order id: %v", err)
	}
}

// Scenario: Dispute escalation: Admin lists open disputes oldest first
func TestListDisputesOldestFirstAndWritesNoAudit(t *testing.T) {
	f := newDisputeFixture()
	ctx := context.Background()
	var ids []string
	for i := 0; i < 3; i++ {
		d, err := f.svc.CreateDispute(ctx, "o-1", "buyer-1", "seller-1", "", "r", nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
		time.Sleep(2 * time.Millisecond)
	}
	got, _, err := f.svc.ListDisputes(ctx, repository.DisputeStatusOpen, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != ids[0] || got[2].ID != ids[2] {
		t.Fatalf("got %+v, want oldest first %v", got, ids)
	}
	if len(f.audit.events) != 0 {
		t.Fatalf("listing must not audit, got %d", len(f.audit.events))
	}
}

// Scenario: Dispute escalation: Pagination boundary
func TestListDisputesPaginationService(t *testing.T) {
	f := newDisputeFixture()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := f.svc.CreateDispute(ctx, "o-1", "buyer-1", "seller-1", "", "r", nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	p1, next, err := f.svc.ListDisputes(ctx, repository.DisputeStatusOpen, 2, "")
	if err != nil || len(p1) != 2 || next == "" {
		t.Fatalf("page1 = %d next=%q err=%v", len(p1), next, err)
	}
	p2, next2, err := f.svc.ListDisputes(ctx, repository.DisputeStatusOpen, 2, next)
	if err != nil || len(p2) != 1 || next2 != "" {
		t.Fatalf("page2 = %d next=%q err=%v", len(p2), next2, err)
	}
	if p2[0].ID == p1[0].ID || p2[0].ID == p1[1].ID {
		t.Fatal("pages overlap")
	}
}
