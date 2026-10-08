package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
	"github.com/buidangphuc/team-audit/internal/handler"
	"github.com/buidangphuc/team-audit/internal/interceptor"
	"github.com/buidangphuc/team-audit/internal/repository"
	"github.com/buidangphuc/team-audit/internal/service"
)

func asUser(id string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), interceptor.Principal{ID: id, Type: "user", Scopes: []string{"listing.read"}})
}

func asAdmin() context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), interceptor.Principal{ID: "admin-1", Type: "user", Scopes: []string{"admin"}})
}

func asService(id string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), interceptor.Principal{ID: id, Type: "service", Scopes: []string{"audit.write"}})
}

func asAnonymous() context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), interceptor.Principal{ID: "anonymous", Type: "anonymous"})
}

func newAuditHandler() *handler.AuditHandler {
	return handler.NewAuditHandler(service.NewAuditService(repository.NewInMemoryAuditRepo()))
}

// The gRPC surface pages newest-first: page 1 hands back a next_cursor that page
// 2 consumes, and the trail is walked from most-recent to oldest without gaps.
func TestQueryAuditLogCursorPaging(t *testing.T) {
	h := newAuditHandler()
	ctx := asAdmin()
	svcCtx := asService("svc-order")

	for _, a := range []string{"a1", "a2", "a3"} { // a3 is newest
		if _, err := h.WriteAuditEvent(svcCtx, &auditv1.WriteAuditEventRequest{
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
	ctx := asUser("u1")

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

func code(err error) codes.Code { return status.Code(err) }

func storedActors(t *testing.T, h *handler.AuditHandler) []string {
	t.Helper()
	res, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{})
	if err != nil {
		t.Fatalf("admin query: %v", err)
	}
	var out []string
	for _, e := range res.GetEvents() {
		out = append(out, e.GetActorId())
	}
	return out
}

func TestWriteAuditEventRequiresAuthentication(t *testing.T) {
	h := newAuditHandler()
	for name, ctx := range map[string]context.Context{"no principal": context.Background(), "anonymous": asAnonymous()} {
		_, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{Action: "x"})
		if code(err) != codes.Unauthenticated {
			t.Fatalf("%s: got %v, want Unauthenticated", name, code(err))
		}
	}
	if got := storedActors(t, h); len(got) != 0 {
		t.Fatalf("rejected writes must not be stored, got %v", got)
	}
}

func TestWriteAuditEventUserActorIsPrincipalNotClientSupplied(t *testing.T) {
	h := newAuditHandler()
	if _, err := h.WriteAuditEvent(asUser("seller-7"), &auditv1.WriteAuditEventRequest{ActorId: "victim-9", Action: "forged"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := storedActors(t, h)
	if len(got) != 1 || got[0] != "seller-7" {
		t.Fatalf("stored actors = %v, want [seller-7]", got)
	}
}

func TestWriteAuditEventServicePrincipalMayActOnBehalf(t *testing.T) {
	h := newAuditHandler()
	if _, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{ActorId: "buyer-3", Action: "order.paid"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{Action: "system.tick"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := storedActors(t, h) // newest first
	if len(got) != 2 || got[0] != "svc-order" || got[1] != "buyer-3" {
		t.Fatalf("stored actors = %v, want [svc-order buyer-3]", got)
	}
}

func TestQueryAuditLogAdminOnly(t *testing.T) {
	h := newAuditHandler()
	if _, err := h.WriteAuditEvent(asUser("u1"), &auditv1.WriteAuditEventRequest{Action: "x"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"no principal", context.Background(), codes.Unauthenticated},
		{"anonymous", asAnonymous(), codes.Unauthenticated},
		{"plain user", asUser("u1"), codes.PermissionDenied},
		{"service without admin", asService("svc-order"), codes.PermissionDenied},
		{"admin", asAdmin(), codes.OK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.QueryAuditLog(c.ctx, &auditv1.QueryAuditLogRequest{})
			if code(err) != c.want {
				t.Fatalf("got %v, want %v", code(err), c.want)
			}
		})
	}
}

func TestInternalErrorIsGenericAndNotLeaked(t *testing.T) {
	err := handler.MapAuditErrForTest(errors.New("pq: password authentication failed for user audit"))
	if code(err) != codes.Internal {
		t.Fatalf("got %v, want Internal", code(err))
	}
	if msg := status.Convert(err).Message(); msg != "internal error" {
		t.Fatalf("message %q leaks the cause", msg)
	}
}
