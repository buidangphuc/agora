package store_test

// Task 2 of identity-user-enforcement: user status fields, ListUsers, SetStatus,
// LiftExpiredSuspension and the CreateSession status gate. Every test runs
// against the in-memory twins and against
// Postgres through the same assertions.
//
// Contract pinned by these tests (the implementer adds exactly this):
//
//	const StatusActive, StatusRestricted, StatusSuspended, StatusBanned = "active", ...
//	User gains: Status, StatusReason string; StatusUntil, StatusChangedAt, CreatedAt time.Time (zero = NULL)
//	Session gains: UserStatus string (status of the owning user read under lock by CreateSession; not persisted)
//	var ErrAccountLocked error                       // CreateSession on a suspended or banned user; nothing inserted
//	type StatusChange struct { UserID, PreviousStatus, Status, Reason string; Until, ChangedAt time.Time }
//	type StatusEnqueueFn func(StatusChange) (OutboxRow, error)
//	type StatusResult struct { User User; SessionsRevoked int }
//	ListUsers(ctx, query, status, cursor string, limit int) ([]User, string, error)
//	SetStatus(ctx, userID, status, reason string, until, now time.Time, revoke EnqueueFn, changed StatusEnqueueFn) (StatusResult, error)
//	LiftExpiredSuspension(ctx, userID string, now time.Time, changed StatusEnqueueFn) (bool, error)
//	(*InMemoryUserRepository).AttachSessions(*InMemorySessionRepository) // links both twins (sessions, outbox, status gate)

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

const (
	evRevoked = "t.SessionRevoked"
	evStatus  = "t.UserStatusChanged"
)

type enfUsers interface {
	store.UserRepository
	ListUsers(ctx context.Context, query, status, cursor string, limit int) ([]store.User, string, error)
	SetStatus(ctx context.Context, userID, status, reason string, until, now time.Time, revoke store.EnqueueFn, changed store.StatusEnqueueFn) (store.StatusResult, error)
	LiftExpiredSuspension(ctx context.Context, userID string, now time.Time, changed store.StatusEnqueueFn) (bool, error)
}

var (
	_ enfUsers = (*storetest.InMemoryUserRepository)(nil)
	_ enfUsers = (*store.PostgresUserRepository)(nil)
)

type enfEnv struct {
	name     string
	users    enfUsers
	sessions store.SessionRepository
	pool     *pgxpool.Pool // nil for the in-memory twin
	outbox   func(t *testing.T) []store.OutboxRow
	ordered  bool // outbox() preserves insertion order
}

func memEnv(t *testing.T) enfEnv {
	t.Helper()
	u := storetest.NewInMemoryUserRepository()
	s := storetest.NewInMemorySessionRepository()
	u.AttachSessions(s)
	return enfEnv{name: "memory", users: u, sessions: s, ordered: true,
		outbox: func(*testing.T) []store.OutboxRow { return s.OutboxRows() }}
}

func pgEnv(t *testing.T) enfEnv {
	t.Helper()
	pool := latestPool(t)
	ctx := context.Background()
	for _, q := range []string{`DELETE FROM identity_outbox_events`, `DELETE FROM users WHERE id LIKE 'ue-%'`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("reset (%s): %v", q, err)
		}
	}
	return enfEnv{name: "postgres", users: store.NewPostgresUserRepository(pool), sessions: store.NewPostgresSessionRepository(pool), pool: pool, ordered: true,
		outbox: func(t *testing.T) []store.OutboxRow {
			t.Helper()
			rows, err := pool.Query(ctx, `SELECT event_id, aggregate_type, aggregate_id, event_type, payload
				FROM identity_outbox_events ORDER BY seq`)
			if err != nil {
				t.Fatalf("read outbox: %v", err)
			}
			defer rows.Close()
			var out []store.OutboxRow
			for rows.Next() {
				var r store.OutboxRow
				if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &r.EventType, &r.Payload); err != nil {
					t.Fatalf("scan outbox: %v", err)
				}
				out = append(out, r)
			}
			return out
		}}
}

func forEachEnv(t *testing.T, fn func(t *testing.T, e enfEnv)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { fn(t, memEnv(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, pgEnv(t)) })
}

var enfNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func (e enfEnv) seedUser(t *testing.T, id, username string, roles ...string) store.User {
	t.Helper()
	if roles == nil {
		roles = []string{"buyer"}
	}
	u, err := e.users.Create(context.Background(), store.User{ID: id, Username: username, PasswordHash: "x", Roles: roles})
	if err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
	return u
}

