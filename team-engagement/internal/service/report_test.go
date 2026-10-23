package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

const (
	rptListingA = "11111111-1111-4111-8111-111111111111"
	rptListingB = "22222222-2222-4222-8222-222222222222"
)

func rptListing(i int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", i) }

// rptLog records the order of side effects across audit and repository.
type rptLog struct {
	mu     sync.Mutex
	events []string
}

func (l *rptLog) add(e string) {
	l.mu.Lock()
	l.events = append(l.events, e)
	l.mu.Unlock()
}

func (l *rptLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// rptAudit is the true external boundary (team-audit). It satisfies
// service.AuditWriter whichever audit method the service chooses to call.
type rptAudit struct {
	log   *rptLog
	err   error
	mu    sync.Mutex
	evs   []upstream.AuditEvent
	other int // hide-event calls (must stay 0)
}

func (a *rptAudit) record(ev upstream.AuditEvent) error {
	if a.log != nil {
		a.log.add("audit")
	}
	if a.err != nil {
		return a.err
	}
	a.mu.Lock()
	a.evs = append(a.evs, ev)
	a.mu.Unlock()
	return nil
}

func (a *rptAudit) WriteHideEvent(_ context.Context, _, _, _, _ string) error {
	a.mu.Lock()
	a.other++
	a.mu.Unlock()
	return nil
}

func (a *rptAudit) WriteEvent(_ context.Context, ev upstream.AuditEvent) error { return a.record(ev) }

func (a *rptAudit) WriteReportResolveEvent(_ context.Context, actorID, reportID, listingID, status, resolutionNote string) error {
	return a.record(upstream.AuditEvent{
		ActorID: actorID, Action: "report.resolve", TargetType: "report", TargetID: reportID,
		Metadata: map[string]string{"status": status, "listing_id": listingID, "resolution_note": resolutionNote},
	})
}

func (a *rptAudit) events() []upstream.AuditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]upstream.AuditEvent(nil), a.evs...)
}

// rptRepo wraps the in-memory repository to log ordering, inject a close
// failure and count calls that must not happen.
type rptRepo struct {
	*storetest.FakeReportRepository
	log       *rptLog
	closeErrs []error // consumed one per CloseReport call
	closes    int
}

func (r *rptRepo) CloseReport(ctx context.Context, id, adminID string, st store.ReportStatus, note string) (bool, error) {
	if r.log != nil {
		r.log.add("close")
	}
	r.closes++
	if len(r.closeErrs) > 0 {
		e := r.closeErrs[0]
		r.closeErrs = r.closeErrs[1:]
		if e != nil {
			return false, e
		}
	}
	return r.FakeReportRepository.CloseReport(ctx, id, adminID, st, note)
}

type rptEnv struct {
	mem   *storetest.FakeReportRepository
	repo  *rptRepo
	audit *rptAudit
	log   *rptLog
	svc   *service.ReportService
}

func newRptEnv() *rptEnv {
	l := &rptLog{}
	mem := storetest.NewFakeReportRepository()
	repo := &rptRepo{FakeReportRepository: mem, log: l}
	au := &rptAudit{log: l}
	return &rptEnv{mem: mem, repo: repo, audit: au, log: l, svc: service.NewReportService(repo, nil).WithAudit(au)}
}

