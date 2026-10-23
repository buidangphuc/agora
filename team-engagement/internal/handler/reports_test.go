package handler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

type reportAuditCall struct{ actor, report, listing, status, note string }

type reportAuditSpy struct {
	calls []reportAuditCall
	err   error
}

func (a *reportAuditSpy) WriteReportResolveEvent(_ context.Context, actorID, reportID, listingID, st, note string) error {
	if a.err != nil {
		return a.err
	}
	a.calls = append(a.calls, reportAuditCall{actorID, reportID, listingID, st, note})
	return nil
}

// reportRepoSpy counts reads/writes and can force failures.
type reportRepoSpy struct {
	*storetest.FakeReportRepository
	ops       int
	createErr error
	closeErr  error
}

func (r *reportRepoSpy) CreateReport(ctx context.Context, in store.Report) (store.Report, error) {
	r.ops++
	if r.createErr != nil {
		return store.Report{}, r.createErr
	}
	return r.FakeReportRepository.CreateReport(ctx, in)
}
func (r *reportRepoSpy) GetReport(ctx context.Context, id string) (store.Report, error) {
	r.ops++
	return r.FakeReportRepository.GetReport(ctx, id)
}
func (r *reportRepoSpy) ListReports(ctx context.Context, s store.ReportStatus, t string, o, l int) ([]store.Report, int64, error) {
	r.ops++
	return r.FakeReportRepository.ListReports(ctx, s, t, o, l)
}
func (r *reportRepoSpy) CloseReport(ctx context.Context, id, adm string, s store.ReportStatus, n string) (bool, error) {
	r.ops++
	if r.closeErr != nil {
		return false, r.closeErr
	}
	return r.FakeReportRepository.CloseReport(ctx, id, adm, s, n)
}

type reportFixture struct {
	h     *handler.EngagementHandler
	repo  *reportRepoSpy
	audit *reportAuditSpy
	logs  *bytes.Buffer
}

func newReportFixture(t *testing.T) *reportFixture {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	repo := &reportRepoSpy{FakeReportRepository: storetest.NewFakeReportRepository()}
	audit := &reportAuditSpy{}
	svc := service.NewReportService(repo, logger).WithAudit(audit)
	h := handler.NewEngagementHandler(storetest.NewFakeRepository(), nil, nil, nil, nil, handler.WithReportService(svc))
	return &reportFixture{h: h, repo: repo, audit: audit, logs: logs}
}

func (f *reportFixture) create(ctx context.Context, target, reason, note string) (*engagementv1.CreateReportResponse, error) {
	return f.h.CreateReport(ctx, &engagementv1.CreateReportRequest{
		TargetType: engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING, TargetId: target, ReasonCode: reason, Note: note,
	})
}

func (f *reportFixture) resolve(ctx context.Context, id string, st engagementv1.ReportStatus, note string) (*engagementv1.ResolveReportResponse, error) {
	return f.h.ResolveReport(ctx, &engagementv1.ResolveReportRequest{ReportId: id, Status: st, ResolutionNote: note})
}

func (f *reportFixture) rows(t *testing.T) []store.Report {
	t.Helper()
	got, _, err := f.repo.FakeReportRepository.ListReports(context.Background(), "", "", 0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f *reportFixture) seedOpen(id, target string, at time.Time) {
	f.repo.SeedReport(store.Report{ID: id, TargetID: target, ReporterID: "rep-" + id, Note: "n", CreatedAt: at, Status: store.ReportStatusOpen})
}

func (f *reportFixture) stillOpen(t *testing.T, id string) {
	t.Helper()
	got, err := f.repo.FakeReportRepository.GetReport(context.Background(), id)
	if err != nil || got.Status != store.ReportStatusOpen || got.ResolvedAt != nil {
		t.Fatalf("report must stay OPEN: %+v %v", got, err)
	}
}

func newListingID() string { return uuid.NewString() }

// ── CreateReport ──

func TestCreateReportStoresOpenReport(t *testing.T) { // LR Listing reports: Buyer reports a listing
	f := newReportFixture(t)
	l := newListingID()
	res, err := f.create(userCtx("buyer-1"), l, "counterfeit", "hàng nhái")
	if err != nil {
		t.Fatalf("CreateReport: %v", err)
	}
	r := res.GetReport()
	if r.GetStatus() != engagementv1.ReportStatus_REPORT_STATUS_OPEN || r.GetReporterId() != "buyer-1" ||
		r.GetCreatedAt() == nil || r.GetTargetId() != l || r.GetReasonCode() != "counterfeit" || r.GetNote() != "hàng nhái" {
		t.Fatalf("report = %v", r)
	}
	if rows := f.rows(t); len(rows) != 1 {
		t.Fatalf("want exactly 1 stored row, got %d", len(rows))
	}
}

func TestCreateReportAnonymousUnauthenticated(t *testing.T) { // LR Listing reports: Anonymous caller is unauthenticated
	f := newReportFixture(t)
	for name, ctx := range map[string]context.Context{"no principal": context.Background(), "anonymous principal": anonCtx()} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.create(ctx, newListingID(), "scam", ""); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("want Unauthenticated, got %v", err)
			}
		})
	}
	if n := len(f.rows(t)); n != 0 {
		t.Fatalf("rows stored: %d", n)
	}
}

