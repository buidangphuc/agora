package listing_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/screen"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

func TestListingService(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository()
	svc := listing.NewListingService(repo)
	ctx := context.Background()

	t.Run("Create and Get Listing", func(t *testing.T) {
		l := store.Listing{
			Title:       "iPhone 15 Pro",
			Description: "Apple smartphone",
			Price:       25000000,
			Currency:    "VND",
			Status:      "published",
			Stock:       10,
		}

		created, err := svc.Create(ctx, l, "seller_1")
		if err != nil {
			t.Fatalf("unexpected error creating listing: %v", err)
		}
		if created.ID == "" {
			t.Fatalf("expected non-empty ID")
		}
		if created.SellerID != "seller_1" {
			t.Errorf("expected seller_1, got %s", created.SellerID)
		}

		got, err := svc.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error getting listing: %v", err)
		}
		if got.Title != "iPhone 15 Pro" {
			t.Errorf("expected iPhone 15 Pro, got %s", got.Title)
		}

		// Update as owner
		got.Title = "iPhone 15 Pro Max"
		updated, err := svc.Update(ctx, got, "seller_1", false)
		if err != nil {
			t.Fatalf("unexpected error updating listing: %v", err)
		}
		if updated.Title != "iPhone 15 Pro Max" {
			t.Errorf("expected iPhone 15 Pro Max, got %s", updated.Title)
		}

		// Update as non-owner without admin fails
		_, err = svc.Update(ctx, got, "another_seller", false)
		if err != listing.ErrForbidden {
			t.Errorf("expected ErrForbidden when non-owner updates, got %v", err)
		}

		// Update as admin succeeds
		got.Title = "iPhone 15 Pro Max (Admin Edited)"
		adminUpdated, err := svc.Update(ctx, got, "another_seller", true)
		if err != nil {
			t.Fatalf("unexpected error admin updating: %v", err)
		}
		if adminUpdated.Title != "iPhone 15 Pro Max (Admin Edited)" {
			t.Errorf("expected admin edited title")
		}

		// Delete non-owner fails
		_, err = svc.Delete(ctx, created.ID, "another_seller", false)
		if err != listing.ErrForbidden {
			t.Errorf("expected ErrForbidden when non-owner deletes")
		}

		// Delete as owner succeeds
		deleted, err := svc.Delete(ctx, created.ID, "seller_1", false)
		if err != nil {
			t.Fatalf("unexpected error deleting: %v", err)
		}
		if deleted.ID != created.ID {
			t.Errorf("expected deleted ID %s, got %s", created.ID, deleted.ID)
		}
	})

	t.Run("CreateWithEvent enqueues one row on success", func(t *testing.T) {
		repo := storetest.NewInMemoryListingRepository()
		txw := storetest.NewInMemoryTxWriter(repo)
		svc := listing.NewListingServiceWithOutbox(repo, txw)

		created, err := svc.CreateWithEvent(ctx, store.Listing{Title: "Tx OK", Currency: "VND"}, "seller_1",
			func(l store.Listing) (store.OutboxRow, error) {
				return store.OutboxRow{EventID: "evt-ok", AggregateID: l.ID, EventType: "test"}, nil
			})
		if err != nil {
			t.Fatalf("CreateWithEvent: %v", err)
		}
		if _, err := repo.Get(ctx, created.ID); err != nil {
			t.Fatalf("listing should be persisted: %v", err)
		}
		rows := txw.Rows()
		if len(rows) != 1 || rows[0].AggregateID != created.ID {
			t.Fatalf("want 1 outbox row for %s, got %+v", created.ID, rows)
		}
	})

	t.Run("CreateWithEvent rolls back both rows on write failure", func(t *testing.T) {
		repo := storetest.NewInMemoryListingRepository()
		txw := storetest.NewInMemoryTxWriter(repo)
		txw.FailWrite = true // simulate the business write failing inside the tx
		svc := listing.NewListingServiceWithOutbox(repo, txw)

		created, err := svc.CreateWithEvent(ctx, store.Listing{ID: "rollback-1", Title: "boom", Currency: "VND"}, "seller_1",
			func(l store.Listing) (store.OutboxRow, error) {
				return store.OutboxRow{EventID: "evt-x", AggregateID: l.ID}, nil
			})
		if err == nil {
			t.Fatal("expected error when the write fails")
		}
		// Neither the listing nor an outbox row may survive a rolled-back write.
		if _, err := repo.Get(ctx, "rollback-1"); err != store.ErrNotFound {
			t.Fatalf("listing must not be persisted on rollback, got %v", err)
		}
		if got := len(txw.Rows()); got != 0 {
			t.Fatalf("no outbox row may survive rollback, got %d", got)
		}
		_ = created
	})

	t.Run("List and ListMine", func(t *testing.T) {
		_, _ = svc.Create(ctx, store.Listing{Title: "Item 1", Price: 100, Status: "published"}, "seller_a")
		_, _ = svc.Create(ctx, store.Listing{Title: "Item 2", Price: 200, Status: "published"}, "seller_a")
		_, _ = svc.Create(ctx, store.Listing{Title: "Item 3", Price: 300, Status: "published"}, "seller_b")

		page, err := svc.List(ctx, "", 10, "published")
		if err != nil {
			t.Fatalf("unexpected error listing: %v", err)
		}
		if len(page.Items) < 3 {
			t.Errorf("expected at least 3 items, got %d", len(page.Items))
		}

		minePage, err := svc.ListMine(ctx, "seller_a", "", 10)
		if err != nil {
			t.Fatalf("unexpected error listing mine: %v", err)
		}
		if len(minePage.Items) != 2 {
			t.Errorf("expected 2 items for seller_a, got %d", len(minePage.Items))
		}
	})
}

