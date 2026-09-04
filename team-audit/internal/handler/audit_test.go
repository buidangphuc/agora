package handler_test

import (
	"context"
	"testing"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
	"github.com/buidangphuc/team-audit/internal/handler"
	"github.com/buidangphuc/team-audit/internal/repository"
	"github.com/buidangphuc/team-audit/internal/service"
)

func newAuditHandler() *handler.AuditHandler {
	return handler.NewAuditHandler(service.NewAuditService(repository.NewInMemoryAuditRepo()))
}

// The gRPC surface pages newest-first: page 1 hands back a next_cursor that page
// 2 consumes, and the trail is walked from most-recent to oldest without gaps.
func TestQueryAuditLogCursorPaging(t *testing.T) {
	h := newAuditHandler()
	ctx := context.Background()

	for _, a := range []string{"a1", "a2", "a3"} { // a3 is newest
		if _, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{
			ActorId: "seller_1", Action: a, TargetType: "listing", TargetId: a,
		}); err != nil {
			t.Fatalf("write %s: %v", a, err)
		}
	}

	page1, err := h.QueryAuditLog(ctx, &auditv1.QueryAuditLogRequest{
		ActorId: "seller_1",
		Page:    &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if page1.GetPage().GetTotal() != 3 {
		t.Fatalf("expected total 3, got %d", page1.GetPage().GetTotal())
	}
	if len(page1.GetEvents()) != 2 ||
		page1.GetEvents()[0].GetAction() != "a3" || page1.GetEvents()[1].GetAction() != "a2" {
		t.Fatalf("expected newest-first [a3,a2]")
	}
	if page1.GetPage().GetNextCursor() == "" {
		t.Fatalf("expected a next_cursor while a page remains")
	}

	page2, err := h.QueryAuditLog(ctx, &auditv1.QueryAuditLogRequest{
		ActorId: "seller_1",
		Page:    &commonv1.PageRequest{PageSize: 2, Cursor: page1.GetPage().GetNextCursor()},
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.GetEvents()) != 1 || page2.GetEvents()[0].GetAction() != "a1" {
		t.Fatalf("expected [a1] on page2")
	}
	if page2.GetPage().GetNextCursor() != "" {
		t.Fatalf("expected empty next_cursor on last page, got %q", page2.GetPage().GetNextCursor())
	}
}

// WriteAuditEvent is fire-and-forget (empty response) and rejects a missing
// action with InvalidArgument.
func TestWriteAuditEventValidation(t *testing.T) {
	h := newAuditHandler()
	ctx := context.Background()

	if _, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{Action: ""}); err == nil {
		t.Fatalf("expected error on empty action")
	}
	resp, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{Action: "system.ping"})
	if err != nil {
		t.Fatalf("valid write failed: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil (empty) ack")
	}
}
