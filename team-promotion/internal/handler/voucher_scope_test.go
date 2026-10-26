package handler_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/handler"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/service"
)

type scopeFixture struct {
	vc       promotionv1.VoucherServiceClient
	vouchers repository.VoucherRepository
	holds    *repository.InMemoryReservationRepository
}

func startScopeServer(t *testing.T, wrap func(repository.VoucherRepository) repository.VoucherRepository) scopeFixture {
	t.Helper()
	var vouchers repository.VoucherRepository = repository.NewInMemoryVoucherRepository()
	if wrap != nil {
		vouchers = wrap(vouchers)
	}
	holds := repository.NewInMemoryReservationRepository()
	svc := service.NewVoucherService(vouchers, holds, nil, nil, nil)
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.AuthUnaryInterceptor()))
	promotionv1.RegisterVoucherServiceServer(srv, handler.NewVoucherHandler(svc, nil))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return scopeFixture{vc: promotionv1.NewVoucherServiceClient(conn), vouchers: vouchers, holds: holds}
}

func (f scopeFixture) seed(t *testing.T, code string, scope promotionv1.VoucherScope, seller string) {
	t.Helper()
	now := time.Now()
	if _, err := f.vouchers.Create(context.Background(), repository.Voucher{
		Code: code, Scope: int32(scope), SellerID: seller,
		DiscountType: int32(promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT), DiscountValue: 10,
		Quota: 5, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func (f scopeFixture) hasHold(id string) bool {
	_, err := f.holds.GetByReservationID(context.Background(), id)
	return err == nil
}

func orderSvcCtx() context.Context {
	return principalCtx("service-team-order", "service", "promotion.reserve")
}

// Scenario: The order service receives the scope
func TestValidateAndReserveHandlerEchoesScope(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "SHOPV", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, "seller-1")
	resp, err := f.vc.ValidateAndReserve(orderSvcCtx(), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "SHOPV", BuyerId: "b1", CartSubtotal: 1000, SellerId: "seller-1",
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if !resp.GetValid() {
		t.Fatalf("valid=false reason=%q", resp.GetReason())
	}
	if resp.GetScope() != promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP || resp.GetSellerId() != "seller-1" {
		t.Errorf("scope=%v seller=%q, want SHOP/seller-1", resp.GetScope(), resp.GetSellerId())
	}
}

// Scenario: The order service receives the scope (rejection path keeps scope)
func TestValidateAndReserveHandlerEchoesScopeOnRejection(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "SHOPV", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, "seller-1")
	resp, err := f.vc.ValidateAndReserve(orderSvcCtx(), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "SHOPV", BuyerId: "b1", CartSubtotal: 1000, SellerId: "seller-2",
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if resp.GetValid() || resp.GetScope() != promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP || resp.GetSellerId() != "seller-1" {
		t.Errorf("valid=%v scope=%v seller=%q, want false/SHOP/seller-1", resp.GetValid(), resp.GetScope(), resp.GetSellerId())
	}
}

// Scenario: A buyer preview receives the scope
func TestValidateAndReservePreviewEchoesScope(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	resp, err := f.vc.ValidateAndReserve(authCtx("buyer-1", "listing.read"), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "preview:buyer-1:abc", Code: "PLAT", BuyerId: "someone-else", CartSubtotal: 1000,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !resp.GetValid() || resp.GetScope() != promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM || resp.GetSellerId() != "" {
		t.Errorf("valid=%v scope=%v seller=%q, want true/PLATFORM/empty", resp.GetValid(), resp.GetScope(), resp.GetSellerId())
	}
	hold, err := f.holds.GetByReservationID(context.Background(), "preview:buyer-1:abc")
	if err != nil {
		t.Fatalf("hold missing: %v", err)
	}
	if hold.BuyerID != "buyer-1" {
		t.Errorf("hold buyer = %q, want principal buyer-1", hold.BuyerID)
	}
}

// Scenario: A buyer outside the preview namespace is denied
func TestValidateAndReservePreviewNamespaceDenied(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	for _, id := range []string{"preview:buyer-2:abc", "order-1"} {
		_, err := f.vc.ValidateAndReserve(authCtx("buyer-1", "listing.read"), &promotionv1.ValidateAndReserveRequest{
			ReservationId: id, Code: "PLAT", BuyerId: "buyer-1", CartSubtotal: 1000,
		})
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: got %v, want PermissionDenied", id, code(err))
		}
		if f.hasHold(id) {
			t.Errorf("%s: hold was created", id)
		}
	}
}

// Scenario: A service without the reserve scope is denied
func TestValidateAndReserveServiceWithoutScopeDenied(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	_, err := f.vc.ValidateAndReserve(principalCtx("service-x", "service", "listing.read"), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "PLAT", BuyerId: "b1", CartSubtotal: 1000,
	})
	if code(err) != codes.PermissionDenied {
		t.Errorf("got %v, want PermissionDenied", code(err))
	}
	if f.hasHold("o1") {
		t.Error("hold was created")
	}
}

// Scenario: An anonymous caller is unauthenticated
func TestValidateAndReserveAnonymousUnauthenticated(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	_, err := f.vc.ValidateAndReserve(principalCtx("anonymous", "anonymous"), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "PLAT", BuyerId: "b1", CartSubtotal: 1000,
	})
	if code(err) != codes.Unauthenticated {
		t.Errorf("got %v, want Unauthenticated", code(err))
	}
	if f.hasHold("o1") {
		t.Error("hold was created")
	}
}

// Scenario: An empty reservation id is rejected
func TestValidateAndReserveEmptyReservationIDInvalid(t *testing.T) {
	f := startScopeServer(t, nil)
	f.seed(t, "PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM, "")
	_, err := f.vc.ValidateAndReserve(orderSvcCtx(), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "", Code: "PLAT", BuyerId: "b1", CartSubtotal: 1000,
	})
	if code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", code(err))
	}
	if f.hasHold("") {
		t.Error("hold was created")
	}
}

type failingVouchers struct{ repository.VoucherRepository }

var errVoucherStore = errors.New("pq: connection refused to 10.0.0.5:5432")

func (failingVouchers) GetByCode(context.Context, string) (repository.Voucher, error) {
	return repository.Voucher{}, errVoucherStore
}
func (failingVouchers) GetByID(context.Context, string) (repository.Voucher, error) {
	return repository.Voucher{}, errVoucherStore
}

// Scenario: A store failure is a generic internal error
func TestValidateAndReserveStoreFailureInternal(t *testing.T) {
	f := startScopeServer(t, func(r repository.VoucherRepository) repository.VoucherRepository {
		return failingVouchers{r}
	})
	_, err := f.vc.ValidateAndReserve(orderSvcCtx(), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "ANY", BuyerId: "b1", CartSubtotal: 1000,
	})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" || strings.Contains(st.Message(), "pq") {
		t.Errorf("code=%v message=%q, want Internal / %q", st.Code(), st.Message(), "internal error")
	}
}
