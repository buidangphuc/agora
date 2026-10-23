package rpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
	"github.com/buidangphuc/team-audit/internal/rpc"
)

func newSeededHandler(t *testing.T) (*rpc.Handler, *faultRepo) {
	t.Helper()
	h, repo := newHandler(t)
	seedEvent(t, repo, "e1", "adm-1", "kyc.review", "order", "o-1", hr(10))
	seedEvent(t, repo, "e2", "adm-1", "kyc.reviewed", "order", "o-1", hr(11))
	seedEvent(t, repo, "e3", "adm-2", "review.hide", "order", "o-2", hr(12))
	seedEvent(t, repo, "e4", "adm-1", "order.force_fail", "order", "o-1", hr(13))
	seedEvent(t, repo, "e5", "adm-1", "kyc.review", "listing", "l-1", hr(14))
	return h, repo
}

func mustQuery(t *testing.T, h *rpc.Handler, req *auditv1.QueryAuditLogRequest, want []string, wantTotal int64) {
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

func allFilters() *auditv1.QueryAuditLogRequest {
	return &auditv1.QueryAuditLogRequest{
		ActorId: "adm-1", TargetType: "order", TargetId: proto.String("o-1"), Action: proto.String("x"),
		CreatedAfter: ts(10), CreatedBefore: ts(12),
	}
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogCursorPaging(t *testing.T) {
	h, repo := newHandler(t)
	seedMany(t, repo, 25, "o-25")

	req := &auditv1.QueryAuditLogRequest{
		TargetType: "order", TargetId: proto.String("o-25"), Page: &commonv1.PageRequest{PageSize: 20},
	}
	p1, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(p1.GetEvents()) != 20 || p1.GetPage().GetTotal() != 25 || p1.GetPage().GetNextCursor() != "20" {
		t.Fatalf("page1: len=%d total=%d cursor=%q", len(p1.GetEvents()), p1.GetPage().GetTotal(), p1.GetPage().GetNextCursor())
	}
	if p1.GetEvents()[0].GetId() != "p-024" || p1.GetEvents()[19].GetId() != "p-005" {
		t.Fatalf("page1 not newest first: %s..%s", p1.GetEvents()[0].GetId(), p1.GetEvents()[19].GetId())
	}
	req.Page.Cursor = p1.GetPage().GetNextCursor()
	p2, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if want := []string{"p-004", "p-003", "p-002", "p-001", "p-000"}; !sameIDs(respIDs(p2), want) {
		t.Fatalf("page2 = %v, want %v", respIDs(p2), want)
	}
	if p2.GetPage().GetTotal() != 25 || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page2 total=%d cursor=%q", p2.GetPage().GetTotal(), p2.GetPage().GetNextCursor())
	}
}

// Scenario: A malformed or negative cursor starts at the first page
func TestMalformedCursorStartsAtFirstPage(t *testing.T) {
	h, repo := newHandler(t)
	seedMany(t, repo, 30, "o-30")
	first, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{Page: &commonv1.PageRequest{PageSize: 10}})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if len(first.GetEvents()) != 10 {
		t.Fatalf("first page len = %d", len(first.GetEvents()))
	}
	for _, cursor := range []string{"zz", "-5", "1.5", "MjA="} {
		res, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{Page: &commonv1.PageRequest{PageSize: 10, Cursor: cursor}})
		if err != nil {
			t.Fatalf("cursor %q: %v", cursor, err)
		}
		if !sameIDs(respIDs(res), respIDs(first)) {
			t.Fatalf("cursor %q: ids = %v, want first page %v", cursor, respIDs(res), respIDs(first))
		}
		if res.GetPage().GetNextCursor() != "10" {
			t.Fatalf("cursor %q: next = %q, want 10", cursor, res.GetPage().GetNextCursor())
		}
	}
}

// Scenario: Page size is clamped to the default and the maximum
func TestPageSizeClamp(t *testing.T) {
	h, repo := newHandler(t)
	seedMany(t, repo, 150, "o-150")
	for size, want := range map[int32]int{0: 20, 100: 100, 101: 100, -3: 20} {
		res, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{Page: &commonv1.PageRequest{PageSize: size}})
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if len(res.GetEvents()) != want {
			t.Fatalf("size %d: got %d events, want %d", size, len(res.GetEvents()), want)
		}
		if res.GetPage().GetTotal() != 150 {
			t.Fatalf("size %d: total = %d, want 150", size, res.GetPage().GetTotal())
		}
	}
}

