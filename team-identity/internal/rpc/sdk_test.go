package rpc_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

const sdkAuthRequired = "authentication required"

func isAuthDenied(err error) bool {
	switch status.Code(err) {
	case codes.PermissionDenied:
		return true
	case codes.Unauthenticated:
		return status.Convert(err).Message() == sdkAuthRequired
	}
	return false
}

type principalCase struct {
	name string
	ctx  context.Context
}

func principalCases() []principalCase {
	bg := context.Background()
	return []principalCase{
		{"no principal", bg},
		{"anonymous", auth.WithPrincipal(bg, auth.Principal{ID: "anonymous", Type: auth.TypeAnonymous})},
		{"user without admin", auth.WithPrincipal(bg, auth.Principal{ID: "u-1", Type: auth.TypeUser})},
		{"user", auth.WithPrincipal(bg, auth.Principal{ID: "u-1", Type: auth.TypeUser, Scopes: []string{"identity.read", "identity.write"}})},
		{"admin user", auth.WithPrincipal(bg, auth.Principal{ID: "u-1", Type: auth.TypeUser, Scopes: []string{"admin"}})},
		{"service without scope", auth.WithPrincipal(bg, auth.Principal{ID: "svc-x", Type: auth.TypeService})},
		{"service identity.read", auth.WithPrincipal(bg, auth.Principal{ID: "svc-x", Type: auth.TypeService, Scopes: []string{"identity.read"}})},
	}
}

type boundaryRPCs struct {
	user, admin, profile, public map[string]func(context.Context) error
}

func newBoundaryRPCs(t *testing.T) boundaryRPCs {
	t.Helper()
	users := storetest.NewInMemoryUserRepository()
	sess := storetest.NewInMemorySessionRepository()
	users.AttachSessions(sess)
	svc := newSvc(t, users, sess)
	ah := rpc.NewAuthHandler(svc)
	sh := rpc.NewSessionHandler(sess, nil)
	dh := rpc.NewAddressHandler(storetest.NewInMemoryAddressRepository(), nil)
	adm := rpc.NewAccountAdminHandler(svc, users, nil)
	ph := rpc.NewProfileHandler(users, nil)
	var n atomic.Int64
	wrap := func(f func(context.Context) error) func(context.Context) error { return f }
	return boundaryRPCs{
		user: map[string]func(context.Context) error{
			"ChangePassword": wrap(func(ctx context.Context) error {
				_, err := ah.ChangePassword(ctx, &identityv1.ChangePasswordRequest{OldPassword: "x", NewPassword: "y"})
				return err
			}),
			"ListSessions": wrap(func(ctx context.Context) error {
				_, err := sh.ListSessions(ctx, &identityv1.ListSessionsRequest{})
				return err
			}),
			"RevokeSession": wrap(func(ctx context.Context) error {
				_, err := sh.RevokeSession(ctx, &identityv1.RevokeSessionRequest{SessionId: "s"})
				return err
			}),
			"ListLoginHistory": wrap(func(ctx context.Context) error {
				_, err := sh.ListLoginHistory(ctx, &identityv1.ListLoginHistoryRequest{})
				return err
			}),
			"ListAddresses": wrap(func(ctx context.Context) error {
				_, err := dh.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
				return err
			}),
			"CreateAddress": wrap(func(ctx context.Context) error {
				_, err := dh.CreateAddress(ctx, &identityv1.CreateAddressRequest{RecipientName: "a", Phone: "1", Street: "s", City: "c"})
				return err
			}),
			"UpdateAddress": wrap(func(ctx context.Context) error {
				_, err := dh.UpdateAddress(ctx, &identityv1.UpdateAddressRequest{Id: "x", RecipientName: "a", Phone: "1", Street: "s", City: "c"})
				return err
			}),
			"DeleteAddress": wrap(func(ctx context.Context) error {
				_, err := dh.DeleteAddress(ctx, &identityv1.DeleteAddressRequest{Id: "x"})
				return err
			}),
			"SetDefaultAddress": wrap(func(ctx context.Context) error {
				_, err := dh.SetDefaultAddress(ctx, &identityv1.SetDefaultAddressRequest{Id: "x"})
				return err
			}),
		},
		admin: map[string]func(context.Context) error{
			"ListUsers": wrap(func(ctx context.Context) error {
				_, err := adm.ListUsers(ctx, &identityv1.ListUsersRequest{})
				return err
			}),
			"SetUserStatus": wrap(func(ctx context.Context) error {
				_, err := adm.SetUserStatus(ctx, &identityv1.SetUserStatusRequest{UserId: "nope", Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam"})
				return err
			}),
		},
		profile: map[string]func(context.Context) error{
			"GetPublicProfiles": wrap(func(ctx context.Context) error {
				_, err := ph.GetPublicProfiles(ctx, &identityv1.GetPublicProfilesRequest{UserIds: []string{"u1"}})
				return err
			}),
		},
		public: map[string]func(context.Context) error{
			"Register": wrap(func(ctx context.Context) error {
				_, err := ah.Register(ctx, &identityv1.RegisterRequest{Username: fmt.Sprintf("pub_%d", n.Add(1)), Password: "password123", Role: "buyer"})
				return err
			}),
			"Login": wrap(func(ctx context.Context) error {
				_, err := ah.Login(ctx, &identityv1.LoginRequest{Username: "nobody", Password: "password123"})
				return err
			}),
			"RefreshSession": wrap(func(ctx context.Context) error {
				_, err := ah.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: "unknown-token"})
				return err
			}),
		},
	}
}

