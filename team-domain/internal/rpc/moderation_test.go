package rpc_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

// Interface contract (task 8): ListingHandler.ModerateListing / ListModerationQueue
// over listing.ListingService.ModerateListing / ListModerationQueue, and
// listing.AuditWriter.WriteListingModerate(ctx, actorID, listingID, sellerID,
// decision, reasonCode, note, fromStatus, toStatus string) error.

var modT1 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

type mhCall struct{ actor, listing, seller, decision, reason, note, from, to string }

type mhAudit struct {
	calls []mhCall
	err   error
}

func (a *mhAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	if ev.Action != "listing.moderate" {
		return a.err
	}
	m := ev.Metadata
	a.calls = append(a.calls, mhCall{ev.ActorID, ev.TargetID, m["seller_id"], m["decision"], m["reason_code"], m["note"], m["from_status"], m["to_status"]})
	return a.err
}

// mhRepo adds the moderation-queue read the Postgres repository provides:
// pending_review only, ordered by (submitted_at, id), keyset by offset cursor.
type mhRepo struct {
	*storetest.InMemoryListingRepository
}

func (r mhRepo) ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (store.Page, error) {
	all, err := r.List(ctx, "", 1000, "pending_review")
	if err != nil {
		return store.Page{}, err
	}
	items := all.Items
	sort.Slice(items, func(i, j int) bool {
		if !items[i].SubmittedAt.Equal(items[j].SubmittedAt) {
			return items[i].SubmittedAt.Before(items[j].SubmittedAt)
		}
		return items[i].ID < items[j].ID
	})
	start := 0
	if cursor != "" {
		for i, l := range items {
			if l.ID == cursor {
				start = i + 1
			}
		}
	}
	size := int(pageSize)
	if size <= 0 || size > 100 {
		size = 50
	}
	end := start + size
	var next string
	if end < len(items) {
		next = items[end-1].ID
	} else {
		end = len(items)
	}
	return store.Page{Items: items[start:end], NextCursor: next, Total: -1}, nil
}

type mhTx struct {
	*storetest.InMemoryTxWriter
	repo           *storetest.InMemoryListingRepository
	beforeModerate func()
}

func (m *mhTx) ModerateTx(ctx context.Context, id, fromStatus string, loadedUpdatedAt time.Time, d store.ModerationUpdate, ev store.EnqueueFn) (store.Listing, error) {
	if m.beforeModerate != nil {
		m.beforeModerate()
	}
	cur, err := m.repo.Get(ctx, id)
	if err != nil {
		return store.Listing{}, err
	}
	if cur.Status != fromStatus || !cur.UpdatedAt.Equal(loadedUpdatedAt) {
		return store.Listing{}, store.ErrStatusChanged
	}
	cur.Status = d.ToStatus
	cur.ModerationReasonCode = d.ReasonCode
	cur.ModerationNote = d.Note
	cur.ModerationDecidedAt = d.DecidedAt
	cur.UpdatedAt = cur.UpdatedAt.Add(time.Second)
	return m.UpdateTx(ctx, cur, ev)
}

type mhFixture struct {
	h     *rpc.ListingHandler
	repo  *storetest.InMemoryListingRepository
	txw   *storetest.InMemoryTxWriter
	tx    *mhTx
	audit *mhAudit
}

func mhListing(id, status string, submitted time.Time) store.Listing {
	return store.Listing{
		ID: id, Title: "t-" + id, Currency: "VND", Price: 100, Status: status, SellerID: "seller-a", Stock: 1,
		UpdatedAt: modT1, SubmittedAt: submitted, ScreenFlags: []string{"keyword:replica"},
	}
}

func newMHFixture(seed ...store.Listing) *mhFixture {
	if len(seed) == 0 {
		seed = []store.Listing{mhListing("L-1", "pending_review", modT1)}
	}
	repo := storetest.NewInMemoryListingRepository(seed...)
	txw := storetest.NewInMemoryTxWriter(repo)
	tx := &mhTx{InMemoryTxWriter: txw, repo: repo}
	fa := &mhAudit{}
	svc := listing.NewListingServiceWithOutbox(mhRepo{repo}, tx).WithAudit(auditx.NewGate(fa, nil))
	return &mhFixture{h: rpc.NewListingHandler(svc, nil, nil), repo: repo, txw: txw, tx: tx, audit: fa}
}

func modReq(id string, d listingv1.ModerationDecision, reason, note string) *listingv1.ModerateListingRequest {
	return &listingv1.ModerateListingRequest{ListingId: id, Decision: d, ReasonCode: reason, Note: note}
}

