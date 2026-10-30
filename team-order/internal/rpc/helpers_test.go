package rpc_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func withPrincipal(ctx context.Context, p *commonv1.Principal) context.Context {
	typ := auth.TypeAnonymous
	switch p.GetType() {
	case commonv1.PrincipalType_PRINCIPAL_TYPE_USER:
		typ = auth.TypeUser
	case commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE:
		typ = auth.TypeService
	}
	return auth.WithPrincipal(ctx, auth.Principal{ID: p.GetId(), Type: typ, Scopes: p.GetScopes()})
}

type idlePromotion struct{}

func (idlePromotion) ValidateAndReserve(context.Context, *promotionv1.ValidateAndReserveRequest, ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle promotion fake")
}

func (idlePromotion) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle promotion fake")
}

func (idlePromotion) ReleaseReservation(context.Context, *promotionv1.ReleaseReservationRequest, ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle promotion fake")
}

type idleFlash struct{}

func (idleFlash) GetActiveFlashSale(context.Context, *promotionv1.GetActiveFlashSaleRequest, ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

func (idleFlash) ReserveFlashSaleStock(context.Context, *promotionv1.ReserveFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle flash fake")
}

func (idleFlash) CommitFlashSaleStock(context.Context, *promotionv1.CommitFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle flash fake")
}

func (idleFlash) ReleaseFlashSaleStock(context.Context, *promotionv1.ReleaseFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	return &promotionv1.ReleaseFlashSaleStockResponse{Released: true}, nil
}

type idleAddress struct {
	identityv1.AddressServiceClient
}

func newSvc(
	t testing.TB,
	orders store.OrderRepository,
	carts store.CartRepository,
	returns store.ReturnRepository,
	ships store.ShipmentRepository,
	domain upstream.DomainClient,
	addrClient identityv1.AddressServiceClient,
	logger *slog.Logger,
	opts ...order.OrderServiceOption,
) *order.OrderService {
	t.Helper()
	if returns == nil {
		returns = fake.NewInMemoryReturnRepository()
	}
	if ships == nil {
		ships = fake.NewInMemoryShipmentRepository()
	}
	if domain == nil {
		domain = upstreamtest.NewDomain(map[string]int32{})
	}
	if addrClient == nil {
		addrClient = idleAddress{}
	}
	all := append([]order.OrderServiceOption{
		order.WithSagaRepository(fake.NewInMemorySagaRepository()),
		order.WithPromotionClient(idlePromotion{}),
		order.WithOrderFlashSaleClient(idleFlash{}),
	}, opts...)
	svc, err := order.NewOrderService(orders, carts, returns, ships, domain, addrClient, logger, all...)
	if err != nil {
		t.Fatalf("NewOrderService: %v", err)
	}
	return svc
}

func newCart(
	t testing.TB,
	repo store.CartRepository,
	orders store.OrderRepository,
	domain upstream.DomainClient,
	logger *slog.Logger,
	opts ...order.CartOption,
) *order.CartService {
	t.Helper()
	all := append([]order.CartOption{order.WithFlashSaleClient(idleFlash{})}, opts...)
	svc, err := order.NewCartService(repo, orders, domain, logger, all...)
	if err != nil {
		t.Fatalf("NewCartService: %v", err)
	}
	return svc
}

type flagsEvaluator = flags.Evaluator

type fakeFlags struct{ enabled bool }

func (f fakeFlags) BooleanEnabled(_ context.Context, _ string, _ bool, _ ...flags.EvalOption) bool {
	return f.enabled
}

type hcfg struct {
	audit auditx.Writer
	flags flags.Evaluator
}

type hopt func(*hcfg)

func withAudit(w auditx.Writer) hopt { return func(c *hcfg) { c.audit = w } }

func withFlags(e flags.Evaluator) hopt { return func(c *hcfg) { c.flags = e } }

func newHandler(t testing.TB, svc *order.OrderService, addr identityv1.AddressServiceClient, logger *slog.Logger, opts ...hopt) *rpc.OrderHandler {
	t.Helper()
	cfg := hcfg{audit: &fakeAudit{}, flags: fakeFlags{enabled: true}}
	for _, o := range opts {
		o(&cfg)
	}
	if addr == nil {
		addr = idleAddress{}
	}
	if logger == nil {
		logger = discardLogger()
	}
	h, err := rpc.NewOrderHandler(svc, addr, cfg.flags, auditx.NewGate(cfg.audit, logger), logger)
	if err != nil {
		t.Fatalf("NewOrderHandler: %v", err)
	}
	return h
}
