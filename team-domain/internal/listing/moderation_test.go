package listing_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

// Interface contract the implementer must provide (task 8):
//
//	listing.ModerationDecision (string): DecisionApprove "approve", DecisionReject "reject", DecisionBan "ban"
//	listing.ModerateInput{ListingID, Decision, ReasonCode, Note}
//	listing.ErrTransitionNotAllowed (source status refused -> handler FailedPrecondition)
//	(*ListingService).ModerateListing(ctx, in, actorID, enqueue) (store.Listing, error)
//	(*ListingService).ListModerationQueue(ctx, cursor, pageSize) (store.Page, error)
//	(*ListingService).WithAudit(*auditx.Gate): the three privileged paths run through Gate.Do
//	the service reaches ModerateTx through its store.TxWriter (PgTxWriter already has it).

var modT0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

type modCall struct {
	actor, listing, seller, decision, reason, note, from, to string
	// storedStatus is the repository status at the moment of the audit call,
	// proving the audit write happens before the change.
	storedStatus string
}

type modAudit struct {
	repo  *storetest.InMemoryListingRepository
	log   *[]string
	calls []modCall
	err   error
}

func (a *modAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	if ev.Action != "listing.moderate" {
		return a.err
	}
	cur, _ := a.repo.Get(context.Background(), ev.TargetID)
	m := ev.Metadata
	a.calls = append(a.calls, modCall{ev.ActorID, ev.TargetID, m["seller_id"], m["decision"], m["reason_code"], m["note"], m["from_status"], m["to_status"], cur.Status})
	*a.log = append(*a.log, "audit")
	return a.err
}

// modTx is the in-memory ModerateTx the real PgTxWriter provides: conditional on
// the loaded status and updated_at, enqueueing through the shared outbox.
type modTx struct {
	*storetest.InMemoryTxWriter
	repo           *storetest.InMemoryListingRepository
	log            *[]string
	beforeModerate func()
}

func (m *modTx) ModerateTx(ctx context.Context, id, fromStatus string, loadedUpdatedAt time.Time, d store.ModerationUpdate, ev store.EnqueueFn) (store.Listing, error) {
	*m.log = append(*m.log, "change")
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
	return m.InMemoryTxWriter.UpdateTx(ctx, cur, ev)
}

type modFixture struct {
	repo  *storetest.InMemoryListingRepository
	txw   *storetest.InMemoryTxWriter
	tx    *modTx
	svc   *listing.ListingService
	audit *modAudit
	log   *[]string
	seen  *capture
}

func newModFixture(stored string) *modFixture {
	log := &[]string{}
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: stored, SellerID: "seller-a", Stock: 1,
		UpdatedAt: modT0, ScreenFlags: []string{"keyword:replica"},
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	tx := &modTx{InMemoryTxWriter: txw, repo: repo, log: log}
	fa := &modAudit{repo: repo, log: log}
	svc := listing.NewListingServiceWithOutbox(repo, tx).WithAudit(gateOf(fa))
	return &modFixture{repo: repo, txw: txw, tx: tx, svc: svc, audit: fa, log: log, seen: &capture{}}
}

func (f *modFixture) moderate(in listing.ModerateInput) (store.Listing, error) {
	if in.ListingID == "" {
		in.ListingID = "L-1"
	}
	return f.svc.ModerateListing(context.Background(), in, "admin-1", f.seen.enqueue)
}

func (f *modFixture) stored(t *testing.T) store.Listing {
	t.Helper()
	l, err := f.repo.Get(context.Background(), "L-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return l
}

func (f *modFixture) assertUntouched(t *testing.T, wantStatus string) {
	t.Helper()
	if got := f.stored(t); got.Status != wantStatus {
		t.Fatalf("stored status = %q, want %q", got.Status, wantStatus)
	}
	if n := len(f.txw.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: Moderation: Approve publishes a pending listing.
func TestModerateApprovePublishes(t *testing.T) {
	f := newModFixture("pending_review")
	got, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove})
	if err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if got.Status != "published" || got.ModerationDecidedAt.IsZero() {
		t.Fatalf("returned = status %q decided_at %v, want published with decided_at set", got.Status, got.ModerationDecidedAt)
	}
	if s := f.stored(t); s.Status != "published" || s.ModerationDecidedAt.IsZero() {
		t.Fatalf("stored = %+v", s)
	}
	if n := len(f.txw.Rows()); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
	if !reflect.DeepEqual(f.seen.statuses, []string{"published"}) {
		t.Fatalf("event statuses = %v, want [published]", f.seen.statuses)
	}
}

// Scenario: Moderation: Reject records reason and note for the seller.
func TestModerateRejectRecordsReason(t *testing.T) {
	f := newModFixture("pending_review")
	got, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionReject, ReasonCode: "counterfeit", Note: "Fake logo"})
	if err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	s := f.stored(t)
	if s.Status != "rejected" || s.ModerationReasonCode != "counterfeit" || s.ModerationNote != "Fake logo" || s.ModerationDecidedAt.IsZero() {
		t.Fatalf("stored = %+v", s)
	}
	if got.Status != "rejected" || got.ModerationReasonCode != "counterfeit" || got.ModerationNote != "Fake logo" {
		t.Fatalf("returned = %+v", got)
	}
	if n := len(f.txw.Rows()); n != 1 || !reflect.DeepEqual(f.seen.statuses, []string{"rejected"}) {
		t.Fatalf("rows=%d statuses=%v, want 1 row with rejected", n, f.seen.statuses)
	}
}