// Scenario: An empty action is invalid
func TestWriteAuditEventValidation(t *testing.T) {
	h, repo := newHandler(t)
	ctx := asService("svc-order")
	if _, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{Action: ""}); code(err) != codes.InvalidArgument {
		t.Fatalf("empty action: got %v, want InvalidArgument", code(err))
	}
	if got := storedActors(t, h); len(got) != 0 {
		t.Fatalf("nothing may be stored, got %v", got)
	}
	resp, err := h.WriteAuditEvent(ctx, &auditv1.WriteAuditEventRequest{Action: "system.ping"})
	if err != nil || resp == nil {
		t.Fatalf("valid write: %v %v", resp, err)
	}
	if repo.queries == 0 {
		t.Fatalf("expected the verification query to reach the repository")
	}
}

// Scenario: An anonymous write is refused
func TestWriteAuditEventRequiresAuthentication(t *testing.T) {
	h, _ := newHandler(t)
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

// Scenario: A user, an admin or a scopeless service cannot write
func TestWriteAuditEventRejectsNonServiceCallers(t *testing.T) {
	h, _ := newHandler(t)
	noScope := principalCtx(auth.Principal{ID: "svc-x", Type: auth.TypeService, Scopes: []string{"promotion.reserve"}})
	userWithScope := principalCtx(auth.Principal{ID: "u1", Type: auth.TypeUser, Scopes: []string{"audit.write"}})
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

// Scenario: A storage failure is generic and not leaked
func TestInternalErrorIsGenericAndNotLeaked(t *testing.T) {
	h, repo := newHandler(t)
	repo.appendErr = errors.New("pq: password=secret authentication failed for user audit")
	_, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{Action: "x"})
	if code(err) != codes.Internal {
		t.Fatalf("write: got %v, want Internal", code(err))
	}
	repo.queryErr = errors.New("pq: password=secret connection refused")
	_, qerr := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{})
	if code(qerr) != codes.Internal {
		t.Fatalf("query: got %v, want Internal", code(qerr))
	}
	for _, e := range []error{err, qerr} {
		if msg := status.Convert(e).Message(); msg != "internal error" || strings.Contains(msg, "secret") {
			t.Fatalf("message %q leaks the cause", msg)
		}
	}
}

// Scenario: A service with audit.write writes on behalf of a user
func TestWriteAuditEventServicePrincipalMayActOnBehalf(t *testing.T) {
	h, _ := newHandler(t)
	if _, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{ActorId: "buyer-3", Action: "order.paid"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := h.WriteAuditEvent(asService("svc-order"), &auditv1.WriteAuditEventRequest{Action: "system.tick"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := storedActors(t, h)
	if len(got) != 2 || got[0] != "svc-order" || got[1] != "buyer-3" {
		t.Fatalf("stored actors = %v, want [svc-order buyer-3]", got)
	}
}

// Scenario: A non-admin cannot query
func TestQueryAuditLogAdminOnly(t *testing.T) {
	h, _ := newHandler(t)
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

// Scenario: A non-admin cannot query; An anonymous write is refused
func TestAuthorizationBoundary(t *testing.T) {
	cases := []struct {
		name       string
		ctx        context.Context
		write, qry codes.Code
	}{
		{"no principal", context.Background(), codes.Unauthenticated, codes.Unauthenticated},
		{"anonymous", asAnonymous(), codes.Unauthenticated, codes.Unauthenticated},
		{"plain user", asUser("u1"), codes.PermissionDenied, codes.PermissionDenied},
		{"admin user", asAdmin(), codes.PermissionDenied, codes.OK},
		{"service with audit.write", asService("svc-a"), codes.OK, codes.PermissionDenied},
		{"service without scope", principalCtx(auth.Principal{ID: "svc-b", Type: auth.TypeService}), codes.PermissionDenied, codes.PermissionDenied},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, _ := newHandler(t)
			if _, err := h.WriteAuditEvent(c.ctx, &auditv1.WriteAuditEventRequest{Action: "x"}); code(err) != c.write {
				t.Fatalf("write: got %v, want %v", code(err), c.write)
			}
			if _, err := h.QueryAuditLog(c.ctx, &auditv1.QueryAuditLogRequest{}); code(err) != c.qry {
				t.Fatalf("query: got %v, want %v", code(err), c.qry)
			}
		})
	}
}

// Scenario: Authorization precedes validation
func TestQueryAuditLogAuthBeforeValidation(t *testing.T) {
	h, repo := newHandler(t)
	_, err := h.QueryAuditLog(asUser("u1"), &auditv1.QueryAuditLogRequest{CreatedAfter: ts(12), CreatedBefore: ts(10)})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied", code(err))
	}
	if repo.queries != 0 {
		t.Fatalf("repository must not be called, got %d", repo.queries)
	}
	_, err = h.WriteAuditEvent(asUser("u1"), &auditv1.WriteAuditEventRequest{Action: ""})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("write with empty action by user: got %v, want PermissionDenied", code(err))
	}
}

