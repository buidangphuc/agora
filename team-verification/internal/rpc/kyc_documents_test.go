package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	commonv1 "github.com/buidangphuc/team-verification/generated/platform/common/v1"
	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/objects"
	"github.com/buidangphuc/team-verification/internal/rpc"
	"github.com/buidangphuc/team-verification/internal/store/storetest"
	"github.com/buidangphuc/team-verification/internal/verification"
)

const (
	stPending     = verificationv1.VerificationStatus_VERIFICATION_STATUS_PENDING
	stVerified    = verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED
	stRejected    = verificationv1.VerificationStatus_VERIFICATION_STATUS_REJECTED
	stNotSubmit   = verificationv1.VerificationStatus_VERIFICATION_STATUS_NOT_SUBMITTED
	stUnspecified = verificationv1.VerificationStatus_VERIFICATION_STATUS_UNSPECIFIED
	jpeg          = "image/jpeg"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type kycEnv struct {
	t     *testing.T
	h     *rpc.Handler
	objs  *objects.Fake
	audit *fakeAudit
	clk   *testClock
}

func newKyc(t *testing.T) *kycEnv {
	t.Helper()
	clk := &testClock{now: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)}
	objs := objects.NewFake()
	audit := &fakeAudit{}
	svc := verification.NewVerificationService(storetest.NewInMemoryKycRepo(),
		verification.WithObjects(objs), verification.WithClock(clk.Now))
	h, err := rpc.NewHandler(svc, audit, quiet())
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return &kycEnv{t: t, h: h, objs: objs, audit: audit, clk: clk}
}

func (e *kycEnv) grantReq(ctx context.Context, ct string, size int64) (*verificationv1.CreateKycUploadUrlResponse, error) {
	return e.h.CreateKycUploadUrl(ctx, &verificationv1.CreateKycUploadUrlRequest{ContentType: ct, SizeBytes: size})
}

func (e *kycEnv) grant(ctx context.Context) string {
	e.t.Helper()
	r, err := e.grantReq(ctx, jpeg, 1024)
	if err != nil {
		e.t.Fatalf("CreateKycUploadUrl: %v", err)
	}
	return r.GetDocRef()
}

func (e *kycEnv) upload(ctx context.Context) string {
	e.t.Helper()
	ref := e.grant(ctx)
	e.objs.Put(ref, jpeg, 1024)
	return ref
}

func (e *kycEnv) submitRefs(ctx context.Context, refs ...string) (*verificationv1.SubmitKycResponse, error) {
	return e.h.SubmitKyc(ctx, &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: refs[0], AdditionalDocRefs: refs[1:]})
}

func (e *kycEnv) submit(ctx context.Context, n int) (string, []string) {
	e.t.Helper()
	refs := make([]string, n)
	for i := range refs {
		refs[i] = e.upload(ctx)
	}
	r, err := e.submitRefs(ctx, refs...)
	if err != nil {
		e.t.Fatalf("SubmitKyc: %v", err)
	}
	return r.GetId(), refs
}

func (e *kycEnv) review(ctx context.Context, id, decision, reason string) error {
	_, err := e.h.ReviewKyc(ctx, &verificationv1.ReviewKycRequest{Id: id, Decision: decision, ReasonCode: reason})
	return err
}

func (e *kycEnv) status(ctx context.Context, userID string) *verificationv1.GetVerificationStatusResponse {
	e.t.Helper()
	r, err := e.h.GetVerificationStatus(ctx, &verificationv1.GetVerificationStatusRequest{UserId: userID})
	if err != nil {
		e.t.Fatalf("GetVerificationStatus: %v", err)
	}
	return r
}

func (e *kycEnv) list(ctx context.Context, st verificationv1.VerificationStatus, cursor string, size int32) (*verificationv1.ListKycSubmissionsResponse, error) {
	return e.h.ListKycSubmissions(ctx, &verificationv1.ListKycSubmissionsRequest{
		Status: st,
		Page:   &commonv1.PageRequest{Cursor: cursor, PageSize: size},
	})
}

func (e *kycEnv) assertNoSubmission(user string) {
	e.t.Helper()
	if got := e.status(buyer(user), "").GetStatus(); got != stNotSubmit {
		e.t.Fatalf("expected no submission for %s, status = %v", user, got)
	}
}

