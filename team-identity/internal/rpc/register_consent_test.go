package rpc_test

// Task 1.5 (identity-legal-consent): Register validates terms_version/privacy_version against the first
// configured entry and writes two consent rows in the same repository call that creates the user.
//
// API assumed by these tests:
//   - identity.AuthService.WithLegalVersions(terms, privacy []string) *AuthService (first entry = current)
//   - the user repository implements
//     CreateRegisteredWithConsents(ctx, u store.User, referralCode string, consents []store.ConsentInput,
//     enqueue store.RegisteredEnqueueFn) (store.User, error)
//     (already implemented by PostgresUserRepository); Register must use it, never CreateRegistered, when
//     the repository offers it.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

type consentUsers struct {
	*storetest.InMemoryUserRepository
	mu       sync.Mutex
	consents map[string][]store.ConsentInput
	plain    int
}

func newConsentUsers() *consentUsers {
	return &consentUsers{InMemoryUserRepository: storetest.NewInMemoryUserRepository(), consents: map[string][]store.ConsentInput{}}
}

func (r *consentUsers) CreateRegistered(ctx context.Context, u store.User, code string, enqueue store.RegisteredEnqueueFn) (store.User, error) {
	r.mu.Lock()
	r.plain++
	r.mu.Unlock()
	return r.InMemoryUserRepository.CreateRegistered(ctx, u, code, enqueue)
}

func (r *consentUsers) CreateRegisteredWithConsents(ctx context.Context, u store.User, code string, in []store.ConsentInput, enqueue store.RegisteredEnqueueFn) (store.User, error) {
	out, err := r.InMemoryUserRepository.CreateRegistered(ctx, u, code, enqueue)
	if err != nil {
		return store.User{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consents[out.ID] = append([]store.ConsentInput(nil), in...)
	return out, nil
}

func (r *consentUsers) rowCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.consents {
		n += len(c)
	}
	return n
}

func consentRegisterHandler(t *testing.T, terms, privacy []string) (*rpc.AuthHandler, *consentUsers) {
	t.Helper()
	users := newConsentUsers()
	svc := newSvc(t, users, storetest.NewInMemorySessionRepository()).WithLegalVersions(terms, privacy)
	return rpc.NewAuthHandler(svc), users
}

func registerReq(name, terms, privacy string) *identityv1.RegisterRequest {
	return &identityv1.RegisterRequest{Username: name, Password: "password123", Role: "buyer", TermsVersion: terms, PrivacyVersion: privacy}
}

func assertNothingCreated(t *testing.T, users *consentUsers, name string) {
	t.Helper()
	if _, err := users.GetByUsername(context.Background(), name); err == nil {
		t.Fatalf("user %q exists after a rejected Register", name)
	}
	if n := users.rowCount(); n != 0 {
		t.Fatalf("%d consent rows written by a rejected Register", n)
	}
}

// Scenario: Register with the current versions stores two consent records
// Scenario: Registration needs no prior authentication
func TestRegisterWithCurrentVersionsStoresTwoConsentRecords(t *testing.T) {
	h, users := consentRegisterHandler(t, []string{"2026-11", "2026-05"}, []string{"p-9", "p-8"})
	// context.Background carries no principal: anonymous caller, no bearer token.
	res, err := h.Register(context.Background(), registerReq("alice", "2026-11", "p-9"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	uid := res.GetResult().GetPrincipal().GetId()
	if uid == "" {
		t.Fatal("no user id in result")
	}
	users.mu.Lock()
	got, plain := users.consents[uid], users.plain
	users.mu.Unlock()
	if plain != 0 {
		t.Fatalf("Register used CreateRegistered %d times; consents must be written by CreateRegisteredWithConsents", plain)
	}
	if len(got) != 2 {
		t.Fatalf("consent rows for the new user id = %d, want 2: %+v", len(got), got)
	}
	byPurpose := map[string]store.ConsentInput{}
	for _, c := range got {
		byPurpose[c.Purpose] = c
	}
	want := map[string]string{"TERMS": "2026-11", "PRIVACY_POLICY": "p-9"}
	for purpose, version := range want {
		c, ok := byPurpose[purpose]
		if !ok {
			t.Fatalf("no %s row: %+v", purpose, got)
		}
		if !c.Granted || c.Version != version || c.Source != "register" {
			t.Fatalf("%s row = %+v, want granted version %s source register", purpose, c, version)
		}
	}
	if len(users.consents) != 1 {
		t.Fatalf("consent rows belong to %d user ids, want only the new one", len(users.consents))
	}
}

// Scenario: Register without versions is rejected
func TestRegisterWithoutVersionsIsRejected(t *testing.T) {
	for name, tc := range map[string][2]string{
		"empty terms":     {"", "p-9"},
		"empty privacy":   {"2026-11", ""},
		"both empty":      {"", ""},
		"blank terms":     {"   ", "p-9"},
		"blank privacy":   {"2026-11", "  "},
		"padded terms":    {" 2026-11", "p-9"},
		"case differs":    {"2026-11", "P-9"},
		"suffix appended": {"2026-11x", "p-9"},
	} {
		t.Run(name, func(t *testing.T) {
			h, users := consentRegisterHandler(t, []string{"2026-11"}, []string{"p-9"})
			_, err := h.Register(context.Background(), registerReq("bob", tc[0], tc[1]))
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
			assertNothingCreated(t, users, "bob")
			_, err = h.Login(context.Background(), &identityv1.LoginRequest{Username: "bob", Password: "password123"})
			if err == nil {
				t.Fatal("login succeeded for a user whose Register was rejected")
			}
		})
	}
}

// Scenario: Register with a stale version is rejected
// Scenario: First entry is the current version (handler level)
func TestRegisterWithStaleVersionIsRejectedAndNamesField(t *testing.T) {
	h, users := consentRegisterHandler(t, []string{"2026-11", "2026-05"}, []string{"p-9", "p-8"})
	_, err := h.Register(context.Background(), registerReq("carol", "2026-05", "p-9"))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("older listed terms version: want InvalidArgument, got %v", err)
	}
	if !strings.Contains(status.Convert(err).Message()+detailFields(err), "terms_version") {
		t.Fatalf("error does not name terms_version: %v", err)
	}
	assertNothingCreated(t, users, "carol")

	_, err = h.Register(context.Background(), registerReq("carol", "2026-11", "p-8"))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("older listed privacy version: want InvalidArgument, got %v", err)
	}
	if !strings.Contains(status.Convert(err).Message()+detailFields(err), "privacy_version") {
		t.Fatalf("error does not name privacy_version: %v", err)
	}
	assertNothingCreated(t, users, "carol")

	if _, err := h.Register(context.Background(), registerReq("carol", "2026-11", "p-9")); err != nil {
		t.Fatalf("current versions after a rejected attempt: %v", err)
	}
}

func detailFields(err error) string {
	var b strings.Builder
	for _, d := range status.Convert(err).Details() {
		b.WriteString(strings.ToLower(strings.TrimSpace(stringOf(d))))
	}
	return b.String()
}

func stringOf(v any) string {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return ""
}
