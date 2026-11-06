package store_test

// Task 1.3 (identity-user-registered): PostgresUserRepository.CreateRegistered.
//
// API assumed by these tests (the implementer must provide it):
//   - store.Registration{UserID, ReferralCode string; RegisteredAt time.Time}
//   - store.RegisteredEnqueueFn func(store.Registration) (store.OutboxRow, error)
//   - (*PostgresUserRepository).CreateRegistered(ctx, u User, referralCode string, enqueue RegisteredEnqueueFn) (User, error)
//     inserts the user and enqueues the row built by enqueue in ONE transaction (aggregate type User,
//     aggregate id = user id); enqueue receives the new user id, the code as given and RegisteredAt
//     taken from the stored users.created_at; a unique violation returns ErrConflict and writes no row;
//     an enqueue or outbox insert failure rolls back the user insert and is not ErrConflict
//   - (*PostgresUserRepository).Create stays event-free (seed admin path)

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-identity/internal/store"
)

const evUserRegistered = "t.UserRegistered"

type regEnv struct {
	pool  *pgxpool.Pool
	users *store.PostgresUserRepository
}

func newRegEnv(t *testing.T) regEnv {
	t.Helper()
	pool := latestPool(t)
	return regEnv{pool: pool, users: store.NewPostgresUserRepository(pool)}
}

