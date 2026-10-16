package handler_test

// Task 8 of identity-user-enforcement: AccountAdminHandler.
//
// Contract pinned by these tests (the implementer adds exactly this):
//
//	func NewAccountAdminHandler(svc *service.AuthService, users UserLister, logger *slog.Logger) *AccountAdminHandler
//	type UserLister interface {
//	    ListUsers(ctx, query, status, cursor string, limit int) ([]repository.User, string, error)
//	}  // *repository.InMemoryUserRepository satisfies it
//	(*AccountAdminHandler).ListUsers / SetUserStatus implement identityv1.AccountAdminServiceServer.
//	Scope "admin" on the forwarded principal is required: no principal => Unauthenticated,
//	principal without it => PermissionDenied. The actor id is the principal id only.
//	Errors: service.ErrInvalidInput => InvalidArgument; repository.ErrNotFound => NotFound;
//	service.ErrStatusPrecondition => FailedPrecondition; service.ErrAuditUnavailable (and any
//	audit failure) => Unavailable with a fixed message that never echoes the cause.
//	Query > 64 bytes => InvalidArgument. Page size and cursor map to ListUsers(limit, cursor).

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

var accNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type accAuditCall struct {
	actor, user, status, reason, note string
	until                             *time.Time
}

type accAudit struct {
	mu    sync.Mutex
	calls []accAuditCall
	err   error
}

func (f *accAudit) WriteUserSetStatus(_ context.Context, actor, user, st, reason, note string, until *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, accAuditCall{actor, user, st, reason, note, until})
	return f.err
}

func (f *accAudit) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.calls) }

type accEnv struct {
	t       *testing.T
	users   *repository.InMemoryUserRepository
	sess    *repository.InMemorySessionRepository
	audit   *accAudit
	svc     *service.AuthService
	h       *handler.AccountAdminHandler
	logs    *bytes.Buffer
	adminID string
}

func newAccEnv(t *testing.T) *accEnv {
	t.Helper()
	e := &accEnv{t: t, users: repository.NewInMemoryUserRepository(), sess: repository.NewInMemorySessionRepository(), audit: &accAudit{}, logs: &bytes.Buffer{}}
	e.adminID = "admin-1"
	if _, err := e.users.Create(context.Background(), repository.User{ID: e.adminID, Username: "root", PasswordHash: "x", Roles: []string{"admin"}}); err != nil {
		t.Fatal(err)
	}
	e.users.AttachSessions(e.sess)
	e.svc = service.NewAuthService(e.users, testSigner(t), time.Hour).
		WithClock(func() time.Time { return accNow }).
		WithSessions(e.sess).
		WithAudit(e.audit)
	logger := slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.h = handler.NewAccountAdminHandler(e.svc, e.users, logger)
	return e
}

// seller registers a seller with one live session and returns its id.
func (e *accEnv) seller(name string) string {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.svc.Register(ctx, name, "password123", "seller"); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.svc.Login(ctx, name, "password123"); err != nil {
		e.t.Fatal(err)
	}
	u, err := e.users.GetByUsername(ctx, name)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.ID
}

func adminCtx(id string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: []string{"admin", "listing.read"},
	})
}

func sellerCtx() context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
		Id: "seller-x", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"listing.read", "listing.write", "identity:read", "identity:write"},
	})
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("code = %v (%v), want %v", status.Code(err), err, want)
	}
}

func (e *accEnv) liveSessions(userID string) int {
	e.t.Helper()
	ss, err := e.sess.ListSessions(context.Background(), userID)
	if err != nil {
		e.t.Fatal(err)
	}
	n := 0
	for _, s := range ss {
		if !s.Revoked {
			n++
		}
	}
	return n
}

// User enforcement: Audit down refuses the change
func TestSetUserStatusAuditDownUnavailableNoChange(t *testing.T) {
	for name, auditErr := range map[string]error{
		"error":             errors.New("boom-secret-cause"),
		"deadline exceeded": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			e := newAccEnv(t)
			uid := e.seller("victim")
			e.audit.err = auditErr
			until := timestamppb.New(accNow.Add(time.Hour))
			_, err := e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{
				UserId: uid, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED, ReasonCode: "scam", Until: until,
			})
			wantCode(t, err, codes.Unavailable)
			if strings.Contains(err.Error(), "boom-secret-cause") {
				t.Fatalf("Unavailable message echoes the cause: %v", err)
			}
			u, _ := e.users.GetByID(context.Background(), uid)
			if u.Status != repository.StatusActive {
				t.Fatalf("user status changed to %q", u.Status)
			}
			if got := e.liveSessions(uid); got != 1 {
				t.Fatalf("live sessions = %d, want 1", got)
			}
			if rows := e.sess.OutboxRows(); len(rows) != 0 {
				t.Fatalf("outbox rows = %d, want 0", len(rows))
			}
		})
	}
}