func (f *mhFixture) assertUnchanged(ctx context.Context, t *testing.T, id, status string) {
	t.Helper()
	l, err := f.repo.Get(ctx, id)
	if err != nil || l.Status != status {
		t.Fatalf("listing %s = %+v, %v; want status %q", id, l, err, status)
	}
	if n := len(f.txw.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

var (
	decReject  = listingv1.ModerationDecision_MODERATION_DECISION_REJECT
	decBan     = listingv1.ModerationDecision_MODERATION_DECISION_BAN
	decApprove = listingv1.ModerationDecision_MODERATION_DECISION_APPROVE
)

// Scenario: Moderation: Reject records reason and note for the seller (handler mapping, admin sees all).
func TestModerateListingRejectResponse(t *testing.T) {
	f := newMHFixture()
	resp, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decReject, "counterfeit", "Fake logo"))
	if err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	l := resp.GetListing()
	if l.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_REJECTED ||
		l.GetModeration().GetReasonCode() != "counterfeit" || l.GetModeration().GetNote() != "Fake logo" ||
		l.GetModeration().GetDecidedAt() == nil {
		t.Fatalf("response listing = %+v", l)
	}
	if len(f.audit.calls) != 1 || f.audit.calls[0] != (mhCall{"admin-1", "L-1", "seller-a", "reject", "counterfeit", "Fake logo", "pending_review", "rejected"}) {
		t.Fatalf("audit calls = %+v", f.audit.calls)
	}
}

// Scenario: Moderation: Approve publishes a pending listing (handler mapping).
func TestModerateListingApproveResponse(t *testing.T) {
	f := newMHFixture()
	resp, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decApprove, "", ""))
	if err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if resp.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED || resp.GetListing().GetModeration().GetDecidedAt() == nil {
		t.Fatalf("response listing = %+v", resp.GetListing())
	}
}

// Scenario: Moderation: Reject of a published listing from a report (anonymous GetListing then NotFound).
func TestModerateListingRejectPublishedHidesFromAnonymous(t *testing.T) {
	f := newMHFixture(mhListing("L-1", "published", modT1))
	if _, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decReject, "misleading", "")); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	_, err := f.h.GetListing(typedCtx("buyer-1", "user", "listing.read"), &listingv1.GetListingRequest{Id: "L-1"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("anonymous GetListing code = %v, want NotFound", status.Code(err))
	}
}

// Scenario: Moderation: ListingChanged carries no note or flags.
func TestModerateListingEventStripsNoteAndFlags(t *testing.T) {
	f := newMHFixture()
	if _, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decReject, "counterfeit", "Fake logo")); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	rows := f.txw.Rows()
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rows[0].Payload, &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	var ev listingv1.ListingChanged
	if err := proto.Unmarshal(env.Payload, &ev); err != nil {
		t.Fatalf("ListingChanged: %v", err)
	}
	if ev.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_UPDATED {
		t.Errorf("change type = %v, want UPDATED", ev.GetChangeType())
	}
	m := ev.GetListing().GetModeration()
	if m.GetReasonCode() != "counterfeit" || m.GetDecidedAt() == nil {
		t.Errorf("event moderation = %+v, want reason counterfeit and decided_at", m)
	}
	if m.GetNote() != "" || len(m.GetScreenFlags()) != 0 {
		t.Errorf("event must carry no note or flags, got note=%q flags=%v", m.GetNote(), m.GetScreenFlags())
	}
	if ev.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_REJECTED {
		t.Errorf("event status = %v, want REJECTED", ev.GetListing().GetStatus())
	}
}

// Scenario: Moderation: Audit down refuses the decision.
func TestModerateListingAuditDownUnavailableNoChange(t *testing.T) {
	for name, aerr := range map[string]error{
		"error":    errors.New("secret upstream detail"),
		"deadline": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			f := newMHFixture()
			f.audit.err = aerr
			_, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decReject, "other", ""))
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("code = %v, want Unavailable (%v)", status.Code(err), err)
			}
			if strings.Contains(status.Convert(err).Message(), "secret upstream detail") {
				t.Errorf("message leaks detail: %q", status.Convert(err).Message())
			}
			f.assertUnchanged(t.Context(), t, "L-1", "pending_review")
		})
	}
}

