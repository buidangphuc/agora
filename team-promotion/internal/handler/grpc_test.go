package handler_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
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
	v, f, _, _ := startAll(t, ownedListings{"l1": "seller-1", "l2": "seller-2"})
	return v, f
}

func startAll(t *testing.T, listings upstreamListings) (promotionv1.VoucherServiceClient, promotionv1.FlashSaleServiceClient, promotionv1.SubscriptionServiceClient, promotionv1.SponsoredServiceClient) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)

	voucherSvc := service.NewVoucherService(
		repository.NewInMemoryVoucherRepository(),
		repository.NewInMemoryReservationRepository(),
		nil, nil, nil,
	)
	flashSvc := service.NewFlashSaleService(repository.NewInMemoryFlashSaleRepository(), nil, nil, listings, nil)

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.AuthUnaryInterceptor()))
	promotionv1.RegisterVoucherServiceServer(srv, handler.NewVoucherHandler(voucherSvc, nil))
	promotionv1.RegisterFlashSaleServiceServer(srv, handler.NewFlashSaleHandler(flashSvc, nil))
	promotionv1.RegisterSubscriptionServiceServer(srv, handler.NewSubscriptionHandler(
		service.NewSubscriptionService(repository.NewInMemorySubscriptionRepository(), nil), nil))
	promotionv1.RegisterSponsoredServiceServer(srv, handler.NewSponsoredHandler(
		service.NewSponsoredService(repository.NewInMemoryAdCampaignRepository(), listings, nil).WithLimits(1000, 5000), nil))

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
	return promotionv1.NewVoucherServiceClient(conn), promotionv1.NewFlashSaleServiceClient(conn),
		promotionv1.NewSubscriptionServiceClient(conn), promotionv1.NewSponsoredServiceClient(conn)
}

// upstreamListings is the listing lookup the services depend on.
type upstreamListings interface {
	GetListing(ctx context.Context, req *listingv1.GetListingRequest, opts ...grpc.CallOption) (*listingv1.GetListingResponse, error)
}

// ownedListings is a fake team-domain listing client: listing id -> seller id.
type ownedListings map[string]string

func (o ownedListings) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	seller, ok := o[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: seller}}, nil
}

// authCtx attaches a resolved principal via the metadata the auth interceptor reads.
func authCtx(id string, scopes ...string) context.Context {
	return principalCtx(id, "user", scopes...)
}

func principalCtx(id, typ string, scopes ...string) context.Context {
	md := metadata.New(map[string]string{
		"x-principal-id":     id,
		"x-principal-type":   typ,
		"x-principal-scopes": strings.Join(scopes, ","),
	})
	return metadata.NewOutgoingContext(context.Background(), md)
}

func TestVoucherServiceEndToEnd(t *testing.T) {
	vc, _ := startServer(t)
	ctx := authCtx("seller-1", "listing.read", "listing.write")

	// CreateVoucher requires a principal.
	if _, err := vc.CreateVoucher(context.Background(), &promotionv1.CreateVoucherRequest{Code: "X"}); err == nil {
		t.Fatal("expected Unauthenticated without a principal")
	}

	created, err := vc.CreateVoucher(ctx, &promotionv1.CreateVoucherRequest{
		Code:          "GRPC20",
		Scope:         promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP,
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
		ReservationId: "preview:seller-1:wire-resv", Code: "GRPC20", BuyerId: "b1", CartSubtotal: 100000, SellerId: "seller-1",
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
		ReservationId: "preview:seller-1:bad", Code: "NOPE", CartSubtotal: 1000,
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
	ctx := authCtx("admin-1", "admin")

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

func code(err error) codes.Code { return status.Code(err) }

func TestCreateVoucherAuthz(t *testing.T) {
	vc, _ := startServer(t)
	seller := authCtx("seller-1", "listing.read", "listing.write")
	buyer := authCtx("buyer-1", "listing.read", "search:read")
	admin := authCtx("admin-1", "listing.write", "admin")
	anon := principalCtx("anonymous", "anonymous")

	mk := func(c string, scope promotionv1.VoucherScope) *promotionv1.CreateVoucherRequest {
		return &promotionv1.CreateVoucherRequest{
			Code: c, Scope: scope, Quota: 5, DiscountValue: 10,
			DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
		}
	}
	shop, platform := promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM

	cases := []struct {
		name string
		ctx  context.Context
		req  *promotionv1.CreateVoucherRequest
		want codes.Code
	}{
		{"anonymous shop", anon, mk("A1", shop), codes.Unauthenticated},
		{"buyer shop", buyer, mk("B1", shop), codes.PermissionDenied},
		{"buyer platform", buyer, mk("B2", platform), codes.PermissionDenied},
		{"seller platform", seller, mk("S1", platform), codes.PermissionDenied},
		{"seller unspecified scope", seller, mk("S2", promotionv1.VoucherScope_VOUCHER_SCOPE_UNSPECIFIED), codes.PermissionDenied},
		{"seller shop", seller, mk("S3", shop), codes.OK},
		{"admin platform", admin, mk("AD1", platform), codes.OK},
		{"admin shop", admin, mk("AD2", shop), codes.OK},
	}
	for _, tc := range cases {
		_, err := vc.CreateVoucher(tc.ctx, tc.req)
		if code(err) != tc.want {
			t.Errorf("%s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}

	// Ownership: a shop voucher is always bound to the caller's id.
	got, err := vc.GetVoucher(seller, &promotionv1.GetVoucherRequest{Code: "S3"})
	if err != nil || got.GetVoucher().GetSellerId() != "seller-1" {
		t.Fatalf("shop voucher owner = %q, err=%v; want seller-1", got.GetVoucher().GetSellerId(), err)
	}
}

func TestCheckoutPathAllowedForServicePrincipal(t *testing.T) {
	vc, _ := startServer(t)
	seller := authCtx("seller-1", "listing.write")
	if _, err := vc.CreateVoucher(seller, &promotionv1.CreateVoucherRequest{
		Code: "SVC10", Scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, Quota: 5, DiscountValue: 10,
		DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// team-order calls reserve/commit/release as a service principal with no admin/seller scope.
	svc := principalCtx("service-team-order", "service", "identity.read", "promotion.reserve")
	if _, err := vc.ValidateAndReserve(svc, &promotionv1.ValidateAndReserveRequest{
		ReservationId: "r1", Code: "SVC10", BuyerId: "b1", CartSubtotal: 1000, SellerId: "seller-1",
	}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := vc.CommitReservation(svc, &promotionv1.CommitReservationRequest{ReservationId: "r1"}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := vc.ReleaseReservation(svc, &promotionv1.ReleaseReservationRequest{ReservationId: "r1"}); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestCreateCampaignAuthz(t *testing.T) {
	_, fc := startServer(t)
	req := &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), codes.Unauthenticated},
		{"buyer", authCtx("buyer-1", "listing.read"), codes.PermissionDenied},
		{"seller", authCtx("seller-1", "listing.write"), codes.OK},
		{"seller foreign listing", authCtx("seller-1", "listing.write"), codes.PermissionDenied},
		{"admin", authCtx("admin-1", "admin"), codes.OK},
	} {
		r := req
		if tc.name == "seller foreign listing" {
			r = &promotionv1.CreateCampaignRequest{ListingId: "l2", SalePrice: 1000, StockCap: 5}
		}
		if _, err := fc.CreateCampaign(tc.ctx, r); code(err) != tc.want {
			t.Errorf("%s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}
}
