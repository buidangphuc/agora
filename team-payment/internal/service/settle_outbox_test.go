package service_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/protobuf/proto"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/events"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

// setupOutboxService wires the service with a real in-memory transactional
// writer over the SAME payment repo, so settle exercises the atomic
// status-write + outbox-enqueue path (AD4 / SA-H3).
func setupOutboxService() (*service.PaymentService, *repository.InMemoryPaymentRepository, *repository.InMemoryTxWriter, *mockOrderClient) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	paymentRepo := repository.NewInMemoryPaymentRepository()
	walletRepo := repository.NewInMemoryWalletRepository()
	txWriter := repository.NewInMemoryTxWriter(paymentRepo)
	orderClient := &mockOrderClient{orders: make(map[string]*orderv1.Order)}
	svc := service.NewPaymentService(paymentRepo, walletRepo, orderClient, logger, service.WithTxWriter(txWriter))
	return svc, paymentRepo, txWriter, orderClient
}

func TestService_Settle_WritesPaymentAndOutboxAtomically(t *testing.T) {
	ctx := context.Background()

	t.Run("settle writes payment=SETTLED and a PaymentSettled outbox row in one tx", func(t *testing.T) {
		svc, paymentRepo, txWriter, orderClient := setupOutboxService()
		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			OrderID: "order-settle-1",
			BuyerID: "buyer-1",
			Amount:  300000,
			Status:  repository.PaymentStatusPending,
		})

		updated, ok, _, err := svc.ProcessMockPayment(ctx, tx.ID, true)
		if err != nil {
			t.Fatalf("ProcessMockPayment failed: %v", err)
		}
		if !ok {
			t.Fatal("expected success = true")
		}

		// 1. Payment persisted as PAID (SETTLED).
		if updated.Status != repository.PaymentStatusPaid {
			t.Fatalf("expected payment PAID, got %v", updated.Status)
		}
		stored, err := paymentRepo.GetTransaction(ctx, tx.ID)
		if err != nil {
			t.Fatalf("GetTransaction: %v", err)
		}
		if stored.Status != repository.PaymentStatusPaid {
			t.Fatalf("stored payment not PAID, got %v", stored.Status)
		}

		// 2. Exactly one PaymentSettled outbox row written in the same tx.
		rows := txWriter.Rows()
		if len(rows) != 1 {
			t.Fatalf("expected 1 outbox row, got %d", len(rows))
		}
		row := rows[0]
		if row.EventType != events.PaymentSettledEventType {
			t.Fatalf("expected event type %q, got %q", events.PaymentSettledEventType, row.EventType)
		}
		if row.AggregateID != "order-settle-1" {
			t.Fatalf("expected Kafka key (aggregate_id) = order_id, got %q", row.AggregateID)
		}
		if row.EventID == "" {
			t.Fatal("expected non-empty event_id (outbox row id)")
		}

		// 3. Payload is a valid EventEnvelope carrying PaymentSettled with the
		//    right ids/status; event_id matches the outbox row id (dedupe key).
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(row.Payload, &env); err != nil {
			t.Fatalf("unmarshal EventEnvelope: %v", err)
		}
		if env.GetEventId() != row.EventID {
			t.Fatalf("envelope event_id %q != outbox event_id %q", env.GetEventId(), row.EventID)
		}
		if env.GetType() != events.PaymentSettledEventType {
			t.Fatalf("envelope type %q", env.GetType())
		}
		var settled paymentv1.PaymentSettled
		if err := proto.Unmarshal(env.GetPayload(), &settled); err != nil {
			t.Fatalf("unmarshal PaymentSettled: %v", err)
		}
		if settled.GetOrderId() != "order-settle-1" || settled.GetPaymentId() != tx.ID {
			t.Fatalf("unexpected PaymentSettled ids: %+v", &settled)
		}
		if settled.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_PAID {
			t.Fatalf("expected PaymentSettled status PAID, got %v", settled.GetStatus())
		}
		if settled.GetOccurredAt() == "" {
			t.Fatal("expected non-empty occurred_at (RFC3339 read-model version)")
		}

		// 4. No synchronous order transition happened (event-carried only).
		if orderClient.updateCalls != 0 {
			t.Fatalf("expected 0 UpdateOrderStatus calls, got %d", orderClient.updateCalls)
		}
	})

	t.Run("rollback leaves neither payment=SETTLED nor an outbox row", func(t *testing.T) {
		svc, paymentRepo, txWriter, _ := setupOutboxService()
		txWriter.FailWrite = true // simulate the tx failing to commit

		tx, _ := paymentRepo.CreateTransaction(ctx, repository.PaymentTransaction{
			OrderID: "order-settle-2",
			BuyerID: "buyer-2",
			Amount:  400000,
			Status:  repository.PaymentStatusPending,
		})

		_, _, _, err := svc.ProcessMockPayment(ctx, tx.ID, true)
		if err == nil {
			t.Fatal("expected settle to fail when the transaction cannot commit")
		}

		// Payment must remain PENDING ...
		stored, gerr := paymentRepo.GetTransaction(ctx, tx.ID)
		if gerr != nil {
			t.Fatalf("GetTransaction: %v", gerr)
		}
		if stored.Status != repository.PaymentStatusPending {
			t.Fatalf("expected payment to remain PENDING after rollback, got %v", stored.Status)
		}
		// ... and NO outbox row must exist.
		if rows := txWriter.Rows(); len(rows) != 0 {
			t.Fatalf("expected 0 outbox rows after rollback, got %d", len(rows))
		}
	})
}
