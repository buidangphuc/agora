package rpc_test

// Task 1.4 (identity-data-export): PrivacyService.ExportMyData.
//
// API assumed by these tests:
//   - rpc.ExportRepository is satisfied by anything with
//     ExportUserData(ctx, userID string) (store.UserExport, error); store.PostgresExportRepository is the real one.
//   - rpc.PrivacyHandler.WithExport(repo rpc.ExportRepository) enables the RPC; the store applies the 1000-row cap
//     and sets store.UserExport.SessionsTruncated / LoginHistoryTruncated, the handler copies those flags and only
//     adds the size shrink on top (halving login_history, flag set) until the document is at most 1 MiB.
//   - a connectivity error (net.Error) from the repository maps to UNAVAILABLE, any other error to INTERNAL,
//     and an error response carries no data.
//   - document keys: account{user_id, username, roles, status, created_at, seller_terms_version,
//     seller_terms_accepted_at}, addresses[{id,...}], consents[{purpose, granted, version, source, recorded_at}]
//     with purpose as the store's value (e.g. "ANALYTICS"), deletion{pending, erase_after}, truncated{login_history,
//     sessions}; timestamps RFC 3339 UTC.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
)

const exportMax = 1048576

var exportT0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

type fakeExport struct {
	mu    sync.Mutex
	data  map[string]store.UserExport
	calls int
	ids   []string
	err   error
}

func (f *fakeExport) ExportUserData(_ context.Context, userID string) (store.UserExport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.ids = append(f.ids, userID)
	if f.err != nil {
		return store.UserExport{}, f.err
	}
	d, ok := f.data[userID]
	if !ok {
		return store.UserExport{}, store.ErrNotFound
	}
	return d, nil
}

func exportEnv(t *testing.T, logger *slog.Logger, exports ...store.UserExport) (*rpc.PrivacyHandler, *fakeExport) {
	t.Helper()
	h, _ := newPrivacy(t, logger)
	f := &fakeExport{data: map[string]store.UserExport{}}
	for _, e := range exports {
		f.data[e.Account.UserID] = e
	}
	return h.WithExport(f), f
}

func baseExport(id string) store.UserExport {
	return store.UserExport{
		Account:      store.ExportAccount{UserID: id, Username: "name-" + id, Roles: []string{"buyer"}, Status: "ACTIVE", CreatedAt: exportT0},
		Addresses:    []store.Address{},
		Sessions:     []store.Session{},
		LoginHistory: []store.LoginEvent{},
		Consents:     []store.Consent{},
	}
}

func loginEvents(n, uaBytes int) []store.LoginEvent {
	out := make([]store.LoginEvent, n)
	ua := strings.Repeat("u", uaBytes)
	for i := range out {
		out[i] = store.LoginEvent{ID: fmt.Sprintf("ev-%d", i), IP: "203.0.113.9", UserAgent: ua, Success: true, CreatedAt: exportT0.Add(-time.Duration(i) * time.Minute)}
	}
	return out
}

