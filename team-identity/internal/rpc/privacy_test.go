package rpc_test

// Tasks 1.6 / 1.7 (identity-legal-consent): PrivacyService.RecordConsent and ListMyConsents.
//
// API assumed by these tests:
//   - rpc.NewPrivacyHandler(repo rpc.ConsentRepository, terms config.LegalTerms, privacy config.PrivacyPolicy,
//     seller config.SellerTerms, logger *slog.Logger) *rpc.PrivacyHandler, registered through rpc.Handlers.Privacy
//   - rpc.ConsentRepository has the two methods of PostgresUserRepository:
//     AppendConsents(ctx, userID, []store.ConsentInput) error and
//     ListConsents(ctx, userID, cursor string, limit int) ([]store.Consent, nextCursor string, error)
//   - the store reports a malformed cursor as a plain error whose text starts with "invalid cursor"; any other
//     repository error is a server fault (Internal) and must not leak its text.
//   - the handler passes the clamped page size (default 20, max 100) as limit; the store handles the extra row.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
)

type fakeConsents struct {
	mu       sync.Mutex
	rows     []store.Consent
	nextID   int64
	appends  int
	lastLim  int
	failList error
	failApp  error
}

func (f *fakeConsents) AppendConsents(_ context.Context, userID string, in []store.ConsentInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appends++
	if f.failApp != nil {
		return f.failApp
	}
	for _, c := range in {
		f.nextID++
		f.rows = append(f.rows, store.Consent{
			ID: f.nextID, UserID: userID, Purpose: c.Purpose, Granted: c.Granted, Version: c.Version, Source: c.Source,
			RecordedAt: time.Now(),
		})
	}
	return nil
}

func (f *fakeConsents) seed(userID string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := 0; i < n; i++ {
		f.nextID++
		f.rows = append(f.rows, store.Consent{
			ID: f.nextID, UserID: userID, Purpose: "ANALYTICS", Granted: i%2 == 0, Version: "v" + strconv.Itoa(i), Source: "banner",
			RecordedAt: time.Date(2026, 11, 1, 0, 0, i, 0, time.UTC),
		})
	}
}

func (f *fakeConsents) ListConsents(_ context.Context, userID, cursor string, limit int) ([]store.Consent, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastLim = limit
	if f.failList != nil {
		return nil, "", f.failList
	}
	start := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 {
			return nil, "", errors.New("invalid cursor")
		}
		start = n
	}
	var mine []store.Consent
	for i := len(f.rows) - 1; i >= 0; i-- {
		if f.rows[i].UserID == userID {
			mine = append(mine, f.rows[i])
		}
	}
	if start >= len(mine) {
		return nil, "", nil
	}
	end := start + limit
	next := ""
	if end < len(mine) {
		next = strconv.Itoa(end)
	} else {
		end = len(mine)
	}
	return mine[start:end], next, nil
}

func (f *fakeConsents) forUser(userID string) []store.Consent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Consent
	for _, r := range f.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeConsents) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

const (
	curTerms   = "2026-11"
	curPrivacy = "p-9"
)

func newPrivacy(t *testing.T, logger *slog.Logger) (*rpc.PrivacyHandler, *fakeConsents) {
	t.Helper()
	repo := &fakeConsents{}
	h := rpc.NewPrivacyHandler(repo,
		config.LegalTerms{Versions: []string{curTerms, "2026-05"}},
		config.PrivacyPolicy{Versions: []string{curPrivacy, "p-8"}},
		config.SellerTerms{Versions: []string{"s-1", "s-0"}},
		logger)
	return h, repo
}

func privCtx(id string) context.Context {
	return withPrincipal(context.Background(), userPrincipal(id))
}

func rec(p identityv1.ConsentPurpose, granted bool, version, source string) *identityv1.ConsentRecord {
	return &identityv1.ConsentRecord{Purpose: p, Granted: granted, Version: version, Source: source}
}

const (
	pAnalytics = identityv1.ConsentPurpose_CONSENT_PURPOSE_ANALYTICS
	pMarketing = identityv1.ConsentPurpose_CONSENT_PURPOSE_MARKETING
	pPersonal  = identityv1.ConsentPurpose_CONSENT_PURPOSE_PERSONALIZATION
	pTerms     = identityv1.ConsentPurpose_CONSENT_PURPOSE_TERMS
	pPrivacy   = identityv1.ConsentPurpose_CONSENT_PURPOSE_PRIVACY_POLICY
	pSeller    = identityv1.ConsentPurpose_CONSENT_PURPOSE_SELLER_TERMS
)