// ---- Listing audit (domain-admin-listing-audit) ----

type auditCall struct {
	kind, actor, listing, seller, status string
}

type fakeAudit struct {
	log   *[]string
	calls []auditCall
	err   error
}

func (f *fakeAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	switch ev.Action {
	case "listing.admin_update":
		f.calls = append(f.calls, auditCall{"update", ev.ActorID, ev.TargetID, ev.Metadata["seller_id"], ev.Metadata["status"]})
	case "listing.admin_delete":
		f.calls = append(f.calls, auditCall{"delete", ev.ActorID, ev.TargetID, ev.Metadata["seller_id"], ""})
	default:
		return f.err
	}
	*f.log = append(*f.log, "audit")
	return f.err
}

func gateOf(w auditx.Writer) *auditx.Gate { return auditx.NewGate(w, nil) }

func requireAuditUnavailable(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
		t.Fatalf("err = %v, want Unavailable %q", err, auditx.Message)
	}
}

func isAuditUnavailable(err error) bool {
	return status.Code(err) == codes.Unavailable && status.Convert(err).Message() == auditx.Message
}

// orderTx wraps the in-memory TxWriter and logs when the change happens.
type orderTx struct {
	*storetest.InMemoryTxWriter
	log *[]string
}

func (o orderTx) UpdateTx(ctx context.Context, l store.Listing, ev store.EnqueueFn) (store.Listing, error) {
	*o.log = append(*o.log, "change")
	return o.InMemoryTxWriter.UpdateTx(ctx, l, ev)
}

func (o orderTx) DeleteTx(ctx context.Context, id string, ev store.EnqueueFn) (store.Listing, error) {
	*o.log = append(*o.log, "change")
	return o.InMemoryTxWriter.DeleteTx(ctx, id, ev)
}

type auditFixture struct {
	repo  *storetest.InMemoryListingRepository
	txw   *storetest.InMemoryTxWriter
	svc   *listing.ListingService
	audit *fakeAudit
	log   *[]string
}

func newAuditFixture(t *testing.T) *auditFixture {
	t.Helper()
	log := &[]string{}
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Status: "published", SellerID: "seller-1", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	fa := &fakeAudit{log: log}
	svc := listing.NewListingServiceWithOutbox(repo, orderTx{txw, log}).WithAudit(gateOf(fa))
	return &auditFixture{repo: repo, txw: txw, svc: svc, audit: fa, log: log}
}

func enq(l store.Listing) (store.OutboxRow, error) {
	return store.OutboxRow{EventID: "e-" + l.ID, AggregateID: l.ID, EventType: "test"}, nil
}

// Scenario: Listing audit: Audit is written before the change.
func TestUpdateWithEventAuditPrecedesUpdate(t *testing.T) {
	f := newAuditFixture(t)
	// The request tries to claim another owner; the event must carry the stored one.
	_, err := f.svc.UpdateWithEvent(context.Background(),
		store.Listing{ID: "L-1", Title: "new", Status: "rejected", SellerID: "forged"}, "admin-1", true, enq)
	if err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if got := *f.log; len(got) != 2 || got[0] != "audit" || got[1] != "change" {
		t.Fatalf("order = %v, want [audit change]", got)
	}
	want := auditCall{"update", "admin-1", "L-1", "seller-1", "published"}
	if len(f.audit.calls) != 1 || f.audit.calls[0] != want {
		t.Fatalf("audit calls = %+v, want [%+v]", f.audit.calls, want)
	}
}