// Scenarios: A protected RPC refuses an anonymous or wrong caller; Public RPCs need no principal
func TestAuthorizationBoundary(t *testing.T) {
	r := newBoundaryRPCs(t)
	type want struct{ none, anon, userNoAdmin, user, admin, svcNoScope, svcRead string }
	const (
		denied   = "unauthenticated"
		forbid   = "permission_denied"
		accepted = "accepted"
	)
	groups := []struct {
		name  string
		calls map[string]func(context.Context) error
		want  want
	}{
		{"user-scoped", r.user, want{denied, denied, accepted, accepted, accepted, forbid, forbid}},
		{"admin", r.admin, want{denied, denied, forbid, forbid, accepted, forbid, forbid}},
		{"profile", r.profile, want{denied, denied, forbid, forbid, forbid, forbid, accepted}},
		{"public", r.public, want{accepted, accepted, accepted, accepted, accepted, accepted, accepted}},
	}
	for _, g := range groups {
		expect := []string{g.want.none, g.want.anon, g.want.userNoAdmin, g.want.user, g.want.admin, g.want.svcNoScope, g.want.svcRead}
		for rpcName, call := range g.calls {
			for i, pc := range principalCases() {
				t.Run(g.name+"/"+rpcName+"/"+pc.name, func(t *testing.T) {
					err := call(pc.ctx)
					switch expect[i] {
					case denied:
						if status.Code(err) != codes.Unauthenticated || status.Convert(err).Message() != sdkAuthRequired {
							t.Fatalf("got %v, want Unauthenticated %q", err, sdkAuthRequired)
						}
					case forbid:
						if status.Code(err) != codes.PermissionDenied {
							t.Fatalf("got %v, want PermissionDenied", err)
						}
					case accepted:
						if isAuthDenied(err) {
							t.Fatalf("got %v, want the call to pass authorization", err)
						}
					}
				})
			}
		}
	}
}

func mdCtxAs(id, typ, scopes string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-principal-id", id, "x-principal-type", typ, "x-principal-scopes", scopes,
	)), cancel
}

// Scenario: Public RPCs need no principal
func TestRefreshSessionIsPublic(t *testing.T) {
	authClient, _ := startServer(t, storetest.NewInMemoryUserRepository(), storetest.NewInMemoryAddressRepository())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reg, err := authClient.Register(ctx, &identityv1.RegisterRequest{Username: "public_user", Password: "password123", Role: "buyer"})
	if err != nil {
		t.Fatalf("Register without principal: %v", err)
	}
	if _, err := authClient.Login(ctx, &identityv1.LoginRequest{Username: "public_user", Password: "password123"}); err != nil {
		t.Fatalf("Login without principal: %v", err)
	}
	res, err := authClient.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: reg.GetResult().GetRefreshToken()})
	if err != nil {
		t.Fatalf("RefreshSession without principal: %v", err)
	}
	if res.GetResult().GetToken() == "" {
		t.Fatal("RefreshSession returned no access token")
	}
}