func (e *kycEnv) assertNoAudit() {
	e.t.Helper()
	if n := e.audit.count(); n != 0 {
		e.t.Fatalf("want 0 audit events, got %d", n)
	}
}

// Scenario: Seller receives an upload URL / Maximum size is accepted.
func TestCreateKycUploadUrlIssuesGrantForCaller_SellerReceivesUploadUrl(t *testing.T) {
	e := newKyc(t)
	for _, c := range []struct {
		ct   string
		size int64
	}{{jpeg, 1048576}, {"image/png", 1}, {"application/pdf", 5242880}} {
		r, err := e.grantReq(buyer("seller_1"), c.ct, c.size)
		if err != nil {
			t.Fatalf("%s %d: %v", c.ct, c.size, err)
		}
		if !strings.HasPrefix(r.GetDocRef(), "kyc/seller_1/") || len(r.GetDocRef()) <= len("kyc/seller_1/") {
			t.Fatalf("doc_ref = %q, want kyc/seller_1/<uuid>", r.GetDocRef())
		}
		if r.GetUploadUrl() == "" {
			t.Fatal("upload_url is empty")
		}
		if got, want := r.GetExpiresAt().AsTime(), e.clk.Now().Add(15*time.Minute); !got.Equal(want) {
			t.Fatalf("expires_at = %v, want %v", got, want)
		}
	}
}

// Scenario: Invalid type or size is rejected.
func TestCreateKycUploadUrlInvalidTypeOrSizeRejected(t *testing.T) {
	e := newKyc(t)
	for name, c := range map[string]struct {
		ct   string
		size int64
	}{
		"gif":          {"image/gif", 1024},
		"empty type":   {"", 1024},
		"zero size":    {jpeg, 0},
		"negative":     {jpeg, -1},
		"over 5 MiB":   {jpeg, 5242881},
		"uppercase ct": {"IMAGE/JPEG", 1024},
	} {
		_, err := e.grantReq(buyer("seller_1"), c.ct, c.size)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		wantCode(t, err, codes.InvalidArgument)
	}
	for i := 0; i < 10; i++ {
		e.grant(buyer("seller_1"))
	}
}

// Scenario: Open grants are capped.
func TestCreateKycUploadUrlOpenGrantsAreCapped(t *testing.T) {
	e := newKyc(t)
	for i := 0; i < 10; i++ {
		e.grant(buyer("seller_1"))
	}
	_, err := e.grantReq(buyer("seller_1"), jpeg, 1024)
	wantCode(t, err, codes.ResourceExhausted)
	e.grant(buyer("seller_2"))

	e.clk.advance(16 * time.Minute)
	e.grant(buyer("seller_1"))
}

// Scenario: Open grants are capped (used grants are not open).
func TestCreateKycUploadUrlUsedGrantsDoNotCountTowardsCap(t *testing.T) {
	e := newKyc(t)
	e.submit(buyer("seller_1"), 1)
	for i := 0; i < 10; i++ {
		e.grant(buyer("seller_1"))
	}
	_, err := e.grantReq(buyer("seller_1"), jpeg, 1024)
	wantCode(t, err, codes.ResourceExhausted)
}

// Scenario: Anonymous and service callers cannot request an upload URL.
func TestCreateKycUploadUrlAnonymousAndServiceDenied(t *testing.T) {
	e := newKyc(t)
	_, err := e.grantReq(anonymous(), jpeg, 1024)
	wantCode(t, err, codes.Unauthenticated)
	_, err = e.grantReq(ctxAs("svc", "service", "verification.read,admin"), jpeg, 1024)
	wantCode(t, err, codes.PermissionDenied)
}

// Scenario: Submit with one uploaded document.
func TestSubmitKycWithOneUploadedDocument(t *testing.T) {
	e := newKyc(t)
	ref := e.upload(buyer("seller_1"))
	r, err := e.submitRefs(buyer("seller_1"), ref)
	if err != nil {
		t.Fatalf("SubmitKyc: %v", err)
	}
	if r.GetStatus() != stPending || r.GetId() == "" {
		t.Fatalf("response = %v", r)
	}
	if got := e.status(buyer("seller_1"), "").GetStatus(); got != stPending {
		t.Fatalf("status = %v, want PENDING", got)
	}
	if !e.objs.Has(ref) {
		t.Fatal("accepted object was removed")
	}
}

