package handler_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/handler"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/service"
)

// startServer stands up an in-process gRPC server (bufconn) wired exactly like
// production: the auth interceptor resolves the principal from metadata, then the
// real handlers over in-memory repositories.
func startServer(t *testing.T) (promotionv1.VoucherServiceClient, promotionv1.FlashSaleServiceClient) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)

	voucherSvc := service.NewVoucherService(
		repository.NewInMemoryVoucherRepository(),
		repository.NewInMemoryReservationRepository(),
		nil, nil, nil,
	)
	flashSvc := service.NewFlashSaleService(repository.NewInMemoryFlashSaleRepository(), nil, nil, nil)

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.AuthUnaryInterceptor()))
	promotionv1.RegisterVoucherServiceServer(srv, handler.NewVoucherHandler(voucherSvc, nil))
	promotionv1.RegisterFlashSaleServiceServer(srv, handler.NewFlashSaleHandler(flashSvc, nil))

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return promotionv1.NewVoucherServiceClient(conn), promotionv1.NewFlashSaleServiceClient(conn)
}

// authCtx attaches a resolved principal via the metadata the auth interceptor reads.
func authCtx(id string) context.Context {
	md := metadata.New(map[string]string{
		"x-principal-id":   id,
		"x-principal-type": "user",
	})
	return metadata.NewOutgoingContext(context.Background(), md)
}

func TestVoucherServiceEndToEnd(t *testing.T) {
	vc, _ := startServer(t)
	ctx := authCtx("seller-1")

	// CreateVoucher requires a principal.
	if _, err := vc.CreateVoucher(context.Background(), &promotionv1.CreateVoucherRequest{Code: "X"}); err == nil {
		t.Fatal("expected Unauthenticated without a principal")
	}

	created, err := vc.CreateVoucher(ctx, &promotionv1.CreateVoucherRequest{
		Code:          "GRPC20",
		DiscountType:  promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
		DiscountValue: 20,
		Quota:         10,
	})
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	if created.GetVoucher().GetId() == "" {
		t.Fatal("expected voucher id")
	}

	// Idempotent ValidateAndReserve over the wire.
	req := &promotionv1.ValidateAndReserveRequest{
		ReservationId: "wire-resv", Code: "GRPC20", BuyerId: "b1", CartSubtotal: 100000,
	}
	r1, err := vc.ValidateAndReserve(ctx, req)
	if err != nil {
		t.Fatalf("reserve 1: %v", err)
	}
	if !r1.GetValid() || r1.GetDiscountAmount() != 20000 {
		t.Fatalf("reserve 1 unexpected: %+v", r1)
	}
	r2, err := vc.ValidateAndReserve(ctx, req)
	if err != nil {
		t.Fatalf("reserve 2: %v", err)
	}
	if r2.GetDiscountAmount() != r1.GetDiscountAmount() {
		t.Fatalf("idempotency broken over gRPC: %d vs %d", r2.GetDiscountAmount(), r1.GetDiscountAmount())
	}

	// Reject unknown code with a reason, no error.
	rej, err := vc.ValidateAndReserve(ctx, &promotionv1.ValidateAndReserveRequest{
		ReservationId: "bad", Code: "NOPE", CartSubtotal: 1000,
	})
	if err != nil {
		t.Fatalf("reserve reject: %v", err)
	}
	if rej.GetValid() || rej.GetReason() == "" {
		t.Fatalf("expected rejection with reason, got %+v", rej)
	}
}

func TestFlashSaleServiceEndToEnd(t *testing.T) {
	_, fc := startServer(t)
	ctx := authCtx("admin-1")

	created, err := fc.CreateCampaign(ctx, &promotionv1.CreateCampaignRequest{
		ListingId: "listing-1",
		SalePrice: 90000,
		StockCap:  100,
	})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	campaignID := created.GetCampaign().GetId()

	stock, err := fc.GetFlashSaleStock(ctx, &promotionv1.GetFlashSaleStockRequest{CampaignId: campaignID})
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	if stock.GetRemaining() != 100 || stock.GetStockCap() != 100 {
		t.Fatalf("stock = %d/%d, want 100/100", stock.GetRemaining(), stock.GetStockCap())
	}
}
