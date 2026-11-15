package store_test

// Task 1.2/1.3 (identity-data-export): read-only export store.
//
// API assumed by these tests (the implementer must provide it, in export.go):
//   - store.NewPostgresExportRepository(pool *pgxpool.Pool) *PostgresExportRepository
//   - (*PostgresExportRepository).ExportUserData(ctx, userID string) (store.UserExport, error)
//     reads in ONE read-only transaction, named columns only, any error returns the zero UserExport
//   - store.UserExport{
//       Account ExportAccount
//       Addresses []Address
//       Sessions []Session            // created_at DESC, at most 1000
//       LoginHistory []LoginEvent     // created_at DESC, at most 1000
//       Consents []Consent            // recorded_at DESC, id DESC, all rows
//       SessionsTruncated, LoginHistoryTruncated bool
//     }
//   - store.ExportAccount{UserID, Username string; Roles []string; Status string; CreatedAt time.Time;
//       SellerTermsVersion string; SellerTermsAcceptedAt, DeletionRequestedAt, EraseAfter *time.Time}
//   The export types carry no password hash, refresh hash, handoff code or status_reason field, and the
//   returned Sessions have empty RefreshHash and PrevRefreshHash.

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
)

var exportEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func seedLoginEvents(t testing.TB, e interface{ exec(string, ...any) }, user string, n int, agentLen int) {
	e.exec(`INSERT INTO login_history (id, user_id, ip, user_agent, success, created_at)
		SELECT $1 || '-lh-' || g, $1, '10.0.0.1', repeat('a', $3), true, $4::timestamptz + g * interval '1 second'
		FROM generate_series(1, $2) g`, user, n, agentLen, exportEpoch)
}

func newExportRepo(t testing.TB) (*store.PostgresExportRepository, func(q string, args ...any), func(q string, args ...any) int) {
	t.Helper()
	pool := latestPool(t)
	exec := func(q string, args ...any) { execAll(t, pool, q, args...) }
	count := func(q string, args ...any) int { return intOf(t, pool, q, args...) }
	return store.NewPostgresExportRepository(pool), exec, count
}

func seedExportUser(exec func(string, ...any), id string) {
	exec(`INSERT INTO users (id, username, password_hash, roles, status_reason) VALUES ($1, $1, 'pw-hash-secret', '{buyer}', 'status-reason-secret')`, id)
}

// Scenario: E1 Export lists every identity-held record of the caller
func TestExportStoreE1ListsAllRecordsNewestFirst(t *testing.T) {
	repo, exec, _ := newExportRepo(t)
	seedExportUser(exec, "u-e1")
	seedExportUser(exec, "u-other")
	for i := 1; i <= 2; i++ {
		exec(`INSERT INTO user_addresses (id, user_id, recipient_name, phone, street, city) VALUES ($1, 'u-e1', 'n', 'p', 's', 'c')`, fmt.Sprintf("a-e1-%d", i))
	}
	exec(`INSERT INTO user_addresses (id, user_id, recipient_name, phone, street, city) VALUES ('a-other', 'u-other', 'n', 'p', 's', 'c')`)
	exec(`INSERT INTO sessions (id, user_id, created_at, refresh_hash, prev_refresh_hash)
		SELECT 's-e1-' || g, 'u-e1', $1::timestamptz + g * interval '1 minute', 'rt-secret-' || g, 'prev-secret-' || g FROM generate_series(1, 3) g`, exportEpoch)
	exec(`INSERT INTO sessions (id, user_id) VALUES ('s-other', 'u-other')`)
	seedLoginEvents(t, adapter(exec), "u-e1", 5, 10)
	exec(`INSERT INTO login_history (id, user_id) VALUES ('lh-other', 'u-other')`)
	for i := 1; i <= 4; i++ {
		exec(`INSERT INTO consents (user_id, purpose, granted, version, source, recorded_at) VALUES ('u-e1', 'analytics', true, $1, 'banner', $2::timestamptz + $3 * interval '1 hour')`, fmt.Sprintf("v%d", i), exportEpoch, i)
	}
	exec(`INSERT INTO consents (user_id, purpose, granted, version, source) VALUES ('u-other', 'terms', true, 'v1', 'register')`)

	got, err := repo.ExportUserData(context.Background(), "u-e1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if got.Account.UserID != "u-e1" || got.Account.Username != "u-e1" || got.Account.Status != store.StatusActive {
		t.Fatalf("account = %+v", got.Account)
	}
	if len(got.Account.Roles) != 1 || got.Account.Roles[0] != "buyer" || got.Account.CreatedAt.IsZero() {
		t.Fatalf("account roles/created = %+v", got.Account)
	}
	if len(got.Addresses) != 2 || len(got.Sessions) != 3 || len(got.LoginHistory) != 5 || len(got.Consents) != 4 {
		t.Fatalf("counts a=%d s=%d l=%d c=%d, want 2/3/5/4", len(got.Addresses), len(got.Sessions), len(got.LoginHistory), len(got.Consents))
	}
	for _, a := range got.Addresses {
		if a.UserID != "u-e1" {
			t.Fatalf("foreign address %+v", a)
		}
	}
	if !sort.SliceIsSorted(got.Sessions, func(i, j int) bool { return got.Sessions[i].CreatedAt.After(got.Sessions[j].CreatedAt) }) || got.Sessions[0].ID != "s-e1-3" {
		t.Fatalf("sessions not newest first: %+v", got.Sessions)
	}
	if got.LoginHistory[0].ID != "u-e1-lh-5" || got.LoginHistory[4].ID != "u-e1-lh-1" {
		t.Fatalf("login history not newest first: %+v", got.LoginHistory)
	}
	if got.Consents[0].Version != "v4" || got.Consents[3].Version != "v1" {
		t.Fatalf("consents not newest first: %+v", got.Consents)
	}
	if got.SessionsTruncated || got.LoginHistoryTruncated {
		t.Fatalf("unexpected truncation flags")
	}
}