func (e *rptEnv) openReports(t *testing.T) []store.Report {
	t.Helper()
	rs, _, err := e.mem.ListReports(context.Background(), store.ReportStatusOpen, "", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func (e *rptEnv) allReports(t *testing.T) int64 {
	t.Helper()
	_, n, err := e.mem.ListReports(context.Background(), "", "", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *rptEnv) seedOpen(id, reporter, listing string) {
	e.mem.SeedReport(store.Report{ID: id, TargetID: listing, ReporterID: reporter, Status: store.ReportStatusOpen, Note: "secret-complaint"})
}

// ---- CreateReport -------------------------------------------------------

// Scenario: Listing reports: Second open report by the same user returns the existing one
func TestCreateReportIdempotentWhileOpen(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	a, err := e.svc.CreateReport(ctx, "u1", rptListingA, "scam", "first")
	if err != nil {
		t.Fatalf("first CreateReport: %v", err)
	}
	if a.Status != store.ReportStatusOpen || a.ReporterID != "u1" || a.TargetID != rptListingA || a.ReasonCode != "scam" || a.ID == "" {
		t.Fatalf("unexpected report: %+v", a)
	}
	b, err := e.svc.CreateReport(ctx, "u1", rptListingA, "scam", "again")
	if err != nil {
		t.Fatalf("second CreateReport: %v", err)
	}
	if b.ID != a.ID {
		t.Errorf("ids differ: %s vs %s", a.ID, b.ID)
	}
	if n := len(e.openReports(t)); n != 1 {
		t.Errorf("open rows = %d, want 1", n)
	}
}

// Scenario: Listing reports: Different users report the same listing
func TestCreateReportTwoReportersTwoRows(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	a, err := e.svc.CreateReport(ctx, "u1", rptListingA, "scam", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.CreateReport(ctx, "u2", rptListingA, "scam", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Error("two reporters must get distinct reports")
	}
	if n := len(e.openReports(t)); n != 2 {
		t.Errorf("open rows = %d, want 2", n)
	}
}

// Scenario: Listing reports: Note is optional and trimmed
func TestCreateReportNoteOptionalAndTrimmed(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	a, err := e.svc.CreateReport(ctx, "u1", rptListingA, "other", "")
	if err != nil {
		t.Fatalf("empty note: %v", err)
	}
	b, err := e.svc.CreateReport(ctx, "u2", rptListingA, "other", "  scam  ")
	if err != nil {
		t.Fatalf("padded note: %v", err)
	}
	if a.Note != "" || b.Note != "scam" {
		t.Errorf("notes = %q, %q; want \"\", \"scam\"", a.Note, b.Note)
	}
}

// Scenario: Listing reports: Note length boundary (service half, runes after trim)
func TestCreateReportNoteLengthBoundary_Service(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	if _, err := e.svc.CreateReport(ctx, "u1", rptListingA, "other", strings.Repeat("ạ", 500)); err != nil {
		t.Errorf("500 runes: %v", err)
	}
	_, err := e.svc.CreateReport(ctx, "u1", rptListingB, "other", strings.Repeat("ạ", 501))
	if !errors.Is(err, service.ErrReportNoteTooLong) {
		t.Errorf("501 runes err = %v, want ErrReportNoteTooLong", err)
	}
	if n := len(e.openReports(t)); n != 1 {
		t.Errorf("open rows = %d, want 1", n)
	}
}

// Scenario: Listing reports: Unknown reason code is rejected
func TestCreateReportUnknownReasonRejected(t *testing.T) {
	e := newRptEnv()
	for _, bad := range []string{"spam", "", "SCAM"} {
		if _, err := e.svc.CreateReport(context.Background(), "u1", rptListingA, bad, ""); !errors.Is(err, service.ErrInvalidReportReason) {
			t.Errorf("reason %q err = %v, want ErrInvalidReportReason", bad, err)
		}
	}
	for _, ok := range []string{"prohibited_item", "counterfeit", "scam", "misleading", "inappropriate", "other"} {
		if _, err := e.svc.CreateReport(context.Background(), "u1", rptListing(len(ok)), ok, ""); err != nil {
			t.Errorf("reason %q: %v", ok, err)
		}
	}
	if n := e.allReports(t); n != 6 {
		t.Errorf("rows = %d, want 6 (only the valid reasons)", n)
	}
}

// Scenario: Listing reports: Target type and target id are validated (service half)
func TestCreateReportTargetIDValidated(t *testing.T) {
	e := newRptEnv()
	for _, bad := range []string{"", "not-a-uuid"} {
		if _, err := e.svc.CreateReport(context.Background(), "u1", bad, "scam", ""); !errors.Is(err, service.ErrInvalidReportTarget) {
			t.Errorf("target %q err = %v, want ErrInvalidReportTarget", bad, err)
		}
	}
	if n := e.allReports(t); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
}

// Scenario: Listing reports: Seller cannot report their own listing
func TestCreateReportOwnListingRefused(t *testing.T) {
	e := newRptEnv()
	e.mem.SeedSellerListing("seller-1", rptListingA)
	_, err := e.svc.CreateReport(context.Background(), "seller-1", rptListingA, "scam", "")
	if !errors.Is(err, service.ErrOwnListing) {
		t.Fatalf("err = %v, want ErrOwnListing", err)
	}
	if n := e.allReports(t); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
	// Another user may still report that listing.
	if _, err := e.svc.CreateReport(context.Background(), "buyer-1", rptListingA, "scam", ""); err != nil {
		t.Errorf("other reporter: %v", err)
	}
}

// Scenario: Listing reports: Open report cap per reporter
func TestCreateReportOpenCapPerReporter(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	for i := 1; i <= 20; i++ {
		e.seedOpen(fmt.Sprintf("rp-%d", i), "u1", rptListing(i))
	}
	_, err := e.svc.CreateReport(ctx, "u1", rptListing(21), "scam", "")
	if !errors.Is(err, service.ErrReportCapReached) {
		t.Fatalf("21st err = %v, want ErrReportCapReached", err)
	}
	if n, _ := e.mem.CountOpenByReporter(ctx, "u1"); n != 20 {
		t.Errorf("open after refusal = %d, want 20", n)
	}
	// Re-report of an already open pair is exempt and returns the existing report.
	got, err := e.svc.CreateReport(ctx, "u1", rptListing(7), "scam", "")
	if err != nil {
		t.Fatalf("re-report at cap: %v", err)
	}
	if got.ID != "rp-7" {
		t.Errorf("re-report id = %q, want rp-7", got.ID)
	}
	if n, _ := e.mem.CountOpenByReporter(ctx, "u1"); n != 20 {
		t.Errorf("open after re-report = %d, want 20", n)
	}
	// The cap is per reporter.
	if _, err := e.svc.CreateReport(ctx, "u2", rptListing(21), "scam", ""); err != nil {
		t.Errorf("other reporter at cap boundary: %v", err)
	}
}

// Scenario: Listing reports: A closed report does not block a new one
func TestCreateReportAfterCloseCreatesNew(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	old := time.Now().Add(-time.Hour)
	e.mem.SeedReport(store.Report{ID: "rp-old", TargetID: rptListingA, ReporterID: "u1",
		Status: store.ReportStatusDismissed, ResolvedAt: &old, ResolvedBy: "adm-1", ResolutionNote: "no"})
	got, err := e.svc.CreateReport(ctx, "u1", rptListingA, "scam", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "rp-old" || got.Status != store.ReportStatusOpen {
		t.Errorf("got %+v, want a new OPEN report", got)
	}
	if n := e.allReports(t); n != 2 {
		t.Errorf("rows = %d, want 2", n)
	}
}

// Scenario: Listing reports: Pseudonymised reporter can report again
func TestCreateReportAfterSweep(t *testing.T) {
	e := newRptEnv()
	ctx := context.Background()
	old := time.Now().Add(-3 * 365 * 24 * time.Hour)
	e.mem.SeedReport(store.Report{ID: "rp-old", TargetID: rptListingA, ReporterID: "u1", Note: "x",
		Status: store.ReportStatusActioned, ResolvedAt: &old, ResolvedBy: "adm-1", ResolutionNote: "ok"})
	if n, err := e.mem.SweepClosedBefore(ctx, time.Now().Add(-2*365*24*time.Hour), 10); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v; want 1, nil", n, err)
	}
	got, err := e.svc.CreateReport(ctx, "u1", rptListingA, "scam", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.ReportStatusOpen || got.ReporterID != "u1" || got.ID == "rp-old" {
		t.Errorf("got %+v, want a new OPEN report with the real reporter id", got)
	}
}

// ---- ResolveReport ------------------------------------------------------

// Scenario: Listing reports: Audit is written before the close
func TestResolveReportAuditBeforeClose(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	got, err := e.svc.ResolveReport(context.Background(), "adm-1", "rp-1", store.ReportStatusActioned, "đã gỡ tin")
	if err != nil {
		t.Fatalf("ResolveReport: %v", err)
	}
	if order := e.log.snapshot(); len(order) != 2 || order[0] != "audit" || order[1] != "close" {
		t.Errorf("order = %v, want [audit close]", order)
	}
	if got.Status != store.ReportStatusActioned || got.ResolvedAt == nil || got.ResolutionNote != "đã gỡ tin" || got.ResolvedBy != "adm-1" {
		t.Errorf("returned report = %+v", got)
	}
	stored, _ := e.mem.GetReport(context.Background(), "rp-1")
	if stored.Status != store.ReportStatusActioned || stored.ResolvedAt == nil {
		t.Errorf("stored = %+v", stored)
	}
}

// Scenario: Listing reports: Audit event carries the required fields
func TestResolveReportAuditEventFields_Service(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "reporter-9", "L-1")
	if _, err := e.svc.ResolveReport(context.Background(), "adm-1", "rp-1", store.ReportStatusActioned, "đã gỡ tin"); err != nil {
		t.Fatal(err)
	}
	evs := e.audit.events()
	if len(evs) != 1 {
		t.Fatalf("audit events = %d, want 1", len(evs))
	}
	ev := evs[0]
	if ev.Action != "report.resolve" || ev.TargetType != "report" || ev.TargetID != "rp-1" || ev.ActorID != "adm-1" {
		t.Errorf("event = %+v", ev)
	}
	if ev.Metadata["status"] != "ACTIONED" || ev.Metadata["listing_id"] != "L-1" || ev.Metadata["resolution_note"] != "đã gỡ tin" {
		t.Errorf("metadata = %v", ev.Metadata)
	}
	for k, v := range ev.Metadata {
		if k == "reporter_id" || k == "note" || strings.Contains(v, "reporter-9") || strings.Contains(v, "secret-complaint") {
			t.Errorf("metadata leaks complainant data: %s=%s", k, v)
		}
	}
}

// Scenario: Listing reports: Audit failure leaves the report open
func TestResolveReportAuditFailureFailsClosed(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	e.audit.err = errors.New("audit down")
	_, err := e.svc.ResolveReport(context.Background(), "adm-1", "rp-1", store.ReportStatusDismissed, "no")
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("err = %v, want ErrAuditUnavailable", err)
	}
	if e.repo.closes != 0 {
		t.Errorf("CloseReport called %d times, want 0", e.repo.closes)
	}
	got, _ := e.mem.GetReport(context.Background(), "rp-1")
	if got.Status != store.ReportStatusOpen || got.ResolvedAt != nil {
		t.Errorf("report = %+v, want OPEN with no resolved_at", got)
	}
	if len(e.openReports(t)) != 1 {
		t.Error("report must still be listed OPEN")
	}
}

// Scenario: Listing reports: No audit upstream configured
func TestResolveReportNilAuditClientUnavailable(t *testing.T) {
	ctx := context.Background()
	for name, mk := range map[string]func(r store.ReportRepository) *service.ReportService{
		"no WithAudit": func(r store.ReportRepository) *service.ReportService { return service.NewReportService(r, nil) },
		"typed nil client": func(r store.ReportRepository) *service.ReportService {
			return service.NewReportService(r, nil).WithAudit((*upstream.AuditClient)(nil))
		},
	} {
		mem := storetest.NewFakeReportRepository()
		mem.SeedReport(store.Report{ID: "rp-1", TargetID: rptListingA, ReporterID: "u1"})
		_, err := mk(mem).ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusActioned, "n")
		if !errors.Is(err, service.ErrAuditUnavailable) {
			t.Errorf("%s: err = %v, want ErrAuditUnavailable", name, err)
		}
		got, _ := mem.GetReport(ctx, "rp-1")
		if got.Status != store.ReportStatusOpen {
			t.Errorf("%s: status = %s, want OPEN", name, got.Status)
		}
	}
}

// Scenario: Listing reports: Close failure after audit is Internal and retryable
func TestResolveReportCloseFailureInternalThenRetry(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	e.repo.closeErrs = []error{errors.New("db down")}
	ctx := context.Background()
	_, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusActioned, "n")
	if err == nil {
		t.Fatal("first call must fail")
	}
	for _, s := range []error{service.ErrAuditUnavailable, service.ErrReportClosed, store.ErrReportNotFound} {
		if errors.Is(err, s) {
			t.Errorf("close failure must be Internal (unclassified), got sentinel %v", s)
		}
	}
	if got, _ := e.mem.GetReport(ctx, "rp-1"); got.Status != store.ReportStatusOpen {
		t.Errorf("status after failed close = %s, want OPEN", got.Status)
	}
	if _, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusActioned, "n"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got, _ := e.mem.GetReport(ctx, "rp-1"); got.Status != store.ReportStatusActioned {
		t.Errorf("status after retry = %s, want ACTIONED", got.Status)
	}
	if n := len(e.audit.events()); n != 2 {
		t.Errorf("audit events = %d, want 2 (at-least-once)", n)
	}
}

// Scenario: Listing reports: Closing again with the same status is refused
func TestResolveReportSameStatusReCloseRefused(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	ctx := context.Background()
	if _, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusDismissed, "first"); err != nil {
		t.Fatal(err)
	}
	first, _ := e.mem.GetReport(ctx, "rp-1")
	_, err := e.svc.ResolveReport(ctx, "adm-2", "rp-1", store.ReportStatusDismissed, "second")
	if !errors.Is(err, service.ErrReportClosed) {
		t.Fatalf("err = %v, want ErrReportClosed", err)
	}
	after, _ := e.mem.GetReport(ctx, "rp-1")
	if after.ResolvedBy != "adm-1" || after.ResolutionNote != "first" || !after.ResolvedAt.Equal(*first.ResolvedAt) {
		t.Errorf("closed fields changed: %+v", after)
	}
	if n := len(e.audit.events()); n != 1 {
		t.Errorf("audit events = %d, want 1 (no second event)", n)
	}
}

