package handler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

const (
	rulingBuyer  = engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR
	rulingSeller = engagementv1.DisputeRuling_DISPUTE_RULING_SELLER_FAVOUR
	rulingNone   = engagementv1.DisputeRuling_DISPUTE_RULING_UNSPECIFIED
)

type countingOrders struct {
	orders fakeOrders
	err    error
	calls  int
}

func (c *countingOrders) GetOrderParties(ctx context.Context, id string) (string, string, error) {
	c.calls++
	if c.err != nil {
		return "", "", c.err
	}
	return c.orders.GetOrderParties(ctx, id)
}

type returnInfo struct {
	orderID     string
	status      orderv1.ReturnStatus
	canEscalate bool
}

type returnFake struct {
	returns map[string]returnInfo
	err     error
	calls   int
	ids     []string
}

func (r *returnFake) GetReturn(_ context.Context, id string) (string, orderv1.ReturnStatus, bool, error) {
	r.calls++
	r.ids = append(r.ids, id)
	if r.err != nil {
		return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, r.err
	}
	info, ok := r.returns[id]
	if !ok {
		return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, upstream.ErrReturnNotFound
	}
	return info.orderID, info.status, info.canEscalate, nil
}

type factRepo struct {
	*storetest.FakeDisputeRepository
	facts []*engagementv1.DisputeResolved
}

func (f *factRepo) ResolveWithEvent(ctx context.Context, d store.Dispute, prev store.DisputeStatus, fact proto.Message) (store.Dispute, error) {
	out, err := f.FakeDisputeRepository.ResolveWithEvent(ctx, d, prev, fact)
	if err != nil {
		return out, err
	}
	if dr, ok := fact.(*engagementv1.DisputeResolved); ok {
		f.facts = append(f.facts, dr)
	}
	return out, nil
}

type escFixture struct {
	h       *handler.EngagementHandler
	repo    *factRepo
	audit   *auditSpy
	orders  *countingOrders
	returns *returnFake
}

func newEscFixture(t *testing.T) *escFixture {
	t.Helper()
	repo := &factRepo{FakeDisputeRepository: storetest.NewFakeDisputeRepository()}
	audit := &auditSpy{}
	orders := &countingOrders{orders: fakeOrders{
		"o-1": {buyer: "buyer-1", seller: "seller-1"},
		"o-2": {buyer: "buyer-2", seller: "seller-2"},
	}}
	returns := &returnFake{returns: map[string]returnInfo{
		"r-1":     {orderID: "o-1", status: orderv1.ReturnStatus_RETURN_STATUS_REJECTED, canEscalate: true},
		"r-no":    {orderID: "o-1", status: orderv1.ReturnStatus_RETURN_STATUS_APPROVED, canEscalate: false},
		"r-other": {orderID: "o-2", status: orderv1.ReturnStatus_RETURN_STATUS_REJECTED, canEscalate: true},
	}}
	h := handler.NewEngagementHandler(
		storetest.NewFakeRepository(),
		service.NewReviewService(storetest.NewFakeReviewRepository(), nil, nil),
		service.NewQAService(storetest.NewFakeQARepository(), nil),
		service.NewDisputeService(repo, nil).WithAudit(audit),
		service.NewCollectionService(storetest.NewFakeCollectionRepository(), nil),
		handler.WithOrderParties(orders),
		handler.WithReturnReader(returns),
	)
	return &escFixture{h: h, repo: repo, audit: audit, orders: orders, returns: returns}
}

