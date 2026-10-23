package payment_test

import (
	"io"
	"log/slog"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

type svcParts struct {
	writer     store.PaymentTxWriter
	ledger     store.LedgerRepository
	settlement store.SettlementLedger
}

type svcOpt func(*svcParts)

func withLedger(l store.LedgerRepository) svcOpt     { return func(p *svcParts) { p.ledger = l } }
func withWriter(w store.PaymentTxWriter) svcOpt      { return func(p *svcParts) { p.writer = w } }
func withSettlement(s store.SettlementLedger) svcOpt { return func(p *svcParts) { p.settlement = s } }

func newSvc(payments store.PaymentRepository, wallet store.WalletRepository, orders upstream.OrderClient, logger *slog.Logger, opts ...svcOpt) *payment.Service {
	parts := &svcParts{}
	for _, o := range opts {
		o(parts)
	}
	mem, _ := payments.(*fake.InMemoryPaymentRepository)
	if payments == nil {
		mem = fake.NewInMemoryPaymentRepository()
		payments = mem
	}
	if mem == nil {
		mem = fake.NewInMemoryPaymentRepository()
	}
	if wallet == nil {
		wallet = fake.NewInMemoryWalletRepository()
	}
	if orders == nil {
		orders = &mockOrderClient{orders: map[string]*orderv1.Order{}}
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if parts.writer == nil {
		parts.writer = fake.NewInMemoryTxWriter(mem)
	}
	if parts.ledger == nil {
		parts.ledger = fake.NewInMemoryLedgerRepository()
	}
	if parts.settlement == nil {
		mled, ok := parts.ledger.(*fake.InMemoryLedgerRepository)
		if !ok {
			mled = fake.NewInMemoryLedgerRepository()
		}
		parts.settlement = fake.NewInMemorySettlementLedger(mem, mled)
	}
	svc, err := payment.NewService(payments, wallet, orders, logger, parts.writer, parts.ledger, parts.settlement, payment.WithProvider(fixtureProvider()))
	if err != nil {
		panic(err)
	}
	return svc
}

func fixtureProvider() payment.Provider {
	set, err := mockdata.New(sdkconfig.MockData{Adapters: "payment-provider"}, "payment-provider")
	if err != nil {
		panic(err)
	}
	return payment.NewProvider(set)
}