// Scenario: Listing audit: Admin who owns the listing is audited.
func TestUpdateWithEventAdminOwnerIsAudited(t *testing.T) {
	f := newAuditFixture(t)
	if _, err := f.svc.UpdateWithEvent(context.Background(),
		store.Listing{ID: "L-1", Title: "mine", Status: "published"}, "seller-1", true, enq); err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if len(f.audit.calls) != 1 || f.audit.calls[0].kind != "update" || f.audit.calls[0].seller != "seller-1" || f.audit.calls[0].actor != "seller-1" {
		t.Fatalf("audit calls = %+v, want 1 update with seller_id seller-1", f.audit.calls)
	}
}

// Scenario: Listing audit: Admin who owns the listing is refused while audit is down.
func TestDeleteWithEventAdminOwnerAuditDownUnavailable(t *testing.T) {
	f := newAuditFixture(t)
	f.audit.err = errors.New("audit down")
	_, err := f.svc.DeleteWithEvent(context.Background(), "L-1", "seller-1", true, enq)
	requireAuditUnavailable(t, err)
	if _, err := f.repo.Get(context.Background(), "L-1"); err != nil {
		t.Fatalf("listing must still exist: %v", err)
	}
	if len(f.txw.Rows()) != 0 {
		t.Fatal("no outbox row may be written")
	}
}

// Scenario: Listing audit: Audit down refuses the admin update (service half, error and deadline).
func TestUpdateWithEventAuditErrorLeavesListingUnchanged(t *testing.T) {
	for name, aerr := range map[string]error{"error": errors.New("audit down"), "deadline": context.DeadlineExceeded} {
		t.Run(name, func(t *testing.T) {
			f := newAuditFixture(t)
			f.audit.err = aerr
			_, err := f.svc.UpdateWithEvent(context.Background(),
				store.Listing{ID: "L-1", Title: "new", Status: "rejected"}, "admin-1", true, enq)
			requireAuditUnavailable(t, err)
			got, _ := f.repo.Get(context.Background(), "L-1")
			if got.Title != "orig" || got.Status != "published" {
				t.Fatalf("listing changed: %+v", got)
			}
			if len(f.txw.Rows()) != 0 {
				t.Fatal("no outbox row may be written")
			}
		})
	}
}