func (f *escFixture) seed(t *testing.T, d store.Dispute) string {
	t.Helper()
	if d.OrderID == "" {
		d.OrderID = "o-1"
	}
	if d.ClaimantID == "" {
		d.ClaimantID = "buyer-1"
	}
	if d.DefendantID == "" {
		d.DefendantID = "seller-1"
	}
	if d.Reason == "" {
		d.Reason = "r"
	}
	got, err := f.repo.CreateDispute(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return got.ID
}

func (f *escFixture) stored(t *testing.T, id string) store.Dispute {
	t.Helper()
	d, err := f.repo.GetDispute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *escFixture) create(ctx context.Context, orderID, defendant, returnID string) (*engagementv1.CreateDisputeResponse, error) {
	return f.h.CreateDispute(ctx, &engagementv1.CreateDisputeRequest{OrderId: orderID, DefendantId: defendant, Reason: "broken", ReturnId: returnID})
}

func (f *escFixture) rule(ctx context.Context, id string, st engagementv1.DisputeStatus, ruling engagementv1.DisputeRuling) (*engagementv1.ResolveDisputeResponse, error) {
	return f.h.ResolveDispute(ctx, &engagementv1.ResolveDisputeRequest{DisputeId: id, Status: st, Ruling: ruling, Resolution: "note"})
}

func (f *escFixture) assertNoAuditNoFact(t *testing.T) {
	t.Helper()
	if len(f.audit.generic) != 0 {
		t.Fatalf("no audit event expected, got %+v", f.audit.generic)
	}
	if len(f.repo.facts) != 0 {
		t.Fatalf("no DisputeResolved expected, got %v", f.repo.facts)
	}
}

func (f *escFixture) disputesOfOrder(t *testing.T, orderID string) []store.Dispute {
	t.Helper()
	ds, err := f.repo.ListByOrder(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

// Scenario: Dispute escalation: Buyer escalates a rejected return
func TestCreateDisputeEscalatesRejectedReturn(t *testing.T) {
	f := newEscFixture(t)
	res, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-1")
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	d := res.GetDispute()
	if d.GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN || d.GetReturnId() != "r-1" ||
		d.GetClaimantId() != "buyer-1" || d.GetDefendantId() != "seller-1" || d.GetOrderId() != "o-1" {
		t.Fatalf("unexpected dispute %v", d)
	}
	if f.returns.calls != 1 || f.returns.ids[0] != "r-1" {
		t.Fatalf("want exactly 1 return read of r-1, got %v", f.returns.ids)
	}
	if got := f.stored(t, d.GetId()); got.ReturnID != "r-1" || got.Status != store.DisputeStatusOpen {
		t.Fatalf("stored = %+v", got)
	}
}

// Scenario: Dispute escalation: Return that cannot be escalated is refused
func TestCreateDisputeReturnNotEscalatable(t *testing.T) {
	f := newEscFixture(t)
	_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-no")
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
		t.Fatalf("no dispute may be stored, got %d", n)
	}
}

// Scenario: Dispute escalation: Second dispute on the same return is refused while one is open
func TestCreateDisputeDuplicateOpenOnReturn(t *testing.T) {
	for _, st := range []store.DisputeStatus{store.DisputeStatusOpen, store.DisputeStatusInvestigating} {
		t.Run(string(st), func(t *testing.T) {
			f := newEscFixture(t)
			f.seed(t, store.Dispute{ReturnID: "r-1", Status: st})
			_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-1")
			if got := status.Code(err); got != codes.AlreadyExists {
				t.Fatalf("want AlreadyExists, got %v", err)
			}
			if n := len(f.disputesOfOrder(t, "o-1")); n != 1 {
				t.Fatalf("disputes for the return = %d, want 1", n)
			}
		})
	}
}

// Scenario: Dispute escalation: Unknown return or return of another order
func TestCreateDisputeReturnNotFoundOrWrongOrder(t *testing.T) {
	for name, rid := range map[string]string{"unknown return": "r-missing", "return of another order": "r-other"} {
		t.Run(name, func(t *testing.T) {
			f := newEscFixture(t)
			_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", rid)
			if got := status.Code(err); got != codes.NotFound {
				t.Fatalf("want NotFound, got %v", err)
			}
			if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
				t.Fatalf("no dispute may be stored, got %d", n)
			}
		})
	}
}

// Scenario: Dispute escalation: Not the buyer of the order
func TestCreateDisputeReturnForeignOrderNoReturnRead(t *testing.T) {
	f := newEscFixture(t)
	_, err := f.create(userCtx("buyer-2"), "o-1", "seller-1", "r-1")
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
	if f.returns.calls != 0 {
		t.Fatalf("team-order must receive 0 return reads, got %d", f.returns.calls)
	}
}

// Scenario: Dispute escalation: Escalation naming another defendant is invalid
func TestCreateDisputeReturnWrongDefendantInvalid(t *testing.T) {
	f := newEscFixture(t)
	_, err := f.create(userCtx("buyer-1"), "o-1", "someone-else", "r-1")
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if f.returns.calls != 0 {
		t.Fatalf("team-order must receive 0 return reads, got %d", f.returns.calls)
	}
	if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
		t.Fatalf("no dispute may be stored, got %d", n)
	}
}

