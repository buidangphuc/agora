package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
)

func sweepStores() (*repository.InMemoryOrderRepository, *repository.InMemoryReturnRepository) {
	orders := repository.NewInMemoryOrderRepository(repository.WithAutoCompleteAfter(7 * 24 * time.Hour))
	returns := repository.NewInMemoryReturnRepository(repository.WithReturnAutoApproveAfter(72 * time.Hour))
	orders.SetReturnSource(returns)
	return orders, returns
}

func shipOrder(t *testing.T, orders *repository.InMemoryOrderRepository, paid bool) repository.Order {
	t.Helper()
	ctx := context.Background()
	o, err := orders.CreateOrder(ctx, repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 100, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	from := repository.OrderStatusPending
	if paid {
		if _, err = orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid, []repository.OrderStatus{from}, ""); err != nil {
			t.Fatal(err)
		}
		from = repository.OrderStatusPaid
	}
	if o, err = orders.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusShipped, []repository.OrderStatus{from}, "T"); err != nil {
		t.Fatal(err)
	}
	return o
}

// Scenarios: Order receipt: Unconfirmed shipped order completes after the window;
// A shipped order inside its window is not completed;
// An open return blocks auto-complete; An approved unrefunded return blocks auto-complete;
// An approved return on a cash-on-delivery order does not block auto-complete;
// A rejected return does not block auto-complete
func TestListAutoCompletableSelectsDueUnblockedShipped(t *testing.T) {
	ctx := context.Background()
	orders, returns := sweepStores()
	due := shipOrder(t, orders, true)
	shipOrder(t, orders, true) // also due; used below as the blocked ones
	pend := shipOrder(t, orders, true)
	appr := shipOrder(t, orders, true)
	codAppr := shipOrder(t, orders, false)
	rej := shipOrder(t, orders, true)
	mk := func(orderID string, to repository.ReturnStatus) {
		ret, err := returns.CreateReturn(ctx, repository.OrderReturn{OrderID: orderID, BuyerID: "b", SellerID: "s", Status: repository.ReturnStatusPending})
		if err != nil {
			t.Fatal(err)
		}
		if to != repository.ReturnStatusPending {
			if _, err := returns.TransitionReturn(ctx, ret.ID, repository.ReturnStatusPending, to); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk(pend.ID, repository.ReturnStatusPending)
	mk(appr.ID, repository.ReturnStatusApproved)
	mk(codAppr.ID, repository.ReturnStatusApproved)
	mk(rej.ID, repository.ReturnStatusRejected)

	got, err := orders.ListAutoCompletable(ctx, time.Now().Add(30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, o := range got {
		ids[o.ID] = true
	}
	if !ids[due.ID] || !ids[codAppr.ID] || !ids[rej.ID] {
		t.Fatalf("due, COD-with-approved-return and rejected-return orders must be selected: %v", ids)
	}
	if ids[pend.ID] || ids[appr.ID] {
		t.Fatalf("orders blocked by a PENDING or online-paid APPROVED return must not be selected: %v", ids)
	}
	early, err := orders.ListAutoCompletable(ctx, time.Now(), 100)
	if err != nil || len(early) != 0 {
		t.Fatalf("nothing is due before the deadline: %v %v", early, err)
	}
}

// Scenario: Order receipt: The sweep handles at most one batch per run
func TestListAutoCompletableHonoursLimitOldestFirst(t *testing.T) {
	ctx := context.Background()
	orders, _ := sweepStores()
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, shipOrder(t, orders, true).ID)
	}
	got, err := orders.ListAutoCompletable(ctx, time.Now().Add(30*24*time.Hour), 3)
	if err != nil || len(got) != 3 {
		t.Fatalf("want 3 rows, got %d (%v)", len(got), err)
	}
	for i := 1; i < len(got); i++ {
		if got[i].AutoCompleteAt.Before(*got[i-1].AutoCompleteAt) {
			t.Fatal("rows must be ordered by auto_complete_at ascending")
		}
	}
}

// Scenarios: Order receipt: Unanswered return is approved after the window;
// A return inside its answer window is not approved; Rejected return is never auto-approved;
// Approved return is not touched by the sweep; The auto-approve pass handles at most one batch per run
func TestListAutoApprovableSelectsOnlyDuePending(t *testing.T) {
	ctx := context.Background()
	_, returns := sweepStores()
	mk := func(to repository.ReturnStatus) repository.OrderReturn {
		ret, err := returns.CreateReturn(ctx, repository.OrderReturn{OrderID: "o", BuyerID: "b", SellerID: "s", Status: repository.ReturnStatusPending})
		if err != nil {
			t.Fatal(err)
		}
		if to != repository.ReturnStatusPending {
			if ret, err = returns.TransitionReturn(ctx, ret.ID, repository.ReturnStatusPending, to); err != nil {
				t.Fatal(err)
			}
		}
		return ret
	}
	pend1, pend2 := mk(repository.ReturnStatusPending), mk(repository.ReturnStatusPending)
	mk(repository.ReturnStatusRejected)
	mk(repository.ReturnStatusApproved)

	got, err := returns.ListAutoApprovable(ctx, time.Now().Add(30*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !(got[0].ID == pend1.ID && got[1].ID == pend2.ID || got[0].ID == pend2.ID && got[1].ID == pend1.ID) {
		t.Fatalf("want exactly the two PENDING returns, got %+v", got)
	}
	if got, err := returns.ListAutoApprovable(ctx, time.Now(), 100); err != nil || len(got) != 0 {
		t.Fatalf("nothing due before auto_approve_at: %v %v", got, err)
	}
	if got, _ := returns.ListAutoApprovable(ctx, time.Now().Add(30*24*time.Hour), 1); len(got) != 1 {
		t.Fatalf("limit 1 must return one row, got %d", len(got))
	}
}