// Scenario: An anonymous query is refused
func TestQueryAuditLogUnauthenticated(t *testing.T) {
	for name, ctx := range map[string]context.Context{"no principal": context.Background(), "anonymous": asAnonymous()} {
		h, repo := newHandler(t)
		_, err := h.QueryAuditLog(ctx, allFilters())
		if code(err) != codes.Unauthenticated {
			t.Fatalf("%s: got %v, want Unauthenticated", name, code(err))
		}
		if repo.queries != 0 {
			t.Fatalf("%s: repository must not be called", name)
		}
	}
}

// Scenario: A non-admin cannot query
func TestQueryAuditLogAdminOnlyWithFilters(t *testing.T) {
	for name, ctx := range map[string]context.Context{"plain user": asUser("u1"), "service": asService("svc-order")} {
		h, repo := newHandler(t)
		_, err := h.QueryAuditLog(ctx, allFilters())
		if code(err) != codes.PermissionDenied {
			t.Fatalf("%s: got %v, want PermissionDenied", name, code(err))
		}
		if repo.queries != 0 {
			t.Fatalf("%s: repository must not be called", name)
		}
	}
}

// Scenario: An inverted time range is invalid
func TestQueryAuditLogInvertedRangeInvalidArgument(t *testing.T) {
	h, repo := newHandler(t)
	_, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{CreatedAfter: ts(12), CreatedBefore: ts(10)})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	if repo.queries != 0 {
		t.Fatalf("repository must not be called, got %d queries", repo.queries)
	}
	if _, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{CreatedAfter: ts(12), CreatedBefore: ts(12)}); err != nil {
		t.Fatalf("equal bounds must succeed, got %v", err)
	}
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogFilterByTarget(t *testing.T) {
	h, _ := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{TargetType: "order", TargetId: proto.String("o-1")},
		[]string{"e4", "e2", "e1"}, 3)
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogFilterByActionExact(t *testing.T) {
	h, _ := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{Action: proto.String("kyc.review")},
		[]string{"e5", "e1"}, 2)
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogFiltersAreAnded(t *testing.T) {
	h, _ := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{
		ActorId: "adm-1", Action: proto.String("order.force_fail"), TargetId: proto.String("o-1"),
	}, []string{"e4"}, 1)
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogEmptyOptionalIsNoFilterAndTimeRange(t *testing.T) {
	h, _ := newSeededHandler(t)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{TargetId: proto.String(""), Action: proto.String("")},
		[]string{"e5", "e4", "e3", "e2", "e1"}, 5)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{CreatedAfter: ts(10), CreatedBefore: ts(12)},
		[]string{"e2", "e1"}, 2)
	mustQuery(t, h, &auditv1.QueryAuditLogRequest{CreatedAfter: ts(13)},
		[]string{"e5", "e4"}, 2)
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogNoMatchIsEmpty(t *testing.T) {
	h, _ := newSeededHandler(t)
	res, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{TargetId: proto.String("nobody")})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.GetEvents()) != 0 || res.GetPage().GetTotal() != 0 || res.GetPage().GetNextCursor() != "" {
		t.Fatalf("want empty page, got %v total=%d cursor=%q",
			respIDs(res), res.GetPage().GetTotal(), res.GetPage().GetNextCursor())
	}
}

// Scenario: The cursor is a decimal offset
func TestQueryAuditLogFilteredPaging(t *testing.T) {
	h, repo := newHandler(t)
	seedMany(t, repo, 25, "o-25")
	seedEvent(t, repo, "other", "adm-1", "a.b", "order", "o-other", hr(5))
	req := &auditv1.QueryAuditLogRequest{TargetType: "order", TargetId: proto.String("o-25"), Page: &commonv1.PageRequest{PageSize: 20}}
	p1, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil || p1.GetPage().GetTotal() != 25 || p1.GetPage().GetNextCursor() != "20" {
		t.Fatalf("page1: %v err=%v", p1, err)
	}
	req.Page.Cursor = p1.GetPage().GetNextCursor()
	p2, err := h.QueryAuditLog(asAdmin(), req)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(p2.GetEvents()) != 5 || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page2 len=%d cursor=%q", len(p2.GetEvents()), p2.GetPage().GetNextCursor())
	}
}
