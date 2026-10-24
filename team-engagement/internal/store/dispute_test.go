package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

func TestInMemoryDisputeRepository(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeDisputeRepository()

	// 1. Create dispute
	d, err := repo.CreateDispute(ctx, store.Dispute{
		OrderID:      "order-100",
		ClaimantID:   "buyer-1",
		DefendantID:  "seller-1",
		Reason:       "Hàng không giống hình ảnh mô tả",
		EvidenceURLs: []string{"https://img.com/1.png"},
	})
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	if d.ID == "" {
		t.Fatal("expected generated dispute ID")
	}
	if d.Status != store.DisputeStatusOpen {
		t.Fatalf("expected status OPEN, got %s", d.Status)
	}

	// 2. Get dispute
	fetched, err := repo.GetDispute(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDispute: %v", err)
	}
	if fetched.Reason != "Hàng không giống hình ảnh mô tả" {
		t.Fatalf("unexpected reason: %s", fetched.Reason)
	}

	// Get nonexistent dispute
	_, err = repo.GetDispute(ctx, "nonexistent-id")
	if !errors.Is(err, store.ErrDisputeNotFound) {
		t.Fatalf("expected ErrDisputeNotFound, got %v", err)
	}

	// 3. Update dispute
	fetched.Status = store.DisputeStatusResolved
	fetched.Resolution = "Đồng ý hoàn tiền 100%"
	updated, err := repo.UpdateDispute(ctx, fetched)
	if err != nil {
		t.Fatalf("UpdateDispute: %v", err)
	}
	if updated.Status != store.DisputeStatusResolved {
		t.Fatalf("expected status RESOLVED, got %s", updated.Status)
	}
	if updated.Resolution != "Đồng ý hoàn tiền 100%" {
		t.Fatalf("unexpected resolution: %s", updated.Resolution)
	}

	// Update nonexistent dispute
	_, err = repo.UpdateDispute(ctx, store.Dispute{ID: "nonexistent-id"})
	if !errors.Is(err, store.ErrDisputeNotFound) {
		t.Fatalf("expected ErrDisputeNotFound, got %v", err)
	}
}

func disputeFact(d store.Dispute) *engagementv1.DisputeResolved {
	return &engagementv1.DisputeResolved{
		DisputeId: d.ID, OrderId: d.OrderID, ReturnId: d.ReturnID,
		Status: engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
		Ruling: d.Ruling,
	}
}

// Scenario: Dispute escalation: Concurrent creates leave one dispute (in-memory mirror)
func TestInMemoryCreateDisputeUniqueOpenPerReturn(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeDisputeRepository()
	in := store.Dispute{OrderID: "o-1", ClaimantID: "b", DefendantID: "s", Reason: "x", ReturnID: "r-1"}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, dup := 0, 0
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.CreateDispute(ctx, in)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, store.ErrOpenDisputeExists):
				dup++
			}
		}()
	}
	wg.Wait()
	if ok != 1 || dup != 4 {
		t.Fatalf("ok=%d dup=%d, want 1 and 4", ok, dup)
	}
	list, err := repo.ListByOrder(ctx, "o-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByOrder len=%d err=%v, want 1", len(list), err)
	}
}

// Scenario: Dispute escalation: A closed dispute lets the return be escalated again (in-memory mirror)
func TestInMemoryCreateDisputeAfterClosedDisputeOnReturn(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeDisputeRepository()
	in := store.Dispute{OrderID: "o-1", ClaimantID: "b", DefendantID: "s", Reason: "x", ReturnID: "r-1"}

	first, err := repo.CreateDispute(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDispute(ctx, in); !errors.Is(err, store.ErrOpenDisputeExists) {
		t.Fatalf("OPEN duplicate: err=%v, want ErrOpenDisputeExists", err)
	}
	first.Status = store.DisputeStatusInvestigating
	if _, err := repo.UpdateDispute(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDispute(ctx, in); !errors.Is(err, store.ErrOpenDisputeExists) {
		t.Fatalf("INVESTIGATING duplicate: err=%v, want ErrOpenDisputeExists", err)
	}
	first.Status = store.DisputeStatusRejected
	if _, err := repo.UpdateDispute(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDispute(ctx, in); err != nil {
		t.Fatalf("create after REJECTED: %v", err)
	}
	list, _ := repo.ListByOrder(ctx, "o-1")
	if len(list) != 2 {
		t.Fatalf("disputes for return = %d, want 2", len(list))
	}
}

// Scenario: Dispute escalation: Dispute without return_id is unchanged (in-memory mirror)
func TestInMemoryDisputesWithoutReturnNeverCollide(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeDisputeRepository()
	in := store.Dispute{OrderID: "o-1", ClaimantID: "b", DefendantID: "s", Reason: "x"}
	for i := 0; i < 3; i++ {
		if _, err := repo.CreateDispute(ctx, in); err != nil {
			t.Fatalf("create %d without return_id: %v", i, err)
		}
	}
}

// Scenario: Dispute escalation: Concurrent resolutions apply once (in-memory mirror)
func TestInMemoryResolveWithEventCompareAndSetOnce(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeDisputeRepository()
	d, err := repo.CreateDispute(ctx, store.Dispute{OrderID: "o-1", ClaimantID: "b", DefendantID: "s", Reason: "x", ReturnID: "r-1", Status: store.DisputeStatusInvestigating})
	if err != nil {
		t.Fatal(err)
	}
	d.Status = store.DisputeStatusResolved
	d.Ruling = engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR

	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ResolveWithEvent(ctx, d, store.DisputeStatusInvestigating, disputeFact(d))
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("winners=%d, want 1", wins)
	}
	got, _ := repo.GetDispute(ctx, d.ID)
	if got.Status != store.DisputeStatusResolved || got.Ruling != engagementv1.DisputeRuling_DISPUTE_RULING_BUYER_FAVOUR {
		t.Fatalf("stored status=%s ruling=%v", got.Status, got.Ruling)
	}
}

// Scenario: Dispute escalation: Admin lists open disputes oldest first (in-memory mirror)
func TestInMemoryListByStatusOldestFirstAndPagination(t *testing.T) {
	ctx := context.Background()
	repo := storetest.NewFakeDisputeRepository()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, st := range []store.DisputeStatus{store.DisputeStatusOpen, store.DisputeStatusResolved, store.DisputeStatusOpen, store.DisputeStatusOpen} {
		if _, err := repo.CreateDispute(ctx, store.Dispute{
			ID: fmt.Sprintf("d-%d", 3-i), OrderID: "o", ClaimantID: "b", DefendantID: "s", Reason: "x",
			Status: st, CreatedAt: base.Add(time.Duration(3-i) * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	page, next, err := repo.ListByStatus(ctx, store.DisputeStatusOpen, 2, "")
	if err != nil || len(page) != 2 || next == "" {
		t.Fatalf("page1 len=%d next=%q err=%v", len(page), next, err)
	}
	if page[0].ID != "d-0" || page[1].ID != "d-1" {
		t.Fatalf("page1 ids = %s,%s, want d-0,d-1 (oldest first)", page[0].ID, page[1].ID)
	}
	page, next, err = repo.ListByStatus(ctx, store.DisputeStatusOpen, 2, next)
	if err != nil || len(page) != 1 || next != "" || page[0].ID != "d-3" {
		t.Fatalf("page2 = %+v next=%q err=%v", page, next, err)
	}
	all, _, err := repo.ListByStatus(ctx, "", 100, "")
	if err != nil || len(all) != 4 {
		t.Fatalf("every status: len=%d err=%v, want 4", len(all), err)
	}
}
