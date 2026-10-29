package identity_test

// Task 1.2 (frontend-session-client-context): RefreshSession passes the ctx ClientInfo
// (clipped ip 64 / UA 256) to SessionRepository.RotateRefresh.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

// clientEnv registers alice from a known first client, so the stored ip/device are non-empty.
func clientEnv(ctx context.Context, t *testing.T) *refreshEnv {
	t.Helper()
	e := &refreshEnv{
		users: &fakeUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository()},
		sess:  &spySessions{InMemorySessionRepository: storetest.NewInMemorySessionRepository()},
		clock: &fakeClock{t: t0},
	}
	e.svc = newSvc(t, e.users, e.sess).WithClock(e.clock.Now)
	ctx = identity.WithClient(ctx, identity.ClientInfo{IP: "203.0.113.1", UserAgent: "AgoraTest/1"})
	res, err := e.svc.Register(ctx, "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	e.uid, e.first = res.UserID, res
	return e
}

func (e *refreshEnv) refreshWith(t *testing.T, tok string, c identity.ClientInfo) (identity.AuthResult, error) {
	t.Helper()
	return e.svc.RefreshSession(identity.WithClient(context.Background(), c), tok)
}

func TestSessionClientContext_RotationRecordsIPViaRepository(t *testing.T) {
	e := clientEnv(t.Context(), t)
	e.clock.Advance(time.Minute)
	if _, err := e.refreshWith(t, e.first.RefreshToken, identity.ClientInfo{IP: "198.51.100.4", UserAgent: "AgoraTest/2"}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := e.sessionRow(t.Context(), t, e.uid)
	if s.IP != "198.51.100.4" || s.Device != "AgoraTest/2" {
		t.Fatalf("want ip/device from ctx, got ip=%q device=%q", s.IP, s.Device)
	}
	if !s.LastSeen.Equal(t0.Add(time.Minute)) {
		t.Fatalf("last_seen = %v, want now", s.LastSeen)
	}
	if s.RefreshHash == sha(e.first.RefreshToken) {
		t.Fatalf("refresh hash must be rotated")
	}
}

func TestSessionClientContext_EmptyClientKeepsStoredValues_Boundary(t *testing.T) {
	cases := map[string]context.Context{
		"no ClientInfo":    context.Background(),
		"empty ClientInfo": identity.WithClient(context.Background(), identity.ClientInfo{}),
	}
	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			e := clientEnv(ctx, t)
			before := e.sessionRow(ctx, t, e.uid)
			if before.IP != "203.0.113.1" || before.Device != "AgoraTest/1" {
				t.Fatalf("precondition: register did not record client: %+v", before)
			}
			e.clock.Advance(time.Minute)
			if _, err := e.svc.RefreshSession(ctx, e.first.RefreshToken); err != nil {
				t.Fatalf("refresh: %v", err)
			}
			s := e.sessionRow(ctx, t, e.uid)
			if s.IP != before.IP || s.Device != before.Device {
				t.Fatalf("ip/device must be kept, got ip=%q device=%q", s.IP, s.Device)
			}
			if !s.LastSeen.After(before.LastSeen) {
				t.Fatalf("last_seen must advance: %v -> %v", before.LastSeen, s.LastSeen)
			}
		})
	}
}

func TestSessionClientContext_OversizeValuesClipped_Boundary(t *testing.T) {
	e := clientEnv(t.Context(), t)
	e.clock.Advance(time.Minute)
	ip, ua := strings.Repeat("1", 100), strings.Repeat("u", 400)
	if _, err := e.refreshWith(t, e.first.RefreshToken, identity.ClientInfo{IP: ip, UserAgent: ua}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := e.sessionRow(t.Context(), t, e.uid)
	if len(s.IP) != 64 || len(s.Device) != 256 {
		t.Fatalf("want clipped 64/256, got %d/%d", len(s.IP), len(s.Device))
	}
}

func TestSessionClientContext_ExactLimitNotClipped_Boundary(t *testing.T) {
	e := clientEnv(t.Context(), t)
	e.clock.Advance(time.Minute)
	ip, ua := strings.Repeat("1", 64), strings.Repeat("u", 256)
	if _, err := e.refreshWith(t, e.first.RefreshToken, identity.ClientInfo{IP: ip, UserAgent: ua}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := e.sessionRow(t.Context(), t, e.uid)
	if s.IP != ip || s.Device != ua {
		t.Fatalf("values at the limit must be stored whole, got %d/%d", len(s.IP), len(s.Device))
	}
}

func TestSessionClientContext_GraceReplayDoesNotUpdate_Boundary(t *testing.T) {
	e := clientEnv(t.Context(), t)
	e.clock.Advance(time.Minute)
	if _, err := e.refreshWith(t, e.first.RefreshToken, identity.ClientInfo{IP: "198.51.100.4", UserAgent: "AgoraTest/2"}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	before := e.sessionRow(t.Context(), t, e.uid)
	e.clock.Advance(2 * time.Second)
	res, err := e.refreshWith(t, e.first.RefreshToken, identity.ClientInfo{IP: "192.0.2.77", UserAgent: "AgoraTest/3"})
	if err != nil || res.RefreshToken != "" {
		t.Fatalf("grace replay: want success without new refresh token, got %+v err=%v", res, err)
	}
	s := e.sessionRow(t.Context(), t, e.uid)
	if s.IP != before.IP || s.Device != before.Device || !s.LastSeen.Equal(before.LastSeen) {
		t.Fatalf("grace replay must not change ip/device/last_seen: %+v -> %+v", before, s)
	}
}

func TestSessionClientContext_RejectedRefreshUpdatesNothing_Error(t *testing.T) {
	e := clientEnv(t.Context(), t)
	before := e.sessionRow(t.Context(), t, e.uid)
	e.clock.Advance(time.Minute)
	c := identity.ClientInfo{IP: "192.0.2.77", UserAgent: "Evil/9"}
	if _, err := e.refreshWith(t, "unknown-token-unknown-token-unknown-token0", c); err == nil {
		t.Fatalf("unknown token must be rejected")
	}
	s := e.sessionRow(t.Context(), t, e.uid)
	if s.IP != before.IP || s.Device != before.Device || !s.LastSeen.Equal(before.LastSeen) {
		t.Fatalf("rejected refresh must not change the row: %+v -> %+v", before, s)
	}
}