// Scenario: Dispute escalation: Team-order unavailable fails closed
func TestCreateDisputeReturnUpstreamUnavailable(t *testing.T) {
	for name, e := range map[string]error{
		"down":           errors.New("connection refused"),
		"not configured": upstream.ErrOrderClientNotConfigured,
	} {
		t.Run(name, func(t *testing.T) {
			f := newEscFixture(t)
			f.returns.err = e
			_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-1")
			if got := status.Code(err); got != codes.Unavailable {
				t.Fatalf("want Unavailable, got %v", err)
			}
			if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
				t.Fatalf("no dispute may be stored, got %d", n)
			}
		})
	}
}

// Scenario: Dispute escalation: Team-order unavailable fails closed
func TestCreateDisputeReturnWithoutReaderUnavailable(t *testing.T) {
	repo := storetest.NewFakeDisputeRepository()
	h := handler.NewEngagementHandler(storetest.NewFakeRepository(),
		service.NewReviewService(storetest.NewFakeReviewRepository(), nil, nil),
		service.NewQAService(storetest.NewFakeQARepository(), nil),
		service.NewDisputeService(repo, nil).WithAudit(&auditSpy{}),
		service.NewCollectionService(storetest.NewFakeCollectionRepository(), nil),
		handler.WithOrderParties(fakeOrders{"o-1": {buyer: "buyer-1", seller: "seller-1"}}))
	_, err := h.CreateDispute(userCtx("buyer-1"), &engagementv1.CreateDisputeRequest{OrderId: "o-1", DefendantId: "seller-1", Reason: "r", ReturnId: "r-1"})
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if ds, _ := repo.ListByOrder(context.Background(), "o-1"); len(ds) != 0 {
		t.Fatalf("no dispute may be stored, got %d", len(ds))
	}
}

// Scenario: Dispute escalation: Dispute without return_id is unchanged
func TestCreateDisputeWithoutReturnUnchanged(t *testing.T) {
	f := newEscFixture(t)
	res, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "")
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	if res.GetDispute().GetReturnId() != "" || res.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN {
		t.Fatalf("unexpected dispute %v", res.GetDispute())
	}
	if f.returns.calls != 0 {
		t.Fatalf("team-order must receive 0 return reads, got %d", f.returns.calls)
	}
	if _, err := f.create(userCtx("buyer-1"), "o-1", "someone-else", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wrong defendant without return must stay PermissionDenied, got %v", err)
	}
}

// Scenario: Dispute escalation: Anonymous caller cannot create a dispute
func TestCreateDisputeReturnAnonymousDenied(t *testing.T) {
	f := newEscFixture(t)
	for name, ctx := range map[string]context.Context{"anonymous": anonCtx(), "no principal": context.Background()} {
		_, err := f.create(ctx, "o-1", "seller-1", "r-1")
		if got := status.Code(err); got != codes.Unauthenticated {
			t.Fatalf("%s: want Unauthenticated, got %v", name, err)
		}
	}
	if f.returns.calls != 0 || f.orders.calls != 0 {
		t.Fatalf("team-order must receive 0 calls, got returns=%d orders=%d", f.returns.calls, f.orders.calls)
	}
}

// Scenario: Dispute escalation: GetDispute shows the link and the ruling
func TestGetDisputeCarriesReturnAndRuling(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusResolved, Ruling: rulingBuyer})
	for _, who := range []string{"buyer-1", "seller-1"} {
		res, err := f.h.GetDispute(userCtx(who), &engagementv1.GetDisputeRequest{DisputeId: id})
		if err != nil {
			t.Fatalf("%s: %v", who, err)
		}
		if res.GetDispute().GetReturnId() != "r-1" || res.GetDispute().GetRuling() != rulingBuyer {
			t.Fatalf("%s: dispute = %v", who, res.GetDispute())
		}
	}
}