// Scenario: Banner choice stores one row per changed purpose
func TestPrivacyRecordConsentBannerStoresOneRowPerPurpose(t *testing.T) {
	h, _ := newPrivacy(t, discardLogger())
	ctx := privCtx("u-1")
	if _, err := h.RecordConsent(ctx, &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{
		rec(pAnalytics, true, "banner-1", "banner"),
		rec(pMarketing, false, "banner-1", "banner"),
	}}); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}
	resp, err := h.ListMyConsents(ctx, &identityv1.ListMyConsentsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[identityv1.ConsentPurpose]*identityv1.ConsentRecord{}
	for _, r := range resp.GetRecords() {
		got[r.GetPurpose()] = r
	}
	if len(resp.GetRecords()) != 2 || got[pAnalytics] == nil || got[pMarketing] == nil || got[pPersonal] != nil {
		t.Fatalf("records = %v, want exactly ANALYTICS and MARKETING", resp.GetRecords())
	}
	if !got[pAnalytics].GetGranted() || got[pMarketing].GetGranted() {
		t.Fatalf("granted flags wrong: %v", resp.GetRecords())
	}
	for _, r := range resp.GetRecords() {
		if r.GetSource() != "banner" || r.GetRecordedAt() == nil {
			t.Fatalf("record %v lacks source banner or recorded_at", r)
		}
	}
}

// Scenario: Re-accepting a new Terms version appends a row
func TestPrivacyReacceptingTermsAppendsRow(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	repo.mu.Lock()
	repo.nextID++
	repo.rows = append(repo.rows, store.Consent{ID: repo.nextID, UserID: "u-2", Purpose: "TERMS", Granted: true, Version: "2026-05", Source: "register", RecordedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)})
	repo.mu.Unlock()
	ctx := privCtx("u-2")
	if _, err := h.RecordConsent(ctx, &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{rec(pTerms, true, curTerms, "account")}}); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}
	resp, err := h.ListMyConsents(ctx, &identityv1.ListMyConsentsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetRecords()) != 2 {
		t.Fatalf("want 2 TERMS rows, got %v", resp.GetRecords())
	}
	if first := resp.GetRecords()[0]; first.GetVersion() != curTerms || first.GetPurpose() != pTerms {
		t.Fatalf("newest row = %v, want TERMS %s", first, curTerms)
	}
	if old := resp.GetRecords()[1]; old.GetVersion() != "2026-05" {
		t.Fatalf("earlier row = %v, want version 2026-05 kept", old)
	}
}

// Scenario: Banner choice stores one row per changed purpose (accepted legal and seller records)
func TestPrivacyRecordConsentAcceptsCurrentLegalAndListedSellerTerms(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	_, err := h.RecordConsent(privCtx("u-3"), &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{
		rec(pTerms, true, curTerms, "account"),
		rec(pPrivacy, true, curPrivacy, "account"),
		rec(pSeller, true, "s-0", "seller_onboarding"),
		rec(pPersonal, false, strings.Repeat("v", 32), "register"),
	}})
	if err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}
	if n := len(repo.forUser("u-3")); n != 4 {
		t.Fatalf("rows = %d, want 4", n)
	}
}