// Scenario: Listing audit: Unknown listing writes no audit event.
func TestUpdateWithEventNotFoundNoAudit(t *testing.T) {
	f := newAuditFixture(t)
	_, err := f.svc.UpdateWithEvent(context.Background(), store.Listing{ID: "nope"}, "admin-1", true, enq)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Listing audit: Unknown listing on delete writes no audit event.
func TestDeleteWithEventNotFoundNoAudit(t *testing.T) {
	f := newAuditFixture(t)
	_, err := f.svc.DeleteWithEvent(context.Background(), "nope", "admin-1", true, enq)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Listing audit: Update failure after audit keeps the audit row.
func TestUpdateWithEventRepoFailureAfterAudit(t *testing.T) {
	f := newAuditFixture(t)
	f.txw.FailWrite = true
	_, err := f.svc.UpdateWithEvent(context.Background(), store.Listing{ID: "L-1", Title: "new"}, "admin-1", true, enq)
	if err == nil || isAuditUnavailable(err) {
		t.Fatalf("err = %v, want the repository failure (not the audit trail refusal)", err)
	}
	if len(f.audit.calls) != 1 {
		t.Fatalf("audit calls = %d, want 1 (already written, not retracted)", len(f.audit.calls))
	}
}

// Scenario: Listing audit: Delete failure after audit keeps the audit row.
func TestDeleteWithEventRepoFailureAfterAudit(t *testing.T) {
	f := newAuditFixture(t)
	f.txw.FailWrite = true
	_, err := f.svc.DeleteWithEvent(context.Background(), "L-1", "admin-1", true, enq)
	if err == nil || isAuditUnavailable(err) {
		t.Fatalf("err = %v, want the repository failure (not the audit trail refusal)", err)
	}
	if len(f.audit.calls) != 1 || f.audit.calls[0].kind != "delete" || f.audit.calls[0].seller != "seller-1" {
		t.Fatalf("audit calls = %+v, want 1 delete with seller_id seller-1", f.audit.calls)
	}
}

// Scenario: Listing audit: Admin delete writes one audit event (service half: order + payload).
func TestDeleteWithEventAdminAuditPrecedesDelete(t *testing.T) {
	f := newAuditFixture(t)
	if _, err := f.svc.DeleteWithEvent(context.Background(), "L-1", "admin-1", true, enq); err != nil {
		t.Fatalf("DeleteWithEvent: %v", err)
	}
	if got := *f.log; len(got) != 2 || got[0] != "audit" || got[1] != "change" {
		t.Fatalf("order = %v, want [audit change]", got)
	}
	want := auditCall{"delete", "admin-1", "L-1", "seller-1", ""}
	if len(f.audit.calls) != 1 || f.audit.calls[0] != want {
		t.Fatalf("audit calls = %+v, want [%+v]", f.audit.calls, want)
	}
}

// Scenario: Listing audit: Owner update writes no audit event (service half; also non-owner non-admin).
func TestWithEventNonAdminPathsMakeNoAuditCall(t *testing.T) {
	f := newAuditFixture(t)
	ctx := context.Background()
	if _, err := f.svc.UpdateWithEvent(ctx, store.Listing{ID: "L-1", Title: "own"}, "seller-1", false, enq); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	if _, err := f.svc.UpdateWithEvent(ctx, store.Listing{ID: "L-1", Title: "x"}, "seller-2", false, enq); !errors.Is(err, listing.ErrForbidden) {
		t.Fatalf("non-owner update err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.DeleteWithEvent(ctx, "L-1", "seller-2", false, enq); !errors.Is(err, listing.ErrForbidden) {
		t.Fatalf("non-owner delete err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.DeleteWithEvent(ctx, "L-1", "seller-1", false, enq); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

func ownershipSvc(stored string) (*listing.ListingService, *storetest.InMemoryListingRepository) {
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Status: stored, SellerID: "seller-a", Stock: 1,
	})
	return listing.NewListingServiceWithOutbox(repo, storetest.NewInMemoryTxWriter(repo)), repo
}

func noEvent(l store.Listing) (store.OutboxRow, error) {
	return store.OutboxRow{EventID: "e-" + l.ID, AggregateID: l.ID, EventType: "test"}, nil
}

// Scenario: Status ownership: Seller cannot un-reject by sending PUBLISHED.
func TestUpdateWithEventRejectedStaysUnpublished(t *testing.T) {
	svc, repo := ownershipSvc("rejected")
	_, _ = svc.UpdateWithEvent(context.Background(),
		store.Listing{ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Stock: 1, Status: "published"},
		"seller-a", false, noEvent)
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status == "published" {
		t.Fatalf("stored status = %q, a seller must not un-reject by sending published", got.Status)
	}
}

// Scenario: Status ownership: Seller cannot move a published listing into a server-only status.
func TestUpdateWithEventClientBannedIgnored(t *testing.T) {
	for _, st := range []string{"banned", "held", "pending_review", "rejected"} {
		t.Run(st, func(t *testing.T) {
			svc, repo := ownershipSvc("published")
			_, err := svc.UpdateWithEvent(context.Background(),
				store.Listing{ID: "L-1", Title: "orig", Currency: "VND", Price: 100, Stock: 1, Status: st},
				"seller-a", false, noEvent)
			if err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			got, _ := repo.Get(context.Background(), "L-1")
			if got.Status != "published" {
				t.Fatalf("stored status = %q, want published", got.Status)
			}
		})
	}
}

// Scenario: Status ownership: Status omitted keeps a published listing published.
func TestUpdateWithEventOmittedStatusKeepsStored(t *testing.T) {
	for _, stored := range []string{"published", "draft"} {
		t.Run(stored, func(t *testing.T) {
			svc, repo := ownershipSvc(stored)
			_, err := svc.UpdateWithEvent(context.Background(),
				store.Listing{ID: "L-1", Title: "orig", Currency: "VND", Price: 999, Stock: 1, Status: ""},
				"seller-a", false, noEvent)
			if err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			got, _ := repo.Get(context.Background(), "L-1")
			if got.Status != stored {
				t.Fatalf("stored status = %q, want %q", got.Status, stored)
			}
		})
	}
}

// Scenario: Status ownership: Server-only statuses sent on create are ignored.
func TestCreateWithEventServerOnlyStatusIgnored(t *testing.T) {
	for _, st := range []string{"rejected", "banned", "pending_review", "held", ""} {
		t.Run("status="+st, func(t *testing.T) {
			svc, _ := ownershipSvc("draft")
			got, err := svc.CreateWithEvent(context.Background(),
				store.Listing{Title: "n", Currency: "VND", Stock: 1, Status: st}, "seller-a", noEvent)
			if err != nil {
				t.Fatalf("CreateWithEvent: %v", err)
			}
			if got.Status != "draft" {
				t.Fatalf("status = %q, want draft", got.Status)
			}
		})
	}
}

// Scenario: Status ownership: Another seller still cannot edit the listing.
func TestUpdateWithEventNonOwnerForbidden(t *testing.T) {
	svc, repo := ownershipSvc("pending_review")
	_, err := svc.UpdateWithEvent(context.Background(),
		store.Listing{ID: "L-1", Title: "x", Currency: "VND", Stock: 1, Status: "published"},
		"seller-b", false, noEvent)
	if err != listing.ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	got, _ := repo.Get(context.Background(), "L-1")
	if got.Status != "pending_review" {
		t.Fatalf("status = %q", got.Status)
	}
}

// ---- Auto-screen and state machine (domain-listing-moderation task 6) ----

type fakeAI struct {
	calls   int
	verdict aiv1.ScreenVerdict
	labels  []string
	err     error
}

func (f *fakeAI) ScreenListing(_ context.Context, _ screen.ScreenInput) (*aiv1.ScreenListingResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &aiv1.ScreenListingResponse{Verdict: f.verdict, Labels: f.labels, Backend: "fake"}, nil
}

func allowAI() *fakeAI { return &fakeAI{verdict: aiv1.ScreenVerdict_SCREEN_VERDICT_ALLOW} }

func termsFile(t *testing.T) *screen.Screener {
	t.Helper()
	p := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(p, []byte("replica\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sc, err := screen.New(p)
	if err != nil {
		t.Fatalf("screen.New: %v", err)
	}
	return sc
}

type screenFixture struct {
	svc  *listing.ListingService
	repo *storetest.InMemoryListingRepository
	txw  *storetest.InMemoryTxWriter
	ai   *fakeAI
}

func newScreenFixture(t *testing.T, stored string, ai *fakeAI) *screenFixture {
	t.Helper()
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Description: "desc", CategoryID: "c1", Currency: "VND", Price: 100,
		Status: stored, SellerID: "seller-a", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	svc := listing.NewListingServiceWithOutbox(repo, txw)
	var ais listing.Screener
	if ai != nil {
		ais = ai
	}
	svc = svc.WithScreener(termsFile(t), ais)
	return &screenFixture{svc: svc, repo: repo, txw: txw, ai: ai}
}

func (f *screenFixture) stored(t *testing.T) store.Listing {
	t.Helper()
	got, err := f.repo.Get(context.Background(), "L-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return got
}

// capture records the status the enqueue callback sees (the persisted listing).
type capture struct{ statuses []string }

func (c *capture) enqueue(l store.Listing) (store.OutboxRow, error) {
	c.statuses = append(c.statuses, l.Status)
	return store.OutboxRow{EventID: "e-" + l.ID, AggregateID: l.ID, EventType: "test"}, nil
}

func newListing(title string, status string) store.Listing {
	return store.Listing{Title: title, Description: "desc", CategoryID: "c1", Currency: "VND", Price: 1, Stock: 1, Status: status}
}

// Scenario: Auto-screen: Clean listing publishes.
func TestCreateWithEventCleanPublishes(t *testing.T) {
	f := newScreenFixture(t, "draft", allowAI())
	var c capture
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("clean phone", "published"), "seller-a", c.enqueue)
	if err != nil {
		t.Fatalf("CreateWithEvent: %v", err)
	}
	if got.Status != "published" {
		t.Fatalf("status = %q, want published", got.Status)
	}
	if len(c.statuses) != 1 || c.statuses[0] != "published" {
		t.Fatalf("event statuses = %v, want [published]", c.statuses)
	}
	if f.ai.calls != 1 {
		t.Fatalf("ai calls = %d, want 1", f.ai.calls)
	}
}

// Scenario: Auto-screen: Keyword hit goes to review without calling the AI.
func TestCreateWithEventKeywordHitPendingReview(t *testing.T) {
	f := newScreenFixture(t, "draft", allowAI())
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("Replica watch", "published"), "seller-a", noEvent)
	if err != nil {
		t.Fatalf("CreateWithEvent: %v", err)
	}
	if got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
	if !reflect.DeepEqual(got.ScreenFlags, []string{"keyword:replica"}) {
		t.Fatalf("flags = %v, want [keyword:replica]", got.ScreenFlags)
	}
	if got.SubmittedAt.IsZero() {
		t.Error("submitted_at must be set when entering pending_review")
	}
	if f.ai.calls != 0 {
		t.Fatalf("ai calls = %d, want 0", f.ai.calls)
	}
}

// Scenario: Auto-screen: AI REVIEW verdict goes to review.
func TestCreateWithEventAIReviewPendingReview(t *testing.T) {
	f := newScreenFixture(t, "draft", &fakeAI{verdict: aiv1.ScreenVerdict_SCREEN_VERDICT_REVIEW, labels: []string{"weapon"}})
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("clean text", "published"), "seller-a", noEvent)
	if err != nil {
		t.Fatalf("CreateWithEvent: %v", err)
	}
	if got.Status != "pending_review" || !reflect.DeepEqual(got.ScreenFlags, []string{"ai:weapon"}) {
		t.Fatalf("status=%q flags=%v, want pending_review [ai:weapon]", got.Status, got.ScreenFlags)
	}
}

// Scenario: Auto-screen: AI error fails safe.
func TestCreateWithEventScreenErrorPendingReview(t *testing.T) {
	f := newScreenFixture(t, "draft", &fakeAI{err: errors.New("team-ai down")})
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("clean text", "published"), "seller-a", noEvent)
	if err != nil {
		t.Fatalf("write must succeed on screen error: %v", err)
	}
	if got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
}

// Scenario: Auto-screen: AI timeout fails safe within the bound.
func TestCreateWithEventScreenTimeoutPendingReview(t *testing.T) {
	f := newScreenFixture(t, "draft", &fakeAI{err: context.DeadlineExceeded})
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("clean text", "published"), "seller-a", noEvent)
	if err != nil {
		t.Fatalf("write must succeed on screen timeout: %v", err)
	}
	if got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
}

// Scenario: Auto-screen: Unconfigured team-ai fails safe (service side).
func TestCreateWithEventNilScreenerPendingReview(t *testing.T) {
	f := newScreenFixture(t, "draft", nil)
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("clean text", "published"), "seller-a", noEvent)
	if err != nil {
		t.Fatalf("CreateWithEvent: %v", err)
	}
	if got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
}

// Scenario: Auto-screen: Draft save is not screened.
func TestCreateWithEventDraftNotScreened(t *testing.T) {
	f := newScreenFixture(t, "draft", allowAI())
	got, err := f.svc.CreateWithEvent(context.Background(), newListing("replica", "draft"), "seller-a", noEvent)
	if err != nil {
		t.Fatalf("CreateWithEvent: %v", err)
	}
	if got.Status != "draft" || f.ai.calls != 0 || len(got.ScreenFlags) != 0 {
		t.Fatalf("status=%q ai=%d flags=%v, want draft, 0 calls, no flags", got.Status, f.ai.calls, got.ScreenFlags)
	}
}

func fullUpdate(title, desc string, price int64, status string) store.Listing {
	return store.Listing{ID: "L-1", Title: title, Description: desc, CategoryID: "c1", Currency: "VND", Price: price, Stock: 7, Status: status}
}

// Scenario: Auto-screen: Price and stock edits do not re-screen.
func TestUpdateWithEventNonContentEditNoScreen(t *testing.T) {
	f := newScreenFixture(t, "published", allowAI())
	_, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("orig", "desc", 555, "published"), "seller-a", false, noEvent)
	if err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	got := f.stored(t)
	if got.Status != "published" || got.Price != 555 || f.ai.calls != 0 {
		t.Fatalf("status=%q price=%d ai=%d, want published 555 0", got.Status, got.Price, f.ai.calls)
	}
}