// Scenario: Dispute escalation: Open dispute cannot be resolved directly
func TestResolveDisputeOpenToResolvedRefused(t *testing.T) {
	for name, ruling := range map[string]engagementv1.DisputeRuling{"without ruling": rulingNone, "with ruling": rulingBuyer} {
		t.Run(name, func(t *testing.T) {
			f := newEscFixture(t)
			id := f.seed(t, store.Dispute{Status: store.DisputeStatusOpen})
			_, err := f.rule(adminCtx(), id, stResolved, ruling)
			if got := status.Code(err); got != codes.FailedPrecondition {
				t.Fatalf("want FailedPrecondition, got %v", err)
			}
			if f.stored(t, id).Status != store.DisputeStatusOpen {
				t.Fatal("dispute must stay OPEN")
			}
			f.assertNoAuditNoFact(t)
		})
	}
}

// Scenario: Dispute escalation: Open dispute can be rejected as spam
func TestResolveDisputeOpenToRejectedAllowed(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{Status: store.DisputeStatusOpen})
	res, err := f.rule(adminCtxAs("adm-1"), id, stRejected, rulingNone)
	if err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if res.GetDispute().GetStatus() != stRejected {
		t.Fatalf("status = %v", res.GetDispute().GetStatus())
	}
	if len(f.audit.generic) != 1 || f.audit.generic[0].Action != "dispute.resolve" {
		t.Fatalf("want exactly 1 dispute.resolve event, got %+v", f.audit.generic)
	}
	if len(f.repo.facts) != 1 {
		t.Fatalf("want exactly 1 DisputeResolved, got %d", len(f.repo.facts))
	}
	if fact := f.repo.facts[0]; fact.GetStatus() != stRejected || fact.GetRuling() != rulingNone || fact.GetDisputeId() != id {
		t.Fatalf("fact = %v", fact)
	}
}

// Scenario: Dispute escalation: Closed dispute stays closed
func TestResolveDisputeClosedRefused(t *testing.T) {
	for _, closed := range []store.DisputeStatus{store.DisputeStatusResolved, store.DisputeStatusRejected} {
		for _, target := range []engagementv1.DisputeStatus{stInvestigating, stResolved, stRejected} {
			f := newEscFixture(t)
			id := f.seed(t, store.Dispute{Status: closed, Resolution: "done"})
			ruling := rulingNone
			if target == stResolved {
				ruling = rulingBuyer
			}
			_, err := f.rule(adminCtx(), id, target, ruling)
			if got := status.Code(err); got != codes.FailedPrecondition {
				t.Fatalf("%s -> %v: want FailedPrecondition, got %v", closed, target, err)
			}
			if d := f.stored(t, id); d.Status != closed || d.Resolution != "done" {
				t.Fatalf("%s -> %v: dispute changed: %+v", closed, target, d)
			}
			f.assertNoAuditNoFact(t)
		}
	}
}

// Scenario: Dispute escalation: Return-linked resolution needs a ruling
func TestResolveReturnLinkedWithoutRulingInvalid(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
	_, err := f.rule(adminCtx(), id, stResolved, rulingNone)
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if f.stored(t, id).Status != store.DisputeStatusInvestigating {
		t.Fatal("dispute must stay INVESTIGATING")
	}
	f.assertNoAuditNoFact(t)
}

// Scenario: Dispute escalation: Ruling on a non-resolving status is invalid
func TestResolveRulingWithNonResolvedStatusInvalid(t *testing.T) {
	cases := []struct {
		name   string
		from   store.DisputeStatus
		target engagementv1.DisputeStatus
	}{
		{"investigating from open", store.DisputeStatusOpen, stInvestigating},
		{"rejected from investigating", store.DisputeStatusInvestigating, stRejected},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newEscFixture(t)
			id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: c.from})
			_, err := f.rule(adminCtx(), id, c.target, rulingBuyer)
			if got := status.Code(err); got != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
			if f.stored(t, id).Status != c.from {
				t.Fatal("dispute must be unchanged")
			}
			f.assertNoAuditNoFact(t)
		})
	}
}

