package rpc_test

// Task 1.7 (identity-user-registered): AuthService.GetMyAccount.
//
// API assumed by these tests: (*AuthHandler).GetMyAccount(ctx, *identityv1.GetMyAccountRequest)
// (*identityv1.GetMyAccountResponse, error); the caller is the verified Principal of ctx
// (auth.RequireUser): no Principal or anonymous -> UNAUTHENTICATED, service principal ->
// PERMISSION_DENIED, missing user row -> NOT_FOUND; the response is {user_id, created_at} only.

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

type accountEnv struct {
	h     *rpc.AuthHandler
	users *storetest.InMemoryUserRepository
}

func newAccountEnv(t *testing.T) accountEnv {
	t.Helper()
	users := storetest.NewInMemoryUserRepository()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	return accountEnv{h: rpc.NewAuthHandler(svc), users: users}
}

func (e accountEnv) seed(t *testing.T, id string, roles []string, created time.Time) {
	t.Helper()
	if _, err := e.users.Create(context.Background(), store.User{ID: id, Username: id, PasswordHash: "x", Roles: roles, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
}

var acctCreated = time.Date(2026, 11, 6, 7, 0, 0, 0, time.UTC)

// Scenario: A signed-in user reads their own account
func TestGetMyAccountReturnsOwnAccount(t *testing.T) {
	e := newAccountEnv(t)
	e.seed(t, "buyer-1", []string{"buyer"}, acctCreated)
	resp, err := e.h.GetMyAccount(withPrincipal(context.Background(), userPrincipal("buyer-1")), &identityv1.GetMyAccountRequest{})
	if err != nil {
		t.Fatalf("GetMyAccount: %v", err)
	}
	if resp.GetUserId() != "buyer-1" {
		t.Fatalf("user_id = %q", resp.GetUserId())
	}
	if !resp.GetCreatedAt().AsTime().Equal(acctCreated) {
		t.Fatalf("created_at = %v, want %v", resp.GetCreatedAt().AsTime(), acctCreated)
	}
}

// Scenario: A seller and a buyer are both allowed
func TestGetMyAccountAllowsSellerAndBuyer(t *testing.T) {
	e := newAccountEnv(t)
	e.seed(t, "buyer-1", []string{"buyer"}, acctCreated)
	e.seed(t, "seller-1", []string{"seller"}, acctCreated.Add(time.Hour))
	for _, id := range []string{"buyer-1", "seller-1"} {
		resp, err := e.h.GetMyAccount(withPrincipal(context.Background(), userPrincipal(id)), &identityv1.GetMyAccountRequest{})
		if err != nil {
			t.Fatalf("GetMyAccount(%s): %v", id, err)
		}
		if resp.GetUserId() != id {
			t.Fatalf("user_id = %q, want %q", resp.GetUserId(), id)
		}
	}
}

// Scenario: An anonymous call is refused
func TestGetMyAccountAnonymousIsUnauthenticated(t *testing.T) {
	e := newAccountEnv(t)
	e.seed(t, "buyer-1", []string{"buyer"}, acctCreated)
	cases := map[string]context.Context{
		"no principal":        context.Background(),
		"anonymous principal": auth.WithPrincipal(context.Background(), auth.Principal{Type: auth.TypeAnonymous}),
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := e.h.GetMyAccount(ctx, &identityv1.GetMyAccountRequest{})
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("code = %v (%v), want Unauthenticated", status.Code(err), err)
			}
			if resp.GetUserId() != "" || resp.GetCreatedAt() != nil {
				t.Fatalf("a refused call must return no account data: %+v", resp)
			}
		})
	}
}

// Scenario: An anonymous call is refused (a service principal is not a user)
func TestGetMyAccountServicePrincipalDenied(t *testing.T) {
	e := newAccountEnv(t)
	e.seed(t, "buyer-1", []string{"buyer"}, acctCreated)
	ctx := withPrincipal(context.Background(), &commonv1.Principal{Id: "buyer-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE})
	resp, err := e.h.GetMyAccount(ctx, &identityv1.GetMyAccountRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v (%v), want PermissionDenied", status.Code(err), err)
	}
	if resp.GetUserId() != "" {
		t.Fatalf("a refused call must return no account data: %+v", resp)
	}
}

// Scenario: A principal whose account no longer exists
func TestGetMyAccountMissingRowIsNotFound(t *testing.T) {
	e := newAccountEnv(t)
	resp, err := e.h.GetMyAccount(withPrincipal(context.Background(), userPrincipal("ghost")), &identityv1.GetMyAccountRequest{})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v (%v), want NotFound", status.Code(err), err)
	}
	if resp.GetUserId() != "" || resp.GetCreatedAt() != nil {
		t.Fatalf("NOT_FOUND must return no account data: %+v", resp)
	}
}

// Scenario: A caller can never read another user's account (over the wire, naming A in metadata as B)
func TestGetMyAccountIgnoresOtherUserNamedInMetadata(t *testing.T) {
	users := storetest.NewInMemoryUserRepository()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	conn := startWire(t, rpc.Handlers{Auth: rpc.NewAuthHandler(svc)}, discardLogger())
	client := identityv1.NewAuthServiceClient(conn)
	e := accountEnv{users: users}
	e.seed(t, "buyer-A", []string{"buyer"}, acctCreated)
	e.seed(t, "buyer-B", []string{"buyer"}, acctCreated.Add(2*time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", "buyer-B", "x-principal-type", "user",
		"x-user-id", "buyer-A", "user-id", "buyer-A", "x-target-user-id", "buyer-A"))
	resp, err := client.GetMyAccount(ctx, &identityv1.GetMyAccountRequest{})
	if err != nil {
		t.Fatalf("GetMyAccount: %v", err)
	}
	if resp.GetUserId() != "buyer-B" {
		t.Fatalf("user_id = %q, want buyer-B", resp.GetUserId())
	}
	if !resp.GetCreatedAt().AsTime().Equal(acctCreated.Add(2 * time.Hour)) {
		t.Fatalf("created_at = %v, want B's own", resp.GetCreatedAt().AsTime())
	}
}

// Scenario: An anonymous call is refused (over the wire, no principal metadata)
func TestGetMyAccountWireAnonymousIsUnauthenticated(t *testing.T) {
	users := storetest.NewInMemoryUserRepository()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	conn := startWire(t, rpc.Handlers{Auth: rpc.NewAuthHandler(svc)}, discardLogger())
	client := identityv1.NewAuthServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.GetMyAccount(ctx, &identityv1.GetMyAccountRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v (%v), want Unauthenticated", status.Code(err), err)
	}
}
