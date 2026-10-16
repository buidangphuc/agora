package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	setUserStatusPath = "/platform.identity.v1.AccountAdminService/SetUserStatus"
	listUsersPath     = "/platform.identity.v1.AccountAdminService/ListUsers"
)

type accountAdminUpstream struct {
	identityv1.AccountAdminServiceClient
	setCalls  atomic.Int64
	listCalls atomic.Int64
	setErr    error
	gotReq    atomic.Pointer[identityv1.SetUserStatusRequest]
	gotPID    atomic.Value // string
}

func (a *accountAdminUpstream) SetUserStatus(ctx context.Context, in *identityv1.SetUserStatusRequest, _ ...grpc.CallOption) (*identityv1.SetUserStatusResponse, error) {
	a.setCalls.Add(1)
	a.gotReq.Store(in)
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if v := md.Get("x-principal-id"); len(v) > 0 {
			a.gotPID.Store(v[0])
		}
	}
	if a.setErr != nil {
		return nil, a.setErr
	}
	return &identityv1.SetUserStatusResponse{SessionsRevoked: 3}, nil
}

func (a *accountAdminUpstream) ListUsers(context.Context, *identityv1.ListUsersRequest, ...grpc.CallOption) (*identityv1.ListUsersResponse, error) {
	a.listCalls.Add(1)
	return &identityv1.ListUsersResponse{}, nil
}

func callAccountAdmin(t *testing.T, up *accountAdminUpstream, path, body string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{AccountAdmin: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token(t, "admin"))
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Scenario: User enforcement edge: Admin reaches SetUserStatus.
func TestAccountAdminSetUserStatusForwarded(t *testing.T) {
	up := &accountAdminUpstream{}
	code, body := callAccountAdmin(t, up, setUserStatusPath,
		`{"user_id":"u-9","status":"USER_STATUS_BANNED","reason_code":"fraud"}`)
	if code != http.StatusOK {
		t.Fatalf("admin SetUserStatus = %d %s, want 200 (not 501)", code, body)
	}
	if n := up.setCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want exactly 1", n)
	}
	if g := up.gotReq.Load(); g.GetUserId() != "u-9" || g.GetReasonCode() != "fraud" || g.GetStatus() != identityv1.UserStatus_USER_STATUS_BANNED {
		t.Fatalf("forwarded request altered: %v", g)
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Fatalf("caller principal id on upstream call = %q, want user-1", pid)
	}
	if !strings.Contains(body, "sessionsRevoked") || !strings.Contains(body, "3") {
		t.Fatalf("response not returned unchanged: %s", body)
	}
}

// Scenario: User enforcement edge: Admin reaches SetUserStatus (ListUsers sibling is routed too).
func TestAccountAdminListUsersForwarded(t *testing.T) {
	up := &accountAdminUpstream{}
	code, body := callAccountAdmin(t, up, listUsersPath, `{"query":"ab"}`)
	if code != http.StatusOK {
		t.Fatalf("admin ListUsers = %d %s, want 200 (not 501)", code, body)
	}
	if n := up.listCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

// Scenario: User enforcement edge: SetUserStatus is not retried.
func TestAccountAdminSetUserStatusNotRetried(t *testing.T) {
	up := &accountAdminUpstream{setErr: status.Error(codes.Unavailable, "identity down")}
	code, body := callAccountAdmin(t, up, setUserStatusPath,
		`{"user_id":"u-9","status":"USER_STATUS_BANNED","reason_code":"fraud"}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("SetUserStatus = %d %s, want 503 unavailable", code, body)
	}
	if n := up.setCalls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want exactly 1 (no retry)", n)
	}
}