// Scenario: Submit with three documents.
func TestSubmitKycWithThreeDocuments(t *testing.T) {
	e := newKyc(t)
	e.submit(buyer("seller_1"), 3)
	l, err := e.list(admin("adm"), stPending, "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(l.GetSubmissions()) != 1 || l.GetSubmissions()[0].GetDocumentCount() != 3 {
		t.Fatalf("submissions = %v", l.GetSubmissions())
	}
}

// Scenario: More than three documents are rejected.
func TestSubmitKycMoreThanThreeDocumentsRejected(t *testing.T) {
	e := newKyc(t)
	refs := []string{e.upload(buyer("seller_1")), e.upload(buyer("seller_1")), e.upload(buyer("seller_1")), e.upload(buyer("seller_1"))}
	_, err := e.submitRefs(buyer("seller_1"), refs...)
	wantCode(t, err, codes.InvalidArgument)
	e.assertNoSubmission("seller_1")
}

// Scenario: Key issued to another user is rejected.
func TestSubmitKycKeyIssuedToAnotherUserRejected(t *testing.T) {
	e := newKyc(t)
	foreign := e.upload(buyer("seller_a"))
	_, errForeign := e.submitRefs(buyer("seller_b"), foreign)
	wantCode(t, errForeign, codes.InvalidArgument)
	_, errUnknown := e.submitRefs(buyer("seller_b"), "kyc/seller_b/00000000-0000-4000-8000-000000000000")
	wantCode(t, errUnknown, codes.InvalidArgument)
	if errForeign.Error() != errUnknown.Error() {
		t.Fatalf("foreign %q differs from unknown %q", errForeign, errUnknown)
	}
	e.assertNoSubmission("seller_b")
	if _, err := e.submitRefs(buyer("seller_a"), foreign); err != nil {
		t.Fatalf("owner can no longer submit: %v", err)
	}
}

// Scenario: Free-text doc_ref is rejected.
func TestSubmitKycFreeTextDocRefRejected(t *testing.T) {
	e := newKyc(t)
	for _, ref := range []string{"my-id-card.jpg", "kyc/seller_1/not-a-uuid", "../kyc/seller_1/x"} {
		_, err := e.submitRefs(buyer("seller_1"), ref)
		wantCode(t, err, codes.InvalidArgument)
	}
	e.assertNoSubmission("seller_1")
}

// Scenario: A key cannot be submitted twice (same key listed twice).
func TestSubmitKycDuplicateKeyInRequestRejected(t *testing.T) {
	e := newKyc(t)
	ref := e.upload(buyer("seller_1"))
	_, err := e.submitRefs(buyer("seller_1"), ref, ref)
	wantCode(t, err, codes.InvalidArgument)
	e.assertNoSubmission("seller_1")
}

// Scenario: Granted but not uploaded key is rejected.
func TestSubmitKycGrantedButNotUploadedRejected(t *testing.T) {
	e := newKyc(t)
	ref := e.grant(buyer("seller_1"))
	_, err := e.submitRefs(buyer("seller_1"), ref)
	wantCode(t, err, codes.FailedPrecondition)
	e.assertNoSubmission("seller_1")
	e.objs.Put(ref, jpeg, 1024)
	if _, err := e.submitRefs(buyer("seller_1"), ref); err != nil {
		t.Fatalf("grant was consumed by the failed submit: %v", err)
	}
}

// Scenario: Oversize or mismatching object is rejected and removed.
func TestSubmitKycOversizeOrMismatchingObjectRejectedAndRemoved(t *testing.T) {
	for name, stored := range map[string]objects.Info{
		"size differs":         {Size: 2048, ContentType: jpeg},
		"over 5 MiB":           {Size: 5242881, ContentType: jpeg},
		"content type differs": {Size: 1024, ContentType: "application/pdf"},
	} {
		e := newKyc(t)
		good := e.upload(buyer("seller_1"))
		bad := e.grant(buyer("seller_1"))
		e.objs.Put(bad, stored.ContentType, stored.Size)
		_, err := e.submitRefs(buyer("seller_1"), good, bad)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		wantCode(t, err, codes.FailedPrecondition)
		if e.objs.Has(bad) {
			t.Fatalf("%s: mismatching object still exists", name)
		}
		e.assertNoSubmission("seller_1")
		if _, err := e.submitRefs(buyer("seller_1"), good); err != nil {
			t.Fatalf("%s: valid key was consumed by the failed submit: %v", name, err)
		}
	}
}

