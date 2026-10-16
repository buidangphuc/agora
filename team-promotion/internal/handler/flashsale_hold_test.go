package handler_test

// Handler tests for the flash-stock hold RPCs (openspec change
// promotion-flash-stock-hold). They rely on service.(*FlashSaleService).WithHolds
// and repository.NewInMemoryFlashHoldRepository, see the service and repository
// hold tests for the assumed interface.

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

type holdFixture struct {
	fc        promotionv1.FlashSaleServiceClient
	campaigns *repository.InMemoryFlashSaleRepository
}

// startHoldServer wires the real handler and service over in-memory repositories;
// holds may be replaced (e.g. by a failing repository).
func startHoldServer(t *testing.T, wrap func(*repository.InMemoryFlashHoldRepository) repository.FlashHoldRepository) holdFixture {
	t.Helper()
	campaigns := repository.NewInMemoryFlashSaleRepository()
	mem := repository.NewInMemoryFlashHoldRepository(campaigns)
	var holds repository.FlashHoldRepository = mem
	if wrap != nil {
		holds = wrap(mem)
	}
	svc := service.NewFlashSaleService(campaigns, nil, nil, nil, nil).WithHolds(holds)

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.AuthUnaryInterceptor()))
	promotionv1.RegisterFlashSaleServiceServer(srv, handler.NewFlashSaleHandler(svc, nil))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return holdFixture{fc: promotionv1.NewFlashSaleServiceClient(conn), campaigns: campaigns}
}

// seed creates a campaign active around now (the handler uses the wall clock).
func (f holdFixture) seed(t *testing.T, variant string, stockCap int64) repository.FlashSaleCampaign {
	t.Helper()
	c, err := f.campaigns.Create(context.Background(), repository.FlashSaleCampaign{
		ListingID: "l1", VariantID: variant, SalePrice: 90000, StockCap: stockCap,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return c
}

func (f holdFixture) remaining(t *testing.T, c repository.FlashSaleCampaign) int64 {
	t.Helper()
	r, err := f.fc.GetFlashSaleStock(authCtx("buyer-1", "listing.read"), &promotionv1.GetFlashSaleStockRequest{CampaignId: c.ID})
	if err != nil {
		t.Fatalf("GetFlashSaleStock: %v", err)
	}
	return r.GetRemaining()
}

func reserveReq(id string, c repository.FlashSaleCampaign, qty int64) *promotionv1.ReserveFlashSaleStockRequest {
	return &promotionv1.ReserveFlashSaleStockRequest{
		ReservationId: id, CampaignId: c.ID, ListingId: c.ListingID, VariantId: c.VariantID,
		Quantity: qty, ExpectedSalePrice: c.SalePrice,
	}
}

var orderSvc = func() context.Context { return principalCtx("service-team-order", "service", "promotion.reserve") }

// Scenarios: A buyer cannot reserve flash stock; A user carrying the service scope is still
// refused; A service without the scope is refused; An anonymous caller is unauthenticated.
func TestFlashHoldRPCsAreServiceOnly(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	if r, err := f.fc.ReserveFlashSaleStock(orderSvc(), reserveReq("o1", c, 2)); err != nil || !r.GetReserved() {
		t.Fatalf("service reserve: %v %+v", err, r)
	}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), codes.Unauthenticated},
		{"no principal", context.Background(), codes.Unauthenticated},
		{"buyer", authCtx("buyer-1"), codes.PermissionDenied},
		{"user with promotion.reserve and admin", authCtx("buyer-1", "promotion.reserve", "admin"), codes.PermissionDenied},
		{"service without scope", principalCtx("service-other", "service", "listing.read"), codes.PermissionDenied},
	} {
		if _, err := f.fc.ReserveFlashSaleStock(tc.ctx, reserveReq("x-"+tc.name, c, 1)); code(err) != tc.want {
			t.Errorf("reserve %s: got %v, want %v", tc.name, code(err), tc.want)
		}
		if _, err := f.fc.CommitFlashSaleStock(tc.ctx, &promotionv1.CommitFlashSaleStockRequest{ReservationId: "o1"}); code(err) != tc.want {
			t.Errorf("commit %s: got %v, want %v", tc.name, code(err), tc.want)
		}
		if _, err := f.fc.ReleaseFlashSaleStock(tc.ctx, &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: "o1"}); code(err) != tc.want {
			t.Errorf("release %s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
	// Refused calls changed nothing: the hold of 2 is still there and still commits.
	if got := f.remaining(t, c); got != 3 {
		t.Fatalf("remaining = %d, want 3", got)
	}
	if r, err := f.fc.CommitFlashSaleStock(orderSvc(), &promotionv1.CommitFlashSaleStockRequest{ReservationId: "o1"}); err != nil || !r.GetCommitted() {
		t.Fatalf("service commit: %v %+v", err, r)
	}
	if r, err := f.fc.ReleaseFlashSaleStock(orderSvc(), &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: "o1"}); err != nil || !r.GetReleased() {
		t.Fatalf("service release: %v %+v", err, r)
	}
}

