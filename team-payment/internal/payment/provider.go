package payment

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	"github.com/buidangphuc/team-payment/internal/store"
)

const providerAdapter = "payment-provider"

type Outcome struct {
	Paid              bool
	ProviderReference string
}

type Provider interface {
	Settle(ctx context.Context, tx store.PaymentTransaction, simulateSuccess bool) (Outcome, error)
}

type liveProvider struct{}

func (liveProvider) Settle(context.Context, store.PaymentTransaction, bool) (Outcome, error) {
	return Outcome{}, status.Error(codes.FailedPrecondition, "mock payments are disabled")
}

type fixtureProvider struct{}

func (fixtureProvider) Settle(_ context.Context, tx store.PaymentTransaction, simulateSuccess bool) (Outcome, error) {
	if simulateSuccess {
		return Outcome{Paid: true, ProviderReference: "mock-" + tx.ID}, nil
	}
	return Outcome{ProviderReference: "MOCK-FAIL-REJECTED"}, nil
}

func NewProvider(set *mockdata.Set) Provider {
	return mockdata.Select[Provider](set, providerAdapter, liveProvider{}, fixtureProvider{})
}