// Scenario: Moderation: Ban takes down a published listing.
func TestModerateBanPublished(t *testing.T) {
	f := newModFixture("published")
	if _, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionBan, ReasonCode: "prohibited_item"}); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if s := f.stored(t); s.Status != "banned" || s.ModerationReasonCode != "prohibited_item" {
		t.Fatalf("stored = %+v", s)
	}
	if n := len(f.txw.Rows()); n != 1 || !reflect.DeepEqual(f.seen.statuses, []string{"banned"}) {
		t.Fatalf("rows=%d statuses=%v, want 1 row with banned", n, f.seen.statuses)
	}
}

// Scenario: Moderation: Reject of a published listing from a report.
func TestModerateRejectPublished(t *testing.T) {
	f := newModFixture("published")
	if _, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionReject, ReasonCode: "misleading"}); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if s := f.stored(t); s.Status != "rejected" || s.ModerationReasonCode != "misleading" {
		t.Fatalf("stored = %+v", s)
	}
}

// Scenario: Moderation: Audit event is written first.
func TestModerateAuditPrecedesChange(t *testing.T) {
	f := newModFixture("pending_review")
	if _, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionReject, ReasonCode: "counterfeit", Note: "Fake logo"}); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if !reflect.DeepEqual(*f.log, []string{"audit", "change"}) {
		t.Fatalf("order = %v, want [audit change]", *f.log)
	}
	want := modCall{actor: "admin-1", listing: "L-1", seller: "seller-a", decision: "reject", reason: "counterfeit",
		note: "Fake logo", from: "pending_review", to: "rejected", storedStatus: "pending_review"}
	if len(f.audit.calls) != 1 || f.audit.calls[0] != want {
		t.Fatalf("audit calls = %+v, want [%+v]", f.audit.calls, want)
	}
}

// Scenario: Moderation: Audit down refuses the decision (service half).
func TestModerateAuditErrorUnavailableNoChange(t *testing.T) {
	for name, aerr := range map[string]error{"error": errors.New("boom"), "deadline": context.DeadlineExceeded} {
		t.Run(name, func(t *testing.T) {
			f := newModFixture("pending_review")
			f.audit.err = aerr
			_, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove})
			requireAuditUnavailable(t, err)
			f.assertUntouched(t, "pending_review")
		})
	}
}

// Scenario: Moderation: Approve of a held listing is refused.
func TestModerateHeldFailedPreconditionNoAudit(t *testing.T) {
	f := newModFixture("held")
	_, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove})
	if !errors.Is(err, listing.ErrTransitionNotAllowed) {
		t.Fatalf("err = %v, want ErrTransitionNotAllowed", err)
	}
	f.assertUntouched(t, "held")
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
}

// Scenario: Moderation: Approve of a rejected listing is refused.
func TestModerateApproveRejectedFailedPrecondition(t *testing.T) {
	f := newModFixture("rejected")
	_, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove})
	if !errors.Is(err, listing.ErrTransitionNotAllowed) {
		t.Fatalf("err = %v, want ErrTransitionNotAllowed", err)
	}
	f.assertUntouched(t, "rejected")
}

// Scenario: Moderation: Approve of a banned listing is refused.
func TestModerateApproveBannedFailedPrecondition(t *testing.T) {
	f := newModFixture("banned")
	_, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove})
	if !errors.Is(err, listing.ErrTransitionNotAllowed) {
		t.Fatalf("err = %v, want ErrTransitionNotAllowed", err)
	}
	f.assertUntouched(t, "banned")
}

