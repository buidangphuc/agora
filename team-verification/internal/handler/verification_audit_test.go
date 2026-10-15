package handler_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/handler"
	"github.com/buidangphuc/team-verification/internal/repository"
	"github.com/buidangphuc/team-verification/internal/service"
)

// auditCall is one recorded WriteKycReviewEvent invocation.
type auditCall struct {
	ActorID, SubmissionID, UserID, Decision string
}

// eventLog records the order of audit writes and repository updates.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// fakeAudit is the AuditWriter fake (team-audit is the external boundary).
type fakeAudit struct {
	mu    sync.Mutex
	calls []auditCall
	err   error
	log   *eventLog
}

func (f *fakeAudit) WriteKycReviewEvent(_ context.Context, actorID, submissionID, userID, decision string) error {
	if f.log != nil {
		f.log.add("audit")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, auditCall{actorID, submissionID, userID, decision})
	return f.err
}

func (f *fakeAudit) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeAudit) first(t *testing.T) auditCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("expected an audit call, got none")
	}
	return f.calls[0]
}

// spyRepo wraps the in-memory repo, logging updates and optionally failing them.
type spyRepo struct {
	repository.KycRepository
	log        *eventLog
	failUpdate bool
}

func (r *spyRepo) UpdateStatus(ctx context.Context, id string, st repository.Status, at time.Time) (*repository.Submission, error) {
	r.log.add("update")
	if r.failUpdate {
		return nil, errors.New("db down")
	}
	return r.KycRepository.UpdateStatus(ctx, id, st, at)
}

type env struct {
	h     *handler.VerificationHandler
	repo  *spyRepo
	audit *fakeAudit
	log   *eventLog
}

func newEnv() *env {
	log := &eventLog{}
	repo := &spyRepo{KycRepository: repository.NewInMemoryKycRepo(), log: log}
	audit := &fakeAudit{log: log}
	svc := service.NewVerificationService(repo)
	return &env{h: handler.NewVerificationHandler(svc, audit), repo: repo, audit: audit, log: log}
}

func (e *env) review(ctx context.Context, id, decision string) (*verificationv1.ReviewKycResponse, error) {
	return e.h.ReviewKyc(ctx, &verificationv1.ReviewKycRequest{Id: id, Decision: decision})
}

// assertPending checks the stored submission is untouched (PENDING, reviewed_at unset).
func (e *env) assertPending(t *testing.T, id string) {
	t.Helper()
	sub, err := e.repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if sub.Status != repository.StatusPending || sub.ReviewedAt != nil {
		t.Fatalf("submission changed: status=%v reviewed_at=%v", sub.Status, sub.ReviewedAt)
	}
}

func (e *env) assertNoUpdate(t *testing.T) {
	t.Helper()
	for _, ev := range e.log.snapshot() {
		if ev == "update" {
			t.Fatalf("repository update must not run, log=%v", e.log.snapshot())
		}
	}
}

// Scenario: Admin approval writes one audit event.
func TestReviewKycWritesAuditEvent(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	rev, err := e.review(admin("adm-1"), id, "approve")
	if err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	if rev.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("want VERIFIED, got %v", rev.GetStatus())
	}
	if n := e.audit.count(); n != 1 {
		t.Fatalf("want exactly 1 audit call, got %d", n)
	}
	want := auditCall{ActorID: "adm-1", SubmissionID: id, UserID: "u-1", Decision: "approve"}
	if got := e.audit.first(t); got != want {
		t.Fatalf("audit call = %+v, want %+v", got, want)
	}
}

// Scenario: Decision alias is normalized in the event.
func TestReviewKycAuditDecisionNormalized(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	rev, err := e.review(admin("adm-1"), id, " Rejected ")
	if err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	if rev.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED {
		t.Fatalf("want REJECTED, got %v", rev.GetStatus())
	}
	if got := e.audit.first(t).Decision; got != "reject" {
		t.Fatalf("audit decision = %q, want %q", got, "reject")
	}
}