func TestCreateReportMissingWriteScopeDenied(t *testing.T) { // LR Listing reports: Caller without the write scope is denied
	f := newReportFixture(t)
	ctx := principalCtx("buyer-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, "engagement:read")
	if _, err := f.create(ctx, newListingID(), "scam", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if n := len(f.rows(t)); n != 0 {
		t.Fatalf("rows stored: %d", n)
	}
}

func TestCreateReportOwnListingRefused(t *testing.T) { // LR Listing reports: Seller cannot report their own listing
	f := newReportFixture(t)
	l := newListingID()
	f.repo.SeedSellerListing("seller-1", l)
	if _, err := f.create(userCtx("seller-1"), l, "scam", ""); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if n := len(f.rows(t)); n != 0 {
		t.Fatalf("rows stored: %d", n)
	}
}

func TestCreateReportUnknownReasonInvalidArgument(t *testing.T) { // LR Listing reports: Unknown reason code is rejected
	f := newReportFixture(t)
	if _, err := f.create(userCtx("buyer-1"), newListingID(), "spam", ""); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if n := len(f.rows(t)); n != 0 {
		t.Fatalf("rows stored: %d", n)
	}
}

func TestCreateReportTargetValidation(t *testing.T) { // LR Listing reports: Target type and target id are validated
	f := newReportFixture(t)
	ctx := userCtx("buyer-1")
	reqs := map[string]*engagementv1.CreateReportRequest{
		"unspecified type": {TargetType: engagementv1.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED, TargetId: newListingID(), ReasonCode: "scam"},
		"bad uuid":         {TargetType: engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING, TargetId: "not-a-uuid", ReasonCode: "scam"},
		"empty id":         {TargetType: engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING, TargetId: "", ReasonCode: "scam"},
	}
	for name, req := range reqs {
		t.Run(name, func(t *testing.T) {
			if _, err := f.h.CreateReport(ctx, req); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
		})
	}
	if n := len(f.rows(t)); n != 0 {
		t.Fatalf("rows stored: %d", n)
	}
}

func TestCreateReportNoteLengthBoundary(t *testing.T) { // LR Listing reports: Note length boundary
	f := newReportFixture(t)
	ctx := userCtx("buyer-1")
	if _, err := f.create(ctx, newListingID(), "scam", strings.Repeat("a", 500)); err != nil {
		t.Fatalf("500 chars: %v", err)
	}
	if _, err := f.create(ctx, newListingID(), "scam", strings.Repeat("a", 501)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("501 chars: want InvalidArgument, got %v", err)
	}
	if _, err := f.create(ctx, newListingID(), "scam", strings.Repeat("ạ", 500)); err != nil {
		t.Fatalf("500 runes (1500 bytes): %v", err)
	}
	if n := len(f.rows(t)); n != 2 {
		t.Fatalf("want 2 rows, got %d", n)
	}
}

func TestCreateReportUnknownListingAccepted(t *testing.T) { // LR Listing reports: Report of an unknown listing is accepted
	f := newReportFixture(t)
	res, err := f.create(userCtx("buyer-1"), newListingID(), "other", "")
	if err != nil || res.GetReport().GetStatus() != engagementv1.ReportStatus_REPORT_STATUS_OPEN {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCreateReportOpenCapResourceExhausted(t *testing.T) { // LR Listing reports: Open report cap per reporter (sentinel mapping)
	f := newReportFixture(t)
	for i := 0; i < 20; i++ {
		f.repo.SeedReport(store.Report{ID: fmt.Sprintf("seed-%d", i), TargetID: newListingID(), ReporterID: "buyer-1", Status: store.ReportStatusOpen})
	}
	if _, err := f.create(userCtx("buyer-1"), newListingID(), "scam", ""); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("want ResourceExhausted, got %v", err)
	}
}

func TestCreateReportNoteNotLogged(t *testing.T) { // LR Listing reports: Report note is never logged
	const secret = "my phone is 0900000000"
	f := newReportFixture(t)
	f.repo.createErr = errors.New("db down")
	_, err := f.create(userCtx("buyer-1"), newListingID(), "scam", secret)
	if status.Code(err) != codes.Internal {
		t.Fatalf("want Internal, got %v", err)
	}
	if strings.Contains(err.Error(), "db down") {
		t.Fatalf("storage detail leaked: %v", err)
	}
	if strings.Contains(f.logs.String(), secret) {
		t.Fatalf("note leaked into logs:\n%s", f.logs.String())
	}
}

func TestListReportsSellerDenied(t *testing.T) { // LR Listing reports: Seller cannot list reports
	f := newReportFixture(t)
	l := newListingID()
	f.seedOpen("rp-1", l, time.Now())
	ctx := principalCtx("seller-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, append(append([]string{}, userScopes...), "listing.write")...)
	res, err := f.h.ListReports(ctx, &engagementv1.ListReportsRequest{TargetId: l})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if len(res.GetReports()) != 0 {
		t.Fatalf("response carries reports: %v", res)
	}
}

// ── ListReports ──

func TestListReportsOldestFirst(t *testing.T) { // RR Listing reports: Admin lists open reports oldest first
	f := newReportFixture(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.seedOpen("rp-3", "L", base.Add(2*time.Hour))
	f.seedOpen("rp-1", "L", base)
	f.seedOpen("rp-2", "L", base.Add(time.Hour))
	res, err := f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{Status: engagementv1.ReportStatus_REPORT_STATUS_OPEN})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range res.GetReports() {
		ids = append(ids, r.GetId())
	}
	if strings.Join(ids, ",") != "rp-1,rp-2,rp-3" || res.GetPage().GetTotal() != 3 {
		t.Fatalf("ids=%v total=%d", ids, res.GetPage().GetTotal())
	}
}

func TestListReportsPageSizeBoundary(t *testing.T) { // RR Listing reports: Page size boundary
	f := newReportFixture(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 150; i++ {
		f.seedOpen(fmt.Sprintf("rp-%03d", i), "L", base.Add(time.Duration(i)*time.Minute))
	}
	res, err := f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{Page: &commonv1.PageRequest{PageSize: 100}})
	if err != nil || len(res.GetReports()) != 100 || res.GetPage().GetNextCursor() == "" || res.GetPage().GetTotal() != 150 {
		t.Fatalf("size 100: n=%d next=%q total=%d err=%v", len(res.GetReports()), res.GetPage().GetNextCursor(), res.GetPage().GetTotal(), err)
	}
	res, err = f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{Page: &commonv1.PageRequest{PageSize: 101}})
	if err != nil || len(res.GetReports()) > 100 || len(res.GetReports()) == 0 {
		t.Fatalf("size 101: n=%d err=%v", len(res.GetReports()), err)
	}
	res, err = f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{})
	if err != nil || len(res.GetReports()) != 20 {
		t.Fatalf("default size: n=%d err=%v", len(res.GetReports()), err)
	}
}

func TestListReportsEmpty(t *testing.T) { // RR Listing reports: Empty result
	f := newReportFixture(t)
	res, err := f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{Status: engagementv1.ReportStatus_REPORT_STATUS_OPEN})
	if err != nil || len(res.GetReports()) != 0 || res.GetPage().GetTotal() != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestListReportsAuthzMatrix(t *testing.T) { // RR Listing reports: Non-admin and anonymous callers are refused
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L", time.Now())
	cases := map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"buyer":        {userCtx("buyer-1"), codes.PermissionDenied},
		"anonymous":    {anonCtx(), codes.Unauthenticated},
		"no principal": {context.Background(), codes.Unauthenticated},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.h.ListReports(c.ctx, &engagementv1.ListReportsRequest{}); status.Code(err) != c.want {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	if f.repo.ops != 0 {
		t.Fatalf("repository touched %d times before authorization", f.repo.ops)
	}
}

func TestListReportsWritesNoAudit(t *testing.T) { // RR Listing reports: Listing is not audited
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L", time.Now())
	if _, err := f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{}); err != nil {
		t.Fatal(err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d", len(f.audit.calls))
	}
}

// ── ResolveReport ──

func TestResolveReportActioned(t *testing.T) { // RR Listing reports: Admin closes a report as actioned
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L-1", time.Now())
	res, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "đã gỡ tin")
	if err != nil {
		t.Fatalf("ResolveReport: %v", err)
	}
	r := res.GetReport()
	if r.GetStatus() != engagementv1.ReportStatus_REPORT_STATUS_ACTIONED || r.GetResolvedAt() == nil || r.GetResolutionNote() != "đã gỡ tin" {
		t.Fatalf("report = %v", r)
	}
}

func TestResolveReportDismissed(t *testing.T) { // RR Listing reports: Admin dismisses a report
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L-1", time.Now())
	res, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_DISMISSED, "không vi phạm")
	if err != nil || res.GetReport().GetStatus() != engagementv1.ReportStatus_REPORT_STATUS_DISMISSED {
		t.Fatalf("res=%v err=%v", res, err)
	}
	list, err := f.h.ListReports(adminCtx(), &engagementv1.ListReportsRequest{Status: engagementv1.ReportStatus_REPORT_STATUS_OPEN})
	if err != nil || len(list.GetReports()) != 0 {
		t.Fatalf("closed report still in OPEN list: %v err=%v", list, err)
	}
}

func TestResolveReportAuditEventFields(t *testing.T) { // RR Listing reports: Audit event carries the required fields
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L-1", time.Now())
	if _, err := f.resolve(adminCtxAs("adm-1"), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "đã gỡ tin"); err != nil {
		t.Fatal(err)
	}
	want := reportAuditCall{actor: "adm-1", report: "rp-1", listing: "L-1", status: "ACTIONED", note: "đã gỡ tin"}
	if len(f.audit.calls) != 1 || f.audit.calls[0] != want {
		t.Fatalf("audit calls = %+v, want exactly %+v", f.audit.calls, want)
	}
	got, _ := f.repo.FakeReportRepository.GetReport(context.Background(), "rp-1")
	if got.ResolvedBy != "adm-1" {
		t.Fatalf("resolved_by = %q, want principal id", got.ResolvedBy)
	}
}

func TestResolveReportInvalidStatus(t *testing.T) { // RR Listing reports: Invalid final status is rejected
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L-1", time.Now())
	for _, st := range []engagementv1.ReportStatus{engagementv1.ReportStatus_REPORT_STATUS_OPEN, engagementv1.ReportStatus_REPORT_STATUS_UNSPECIFIED} {
		if _, err := f.resolve(adminCtx(), "rp-1", st, "note"); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("status %v: want InvalidArgument, got %v", st, err)
		}
	}
	if len(f.audit.calls) != 0 {
		t.Fatal("audit written")
	}
	f.stillOpen(t, "rp-1")
}