type blockingSessions struct {
	store.SessionRepository
	entered chan struct{}
}

func (b *blockingSessions) ListSessions(ctx context.Context, _ string) ([]store.Session, error) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func userCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: auth.TypeUser})
}

func blockingWire(t *testing.T) (identityv1.SessionServiceClient, *blockingSessions) {
	t.Helper()
	b := &blockingSessions{SessionRepository: storetest.NewInMemorySessionRepository(), entered: make(chan struct{}, 1)}
	logger := discardLogger()
	conn := startWire(t, rpc.Handlers{
		Auth:    rpc.NewAuthHandler(newMemSvc(t)),
		Address: rpc.NewAddressHandler(storetest.NewInMemoryAddressRepository(), logger),
		Session: rpc.NewSessionHandler(b, logger),
		Profile: rpc.NewProfileHandler(storetest.NewInMemoryUserRepository(), logger),
		Admin:   rpc.NewAccountAdminHandler(newMemSvc(t), storetest.NewInMemoryUserRepository(), logger),
	}, logger)
	return identityv1.NewSessionServiceClient(conn), b
}

// Scenario: A cancelled request returns Canceled
func TestCanceledRequestMapsToCanceled(t *testing.T) {
	t.Run("handler", func(t *testing.T) {
		h := rpc.NewSessionHandler(&blockingSessions{SessionRepository: storetest.NewInMemorySessionRepository(), entered: make(chan struct{}, 1)}, nil)
		ctx, cancel := context.WithCancel(userCtx("u-1"))
		cancel()
		_, err := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
		if status.Code(err) != codes.Canceled {
			t.Fatalf("code = %v (%v), want Canceled", status.Code(err), err)
		}
	})
	t.Run("wire", func(t *testing.T) {
		client, b := blockingWire(t)
		ctx, cancel := mdCtxAs("u-1", "user", "")
		defer cancel()
		ctx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			_, err := client.ListSessions(ctx, &identityv1.ListSessionsRequest{})
			done <- err
		}()
		<-b.entered
		stop()
		if err := <-done; status.Code(err) != codes.Canceled {
			t.Fatalf("code = %v (%v), want Canceled", status.Code(err), err)
		}
	})
}

// Scenario: A timed-out request returns DeadlineExceeded
func TestDeadlineExceededMapsToDeadlineExceeded(t *testing.T) {
	t.Run("handler", func(t *testing.T) {
		h := rpc.NewSessionHandler(&blockingSessions{SessionRepository: storetest.NewInMemorySessionRepository(), entered: make(chan struct{}, 1)}, nil)
		ctx, cancel := context.WithDeadline(userCtx("u-1"), time.Now().Add(-time.Second))
		defer cancel()
		_, err := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("code = %v (%v), want DeadlineExceeded", status.Code(err), err)
		}
	})
	t.Run("wire", func(t *testing.T) {
		client, _ := blockingWire(t)
		ctx, cancel := mdCtxAs("u-1", "user", "")
		defer cancel()
		ctx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
		defer stop()
		_, err := client.ListSessions(ctx, &identityv1.ListSessionsRequest{})
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("code = %v (%v), want DeadlineExceeded", status.Code(err), err)
		}
	})
}

// Scenario: A duplicate username returns the explicit message
func TestRegisterDuplicateUsernameMessage(t *testing.T) {
	h := rpc.NewAuthHandler(newMemSvc(t))
	req := &identityv1.RegisterRequest{Username: "dup_user", Password: "password123", Role: "buyer"}
	if _, err := h.Register(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	_, err := h.Register(context.Background(), req)
	if status.Code(err) != codes.AlreadyExists || status.Convert(err).Message() != "username already exists" {
		t.Fatalf("got %v, want AlreadyExists %q", err, "username already exists")
	}
}

type uniqueViolationAddrs struct {
	store.AddressRepository
}

func (uniqueViolationAddrs) Create(context.Context, store.Address) (store.Address, error) {
	return store.Address{}, fmt.Errorf("insert address: %w", &pgconn.PgError{Code: "23505", ConstraintName: "addresses_one_default_per_user_idx"})
}

// Scenario: An unmapped unique violation returns AlreadyExists
func TestUniqueViolationMapsToAlreadyExists(t *testing.T) {
	h := rpc.NewAddressHandler(uniqueViolationAddrs{}, nil)
	_, err := h.CreateAddress(principalContext("user-1"), &identityv1.CreateAddressRequest{RecipientName: "a", Phone: "1", Street: "s", City: "c"})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v (%v), want AlreadyExists", status.Code(err), err)
	}
	if strings.Contains(status.Convert(err).Message(), "addresses_one_default_per_user_idx") {
		t.Fatalf("constraint name leaked: %q", status.Convert(err).Message())
	}
}