func doExport(t *testing.T, h *rpc.PrivacyHandler, userID string) (*identityv1.ExportMyDataResponse, map[string]json.RawMessage) {
	t.Helper()
	resp, err := h.ExportMyData(privCtx(userID), &identityv1.ExportMyDataRequest{})
	if err != nil {
		t.Fatalf("ExportMyData: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(resp.GetData(), &doc); err != nil {
		t.Fatalf("data is not a JSON object: %v", err)
	}
	return resp, doc
}

func rawArr(t *testing.T, doc map[string]json.RawMessage, key string) []map[string]any {
	t.Helper()
	raw, ok := doc[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	if string(raw) == "null" {
		t.Fatalf("%q is null, want an array", key)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%q: %v", key, err)
	}
	return out
}

func rawObj(t *testing.T, doc map[string]json.RawMessage, key string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(doc[key], &out); err != nil {
		t.Fatalf("%q: %v", key, err)
	}
	return out
}

// Scenario: E1 Export lists every identity-held record of the caller
func TestExportMyDataListsAllRecords(t *testing.T) {
	e := baseExport("u-1")
	e.Addresses = []store.Address{{ID: "a-1", UserID: "u-1", City: "Hanoi"}, {ID: "a-2", UserID: "u-1", City: "Hue"}}
	e.Sessions = []store.Session{{ID: "s-1", CreatedAt: exportT0}, {ID: "s-2", CreatedAt: exportT0}, {ID: "s-3", CreatedAt: exportT0}}
	e.LoginHistory = loginEvents(5, 10)
	for i := 0; i < 4; i++ {
		e.Consents = append(e.Consents, store.Consent{ID: int64(i + 1), Purpose: "ANALYTICS", Version: "v", Source: "banner", RecordedAt: exportT0.Add(time.Duration(i) * time.Hour)})
	}
	h, _ := exportEnv(t, nil, e)
	resp, doc := doExport(t, h, "u-1")
	if resp.GetContentType() != "application/json; charset=utf-8" {
		t.Errorf("content_type = %q", resp.GetContentType())
	}
	for key, want := range map[string]int{"addresses": 2, "sessions": 3, "login_history": 5, "consents": 4} {
		if got := len(rawArr(t, doc, key)); got != want {
			t.Errorf("%s = %d, want %d", key, got, want)
		}
	}
	if got := rawObj(t, doc, "account")["user_id"]; got != "u-1" {
		t.Errorf("account.user_id = %v", got)
	}
	var ver int
	if err := json.Unmarshal(doc["schema_version"], &ver); err != nil || ver != 1 {
		t.Errorf("schema_version = %s", doc["schema_version"])
	}
	var gen string
	if err := json.Unmarshal(doc["generated_at"], &gen); err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, gen)
	if err != nil || !strings.HasSuffix(gen, "Z") {
		t.Fatalf("generated_at %q is not RFC 3339 UTC: %v", gen, err)
	}
	if got := resp.GetGeneratedAt().AsTime().Truncate(time.Second); !got.Equal(at.Truncate(time.Second)) || resp.GetGeneratedAt() == nil {
		t.Errorf("response generated_at %v differs from document %v", got, at)
	}
}

// Scenario: E2 Export never contains credentials
func TestExportMyDataExcludesCredentialsAndPinsKeys(t *testing.T) {
	e := baseExport("u-1")
	e.Sessions = []store.Session{{
		ID: "s-1", CreatedAt: exportT0, RefreshHash: "RTOKHASH-9f3a", PrevRefreshHash: "PREVHASH-77b1",
		RefreshExpiresAt: exportT0.Add(time.Hour), UserStatus: "STATUS-REASON-LEAK",
	}}
	e.Account.Username = "plain"
	h, _ := exportEnv(t, nil, e)
	resp, _ := doExport(t, h, "u-1")
	for _, secret := range []string{"RTOKHASH-9f3a", "PREVHASH-77b1", "password", "hash", "handoff", "status_reason", "STATUS-REASON-LEAK"} {
		if bytes.Contains(bytes.ToLower(resp.GetData()), bytes.ToLower([]byte(secret))) {
			t.Errorf("document contains %q", secret)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(resp.GetData()))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"schema_version", "generated_at", "account", "addresses", "sessions", "login_history", "consents", "deletion", "truncated"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("top-level keys = %v, want %v", keys, want)
	}
	var doc map[string]json.RawMessage
	_ = json.Unmarshal(resp.GetData(), &doc)
	acct := rawObj(t, doc, "account")
	wantAcct := []string{"created_at", "roles", "seller_terms_accepted_at", "seller_terms_version", "status", "user_id", "username"}
	var gotAcct []string
	for k := range acct {
		gotAcct = append(gotAcct, k)
	}
	if len(gotAcct) != len(wantAcct) {
		t.Errorf("account keys = %v, want %v", gotAcct, wantAcct)
	}
	for _, k := range wantAcct {
		if _, ok := acct[k]; !ok {
			t.Errorf("account lacks %q", k)
		}
	}
}

// Scenario: E3 Consents are listed newest first with deletion state
func TestExportMyDataConsentsNewestFirstAndDeletionState(t *testing.T) {
	t1 := exportT0
	t2 := exportT0.Add(3 * time.Hour)
	req := t2.Add(time.Hour)
	erase := req.Add(48 * time.Hour)
	e := baseExport("u-1")
	e.Consents = []store.Consent{
		{ID: 2, Purpose: "ANALYTICS", Granted: true, Version: "v", Source: "account", RecordedAt: t2},
		{ID: 1, Purpose: "TERMS", Granted: true, Version: "2026-11", Source: "register", RecordedAt: t1},
	}
	e.Account.DeletionRequestedAt = &req
	e.Account.EraseAfter = &erase
	h, _ := exportEnv(t, nil, e)
	_, doc := doExport(t, h, "u-1")
	cs := rawArr(t, doc, "consents")
	if len(cs) != 2 || cs[0]["purpose"] != "ANALYTICS" || cs[1]["purpose"] != "TERMS" {
		t.Fatalf("consents order = %v", cs)
	}
	if got, _ := time.Parse(time.RFC3339, fmt.Sprint(cs[0]["recorded_at"])); !got.Equal(t2) {
		t.Errorf("consents[0].recorded_at = %v", cs[0]["recorded_at"])
	}
	d := rawObj(t, doc, "deletion")
	if d["pending"] != true {
		t.Errorf("deletion.pending = %v", d["pending"])
	}
	if got, err := time.Parse(time.RFC3339, fmt.Sprint(d["erase_after"])); err != nil || !got.Equal(erase) {
		t.Errorf("deletion.erase_after = %v, want %v", d["erase_after"], erase)
	}
}

// Scenario: E4 A user with no addresses and no deletion gets empty collections
func TestExportMyDataEmptyAccount(t *testing.T) {
	e := baseExport("u-1")
	e.Addresses = nil
	e.Sessions = nil
	e.LoginHistory = nil
	e.Consents = nil
	h, _ := exportEnv(t, nil, e)
	_, doc := doExport(t, h, "u-1")
	for _, k := range []string{"addresses", "sessions", "login_history", "consents"} {
		if string(doc[k]) != "[]" {
			t.Errorf("%s = %s, want []", k, doc[k])
		}
	}
	d := rawObj(t, doc, "deletion")
	if len(d) != 2 || d["pending"] != false {
		t.Errorf("deletion = %v", d)
	}
	if v, ok := d["erase_after"]; !ok || v != nil {
		t.Errorf("deletion.erase_after = %v (present=%v), want null", v, ok)
	}
	tr := rawObj(t, doc, "truncated")
	if tr["login_history"] != false || tr["sessions"] != false {
		t.Errorf("truncated = %v", tr)
	}
}

// Scenario: E6 Service principal is refused (and anonymous, with no read)
func TestExportMyDataServicePrincipalDenied(t *testing.T) {
	h, f := exportEnv(t, nil, baseExport("u-1"))
	cases := map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"service":   {withPrincipal(context.Background(), &commonv1.Principal{Id: "u-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, Scopes: []string{"admin", "identity.read"}}), codes.PermissionDenied},
		"anonymous": {context.Background(), codes.Unauthenticated},
	}
	for name, c := range cases {
		resp, err := h.ExportMyData(c.ctx, &identityv1.ExportMyDataRequest{})
		if status.Code(err) != c.want {
			t.Errorf("%s: code = %v (%v), want %v", name, status.Code(err), err, c.want)
		}
		if len(resp.GetData()) != 0 {
			t.Errorf("%s: refused call returned data", name)
		}
	}
	if f.calls != 0 {
		t.Errorf("repository read %d times for refused callers", f.calls)
	}
}

// Scenario: E7 One user never sees another user's data
func TestExportMyDataIsolatedPerUser(t *testing.T) {
	a, b := baseExport("u-A"), baseExport("u-B")
	a.Addresses = []store.Address{{ID: "addr-A1", UserID: "u-A"}}
	b.Addresses = []store.Address{{ID: "addr-B1", UserID: "u-B"}, {ID: "addr-B2", UserID: "u-B"}}
	h, f := exportEnv(t, nil, a, b)
	resp, doc := doExport(t, h, "u-B")
	if len(f.ids) != 1 || f.ids[0] != "u-B" {
		t.Fatalf("repository asked for %v, want only the principal's id", f.ids)
	}
	if bytes.Contains(resp.GetData(), []byte("addr-A1")) || bytes.Contains(resp.GetData(), []byte("u-A")) {
		t.Error("document holds user A data")
	}
	if got := len(rawArr(t, doc, "addresses")); got != 2 {
		t.Errorf("addresses = %d, want 2", got)
	}
	if rawObj(t, doc, "account")["user_id"] != "u-B" {
		t.Error("account.user_id is not the caller")
	}
}

// Scenario: E8 A user with a pending deletion can still export
func TestExportMyDataAllowedWhileDeletionPending(t *testing.T) {
	req := exportT0
	erase := time.Now().Add(24 * time.Hour)
	e := baseExport("u-1")
	e.Account.DeletionRequestedAt = &req
	e.Account.EraseAfter = &erase
	h, _ := exportEnv(t, nil, e)
	_, doc := doExport(t, h, "u-1")
	if rawObj(t, doc, "deletion")["pending"] != true {
		t.Error("deletion.pending must be true")
	}
}

// Scenario: E9 Login history over the cap is truncated and flagged
func TestExportMyDataTruncatesLoginHistory(t *testing.T) {
	e := baseExport("u-1")
	e.LoginHistory = loginEvents(1000, 20)
	e.LoginHistoryTruncated = true
	h, _ := exportEnv(t, nil, e)
	resp, doc := doExport(t, h, "u-1")
	if got := len(rawArr(t, doc, "login_history")); got != 1000 {
		t.Errorf("login_history = %d, want 1000", got)
	}
	if rawObj(t, doc, "truncated")["login_history"] != true {
		t.Error("truncated.login_history must be true")
	}
	if len(resp.GetData()) > exportMax {
		t.Errorf("len(data) = %d", len(resp.GetData()))
	}
}

// Scenario: E9 sessions flag is carried over too
func TestExportMyDataTruncatesSessionsFlag(t *testing.T) {
	e := baseExport("u-1")
	e.SessionsTruncated = true
	h, _ := exportEnv(t, nil, e)
	_, doc := doExport(t, h, "u-1")
	tr := rawObj(t, doc, "truncated")
	if tr["sessions"] != true || tr["login_history"] != false {
		t.Errorf("truncated = %v", tr)
	}
}

// Scenario: E10 Exactly 1000 login events are not truncated
func TestExportMyDataCapBoundary(t *testing.T) {
	e := baseExport("u-1")
	e.LoginHistory = loginEvents(1000, 20)
	h, _ := exportEnv(t, nil, e)
	_, doc := doExport(t, h, "u-1")
	if got := len(rawArr(t, doc, "login_history")); got != 1000 {
		t.Errorf("login_history = %d, want 1000", got)
	}
	if rawObj(t, doc, "truncated")["login_history"] != false {
		t.Error("truncated.login_history must be false at exactly 1000")
	}
}

// Scenario: E11 Oversized document is shrunk below 1 MiB
func TestExportMyDataShrinksOversizedDocument(t *testing.T) {
	e := baseExport("u-1")
	e.LoginHistory = loginEvents(1000, 2000)
	h, _ := exportEnv(t, nil, e)
	resp, doc := doExport(t, h, "u-1")
	if len(resp.GetData()) > exportMax {
		t.Fatalf("len(data) = %d, want <= %d", len(resp.GetData()), exportMax)
	}
	got := rawArr(t, doc, "login_history")
	if len(got) == 0 || len(got) >= 1000 {
		t.Errorf("login_history = %d, want shrunk but non-empty", len(got))
	}
	if got[0]["id"] != "ev-0" {
		t.Errorf("shrink must keep the newest events first, got %v", got[0]["id"])
	}
	if rawObj(t, doc, "truncated")["login_history"] != true {
		t.Error("truncated.login_history must be true after shrinking")
	}
}

// Scenario: E13 A database error returns no partial file
func TestExportMyDataDatabaseErrorReturnsNoData(t *testing.T) {
	cases := map[string]struct {
		err  error
		want codes.Code
	}{
		"connection error": {fmt.Errorf("export login history: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}), codes.Unavailable},
		"other error":      {errors.New("export sessions: syntax error near SELECT secret_column"), codes.Internal},
	}
	for name, c := range cases {
		h, f := exportEnv(t, nil, baseExport("u-1"))
		f.err = c.err
		resp, err := h.ExportMyData(privCtx("u-1"), &identityv1.ExportMyDataRequest{})
		if status.Code(err) != c.want {
			t.Errorf("%s: code = %v (%v), want %v", name, status.Code(err), err, c.want)
		}
		if len(resp.GetData()) != 0 {
			t.Errorf("%s: data must be empty on error", name)
		}
		if err != nil && strings.Contains(err.Error(), "secret_column") {
			t.Errorf("%s: driver text leaked: %v", name, err)
		}
	}
}

// Scenario: E15 Logs carry no personal data
func TestExportMyDataLogsNoPersonalData(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e := baseExport("user-id-7f3c")
	e.Account.Username = "export_log_probe"
	for i := 0; i < 7; i++ {
		e.Addresses = append(e.Addresses, store.Address{ID: fmt.Sprintf("a%d", i), UserID: e.Account.UserID, Street: "12 Probe Street", RecipientName: "Probe Person", Phone: "0909000111"})
	}
	for i := 0; i < 11; i++ {
		e.Sessions = append(e.Sessions, store.Session{ID: fmt.Sprintf("s%d", i), IP: "198.51.100.23", Device: "ProbeDevice", CreatedAt: exportT0})
	}
	e.LoginHistory = loginEvents(13, 5)
	for i := 0; i < 3; i++ {
		e.Consents = append(e.Consents, store.Consent{ID: int64(i + 1), Purpose: "TERMS", Version: "v", Source: "register", RecordedAt: exportT0})
	}
	h, _ := exportEnv(t, logger, e)
	resp, _ := doExport(t, h, "user-id-7f3c")
	logs := buf.String()
	for _, banned := range []string{"user-id-7f3c", "export_log_probe", "Probe Street", "Probe Person", "0909000111", "198.51.100.23", "203.0.113.9", "ProbeDevice"} {
		if strings.Contains(logs, banned) {
			t.Errorf("logs contain %q", banned)
		}
	}
	for _, n := range []int{len(resp.GetData()), 7, 11, 13, 3} {
		if !regexp.MustCompile(`\b` + fmt.Sprint(n) + `\b`).MatchString(logs) {
			t.Errorf("logs lack the number %d: %s", n, logs)
		}
	}
}
