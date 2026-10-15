package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

func TestListingService(t *testing.T) {
	repo := repository.NewInMemoryListingRepository()
	svc := service.NewListingService(repo)
	ctx := context.Background()

	t.Run("Create and Get Listing", func(t *testing.T) {
		l := repository.Listing{
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
		if err != service.ErrForbidden {
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
		if err != service.ErrForbidden {
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
		repo := repository.NewInMemoryListingRepository()
		txw := repository.NewInMemoryTxWriter(repo)
		svc := service.NewListingServiceWithOutbox(repo, txw)

		created, err := svc.CreateWithEvent(ctx, repository.Listing{Title: "Tx OK", Currency: "VND"}, "seller_1",
			func(l repository.Listing) (repository.OutboxRow, error) {
				return repository.OutboxRow{EventID: "evt-ok", AggregateID: l.ID, EventType: "test"}, nil
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
		repo := repository.NewInMemoryListingRepository()
		txw := repository.NewInMemoryTxWriter(repo)
		txw.FailWrite = true // simulate the business write failing inside the tx
		svc := service.NewListingServiceWithOutbox(repo, txw)

		created, err := svc.CreateWithEvent(ctx, repository.Listing{ID: "rollback-1", Title: "boom", Currency: "VND"}, "seller_1",
			func(l repository.Listing) (repository.OutboxRow, error) {
				return repository.OutboxRow{EventID: "evt-x", AggregateID: l.ID}, nil
			})
		if err == nil {
			t.Fatal("expected error when the write fails")
		}
		// Neither the listing nor an outbox row may survive a rolled-back write.
		if _, err := repo.Get(ctx, "rollback-1"); err != repository.ErrNotFound {
			t.Fatalf("listing must not be persisted on rollback, got %v", err)
		}
		if got := len(txw.Rows()); got != 0 {
			t.Fatalf("no outbox row may survive rollback, got %d", got)
		}
		_ = created
	})

	t.Run("List and ListMine", func(t *testing.T) {
		_, _ = svc.Create(ctx, repository.Listing{Title: "Item 1", Price: 100, Status: "published"}, "seller_a")
		_, _ = svc.Create(ctx, repository.Listing{Title: "Item 2", Price: 200, Status: "published"}, "seller_a")
		_, _ = svc.Create(ctx, repository.Listing{Title: "Item 3", Price: 300, Status: "published"}, "seller_b")

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

// fakeAudit is the AuditWriter fake: records calls in a shared ordered log.
type fakeAudit struct {
	log   *[]string
	calls []auditCall
	err   error
}

func (f *fakeAudit) WriteListingAdminUpdate(_ context.Context, actorID, listingID, sellerID, status string) error {
	f.calls = append(f.calls, auditCall{"update", actorID, listingID, sellerID, status})
	*f.log = append(*f.log, "audit")
	return f.err
}

func (f *fakeAudit) WriteListingAdminDelete(_ context.Context, actorID, listingID, sellerID string) error {
	f.calls = append(f.calls, auditCall{"delete", actorID, listingID, sellerID, ""})
	*f.log = append(*f.log, "audit")
	return f.err
}

// orderTx wraps the in-memory TxWriter and logs when the change happens.
type orderTx struct {
	*repository.InMemoryTxWriter
	log *[]string
}

func (o orderTx) UpdateTx(ctx context.Context, l repository.Listing, ev repository.EnqueueFn) (repository.Listing, error) {
	*o.log = append(*o.log, "change")
	return o.InMemoryTxWriter.UpdateTx(ctx, l, ev)
}

func (o orderTx) DeleteTx(ctx context.Context, id string, ev repository.EnqueueFn) (repository.Listing, error) {
	*o.log = append(*o.log, "change")
	return o.InMemoryTxWriter.DeleteTx(ctx, id, ev)
}

type auditFixture struct {
	repo  *repository.InMemoryListingRepository
	txw   *repository.InMemoryTxWriter
	svc   *service.ListingService
	audit *fakeAudit
	log   *[]string
}

func newAuditFixture(t *testing.T) *auditFixture {
	t.Helper()
	log := &[]string{}
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Status: "published", SellerID: "seller-1", Stock: 1,
	})
	txw := repository.NewInMemoryTxWriter(repo)
	fa := &fakeAudit{log: log}
	svc := service.NewListingServiceWithOutbox(repo, orderTx{txw, log}).WithAudit(fa)
	return &auditFixture{repo: repo, txw: txw, svc: svc, audit: fa, log: log}
}

func enq(l repository.Listing) (repository.OutboxRow, error) {
	return repository.OutboxRow{EventID: "e-" + l.ID, AggregateID: l.ID, EventType: "test"}, nil
}

// Scenario: Listing audit: Audit is written before the change.
func TestUpdateWithEventAuditPrecedesUpdate(t *testing.T) {
	f := newAuditFixture(t)
	// The request tries to claim another owner; the event must carry the stored one.
	_, err := f.svc.UpdateWithEvent(context.Background(),
		repository.Listing{ID: "L-1", Title: "new", Status: "rejected", SellerID: "forged"}, "admin-1", true, enq)
	if err != nil {
		t.Fatalf("UpdateWithEvent: %v", err)
	}
	if got := *f.log; len(got) != 2 || got[0] != "audit" || got[1] != "change" {
		t.Fatalf("order = %v, want [audit change]", got)
	}
	want := auditCall{"update", "admin-1", "L-1", "seller-1", "rejected"}
	if len(f.audit.calls) != 1 || f.audit.calls[0] != want {
		t.Fatalf("audit calls = %+v, want [%+v]", f.audit.calls, want)
	}
}

// Scenario: Listing audit: Admin who owns the listing is audited.
func TestUpdateWithEventAdminOwnerIsAudited(t *testing.T) {
	f := newAuditFixture(t)
	if _, err := f.svc.UpdateWithEvent(context.Background(),
		repository.Listing{ID: "L-1", Title: "mine", Status: "published"}, "seller-1", true, enq); err != nil {
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
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("err = %v, want ErrAuditUnavailable", err)
	}
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
				repository.Listing{ID: "L-1", Title: "new", Status: "rejected"}, "admin-1", true, enq)
			if !errors.Is(err, service.ErrAuditUnavailable) {
				t.Fatalf("err = %v, want ErrAuditUnavailable", err)
			}
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

// Scenario: Listing audit: No audit upstream configured.
func TestUpdateWithEventAdminNilAuditUnavailable(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L-1", Title: "orig", SellerID: "seller-1", Status: "published"})
	txw := repository.NewInMemoryTxWriter(repo)
	svc := service.NewListingServiceWithOutbox(repo, txw) // no WithAudit
	_, err := svc.UpdateWithEvent(context.Background(), repository.Listing{ID: "L-1", Title: "new"}, "admin-1", true, enq)
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("update err = %v, want ErrAuditUnavailable", err)
	}
	_, err = svc.DeleteWithEvent(context.Background(), "L-1", "admin-1", true, enq)
	if !errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("delete err = %v, want ErrAuditUnavailable", err)
	}
	got, err := repo.Get(context.Background(), "L-1")
	if err != nil || got.Title != "orig" {
		t.Fatalf("listing must be unchanged, got %+v, %v", got, err)
	}
	// The owner (no admin scope) is unaffected by the missing audit upstream.
	if _, err := svc.UpdateWithEvent(context.Background(), repository.Listing{ID: "L-1", Title: "own"}, "seller-1", false, enq); err != nil {
		t.Fatalf("owner update must succeed without audit: %v", err)
	}
}

// Scenario: Listing audit: Unknown listing writes no audit event.
func TestUpdateWithEventNotFoundNoAudit(t *testing.T) {
	f := newAuditFixture(t)
	_, err := f.svc.UpdateWithEvent(context.Background(), repository.Listing{ID: "nope"}, "admin-1", true, enq)
	if !errors.Is(err, repository.ErrNotFound) {
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
	if !errors.Is(err, repository.ErrNotFound) {
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
	_, err := f.svc.UpdateWithEvent(context.Background(), repository.Listing{ID: "L-1", Title: "new"}, "admin-1", true, enq)
	if err == nil || errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("err = %v, want the repository failure (not ErrAuditUnavailable)", err)
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
	if err == nil || errors.Is(err, service.ErrAuditUnavailable) {
		t.Fatalf("err = %v, want the repository failure (not ErrAuditUnavailable)", err)
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
	if _, err := f.svc.UpdateWithEvent(ctx, repository.Listing{ID: "L-1", Title: "own"}, "seller-1", false, enq); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	if _, err := f.svc.UpdateWithEvent(ctx, repository.Listing{ID: "L-1", Title: "x"}, "seller-2", false, enq); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("non-owner update err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.DeleteWithEvent(ctx, "L-1", "seller-2", false, enq); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("non-owner delete err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.DeleteWithEvent(ctx, "L-1", "seller-1", false, enq); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}
