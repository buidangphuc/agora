package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

func TestDisputeService(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryDisputeRepository()
	svc := service.NewDisputeService(repo, nil)

	t.Run("CreateDispute Validations", func(t *testing.T) {
		// Empty order ID
		_, err := svc.CreateDispute(ctx, "", "buyer-1", "seller-1", "Hỏng hàng", nil)
		if err != service.ErrEmptyOrderID {
			t.Fatalf("expected ErrEmptyOrderID, got %v", err)
		}

		// Empty claimant ID
		_, err = svc.CreateDispute(ctx, "order-1", "", "seller-1", "Hỏng hàng", nil)
		if err != service.ErrEmptyClaimantID {
			t.Fatalf("expected ErrEmptyClaimantID, got %v", err)
		}

		// Empty defendant ID
		_, err = svc.CreateDispute(ctx, "order-1", "buyer-1", "", "Hỏng hàng", nil)
		if err != service.ErrEmptyDefendantID {
			t.Fatalf("expected ErrEmptyDefendantID, got %v", err)
		}

		// Claimant and Defendant same
		_, err = svc.CreateDispute(ctx, "order-1", "user-1", "user-1", "Hỏng hàng", nil)
		if err != service.ErrSameClaimantAndDef {
			t.Fatalf("expected ErrSameClaimantAndDef, got %v", err)
		}

		// Empty reason
		_, err = svc.CreateDispute(ctx, "order-1", "buyer-1", "seller-1", "   ", nil)
		if err != service.ErrEmptyReason {
			t.Fatalf("expected ErrEmptyReason, got %v", err)
		}
	})

	t.Run("Lifecycle: Create -> Get -> Resolve", func(t *testing.T) {
		disp, err := svc.CreateDispute(ctx, "order-101", "buyer-101", "seller-101", "Sản phẩm bị trầy xước nặng", []string{"https://img.com/scratch.png"})
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
		_, err = svc.ResolveDispute(ctx, "adm-1", "", repository.DisputeStatusResolved, "Xong")
		if err != service.ErrEmptyDisputeID {
			t.Fatalf("expected ErrEmptyDisputeID, got %v", err)
		}

		_, err = svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatus("INVALID_STATUS"), "Xong")
		if err != service.ErrInvalidDisputeStatus {
			t.Fatalf("expected ErrInvalidDisputeStatus, got %v", err)
		}

		// Resolve dispute successfully
		resolved, err := svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatusResolved, "Hoàn tiền 50% cho người mua")
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
		_, err = svc.ResolveDispute(ctx, "adm-1", disp.ID, repository.DisputeStatusRejected, "Từ chối sau khi đã đóng")
		if err != service.ErrDisputeClosed {
			t.Fatalf("expected ErrDisputeClosed, got %v", err)
		}
	})
}

// ── Dispute audit (engagement-dispute-audit) ──

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

type disputeSpyRepo struct {
	*repository.InMemoryDisputeRepository
	log       *disputeCallLog
	updateErr error
}

func (r *disputeSpyRepo) UpdateDispute(ctx context.Context, d repository.Dispute) (repository.Dispute, error) {
	r.log.calls = append(r.log.calls, "update")
	if r.updateErr != nil {
		return repository.Dispute{}, r.updateErr
	}
	return r.InMemoryDisputeRepository.UpdateDispute(ctx, d)
}

type disputeAuditFixture struct {
	svc   *service.DisputeService
	repo  *disputeSpyRepo
	audit *disputeAuditFake
	log   *disputeCallLog
	id    string
}

