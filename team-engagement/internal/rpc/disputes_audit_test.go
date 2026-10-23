package rpc_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/rpc"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

type disputeAuditFixture struct {
	h     *rpc.Handler
	repo  *storetest.FakeDisputeRepository
	audit *auditSpy
}

func newDisputeAuditFixture(t *testing.T) *disputeAuditFixture {
	t.Helper()
	repo := storetest.NewFakeDisputeRepository()
	audit := &auditSpy{}
	orders := &orderFake{orders: fakeOrders{"order-1": {buyer: "buyer-1", seller: "seller-1"}}}
	h := newHandler(t, deps{
		disputes: engagement.NewDisputeService(repo, auditx.NewGate(audit, nil), nil),
		orders:   orders,
	})
	return &disputeAuditFixture{h: h, repo: repo, audit: audit}
}

func (f *disputeAuditFixture) seed(t *testing.T, st store.DisputeStatus) string {
	t.Helper()
	d, err := f.repo.CreateDispute(context.Background(), store.Dispute{
		OrderID: "o-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "r", Status: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d.ID
}

func (f *disputeAuditFixture) resolve(ctx context.Context, id string, st engagementv1.DisputeStatus, note string) error {
	_, err := f.h.ResolveDispute(ctx, &engagementv1.ResolveDisputeRequest{DisputeId: id, Status: st, Resolution: note})
	return err
}

func (f *disputeAuditFixture) stored(t *testing.T, id string) store.Dispute {
	t.Helper()
	d, err := f.repo.GetDispute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *disputeAuditFixture) assertNoEvent(t *testing.T) {
	t.Helper()
	if len(f.audit.generic) != 0 {
		t.Fatalf("no audit event expected, got %+v", f.audit.generic)
	}
}

const (
	stResolved      = engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED
	stRejected      = engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED
	stInvestigating = engagementv1.DisputeStatus_DISPUTE_STATUS_INVESTIGATING
)

// Scenario: Dispute audit: Admin resolves a dispute and one event is written
func TestResolveDisputeWritesAuditEvent(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusInvestigating)
	if err := f.resolve(adminCtxAs("adm-1"), id, stResolved, "refund buyer"); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if f.stored(t, id).Status != store.DisputeStatusResolved {
		t.Fatal("dispute must be resolved")
	}
	if len(f.audit.generic) != 1 {
		t.Fatalf("want exactly 1 event, got %+v", f.audit.generic)
	}
	ev := f.audit.generic[0]
	if ev.Action != "dispute.resolve" || ev.TargetType != "dispute" || ev.TargetID != id || ev.ActorID != "adm-1" {
		t.Errorf("unexpected event %+v", ev)
	}
	if m := ev.Metadata; len(m) != 5 || m["ruling"] != "" || m["return_id"] != "" || m["resolution"] != "refund buyer" || m["order_id"] != "o-1" || m["status"] != "resolved" {
		t.Errorf("metadata = %v", m)
	}
}

// Scenario: Dispute audit: Investigating dispute can be updated again with one event each
func TestResolveDisputeInvestigatingRepeatAudited(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusOpen)
	for i := 0; i < 2; i++ {
		if err := f.resolve(adminCtx(), id, stInvestigating, "looking"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if len(f.audit.generic) != 2 {
		t.Fatalf("want 2 events, got %d", len(f.audit.generic))
	}
}

// Scenario: Dispute audit: Any permitted target status is audited
func TestResolveDisputeAuditsEveryStatus(t *testing.T) {
	f := newDisputeAuditFixture(t)
	for st, want := range map[engagementv1.DisputeStatus]string{stRejected: "rejected", stInvestigating: "investigating"} {
		before := len(f.audit.generic)
		if err := f.resolve(adminCtx(), f.seed(t, store.DisputeStatusOpen), st, "n"); err != nil {
			t.Fatalf("%v: %v", st, err)
		}
		if len(f.audit.generic) != before+1 || f.audit.generic[before].Metadata["status"] != want {
			t.Fatalf("%v: events = %+v, want one with status %q", st, f.audit.generic, want)
		}
	}
}

// Scenario: Dispute audit: Resolution note is trimmed in the event
func TestResolveDisputeAuditResolutionTrimmed(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusInvestigating)
	if err := f.resolve(adminCtx(), id, stResolved, "  refund  "); err != nil {
		t.Fatal(err)
	}
	if len(f.audit.generic) != 1 || f.audit.generic[0].Metadata["resolution"] != "refund" {
		t.Fatalf("events = %+v", f.audit.generic)
	}
	if got := f.stored(t, id).Resolution; got != "refund" {
		t.Fatalf("stored resolution = %q", got)
	}
}

// Scenario: Dispute audit: Empty resolution is audited as empty
func TestResolveDisputeEmptyResolutionAudited(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusOpen)
	if err := f.resolve(adminCtx(), id, stInvestigating, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.audit.generic) != 1 {
		t.Fatalf("events = %+v", f.audit.generic)
	}
	if v, ok := f.audit.generic[0].Metadata["resolution"]; !ok || v != "" {
		t.Fatalf("resolution = %q present=%v", v, ok)
	}
}

