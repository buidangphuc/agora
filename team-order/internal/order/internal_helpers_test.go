package order

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/store/fake"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type internalIdlePromotion struct{}

func (internalIdlePromotion) ValidateAndReserve(context.Context, *promotionv1.ValidateAndReserveRequest, ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle")
}

func (internalIdlePromotion) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle")
}

func (internalIdlePromotion) ReleaseReservation(context.Context, *promotionv1.ReleaseReservationRequest, ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle")
}

type internalIdleFlash struct{}

func (internalIdleFlash) GetActiveFlashSale(context.Context, *promotionv1.GetActiveFlashSaleRequest, ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error) {
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

func (internalIdleFlash) ReserveFlashSaleStock(context.Context, *promotionv1.ReserveFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle")
}

func (internalIdleFlash) CommitFlashSaleStock(context.Context, *promotionv1.CommitFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	return nil, status.Error(codes.Unimplemented, "idle")
}

func (internalIdleFlash) ReleaseFlashSaleStock(context.Context, *promotionv1.ReleaseFlashSaleStockRequest, ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	return &promotionv1.ReleaseFlashSaleStockResponse{Released: true}, nil
}

type internalIdleAddress struct {
	identityv1.AddressServiceClient
}

func internalSvc(t testing.TB, opts ...OrderServiceOption) *OrderService {
	t.Helper()
	all := append([]OrderServiceOption{
		WithSagaRepository(fake.NewInMemorySagaRepository()),
		WithPromotionClient(internalIdlePromotion{}),
		WithOrderFlashSaleClient(internalIdleFlash{}),
	}, opts...)
	svc, err := NewOrderService(nil, nil, fake.NewInMemoryReturnRepository(), fake.NewInMemoryShipmentRepository(),
		upstreamtest.NewDomain(map[string]int32{}), internalIdleAddress{}, nil, all...)
	if err != nil {
		t.Fatalf("NewOrderService: %v", err)
	}
	return svc
}
