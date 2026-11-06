package identity_test

// Task 1.2 / 1.9 (identity-user-registered): referral code normalisation in Register.
//
// API assumed by these tests (the implementer must provide it):
//   - (*AuthService).RegisterWithReferral(ctx, username, password, role, referralCode string) (AuthResult, error)
//     and Register(ctx, username, password, role) keeps working and behaves as RegisterWithReferral(..., "")
//   - store.Registration{UserID, ReferralCode string; RegisteredAt time.Time}
//   - store.RegisteredEnqueueFn func(store.Registration) (store.OutboxRow, error)
//   - the user repository passed to NewAuthService implements
//     CreateRegistered(ctx, u store.User, referralCode string, enqueue store.RegisteredEnqueueFn) (store.User, error)
//     with the semantics of PostgresUserRepository.CreateRegistered (store.ErrConflict on a duplicate
//     username with no outbox row; enqueue is called once, after the user is accepted, with
//     Registration{UserID: u.ID, ReferralCode: referralCode, RegisteredAt: <store time>})
//   - the code handed to CreateRegistered is already normalised: trimmed, kept only when it matches
//     ^[A-Za-z0-9_-]{1,32}$ (case kept), otherwise ""
//   - the enqueue closure the service builds returns an OutboxRow with EventType
//     events.UserRegisteredEventType, EventID a uuid equal to the envelope event_id, AggregateID == user id and a
//     Payload produced by events.BuildUserRegisteredEnvelope
//   - EnsureAdmin keeps using Create and emits no UserRegistered event

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

var regAt = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

type regUsers struct {
	*storetest.InMemoryUserRepository
	mu      sync.Mutex
	codes   []string
	created []store.User
	rows    []store.OutboxRow
	plain   int
}

func newRegUsers() *regUsers {
	return &regUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository()}
}

func (r *regUsers) Create(ctx context.Context, u store.User) (store.User, error) {
	r.mu.Lock()
	r.plain++
	r.mu.Unlock()
	return r.InMemoryUserRepository.Create(ctx, u)
}

func (r *regUsers) CreateRegistered(ctx context.Context, u store.User, code string, enqueue store.RegisteredEnqueueFn) (store.User, error) {
	out, err := r.InMemoryUserRepository.Create(ctx, u)
	if err != nil {
		return store.User{}, err
	}
	row, err := enqueue(store.Registration{UserID: out.ID, ReferralCode: code, RegisteredAt: regAt})
	if err != nil {
		return store.User{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codes = append(r.codes, code)
	r.created = append(r.created, out)
	r.rows = append(r.rows, row)
	return out, nil
}

func (r *regUsers) lastCode(t *testing.T) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.codes) == 0 {
		t.Fatal("repository CreateRegistered was never called")
	}
	return r.codes[len(r.codes)-1]
}

func decodeRow(t *testing.T, row store.OutboxRow) (*eventsv1.EventEnvelope, *identityv1.UserRegistered) {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var ev identityv1.UserRegistered
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return &env, &ev
}

func registerWith(t *testing.T, code string) (*regUsers, string) {
	t.Helper()
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	res, err := svc.RegisterWithReferral(context.Background(), "alice", "password123", "buyer", code)
	if err != nil {
		t.Fatalf("RegisterWithReferral(%q): %v", code, err)
	}
	return users, res.UserID
}

func payloadCode(t *testing.T, users *regUsers) string {
	t.Helper()
	users.mu.Lock()
	defer users.mu.Unlock()
	if len(users.rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(users.rows))
	}
	_, ev := decodeRow(t, users.rows[0])
	return ev.GetReferralCode()
}

// Scenarios: Register with a well-formed code succeeds, Register without a code succeeds,
// A malformed code is dropped, not rejected, Surrounding whitespace is trimmed before validation,
// A code that is empty after trimming becomes empty, A non-ASCII character is dropped,
// Case is preserved, Boundary length of 32 versus 33 characters
func TestRegisterNormalisesReferralCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"well-formed with underscore and dash", "AB12cd_-", "AB12cd_-"},
		{"empty", "", ""},
		{"malformed space and punctuation", "bad code;DROP TABLE", ""},
		{"surrounding spaces trimmed", " AB12 ", "AB12"},
		{"surrounding tab and newline trimmed", "\tAB12\n", "AB12"},
		{"only spaces", "   ", ""},
		{"non-ascii e acute", "ABé12", ""},
		{"case preserved", "REFa1B2c3D4", "REFa1B2c3D4"},
		{"single character", "a", "a"},
		{"32 characters", strings.Repeat("a", 32), strings.Repeat("a", 32)},
		{"33 characters", strings.Repeat("a", 33), ""},
		{"32 characters padded with spaces", "  " + strings.Repeat("Z", 32) + "  ", strings.Repeat("Z", 32)},
		{"inner space", "AB 12", ""},
		{"sql quote", "AB'12", ""},
		{"dot is outside the charset", "AB.12", ""},
		{"unicode digit", "AB١٢", ""},
		{"null byte", "AB\x0012", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			users, _ := registerWith(t, tc.in)
			if got := users.lastCode(t); got != tc.want {
				t.Fatalf("code handed to the store = %q, want %q", got, tc.want)
			}
			if got := payloadCode(t, users); got != tc.want {
				t.Fatalf("UserRegistered referral_code = %q, want %q", got, tc.want)
			}
		})
	}
}