func newDisputeAuditFixture(t *testing.T) *disputeAuditFixture {
	t.Helper()
	log := &disputeCallLog{}
	repo := &disputeSpyRepo{InMemoryDisputeRepository: repository.NewInMemoryDisputeRepository(), log: log}
	d, err := repo.InMemoryDisputeRepository.CreateDispute(context.Background(), repository.Dispute{
		OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "r", Status: repository.DisputeStatusOpen,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit := &disputeAuditFake{log: log}
	return &disputeAuditFixture{svc: service.NewDisputeService(repo, nil).WithAudit(audit), repo: repo, audit: audit, log: log, id: d.ID}
}

func (f *disputeAuditFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	got, err := f.repo.InMemoryDisputeRepository.GetDispute(context.Background(), f.id)
	if err != nil || got.Status != repository.DisputeStatusOpen || got.Resolution != "" {
		t.Fatalf("dispute must be unchanged: %+v %v", got, err)
	}
}

// Scenario: Dispute audit: Audit is written before the update
func TestResolveDisputeAuditBeforeUpdate(t *testing.T) {
	f := newDisputeAuditFixture(t)
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "refund buyer"); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if got := f.log.calls; len(got) != 2 || got[0] != "audit" || got[1] != "update" {
		t.Fatalf("call order = %v, want [audit update]", got)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(f.audit.events))
	}
	ev := f.audit.events[0]
	if ev.ActorID != "adm-1" || ev.Action != "dispute.resolve" || ev.TargetType != "dispute" || ev.TargetID != f.id {
		t.Errorf("unexpected event %+v", ev)
	}
	if m := ev.Metadata; len(m) != 3 || m["resolution"] != "refund buyer" || m["order_id"] != "o-1" || m["status"] != "resolved" {
		t.Errorf("metadata = %v", m)
	}
}

// Scenario: Dispute audit: Audit error leaves the dispute open
func TestResolveDisputeAuditFailureFailsClosed(t *testing.T) {
	f := newDisputeAuditFixture(t)
	f.audit.err = errors.New("audit down")
	_, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "x")
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("want ErrAuditUnavailable, got %v", err)
	}
	f.assertUnchanged(t)
	for _, c := range f.log.calls {
		if c == "update" {
			t.Fatal("repository updated despite audit failure")
		}
	}
}

// Scenario: Dispute audit: timeout is Unavailable (service half)
func TestResolveDisputeAuditTimeoutUnavailable(t *testing.T) {
	f := newDisputeAuditFixture(t)
	f.audit.err = context.DeadlineExceeded
	_, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "x")
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("want ErrAuditUnavailable, got %v", err)
	}
	f.assertUnchanged(t)
}

// Scenario: Dispute audit: No audit upstream configured
func TestResolveDisputeNilAuditUnavailable(t *testing.T) {
	f := newDisputeAuditFixture(t)
	for name, svc := range map[string]*service.DisputeService{
		"never wired": service.NewDisputeService(f.repo, nil),
		"nil writer":  service.NewDisputeService(f.repo, nil).WithAudit(nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "x")
			if !errors.Is(err, service.ErrAuditUnavailable) {
				t.Fatalf("want ErrAuditUnavailable, got %v", err)
			}
			f.assertUnchanged(t)
		})
	}
}

// Scenario: Dispute audit: Update failure after audit is Internal and retryable
func TestResolveDisputeUpdateFailureInternalThenRetry(t *testing.T) {
	f := newDisputeAuditFixture(t)
	f.repo.updateErr = errors.New("db down")
	_, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "x")
	if err == nil || errors.Is(err, service.ErrAuditUnavailable) || errors.Is(err, service.ErrDisputeClosed) {
		t.Fatalf("want a plain internal error, got %v", err)
	}
	f.repo.updateErr = nil
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "x")
	if err != nil || got.Status != repository.DisputeStatusResolved {
		t.Fatalf("retry = %+v, %v", got, err)
	}
	if len(f.audit.events) != 2 {
		t.Fatalf("want 2 dispute.resolve events (one per attempt), got %d", len(f.audit.events))
	}
}

// Scenario: Dispute audit: Resolution note is trimmed in the event
func TestResolveDisputeAuditResolutionTrimmedService(t *testing.T) {
	f := newDisputeAuditFixture(t)
	got, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusResolved, "  refund  ")
	if err != nil || got.Resolution != "refund" {
		t.Fatalf("resolution = %q, %v", got.Resolution, err)
	}
	if len(f.audit.events) != 1 || f.audit.events[0].Metadata["resolution"] != "refund" {
		t.Fatalf("events = %+v", f.audit.events)
	}
}

// Scenario: Dispute audit: Empty resolution is audited as empty
func TestResolveDisputeEmptyResolutionAuditedService(t *testing.T) {
	f := newDisputeAuditFixture(t)
	if _, err := f.svc.ResolveDispute(context.Background(), "adm-1", f.id, repository.DisputeStatusInvestigating, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.audit.events) != 1 {
		t.Fatalf("events = %+v", f.audit.events)
	}
	if v, ok := f.audit.events[0].Metadata["resolution"]; !ok || v != "" {
		t.Fatalf("metadata resolution = %q present=%v, want present and empty", v, ok)
	}
}