// Scenario: Auto-screen: Title edit of a published listing is re-screened.
func TestUpdateWithEventContentEditRescreens(t *testing.T) {
	f := newScreenFixture(t, "published", allowAI())
	var c capture
	_, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("replica bag", "desc", 100, ""), "seller-a", false, c.enqueue)
	if err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if got := f.stored(t); got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
	if len(c.statuses) != 1 || c.statuses[0] != "pending_review" {
		t.Fatalf("event statuses = %v, want [pending_review]", c.statuses)
	}
}

// Scenario: Auto-screen: Content edit sending a server-only status is re-screened.
func TestUpdateWithEventServerOnlyStatusContentEditRescreens(t *testing.T) {
	t.Run("keyword", func(t *testing.T) {
		f := newScreenFixture(t, "published", allowAI())
		if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("orig", "a replica item", 100, "banned"), "seller-a", false, noEvent); err != nil {
			t.Fatalf("UpdateWithEvent: %v", err)
		}
		if got := f.stored(t); got.Status != "pending_review" {
			t.Fatalf("status = %q, want pending_review (not published, not banned)", got.Status)
		}
	})
	t.Run("ai runs once", func(t *testing.T) {
		f := newScreenFixture(t, "published", &fakeAI{verdict: aiv1.ScreenVerdict_SCREEN_VERDICT_REVIEW, labels: []string{"weapon"}})
		if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("orig", "new text", 100, "banned"), "seller-a", false, noEvent); err != nil {
			t.Fatalf("UpdateWithEvent: %v", err)
		}
		if got := f.stored(t); got.Status != "pending_review" || f.ai.calls != 1 {
			t.Fatalf("status=%q ai=%d, want pending_review and exactly 1 screen", got.Status, f.ai.calls)
		}
	})
}