type execAdapter func(string, ...any)

func (f execAdapter) exec(q string, a ...any) { f(q, a...) }

func adapter(f func(string, ...any)) execAdapter { return execAdapter(f) }

// Scenario: E4 deletion and seller-terms state are carried
func TestExportStoreE1AccountCarriesDeletionAndSellerTerms(t *testing.T) {
	repo, exec, _ := newExportRepo(t)
	seedExportUser(exec, "u-del")
	req := exportEpoch.Add(time.Hour)
	exec(`UPDATE users SET deletion_requested_at = $2, erase_after = $3, seller_terms_version = 'V1', seller_terms_accepted_at = $2 WHERE id = $1`, "u-del", req, req.Add(48*time.Hour))
	seedExportUser(exec, "u-fresh")

	got, err := repo.ExportUserData(context.Background(), "u-del")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if got.Account.EraseAfter == nil || !got.Account.EraseAfter.Equal(req.Add(48*time.Hour)) {
		t.Fatalf("erase_after = %v", got.Account.EraseAfter)
	}
	if got.Account.DeletionRequestedAt == nil || !got.Account.DeletionRequestedAt.Equal(req) {
		t.Fatalf("deletion_requested_at = %v", got.Account.DeletionRequestedAt)
	}
	if got.Account.SellerTermsVersion != "V1" || got.Account.SellerTermsAcceptedAt == nil || !got.Account.SellerTermsAcceptedAt.Equal(req) {
		t.Fatalf("seller terms = %+v", got.Account)
	}
	fresh, err := repo.ExportUserData(context.Background(), "u-fresh")
	if err != nil {
		t.Fatalf("export fresh: %v", err)
	}
	if fresh.Account.EraseAfter != nil || fresh.Account.DeletionRequestedAt != nil || fresh.Account.SellerTermsAcceptedAt != nil || fresh.Account.SellerTermsVersion != "" {
		t.Fatalf("fresh account carries state: %+v", fresh.Account)
	}
	if fresh.Addresses == nil || fresh.Sessions == nil || fresh.LoginHistory == nil || fresh.Consents == nil {
		t.Fatalf("empty collections must be non-nil slices for a stable JSON []: %+v", fresh)
	}
}