// Scenario: Audit is written before the review is applied.
func TestReviewKycAuditBeforeUpdate(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	if _, err := e.review(admin("adm-1"), id, "approve"); err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	got := e.log.snapshot()
	if len(got) != 2 || got[0] != "audit" || got[1] != "update" {
		t.Fatalf("call order = %v, want [audit update]", got)
	}
}

// Scenario: Actor comes from the principal, not the request.
func TestReviewKycAuditActorAndNoDocumentData(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	if _, err := e.review(admin("adm-1"), id, "approve"); err != nil {
		t.Fatalf("ReviewKyc: %v", err)
	}
	c := e.audit.first(t)
	if c.ActorID != "adm-1" {
		t.Fatalf("actor = %q, want adm-1", c.ActorID)
	}
	// The writer signature carries no document fields; the submission's doc ref must
	// never leak into any argument.
	for _, v := range []string{c.ActorID, c.SubmissionID, c.UserID, c.Decision} {
		if v == "national_id" || v == "mock-ref" {
			t.Fatalf("document data leaked into audit call: %+v", c)
		}
	}
}

// Scenario: Non-admin caller is denied without an audit write; Non-admin is denied and nothing is written.
func TestReviewKycNonAdminNoAudit(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("alice"))
	_, err := e.review(buyer("bob"), id, "approve")
	wantCode(t, err, codes.PermissionDenied)
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
	e.assertPending(t, id)
}

// Scenario: Anonymous and identity-less callers are refused.
func TestReviewKycAnonymousNoAudit(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("alice"))
	for name, ctx := range map[string]context.Context{
		"anonymous":   anonymous(),
		"no identity": context.Background(),
		"empty id":    ctxAs("", "user", "admin"),
	} {
		_, err := e.review(ctx, id, "approve")
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: want Unauthenticated, got %v", name, err)
		}
	}
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
	e.assertPending(t, id)
}

// Scenario: Admin cannot review their own submission.
func TestReviewKycSelfReviewNoAudit(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, admin("adm-1"))
	_, err := e.review(admin("adm-1"), id, "approve")
	wantCode(t, err, codes.PermissionDenied)
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
	e.assertPending(t, id)
}

// Scenario: Unknown submission.
func TestReviewKycNotFoundNoAudit(t *testing.T) {
	e := newEnv()
	_, err := e.review(admin("adm-1"), "kyc_missing", "approve")
	wantCode(t, err, codes.NotFound)
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
}

// Scenario: Empty submission id is rejected.
func TestReviewKycEmptyIDNoAudit(t *testing.T) {
	e := newEnv()
	_, err := e.review(admin("adm-1"), "", "approve")
	wantCode(t, err, codes.InvalidArgument)
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
}

// Scenario: Invalid decision is rejected before audit.
func TestReviewKycInvalidDecisionNoAudit(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	for _, d := range []string{"maybe", ""} {
		_, err := e.review(admin("adm-1"), id, d)
		wantCode(t, err, codes.InvalidArgument)
	}
	if n := e.audit.count(); n != 0 {
		t.Fatalf("want 0 audit calls, got %d", n)
	}
	e.assertNoUpdate(t)
	e.assertPending(t, id)
}

// Scenario: Already reviewed submission is refused before audit.
func TestReviewKycAlreadyReviewedNoAudit(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	if _, err := e.review(admin("adm-1"), id, "approve"); err != nil {
		t.Fatalf("first review: %v", err)
	}
	before := e.audit.count()
	_, err := e.review(admin("adm-2"), id, "reject")
	wantCode(t, err, codes.FailedPrecondition)
	if n := e.audit.count(); n != before {
		t.Fatalf("refused review wrote an audit event: %d -> %d", before, n)
	}
	st, _ := e.h.GetVerificationStatus(buyer("u-1"), &verificationv1.GetVerificationStatusRequest{})
	if st.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("status changed: %v", st.GetStatus())
	}
}