// Scenarios: Reserve rejects malformed input; Commit rejects an empty reservation id;
// Release rejects an empty reservation id.
func TestFlashHoldRPCsRejectInvalidInput(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	mut := func(fn func(*promotionv1.ReserveFlashSaleStockRequest)) *promotionv1.ReserveFlashSaleStockRequest {
		r := reserveReq("r1", c, 1)
		fn(r)
		return r
	}
	for name, req := range map[string]*promotionv1.ReserveFlashSaleStockRequest{
		"empty reservation id": mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.ReservationId = "" }),
		"empty campaign id":    mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.CampaignId = "" }),
		"empty listing id":     mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.ListingId = "" }),
		"quantity 0":           mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.Quantity = 0 }),
		"quantity -1":          mut(func(r *promotionv1.ReserveFlashSaleStockRequest) { r.Quantity = -1 }),
	} {
		resp, err := f.fc.ReserveFlashSaleStock(orderSvc(), req)
		if code(err) != codes.InvalidArgument {
			t.Errorf("reserve %s: got %v, want InvalidArgument", name, code(err))
		}
		if resp.GetReason() != "" {
			t.Errorf("reserve %s: reason %q, want none", name, resp.GetReason())
		}
	}
	if got := f.remaining(t, c); got != 5 {
		t.Fatalf("remaining = %d, want 5 (no hold created)", got)
	}
	for name, id := range map[string]string{"empty": "", "201 bytes": strings.Repeat("a", 201)} {
		if _, err := f.fc.CommitFlashSaleStock(orderSvc(), &promotionv1.CommitFlashSaleStockRequest{ReservationId: id}); code(err) != codes.InvalidArgument {
			t.Errorf("commit %s: got %v, want InvalidArgument", name, code(err))
		}
		if _, err := f.fc.ReleaseFlashSaleStock(orderSvc(), &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: id}); code(err) != codes.InvalidArgument {
			t.Errorf("release %s: got %v, want InvalidArgument", name, code(err))
		}
	}
}

// Scenario: A reservation id of 200 bytes is accepted and 201 is rejected.
func TestFlashHoldReservationIDLengthBoundary(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	r, err := f.fc.ReserveFlashSaleStock(orderSvc(), reserveReq(strings.Repeat("a", 200), c, 1))
	if err != nil || !r.GetReserved() || r.GetReason() != "" {
		t.Fatalf("200 bytes: %v %+v, want reserved with empty reason", err, r)
	}
	if _, err := f.fc.ReserveFlashSaleStock(orderSvc(), reserveReq(strings.Repeat("a", 201), c, 1)); code(err) != codes.InvalidArgument {
		t.Fatalf("201 bytes: got %v, want InvalidArgument", code(err))
	}
}

// Scenario: An empty variant id is valid.
func TestFlashHoldEmptyVariantAccepted(t *testing.T) {
	f := startHoldServer(t, nil)
	c := f.seed(t, "", 5)
	req := reserveReq("r1", c, 1)
	req.VariantId = ""
	if r, err := f.fc.ReserveFlashSaleStock(orderSvc(), req); err != nil || !r.GetReserved() {
		t.Fatalf("%v %+v, want reserved=true", err, r)
	}
}

// Scenario: Stock of an unknown campaign is not found.
func TestGetFlashSaleStockUnknownNotFound(t *testing.T) {
	f := startHoldServer(t, nil)
	_, err := f.fc.GetFlashSaleStock(authCtx("buyer-1", "listing.read"), &promotionv1.GetFlashSaleStockRequest{CampaignId: "nope"})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", code(err))
	}
}

// failingHolds makes every hold-store call fail with a revealing error.
type failingHolds struct {
	*repository.InMemoryFlashHoldRepository
}

var errHoldStore = errors.New("pq: connection refused to 10.0.0.5:5432")

func (failingHolds) Reserve(context.Context, repository.FlashHold, time.Time) (repository.FlashHold, bool, error) {
	return repository.FlashHold{}, false, errHoldStore
}
func (failingHolds) Get(context.Context, string) (repository.FlashHold, error) {
	return repository.FlashHold{}, errHoldStore
}
func (failingHolds) Commit(context.Context, string, time.Time) (bool, error) {
	return false, errHoldStore
}
func (failingHolds) Release(context.Context, string, time.Time) (bool, error) {
	return false, errHoldStore
}
func (failingHolds) ActiveHeld(context.Context, string, time.Time) (int64, error) {
	return 0, errHoldStore
}

// Scenario: A store failure is a generic internal error.
func TestFlashHoldStoreFailureIsInternal(t *testing.T) {
	f := startHoldServer(t, func(m *repository.InMemoryFlashHoldRepository) repository.FlashHoldRepository {
		return failingHolds{m}
	})
	c := f.seed(t, "", 5)
	check := func(op string, err error) {
		t.Helper()
		st, _ := status.FromError(err)
		if st.Code() != codes.Internal || st.Message() != "internal error" || strings.Contains(st.Message(), "pq") {
			t.Errorf("%s: code=%v message=%q, want Internal / %q", op, st.Code(), st.Message(), "internal error")
		}
	}
	_, err := f.fc.ReserveFlashSaleStock(orderSvc(), reserveReq("r1", c, 1))
	check("reserve", err)
	_, err = f.fc.CommitFlashSaleStock(orderSvc(), &promotionv1.CommitFlashSaleStockRequest{ReservationId: "r1"})
	check("commit", err)
	_, err = f.fc.ReleaseFlashSaleStock(orderSvc(), &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: "r1"})
	check("release", err)
}
