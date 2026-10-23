package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/rpc"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
	"github.com/buidangphuc/team-payment/internal/upstream"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func principalCtx(id string, ptype auth.PrincipalType, scopes ...string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: ptype, Scopes: scopes})
}

func fixtureProvider() payment.Provider {
	set, err := mockdata.New(sdkconfig.MockData{Adapters: "payment-provider"}, "payment-provider")
	if err != nil {
		panic(err)
	}
	return payment.NewProvider(set)
}

func liveProvider() payment.Provider {
	set, err := mockdata.New(sdkconfig.MockData{}, "payment-provider")
	if err != nil {
		panic(err)
	}
	return payment.NewProvider(set)
}

type stores struct {
	payments   *fake.InMemoryPaymentRepository
	payRepo    store.PaymentRepository
	wallets    *fake.InMemoryWalletRepository
	ledger     *fake.InMemoryLedgerRepository
	writer     *fake.InMemoryTxWriter
	settlement store.SettlementLedger
}

func newStores() *stores {
	payments := fake.NewInMemoryPaymentRepository()
	ledger := fake.NewInMemoryLedgerRepository()
	return &stores{
		payments:   payments,
		payRepo:    payments,
		wallets:    fake.NewInMemoryWalletRepository(),
		ledger:     ledger,
		writer:     fake.NewInMemoryTxWriter(payments),
		settlement: fake.NewInMemorySettlementLedger(payments, ledger),
	}
}

func (s *stores) service(t testing.TB, orders upstream.OrderClient, opts ...payment.Option) *payment.Service {
	t.Helper()
	if orders == nil {
		orders = &mockOrderClient{orders: map[string]*orderv1.Order{}}
	}
	all := append([]payment.Option{payment.WithProvider(fixtureProvider())}, opts...)
	svc, err := payment.NewService(s.payRepo, s.wallets, orders, discardLogger(), s.writer, s.ledger, s.settlement, all...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type writerFunc func(ctx context.Context, ev auditx.Event) error

func (f writerFunc) WriteAuditEvent(ctx context.Context, ev auditx.Event) error { return f(ctx, ev) }

var permissiveAudit auditx.Writer = writerFunc(func(context.Context, auditx.Event) error { return nil })

func newHandlerLogged(t testing.TB, svc *payment.Service, w auditx.Writer, logger *slog.Logger) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(svc, auditx.NewGate(w, logger), logger)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func newHandler(t testing.TB, svc *payment.Service, w auditx.Writer) *rpc.Handler {
	t.Helper()
	return newHandlerLogged(t, svc, w, discardLogger())
}

func startServer(t testing.TB, h *rpc.Handler) paymentv1.PaymentServiceClient {
	t.Helper()
	srv := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServerInterceptor()))
	rpc.Register(srv, h)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return paymentv1.NewPaymentServiceClient(conn)
}