// Scenario: Auto-screen: Draft published later is screened at that moment.
func TestUpdateWithEventDraftToPublishScreens(t *testing.T) {
	f := newScreenFixture(t, "draft", allowAI())
	if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("replica", "desc", 100, "published"), "seller-a", false, noEvent); err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if got := f.stored(t); got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
}

// Scenario: Status ownership: Seller edit of a rejected listing goes to manual review.
func TestUpdateWithEventRejectedGoesToPendingReview(t *testing.T) {
	for _, sent := range []string{"published", "draft", ""} {
		t.Run("sent="+sent, func(t *testing.T) {
			f := newScreenFixture(t, "rejected", allowAI())
			if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("orig", "desc", 100, sent), "seller-a", false, noEvent); err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			got := f.stored(t)
			if got.Status != "pending_review" {
				t.Fatalf("status = %q, want pending_review (ALLOW verdict must not publish)", got.Status)
			}
			if got.SubmittedAt.IsZero() {
				t.Error("submitted_at must be set")
			}
		})
	}
}

// Scenario: Status ownership: Seller can withdraw a published listing to draft.
func TestUpdateWithEventPublishedToDraft(t *testing.T) {
	f := newScreenFixture(t, "published", allowAI())
	var c capture
	if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("orig", "desc", 100, "draft"), "seller-a", false, c.enqueue); err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if got := f.stored(t); got.Status != "draft" {
		t.Fatalf("status = %q, want draft", got.Status)
	}
	if len(c.statuses) != 1 || c.statuses[0] != "draft" {
		t.Fatalf("event statuses = %v, want [draft]", c.statuses)
	}
}