// Scenario: Listing reports: Closing with a different status is refused
func TestResolveReportConflictingStatusRefused(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	ctx := context.Background()
	if _, err := e.svc.ResolveReport(ctx, "adm-1", "rp-1", store.ReportStatusDismissed, "first"); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.ResolveReport(ctx, "adm-2", "rp-1", store.ReportStatusActioned, "second")
	if !errors.Is(err, service.ErrReportClosed) {
		t.Fatalf("err = %v, want ErrReportClosed", err)
	}
	if got, _ := e.mem.GetReport(ctx, "rp-1"); got.Status != store.ReportStatusDismissed {
		t.Errorf("status = %s, want DISMISSED", got.Status)
	}
	if n := len(e.audit.events()); n != 1 {
		t.Errorf("audit events = %d, want 1", n)
	}
}

// Scenario: Listing reports: Closing a report leaves the listing alone
func TestResolveReportTouchesOnlyAudit(t *testing.T) {
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	if _, err := e.svc.ResolveReport(context.Background(), "adm-1", "rp-1", store.ReportStatusActioned, "n"); err != nil {
		t.Fatal(err)
	}
	if e.audit.other != 0 {
		t.Errorf("hide-event calls = %d, want 0", e.audit.other)
	}
	if n := len(e.audit.events()); n != 1 {
		t.Errorf("audit events = %d, want exactly 1", n)
	}
	if order := e.log.snapshot(); len(order) != 2 {
		t.Errorf("side effects = %v, want only audit then close", order)
	}
}

