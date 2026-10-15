package handler_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	ctx := asService("svc-order")

	if _, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{Action: ""}); code(err) != codes.InvalidArgument {
		t.Fatalf("empty action: got %v, want InvalidArgument", code(err))
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

func TestWriteAuditEventRejectsNonServiceCallers(t *testing.T) {
	h := newAuditHandler()
	noScope := interceptor.ContextWithPrincipal(context.Background(), interceptor.Principal{ID: "svc-x", Type: "service", Scopes: []string{"promotion.reserve"}})
	userWithScope := interceptor.ContextWithPrincipal(context.Background(), interceptor.Principal{ID: "u1", Type: "user", Scopes: []string{"audit.write"}})
	for name, ctx := range map[string]context.Context{
		"buyer/seller user":      asUser("seller-7"),
		"admin user":             asAdmin(),
		"user holding the scope": userWithScope,
		"service without scope":  noScope,
	} {
		_, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{ActorId: "victim-9", Action: "forged"})
		if code(err) != codes.PermissionDenied {
			t.Fatalf("%s: got %v, want PermissionDenied", name, code(err))
		}
	}
	if got := storedActors(t, h); len(got) != 0 {
		t.Fatalf("rejected writes must not be stored, got %v", got)
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
	if _, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{Action: "x"}); err != nil {
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

// spyRepo counts Query calls so "the repository is not called" is observable.
type spyRepo struct {
	repository.AuditRepository
	queries int
}

func (s *spyRepo) Query(ctx context.Context, f repository.QueryFilter) ([]*auditv1.AuditEvent, int64, error) {
	s.queries++
	return s.AuditRepository.Query(ctx, f)
}

func newSpiedHandler() (*handler.AuditHandler, *spyRepo) {
	spy := &spyRepo{AuditRepository: repository.NewInMemoryAuditRepo()}
	return handler.NewAuditHandler(service.NewAuditService(spy)), spy
}

func hr(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

func ts(h int) *timestamppb.Timestamp { return timestamppb.New(hr(h)) }

// seedEvent appends an event with a fixed timestamp straight into the repo.
func seedEvent(t *testing.T, repo repository.AuditRepository, id, actor, action, tType, tID string, at time.Time) {
	t.Helper()
	if _, err := repo.Append(context.Background(), &auditv1.AuditEvent{
		Id: id, ActorId: actor, Action: action, TargetType: tType, TargetId: tID, CreatedAt: timestamppb.New(at),
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func newSeededHandler(t *testing.T) *handler.AuditHandler {
	t.Helper()
	repo := repository.NewInMemoryAuditRepo()
	seedEvent(t, repo, "e1", "adm-1", "kyc.review", "order", "o-1", hr(10))
	seedEvent(t, repo, "e2", "adm-1", "kyc.reviewed", "order", "o-1", hr(11))
	seedEvent(t, repo, "e3", "adm-2", "review.hide", "order", "o-2", hr(12))
	seedEvent(t, repo, "e4", "adm-1", "order.force_fail", "order", "o-1", hr(13))
	seedEvent(t, repo, "e5", "adm-1", "kyc.review", "listing", "l-1", hr(14))
	return handler.NewAuditHandler(service.NewAuditService(repo))
}

func respIDs(r *auditv1.QueryAuditLogResponse) []string {
	out := []string{}
	for _, e := range r.GetEvents() {
		out = append(out, e.GetId())
	}
	return out
}

func sameIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func mustQuery(t *testing.T, h *handler.AuditHandler, req *auditv1.QueryAuditLogRequest, want []string, wantTotal int64) {
	t.Helper()
	res, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !sameIDs(respIDs(res), want) {
		t.Fatalf("ids = %v, want %v", respIDs(res), want)
	}
	if res.GetPage().GetTotal() != wantTotal {
		t.Fatalf("total = %d, want %d", res.GetPage().GetTotal(), wantTotal)
	}
}

// Scenario: Target id and target type narrow to one target.
func TestQueryAuditLogFilterByTarget(t *testing.T) {
	h := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{TargetType: "order", TargetId: proto.String("o-1")},
		[]string{"e4", "e2", "e1"}, 3)
}

// Scenario: Action filter is an exact match.
func TestQueryAuditLogFilterByActionExact(t *testing.T) {
	h := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{Action: proto.String("kyc.review")},
		[]string{"e5", "e1"}, 2)
}

// Scenario: Filters combine with AND.
func TestQueryAuditLogFiltersAreAnded(t *testing.T) {
	h := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{
		ActorId: "adm-1", Action: proto.String("order.force_fail"), TargetId: proto.String("o-1"),
	}, []string{"e4"}, 1)
}

// An explicitly empty optional string means no filter (spec: empty = unset),
// and a time range is honoured end to end (inclusive lower, exclusive upper).
func TestQueryAuditLogEmptyOptionalIsNoFilterAndTimeRange(t *testing.T) {
	h := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{TargetId: proto.String(""), Action: proto.String("")},
		[]string{"e5", "e4", "e3", "e2", "e1"}, 5)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{CreatedAfter: ts(10), CreatedBefore: ts(12)},
		[]string{"e2", "e1"}, 2)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{CreatedAfter: ts(13)},
		[]string{"e5", "e4"}, 2)
}

