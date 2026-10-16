package consumer_test

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc"

	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/repository"
)

// flashSpy is a full promotion client (voucher + flash) handed to the consumer
// through its only promotion seam; any flash RPC the consumer issues is recorded.
type flashSpy struct {
	promotionv1.FlashSaleServiceClient
	mu         sync.Mutex
	flashCalls []string
	committed  []string
}

func (f *flashSpy) note(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flashCalls = append(f.flashCalls, s)
}

func (f *flashSpy) GetActiveFlashSale(context.Context, *promotionv1.GetActiveFlashSaleRequest, ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	f.note("get")
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}
func (f *flashSpy) ReserveFlashSaleStock(context.Context, *promotionv1.ReserveFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	f.note("reserve")
	return &promotionv1.ReserveFlashSaleStockResponse{Reserved: true}, nil
}
func (f *flashSpy) CommitFlashSaleStock(context.Context, *promotionv1.CommitFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	f.note("commit")
	return &promotionv1.CommitFlashSaleStockResponse{Committed: true}, nil
}
func (f *flashSpy) ReleaseFlashSaleStock(context.Context, *promotionv1.ReleaseFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	f.note("release")
	return &promotionv1.ReleaseFlashSaleStockResponse{Released: true}, nil
}
func (f *flashSpy) CommitReservation(_ context.Context, in *promotionv1.CommitReservationRequest, _ ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.committed = append(f.committed, in.GetReservationId())
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

// Regression guard (green by design): the flash hold is committed in checkout
// Phase B, so settling payment must make no flash RPC, even for an order that
// carries a flash-priced item and a voucher.
func TestPaymentConsumerMakesNoFlashCall(t *testing.T) {
	ctx := context.Background()
	store := &countingOrderStore{orders: map[string]repository.Order{
		"ord_f": {
			ID: "ord_f", Status: repository.OrderStatusPending, VoucherCode: "SAVE", DiscountAmount: 1000,
			Items: []repository.OrderItem{{ListingID: "lst_1", Quantity: 2, UnitPrice: 350000}},
		},
	}}
	spy := &flashSpy{}
	c := consumer.NewPaymentConsumer(store, repository.NewInMemoryProcessedEventRepository(), nil, consumer.WithVoucherCommitter(spy))

	env := envelopeFor(t, "evt_f", &paymentv1.PaymentSettled{
		OrderId: "ord_f",
		Status:  paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})
	if err := c.HandleEnvelope(ctx, env); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	o, _ := store.GetOrder(ctx, "ord_f")
	if o.Status != repository.OrderStatusPaid {
		t.Fatalf("order status = %v, want Paid", o.Status)
	}
	if len(spy.flashCalls) != 0 {
		t.Errorf("consumer made flash calls: %v", spy.flashCalls)
	}
}