// Scenario: Dispute audit: Audit error leaves the dispute open (handler maps to Unavailable)
func TestResolveDisputeAuditDownIsUnavailable(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusInvestigating)
	f.audit.err = errors.New("team-audit down")
	if got := status.Code(f.resolve(adminCtx(), id, stResolved, "x")); got != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", got)
	}
	if d := f.stored(t, id); d.Status != store.DisputeStatusInvestigating || d.Resolution != "" {
		t.Fatalf("dispute must be unchanged: %+v", d)
	}
}

// Scenario: Dispute audit: Other dispute RPCs ignore team-audit availability
func TestDisputeReadWriteIndependentOfAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	f.audit.err = errors.New("team-audit down")
	res, err := f.h.CreateDispute(userCtx("buyer-1"), &engagementv1.CreateDisputeRequest{OrderId: "order-1", DefendantId: "seller-1", Reason: "broken"})
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	if _, err := f.h.GetDispute(userCtx("buyer-1"), &engagementv1.GetDisputeRequest{DisputeId: res.GetDispute().GetId()}); err != nil {
		t.Fatalf("GetDispute: %v", err)
	}
}

// Scenario: Dispute audit: Anonymous caller is unauthenticated
func TestResolveDisputeAnonymousNoAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusOpen)
	for name, ctx := range map[string]context.Context{"anonymous": anonCtx(), "no principal": context.Background()} {
		if got := status.Code(f.resolve(ctx, id, stResolved, "x")); got != codes.Unauthenticated {
			t.Fatalf("%s: want Unauthenticated, got %v", name, got)
		}
	}
	f.assertNoEvent(t)
}

// Scenario: Dispute audit: Non-admin is denied
func TestResolveDisputeNonAdminDeniedNoAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusOpen)
	for name, ctx := range map[string]context.Context{
		"buyer":   userCtx("buyer-1"),
		"seller":  userCtx("seller-1"),
		"service": principalCtx("service-x", auth.TypeService, "engagement:write", "engagement:read"),
	} {
		if got := status.Code(f.resolve(ctx, id, stResolved, "x")); got != codes.PermissionDenied {
			t.Fatalf("%s: want PermissionDenied, got %v", name, got)
		}
	}
	f.assertNoEvent(t)
	if d := f.stored(t, id); d.Status != store.DisputeStatusOpen {
		t.Fatalf("dispute changed: %+v", d)
	}
}

// Scenario: Dispute audit: Authorization precedes validation
func TestResolveDisputeAuthBeforeValidationAndAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	f.audit.err = errors.New("team-audit down")
	if got := status.Code(f.resolve(userCtx("buyer-1"), "", stResolved, "x")); got != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", got)
	}
}

// Scenario: Dispute audit: Empty dispute id is rejected
func TestResolveDisputeEmptyIDNoAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	if got := status.Code(f.resolve(adminCtx(), "", stResolved, "x")); got != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", got)
	}
	f.assertNoEvent(t)
}

// Scenario: Dispute audit: Invalid status is rejected
func TestResolveDisputeInvalidStatusNoAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	id := f.seed(t, store.DisputeStatusOpen)
	for _, st := range []engagementv1.DisputeStatus{engagementv1.DisputeStatus_DISPUTE_STATUS_UNSPECIFIED, engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN} {
		if got := status.Code(f.resolve(adminCtx(), id, st, "x")); got != codes.InvalidArgument {
			t.Fatalf("%v: want InvalidArgument, got %v", st, got)
		}
	}
	f.assertNoEvent(t)
}

// Scenario: Dispute audit: Unknown dispute
func TestResolveDisputeNotFoundNoAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	if got := status.Code(f.resolve(adminCtx(), "nope", stResolved, "x")); got != codes.NotFound {
		t.Fatalf("want NotFound, got %v", got)
	}
	f.audit.err = errors.New("team-audit down")
	if got := status.Code(f.resolve(adminCtx(), "nope", stResolved, "x")); got != codes.NotFound {
		t.Fatalf("audit down: want NotFound, got %v", got)
	}
	f.assertNoEvent(t)
}

// Scenario: Dispute audit: Already closed dispute is not re-audited
func TestResolveDisputeClosedNoAudit(t *testing.T) {
	f := newDisputeAuditFixture(t)
	for _, st := range []store.DisputeStatus{store.DisputeStatusResolved, store.DisputeStatusRejected} {
		id := f.seed(t, st)
		if got := status.Code(f.resolve(adminCtx(), id, stResolved, "x")); got != codes.FailedPrecondition {
			t.Fatalf("%s: want FailedPrecondition, got %v", st, got)
		}
	}
	f.assertNoEvent(t)
}