func (e regEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (e regEnv) registeredRows(t *testing.T, userID string) int {
	return e.count(t, `SELECT count(*) FROM identity_outbox_events WHERE aggregate_id=$1 AND event_type=$2`, userID, evUserRegistered)
}

func registeredBuilder(seen *[]store.Registration) store.RegisteredEnqueueFn {
	return func(r store.Registration) (store.OutboxRow, error) {
		*seen = append(*seen, r)
		return store.OutboxRow{
			EventID:     "evt-reg-" + r.UserID,
			AggregateID: r.UserID,
			EventType:   evUserRegistered,
			Payload:     []byte(r.ReferralCode),
			RequestID:   "req-reg",
		}, nil
	}
}

// Scenarios: One event per successful registration, The user row and the event commit together
func TestCreateRegisteredWritesUserAndEventTogether(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	var seen []store.Registration

	out, err := e.users.CreateRegistered(ctx, store.User{ID: "u-reg-1", Username: "reg1", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12cd", registeredBuilder(&seen))
	if err != nil {
		t.Fatalf("CreateRegistered: %v", err)
	}
	if out.ID != "u-reg-1" || out.CreatedAt.IsZero() {
		t.Fatalf("returned user: %+v", out)
	}
	if _, err := e.users.GetByID(ctx, "u-reg-1"); err != nil {
		t.Fatalf("user row missing: %v", err)
	}
	if n := e.registeredRows(t, "u-reg-1"); n != 1 {
		t.Fatalf("UserRegistered rows = %d, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM identity_outbox_events WHERE aggregate_id='u-reg-1' AND aggregate_type='User' AND status='pending' AND event_id='evt-reg-u-reg-1'`); n != 1 {
		t.Fatalf("pending User-aggregate row keyed by the user id = %d, want 1", n)
	}
	if len(seen) != 1 || seen[0].UserID != "u-reg-1" || seen[0].ReferralCode != "AB12cd" {
		t.Fatalf("enqueue saw %+v", seen)
	}
	if d := seen[0].RegisteredAt.Sub(out.CreatedAt); d < -5*time.Second || d > 5*time.Second {
		t.Fatalf("registered_at %v not within 5s of created_at %v", seen[0].RegisteredAt, out.CreatedAt)
	}
}

// Scenario: One event per successful registration (empty code still writes one event)
func TestCreateRegisteredEmptyCodeWritesEvent(t *testing.T) {
	e := newRegEnv(t)
	var seen []store.Registration
	if _, err := e.users.CreateRegistered(context.Background(), store.User{ID: "u-reg-2", Username: "reg2", PasswordHash: "x", Roles: []string{"buyer"}}, "", registeredBuilder(&seen)); err != nil {
		t.Fatalf("CreateRegistered: %v", err)
	}
	if n := e.registeredRows(t, "u-reg-2"); n != 1 {
		t.Fatalf("UserRegistered rows = %d, want 1", n)
	}
	if len(seen) != 1 || seen[0].ReferralCode != "" {
		t.Fatalf("enqueue saw %+v", seen)
	}
}

// Scenario: A duplicate username writes no event
func TestCreateRegisteredDuplicateUsernameWritesNoEvent(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	var seen []store.Registration
	if _, err := e.users.CreateRegistered(ctx, store.User{ID: "u-dup-1", Username: "dupname", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12", registeredBuilder(&seen)); err != nil {
		t.Fatalf("first: %v", err)
	}
	before := e.count(t, `SELECT count(*) FROM identity_outbox_events WHERE event_type=$1`, evUserRegistered)

	_, err := e.users.CreateRegistered(ctx, store.User{ID: "u-dup-2", Username: "dupname", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12", registeredBuilder(&seen))
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate err = %v, want store.ErrConflict", err)
	}
	if n := e.registeredRows(t, "u-dup-2"); n != 0 {
		t.Fatalf("rows for the rejected attempt = %d, want 0", n)
	}
	if after := e.count(t, `SELECT count(*) FROM identity_outbox_events WHERE event_type=$1`, evUserRegistered); after != before {
		t.Fatalf("outbox rows %d -> %d, want unchanged", before, after)
	}
	if n := e.count(t, `SELECT count(*) FROM users WHERE id='u-dup-2'`); n != 0 {
		t.Fatalf("rejected user row exists")
	}
}

// Scenario: The user row and the event commit together (enqueue failure rolls back the user)
func TestCreateRegisteredEnqueueFailureRollsBackUser(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	boom := errors.New("build event failed")
	_, err := e.users.CreateRegistered(ctx, store.User{ID: "u-rb-1", Username: "rb1", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12",
		func(store.Registration) (store.OutboxRow, error) { return store.OutboxRow{}, boom })
	if err == nil || errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want a non-conflict error", err)
	}
	if n := e.count(t, `SELECT count(*) FROM users WHERE id='u-rb-1'`); n != 0 {
		t.Fatalf("user row survived a failed enqueue: no account may exist without its event")
	}
}

// Scenario: The user row and the event commit together (outbox insert failure rolls back the user)
func TestCreateRegisteredOutboxInsertFailureRollsBackUser(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	_, err := e.users.CreateRegistered(ctx, store.User{ID: "u-rb-2", Username: "rb2", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12",
		func(r store.Registration) (store.OutboxRow, error) {
			return store.OutboxRow{EventID: "", AggregateID: r.UserID, EventType: evUserRegistered, Payload: nil}, nil
		})
	if err == nil {
		t.Fatal("an outbox row the store rejects must fail CreateRegistered")
	}
	if errors.Is(err, store.ErrConflict) {
		t.Fatalf("outbox failure must not look like a username conflict: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM users WHERE id='u-rb-2'`); n != 0 {
		t.Fatalf("user row survived a failed outbox insert")
	}
}

// Scenario: One event per successful registration (the seed-admin Create path emits nothing)
func TestCreateSeedAdminWritesNoUserRegistered(t *testing.T) {
	e := newRegEnv(t)
	before := e.count(t, `SELECT count(*) FROM identity_outbox_events`)
	if _, err := e.users.Create(context.Background(), store.User{ID: "u-seed-1", Username: "seedadmin", PasswordHash: "x", Roles: []string{"admin"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if after := e.count(t, `SELECT count(*) FROM identity_outbox_events`); after != before {
		t.Fatalf("Create wrote outbox rows: %d -> %d", before, after)
	}
}