// Scenario: No match returns an empty page.
func TestQueryAuditLogNoMatchIsEmpty(t *testing.T) {
	h := newSeededHandler(t)
	res, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{TargetId: proto.String("nobody")})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.GetEvents()) != 0 || res.GetPage().GetTotal() != 0 || res.GetPage().GetNextCursor() != "" {
		t.Fatalf("want empty page, got %v total=%d cursor=%q",
			respIDs(res), res.GetPage().GetTotal(), res.GetPage().GetNextCursor())
	}
}

// Scenario: Paging walks a filtered result.
func TestQueryAuditLogFilteredPaging(t *testing.T) {
	repo := repository.NewInMemoryAuditRepo()
	for i := 0; i < 25; i++ {
		seedEvent(t, repo, fmt.Sprintf("p-%02d", i), "adm-1", "a.b", "order", "o-25", hr(0).Add(time.Duration(i)*time.Minute))
	}
	seedEvent(t, repo, "other", "adm-1", "a.b", "order", "o-other", hr(5))
	h := handler.NewAuditHandler(service.NewAuditService(repo))

	req := &auditv1.QueryAuditLogRequest{TargetType: "order", TargetId: proto.String("o-25"), Page: &commonv1.PageRequest{PageSize: 20}}
	p1, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(p1.GetEvents()) != 20 || p1.GetPage().GetTotal() != 25 || p1.GetPage().GetNextCursor() == "" {
		t.Fatalf("page1: len=%d total=%d cursor=%q", len(p1.GetEvents()), p1.GetPage().GetTotal(), p1.GetPage().GetNextCursor())
	}
	if p1.GetEvents()[0].GetId() != "p-24" || p1.GetEvents()[19].GetId() != "p-05" {
		t.Fatalf("page1 not newest first: %s..%s", p1.GetEvents()[0].GetId(), p1.GetEvents()[19].GetId())
	}
	req.Page.Cursor = p1.GetPage().GetNextCursor()
	p2, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if want := []string{"p-04", "p-03", "p-02", "p-01", "p-00"}; !sameIDs(respIDs(p2), want) {
		t.Fatalf("page2 = %v, want %v", respIDs(p2), want)
	}
	if p2.GetPage().GetTotal() != 25 || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page2 total=%d cursor=%q", p2.GetPage().GetTotal(), p2.GetPage().GetNextCursor())
	}
}

// Scenario: created_before earlier than created_after.
func TestQueryAuditLogInvertedRangeInvalidArgument(t *testing.T) {
	h, spy := newSpiedHandler()
	_, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{CreatedAfter: ts(12), CreatedBefore: ts(10)})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	if spy.queries != 0 {
		t.Fatalf("repository must not be called, got %d queries", spy.queries)
	}
	// Equal bounds are legal.
	if _, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{CreatedAfter: ts(12), CreatedBefore: ts(12)}); err != nil {
		t.Fatalf("equal bounds must succeed, got %v", err)
	}
}

// Scenario: Authorization precedes validation.
func TestQueryAuditLogAuthBeforeValidation(t *testing.T) {
	h, spy := newSpiedHandler()
	_, err := h.QueryAuditLog(asUser("u1"), &auditv1.QueryAuditLogRequest{CreatedAfter: ts(12), CreatedBefore: ts(10)})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied", code(err))
	}
	if spy.queries != 0 {
		t.Fatalf("repository must not be called, got %d", spy.queries)
	}
}

func allFilters() *auditv1.QueryAuditLogRequest {
	return &auditv1.QueryAuditLogRequest{
		ActorId: "adm-1", TargetType: "order", TargetId: proto.String("o-1"), Action: proto.String("x"),
		CreatedAfter: ts(10), CreatedBefore: ts(12),
	}
}

// Scenario: Handler refuses a missing principal with any filter.
func TestQueryAuditLogUnauthenticated(t *testing.T) {
	for name, ctx := range map[string]context.Context{"no principal": context.Background(), "anonymous": asAnonymous()} {
		h, spy := newSpiedHandler()
		_, err := h.QueryAuditLog(ctx, allFilters())
		if code(err) != codes.Unauthenticated {
			t.Fatalf("%s: got %v, want Unauthenticated", name, code(err))
		}
		if spy.queries != 0 {
			t.Fatalf("%s: repository must not be called", name)
		}
	}
}

// Scenario: Handler denies a non-admin before reading filters.
func TestQueryAuditLogAdminOnlyWithFilters(t *testing.T) {
	for name, ctx := range map[string]context.Context{"plain user": asUser("u1"), "service": asService("svc-order")} {
		h, spy := newSpiedHandler()
		_, err := h.QueryAuditLog(ctx, allFilters())
		if code(err) != codes.PermissionDenied {
			t.Fatalf("%s: got %v, want PermissionDenied", name, code(err))
		}
		if spy.queries != 0 {
			t.Fatalf("%s: repository must not be called", name)
		}
	}
}