// User enforcement: Invalid input is rejected before any audit
func TestSetUserStatusInvalidInput(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("victim")
	ok := identityv1.UserStatus_USER_STATUS_RESTRICTED
	future := timestamppb.New(accNow.Add(time.Hour))
	cases := map[string]*identityv1.SetUserStatusRequest{
		"status unspecified":  {UserId: uid, Status: identityv1.UserStatus_USER_STATUS_UNSPECIFIED, ReasonCode: "scam"},
		"empty reason":        {UserId: uid, Status: ok, ReasonCode: ""},
		"reason 65 bytes":     {UserId: uid, Status: ok, ReasonCode: strings.Repeat("a", 65)},
		"upper case":          {UserId: uid, Status: ok, ReasonCode: "Scam"},
		"hyphen":              {UserId: uid, Status: ok, ReasonCode: "scam-x"},
		"leading digit":       {UserId: uid, Status: ok, ReasonCode: "1scam"},
		"trailing underscore": {UserId: uid, Status: ok, ReasonCode: "scam_"},
		"leading underscore":  {UserId: uid, Status: ok, ReasonCode: "_scam"},
		"double underscore":   {UserId: uid, Status: ok, ReasonCode: "scam__x"},
		"space":               {UserId: uid, Status: ok, ReasonCode: "scam x"},
		"note 501 chars":      {UserId: uid, Status: ok, ReasonCode: "scam", Note: strings.Repeat("ñ", 501)},
		"empty user id":       {UserId: "", Status: ok, ReasonCode: "scam"},
		"suspended no until":  {UserId: uid, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED, ReasonCode: "scam"},
		"suspended until now": {UserId: uid, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED, ReasonCode: "scam", Until: timestamppb.New(accNow)},
		"suspended past":      {UserId: uid, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED, ReasonCode: "scam", Until: timestamppb.New(accNow.Add(-time.Minute))},
		"banned with until":   {UserId: uid, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam", Until: future},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := e.h.SetUserStatus(adminCtx(e.adminID), req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
	if n := e.audit.count(); n != 0 {
		t.Fatalf("audit calls = %d, want 0", n)
	}
	if u, _ := e.users.GetByID(context.Background(), uid); u.Status != repository.StatusActive {
		t.Fatalf("user status changed to %q", u.Status)
	}
}

// User enforcement: Input boundaries are accepted
func TestSetUserStatusInputBoundaries(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("victim")
	long64 := strings.Repeat("a", 64)
	note500 := strings.Repeat("ñ", 500)
	cases := []struct {
		name   string
		status identityv1.UserStatus
		reason string
		note   string
		until  *timestamppb.Timestamp
	}{
		{"reason 64 bytes", identityv1.UserStatus_USER_STATUS_RESTRICTED, long64, "", nil},
		{"reason a", identityv1.UserStatus_USER_STATUS_RESTRICTED, "a", "", nil},
		{"reason with digit", identityv1.UserStatus_USER_STATUS_RESTRICTED, "counterfeit_seller2", "", nil},
		{"note 500 chars", identityv1.UserStatus_USER_STATUS_RESTRICTED, "scam", note500, nil},
		{"until one second ahead", identityv1.UserStatus_USER_STATUS_SUSPENDED, "scam", "", timestamppb.New(accNow.Add(time.Second))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := e.audit.count()
			_, err := e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{
				UserId: uid, Status: c.status, ReasonCode: c.reason, Note: c.note, Until: c.until,
			})
			if err != nil {
				t.Fatalf("SetUserStatus: %v", err)
			}
			if e.audit.count() != before+1 {
				t.Fatalf("audit calls = %d, want %d", e.audit.count(), before+1)
			}
			last := e.audit.calls[len(e.audit.calls)-1]
			if last.note != c.note || last.reason != c.reason || last.user != uid || last.actor != e.adminID {
				t.Fatalf("audit event = %+v", last)
			}
		})
	}
}