// Scenario: Moderation: Approve of a held listing is refused (every other source status and decision).
func TestModerateTransitionMatrix(t *testing.T) {
	type tc struct {
		from     string
		decision listing.ModerationDecision
		allowed  bool
	}
	var cases []tc
	for _, from := range []string{"draft", "rejected", "banned", "held"} {
		for _, d := range []listing.ModerationDecision{listing.DecisionApprove, listing.DecisionReject, listing.DecisionBan} {
			cases = append(cases, tc{from, d, false})
		}
	}
	cases = append(cases,
		tc{"published", listing.DecisionApprove, false},
		tc{"published", listing.DecisionReject, true},
		tc{"published", listing.DecisionBan, true},
		tc{"pending_review", listing.DecisionApprove, true},
		tc{"pending_review", listing.DecisionReject, true},
		tc{"pending_review", listing.DecisionBan, true},
	)
	for _, c := range cases {
		t.Run(c.from+"/"+string(c.decision), func(t *testing.T) {
			f := newModFixture(c.from)
			_, err := f.moderate(listing.ModerateInput{Decision: c.decision, ReasonCode: "other"})
			if c.allowed {
				if err != nil {
					t.Fatalf("err = %v, want success", err)
				}
				return
			}
			if !errors.Is(err, listing.ErrTransitionNotAllowed) {
				t.Fatalf("err = %v, want ErrTransitionNotAllowed", err)
			}
			f.assertUntouched(t, c.from)
			if len(f.audit.calls) != 0 {
				t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
			}
		})
	}
}

// Scenario: Moderation: Seller edit between check and decision is refused.
func TestModerateStaleLoadFailedPrecondition(t *testing.T) {
	f := newModFixture("pending_review")
	f.tx.beforeModerate = func() {
		cur, _ := f.repo.Get(context.Background(), "L-1")
		cur.Title = "seller edited"
		cur.UpdatedAt = cur.UpdatedAt.Add(time.Minute)
		_, _ = f.repo.Update(context.Background(), cur)
	}
	_, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove})
	if !errors.Is(err, store.ErrStatusChanged) {
		t.Fatalf("err = %v, want store.ErrStatusChanged", err)
	}
	if s := f.stored(t); s.Status != "pending_review" || s.Title != "seller edited" {
		t.Fatalf("stored = %+v, want the seller's edit and pending_review", s)
	}
	if n := len(f.txw.Rows()); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

// Scenario: Moderation: Second decision on a decided listing is refused.
func TestModerateTwiceSecondRefused(t *testing.T) {
	f := newModFixture("pending_review")
	in := listing.ModerateInput{Decision: listing.DecisionReject, ReasonCode: "duplicate"}
	if _, err := f.moderate(in); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := f.moderate(in); !errors.Is(err, listing.ErrTransitionNotAllowed) {
		t.Fatalf("second err = %v, want ErrTransitionNotAllowed", err)
	}
	if len(f.audit.calls) != 1 {
		t.Fatalf("audit calls = %d, want exactly 1", len(f.audit.calls))
	}
	if n := len(f.txw.Rows()); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
}

// Scenario: Moderation: Unknown listing writes no audit event.
func TestModerateNotFoundNoAudit(t *testing.T) {
	f := newModFixture("pending_review")
	_, err := f.moderate(listing.ModerateInput{ListingID: "nope", Decision: listing.DecisionApprove})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want store.ErrNotFound", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
	}
	f.assertUntouched(t, "pending_review")
}

// Scenario: Moderation: Admin who owns the listing is audited.
func TestModerateAdminOwnListingAudited(t *testing.T) {
	f := newModFixture("pending_review")
	own := f.stored(t)
	own.SellerID = "admin-1"
	if _, err := f.repo.Update(context.Background(), own); err != nil {
		t.Fatal(err)
	}
	if _, err := f.moderate(listing.ModerateInput{Decision: listing.DecisionApprove}); err != nil {
		t.Fatalf("ModerateListing: %v", err)
	}
	if f.stored(t).Status != "published" {
		t.Fatal("decision must be applied")
	}
	if len(f.audit.calls) != 1 || f.audit.calls[0].actor != "admin-1" || f.audit.calls[0].seller != "admin-1" {
		t.Fatalf("audit calls = %+v, want exactly 1 with actor admin-1", f.audit.calls)
	}
}