func TestResolveReportEmptyNoteInvalidArgument(t *testing.T) { // RR Listing reports: Empty or whitespace note is rejected
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L-1", time.Now())
	for _, n := range []string{"", "   "} {
		if _, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, n); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("note %q: want InvalidArgument, got %v", n, err)
		}
	}
	if len(f.audit.calls) != 0 {
		t.Fatal("audit written")
	}
	f.stillOpen(t, "rp-1")
}

func TestResolveReportNoteLengthBoundary(t *testing.T) { // RR Listing reports: Resolution note length boundary
	f := newReportFixture(t)
	f.seedOpen("rp-ok", "L", time.Now())
	f.seedOpen("rp-long", "L", time.Now())
	if _, err := f.resolve(adminCtx(), "rp-ok", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, strings.Repeat("a", 500)); err != nil {
		t.Fatalf("500 chars: %v", err)
	}
	if _, err := f.resolve(adminCtx(), "rp-long", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, strings.Repeat("a", 501)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("501 chars: want InvalidArgument, got %v", err)
	}
	f.stillOpen(t, "rp-long")
}

func TestResolveReportNotFoundAndEmptyID(t *testing.T) { // RR Listing reports: Unknown report
	f := newReportFixture(t)
	if _, err := f.resolve(adminCtx(), "no-such", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "n"); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown id: want NotFound, got %v", err)
	}
	if _, err := f.resolve(adminCtx(), "", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "n"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty id: want InvalidArgument, got %v", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatal("audit written")
	}
}