// Scenario: Moderation: Approve of a held listing is refused (handler code mapping, every refused source).
func TestModerateListingRefusedTransitionFailedPrecondition(t *testing.T) {
	for _, st := range []string{"held", "rejected", "banned", "draft"} {
		t.Run(st, func(t *testing.T) {
			f := newMHFixture(mhListing("L-1", st, modT1))
			_, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decApprove, "", ""))
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
			}
			f.assertUnchanged(t.Context(), t, "L-1", st)
			if len(f.audit.calls) != 0 {
				t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
			}
		})
	}
}

// Scenario: Moderation: Seller edit between check and decision is refused (handler code mapping).
func TestModerateListingStaleLoadFailedPrecondition(t *testing.T) {
	f := newMHFixture()
	f.tx.beforeModerate = func() {
		cur, _ := f.repo.Get(context.Background(), "L-1")
		cur.Title = "seller edited"
		cur.UpdatedAt = cur.UpdatedAt.Add(time.Minute)
		_, _ = f.repo.Update(context.Background(), cur)
	}
	_, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decApprove, "", ""))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (%v)", status.Code(err), err)
	}
	f.assertUnchanged(t.Context(), t, "L-1", "pending_review")
}

// Scenario: Moderation: Unknown listing writes no audit event (handler NotFound).
func TestModerateListingUnknownListingNotFound(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ModerateListing(adminCtx(), modReq("nope", decApprove, "", ""))
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
}

// Scenario: Moderation: Invalid request on an unknown listing is invalid.
func TestModerateListingValidatesBeforeLoad(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ModerateListing(adminCtx(), modReq("nope", decBan, "because", ""))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument (validation before load)", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
}

// Scenario: Moderation: Reject without a reason code is invalid.
func TestModerateListingRejectNeedsReason(t *testing.T) {
	for _, d := range []listingv1.ModerationDecision{decReject, decBan} {
		f := newMHFixture()
		_, err := f.h.ModerateListing(adminCtx(), modReq("L-1", d, "", "note"))
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%v: code = %v, want InvalidArgument", d, status.Code(err))
		}
		if len(f.audit.calls) != 0 {
			t.Fatalf("%v: audit calls = %d, want 0", d, len(f.audit.calls))
		}
		f.assertUnchanged(t.Context(), t, "L-1", "pending_review")
	}
}

// Scenario: Moderation: Unknown reason code is invalid (and every listed code is accepted).
func TestModerateListingUnknownReasonCode(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decBan, "because", ""))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
	for _, code := range []string{"prohibited_item", "counterfeit", "misleading", "inappropriate", "duplicate", "seller_enforcement", "other"} {
		g := newMHFixture()
		if _, err := g.h.ModerateListing(adminCtx(), modReq("L-1", decReject, code, "")); err != nil {
			t.Errorf("reason code %q must be accepted: %v", code, err)
		}
	}
}

// Scenario: Moderation: Note limit is 500 characters.
func TestModerateListingNoteBoundary500(t *testing.T) {
	cases := map[string]string{"ascii": "x", "multibyte": "é"}
	for name, ch := range cases {
		t.Run(name, func(t *testing.T) {
			f := newMHFixture()
			if _, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decReject, "other", strings.Repeat(ch, 500))); err != nil {
				t.Fatalf("500-character note must succeed: %v", err)
			}
			g := newMHFixture()
			_, err := g.h.ModerateListing(adminCtx(), modReq("L-1", decReject, "other", strings.Repeat(ch, 501)))
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("501-character note code = %v, want InvalidArgument", status.Code(err))
			}
			if len(g.audit.calls) != 0 {
				t.Fatalf("501-character note wrote %d audit events, want 0", len(g.audit.calls))
			}
		})
	}
}

// Scenario: Moderation: Unspecified decision is invalid.
func TestModerateListingUnspecifiedDecision(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ModerateListing(adminCtx(), modReq("L-1", listingv1.ModerationDecision_MODERATION_DECISION_UNSPECIFIED, "other", ""))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
}

// Scenario: Moderation: Invalid request on an unknown listing is invalid (empty id).
func TestModerateListingEmptyIDInvalid(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ModerateListing(adminCtx(), modReq("", decApprove, "", ""))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// Scenario: Moderation: Seller cannot moderate.
func TestModerateListingNonAdminDenied(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"owner seller": ownerCtx(),
		"other seller": otherCtx(),
		"no scopes":    typedCtx("seller-a", "user"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newMHFixture()
			_, err := f.h.ModerateListing(ctx, modReq("L-1", decApprove, "", ""))
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
			}
			if len(f.audit.calls) != 0 {
				t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
			}
			f.assertUnchanged(ctx, t, "L-1", "pending_review")
		})
	}
}