func (e enfEnv) seedSessions(t *testing.T, userID string, n int) []store.Session {
	t.Helper()
	var out []store.Session
	for i := 0; i < n; i++ {
		s, err := e.sessions.CreateSession(context.Background(), store.Session{
			UserID: userID, Device: "d", IP: "1.1.1.1",
			RefreshHash: fmt.Sprintf("h-%s-%d", userID, i), RefreshExpiresAt: enfNow.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("seed session: %v", err)
		}
		out = append(out, s)
	}
	return out
}

func revokeBuilder(rows *[]store.Session) store.EnqueueFn {
	return func(s store.Session) (store.OutboxRow, error) {
		*rows = append(*rows, s)
		return store.OutboxRow{EventID: uuid.NewString(), AggregateType: "Session", AggregateID: s.UserID, EventType: evRevoked, Payload: []byte(s.ID)}, nil
	}
}

func statusBuilder(changes *[]store.StatusChange) store.StatusEnqueueFn {
	return func(c store.StatusChange) (store.OutboxRow, error) {
		*changes = append(*changes, c)
		return store.OutboxRow{EventID: uuid.NewString(), AggregateType: "User", AggregateID: c.UserID, EventType: evStatus,
			Payload: []byte(c.PreviousStatus + ">" + c.Status)}, nil
	}
}

func countType(rows []store.OutboxRow, typ string) int {
	n := 0
	for _, r := range rows {
		if r.EventType == typ {
			n++
		}
	}
	return n
}

func activeSessionCount(t *testing.T, e enfEnv, userID string) int {
	t.Helper()
	ss, err := e.sessions.ListSessions(context.Background(), userID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	n := 0
	for _, s := range ss {
		if !s.Revoked {
			n++
		}
	}
	return n
}

func mustGet(t *testing.T, e enfEnv, id string) store.User {
	t.Helper()
	u, err := e.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get user %s: %v", id, err)
	}
	return u
}

// Scenario: User enforcement: New user is active
func TestCreateUserDefaultsToActive(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		created := e.seedUser(t, "ue-new", "ue-new-user")
		for label, u := range map[string]store.User{"create result": created, "reloaded": mustGet(t, e, "ue-new")} {
			if u.Status != store.StatusActive {
				t.Errorf("%s: status = %q, want active", label, u.Status)
			}
			if u.StatusReason != "" {
				t.Errorf("%s: status_reason = %q, want empty", label, u.StatusReason)
			}
			if !u.StatusUntil.IsZero() {
				t.Errorf("%s: status_until = %v, want none", label, u.StatusUntil)
			}
		}
	})
}

// Scenario: User enforcement: One SessionRevoked per session then one UserStatusChanged
func TestSetUserStatusOutboxRows(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		e.seedUser(t, "ue-o1", "ue-o1")
		sess := e.seedSessions(t, "ue-o1", 3)
		until := enfNow.Add(48 * time.Hour)

		var revoked []store.Session
		var changes []store.StatusChange
		res, err := e.users.SetStatus(ctx, "ue-o1", store.StatusSuspended, "scam", until, enfNow, revokeBuilder(&revoked), statusBuilder(&changes))
		if err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		if res.SessionsRevoked != 3 {
			t.Errorf("SessionsRevoked = %d, want 3", res.SessionsRevoked)
		}
		if res.User.Status != store.StatusSuspended || res.User.StatusReason != "scam" || !res.User.StatusUntil.Equal(until) || !res.User.StatusChangedAt.Equal(enfNow) {
			t.Errorf("result user = %+v, want suspended/scam/until/changed_at=now", res.User)
		}
		got := mustGet(t, e, "ue-o1")
		if got.Status != store.StatusSuspended || got.StatusReason != "scam" || !got.StatusUntil.Equal(until) || !got.StatusChangedAt.Equal(enfNow) {
			t.Errorf("stored user = %+v", got)
		}

		if len(revoked) != 3 {
			t.Fatalf("revoke builder calls = %d, want 3", len(revoked))
		}
		seen := map[string]bool{}
		for _, s := range revoked {
			if !s.Revoked || s.RefreshHash != "" || s.PrevRefreshHash != "" {
				t.Errorf("builder got session %s not revoked/cleared: %+v", s.ID, s)
			}
			seen[s.ID] = true
		}
		for _, s := range sess {
			if !seen[s.ID] {
				t.Errorf("session %s never passed to revoke builder", s.ID)
			}
		}
		if len(changes) != 1 {
			t.Fatalf("status builder calls = %d, want 1", len(changes))
		}
		c := changes[0]
		if c.UserID != "ue-o1" || c.PreviousStatus != store.StatusActive || c.Status != store.StatusSuspended || c.Reason != "scam" || !c.Until.Equal(until) || !c.ChangedAt.Equal(enfNow) {
			t.Errorf("StatusChange = %+v", c)
		}

		rows := e.outbox(t)
		if len(rows) != 4 || countType(rows, evRevoked) != 3 || countType(rows, evStatus) != 1 {
			t.Fatalf("outbox = %d rows (%d revoked, %d status), want 4 (3, 1)", len(rows), countType(rows, evRevoked), countType(rows, evStatus))
		}
		if e.ordered {
			for i := 0; i < 3; i++ {
				if rows[i].EventType != evRevoked {
					t.Errorf("row %d type = %s, want SessionRevoked first", i, rows[i].EventType)
				}
			}
			if rows[3].EventType != evStatus {
				t.Errorf("last row type = %s, want UserStatusChanged last", rows[3].EventType)
			}
		}
		for _, r := range rows {
			if r.AggregateID != "ue-o1" {
				t.Errorf("row aggregate id = %q, want the user id", r.AggregateID)
			}
		}
		if n := activeSessionCount(t, e, "ue-o1"); n != 0 {
			t.Errorf("active sessions after suspend = %d, want 0", n)
		}
	})
}