// Scenario: Register without a code succeeds (the legacy four-argument Register still works)
func TestRegisterWithoutCodeStillEmitsEmptyCode(t *testing.T) {
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	res, err := svc.Register(context.Background(), "bob", "password123", "buyer")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Token == "" || res.UserID == "" {
		t.Fatalf("result: %+v", res)
	}
	if got := payloadCode(t, users); got != "" {
		t.Fatalf("referral_code = %q, want empty", got)
	}
}

// Scenario: One event per successful registration (service builds the envelope for the new user)
func TestRegisterBuildsUserRegisteredRow(t *testing.T) {
	users, uid := registerWith(t, "AB12cd")
	users.mu.Lock()
	defer users.mu.Unlock()
	row := users.rows[0]
	env, ev := decodeRow(t, row)
	if row.EventType != events.UserRegisteredEventType || env.GetType() != events.UserRegisteredEventType {
		t.Fatalf("event type = %q / %q", row.EventType, env.GetType())
	}
	if row.AggregateID != uid || ev.GetUserId() != uid {
		t.Fatalf("aggregate %q / payload user %q, want %q", row.AggregateID, ev.GetUserId(), uid)
	}
	if _, err := uuid.Parse(row.EventID); err != nil {
		t.Fatalf("event id %q is not a uuid: %v", row.EventID, err)
	}
	if env.GetEventId() != row.EventID {
		t.Fatalf("envelope event_id %q != row event id %q", env.GetEventId(), row.EventID)
	}
	if !ev.GetRegisteredAt().AsTime().Equal(regAt) {
		t.Fatalf("registered_at = %v, want %v", ev.GetRegisteredAt().AsTime(), regAt)
	}
	if ev.GetReferralCode() != "AB12cd" {
		t.Fatalf("referral_code = %q", ev.GetReferralCode())
	}
}

// Scenario: One event per successful registration (a second registration gets a distinct event id)
func TestRegisterEventIDsAreDistinct(t *testing.T) {
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	for _, name := range []string{"u1", "u2"} {
		if _, err := svc.RegisterWithReferral(context.Background(), name, "password123", "buyer", "AB12"); err != nil {
			t.Fatal(err)
		}
	}
	if len(users.rows) != 2 || users.rows[0].EventID == users.rows[1].EventID {
		t.Fatalf("rows: %+v", users.rows)
	}
}

// Scenario: A duplicate username writes no event
func TestRegisterDuplicateUsernameWritesNoEvent(t *testing.T) {
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	ctx := context.Background()
	if _, err := svc.RegisterWithReferral(ctx, "dup", "password123", "buyer", "AB12"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RegisterWithReferral(ctx, "dup", "password123", "buyer", "AB12")
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second Register err = %v, want store.ErrConflict", err)
	}
	if len(users.rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(users.rows))
	}
}

// Scenario: A client-supplied user id or role escalation is ignored
func TestRegisterRoleAdminIgnored(t *testing.T) {
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	res, err := svc.RegisterWithReferral(context.Background(), "mallory", "password123", "admin", "AB12")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(users.created) != 1 {
		t.Fatalf("users created = %d", len(users.created))
	}
	u := users.created[0]
	if len(u.Roles) != 1 || u.Roles[0] != "buyer" {
		t.Fatalf("roles = %v, want [buyer]", u.Roles)
	}
	if _, err := uuid.Parse(u.ID); err != nil || res.UserID != u.ID {
		t.Fatalf("user id %q (result %q) must be a server-generated uuid", u.ID, res.UserID)
	}
	_, ev := decodeRow(t, users.rows[0])
	if ev.GetUserId() != u.ID {
		t.Fatalf("event user_id = %q, want %q", ev.GetUserId(), u.ID)
	}
}

// Scenario: Register with a well-formed code succeeds (invalid username or password still rejected, no event)
func TestRegisterInvalidInputWritesNoEvent(t *testing.T) {
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	if _, err := svc.RegisterWithReferral(context.Background(), " ", "password123", "buyer", "AB12"); err == nil {
		t.Fatal("blank username must be rejected")
	}
	if _, err := svc.RegisterWithReferral(context.Background(), "ok", "abc", "buyer", "AB12"); err == nil {
		t.Fatal("short password must be rejected")
	}
	if len(users.rows) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(users.rows))
	}
}

// Scenario: One event per successful registration (the seed-admin path emits nothing)
func TestEnsureAdminEmitsNoUserRegistered(t *testing.T) {
	users := newRegUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository())
	if err := svc.EnsureAdmin(context.Background(), "root", "password123"); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}
	if len(users.rows) != 0 || len(users.created) != 0 {
		t.Fatalf("seed admin went through the registration path: rows=%d created=%d", len(users.rows), len(users.created))
	}
	if users.plain != 1 {
		t.Fatalf("plain Create calls = %d, want 1", users.plain)
	}
	if _, err := users.GetByUsername(context.Background(), "root"); err != nil {
		t.Fatalf("admin not stored: %v", err)
	}
}
