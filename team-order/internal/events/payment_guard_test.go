package events_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

// flakyCommitter fails its first `failures` commits, then succeeds.
type flakyCommitter struct {
	mu        sync.Mutex
	failures  int
	attempts  int
	committed []string
}

func (f *flakyCommitter) CommitReservation(_ context.Context, in *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.failures > 0 {
		f.failures--
		return nil, errors.New("promotion unavailable")
	}
	f.committed = append(f.committed, in.GetReservationId())
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

func paid(orderID string) *paymentv1.PaymentSettled {
	return &paymentv1.PaymentSettled{PaymentId: "pay-" + orderID, OrderId: orderID, Status: paymentv1.PaymentStatus_PAYMENT_STATUS_PAID}
}

func TestPaymentConsumer_LateSuccessAfterCancelIsIgnored(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_c": {ID: "ord_c", Status: store.OrderStatusCancelled, VoucherCode: "ONE"},
	}}
	promo := &flakyCommitter{}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil, events.WithVoucherCommitter(promo))
	if err := c.HandleEnvelope(ctx, envelopeFor(t, "evt_late", paid("ord_c"))); err != nil {
		t.Fatalf("a late payment must be acknowledged, got %v", err)
	}
	o, _ := fake.GetOrder(ctx, "ord_c")
	if o.Status != store.OrderStatusCancelled || o.PaidAt != nil {
		t.Fatalf("order must stay Cancelled and unpaid: %+v", o)
	}
	if promo.attempts != 0 {
		t.Fatalf("a cancelled order's voucher must never be committed (%d attempts)", promo.attempts)
	}
}

func TestPaymentConsumer_VoucherCommitErrorRedeliversThenCommitsOnce(t *testing.T) {
	ctx := context.Background()
	fake := &countingOrderStore{orders: map[string]store.Order{
		"ord_v": {ID: "ord_v", Status: store.OrderStatusPending, VoucherCode: "SAVE"},
	}}
	promo := &flakyCommitter{failures: 1}
	c := events.NewPaymentConsumer(store.NewInMemoryTransactor(fake), nil, events.WithVoucherCommitter(promo))
	env := envelopeFor(t, "evt_v", paid("ord_v"))

	if err := c.HandleEnvelope(ctx, env); err == nil {
		t.Fatal("a voucher commit error must be returned so the event redelivers")
	}
	if o, _ := fake.GetOrder(ctx, "ord_v"); o.Status != store.OrderStatusPaid {
		t.Fatalf("the order is Paid after the first delivery, got %v", o.Status)
	}
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if err := c.HandleEnvelope(ctx, env); err != nil { // deduped now
		t.Fatalf("third delivery: %v", err)
	}
	if len(promo.committed) != 1 || promo.committed[0] != "ord_v" {
		t.Fatalf("voucher must be committed exactly once: %v", promo.committed)
	}
	if fake.updates != 1 {
		t.Fatalf("exactly one PAID transition, got %d", fake.updates)
	}
}

// A settlement racing a cancel on the same Pending order ends Cancelled with the
// stock restored once, whichever write lands first.
func TestPaymentConsumer_PaymentRacingCancelEndsCancelled(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ctx := context.Background()
			domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 10})
			orders := store.NewInMemoryOrderRepository()
			sagas := store.NewInMemorySagaRepository()
			carts := store.NewInMemoryCartRepository()
			_, _ = carts.AddItem(ctx, store.CartItem{UserID: "b", ListingID: "lst_1", Quantity: 2, UnitPrice: 10, SellerID: "s"})
			svc := order.NewOrderService(orders, carts, nil, nil, domain, nil, nil,
				order.WithSagaRepository(sagas), order.WithReleaseRetry(time.Second, 1, time.Millisecond))
			placed, err := svc.CreateOrdersFromCart(ctx, "b", store.Address{}, nil, 1, "")
			if err != nil {
				t.Fatal(err)
			}
			id := placed[0].ID
			c := events.NewPaymentConsumer(store.NewInMemoryTransactor(orders), nil)

			var wg sync.WaitGroup
			start := make(chan struct{})
			var cancelErr, payErr error
			wg.Add(2)
			go func() { defer wg.Done(); <-start; _, cancelErr = svc.CancelOrder(ctx, id) }()
			go func() { defer wg.Done(); <-start; payErr = c.HandleEnvelope(ctx, envelopeFor(t, "evt-"+id, paid(id))) }()
			close(start)
			wg.Wait()

			if cancelErr != nil || payErr != nil {
				t.Fatalf("cancel=%v pay=%v", cancelErr, payErr)
			}
			o, _ := orders.GetOrder(ctx, id)
			if o.Status != store.OrderStatusCancelled {
				t.Fatalf("order must end Cancelled, is %v", o.Status)
			}
			if domain.Stock("lst_1") != 10 || domain.Calls.Release != 1 {
				t.Fatalf("stock %d after %d releases, want 10 after 1", domain.Stock("lst_1"), domain.Calls.Release)
			}
		})
	}
}