// Scenario: Status ownership: Banned listing refuses seller writes / Held listing refuses seller writes.
func TestUpdateWithEventLockedStatusRefused(t *testing.T) {
	for _, st := range []string{"banned", "held"} {
		t.Run(st, func(t *testing.T) {
			f := newScreenFixture(t, st, allowAI())
			_, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("changed", "desc", 999, "published"), "seller-a", false, noEvent)
			if !errors.Is(err, listing.ErrStatusLocked) {
				t.Fatalf("err = %v, want ErrStatusLocked", err)
			}
			got := f.stored(t)
			if got.Status != st || got.Title != "orig" || len(f.txw.Rows()) != 0 {
				t.Fatalf("row changed: status=%q title=%q rows=%d", got.Status, got.Title, len(f.txw.Rows()))
			}
		})
	}
}

// Scenario: Status ownership: Banned or held listing refuses seller delete.
func TestDeleteWithEventLockedStatusRefused(t *testing.T) {
	for _, st := range []string{"banned", "held"} {
		t.Run(st, func(t *testing.T) {
			f := newScreenFixture(t, st, allowAI())
			_, err := f.svc.DeleteWithEvent(context.Background(), "L-1", "seller-a", false, noEvent)
			if !errors.Is(err, listing.ErrStatusLocked) {
				t.Fatalf("err = %v, want ErrStatusLocked", err)
			}
			if got := f.stored(t); got.Status != st || len(f.txw.Rows()) != 0 {
				t.Fatalf("status=%q rows=%d, want unchanged and no outbox row", got.Status, len(f.txw.Rows()))
			}
		})
	}
}

// Scenario: Status ownership: Admin's own listing goes through the screen.
func TestAdminOwnListingFollowsSellerPath(t *testing.T) {
	f := newScreenFixture(t, "draft", allowAI())
	log := &[]string{}
	fa := &fakeAudit{log: log}
	f.svc = f.svc.WithAudit(gateOf(fa))
	created, err := f.svc.CreateWithEvent(context.Background(), newListing("replica", "published"), "admin-1", noEvent)
	if err != nil {
		t.Fatalf("CreateWithEvent: %v", err)
	}
	if created.Status != "pending_review" || !reflect.DeepEqual(created.ScreenFlags, []string{"keyword:replica"}) {
		t.Fatalf("status=%q flags=%v, want pending_review [keyword:replica]", created.Status, created.ScreenFlags)
	}
	upd := created
	upd.Description = "other description"
	if _, err := f.svc.UpdateWithEvent(context.Background(), upd, "admin-1", true, noEvent); err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if len(fa.calls) != 1 || fa.calls[0].kind != "update" {
		t.Fatalf("audit calls = %+v, want exactly 1 update", fa.calls)
	}
}