// Scenario: Invalid records are rejected without partial writes
func TestPrivacyInvalidRecordsRejectedWithoutPartialWrites(t *testing.T) {
	good := rec(pAnalytics, true, "v1", "banner")
	seven := make([]*identityv1.ConsentRecord, 0, 7)
	for _, p := range []identityv1.ConsentPurpose{pAnalytics, pMarketing, pPersonal, pTerms, pPrivacy, pSeller} {
		seven = append(seven, rec(p, true, curTerms, "banner"))
	}
	seven = append(seven, rec(pAnalytics, true, "v", "banner"))
	for name, recs := range map[string][]*identityv1.ConsentRecord{
		"seven records":         seven,
		"duplicate purpose":     {good, rec(pAnalytics, false, "v2", "banner")},
		"unspecified purpose":   {good, rec(identityv1.ConsentPurpose_CONSENT_PURPOSE_UNSPECIFIED, true, "v1", "banner")},
		"unknown enum value":    {good, rec(identityv1.ConsentPurpose(99), true, "v1", "banner")},
		"33 char version":       {good, rec(pMarketing, true, strings.Repeat("v", 33), "banner")},
		"empty version":         {good, rec(pMarketing, true, "", "banner")},
		"unknown source":        {good, rec(pMarketing, true, "v1", "popup")},
		"empty source":          {good, rec(pMarketing, true, "v1", "")},
		"empty list":            {},
		"nil record in list":    {good, nil},
		"source is case-strict": {good, rec(pMarketing, true, "v1", "Banner")},
	} {
		t.Run(name, func(t *testing.T) {
			h, repo := newPrivacy(t, discardLogger())
			_, err := h.RecordConsent(privCtx("u-4"), &identityv1.RecordConsentRequest{Records: recs})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
			if repo.total() != 0 {
				t.Fatalf("%d rows written by a rejected call", repo.total())
			}
		})
	}
}

// Scenario: Invalid records are rejected without partial writes (boundaries accepted)
func TestPrivacyBoundaryRecordCountsAreAccepted(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	six := []*identityv1.ConsentRecord{
		rec(pAnalytics, true, "v", "account"), rec(pMarketing, true, "v", "account"), rec(pPersonal, true, "v", "account"),
		rec(pTerms, true, curTerms, "account"), rec(pPrivacy, true, curPrivacy, "account"), rec(pSeller, true, "s-1", "account"),
	}
	if _, err := h.RecordConsent(privCtx("u-5"), &identityv1.RecordConsentRequest{Records: six}); err != nil {
		t.Fatalf("six records: %v", err)
	}
	if _, err := h.RecordConsent(privCtx("u-5"), &identityv1.RecordConsentRequest{Records: six[:1]}); err != nil {
		t.Fatalf("one record: %v", err)
	}
	if n := len(repo.forUser("u-5")); n != 7 {
		t.Fatalf("rows = %d, want 7", n)
	}
}

// Scenario: Stale or withdrawn legal acceptance is rejected
func TestPrivacyStaleOrWithdrawnLegalAcceptanceRejected(t *testing.T) {
	ok := rec(pAnalytics, true, "v1", "banner")
	for name, bad := range map[string]*identityv1.ConsentRecord{
		"stale terms":              rec(pTerms, true, "2026-05", "account"),
		"unlisted terms":           rec(pTerms, true, "1999-01", "account"),
		"terms withdrawn":          rec(pTerms, false, curTerms, "account"),
		"stale privacy":            rec(pPrivacy, true, "p-8", "account"),
		"privacy withdrawn":        rec(pPrivacy, false, curPrivacy, "account"),
		"seller terms not listed":  rec(pSeller, true, "s-9", "seller_onboarding"),
		"seller terms withdrawn":   rec(pSeller, false, "s-1", "seller_onboarding"),
		"terms uses privacy value": rec(pTerms, true, curPrivacy, "account"),
	} {
		t.Run(name, func(t *testing.T) {
			h, repo := newPrivacy(t, discardLogger())
			_, err := h.RecordConsent(privCtx("u-6"), &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{ok, bad}})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
			if repo.total() != 0 {
				t.Fatalf("%d rows written, including for the valid record in the same call", repo.total())
			}
		})
	}
}

// Scenario: Client time and user id are ignored
func TestPrivacyClientTimeIgnoredAndUserFromPrincipal(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	r := rec(pAnalytics, true, "v1", "banner")
	r.RecordedAt = timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	before := time.Now()
	if _, err := h.RecordConsent(privCtx("u-7"), &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{r}}); err != nil {
		t.Fatal(err)
	}
	rows := repo.forUser("u-7")
	if len(rows) != 1 || repo.total() != 1 {
		t.Fatalf("rows for principal = %d, total = %d, want 1 and 1", len(rows), repo.total())
	}
	if rows[0].RecordedAt.Before(before.Add(-time.Second)) {
		t.Fatalf("stored recorded_at %v is client time, want server time", rows[0].RecordedAt)
	}
	resp, err := h.ListMyConsents(privCtx("u-7"), &identityv1.ListMyConsentsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetRecords()[0].GetRecordedAt().AsTime(); got.Year() == 2000 || time.Since(got) > 5*time.Second {
		t.Fatalf("listed recorded_at = %v, want within 5s of now", got)
	}
}

