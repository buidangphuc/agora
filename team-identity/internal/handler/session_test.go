package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
)

func principalContext(userID string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id:     userID,
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"identity:read", "identity:write"},
	})
}

func TestSessionHandler_ListAndRevoke(t *testing.T) {
	repo := repository.NewInMemorySessionRepository()
	h := handler.NewSessionHandler(repo, nil)

	// Seed two sessions for the caller.
	s1, _ := repo.CreateSession(context.Background(), repository.Session{UserID: "user-1", Device: "iPhone", IP: "1.1.1.1"})
	_, _ = repo.CreateSession(context.Background(), repository.Session{UserID: "user-1", Device: "MacBook", IP: "1.1.1.2"})

	ctx := principalContext("user-1")

	listRes, err := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(listRes.GetSessions()) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(listRes.GetSessions()))
	}

	// Revoke one and confirm it flips to revoked.
	if _, err := h.RevokeSession(ctx, &identityv1.RevokeSessionRequest{SessionId: s1.ID}); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	after, _ := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
	var revoked bool
	for _, s := range after.GetSessions() {
		if s.GetId() == s1.ID {
			revoked = s.GetRevoked()
		}
	}
	if !revoked {
		t.Fatal("expected session to be revoked")
	}
}

func TestSessionHandler_RevokeCrossUserIsolation(t *testing.T) {
	repo := repository.NewInMemorySessionRepository()
	h := handler.NewSessionHandler(repo, nil)

	// A session that belongs to user-2.
	victim, _ := repo.CreateSession(context.Background(), repository.Session{UserID: "user-2", Device: "iPad", IP: "2.2.2.2"})

	// user-1 must not be able to revoke it, nor see it.
	ctx := principalContext("user-1")

	if _, err := h.RevokeSession(ctx, &identityv1.RevokeSessionRequest{SessionId: victim.ID}); status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound revoking another user's session, got %v", err)
	}

	list, _ := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
	if len(list.GetSessions()) != 0 {
		t.Fatalf("user-1 should see 0 sessions, got %d", len(list.GetSessions()))
	}

	// Confirm the victim's session is untouched.
	victimSessions, _ := repo.ListSessions(context.Background(), "user-2")
	if len(victimSessions) != 1 || victimSessions[0].Revoked {
		t.Fatalf("victim session should be intact and not revoked, got %+v", victimSessions)
	}
}

func TestSessionHandler_ListLoginHistoryPaginationAndScope(t *testing.T) {
	repo := repository.NewInMemorySessionRepository()
	h := handler.NewSessionHandler(repo, nil)

	// 3 events for user-1, 1 for user-2.
	for i := 0; i < 3; i++ {
		if _, err := repo.RecordLogin(context.Background(), repository.LoginEvent{UserID: "user-1", IP: "1.1.1.1", UserAgent: "chrome", Success: i%2 == 0}); err != nil {
			t.Fatalf("RecordLogin: %v", err)
		}
	}
	_, _ = repo.RecordLogin(context.Background(), repository.LoginEvent{UserID: "user-2", IP: "2.2.2.2", UserAgent: "firefox", Success: true})

	ctx := principalContext("user-1")

	// First page of size 2.
	page1, err := h.ListLoginHistory(ctx, &identityv1.ListLoginHistoryRequest{
		Page: &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatalf("ListLoginHistory page1: %v", err)
	}
	if len(page1.GetEvents()) != 2 {
		t.Fatalf("want 2 events on page1, got %d", len(page1.GetEvents()))
	}
	if page1.GetPage().GetTotal() != 3 {
		t.Fatalf("want total 3, got %d", page1.GetPage().GetTotal())
	}
	if page1.GetPage().GetNextCursor() == "" {
		t.Fatal("expected a next cursor for page1")
	}

	// Second page via cursor: 1 remaining event, no further cursor.
	page2, err := h.ListLoginHistory(ctx, &identityv1.ListLoginHistoryRequest{
		Page: &commonv1.PageRequest{PageSize: 2, Cursor: page1.GetPage().GetNextCursor()},
	})
	if err != nil {
		t.Fatalf("ListLoginHistory page2: %v", err)
	}
	if len(page2.GetEvents()) != 1 {
		t.Fatalf("want 1 event on page2, got %d", len(page2.GetEvents()))
	}
	if page2.GetPage().GetNextCursor() != "" {
		t.Fatalf("expected no next cursor on last page, got %q", page2.GetPage().GetNextCursor())
	}
}

func TestSessionHandler_AnonymousAndBadInput(t *testing.T) {
	repo := repository.NewInMemorySessionRepository()
	h := handler.NewSessionHandler(repo, nil)

	// No principal on the context => Unauthenticated, no panic.
	if _, err := h.ListSessions(context.Background(), &identityv1.ListSessionsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
	if _, err := h.ListLoginHistory(context.Background(), &identityv1.ListLoginHistoryRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}

	// Authenticated but empty session_id => InvalidArgument.
	ctx := principalContext("user-1")
	if _, err := h.RevokeSession(ctx, &identityv1.RevokeSessionRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument for empty session_id, got %v", err)
	}

	// Nil page and garbage cursor must not panic; returns empty history.
	res, err := h.ListLoginHistory(ctx, &identityv1.ListLoginHistoryRequest{
		Page: &commonv1.PageRequest{Cursor: "not-a-number"},
	})
	if err != nil {
		t.Fatalf("ListLoginHistory garbage cursor: %v", err)
	}
	if len(res.GetEvents()) != 0 {
		t.Fatalf("want 0 events, got %d", len(res.GetEvents()))
	}
}