// Scenario: Audit error leaves the submission pending.
func TestReviewKycAuditErrorUnavailableNoChange(t *testing.T) {
	e := newEnv()
	e.audit.err = status.Error(codes.Internal, "audit db down")
	id := submit(t, e.h, buyer("u-1"))
	_, err := e.review(admin("adm-1"), id, "approve")
	wantCode(t, err, codes.Unavailable)
	e.assertNoUpdate(t)
	e.assertPending(t, id)
	st, err := e.h.GetVerificationStatus(buyer("u-1"), &verificationv1.GetVerificationStatusRequest{})
	if err != nil || st.GetBadge() || st.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
		t.Fatalf("want PENDING/no badge, got %v err=%v", st, err)
	}
	if n := e.audit.count(); n != 1 {
		t.Fatalf("audit must be a single attempt (no retry), got %d calls", n)
	}
}

// Scenario: Audit timeout error is Unavailable.
func TestReviewKycAuditTimeoutUnavailableNoChange(t *testing.T) {
	for name, aerr := range map[string]error{
		"wrapped context error": fmt.Errorf("write audit event: %w", context.DeadlineExceeded),
		"grpc deadline":         status.Error(codes.DeadlineExceeded, "slow"),
	} {
		e := newEnv()
		e.audit.err = aerr
		id := submit(t, e.h, buyer("u-1"))
		_, err := e.review(admin("adm-1"), id, "approve")
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("%s: want Unavailable, got %v", name, err)
		}
		e.assertPending(t, id)
	}
}

// Scenario: KYC audit: no audit upstream configured.
func TestReviewKycNilAuditUnavailable(t *testing.T) {
	log := &eventLog{}
	repo := &spyRepo{KycRepository: repository.NewInMemoryKycRepo(), log: log}
	h := handler.NewVerificationHandler(service.NewVerificationService(repo), nil)
	id := submit(t, h, buyer("u-1"))
	_, err := h.ReviewKyc(admin("adm-1"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	wantCode(t, err, codes.Unavailable)
	sub, _ := repo.GetByID(context.Background(), id)
	if sub.Status != repository.StatusPending || sub.ReviewedAt != nil {
		t.Fatalf("submission changed: %+v", sub)
	}
}

// Scenario: Update failure after audit then retry.
func TestReviewKycUpdateFailureInternalThenRetry(t *testing.T) {
	e := newEnv()
	id := submit(t, e.h, buyer("u-1"))
	e.repo.failUpdate = true
	_, err := e.review(admin("adm-1"), id, "approve")
	wantCode(t, err, codes.Internal)
	e.repo.failUpdate = false
	rev, err := e.review(admin("adm-1"), id, "approve")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if rev.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		t.Fatalf("retry status = %v, want VERIFIED", rev.GetStatus())
	}
	if n := e.audit.count(); n != 2 {
		t.Fatalf("want 2 audit events (at-least-once), got %d", n)
	}
}

// Scenario: Submit and status work with audit down.
func TestSubmitAndStatusIndependentOfAudit(t *testing.T) {
	failing := newEnv()
	failing.audit.err = status.Error(codes.Unavailable, "down")
	nilAudit := handler.NewVerificationHandler(service.NewVerificationService(repository.NewInMemoryKycRepo()), nil)

	for name, h := range map[string]*handler.VerificationHandler{"failing fake": failing.h, "nil writer": nilAudit} {
		submit(t, h, buyer("u-1"))
		st, err := h.GetVerificationStatus(buyer("u-1"), &verificationv1.GetVerificationStatusRequest{})
		if err != nil {
			t.Fatalf("%s: status: %v", name, err)
		}
		if st.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
			t.Fatalf("%s: want PENDING, got %v", name, st.GetStatus())
		}
	}
	if n := failing.audit.count(); n != 0 {
		t.Fatalf("submit/status must make no audit call, got %d", n)
	}
}