// Scenario: Anonymous and service principals cannot record consent
// Scenario: Anonymous caller cannot list
func TestPrivacyAnonymousAndServicePrincipalsRefused(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	repo.seed("u-8", 1)
	anonPrincipal := withPrincipal(context.Background(), &commonv1.Principal{Id: "anonymous", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS})
	svcPrincipal := withPrincipal(context.Background(), &commonv1.Principal{Id: "service-team-x", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE, Scopes: []string{"identity.read", "identity.write"}})
	req := &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{rec(pAnalytics, true, "v1", "banner")}}
	for name, tc := range map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"no principal":        {context.Background(), codes.Unauthenticated},
		"anonymous principal": {anonPrincipal, codes.Unauthenticated},
		"service principal":   {svcPrincipal, codes.PermissionDenied},
	} {
		t.Run(name, func(t *testing.T) {
			before := repo.total()
			if _, err := h.RecordConsent(tc.ctx, req); status.Code(err) != tc.want {
				t.Fatalf("RecordConsent: want %v, got %v", tc.want, err)
			}
			if repo.total() != before {
				t.Fatal("a row was written")
			}
			if _, err := h.ListMyConsents(tc.ctx, &identityv1.ListMyConsentsRequest{}); status.Code(err) != tc.want {
				t.Fatalf("ListMyConsents: want %v, got %v", tc.want, err)
			}
		})
	}
}

// Scenario: Anonymous and service principals cannot record consent (over the wire, with real interceptors)
func TestPrivacyWireRefusesAnonymousAndServiceAndLeavesOtherRPCsUnimplemented(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	conn := startWire(t, rpc.Handlers{Privacy: h}, discardLogger())
	c := identityv1.NewPrivacyServiceClient(conn)
	bg, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{rec(pAnalytics, true, "v1", "banner")}}

	if _, err := c.RecordConsent(bg, req); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: want Unauthenticated, got %v", err)
	}
	svc := metadata.NewOutgoingContext(bg, metadata.Pairs("x-principal-id", "service-team-x", "x-principal-type", "service", "x-principal-scopes", "identity.read"))
	if _, err := c.RecordConsent(svc, req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("service: want PermissionDenied, got %v", err)
	}
	if repo.total() != 0 {
		t.Fatal("a row was written for a refused caller")
	}
	ctx, cancel2 := principalCtx(t, "u-9")
	defer cancel2()
	if _, err := c.RecordConsent(ctx, req); err != nil {
		t.Fatalf("signed-in user: %v", err)
	}
	if n := len(repo.forUser("u-9")); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	for name, call := range map[string]func() error{
		"RequestAccountDeletion": func() error {
			_, e := c.RequestAccountDeletion(ctx, &identityv1.RequestAccountDeletionRequest{})
			return e
		},
		"CancelAccountDeletion": func() error {
			_, e := c.CancelAccountDeletion(ctx, &identityv1.CancelAccountDeletionRequest{})
			return e
		},
		"GetAccountDeletionStatus": func() error {
			_, e := c.GetAccountDeletionStatus(ctx, &identityv1.GetAccountDeletionStatusRequest{})
			return e
		},
		"ExportMyData": func() error { _, e := c.ExportMyData(ctx, &identityv1.ExportMyDataRequest{}); return e },
	} {
		if err := call(); status.Code(err) != codes.Unimplemented {
			t.Fatalf("%s: want Unimplemented, got %v", name, err)
		}
	}
}