// Scenario: E2 Export never contains credentials
func TestExportStoreE2NoCredentialsInExport(t *testing.T) {
	repo, exec, _ := newExportRepo(t)
	seedExportUser(exec, "u-e2")
	exec(`INSERT INTO sessions (id, user_id, refresh_hash, prev_refresh_hash, refresh_expires_at) VALUES ('s-e2', 'u-e2', 'rt-secret', 'prev-secret', now() + interval '1 day')`)
	exec(`INSERT INTO handoff_codes (code_hash, user_id, source_sid, target_app, expires_at) VALUES (decode(md5('h'), 'hex'), 'u-e2', 's-e2', 'seller', now() + interval '1 hour')`)

	got, err := repo.ExportUserData(context.Background(), "u-e2")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].RefreshHash != "" || got.Sessions[0].PrevRefreshHash != "" || !got.Sessions[0].RefreshExpiresAt.IsZero() {
		t.Fatalf("session leaks refresh state: %+v", got.Sessions)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "secret") {
		t.Fatalf("export value carries a secret: %+v", got)
	}
	banned := []string{"password", "statusreason", "refreshhash", "handoff", "codehash", "token"}
	var walk func(rt reflect.Type, path string)
	seen := map[reflect.Type]bool{}
	walk = func(rt reflect.Type, path string) {
		for rt.Kind() == reflect.Slice || rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || rt == reflect.TypeOf(time.Time{}) || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			name := strings.ToLower(f.Name)
			if rt == reflect.TypeOf(store.Session{}) && (f.Name == "RefreshHash" || f.Name == "PrevRefreshHash") {
				continue
			}
			for _, b := range banned {
				if strings.Contains(name, b) {
					t.Errorf("%s.%s is a credential-like field", path, f.Name)
				}
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(store.UserExport{}), "UserExport")
}

// Scenario: E9 Login history over the cap is truncated and flagged
func TestExportStoreE9LoginHistoryOverCapKeepsNewest1000(t *testing.T) {
	repo, exec, _ := newExportRepo(t)
	seedExportUser(exec, "u-e9")
	seedLoginEvents(t, adapter(exec), "u-e9", 1500, 20)

	got, err := repo.ExportUserData(context.Background(), "u-e9")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(got.LoginHistory) != 1000 || !got.LoginHistoryTruncated {
		t.Fatalf("len=%d truncated=%v, want 1000/true", len(got.LoginHistory), got.LoginHistoryTruncated)
	}
	if got.LoginHistory[0].ID != "u-e9-lh-1500" || got.LoginHistory[999].ID != "u-e9-lh-501" {
		t.Fatalf("not the newest 1000: first=%s last=%s", got.LoginHistory[0].ID, got.LoginHistory[999].ID)
	}
	if got.SessionsTruncated {
		t.Fatalf("sessions flag set without sessions")
	}
}

// Scenario: E10 Exactly 1000 login events are not truncated (1000/1001 boundary)
func TestExportStoreE10CapBoundary(t *testing.T) {
	for _, tc := range []struct {
		n         int
		wantLen   int
		wantTrunc bool
	}{{999, 999, false}, {1000, 1000, false}, {1001, 1000, true}} {
		t.Run(fmt.Sprintf("login_%d", tc.n), func(t *testing.T) {
			repo, exec, _ := newExportRepo(t)
			seedExportUser(exec, "u-e10")
			seedLoginEvents(t, adapter(exec), "u-e10", tc.n, 5)
			got, err := repo.ExportUserData(context.Background(), "u-e10")
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			if len(got.LoginHistory) != tc.wantLen || got.LoginHistoryTruncated != tc.wantTrunc {
				t.Fatalf("len=%d truncated=%v, want %d/%v", len(got.LoginHistory), got.LoginHistoryTruncated, tc.wantLen, tc.wantTrunc)
			}
		})
	}
}

// Scenario: E10 sessions share the 1000 cap and their own flag
func TestExportStoreE10SessionsCapBoundary(t *testing.T) {
	for _, tc := range []struct {
		n         int
		wantLen   int
		wantTrunc bool
	}{{1000, 1000, false}, {1001, 1000, true}} {
		t.Run(fmt.Sprintf("sessions_%d", tc.n), func(t *testing.T) {
			repo, exec, _ := newExportRepo(t)
			seedExportUser(exec, "u-s")
			exec(`INSERT INTO sessions (id, user_id, created_at) SELECT 's-' || g, 'u-s', $2::timestamptz + g * interval '1 second' FROM generate_series(1, $1) g`, tc.n, exportEpoch)
			got, err := repo.ExportUserData(context.Background(), "u-s")
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			if len(got.Sessions) != tc.wantLen || got.SessionsTruncated != tc.wantTrunc {
				t.Fatalf("len=%d truncated=%v, want %d/%v", len(got.Sessions), got.SessionsTruncated, tc.wantLen, tc.wantTrunc)
			}
			if got.Sessions[0].ID != fmt.Sprintf("s-%d", tc.n) {
				t.Fatalf("first = %s, want newest", got.Sessions[0].ID)
			}
			if got.LoginHistoryTruncated {
				t.Fatalf("login flag set without login rows")
			}
		})
	}
}