// Scenario: Expired grant is rejected.
func TestSubmitKycExpiredGrantRejected(t *testing.T) {
	e := newKyc(t)
	ref := e.upload(buyer("seller_1"))
	e.clk.advance(16 * time.Minute)
	_, err := e.submitRefs(buyer("seller_1"), ref)
	wantCode(t, err, codes.FailedPrecondition)
	e.assertNoSubmission("seller_1")
}

// Scenario: Submit with one uploaded document (a grant inside its window is accepted).
func TestSubmitKycGrantInsideWindowAccepted(t *testing.T) {
	e := newKyc(t)
	ref := e.upload(buyer("seller_1"))
	e.clk.advance(14 * time.Minute)
	if _, err := e.submitRefs(buyer("seller_1"), ref); err != nil {
		t.Fatalf("SubmitKyc: %v", err)
	}
}

// Scenario: Anonymous caller cannot submit.
func TestSubmitKycAnonymousUnauthenticated(t *testing.T) {
	e := newKyc(t)
	_, err := e.submitRefs(anonymous(), "kyc/anonymous/00000000-0000-4000-8000-000000000000")
	wantCode(t, err, codes.Unauthenticated)
}

// Scenario: Seller who never submitted reads NOT_SUBMITTED.
func TestGetVerificationStatusNotSubmitted(t *testing.T) {
	e := newKyc(t)
	r := e.status(buyer("seller_1"), "")
	if r.GetStatus() != stNotSubmit || r.GetBadge() || r.GetRejectReasonCode() != "" {
		t.Fatalf("response = %v", r)
	}
}

// Scenario: Rejected seller sees the reason code.
func TestGetVerificationStatusRejectedShowsReasonCode(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	if err := e.review(admin("adm"), id, "reject", "document_unreadable"); err != nil {
		t.Fatalf("review: %v", err)
	}
	for name, r := range map[string]*verificationv1.GetVerificationStatusResponse{
		"owner": e.status(buyer("seller_1"), ""),
		"admin": e.status(admin("adm"), "seller_1"),
	} {
		if r.GetStatus() != stRejected || r.GetRejectReasonCode() != "document_unreadable" {
			t.Fatalf("%s: response = %v", name, r)
		}
	}
}

// Scenario: Reject without a reason code is rejected.
func TestReviewKycRejectWithoutValidReasonCodeRejected(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	for name, code := range map[string]string{
		"empty":     "",
		"bad chars": "Bad Reason!",
		"too long":  strings.Repeat("a", 65),
		"uppercase": "Document_Expired",
	} {
		err := e.review(admin("adm"), id, "reject", code)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		wantCode(t, err, codes.InvalidArgument)
	}
	if got := e.status(buyer("seller_1"), "").GetStatus(); got != stPending {
		t.Fatalf("status = %v, want PENDING", got)
	}
	e.assertNoAudit()
	if err := e.review(admin("adm"), id, "reject", strings.Repeat("a", 64)); err != nil {
		t.Fatalf("64-char reason code refused: %v", err)
	}
}

// Scenario: Approve with a reason code is rejected.
func TestReviewKycApproveWithReasonCodeRejected(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	wantCode(t, e.review(admin("adm"), id, "approve", "document_expired"), codes.InvalidArgument)
	if got := e.status(buyer("seller_1"), "").GetStatus(); got != stPending {
		t.Fatalf("status = %v, want PENDING", got)
	}
	e.assertNoAudit()
}

// Scenario: Reason code is audited.
func TestReviewKycReasonCodeIsAudited(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	if err := e.review(admin("adm"), id, "reject", "document_expired"); err != nil {
		t.Fatalf("review: %v", err)
	}
	if len(e.audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(e.audit.events))
	}
	ev := e.audit.events[0]
	if ev.Action != "kyc.review" || ev.TargetID != id || ev.Metadata["reason_code"] != "document_expired" {
		t.Fatalf("event = %+v", ev)
	}

	id2, _ := e.submit(buyer("seller_2"), 1)
	if err := e.review(admin("adm"), id2, "approve", ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if v, ok := e.audit.events[1].Metadata["reason_code"]; !ok || v != "" {
		t.Fatalf("approve metadata = %v, want empty reason_code present", e.audit.events[1].Metadata)
	}
}

