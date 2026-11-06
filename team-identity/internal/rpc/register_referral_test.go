package rpc_test

// Task 1.4 / 1.5 (identity-user-registered): the Register handler maps referral_code into the service.
//
// API assumed by these tests (same as internal/identity/register_referral_test.go): the user repository
// implements CreateRegistered(ctx, u store.User, referralCode string, enqueue store.RegisteredEnqueueFn)
// (store.User, error), and AuthService has RegisterWithReferral(ctx, username, password, role, referralCode).
// The handler calls RegisterWithReferral with RegisterRequest.referral_code.

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

type regSpyUsers struct {
	*storetest.InMemoryUserRepository
	mu    sync.Mutex
	codes []string
	rows  []store.OutboxRow
}

func newRegSpyUsers() *regSpyUsers {
	return &regSpyUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository()}
}

func (r *regSpyUsers) CreateRegistered(ctx context.Context, u store.User, code string, enqueue store.RegisteredEnqueueFn) (store.User, error) {
	out, err := r.Create(ctx, u)
	if err != nil {
		return store.User{}, err
	}
	row, err := enqueue(store.Registration{UserID: out.ID, ReferralCode: code, RegisteredAt: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		return store.User{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codes = append(r.codes, code)
	r.rows = append(r.rows, row)
	return out, nil
}

func (r *regSpyUsers) eventCode(t *testing.T, i int) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if i >= len(r.rows) {
		t.Fatalf("outbox rows = %d, want index %d", len(r.rows), i)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(r.rows[i].Payload, &env); err != nil {
		t.Fatal(err)
	}
	var ev identityv1.UserRegistered
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatal(err)
	}
	if env.GetType() != events.UserRegisteredEventType {
		t.Fatalf("type = %q", env.GetType())
	}
	return ev.GetReferralCode()
}

func shape(m protoreflect.Message, prefix string, out *[]string) {
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		name := prefix + string(fd.Name())
		*out = append(*out, name)
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			shape(v.Message(), name+".", out)
		}
		return true
	})
}

func shapeOf(msg proto.Message) []string {
	var out []string
	shape(msg.ProtoReflect(), "", &out)
	sort.Strings(out)
	return out
}

func newRegHandler(t *testing.T, logger *slog.Logger) (*rpc.AuthHandler, *regSpyUsers) {
	t.Helper()
	users := newRegSpyUsers()
	svc := newSvcWith(t, users, storetest.NewInMemorySessionRepository(), nopAudit{}, logger)
	return rpc.NewAuthHandler(svc).WithLogger(logger), users
}

// Scenario: Register with a well-formed code succeeds (handler maps referral_code into the service)
func TestHandlerRegisterMapsReferralCode(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"well-formed", "AB12cd_-", "AB12cd_-"},
		{"none", "", ""},
		{"malformed", "bad code;DROP TABLE", ""},
		{"trimmed", " AB12 ", "AB12"},
		{"non-ascii", "ABé12", ""},
		{"33 characters", strings.Repeat("a", 33), ""},
		{"32 characters", strings.Repeat("a", 32), strings.Repeat("a", 32)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, users := newRegHandler(t, discardLogger())
			_, err := h.Register(context.Background(), &identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer", ReferralCode: tc.in})
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			if got := users.eventCode(t, 0); got != tc.want {
				t.Fatalf("event referral_code = %q, want %q", got, tc.want)
			}
		})
	}
}

// Scenario: An unknown code never reveals itself (response shape identical for known, unknown, malformed and no code)
func TestHandlerRegisterResponseShapeIndependentOfCode(t *testing.T) {
	codes := []string{"", "KNOWN123", "NOSUCHCODE", "bad code;DROP TABLE", strings.Repeat("a", 33)}
	var base []string
	for i, code := range codes {
		h, _ := newRegHandler(t, discardLogger())
		resp, err := h.Register(context.Background(), &identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer", ReferralCode: code})
		if err != nil {
			t.Fatalf("Register(%q): %v", code, err)
		}
		got := shapeOf(resp)
		if i == 0 {
			base = got
			continue
		}
		if !reflect.DeepEqual(base, got) {
			t.Fatalf("response shape for %q = %v, want %v", code, got, base)
		}
		if resp.GetResult().GetToken() == "" || resp.GetResult().GetPrincipal().GetId() == "" {
			t.Fatalf("response lacks token or principal: %+v", resp)
		}
	}
}

// Scenario: A duplicate username writes no event (handler maps the conflict, no row)
func TestHandlerRegisterDuplicateWritesNoEvent(t *testing.T) {
	h, users := newRegHandler(t, discardLogger())
	ctx := context.Background()
	req := &identityv1.RegisterRequest{Username: "dup", Password: "password123", Role: "buyer", ReferralCode: "AB12"}
	if _, err := h.Register(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Register(ctx, req); err == nil {
		t.Fatal("duplicate must fail")
	}
	if len(users.rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(users.rows))
	}
}

// Scenario: Register logs contain no raw identifiers
func TestHandlerRegisterLogsNoRawIdentifiers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h, _ := newRegHandler(t, logger)
	const code = "REFa1B2c3D4"
	resp, err := h.Register(context.Background(), &identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer", ReferralCode: code})
	if err != nil {
		t.Fatal(err)
	}
	uid := resp.GetResult().GetPrincipal().GetId()
	out := buf.String()
	if strings.Contains(out, uid) {
		t.Errorf("raw user id leaked into log:\n%s", out)
	}
	if strings.Contains(out, code) {
		t.Errorf("referral code leaked into log:\n%s", out)
	}
}

// Scenario: A failed Register logs no raw identifiers either
func TestHandlerRegisterFailureLogsNoRawIdentifiers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h, _ := newRegHandler(t, logger)
	const code = "REFa1B2c3D4"
	first, err := h.Register(context.Background(), &identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer", ReferralCode: code})
	if err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if _, err := h.Register(context.Background(), &identityv1.RegisterRequest{Username: "alice", Password: "password123", Role: "buyer", ReferralCode: code}); err == nil {
		t.Fatal("duplicate must fail")
	}
	out := buf.String()
	if strings.Contains(out, code) {
		t.Errorf("referral code leaked into the error log:\n%s", out)
	}
	if strings.Contains(out, first.GetResult().GetPrincipal().GetId()) {
		t.Errorf("raw user id leaked into the error log:\n%s", out)
	}
}