type panickingSessions struct {
	store.SessionRepository
	calls atomic.Int32
}

func (p *panickingSessions) ListSessions(ctx context.Context, userID string) ([]store.Session, error) {
	if p.calls.Add(1) == 1 {
		panic("boom-secret-panic")
	}
	return p.SessionRepository.ListSessions(ctx, userID)
}

// Scenario: A panicking handler does not stop the server
func TestPanicIsRecoveredAsInternal(t *testing.T) {
	ps := &panickingSessions{SessionRepository: storetest.NewInMemorySessionRepository()}
	logger := discardLogger()
	conn := startWire(t, rpc.Handlers{
		Auth:    rpc.NewAuthHandler(newMemSvc(t)),
		Address: rpc.NewAddressHandler(storetest.NewInMemoryAddressRepository(), logger),
		Session: rpc.NewSessionHandler(ps, logger),
		Profile: rpc.NewProfileHandler(storetest.NewInMemoryUserRepository(), logger),
		Admin:   rpc.NewAccountAdminHandler(newMemSvc(t), storetest.NewInMemoryUserRepository(), logger),
	}, logger)
	client := identityv1.NewSessionServiceClient(conn)
	ctx, cancel := mdCtxAs("u-1", "user", "")
	defer cancel()
	_, err := client.ListSessions(ctx, &identityv1.ListSessionsRequest{})
	if status.Code(err) != codes.Internal || strings.Contains(status.Convert(err).Message(), "boom-secret-panic") {
		t.Fatalf("first call = %v, want Internal without panic text", err)
	}
	if _, err := client.ListSessions(ctx, &identityv1.ListSessionsRequest{}); err != nil {
		t.Fatalf("second call after panic: %v", err)
	}
}

// Scenario: Locked accounts keep their message
func TestLoginBannedKeepsLockedMessage(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("locked_user")
	if _, err := e.svc.SetUserStatus(context.Background(), e.adminID, uid, store.StatusBanned, "scam", "", nil); err != nil {
		t.Fatal(err)
	}
	_, err := rpc.NewAuthHandler(e.svc).Login(context.Background(), &identityv1.LoginRequest{Username: "locked_user", Password: "password123"})
	if status.Code(err) != codes.PermissionDenied || status.Convert(err).Message() != "tài khoản bị khóa" {
		t.Fatalf("got %v, want PermissionDenied %q", err, "tài khoản bị khóa")
	}
}

// Scenario: Locked accounts keep their message (the other explicit messages stay unchanged)
func TestExplicitErrorMessagesUnchanged(t *testing.T) {
	h := rpc.NewAuthHandler(newMemSvc(t))
	ctx := context.Background()
	if _, err := h.Register(ctx, &identityv1.RegisterRequest{Username: "msg_user", Password: "password123", Role: "buyer"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.Login(ctx, &identityv1.LoginRequest{Username: "msg_user", Password: "wrong-password"})
	if status.Code(err) != codes.Unauthenticated || status.Convert(err).Message() != "invalid credentials" {
		t.Fatalf("wrong password: got %v, want Unauthenticated %q", err, "invalid credentials")
	}
	_, err = h.Register(ctx, &identityv1.RegisterRequest{Username: "", Password: "", Role: "buyer"})
	if status.Code(err) != codes.InvalidArgument || status.Convert(err).Message() != "invalid input" {
		t.Fatalf("empty register: got %v, want InvalidArgument %q", err, "invalid input")
	}
}