// Scenario: Moderation: Seller cannot moderate (denied before validation, no scope oracle).
func TestModerateListingNonAdminDeniedBeforeValidation(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ModerateListing(ownerCtx(), modReq("", listingv1.ModerationDecision_MODERATION_DECISION_UNSPECIFIED, "", ""))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

// Scenario: Moderation: Admin who owns the listing is audited (handler).
func TestModerateListingAdminOwnListingAudited(t *testing.T) {
	own := mhListing("L-1", "pending_review", modT1)
	own.SellerID = "admin-1"
	f := newMHFixture(own)
	if _, err := f.h.ModerateListing(adminCtx(), modReq("L-1", decApprove, "", "")); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if len(f.audit.calls) != 1 || f.audit.calls[0].actor != "admin-1" {
		t.Fatalf("audit calls = %+v, want exactly 1 by admin-1", f.audit.calls)
	}
}

// Scenario: Queue: Empty queue.
func TestListModerationQueueEmpty(t *testing.T) {
	f := newMHFixture(mhListing("L-1", "published", modT1))
	resp, err := f.h.ListModerationQueue(adminCtx(), &listingv1.ListModerationQueueRequest{})
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if len(resp.GetItems()) != 0 || resp.GetPage().GetNextCursor() != "" {
		t.Fatalf("response = %+v, want 0 items and empty next_cursor", resp)
	}
}

// Scenario: Queue: Oldest submission first (handler item shape: flags and submitted_at).
func TestListModerationQueueItemsCarryFlagsAndSubmittedAt(t *testing.T) {
	f := newMHFixture(
		mhListing("L-b", "pending_review", modT1.Add(2*time.Hour)),
		mhListing("L-a", "pending_review", modT1.Add(time.Hour)),
		mhListing("L-pub", "published", modT1),
	)
	resp, err := f.h.ListModerationQueue(adminCtx(), &listingv1.ListModerationQueueRequest{})
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if len(resp.GetItems()) != 2 || resp.GetItems()[0].GetListing().GetId() != "L-a" || resp.GetItems()[1].GetListing().GetId() != "L-b" {
		t.Fatalf("items = %+v, want L-a then L-b", resp.GetItems())
	}
	it := resp.GetItems()[0]
	if got := it.GetListing().GetModeration().GetScreenFlags(); len(got) != 1 || got[0] != "keyword:replica" {
		t.Errorf("screen_flags = %v, want [keyword:replica]", got)
	}
	if it.GetSubmittedAt() == nil || !it.GetSubmittedAt().AsTime().Equal(modT1.Add(time.Hour)) {
		t.Errorf("submitted_at = %v, want %v", it.GetSubmittedAt(), modT1.Add(time.Hour))
	}
}

// Scenario: Queue: Pagination boundary (handler passes page size and cursor through).
func TestListModerationQueuePaginationPassthrough(t *testing.T) {
	f := newMHFixture(
		mhListing("L-1", "pending_review", modT1),
		mhListing("L-2", "pending_review", modT1.Add(time.Minute)),
		mhListing("L-3", "pending_review", modT1.Add(2*time.Minute)),
	)
	p1, err := f.h.ListModerationQueue(adminCtx(), &listingv1.ListModerationQueueRequest{Page: &commonv1.PageRequest{PageSize: 2}})
	if err != nil || len(p1.GetItems()) != 2 || p1.GetPage().GetNextCursor() == "" {
		t.Fatalf("page 1 = %+v, %v; want 2 items and a cursor", p1, err)
	}
	p2, err := f.h.ListModerationQueue(adminCtx(), &listingv1.ListModerationQueueRequest{
		Page: &commonv1.PageRequest{PageSize: 2, Cursor: p1.GetPage().GetNextCursor()},
	})
	if err != nil || len(p2.GetItems()) != 1 || p2.GetItems()[0].GetListing().GetId() != "L-3" || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page 2 = %+v, %v; want L-3 and empty cursor", p2, err)
	}
}

// Scenario: Queue: Seller is denied.
func TestListModerationQueueNonAdminDenied(t *testing.T) {
	f := newMHFixture()
	_, err := f.h.ListModerationQueue(ownerCtx(), &listingv1.ListModerationQueueRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

// Scenario: Queue: Reading the queue writes no audit event.
func TestListModerationQueueNotAudited(t *testing.T) {
	f := newMHFixture()
	if _, err := f.h.ListModerationQueue(adminCtx(), &listingv1.ListModerationQueueRequest{}); err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
}