// Scenario: Another user cannot read a status.
func TestGetVerificationStatusAnotherUserDenied(t *testing.T) {
	e := newKyc(t)
	e.submit(buyer("seller_b"), 1)
	_, err := e.h.GetVerificationStatus(buyer("seller_a"), &verificationv1.GetVerificationStatusRequest{UserId: "seller_b"})
	wantCode(t, err, codes.PermissionDenied)
}

// Scenario: Payment service reads any seller's status.
func TestGetVerificationStatusServiceWithScopeReadsAnyUser(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	if err := e.review(admin("adm"), id, "reject", "document_unreadable"); err != nil {
		t.Fatalf("review: %v", err)
	}
	r := e.status(ctxAs("team-payment", "service", "verification.read"), "seller_1")
	if r.GetStatus() != stRejected || r.GetRejectReasonCode() != "" {
		t.Fatalf("response = %v", r)
	}
}

// Scenario: Service principal without the scope is refused.
func TestGetVerificationStatusServiceWithoutScopeDenied(t *testing.T) {
	e := newKyc(t)
	e.submit(buyer("seller_1"), 1)
	_, err := e.h.GetVerificationStatus(ctxAs("team-payment", "service", "listing.read"),
		&verificationv1.GetVerificationStatusRequest{UserId: "seller_1"})
	wantCode(t, err, codes.PermissionDenied)
}

// Scenario: Pending queue is paged oldest first.
func TestListKycSubmissionsPendingQueuePagedOldestFirst(t *testing.T) {
	e := newKyc(t)
	var order []string
	for i := 0; i < 25; i++ {
		id, _ := e.submit(buyer(fmt.Sprintf("seller_%02d", i)), 1)
		order = append(order, id)
		e.clk.advance(time.Second)
	}
	var got []string
	var sizes []int
	cursor := ""
	for page := 0; page < 5; page++ {
		r, err := e.list(admin("adm"), stPending, cursor, 10)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		sizes = append(sizes, len(r.GetSubmissions()))
		for _, s := range r.GetSubmissions() {
			got = append(got, s.GetId())
		}
		cursor = r.GetPage().GetNextCursor()
		if cursor == "" {
			break
		}
	}
	if fmt.Sprint(sizes) != "[10 10 5]" {
		t.Fatalf("page sizes = %v, want [10 10 5]", sizes)
	}
	if fmt.Sprint(got) != fmt.Sprint(order) {
		t.Fatalf("order = %v, want %v", got, order)
	}
}