// User enforcement: Non-admin cannot set a status
func TestSetUserStatusRequiresAdminScope(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("victim")
	req := &identityv1.SetUserStatusRequest{UserId: uid, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam"}

	_, err := e.h.SetUserStatus(sellerCtx(), req)
	wantCode(t, err, codes.PermissionDenied)
	_, err = e.h.SetUserStatus(context.Background(), req)
	wantCode(t, err, codes.Unauthenticated)

	if n := e.audit.count(); n != 0 {
		t.Fatalf("audit calls = %d, want 0", n)
	}
	if u, _ := e.users.GetByID(context.Background(), uid); u.Status != repository.StatusActive {
		t.Fatalf("user status changed to %q", u.Status)
	}
}

// User enforcement: Ban writes one audit event and revokes sessions (handler mapping, actor from principal)
func TestSetUserStatusBanMapsResponseAndErrors(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("victim")
	res, err := e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{
		UserId: uid, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetSessionsRevoked() != 1 || res.GetUser().GetId() != uid || res.GetUser().GetStatus() != identityv1.UserStatus_USER_STATUS_BANNED {
		t.Fatalf("response = %v", res)
	}
	if e.audit.count() != 1 || e.audit.calls[0].actor != e.adminID {
		t.Fatalf("audit = %+v", e.audit.calls)
	}

	// Error mapping: unknown user, self, admin target.
	_, err = e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{UserId: "nope", Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam"})
	wantCode(t, err, codes.NotFound)
	_, err = e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{UserId: e.adminID, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam"})
	wantCode(t, err, codes.FailedPrecondition)
}

// User enforcement: Audit metadata never leaks into logs
func TestSetUserStatusNoteNeverLogged(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("victim")
	const marker = "NOTE-MARKER-7f3a9"
	if _, err := e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{
		UserId: uid, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam", Note: marker,
	}); err != nil {
		t.Fatal(err)
	}
	// Failure paths log too (audit down); the note must not appear there either.
	e.audit.err = errors.New("down")
	_, _ = e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{
		UserId: uid, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam", Note: marker,
	})
	if strings.Contains(e.logs.String(), marker) {
		t.Fatalf("note leaked into logs:\n%s", e.logs.String())
	}
}

// User enforcement: Oversized query is rejected
func TestListUsersQueryTooLong(t *testing.T) {
	e := newAccEnv(t)
	_, err := e.h.ListUsers(adminCtx(e.adminID), &identityv1.ListUsersRequest{Query: strings.Repeat("a", 65)})
	wantCode(t, err, codes.InvalidArgument)
	if _, err := e.h.ListUsers(adminCtx(e.adminID), &identityv1.ListUsersRequest{Query: strings.Repeat("a", 64)}); err != nil {
		t.Fatalf("64-byte query must be accepted: %v", err)
	}
}

// User enforcement: Listing never exposes credentials and is not audited
func TestListUsersNoCredentialsNoAudit(t *testing.T) {
	e := newAccEnv(t)
	uid := e.seller("alice")
	e.seller("alina")
	e.seller("bob")
	stored, _ := e.users.GetByID(context.Background(), uid)
	if stored.PasswordHash == "" {
		t.Fatal("fixture: expected a stored password hash")
	}

	res, err := e.h.ListUsers(adminCtx(e.adminID), &identityv1.ListUsersRequest{Query: "ali"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetUsers()) != 2 || res.GetUsers()[0].GetUsername() != "alice" || res.GetUsers()[1].GetUsername() != "alina" {
		t.Fatalf("users = %v", res.GetUsers())
	}
	if res.GetUsers()[0].GetStatus() != identityv1.UserStatus_USER_STATUS_ACTIVE {
		t.Fatalf("status = %v", res.GetUsers()[0].GetStatus())
	}
	if strings.Contains(prototext.Format(res), stored.PasswordHash) {
		t.Fatal("password hash serialized in ListUsers response")
	}
	if e.audit.count() != 0 {
		t.Fatalf("audit calls = %d, want 0", e.audit.count())
	}
}

// User enforcement: Listing never exposes credentials and is not audited (status filter + paging pass-through)
func TestListUsersStatusFilterAndPaging(t *testing.T) {
	e := newAccEnv(t)
	a := e.seller("u1")
	e.seller("u2")
	e.seller("u3")
	if _, err := e.h.SetUserStatus(adminCtx(e.adminID), &identityv1.SetUserStatusRequest{UserId: a, Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "scam"}); err != nil {
		t.Fatal(err)
	}
	res, err := e.h.ListUsers(adminCtx(e.adminID), &identityv1.ListUsersRequest{Status: identityv1.UserStatus_USER_STATUS_BANNED})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetUsers()) != 1 || res.GetUsers()[0].GetId() != a {
		t.Fatalf("banned filter = %v", res.GetUsers())
	}

	p1, err := e.h.ListUsers(adminCtx(e.adminID), &identityv1.ListUsersRequest{Query: "u", Page: &commonv1.PageRequest{PageSize: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.GetUsers()) != 2 || p1.GetPage().GetNextCursor() == "" {
		t.Fatalf("page 1 = %v", p1)
	}
	p2, err := e.h.ListUsers(adminCtx(e.adminID), &identityv1.ListUsersRequest{Query: "u", Page: &commonv1.PageRequest{PageSize: 2, Cursor: p1.GetPage().GetNextCursor()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.GetUsers()) != 1 || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page 2 = %v", p2)
	}
}

// User enforcement: Non-admin cannot list users
func TestListUsersRequiresAdminScope(t *testing.T) {
	e := newAccEnv(t)
	_, err := e.h.ListUsers(sellerCtx(), &identityv1.ListUsersRequest{})
	wantCode(t, err, codes.PermissionDenied)
	_, err = e.h.ListUsers(context.Background(), &identityv1.ListUsersRequest{})
	wantCode(t, err, codes.Unauthenticated)
}
