package store_test

// Task 1.3/1.4 (identity-legal-consent): consents ledger store.
//
// API assumed by these tests (the implementer must provide it):
//   - store.ConsentInput{Purpose string; Granted bool; Version, Source string}
//   - store.Consent{ID int64; UserID, Purpose string; Granted bool; Version, Source string; RecordedAt time.Time}
//   - (*PostgresUserRepository).AppendConsents(ctx, userID string, in []ConsentInput) error
//     inserts all rows in one transaction; recorded_at is the database time
//   - (*PostgresUserRepository).ListConsents(ctx, userID, cursor string, limit int) ([]Consent, string, error)
//     newest (recorded_at, id) first, only the user's rows, opaque keyset cursor, "" when no more pages
//   - (*PostgresUserRepository).CreateRegisteredWithConsents(ctx, u User, referralCode string,
//     consents []ConsentInput, enqueue RegisteredEnqueueFn) (User, error): user, outbox row and consent rows
//     (user id filled in by the store) in ONE transaction; any failure leaves no user row
//   - migration 0022_consents creates table consents(id, user_id, purpose, granted, version, source, recorded_at)

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buidangphuc/team-identity/internal/store"
)

func seedConsentUser(t *testing.T, e regEnv, id string) {
	t.Helper()
	if _, err := e.users.Create(context.Background(), store.User{ID: id, Username: "n-" + id, PasswordHash: "x", Roles: []string{"buyer"}}); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func consentBatch(n int, tag string) []store.ConsentInput {
	out := make([]store.ConsentInput, n)
	for i := range out {
		out[i] = store.ConsentInput{Purpose: "ANALYTICS", Granted: i%2 == 0, Version: fmt.Sprintf("%s-%d", tag, i), Source: "banner"}
	}
	return out
}

// Scenario: History is newest first and paged
func TestConsentListPagesTenTenFiveNewestFirst(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	seedConsentUser(t, e, "u-page")
	for i := 0; i < 5; i++ {
		if err := e.users.AppendConsents(ctx, "u-page", consentBatch(5, fmt.Sprintf("b%d", i))); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	var sizes []int
	var all []store.Consent
	cursor := ""
	for i := 0; i < 5; i++ {
		page, next, err := e.users.ListConsents(ctx, "u-page", cursor, 10)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		sizes = append(sizes, len(page))
		all = append(all, page...)
		if next == "" {
			break
		}
		cursor = next
	}
	if fmt.Sprint(sizes) != "[10 10 5]" {
		t.Fatalf("page sizes = %v, want [10 10 5]", sizes)
	}
	seen := map[int64]bool{}
	for i, c := range all {
		if seen[c.ID] {
			t.Fatalf("record %d repeated", c.ID)
		}
		seen[c.ID] = true
		if c.UserID != "u-page" || c.Source != "banner" || c.Purpose != "ANALYTICS" || c.RecordedAt.IsZero() {
			t.Fatalf("bad record %+v", c)
		}
		if i > 0 {
			p := all[i-1]
			if c.RecordedAt.After(p.RecordedAt) || (c.RecordedAt.Equal(p.RecordedAt) && c.ID >= p.ID) {
				t.Fatalf("not newest first at %d: %+v after %+v", i, c, p)
			}
		}
	}
	if len(seen) != 25 {
		t.Fatalf("distinct records = %d, want 25", len(seen))
	}
}

// Scenario: History is newest first and paged (ties in recorded_at broken by insertion order)
func TestConsentListTiesNewestInsertedFirst(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	seedConsentUser(t, e, "u-tie")
	if err := e.users.AppendConsents(ctx, "u-tie", consentBatch(6, "t")); err != nil {
		t.Fatalf("append: %v", err)
	}
	var got []string
	cursor := ""
	for {
		page, next, err := e.users.ListConsents(ctx, "u-tie", cursor, 4)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, c := range page {
			got = append(got, c.Version)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	want := "[t-5 t-4 t-3 t-2 t-1 t-0]"
	if fmt.Sprint(got) != want {
		t.Fatalf("order = %v, want %s", got, want)
	}
}

// Scenario: A user never sees another user's records
func TestConsentListIsolatedPerUser(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	seedConsentUser(t, e, "u-a")
	seedConsentUser(t, e, "u-b")
	if err := e.users.AppendConsents(ctx, "u-a", consentBatch(3, "a")); err != nil {
		t.Fatalf("append a: %v", err)
	}
	if err := e.users.AppendConsents(ctx, "u-b", consentBatch(1, "b")); err != nil {
		t.Fatalf("append b: %v", err)
	}
	got, next, err := e.users.ListConsents(ctx, "u-b", "", 20)
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	if len(got) != 1 || got[0].UserID != "u-b" || got[0].Version != "b-0" || next != "" {
		t.Fatalf("b sees %+v next=%q", got, next)
	}
	none, next, err := e.users.ListConsents(ctx, "u-nobody", "", 20)
	if err != nil || len(none) != 0 || next != "" {
		t.Fatalf("unknown user: %+v %q %v", none, next, err)
	}
}

// Scenario: Consent insert failure rolls back the account
func TestConsentInsertFailureRollsBackAccount(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `CREATE FUNCTION consents_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'consent write refused'; END $$`); err != nil {
		t.Fatalf("create fn: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `CREATE TRIGGER consents_fail BEFORE INSERT ON consents FOR EACH ROW EXECUTE FUNCTION consents_fail()`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	var seen []store.Registration
	_, err := e.users.CreateRegisteredWithConsents(ctx, store.User{ID: "u-cr-1", Username: "crname", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12",
		[]store.ConsentInput{
			{Purpose: "TERMS", Granted: true, Version: "2026-11", Source: "register"},
			{Purpose: "PRIVACY_POLICY", Granted: true, Version: "2026-11", Source: "register"},
		}, registeredBuilder(&seen))
	if err == nil {
		t.Fatal("consent insert failure must fail registration")
	}
	if n := e.count(t, `SELECT count(*) FROM users WHERE id='u-cr-1' OR username='crname'`); n != 0 {
		t.Fatalf("user row left behind: %d", n)
	}
	if n := e.registeredRows(t, "u-cr-1"); n != 0 {
		t.Fatalf("outbox rows left behind: %d", n)
	}
	if _, err := e.pool.Exec(ctx, `DROP TRIGGER consents_fail ON consents`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if _, err := e.users.CreateRegisteredWithConsents(ctx, store.User{ID: "u-cr-2", Username: "crname", PasswordHash: "x", Roles: []string{"buyer"}}, "AB12",
		[]store.ConsentInput{{Purpose: "TERMS", Granted: true, Version: "2026-11", Source: "register"}}, registeredBuilder(&seen)); err != nil {
		t.Fatalf("username must be registrable again: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM consents WHERE user_id='u-cr-2'`); n != 1 {
		t.Fatalf("consent rows for success = %d, want 1", n)
	}
}

// Scenario: Register with the current versions stores two consent records (store level, atomic success)
func TestConsentRegisteredWritesUserAndConsentsTogether(t *testing.T) {
	e := newRegEnv(t)
	ctx := context.Background()
	var seen []store.Registration
	if _, err := e.users.CreateRegisteredWithConsents(ctx, store.User{ID: "u-ok", Username: "okname", PasswordHash: "x", Roles: []string{"buyer"}}, "",
		[]store.ConsentInput{
			{Purpose: "TERMS", Granted: true, Version: "2026-11", Source: "register"},
			{Purpose: "PRIVACY_POLICY", Granted: true, Version: "2026-11", Source: "register"},
		}, registeredBuilder(&seen)); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, _, err := e.users.ListConsents(ctx, "u-ok", "", 20)
	if err != nil || len(got) != 2 {
		t.Fatalf("consents = %+v err=%v, want 2", got, err)
	}
	for _, c := range got {
		if c.UserID != "u-ok" || !c.Granted || c.Source != "register" || c.Version != "2026-11" {
			t.Fatalf("bad row %+v", c)
		}
	}
}

// Scenario: The consent ledger is append-only (no update/delete in repo code)
func TestConsentLedgerAppendOnlyInRepoCode(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob store sources: %v %d", err, len(files))
	}
	found := false
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ToLower(strings.Join(strings.Fields(string(b)), " "))
		if strings.Contains(s, "insert into consents") {
			found = true
		}
		for _, bad := range []string{"update consents", "delete from consents", "truncate consents"} {
			if strings.Contains(s, bad) {
				t.Fatalf("%s contains %q: the ledger is append-only", f, bad)
			}
		}
	}
	if !found {
		t.Fatal("no INSERT INTO consents in store sources")
	}
}

// Scenario: The consent ledger is append-only (index and shape from migration 0022)
func TestConsentMigrationCreatesLedgerWithIndex(t *testing.T) {
	e := newRegEnv(t)
	if n := e.count(t, `SELECT count(*) FROM information_schema.columns WHERE table_name='consents' AND column_name IN ('id','user_id','purpose','granted','version','source','recorded_at')`); n != 7 {
		t.Fatalf("consents columns = %d, want 7", n)
	}
	if n := e.count(t, `SELECT count(*) FROM pg_indexes WHERE tablename='consents' AND indexdef ILIKE '%user_id%recorded_at DESC%id DESC%'`); n != 1 {
		t.Fatalf("keyset index missing")
	}
}