// Scenario: Pending queue is paged oldest first (equal created_at ties break on id, no duplicates).
func TestListKycSubmissionsEqualCreatedAtNoDuplicatesAcrossPages(t *testing.T) {
	e := newKyc(t)
	for i := 0; i < 5; i++ {
		e.submit(buyer(fmt.Sprintf("seller_%d", i)), 1)
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 6; page++ {
		r, err := e.list(admin("adm"), stPending, cursor, 2)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, s := range r.GetSubmissions() {
			if seen[s.GetId()] {
				t.Fatalf("duplicate %s", s.GetId())
			}
			seen[s.GetId()] = true
		}
		if cursor = r.GetPage().GetNextCursor(); cursor == "" {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d submissions, want 5", len(seen))
	}
}

// Scenario: Pending queue is paged oldest first (status filter and default page size).
func TestListKycSubmissionsFilterAndDefaultPageSize(t *testing.T) {
	e := newKyc(t)
	var verifiedID string
	for i := 0; i < 25; i++ {
		id, _ := e.submit(buyer(fmt.Sprintf("seller_%02d", i)), 1)
		if i == 0 {
			verifiedID = id
		}
		e.clk.advance(time.Second)
	}
	if err := e.review(admin("adm"), verifiedID, "approve", ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	all, err := e.list(admin("adm"), stUnspecified, "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all.GetSubmissions()) != 20 || all.GetPage().GetNextCursor() == "" {
		t.Fatalf("default page: %d rows, cursor %q", len(all.GetSubmissions()), all.GetPage().GetNextCursor())
	}
	ver, err := e.list(admin("adm"), stVerified, "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ver.GetSubmissions()) != 1 || ver.GetSubmissions()[0].GetId() != verifiedID || ver.GetSubmissions()[0].GetStatus() != stVerified {
		t.Fatalf("verified filter = %v", ver.GetSubmissions())
	}
	pend, err := e.list(admin("adm"), stPending, "", 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pend.GetSubmissions()) != 24 {
		t.Fatalf("pending = %d, want 24", len(pend.GetSubmissions()))
	}
}

// Scenario: Page size is clamped and bad cursor rejected.
func TestListKycSubmissionsPageSizeClampedAndBadCursorRejected(t *testing.T) {
	e := newKyc(t)
	for i := 0; i < 101; i++ {
		e.submit(buyer(fmt.Sprintf("seller_%03d", i)), 1)
	}
	r, err := e.list(admin("adm"), stPending, "", 500)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(r.GetSubmissions()) != 100 {
		t.Fatalf("rows = %d, want 100", len(r.GetSubmissions()))
	}
	_, err = e.list(admin("adm"), stPending, "garbage", 10)
	wantCode(t, err, codes.InvalidArgument)
}

// Scenario: Filtering by NOT_SUBMITTED is rejected.
func TestListKycSubmissionsFilterNotSubmittedRejected(t *testing.T) {
	e := newKyc(t)
	_, err := e.list(admin("adm"), stNotSubmit, "", 0)
	wantCode(t, err, codes.InvalidArgument)
}

// Scenario: Non-admin cannot list.
func TestListKycSubmissionsNonAdminDenied(t *testing.T) {
	e := newKyc(t)
	_, err := e.list(buyer("seller_1"), stPending, "", 0)
	wantCode(t, err, codes.PermissionDenied)
	_, err = e.list(anonymous(), stPending, "", 0)
	wantCode(t, err, codes.Unauthenticated)
}

func (e *kycEnv) docURL(ctx context.Context, id string, idx int32) (*verificationv1.GetKycDocumentUrlResponse, error) {
	return e.h.GetKycDocumentUrl(ctx, &verificationv1.GetKycDocumentUrlRequest{SubmissionId: id, Index: idx})
}

// Scenario: Admin opens a document.
func TestGetKycDocumentUrlAdminOpensDocument(t *testing.T) {
	e := newKyc(t)
	id, refs := e.submit(buyer("seller_1"), 3)
	for idx, ref := range refs {
		e.audit.mu.Lock()
		e.audit.events = nil
		e.audit.calls = nil
		e.audit.mu.Unlock()
		r, err := e.docURL(admin("adm"), id, int32(idx))
		if err != nil {
			t.Fatalf("index %d: %v", idx, err)
		}
		if !strings.Contains(r.GetUrl(), ref) {
			t.Fatalf("index %d: url %q does not point at %q", idx, r.GetUrl(), ref)
		}
		if got, want := r.GetExpiresAt().AsTime(), e.clk.Now().Add(5*time.Minute); !got.Equal(want) {
			t.Fatalf("expires_at = %v, want %v", got, want)
		}
		if len(e.audit.events) != 1 {
			t.Fatalf("audit events = %d, want 1", len(e.audit.events))
		}
		ev := e.audit.events[0]
		if ev.Action != "kyc.document_view" || ev.ActorID != "adm" || ev.TargetType != "kyc_submission" || ev.TargetID != id {
			t.Fatalf("event = %+v", ev)
		}
		if ev.Metadata["user_id"] != "seller_1" || ev.Metadata["index"] != fmt.Sprint(idx) {
			t.Fatalf("metadata = %v", ev.Metadata)
		}
		for k, v := range ev.Metadata {
			if strings.Contains(v, ref) || strings.Contains(k, "kyc/") {
				t.Fatalf("audit metadata leaks the object key: %v", ev.Metadata)
			}
		}
	}
}

// Scenario: Audit outage blocks the document.
func TestGetKycDocumentUrlAuditOutageBlocksDocument(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	e.audit.err = errors.New("audit down")
	r, err := e.docURL(admin("adm"), id, 0)
	wantCode(t, err, codes.Unavailable)
	if r.GetUrl() != "" {
		t.Fatalf("url returned during audit outage: %q", r.GetUrl())
	}
}

// Scenario: Index out of range or unknown submission.
func TestGetKycDocumentUrlIndexOutOfRangeOrUnknownSubmission(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 3)
	for _, idx := range []int32{3, -1, 100} {
		_, err := e.docURL(admin("adm"), id, idx)
		wantCode(t, err, codes.InvalidArgument)
	}
	_, err := e.docURL(admin("adm"), "kyc_missing", 0)
	wantCode(t, err, codes.NotFound)
	e.assertNoAudit()
}

// Scenario: Non-admin cannot obtain a document URL.
func TestGetKycDocumentUrlNonAdminDenied(t *testing.T) {
	e := newKyc(t)
	id, _ := e.submit(buyer("seller_1"), 1)
	_, err := e.docURL(buyer("seller_1"), id, 0)
	wantCode(t, err, codes.PermissionDenied)
	_, err = e.docURL(buyer("seller_2"), id, 0)
	wantCode(t, err, codes.PermissionDenied)
	_, err = e.docURL(anonymous(), id, 0)
	wantCode(t, err, codes.Unauthenticated)
	e.assertNoAudit()
}

func (e *kycEnv) badges(ctx context.Context, ids ...string) (*verificationv1.GetPublicBadgesResponse, error) {
	return e.h.GetPublicBadges(ctx, &verificationv1.GetPublicBadgesRequest{UserIds: ids})
}

// Scenario: Verified seller shows a badge.
func TestGetPublicBadgesVerifiedSellerShowsBadge(t *testing.T) {
	e := newKyc(t)
	vid, _ := e.submit(buyer("verified"), 1)
	e.submit(buyer("pending"), 1)
	rid, _ := e.submit(buyer("rejected"), 1)
	if err := e.review(admin("adm"), vid, "approve", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.review(admin("adm"), rid, "reject", "document_unreadable"); err != nil {
		t.Fatal(err)
	}
	r, err := e.badges(anonymous(), "verified", "pending", "rejected", "unknown")
	if err != nil {
		t.Fatalf("GetPublicBadges: %v", err)
	}
	got := map[string]bool{}
	for _, b := range r.GetBadges() {
		got[b.GetUserId()] = b.GetVerified()
	}
	if len(r.GetBadges()) != 4 || len(got) != 4 {
		t.Fatalf("badges = %v", r.GetBadges())
	}
	if !got["verified"] || got["pending"] || got["rejected"] || got["unknown"] {
		t.Fatalf("badges = %v", got)
	}
}

// Scenario: Verified seller shows a badge (distinct ids, latest submission decides).
func TestGetPublicBadgesDedupesAndUsesLatestSubmission(t *testing.T) {
	e := newKyc(t)
	vid, _ := e.submit(buyer("seller_1"), 1)
	if err := e.review(admin("adm"), vid, "approve", ""); err != nil {
		t.Fatal(err)
	}
	r, err := e.badges(anonymous(), "seller_1", "seller_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GetBadges()) != 1 || !r.GetBadges()[0].GetVerified() {
		t.Fatalf("badges = %v", r.GetBadges())
	}
	e.clk.advance(time.Minute)
	e.submit(buyer("seller_1"), 1)
	r, err = e.badges(anonymous(), "seller_1")
	if err != nil {
		t.Fatal(err)
	}
	if r.GetBadges()[0].GetVerified() {
		t.Fatal("badge still shown while the latest submission is PENDING")
	}
}

// Scenario: Empty or oversized id list is rejected.
func TestGetPublicBadgesEmptyOrOversizedListRejected(t *testing.T) {
	e := newKyc(t)
	_, err := e.badges(anonymous())
	wantCode(t, err, codes.InvalidArgument)
	_, err = e.badges(anonymous(), manyIDs(51)...)
	wantCode(t, err, codes.InvalidArgument)
}

// Scenario: Boundary of 50 ids is accepted.
func TestGetPublicBadgesBoundaryOf50IdsAccepted(t *testing.T) {
	e := newKyc(t)
	r, err := e.badges(anonymous(), manyIDs(50)...)
	if err != nil {
		t.Fatalf("GetPublicBadges: %v", err)
	}
	if len(r.GetBadges()) != 50 {
		t.Fatalf("badges = %d, want 50", len(r.GetBadges()))
	}
}

func manyIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("user_%02d", i)
	}
	return ids
}