func TestResolveReportAuthzMatrix(t *testing.T) { // RR Listing reports: Non-admin and anonymous callers cannot close
	f := newReportFixture(t)
	f.repo.SeedReport(store.Report{ID: "rp-1", TargetID: "L", ReporterID: "reporter-1", Status: store.ReportStatusOpen})
	cases := map[string]struct {
		ctx  context.Context
		want codes.Code
	}{
		"buyer":        {userCtx("buyer-1"), codes.PermissionDenied},
		"reporter":     {userCtx("reporter-1"), codes.PermissionDenied},
		"anonymous":    {anonCtx(), codes.Unauthenticated},
		"no principal": {context.Background(), codes.Unauthenticated},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.resolve(c.ctx, "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "n"); status.Code(err) != c.want {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	// authorization precedes validation
	if _, err := f.resolve(userCtx("buyer-1"), "", engagementv1.ReportStatus_REPORT_STATUS_OPEN, ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("authz must precede validation, got %v", err)
	}
	if len(f.audit.calls) != 0 || f.repo.ops != 0 {
		t.Fatalf("audit=%d repo ops=%d", len(f.audit.calls), f.repo.ops)
	}
	f.stillOpen(t, "rp-1")
}

func TestResolveReportSentinelMapping(t *testing.T) { // RR Listing reports: error mapping (closed, audit down, close failure)
	t.Run("already closed is FailedPrecondition", func(t *testing.T) {
		f := newReportFixture(t)
		f.seedOpen("rp-1", "L", time.Now())
		if _, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_DISMISSED, "n"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_DISMISSED, "again"); status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("want FailedPrecondition, got %v", err)
		}
		if len(f.audit.calls) != 1 {
			t.Fatalf("audit calls = %d, want 1", len(f.audit.calls))
		}
	})
	t.Run("audit down is Unavailable and report stays open", func(t *testing.T) {
		f := newReportFixture(t)
		f.seedOpen("rp-1", "L", time.Now())
		f.audit.err = errors.New("audit down")
		if _, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "n"); status.Code(err) != codes.Unavailable {
			t.Fatalf("want Unavailable, got %v", err)
		}
		f.stillOpen(t, "rp-1")
	})
	t.Run("close failure is Internal without detail", func(t *testing.T) {
		f := newReportFixture(t)
		f.seedOpen("rp-1", "L", time.Now())
		f.repo.closeErr = errors.New("db down: secret-dsn")
		_, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, "n")
		if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "secret-dsn") {
			t.Fatalf("want sanitized Internal, got %v", err)
		}
	})
}

func TestResolveReportNoteNeverLogged(t *testing.T) { // RR Listing reports: resolution note is never logged
	const secret = "sensitive-resolution-note-9c1e"
	f := newReportFixture(t)
	f.seedOpen("rp-1", "L", time.Now())
	f.audit.err = errors.New("audit down")
	_, err := f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, secret)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	f.audit.err = nil
	f.repo.closeErr = errors.New("db down")
	_, _ = f.resolve(adminCtx(), "rp-1", engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, secret)
	if strings.Contains(f.logs.String(), secret) {
		t.Fatalf("resolution note leaked into logs:\n%s", f.logs.String())
	}
}
