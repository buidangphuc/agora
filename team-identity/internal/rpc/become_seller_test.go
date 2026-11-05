package rpc_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

const bsCurrent = "V1"

type bsUsers struct {
	*storetest.InMemoryUserRepository
	mu     sync.Mutex
	roles  map[string][]string
	outbox []store.OutboxRow
}

func (f *bsUsers) GetByID(ctx context.Context, id string) (store.User, error) {
	u, err := f.InMemoryUserRepository.GetByID(ctx, id)
	if err != nil {
		return u, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.roles[id]; ok {
		u.Roles = append([]string(nil), r...)
	}
	return u, nil
}

func (f *bsUsers) GrantSeller(ctx context.Context, userID, _ string, now time.Time, enqueue store.SellerEnqueueFn, audit func(context.Context) error) (store.SellerGrantResult, error) {
	u, err := f.GetByID(ctx, userID)
	if err != nil {
		return store.SellerGrantResult{}, err
	}
	if u.Status != store.StatusActive {
		return store.SellerGrantResult{}, store.ErrNotActive
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range u.Roles {
		if r == "seller" {
			return store.SellerGrantResult{User: u}, nil
		}
	}
	row, err := enqueue(store.SellerGrant{UserID: userID, Role: "seller", GrantedAt: now})
	if err != nil {
		return store.SellerGrantResult{}, err
	}
	if audit != nil {
		if err := audit(ctx); err != nil {
			return store.SellerGrantResult{}, err
		}
	}
	f.outbox = append(f.outbox, row)
	u.Roles = append(u.Roles, "seller")
	f.roles[userID] = u.Roles
	return store.SellerGrantResult{User: u, Granted: true}, nil
}

func (f *bsUsers) rows() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.outbox) }

type bsRPC struct {
	client identityv1.AuthServiceClient
	users  *bsUsers
	sess   *storetest.InMemorySessionRepository
	uid    string
	sid    string
	token  string
}

func newBSRPC(t *testing.T) *bsRPC {
	t.Helper()
	return newBSRPCAudit(t, nopAudit{})
}

func newBSRPCAudit(t *testing.T, w auditx.Writer) *bsRPC {
	t.Helper()
	e := &bsRPC{
		users: &bsUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository(), roles: map[string][]string{}},
		sess:  storetest.NewInMemorySessionRepository(),
	}
	svc := newSvcWith(t, e.users, e.sess, w, nil).WithSellerTerms([]string{bsCurrent, "V0"})
	reg, err := svc.Register(context.Background(), "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	e.uid, e.token, e.sid = reg.UserID, reg.Token, parseSID(t, reg.Token)
	conn := startWire(t, rpc.Handlers{Auth: rpc.NewAuthHandler(svc)}, discardLogger())
	e.client = identityv1.NewAuthServiceClient(conn)
	return e
}

