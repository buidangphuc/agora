package order_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

// recordingSagaRepo records the ids of the sagas it creates so a test can inspect
// one checkout attempt's reservations.
type recordingSagaRepo struct {
	store.SagaRepository
	mu  sync.Mutex
	ids []string
}

func (r *recordingSagaRepo) CreateSaga(ctx context.Context, s store.Saga) (store.Saga, bool, error) {
	got, created, err := r.SagaRepository.CreateSaga(ctx, s)
	if err == nil && created {
		r.mu.Lock()
		r.ids = append(r.ids, got.ID)
		r.mu.Unlock()
	}
	return got, created, err
}

func (r *recordingSagaRepo) sagaIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func (r *recordingSagaRepo) lastSagaID() string {
	ids := r.sagaIDs()
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}

func reserveReq(res store.Reservation) *listingv1.ReserveStockRequest {
	return &listingv1.ReserveStockRequest{ListingId: res.ListingID, VariantId: res.VariantID, Quantity: res.Quantity, ReservationId: res.ID}
}

// cancelWithoutRelease moves an order to Cancelled without releasing anything:
// a cancel that crashed right after its claim.
func cancelWithoutRelease(t *testing.T, orders store.OrderRepository, id string) {
	t.Helper()
	if _, err := orders.UpdateOrderStatusFrom(context.Background(), id, store.OrderStatusCancelled, []store.OrderStatus{store.OrderStatusPending, store.OrderStatusPaid}, ""); err != nil {
		t.Fatal(err)
	}
}

func releaseReq(id string) *listingv1.ReleaseStockRequest {
	return &listingv1.ReleaseStockRequest{ReservationId: id}
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