// Scenario: User enforcement: Events keep per-user order
// Scenario: Both event types share one aggregate and keep order
// The revoke builder labels its rows "Session" and the status builder "User":
// the store must still put all four rows on the user aggregate, and the SDK
// relayer must publish the three revokes before the status record. The status
// event has the smallest id and the revokes the largest, so ordering by id
// instead of by insertion surfaces as a wrong order.
func TestSetUserStatusOutboxOrderPG(t *testing.T) {
	e := pgEnv(t)
	ctx := context.Background()
	e.seedUser(t, "ue-ord", "ue-ord")
	e.seedSessions(t, "ue-ord", 3)

	n := 0
	revoke := func(s store.Session) (store.OutboxRow, error) {
		n++
		return store.OutboxRow{EventID: fmt.Sprintf("ffffffff-%d", n), AggregateType: "Session", AggregateID: s.UserID, EventType: evRevoked, Payload: []byte(fmt.Sprintf("revoked-%d", n))}, nil
	}
	changed := func(c store.StatusChange) (store.OutboxRow, error) {
		return store.OutboxRow{EventID: "00000000-status", AggregateType: "User", AggregateID: c.UserID, EventType: evStatus, Payload: []byte("status")}, nil
	}
	if _, err := e.users.SetStatus(ctx, "ue-ord", store.StatusBanned, "scam", time.Time{}, enfNow, revoke, changed); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	var rows, users int
	if err := e.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE aggregate_type = 'User' AND aggregate_id = 'ue-ord') FROM identity_outbox_events`).Scan(&rows, &users); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 4 || users != 4 {
		t.Fatalf("rows = %d, of which on aggregate User/ue-ord = %d, want 4 and 4", rows, users)
	}

	got := relayValues(t, e.pool, 4)
	want := []string{"revoked-1", "revoked-2", "revoked-3", "status"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("relay order = %v, want %v", got, want)
	}
}

// Scenario: User enforcement: User without sessions still emits the status event
func TestSetUserStatusNoSessions(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		e.seedUser(t, "ue-ns", "ue-ns")
		var revoked []store.Session
		var changes []store.StatusChange
		res, err := e.users.SetStatus(context.Background(), "ue-ns", store.StatusBanned, "scam", time.Time{}, enfNow, revokeBuilder(&revoked), statusBuilder(&changes))
		if err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		rows := e.outbox(t)
		if res.SessionsRevoked != 0 || len(revoked) != 0 || countType(rows, evRevoked) != 0 {
			t.Errorf("revoked = %d / builder %d / rows %d, want 0", res.SessionsRevoked, len(revoked), countType(rows, evRevoked))
		}
		if countType(rows, evStatus) != 1 || len(rows) != 1 {
			t.Errorf("outbox = %d rows (%d status), want exactly 1 status row", len(rows), countType(rows, evStatus))
		}
		if got := mustGet(t, e, "ue-ns"); got.Status != store.StatusBanned {
			t.Errorf("status = %q, want banned", got.Status)
		}
	})
}

// Scenario: User enforcement: Already revoked sessions are not revoked twice
func TestSetUserStatusSkipsRevokedSessions(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		e.seedUser(t, "ue-sk", "ue-sk")
		sess := e.seedSessions(t, "ue-sk", 3)
		for _, s := range sess[:2] {
			if err := e.sessions.RevokeSession(ctx, s.ID, "ue-sk", nil); err != nil {
				t.Fatalf("pre-revoke: %v", err)
			}
		}
		var revoked []store.Session
		var changes []store.StatusChange
		res, err := e.users.SetStatus(ctx, "ue-sk", store.StatusBanned, "scam", time.Time{}, enfNow, revokeBuilder(&revoked), statusBuilder(&changes))
		if err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		rows := e.outbox(t)
		if res.SessionsRevoked != 1 || len(revoked) != 1 || revoked[0].ID != sess[2].ID || countType(rows, evRevoked) != 1 {
			t.Errorf("SessionsRevoked=%d builder=%d rows=%d, want exactly the one active session", res.SessionsRevoked, len(revoked), countType(rows, evRevoked))
		}
	})
}

// Scenario: User enforcement: Status change and revokes are atomic
func TestSetUserStatusRollsBackOnOutboxFailure(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name   string
		revoke func(calls *int) store.EnqueueFn
		fail   func() store.StatusEnqueueFn
	}{
		{"second revoke event fails", func(calls *int) store.EnqueueFn {
			return func(s store.Session) (store.OutboxRow, error) {
				*calls++
				if *calls == 2 {
					return store.OutboxRow{}, boom
				}
				return store.OutboxRow{EventID: uuid.NewString(), AggregateType: "Session", AggregateID: s.UserID, EventType: evRevoked, Payload: []byte(s.ID)}, nil
			}
		}, nil},
		{"status event fails", nil, func() store.StatusEnqueueFn {
			return func(store.StatusChange) (store.OutboxRow, error) { return store.OutboxRow{}, boom }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forEachEnv(t, func(t *testing.T, e enfEnv) {
				e.seedUser(t, "ue-rb", "ue-rb")
				e.seedSessions(t, "ue-rb", 3)
				calls := 0
				var revoke store.EnqueueFn = func(s store.Session) (store.OutboxRow, error) {
					return store.OutboxRow{EventID: uuid.NewString(), AggregateType: "Session", AggregateID: s.UserID, EventType: evRevoked, Payload: []byte(s.ID)}, nil
				}
				if tc.revoke != nil {
					revoke = tc.revoke(&calls)
				}
				var changes []store.StatusChange
				changed := statusBuilder(&changes)
				if tc.fail != nil {
					changed = tc.fail()
				}
				_, err := e.users.SetStatus(context.Background(), "ue-rb", store.StatusSuspended, "scam", enfNow.Add(time.Hour), enfNow, revoke, changed)
				if !errors.Is(err, boom) {
					t.Fatalf("err = %v, want the builder error", err)
				}
				if got := mustGet(t, e, "ue-rb"); got.Status != store.StatusActive || got.StatusReason != "" || !got.StatusUntil.IsZero() {
					t.Errorf("user changed despite failure: %+v", got)
				}
				if n := activeSessionCount(t, e, "ue-rb"); n != 3 {
					t.Errorf("active sessions = %d, want all 3 untouched", n)
				}
				ss, _ := e.sessions.ListSessions(context.Background(), "ue-rb")
				for _, s := range ss {
					if s.RefreshHash == "" {
						t.Errorf("session %s lost its refresh hash on rollback", s.ID)
					}
				}
				if rows := e.outbox(t); len(rows) != 0 {
					t.Errorf("outbox rows after rollback = %d, want 0", len(rows))
				}
			})
		})
	}
}

// Scenario: User enforcement: Restrict revokes sessions and keeps the user able to log in
// (repository half: restricted, suspended and banned all revoke; the user row is
// not the thing that blocks a restricted login)
func TestSetUserStatusRevokingTargets(t *testing.T) {
	for _, target := range []string{store.StatusRestricted, store.StatusSuspended, store.StatusBanned} {
		t.Run(target, func(t *testing.T) {
			forEachEnv(t, func(t *testing.T, e enfEnv) {
				e.seedUser(t, "ue-rt", "ue-rt")
				e.seedSessions(t, "ue-rt", 2)
				var revoked []store.Session
				var changes []store.StatusChange
				var until time.Time
				if target == store.StatusSuspended {
					until = enfNow.Add(time.Hour)
				}
				res, err := e.users.SetStatus(context.Background(), "ue-rt", target, "abuse", until, enfNow, revokeBuilder(&revoked), statusBuilder(&changes))
				if err != nil {
					t.Fatalf("SetStatus: %v", err)
				}
				if res.SessionsRevoked != 2 || activeSessionCount(t, e, "ue-rt") != 0 {
					t.Errorf("SessionsRevoked=%d active=%d, want 2 and 0", res.SessionsRevoked, activeSessionCount(t, e, "ue-rt"))
				}
				if got := mustGet(t, e, "ue-rt"); got.Status != target {
					t.Errorf("status = %q, want %q", got.Status, target)
				}
			})
		})
	}
}

// Scenario: User enforcement: Reinstating to active revokes nothing
func TestSetUserStatusActiveRevokesNothing(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		e.seedUser(t, "ue-ac", "ue-ac")
		var pre []store.Session
		var preChanges []store.StatusChange
		if _, err := e.users.SetStatus(ctx, "ue-ac", store.StatusSuspended, "scam", enfNow.Add(time.Hour), enfNow.Add(-time.Hour), revokeBuilder(&pre), statusBuilder(&preChanges)); err != nil {
			t.Fatalf("suspend: %v", err)
		}
		// A session that survived somehow (created after the suspension in this fixture).
		sess := e.seedSessionsRaw(t, "ue-ac")

		var revoked []store.Session
		var changes []store.StatusChange
		later := enfNow
		res, err := e.users.SetStatus(ctx, "ue-ac", store.StatusActive, "appeal_accepted", time.Time{}, later, revokeBuilder(&revoked), statusBuilder(&changes))
		if err != nil {
			t.Fatalf("reinstate: %v", err)
		}
		if res.SessionsRevoked != 0 || len(revoked) != 0 {
			t.Errorf("SessionsRevoked=%d builder=%d, want 0", res.SessionsRevoked, len(revoked))
		}
		got := mustGet(t, e, "ue-ac")
		if got.Status != store.StatusActive || got.StatusReason != "appeal_accepted" || !got.StatusUntil.IsZero() || !got.StatusChangedAt.Equal(later) {
			t.Errorf("stored user = %+v, want active/appeal_accepted/no until/changed_at=now", got)
		}
		if n := activeSessionCount(t, e, "ue-ac"); n != 1 {
			t.Errorf("active sessions = %d, want %d untouched", n, 1)
		}
		_ = sess
		rows := e.outbox(t)
		if countType(rows, evRevoked) != 0 || countType(rows, evStatus) != 2 { // suspend + reinstate
			t.Errorf("outbox: %d revoked, %d status; want 0 revoked and the 2 status rows", countType(rows, evRevoked), countType(rows, evStatus))
		}
		if len(changes) != 1 || changes[0].PreviousStatus != store.StatusSuspended || changes[0].Status != store.StatusActive || changes[0].Reason != "appeal_accepted" || !changes[0].Until.IsZero() {
			t.Errorf("StatusChange = %+v", changes)
		}
	})
}

// seedSessionsRaw inserts one active session bypassing the status gate, as a
// session that predates the restriction would look.
func (e enfEnv) seedSessionsRaw(t *testing.T, userID string) store.Session {
	t.Helper()
	ctx := context.Background()
	if e.pool != nil {
		id := uuid.NewString()
		if _, err := e.pool.Exec(ctx, `INSERT INTO sessions (id, user_id, device, ip) VALUES ($1, $2, 'd', '1.1.1.1')`, id, userID); err != nil {
			t.Fatalf("raw session: %v", err)
		}
		return store.Session{ID: id, UserID: userID}
	}
	// In-memory twin: SeedRaw stores the session as given, bypassing the status gate.
	return e.sessions.(*storetest.InMemorySessionRepository).SeedRaw(store.Session{ID: uuid.NewString(), UserID: userID, CreatedAt: enfNow, LastSeen: enfNow})
}

// Scenario: User enforcement: Setting the same status updates reason and until
func TestSetUserStatusSameStatusRevokesAgain(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		e.seedUser(t, "ue-sm", "ue-sm")
		var r1 []store.Session
		var c1 []store.StatusChange
		first := enfNow.Add(24 * time.Hour)
		if _, err := e.users.SetStatus(ctx, "ue-sm", store.StatusSuspended, "spam", first, enfNow, revokeBuilder(&r1), statusBuilder(&c1)); err != nil {
			t.Fatalf("first: %v", err)
		}
		e.seedSessionsRaw(t, "ue-sm")

		var r2 []store.Session
		var c2 []store.StatusChange
		second := enfNow.Add(72 * time.Hour)
		res, err := e.users.SetStatus(ctx, "ue-sm", store.StatusSuspended, "scam", second, enfNow.Add(time.Minute), revokeBuilder(&r2), statusBuilder(&c2))
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		if res.SessionsRevoked != 1 || len(r2) != 1 {
			t.Errorf("SessionsRevoked=%d builder=%d, want 1", res.SessionsRevoked, len(r2))
		}
		got := mustGet(t, e, "ue-sm")
		if got.Status != store.StatusSuspended || got.StatusReason != "scam" || !got.StatusUntil.Equal(second) {
			t.Errorf("stored user = %+v, want suspended/scam/until=second", got)
		}
		if len(c2) != 1 || c2[0].PreviousStatus != store.StatusSuspended || c2[0].Status != store.StatusSuspended {
			t.Errorf("StatusChange = %+v, want previous suspended", c2)
		}
		if rows := e.outbox(t); countType(rows, evRevoked) != 1 || countType(rows, evStatus) != 2 {
			t.Errorf("outbox: %d revoked, %d status; want 1 and 2 (first change + second)", countType(rows, evRevoked), countType(rows, evStatus))
		}
	})
}

// Scenario: User enforcement: Unknown user is NotFound without audit
func TestSetUserStatusUnknownUser(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		var revoked []store.Session
		var changes []store.StatusChange
		_, err := e.users.SetStatus(context.Background(), "ue-missing", store.StatusBanned, "scam", time.Time{}, enfNow, revokeBuilder(&revoked), statusBuilder(&changes))
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
		if len(changes) != 0 || len(e.outbox(t)) != 0 {
			t.Errorf("builders %d / outbox %d, want none", len(changes), len(e.outbox(t)))
		}
	})
}

func seedNamed(t *testing.T, e enfEnv, prefix string, names ...string) {
	t.Helper()
	for i, n := range names {
		e.seedUser(t, fmt.Sprintf("ue-%s-%d", prefix, i), n)
	}
}

func usernames(us []store.User) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Username
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Scenario: User enforcement: Find by username prefix
func TestListUsersPrefix(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		seedNamed(t, e, "px", "tran.c", "nguyen.b", "nguyen.a")
		got, next, err := e.users.ListUsers(context.Background(), "nguyen", "", "", 20)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if want := []string{"nguyen.a", "nguyen.b"}; !equalStrings(usernames(got), want) {
			t.Errorf("users = %v, want %v in order", usernames(got), want)
		}
		if next != "" {
			t.Errorf("next cursor = %q, want empty", next)
		}
	})
}

// Scenario: User enforcement: Find by exact id
func TestListUsersExactID(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		seedNamed(t, e, "id", "zeta.one", "zeta.two", "zeta.three")
		got, _, err := e.users.ListUsers(context.Background(), "ue-id-1", "", "", 20)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if len(got) != 1 || got[0].ID != "ue-id-1" || got[0].Username != "zeta.two" {
			t.Errorf("users = %+v, want exactly ue-id-1", got)
		}
	})
}

// Scenario: User enforcement: Filter by status
func TestListUsersStatusFilter(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		for i := 0; i < 10; i++ {
			e.seedUser(t, fmt.Sprintf("ue-sf-%02d", i), fmt.Sprintf("sf-user%02d", i))
		}
		until := enfNow.Add(24 * time.Hour)
		for _, id := range []string{"ue-sf-03", "ue-sf-07"} {
			var r []store.Session
			var c []store.StatusChange
			if _, err := e.users.SetStatus(ctx, id, store.StatusSuspended, "scam", until, enfNow, revokeBuilder(&r), statusBuilder(&c)); err != nil {
				t.Fatalf("suspend %s: %v", id, err)
			}
		}
		got, _, err := e.users.ListUsers(ctx, "sf-", store.StatusSuspended, "", 20)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if want := []string{"sf-user03", "sf-user07"}; !equalStrings(usernames(got), want) {
			t.Fatalf("users = %v, want %v", usernames(got), want)
		}
		for _, u := range got {
			if u.Status != store.StatusSuspended || u.StatusReason != "scam" || !u.StatusUntil.Equal(until) {
				t.Errorf("user %s = %+v, want suspended/scam/until", u.Username, u)
			}
		}
	})
}

// Scenario: User enforcement: Wildcards in the query are literal
func TestListUsersEscapesLikeWildcards(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		seedNamed(t, e, "wc", "%pct", "_und", `a\bs`, "abc", "aXc", "bcd")
		for _, tc := range []struct {
			q    string
			want []string
		}{
			{"%", []string{"%pct"}},
			{"_", []string{"_und"}},
			{`a\`, []string{`a\bs`}},
			{"a_c", nil},
			{"a%", nil}, // literal "a%" matches no username
		} {
			got, _, err := e.users.ListUsers(context.Background(), tc.q, "", "", 20)
			if err != nil {
				t.Fatalf("query %q failed: %v", tc.q, err)
			}
			names := usernames(got)
			sort.Strings(names)
			if !equalStrings(names, tc.want) && !(len(names) == 0 && len(tc.want) == 0) {
				t.Errorf("query %q = %v, want %v", tc.q, names, tc.want)
			}
		}
	})
}

// Scenario: User enforcement: Paging boundaries
func TestListUsersPaging(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		for i := 0; i < 101; i++ {
			e.seedUser(t, fmt.Sprintf("ue-pg-%03d", i), fmt.Sprintf("pg-user%03d", i))
		}
		p1, c1, err := e.users.ListUsers(ctx, "pg-", "", "", 100)
		if err != nil || len(p1) != 100 || c1 == "" {
			t.Fatalf("page 1: n=%d cursor=%q err=%v, want 100 and a cursor", len(p1), c1, err)
		}
		p2, c2, err := e.users.ListUsers(ctx, "pg-", "", c1, 100)
		if err != nil || len(p2) != 1 || c2 != "" {
			t.Fatalf("page 2: n=%d cursor=%q err=%v, want 1 and no cursor", len(p2), c2, err)
		}
		if p2[0].Username != "pg-user100" || p1[99].Username != "pg-user099" {
			t.Errorf("page boundary = %s | %s, want pg-user099 | pg-user100", p1[99].Username, p2[0].Username)
		}
		p3, c3, err := e.users.ListUsers(ctx, "pg-", "", "", 500)
		if err != nil || len(p3) != 100 || c3 == "" {
			t.Fatalf("clamped page: n=%d cursor=%q err=%v, want 100 and a cursor", len(p3), c3, err)
		}
		p4, _, err := e.users.ListUsers(ctx, "pg-", "", "", 0)
		if err != nil || len(p4) != 20 {
			t.Fatalf("default page: n=%d err=%v, want 20", len(p4), err)
		}
	})
}

// Scenario: Login enforcement: Expired suspension is lifted / Suspension ending exactly now is lifted
// (repository half: the conditional update and its outbox row)
func TestLiftExpiredSuspension(t *testing.T) {
	suspend := func(t *testing.T, e enfEnv, id string, until time.Time) {
		t.Helper()
		e.seedUser(t, id, id)
		var r []store.Session
		var c []store.StatusChange
		if _, err := e.users.SetStatus(context.Background(), id, store.StatusSuspended, "scam", until, enfNow.Add(-48*time.Hour), revokeBuilder(&r), statusBuilder(&c)); err != nil {
			t.Fatalf("suspend: %v", err)
		}
	}
	for _, tc := range []struct {
		name   string
		until  time.Time
		lifted bool
	}{
		{"until in the past", enfNow.Add(-time.Second), true},
		{"until exactly now", enfNow, true},
		{"until one second ahead", enfNow.Add(time.Second), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forEachEnv(t, func(t *testing.T, e enfEnv) {
				suspend(t, e, "ue-lf", tc.until)
				base := len(e.outbox(t))
				var changes []store.StatusChange
				lifted, err := e.users.LiftExpiredSuspension(context.Background(), "ue-lf", enfNow, statusBuilder(&changes))
				if err != nil {
					t.Fatalf("Lift: %v", err)
				}
				if lifted != tc.lifted {
					t.Fatalf("lifted = %v, want %v", lifted, tc.lifted)
				}
				got := mustGet(t, e, "ue-lf")
				rows := e.outbox(t)
				if tc.lifted {
					if got.Status != store.StatusActive || got.StatusReason != "" || !got.StatusUntil.IsZero() {
						t.Errorf("user after lift = %+v, want active with reason and until cleared", got)
					}
					if len(changes) != 1 || changes[0].PreviousStatus != store.StatusSuspended || changes[0].Status != store.StatusActive || changes[0].UserID != "ue-lf" || !changes[0].ChangedAt.Equal(enfNow) {
						t.Errorf("StatusChange = %+v", changes)
					}
					if len(rows) != base+1 {
						t.Errorf("outbox rows = %d, want %d (one UserStatusChanged)", len(rows), base+1)
					}
				} else {
					if got.Status != store.StatusSuspended || got.StatusReason != "scam" || got.StatusUntil.IsZero() {
						t.Errorf("user changed on a miss: %+v", got)
					}
					if len(changes) != 0 || len(rows) != base {
						t.Errorf("builder calls %d / new rows %d on a miss, want 0", len(changes), len(rows)-base)
					}
				}
			})
		})
	}
}

// Scenario: Login enforcement: A banned user is never lifted by time
func TestLiftExpiredSuspensionOnlyTouchesSuspended(t *testing.T) {
	for _, st := range []string{store.StatusBanned, store.StatusRestricted, store.StatusActive} {
		t.Run(st, func(t *testing.T) {
			forEachEnv(t, func(t *testing.T, e enfEnv) {
				e.seedUser(t, "ue-lo", "ue-lo")
				if st != store.StatusActive {
					var r []store.Session
					var c []store.StatusChange
					if _, err := e.users.SetStatus(context.Background(), "ue-lo", st, "scam", time.Time{}, enfNow.Add(-72*time.Hour), revokeBuilder(&r), statusBuilder(&c)); err != nil {
						t.Fatalf("set %s: %v", st, err)
					}
				}
				base := len(e.outbox(t))
				var changes []store.StatusChange
				lifted, err := e.users.LiftExpiredSuspension(context.Background(), "ue-lo", enfNow, statusBuilder(&changes))
				if err != nil || lifted {
					t.Fatalf("lifted=%v err=%v, want false,nil", lifted, err)
				}
				if got := mustGet(t, e, "ue-lo"); got.Status != st {
					t.Errorf("status = %q, want %q unchanged", got.Status, st)
				}
				if len(changes) != 0 || len(e.outbox(t)) != base {
					t.Errorf("event written on a miss")
				}
			})
		})
	}
}

// Scenario: Login enforcement: Failed lift fails the login closed (repository half)
func TestLiftExpiredSuspensionRollsBackOnEventFailure(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		e.seedUser(t, "ue-lr", "ue-lr")
		var r []store.Session
		var c []store.StatusChange
		if _, err := e.users.SetStatus(context.Background(), "ue-lr", store.StatusSuspended, "scam", enfNow.Add(-time.Hour), enfNow.Add(-48*time.Hour), revokeBuilder(&r), statusBuilder(&c)); err != nil {
			t.Fatalf("suspend: %v", err)
		}
		base := len(e.outbox(t))
		boom := errors.New("boom")
		lifted, err := e.users.LiftExpiredSuspension(context.Background(), "ue-lr", enfNow, func(store.StatusChange) (store.OutboxRow, error) { return store.OutboxRow{}, boom })
		if !errors.Is(err, boom) || lifted {
			t.Fatalf("lifted=%v err=%v, want false and the builder error", lifted, err)
		}
		if got := mustGet(t, e, "ue-lr"); got.Status != store.StatusSuspended {
			t.Errorf("status = %q, want suspended after the failed lift", got.Status)
		}
		if len(e.outbox(t)) != base {
			t.Errorf("outbox grew on a failed lift")
		}
	})
}

// Scenario: Login enforcement: Concurrent logins lift once (in-memory twin)
func TestLiftExpiredSuspensionOnceUnderConcurrency(t *testing.T) {
	liftOnceConcurrently(t, memEnv(t))
}

// Scenario: Login enforcement: Concurrent logins lift once
func TestLiftExpiredSuspensionOnceUnderConcurrencyPG(t *testing.T) {
	liftOnceConcurrently(t, pgEnv(t))
}

func liftOnceConcurrently(t *testing.T, e enfEnv) {
	t.Helper()
	ctx := context.Background()
	e.seedUser(t, "ue-cc", "ue-cc")
	var r []store.Session
	var c []store.StatusChange
	if _, err := e.users.SetStatus(ctx, "ue-cc", store.StatusSuspended, "scam", enfNow.Add(-time.Second), enfNow.Add(-48*time.Hour), revokeBuilder(&r), statusBuilder(&c)); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	base := countType(e.outbox(t), evStatus)

	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	var winners, failures int
	var changes []store.StatusChange
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var local []store.StatusChange
			lifted, err := e.users.LiftExpiredSuspension(ctx, "ue-cc", enfNow, statusBuilder(&local))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
			}
			if lifted {
				winners++
			}
			changes = append(changes, local...)
		}()
	}
	close(start)
	wg.Wait()
	if failures != 0 || winners != 1 {
		t.Fatalf("winners=%d failures=%d, want exactly 1 lift and no error (the others report lifted=false)", winners, failures)
	}
	if got := countType(e.outbox(t), evStatus) - base; got != 1 || len(changes) != 1 {
		t.Errorf("UserStatusChanged rows = %d, builder calls = %d, want 1 and 1", got, len(changes))
	}
	if got := mustGet(t, e, "ue-cc"); got.Status != store.StatusActive {
		t.Errorf("status = %q, want active", got.Status)
	}
}

// Scenario: Login enforcement: Ban committed during login leaves no live session (gate half)
func TestCreateSessionStatusGate(t *testing.T) {
	forEachEnv(t, func(t *testing.T, e enfEnv) {
		ctx := context.Background()
		for _, tc := range []struct {
			status string
			locked bool
		}{{store.StatusActive, false}, {store.StatusRestricted, false}, {store.StatusSuspended, true}, {store.StatusBanned, true}} {
			id := "ue-g-" + tc.status
			e.seedUser(t, id, id)
			if tc.status != store.StatusActive {
				var r []store.Session
				var c []store.StatusChange
				var until time.Time
				if tc.status == store.StatusSuspended {
					until = enfNow.Add(time.Hour)
				}
				if _, err := e.users.SetStatus(ctx, id, tc.status, "scam", until, enfNow, revokeBuilder(&r), statusBuilder(&c)); err != nil {
					t.Fatalf("set %s: %v", tc.status, err)
				}
			}
			s, err := e.sessions.CreateSession(ctx, store.Session{UserID: id, Device: "d", IP: "1.1.1.1", RefreshHash: "gate-" + tc.status, RefreshExpiresAt: enfNow.Add(time.Hour)})
			if tc.locked {
				if !errors.Is(err, store.ErrAccountLocked) {
					t.Errorf("%s: err = %v, want ErrAccountLocked", tc.status, err)
				}
				if ss, _ := e.sessions.ListSessions(ctx, id); len(ss) != 0 {
					t.Errorf("%s: %d session rows inserted, want 0", tc.status, len(ss))
				}
				continue
			}
			if err != nil {
				t.Errorf("%s: err = %v, want a session", tc.status, err)
				continue
			}
			if s.UserStatus != tc.status {
				t.Errorf("%s: store.Session.UserStatus = %q, want the status read at creation", tc.status, s.UserStatus)
			}
		}
	})
}

// Scenario: Login enforcement: Ban committed during login leaves no live session
// A ban transaction holds the user row (FOR UPDATE) and commits while a login's
// CreateSession is already waiting: the insert must be refused. The reverse
// order (session commits first) must be revoked by the ban.
func TestCreateSessionRechecksStatusUnderLockPG(t *testing.T) {
	e := pgEnv(t)
	ctx := context.Background()

	t.Run("ban commits before the insert", func(t *testing.T) {
		e.seedUser(t, "ue-race1", "ue-race1")
		banTx, err := e.pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = banTx.Rollback(ctx) }()
		if _, err := banTx.Exec(ctx, `SELECT 1 FROM users WHERE id = 'ue-race1' FOR UPDATE`); err != nil {
			t.Fatalf("lock user: %v", err)
		}

		done := make(chan error, 1)
		go func() {
			_, err := e.sessions.CreateSession(ctx, store.Session{UserID: "ue-race1", Device: "d", IP: "1.1.1.1", RefreshHash: "race-1", RefreshExpiresAt: enfNow.Add(time.Hour)})
			done <- err
		}()

		deadline := time.Now().Add(10 * time.Second)
		for waiting := 0; waiting == 0; {
			select {
			case err := <-done:
				t.Fatalf("CreateSession returned (%v) while the ban held the user row: it must wait on the user lock", err)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("CreateSession never waited on the user row lock")
			}
			if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
				WHERE wait_event_type = 'Lock' AND datname = current_database() AND pid <> pg_backend_pid()`).Scan(&waiting); err != nil {
				t.Fatalf("poll locks: %v", err)
			}
		}

		if _, err := banTx.Exec(ctx, `UPDATE users SET status='banned', status_reason='scam', status_changed_at=now() WHERE id='ue-race1'`); err != nil {
			t.Fatalf("ban: %v", err)
		}
		if err := banTx.Commit(ctx); err != nil {
			t.Fatalf("commit ban: %v", err)
		}
		if err := <-done; !errors.Is(err, store.ErrAccountLocked) {
			t.Fatalf("CreateSession err = %v, want ErrAccountLocked", err)
		}
		if ss, _ := e.sessions.ListSessions(ctx, "ue-race1"); len(ss) != 0 {
			t.Errorf("session rows after refused insert = %d, want 0", len(ss))
		}
	})

	t.Run("session commits before the ban", func(t *testing.T) {
		e.seedUser(t, "ue-race2", "ue-race2")
		if _, err := e.sessions.CreateSession(ctx, store.Session{UserID: "ue-race2", Device: "d", IP: "1.1.1.1", RefreshHash: "race-2", RefreshExpiresAt: enfNow.Add(time.Hour)}); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		var r []store.Session
		var c []store.StatusChange
		res, err := e.users.SetStatus(ctx, "ue-race2", store.StatusBanned, "scam", time.Time{}, enfNow, revokeBuilder(&r), statusBuilder(&c))
		if err != nil {
			t.Fatalf("ban: %v", err)
		}
		if res.SessionsRevoked != 1 || len(r) != 1 {
			t.Errorf("SessionsRevoked=%d builder=%d, want 1", res.SessionsRevoked, len(r))
		}
		if n := activeSessionCount(t, e, "ue-race2"); n != 0 {
			t.Errorf("active sessions after ban = %d, want 0", n)
		}
	})
}