func bsCtx(t *testing.T, pairs ...string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func (e *bsRPC) userCtx(t *testing.T) context.Context {
	return bsCtx(t, "x-principal-id", e.uid, "x-principal-type", "user", "x-principal-sid", e.sid)
}

func (e *bsRPC) wantCode(ctx context.Context, t *testing.T, version string, want codes.Code) {
	t.Helper()
	resp, err := e.client.BecomeSeller(ctx, &identityv1.BecomeSellerRequest{TermsVersion: version})
	if status.Code(err) != want {
		t.Fatalf("BecomeSeller(%q) = %v, want %v", version, err, want)
	}
	if resp.GetResult().GetToken() != "" {
		t.Fatalf("a refused call must carry no token")
	}
	if e.users.rows() != 0 {
		t.Fatalf("outbox rows = %d, want 0", e.users.rows())
	}
}

// Seller onboarding: a buyer becomes a seller
func TestBecomeSellerRPCReturnsSellerToken(t *testing.T) {
	e := newBSRPC(t)
	resp, err := e.client.BecomeSeller(e.userCtx(t), &identityv1.BecomeSellerRequest{TermsVersion: bsCurrent})
	if err != nil {
		t.Fatalf("BecomeSeller: %v", err)
	}
	res := resp.GetResult()
	if res.GetToken() == "" || res.GetToken() == e.token {
		t.Fatalf("want a fresh token")
	}
	if got := parseSID(t, res.GetToken()); got != e.sid {
		t.Fatalf("token sid = %q, want %q", got, e.sid)
	}
	if res.GetPrincipal().GetId() != e.uid {
		t.Fatalf("principal id = %q, want %q", res.GetPrincipal().GetId(), e.uid)
	}
	found := false
	for _, s := range res.GetPrincipal().GetScopes() {
		if s == "listing.write" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scopes %v lack listing.write", res.GetPrincipal().GetScopes())
	}
	if res.GetRefreshToken() != "" || res.GetRefreshExpiresAt() != nil {
		t.Fatalf("refresh token must be empty")
	}
}

// Seller onboarding: an unknown terms version is refused
func TestBecomeSellerRPCUnknownTermsInvalidArgument(t *testing.T) {
	for _, v := range []string{"", "bogus", "V0", "v1", " V1", "V1 "} {
		e := newBSRPC(t)
		e.wantCode(e.userCtx(t), t, v, codes.InvalidArgument)
	}
}

// Seller onboarding: a non-ACTIVE account is refused
func TestBecomeSellerRPCNonActivePermissionDenied(t *testing.T) {
	e := newBSRPC(t)
	noRow := func(store.Session) (store.OutboxRow, error) { return store.OutboxRow{}, nil }
	chg := func(store.StatusChange) (store.OutboxRow, error) { return store.OutboxRow{}, nil }
	if _, err := e.users.SetStatus(context.Background(), e.uid, store.StatusRestricted, "test", time.Now().Add(time.Hour), time.Now(), noRow, chg); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	e.wantCode(e.userCtx(t), t, bsCurrent, codes.PermissionDenied)
}

// Seller onboarding: a service or anonymous caller is refused
func TestBecomeSellerRPCRequiresUserPrincipal(t *testing.T) {
	e := newBSRPC(t)
	e.wantCode(bsCtx(t), t, bsCurrent, codes.Unauthenticated)
	e.wantCode(bsCtx(t, "x-principal-id", "svc-orders", "x-principal-type", "service", "x-principal-sid", e.sid), t, bsCurrent, codes.PermissionDenied)
}

// Seller onboarding: a revoked session cannot mint a token
func TestBecomeSellerRPCRevokedSessionUnauthenticated(t *testing.T) {
	e := newBSRPC(t)
	if err := e.sess.RevokeSession(context.Background(), e.sid, e.uid, nil); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	e.wantCode(e.userCtx(t), t, bsCurrent, codes.Unauthenticated)
}

// Seller onboarding: a revoked session cannot mint a token (forwarded sid missing)
func TestBecomeSellerRPCMissingSidUnauthenticated(t *testing.T) {
	e := newBSRPC(t)
	e.wantCode(bsCtx(t, "x-principal-id", e.uid, "x-principal-type", "user"), t, bsCurrent, codes.Unauthenticated)
}

type downAudit struct{}

func (downAudit) WriteAuditEvent(context.Context, auditx.Event) error {
	return errors.New("audit store connection refused at 10.0.0.7:5432")
}

// Seller onboarding: audit unavailable refuses the grant
func TestBecomeSellerRPCAuditUnavailableErrorInfo(t *testing.T) {
	e := newBSRPCAudit(t, downAudit{})
	resp, err := e.client.BecomeSeller(e.userCtx(t), &identityv1.BecomeSellerRequest{TermsVersion: bsCurrent})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Fatalf("BecomeSeller = %v, want Unavailable", err)
	}
	if resp.GetResult().GetToken() != "" {
		t.Fatalf("a refused call must carry no token")
	}
	var info *errdetails.ErrorInfo
	for _, d := range st.Details() {
		if i, ok := d.(*errdetails.ErrorInfo); ok {
			info = i
		}
	}
	if info == nil || info.GetDomain() != "identity.platform" || info.GetReason() != "AUDIT_UNAVAILABLE" {
		t.Fatalf("ErrorInfo = %v, want identity.platform/AUDIT_UNAVAILABLE", info)
	}
	if strings.Contains(st.Message(), "10.0.0.7") {
		t.Fatalf("message leaks the upstream error: %q", st.Message())
	}
	if e.users.rows() != 0 {
		t.Fatalf("outbox rows = %d, want 0", e.users.rows())
	}
}