// Scenario: Listing reports: Invalid final status is rejected / Empty or whitespace note /
// Resolution note length boundary / Unknown report (service half; none may audit).
func TestResolveReportValidationNeverAudits(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		status store.ReportStatus
		note   string
		want   error
	}{
		{"status OPEN", "rp-1", store.ReportStatusOpen, "n", service.ErrInvalidReportStatus},
		{"status empty", "rp-1", "", "n", service.ErrInvalidReportStatus},
		{"empty note", "rp-1", store.ReportStatusActioned, "", service.ErrResolutionNoteRequired},
		{"whitespace note", "rp-1", store.ReportStatusActioned, "   ", service.ErrResolutionNoteRequired},
		{"501 runes", "rp-1", store.ReportStatusActioned, strings.Repeat("ạ", 501), service.ErrResolutionNoteTooLong},
		{"empty id", "", store.ReportStatusActioned, "n", service.ErrEmptyReportID},
		{"unknown id", "rp-404", store.ReportStatusActioned, "n", store.ErrReportNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newRptEnv()
			e.seedOpen("rp-1", "u1", rptListingA)
			_, err := e.svc.ResolveReport(context.Background(), "adm-1", c.id, c.status, c.note)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if n := len(e.audit.events()); n != 0 {
				t.Errorf("audit events = %d, want 0", n)
			}
			if got, _ := e.mem.GetReport(context.Background(), "rp-1"); got.Status != store.ReportStatusOpen {
				t.Errorf("status = %s, want OPEN", got.Status)
			}
		})
	}
	// Boundary: exactly 500 runes (and a padded note trimmed) succeeds.
	e := newRptEnv()
	e.seedOpen("rp-1", "u1", rptListingA)
	if _, err := e.svc.ResolveReport(context.Background(), "adm-1", "rp-1", store.ReportStatusActioned, "  "+strings.Repeat("ạ", 500)+"  "); err != nil {
		t.Errorf("500 runes: %v", err)
	}
}