// Scenario: Dispute escalation: Non-admin cannot rule
func TestResolveDisputeRulingNonAdminDenied(t *testing.T) {
	for _, c := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"claimant", userCtx("buyer-1"), codes.PermissionDenied},
		{"defendant", userCtx("seller-1"), codes.PermissionDenied},
		{"anonymous", anonCtx(), codes.Unauthenticated},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newEscFixture(t)
			id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
			_, err := f.rule(c.ctx, id, stResolved, rulingBuyer)
			if got := status.Code(err); got != c.want {
				t.Fatalf("want %v, got %v", c.want, err)
			}
			if d := f.stored(t, id); d.Status != store.DisputeStatusInvestigating || d.Ruling != rulingNone {
				t.Fatalf("dispute changed: %+v", d)
			}
		})
	}
}

// Scenario: Dispute escalation: A refused ruling writes no audit event and no fact
func TestResolveDisputeDeniedWritesNoAuditNoFact(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
	for _, ctx := range []context.Context{userCtx("buyer-1"), userCtx("seller-1"), anonCtx()} {
		if _, err := f.rule(ctx, id, stResolved, rulingBuyer); err == nil {
			t.Fatal("non-admin ruling must be refused")
		}
	}
	f.assertNoAuditNoFact(t)
}

// Scenario: Dispute escalation: Buyer-favour ruling writes the audit event and one fact
func TestResolveDisputeBuyerFavourAuditAndEvent(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
	before := time.Now().Add(-time.Minute)
	res, err := f.rule(adminCtxAs("adm-1"), id, stResolved, rulingBuyer)
	if err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if res.GetDispute().GetStatus() != stResolved || res.GetDispute().GetRuling() != rulingBuyer || res.GetDispute().GetReturnId() != "r-1" {
		t.Fatalf("response = %v", res.GetDispute())
	}
	if d := f.stored(t, id); d.Status != store.DisputeStatusResolved || d.Ruling != rulingBuyer {
		t.Fatalf("stored = %+v", d)
	}
	if len(f.audit.generic) != 1 {
		t.Fatalf("want exactly 1 audit event, got %+v", f.audit.generic)
	}
	ev := f.audit.generic[0]
	if ev.Action != "dispute.resolve" || ev.TargetID != id || ev.ActorID != "adm-1" ||
		ev.Metadata["ruling"] != "buyer_favour" || ev.Metadata["return_id"] != "r-1" || ev.Metadata["status"] != "resolved" {
		t.Fatalf("event = %+v", ev)
	}
	if len(f.repo.facts) != 1 {
		t.Fatalf("want exactly 1 DisputeResolved, got %d", len(f.repo.facts))
	}
	fact := f.repo.facts[0]
	if fact.GetDisputeId() != id || fact.GetOrderId() != "o-1" || fact.GetReturnId() != "r-1" ||
		fact.GetStatus() != stResolved || fact.GetRuling() != rulingBuyer || fact.GetResolverId() != "adm-1" {
		t.Fatalf("fact = %v", fact)
	}
	if fact.GetResolvedAt() == nil || fact.GetResolvedAt().AsTime().Before(before) {
		t.Fatalf("resolved_at not set: %v", fact.GetResolvedAt())
	}
}

// Scenario: Dispute escalation: Rejected dispute carries no ruling
func TestResolveDisputeRejectedEventNoRuling(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
	if _, err := f.rule(adminCtx(), id, stRejected, rulingNone); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if len(f.repo.facts) != 1 {
		t.Fatalf("want exactly 1 DisputeResolved, got %d", len(f.repo.facts))
	}
	if fact := f.repo.facts[0]; fact.GetStatus() != stRejected || fact.GetRuling() != rulingNone {
		t.Fatalf("fact = %v", fact)
	}
}

