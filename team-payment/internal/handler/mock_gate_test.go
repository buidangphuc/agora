package handler_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/handler"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

const leakSecret = "pq: connection refused host=10.1.2.3 password=hunter2"

// failingPayments fails every read with a secret-bearing error.
type failingPayments struct {
	*repository.InMemoryPaymentRepository
}

func (failingPayments) GetTransaction(context.Context, string) (repository.PaymentTransaction, error) {
	return repository.PaymentTransaction{}, errors.New(leakSecret)
}

func (failingPayments) GetTransactionByOrderID(context.Context, string) (repository.PaymentTransaction, error) {
	return repository.PaymentTransaction{}, errors.New(leakSecret)
}

func TestProcessMockPayment_DisabledByDefault(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	repo := repository.NewInMemoryPaymentRepository()
	svc := service.NewPaymentService(repo, repository.NewInMemoryWalletRepository(), nil, logger,
		service.WithLedgerRepo(repository.NewInMemoryLedgerRepository()))
	h := handler.NewPaymentHandler(svc, logger) // no WithMockPayments

	tx, err := repo.CreateTransaction(context.Background(), repository.PaymentTransaction{
		OrderID: "order-1", BuyerID: "buyer-1", Amount: 1000, Currency: "VND",
		Status: repository.PaymentStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, simulate := range []bool{true, false} {
		_, err = h.ProcessMockPayment(principalCtx("buyer-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, "admin"), &paymentv1.ProcessMockPaymentRequest{
			TransactionId: tx.ID, SimulateSuccess: simulate,
		})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("want FailedPrecondition, got %v", err)
		}
	}
	got, _ := repo.GetTransaction(context.Background(), tx.ID)
	if got.Status != repository.PaymentStatusPending {
		t.Fatalf("transaction changed while mock payments disabled: %v", got.Status)
	}
}

func TestInternalErrorsAreNotEchoed(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := service.NewPaymentService(
		failingPayments{repository.NewInMemoryPaymentRepository()},
		repository.NewInMemoryWalletRepository(), nil, logger,
		service.WithLedgerRepo(repository.NewInMemoryLedgerRepository()))
	h := handler.NewPaymentHandler(svc, logger, handler.WithMockPayments(true))
	ctx := principalCtx("buyer-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER)

	_, err1 := h.GetPayment(ctx, &paymentv1.GetPaymentRequest{Id: "tx-1"})
	_, err2 := h.ProcessMockPayment(ctx, &paymentv1.ProcessMockPaymentRequest{TransactionId: "tx-1", SimulateSuccess: true})
	for i, err := range []error{err1, err2} {
		if status.Code(err) != codes.Internal {
			t.Fatalf("case %d: want Internal, got %v", i, err)
		}
		if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "10.1.2.3") {
			t.Fatalf("case %d: internal error leaked cause: %v", i, err)
		}
	}
	if !strings.Contains(logs.String(), "hunter2") {
		t.Fatalf("cause must be logged server-side, logs: %s", logs.String())
	}
}
