package service_test

import (
	"context"
	"testing"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/repository"
	"github.com/buidangphuc/team-audit/internal/service"
)

func newAuditService() *service.AuditService {
	return service.NewAuditService(repository.NewInMemoryAuditRepo())
}

// Write then Query returns the event, with a server-assigned id + created_at.
func TestWriteQueryRoundtrip(t *testing.T) {
	svc := newAuditService()
	ctx := context.Background()

	ev, err := svc.Write(ctx, "seller_1", "listing.update", "listing", "lst_9", map[string]string{"price": "100"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if ev.GetId() == "" {
		t.Fatalf("expected server-assigned id")
	}
	if ev.GetCreatedAt() == nil {
		t.Fatalf("expected server-assigned created_at")
	}

	events, total, err := svc.Query(ctx, "", "", 20, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || len(events) != 1 {
		t.Fatalf("expected 1 event, got total=%d len=%d", total, len(events))
	}
	got := events[0]
	if got.GetActorId() != "seller_1" || got.GetAction() != "listing.update" ||
		got.GetTargetType() != "listing" || got.GetTargetId() != "lst_9" {
		t.Fatalf("roundtrip field mismatch: %+v", got)
	}
	if got.GetMetadata()["price"] != "100" {
		t.Fatalf("metadata not roundtripped: %v", got.GetMetadata())
	}
}

// Query filtered by actor returns only that actor's events, and by target type
// returns only that target's — the two filters an audit query supports.
func TestQueryFilterByActorAndTarget(t *testing.T) {
	svc := newAuditService()
	ctx := context.Background()

	mustWrite(t, svc, "seller_1", "listing.update", "listing", "lst_1")
	mustWrite(t, svc, "seller_2", "payout.request", "payout", "po_1")
	mustWrite(t, svc, "seller_1", "order.ship", "order", "ord_1")

	byActor, total, err := svc.Query(ctx, "seller_1", "", 20, 0)
	if err != nil {
		t.Fatalf("query by actor: %v", err)
	}
	if total != 2 || len(byActor) != 2 {
		t.Fatalf("expected 2 events for seller_1, got total=%d len=%d", total, len(byActor))
	}
	for _, ev := range byActor {
		if ev.GetActorId() != "seller_1" {
			t.Fatalf("actor filter leaked: %v", ev.GetActorId())
		}
	}

	byTarget, total, err := svc.Query(ctx, "", "payout", 20, 0)
	if err != nil {
		t.Fatalf("query by target: %v", err)
	}
	if total != 1 || len(byTarget) != 1 || byTarget[0].GetTargetType() != "payout" {
		t.Fatalf("expected 1 payout event, got total=%d len=%d", total, len(byTarget))
	}
}

// Paging returns events newest-first and the offset walks back through history
// without overlap or gaps.
func TestQueryPaginationNewestFirst(t *testing.T) {
	svc := newAuditService()
	ctx := context.Background()

	mustWrite(t, svc, "seller_1", "a1", "listing", "lst_1") // oldest
	mustWrite(t, svc, "seller_1", "a2", "listing", "lst_2")
	mustWrite(t, svc, "seller_1", "a3", "listing", "lst_3") // newest

	page1, total, err := svc.Query(ctx, "seller_1", "", 2, 0)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(page1) != 2 || page1[0].GetAction() != "a3" || page1[1].GetAction() != "a2" {
		t.Fatalf("expected newest-first [a3,a2], got %v", actions(page1))
	}

	page2, _, err := svc.Query(ctx, "seller_1", "", 2, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 || page2[0].GetAction() != "a1" {
		t.Fatalf("expected [a1] on page2, got %v", actions(page2))
	}
}

// A write with no action is rejected; actor_id may be empty (anonymous/system).
func TestWriteValidation(t *testing.T) {
	svc := newAuditService()
	ctx := context.Background()

	if _, err := svc.Write(ctx, "seller_1", "", "listing", "lst_1", nil); err != service.ErrEmptyAction {
		t.Fatalf("expected ErrEmptyAction, got %v", err)
	}
	// Empty actor is allowed (anonymous / system-originated events).
	if _, err := svc.Write(ctx, "", "system.startup", "", "", nil); err != nil {
		t.Fatalf("anonymous write should succeed, got %v", err)
	}
}

func mustWrite(t *testing.T, svc *service.AuditService, actor, action, tType, tID string) {
	t.Helper()
	if _, err := svc.Write(context.Background(), actor, action, tType, tID, nil); err != nil {
		t.Fatalf("write %s/%s: %v", actor, action, err)
	}
}

func actions(evs []*auditv1.AuditEvent) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.GetAction()
	}
	return out
}