// Scenario: Dispute escalation: Seller-favour ruling is announced
func TestResolveDisputeSellerFavourEvent(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
	if _, err := f.rule(adminCtx(), id, stResolved, rulingSeller); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if len(f.repo.facts) != 1 || f.repo.facts[0].GetRuling() != rulingSeller {
		t.Fatalf("facts = %v", f.repo.facts)
	}
	if f.audit.generic[0].Metadata["ruling"] != "seller_favour" {
		t.Fatalf("audit metadata = %v", f.audit.generic[0].Metadata)
	}
}

// Scenario: Dispute escalation: Open dispute moves to investigating
func TestResolveDisputeInvestigatingWritesNoFact(t *testing.T) {
	f := newEscFixture(t)
	id := f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusOpen})
	if _, err := f.rule(adminCtx(), id, stInvestigating, rulingNone); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if f.stored(t, id).Status != store.DisputeStatusInvestigating || len(f.repo.facts) != 0 {
		t.Fatalf("status/facts wrong: %+v %v", f.stored(t, id), f.repo.facts)
	}
}

// Scenario: Dispute escalation: Buyer and seller list the order's disputes newest first
func TestListOrderDisputesPartiesNewestFirst(t *testing.T) {
	f := newEscFixture(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := f.seed(t, store.Dispute{Status: store.DisputeStatusRejected, CreatedAt: base})
	newer := f.seed(t, store.Dispute{Status: store.DisputeStatusOpen, CreatedAt: base.Add(time.Hour)})
	f.seed(t, store.Dispute{OrderID: "o-2", ClaimantID: "buyer-2", DefendantID: "seller-2", CreatedAt: base})
	for _, who := range []string{"buyer-1", "seller-1"} {
		res, err := f.h.ListOrderDisputes(userCtx(who), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
		if err != nil {
			t.Fatalf("%s: %v", who, err)
		}
		ds := res.GetDisputes()
		if len(ds) != 2 || ds[0].GetId() != newer || ds[1].GetId() != older {
			t.Fatalf("%s: disputes = %v", who, ds)
		}
	}
}

// Scenario: Dispute escalation: Admin lists any order's disputes
func TestListOrderDisputesAdminNoUpstream(t *testing.T) {
	f := newEscFixture(t)
	f.orders.err = errors.New("team-order down")
	f.seed(t, store.Dispute{})
	res, err := f.h.ListOrderDisputes(adminCtx(), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
	if err != nil {
		t.Fatalf("ListOrderDisputes: %v", err)
	}
	if len(res.GetDisputes()) != 1 {
		t.Fatalf("disputes = %v", res.GetDisputes())
	}
	if f.orders.calls != 0 {
		t.Fatalf("team-order must receive 0 calls, got %d", f.orders.calls)
	}
}

// Scenario: Dispute escalation: Non-party is denied
func TestListOrderDisputesNonPartyDenied(t *testing.T) {
	f := newEscFixture(t)
	f.seed(t, store.Dispute{})
	res, err := f.h.ListOrderDisputes(userCtx("stranger"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if len(res.GetDisputes()) != 0 {
		t.Fatalf("no dispute may be returned, got %v", res.GetDisputes())
	}
}

// Scenario: Dispute escalation: Unknown order gives the same answer as a foreign order
func TestListOrderDisputesUnknownOrderDenied(t *testing.T) {
	f := newEscFixture(t)
	_, errUnknown := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-missing"})
	_, errForeign := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-2"})
	if status.Code(errUnknown) != codes.PermissionDenied || status.Code(errForeign) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied for both, got %v / %v", errUnknown, errForeign)
	}
	if status.Convert(errUnknown).Message() != status.Convert(errForeign).Message() {
		t.Fatalf("messages differ: %q vs %q", status.Convert(errUnknown).Message(), status.Convert(errForeign).Message())
	}
}

// Scenario: Dispute escalation: Order without disputes lists empty
func TestListOrderDisputesEmpty(t *testing.T) {
	f := newEscFixture(t)
	res, err := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
	if err != nil {
		t.Fatalf("ListOrderDisputes: %v", err)
	}
	if len(res.GetDisputes()) != 0 {
		t.Fatalf("disputes = %v", res.GetDisputes())
	}
}

// Scenario: Dispute escalation: List needs an order id and a login
func TestListOrderDisputesValidationAndAnonymous(t *testing.T) {
	f := newEscFixture(t)
	if _, err := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty order id: want InvalidArgument, got %v", err)
	}
	for name, ctx := range map[string]context.Context{"anonymous": anonCtx(), "no principal": context.Background()} {
		if _, err := f.h.ListOrderDisputes(ctx, &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: want Unauthenticated, got %v", name, err)
		}
	}
}

// Scenario: Dispute escalation: List without engagement read scope is denied
func TestListOrderDisputesMissingScopeDenied(t *testing.T) {
	f := newEscFixture(t)
	f.seed(t, store.Dispute{})
	ctx := principalCtx("buyer-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, "listing.read")
	res, err := f.h.ListOrderDisputes(ctx, &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if len(res.GetDisputes()) != 0 || f.orders.calls != 0 {
		t.Fatalf("disputes=%v orderCalls=%d", res.GetDisputes(), f.orders.calls)
	}
}

// Scenario: Dispute escalation: Team-order down blocks a non-admin list
func TestListOrderDisputesUpstreamUnavailable(t *testing.T) {
	f := newEscFixture(t)
	f.seed(t, store.Dispute{})
	f.orders.err = errors.New("connection refused")
	res, err := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if len(res.GetDisputes()) != 0 {
		t.Fatalf("no dispute may be returned, got %v", res.GetDisputes())
	}
}

// Scenario: Dispute escalation: Listing writes no audit event
func TestListDisputesNotAudited(t *testing.T) {
	f := newEscFixture(t)
	f.seed(t, store.Dispute{})
	if _, err := f.h.ListDisputes(adminCtx(), &engagementv1.ListDisputesRequest{}); err != nil {
		t.Fatalf("ListDisputes: %v", err)
	}
	if _, err := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"}); err != nil {
		t.Fatalf("ListOrderDisputes: %v", err)
	}
	f.assertNoAuditNoFact(t)
}

// Scenario: Dispute escalation: Admin lists open disputes oldest first
func TestListDisputesAdminFiltersAndOrders(t *testing.T) {
	f := newEscFixture(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := f.seed(t, store.Dispute{Status: store.DisputeStatusOpen, CreatedAt: base.Add(3 * time.Hour)})
	a := f.seed(t, store.Dispute{Status: store.DisputeStatusOpen, CreatedAt: base.Add(1 * time.Hour)})
	b := f.seed(t, store.Dispute{Status: store.DisputeStatusOpen, CreatedAt: base.Add(2 * time.Hour)})
	f.seed(t, store.Dispute{Status: store.DisputeStatusResolved, CreatedAt: base})
	res, err := f.h.ListDisputes(adminCtx(), &engagementv1.ListDisputesRequest{Status: engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN})
	if err != nil {
		t.Fatalf("ListDisputes: %v", err)
	}
	ds := res.GetDisputes()
	if len(ds) != 3 || ds[0].GetId() != a || ds[1].GetId() != b || ds[2].GetId() != c {
		t.Fatalf("disputes = %v", ds)
	}
	page, err := f.h.ListDisputes(adminCtx(), &engagementv1.ListDisputesRequest{
		Status: engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN, Page: &commonv1.PageRequest{PageSize: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetDisputes()) != 2 || page.GetPage().GetNextCursor() == "" {
		t.Fatalf("first page = %v", page)
	}
}

// Scenario: Dispute escalation: Non-admin and anonymous are refused
func TestListDisputesNonAdminDenied(t *testing.T) {
	f := newEscFixture(t)
	f.seed(t, store.Dispute{})
	for _, c := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"buyer", userCtx("buyer-1"), codes.PermissionDenied},
		{"seller", userCtx("seller-1"), codes.PermissionDenied},
		{"anonymous", anonCtx(), codes.Unauthenticated},
	} {
		res, err := f.h.ListDisputes(c.ctx, &engagementv1.ListDisputesRequest{})
		if got := status.Code(err); got != c.want {
			t.Fatalf("%s: want %v, got %v", c.name, c.want, err)
		}
		if len(res.GetDisputes()) != 0 {
			t.Fatalf("%s: no dispute may be returned", c.name)
		}
	}
}