// Scenario: Page size is clamped and bad cursors are rejected
func TestPrivacyListPageSizeClampedAndBadCursorRejected(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	repo.seed("u-10", 130)
	ctx := privCtx("u-10")
	for _, tc := range []struct {
		size    int32
		wantLen int
		wantLim int
	}{
		{500, 100, 100},
		{100, 100, 100},
		{101, 100, 100},
		{0, 20, 20},
		{-5, 20, 20},
		{7, 7, 7},
	} {
		resp, err := h.ListMyConsents(ctx, &identityv1.ListMyConsentsRequest{Page: &commonv1.PageRequest{PageSize: tc.size}})
		if err != nil {
			t.Fatalf("page_size=%d: %v", tc.size, err)
		}
		if len(resp.GetRecords()) != tc.wantLen || repo.lastLim != tc.wantLim {
			t.Fatalf("page_size=%d: got %d records with store limit %d, want %d and %d", tc.size, len(resp.GetRecords()), repo.lastLim, tc.wantLen, tc.wantLim)
		}
	}
	for _, cur := range []string{"%%%not-a-cursor", "not-a-number", "-3"} {
		_, err := h.ListMyConsents(ctx, &identityv1.ListMyConsentsRequest{Page: &commonv1.PageRequest{Cursor: cur}})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("cursor %q: want InvalidArgument, got %v", cur, err)
		}
	}
}

// Scenario: Page size is clamped and bad cursors are rejected (paging walks every record once)
func TestPrivacyListFollowsCursorsWithoutRepeats(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	repo.seed("u-11", 25)
	repo.seed("u-other", 5)
	ctx := privCtx("u-11")
	var sizes []int
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 10; i++ {
		resp, err := h.ListMyConsents(ctx, &identityv1.ListMyConsentsRequest{Page: &commonv1.PageRequest{PageSize: 10, Cursor: cursor}})
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(resp.GetRecords()))
		for _, r := range resp.GetRecords() {
			if seen[r.GetVersion()] {
				t.Fatalf("record %s repeated", r.GetVersion())
			}
			seen[r.GetVersion()] = true
		}
		cursor = resp.GetPage().GetNextCursor()
		if cursor == "" {
			break
		}
	}
	if len(sizes) != 3 || sizes[0] != 10 || sizes[1] != 10 || sizes[2] != 5 {
		t.Fatalf("page sizes = %v, want [10 10 5]", sizes)
	}
}

// Scenario: A user with no records gets an empty list
func TestPrivacyListForUserWithNoRecordsIsEmpty(t *testing.T) {
	h, repo := newPrivacy(t, discardLogger())
	repo.seed("someone-else", 3)
	resp, err := h.ListMyConsents(privCtx("legacy-user"), &identityv1.ListMyConsentsRequest{})
	if err != nil {
		t.Fatalf("ListMyConsents: %v", err)
	}
	if len(resp.GetRecords()) != 0 || resp.GetPage().GetNextCursor() != "" {
		t.Fatalf("want empty list and empty next_cursor, got %v", resp)
	}
}

// Scenario: Invalid records are rejected without partial writes (store failure is a server fault, text not leaked)
func TestPrivacyRepositoryFailuresAreInternalAndDoNotLeak(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h, repo := newPrivacy(t, logger)
	repo.failApp = errors.New("pq: connection reset by peer SECRET-DSN")
	repo.failList = errors.New("pq: relation exploded SECRET-DSN")
	_, err := h.RecordConsent(privCtx("u-12"), &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{rec(pAnalytics, true, "v1", "banner")}})
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "SECRET-DSN") {
		t.Fatalf("RecordConsent: want Internal without store text, got %v", err)
	}
	_, err = h.ListMyConsents(privCtx("u-12"), &identityv1.ListMyConsentsRequest{})
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "SECRET-DSN") {
		t.Fatalf("ListMyConsents: want Internal without store text, got %v", err)
	}
}

// Scenario: Banner choice stores one row per changed purpose (logs carry no raw ids or payloads)
func TestPrivacyLogsCarryNoRawUserIdOrVersions(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h, _ := newPrivacy(t, logger)
	ctx := privCtx("raw-user-id-4242")
	_, _ = h.RecordConsent(ctx, &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{rec(pAnalytics, true, "marker-version-777", "banner")}})
	_, _ = h.RecordConsent(ctx, &identityv1.RecordConsentRequest{Records: []*identityv1.ConsentRecord{rec(pTerms, true, "marker-stale-888", "account")}})
	_, _ = h.ListMyConsents(ctx, &identityv1.ListMyConsentsRequest{Page: &commonv1.PageRequest{Cursor: "marker-cursor-999"}})
	for _, marker := range []string{"raw-user-id-4242", "marker-version-777", "marker-stale-888", "marker-cursor-999"} {
		if strings.Contains(buf.String(), marker) {
			t.Fatalf("log output contains %q: %s", marker, buf.String())
		}
	}
}