// Scenario: Status ownership: Admin content edit does not re-screen.
func TestUpdateWithEventAdminEditNoScreen(t *testing.T) {
	for _, title := range []string{"better title", "replica title"} {
		t.Run(title, func(t *testing.T) {
			f := newScreenFixture(t, "published", allowAI())
			f.svc = f.svc.WithAudit(gateOf(&fakeAudit{log: new([]string)}))
			if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate(title, "desc", 100, ""), "admin-1", true, noEvent); err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			got := f.stored(t)
			if got.Title != title || got.Status != "published" || f.ai.calls != 0 {
				t.Fatalf("title=%q status=%q ai=%d, want %q published 0", got.Title, got.Status, f.ai.calls, title)
			}
		})
	}
}

// Scenario: Status ownership: Admin UpdateListing cannot set status (service side).
func TestUpdateWithEventAdminCannotSetStatus(t *testing.T) {
	for _, sent := range []string{"published", "draft", "banned", "pending_review"} {
		t.Run("sent="+sent, func(t *testing.T) {
			f := newScreenFixture(t, "rejected", allowAI())
			fa := &fakeAudit{log: new([]string)}
			f.svc = f.svc.WithAudit(gateOf(fa))
			var c capture
			if _, err := f.svc.UpdateWithEvent(context.Background(), fullUpdate("orig", "desc", 100, sent), "admin-1", true, c.enqueue); err != nil {
				t.Fatalf("UpdateWithEvent: %v", err)
			}
			if len(fa.calls) != 1 || fa.calls[0].status != "rejected" {
				t.Fatalf("audit calls = %+v, want 1 with status rejected", fa.calls)
			}
			if got := f.stored(t); got.Status != "rejected" {
				t.Fatalf("status = %q, want rejected", got.Status)
			}
			if len(c.statuses) != 1 || c.statuses[0] != "rejected" {
				t.Fatalf("event statuses = %v, want [rejected]", c.statuses)
			}
		})
	}
}

// Scenario: Seller hold: Stock cannot be reserved on a held listing (service/repository contract).
func TestReserveStockIdempotentHeldListingNotReservable(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewInMemoryListingRepository(store.Listing{ID: "L1", Currency: "VND", Status: "held", Stock: 5, SellerID: "seller-a"})
	svc := listing.NewListingService(repo)
	err := svc.ReserveStockIdempotent(ctx, "r-new", "L1", "", 2)
	if !errors.Is(err, store.ErrListingNotReservable) {
		t.Fatalf("err = %v, want ErrListingNotReservable", err)
	}
	if got, _ := repo.Get(ctx, "L1"); got.Stock != 5 {
		t.Fatalf("stock = %d, want 5", got.Stock)
	}
}

// restorer is the hold/restore capability the in-memory writer must expose (mirrors PgTxWriter).
type restorer interface {
	HoldSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue store.EnqueueFn) (int, error)
	RestoreSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue store.EnqueueFn) (int, error)
}

// queueRepo adds the pending_review queue read the Postgres repository provides.
type queueRepo struct {
	*storetest.InMemoryListingRepository
}

func (r queueRepo) ListModerationQueue(ctx context.Context, cursor string, pageSize int32) (store.Page, error) {
	return r.List(ctx, cursor, pageSize, "pending_review")
}

// Scenario: Seller hold: Restored pending listing is back in the moderation queue
func TestRestoreSellerListingsPendingReturnsToQueueWithoutScreen(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Description: "desc", CategoryID: "c1", Currency: "VND", Price: 100,
		Status: "pending_review", SellerID: "seller-a", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	ai := allowAI()
	svc := listing.NewListingServiceWithOutbox(queueRepo{repo}, txw).WithScreener(termsFile(t), ai)

	rs, ok := any(txw).(restorer)
	if !ok {
		t.Fatalf("InMemoryTxWriter must implement RestoreSellerListings like PgTxWriter")
	}
	t0 := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if n, err := rs.HoldSellerListings(ctx, "seller-a", "ev-1", t0, noEvent); err != nil || n != 1 {
		t.Fatalf("HoldSellerListings: n=%d err=%v", n, err)
	}
	if held, _ := repo.Get(ctx, "L-1"); held.Status != "held" {
		t.Fatalf("status after hold = %q, want held", held.Status)
	}
	if _, err := rs.RestoreSellerListings(ctx, "seller-a", "ev-2", t0.Add(time.Hour), noEvent); err != nil {
		t.Fatalf("RestoreSellerListings: %v", err)
	}
	got, _ := repo.Get(ctx, "L-1")
	if got.Status != "pending_review" {
		t.Fatalf("status = %q, want pending_review", got.Status)
	}
	page, err := svc.ListModerationQueue(ctx, "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "L-1" {
		t.Fatalf("queue = %+v err=%v, want [L-1]", page.Items, err)
	}
	if ai.calls != 0 {
		t.Fatalf("screen calls = %d, want 0 during restore", ai.calls)
	}
}