// Scenario: E13 A database error returns no partial file
func TestExportStoreE13ErrorReturnsZeroExport(t *testing.T) {
	for name, breakIt := range map[string]func(exec func(string, ...any)){
		"login_history read fails": func(exec func(string, ...any)) { exec(`ALTER TABLE login_history RENAME TO login_history_gone`) },
		"consents read fails":      func(exec func(string, ...any)) { exec(`ALTER TABLE consents RENAME TO consents_gone`) },
	} {
		t.Run(name, func(t *testing.T) {
			repo, exec, _ := newExportRepo(t)
			seedExportUser(exec, "u-e13")
			exec(`INSERT INTO user_addresses (id, user_id, recipient_name, phone, street, city) VALUES ('a13', 'u-e13', 'n', 'p', 's', 'c')`)
			breakIt(exec)
			got, err := repo.ExportUserData(context.Background(), "u-e13")
			if err == nil {
				t.Fatalf("want error")
			}
			if !reflect.DeepEqual(got, store.UserExport{}) {
				t.Fatalf("partial export returned with error: %+v", got)
			}
		})
	}
}

// Scenario: E13 connectivity failure returns an error and no data
func TestExportStoreE13ClosedPoolFails(t *testing.T) {
	pool := latestPool(t)
	execAll(t, pool, `INSERT INTO users (id, username, password_hash, roles) VALUES ('u-c', 'u-c', 'x', '{buyer}')`)
	repo := store.NewPostgresExportRepository(pool)
	pool.Close()
	got, err := repo.ExportUserData(context.Background(), "u-c")
	if err == nil || !reflect.DeepEqual(got, store.UserExport{}) {
		t.Fatalf("err=%v got=%+v, want error and zero export", err, got)
	}
}

// Scenario: E13 unknown user is an error, not an empty document
func TestExportStoreE13UnknownUserIsNotFound(t *testing.T) {
	repo, _, _ := newExportRepo(t)
	got, err := repo.ExportUserData(context.Background(), "nobody")
	if err == nil || !reflect.DeepEqual(got, store.UserExport{}) {
		t.Fatalf("err=%v got=%+v, want error and zero export", err, got)
	}
}

// Scenario: E12 Latency for the reference account
func TestExportMyDataLatencyP95(t *testing.T) {
	repo, exec, _ := newExportRepo(t)
	seedExportUser(exec, "u-ref")
	exec(`INSERT INTO user_addresses (id, user_id, recipient_name, phone, street, ward, district, city)
		SELECT 'ref-a-' || g, 'u-ref', 'name', '0900000000', 'street', 'ward', 'district', 'city' FROM generate_series(1, 20) g`)
	exec(`INSERT INTO sessions (id, user_id, device, ip, created_at) SELECT 'ref-s-' || g, 'u-ref', 'dev', '10.0.0.1', $1::timestamptz + g * interval '1 second' FROM generate_series(1, 200) g`, exportEpoch)
	seedLoginEvents(t, adapter(exec), "u-ref", 1000, 120)
	exec(`INSERT INTO consents (user_id, purpose, granted, version, source) SELECT 'u-ref', 'analytics', g % 2 = 0, 'v' || g, 'banner' FROM generate_series(1, 12) g`)

	const runs = 200
	d := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		got, err := repo.ExportUserData(context.Background(), "u-ref")
		d = append(d, time.Since(start))
		if err != nil {
			t.Fatalf("export %d: %v", i, err)
		}
		if i == 0 && (len(got.Addresses) != 20 || len(got.Sessions) != 200 || len(got.LoginHistory) != 1000 || len(got.Consents) != 12) {
			t.Fatalf("reference account counts a=%d s=%d l=%d c=%d", len(got.Addresses), len(got.Sessions), len(got.LoginHistory), len(got.Consents))
		}
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	if p95 := d[runs*95/100-1]; p95 > 500*time.Millisecond {
		t.Fatalf("p95 = %v, want <= 500ms", p95)
	}
}
