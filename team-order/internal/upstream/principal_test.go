package upstream

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	sdkauth "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

type seenMD struct {
	mu   sync.Mutex
	seen map[string]metadata.MD
}

func (s *seenMD) rec(ctx context.Context, rpc string) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	s.seen[rpc] = md
	s.mu.Unlock()
}

func (s *seenMD) get(rpc string) metadata.MD {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[rpc]
}

type fakeListing struct {
	listingv1.UnimplementedListingServiceServer
	*seenMD
}

func (f fakeListing) GetListing(ctx context.Context, _ *listingv1.GetListingRequest) (*listingv1.GetListingResponse, error) {
	f.rec(ctx, "GetListing")
	return &listingv1.GetListingResponse{}, nil
}

func (f fakeListing) ReserveStock(ctx context.Context, _ *listingv1.ReserveStockRequest) (*listingv1.ReserveStockResponse, error) {
	f.rec(ctx, "ReserveStock")
	return &listingv1.ReserveStockResponse{}, nil
}

func (f fakeListing) ReleaseStock(ctx context.Context, _ *listingv1.ReleaseStockRequest) (*listingv1.ReleaseStockResponse, error) {
	f.rec(ctx, "ReleaseStock")
	return &listingv1.ReleaseStockResponse{}, nil
}

func (f fakeListing) CommitReservation(ctx context.Context, _ *listingv1.CommitReservationRequest) (*listingv1.CommitReservationResponse, error) {
	f.rec(ctx, "CommitReservation")
	return &listingv1.CommitReservationResponse{}, nil
}

type fakeVoucher struct {
	promotionv1.UnimplementedVoucherServiceServer
	*seenMD
}

func (f fakeVoucher) ValidateAndReserve(ctx context.Context, _ *promotionv1.ValidateAndReserveRequest) (*promotionv1.ValidateAndReserveResponse, error) {
	f.rec(ctx, "ValidateAndReserve")
	return &promotionv1.ValidateAndReserveResponse{}, nil
}

func (f fakeVoucher) CommitReservation(ctx context.Context, _ *promotionv1.CommitReservationRequest) (*promotionv1.CommitReservationResponse, error) {
	f.rec(ctx, "VoucherCommit")
	return &promotionv1.CommitReservationResponse{}, nil
}

func (f fakeVoucher) ReleaseReservation(ctx context.Context, _ *promotionv1.ReleaseReservationRequest) (*promotionv1.ReleaseReservationResponse, error) {
	f.rec(ctx, "VoucherRelease")
	return &promotionv1.ReleaseReservationResponse{}, nil
}

type fakeFlash struct {
	promotionv1.UnimplementedFlashSaleServiceServer
	*seenMD
}

func (f fakeFlash) ReserveFlashSaleStock(ctx context.Context, _ *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	f.rec(ctx, "ReserveFlashSaleStock")
	return &promotionv1.ReserveFlashSaleStockResponse{}, nil
}

func (f fakeFlash) CommitFlashSaleStock(ctx context.Context, _ *promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error) {
	f.rec(ctx, "CommitFlashSaleStock")
	return &promotionv1.CommitFlashSaleStockResponse{}, nil
}

func (f fakeFlash) ReleaseFlashSaleStock(ctx context.Context, _ *promotionv1.ReleaseFlashSaleStockRequest) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	f.rec(ctx, "ReleaseFlashSaleStock")
	return &promotionv1.ReleaseFlashSaleStockResponse{}, nil
}

func (f fakeFlash) GetActiveFlashSale(ctx context.Context, _ *promotionv1.GetActiveFlashSaleRequest) (*promotionv1.GetActiveFlashSaleResponse, error) {
	f.rec(ctx, "GetActiveFlashSale")
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

type fakeAddress struct {
	identityv1.UnimplementedAddressServiceServer
	*seenMD
}

func (f fakeAddress) ListAddresses(ctx context.Context, _ *identityv1.ListAddressesRequest) (*identityv1.ListAddressesResponse, error) {
	f.rec(ctx, "ListAddresses")
	return &identityv1.ListAddressesResponse{}, nil
}

type wired struct {
	seen    *seenMD
	stock   DomainClient
	voucher promotionv1.VoucherServiceClient
	flash   promotionv1.FlashSaleServiceClient
	address identityv1.AddressServiceClient
}

// wire serves the four upstream services on one listener and dials it with the
// plain SDK client (no principal option), wrapped exactly as production wraps it.
func wire(t *testing.T) *wired {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &seenMD{seen: map[string]metadata.MD{}}
	srv := grpc.NewServer()
	listingv1.RegisterListingServiceServer(srv, fakeListing{seenMD: s})
	promotionv1.RegisterVoucherServiceServer(srv, fakeVoucher{seenMD: s})
	promotionv1.RegisterFlashSaleServiceServer(srv, fakeFlash{seenMD: s})
	identityv1.RegisterAddressServiceServer(srv, fakeAddress{seenMD: s})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := sdkupstream.NewClient(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &wired{
		seen:    s,
		stock:   NewServiceStockClient(listingv1.NewListingServiceClient(conn)),
		voucher: NewServicePromotionClient(promotionv1.NewVoucherServiceClient(conn)),
		flash:   NewServiceFlashClient(promotionv1.NewFlashSaleServiceClient(conn)),
		address: identityv1.NewAddressServiceClient(conn),
	}
}

func buyerCtx() context.Context {
	md := metadata.Pairs(
		"x-principal-id", "u-7",
		"x-principal-type", "user",
		"x-principal-scopes", "listing.read",
		"authorization", "Bearer buyer-token",
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	return sdkauth.WithPrincipal(ctx, sdkauth.Principal{ID: "u-7", Type: sdkauth.TypeUser, Scopes: []string{"listing.read"}})
}

func one(md metadata.MD, k string) string {
	if v := md.Get(k); len(v) == 1 {
		return v[0]
	}
	return ""
}

func assertPrincipal(t *testing.T, rpc string, md metadata.MD, id, typ, scopes string) {
	t.Helper()
	if md == nil {
		t.Fatalf("%s: upstream never received the call", rpc)
	}
	if got := md.Get("x-principal-id"); len(got) != 1 || got[0] != id {
		t.Errorf("%s: principal id = %v, want [%s]", rpc, got, id)
	}
	if got := md.Get("x-principal-type"); len(got) != 1 || got[0] != typ {
		t.Errorf("%s: principal type = %v, want [%s]", rpc, got, typ)
	}
	if got := md.Get("x-principal-scopes"); len(got) != 1 || got[0] != scopes {
		t.Errorf("%s: scopes = %v, want exactly [%s]", rpc, got, scopes)
	}
}

// Scenario: Stock changes are sent as the service principal with the stock scope
func TestStockCallsCarryServicePrincipal(t *testing.T) {
	w := wire(t)
	ctx := buyerCtx()
	if _, err := w.stock.ReserveStock(ctx, &listingv1.ReserveStockRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.stock.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.stock.CommitReservation(ctx, &listingv1.CommitReservationRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []string{"ReserveStock", "ReleaseStock", "CommitReservation"} {
		md := w.seen.get(rpc)
		assertPrincipal(t, rpc, md, "service-team-order", "service", "listing.write")
		if v := md.Get("authorization"); len(v) != 0 {
			t.Errorf("%s: authorization forwarded: %v", rpc, v)
		}
	}
}

// Scenario: Voucher and flash holds are sent with the promotion scope
func TestPromotionCallsCarryPromotionScope(t *testing.T) {
	w := wire(t)
	ctx := buyerCtx()
	if _, err := w.voucher.ValidateAndReserve(ctx, &promotionv1.ValidateAndReserveRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.voucher.CommitReservation(ctx, &promotionv1.CommitReservationRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.voucher.ReleaseReservation(ctx, &promotionv1.ReleaseReservationRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.flash.ReserveFlashSaleStock(ctx, &promotionv1.ReserveFlashSaleStockRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.flash.CommitFlashSaleStock(ctx, &promotionv1.CommitFlashSaleStockRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.flash.ReleaseFlashSaleStock(ctx, &promotionv1.ReleaseFlashSaleStockRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []string{
		"ValidateAndReserve", "VoucherCommit", "VoucherRelease",
		"ReserveFlashSaleStock", "CommitFlashSaleStock", "ReleaseFlashSaleStock",
	} {
		assertPrincipal(t, rpc, w.seen.get(rpc), "service-team-order", "service", "promotion.reserve")
	}
}

// Scenario: A buyer read forwards the buyer principal unchanged
func TestBuyerReadsForwardBuyerPrincipal(t *testing.T) {
	w := wire(t)
	ctx := buyerCtx()
	if _, err := w.stock.GetListing(ctx, &listingv1.GetListingRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.flash.GetActiveFlashSale(ctx, &promotionv1.GetActiveFlashSaleRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []string{"GetListing", "GetActiveFlashSale"} {
		assertPrincipal(t, rpc, w.seen.get(rpc), "u-7", "user", "listing.read")
	}
}

// Scenario: A service-principal call does not forward the buyer token
func TestServiceCallDoesNotForwardBuyerToken(t *testing.T) {
	w := wire(t)
	ctx := buyerCtx()
	if _, err := w.stock.CommitReservation(ctx, &listingv1.CommitReservationRequest{}); err != nil {
		t.Fatal(err)
	}
	md := w.seen.get("CommitReservation")
	if md == nil {
		t.Fatal("upstream never received the call")
	}
	if v := md.Get("authorization"); len(v) != 0 {
		t.Fatalf("authorization forwarded: %v", v)
	}
	if got := one(md, "x-principal-id"); got == "u-7" || got != "service-team-order" {
		t.Fatalf("principal id = %q, want service-team-order and never u-7", got)
	}
}

// Scenario: A background read gets the read-only service scope
func TestBackgroundReadGetsReadOnlyScope(t *testing.T) {
	w := wire(t)
	if _, err := w.stock.GetListing(context.Background(), &listingv1.GetListingRequest{}); err != nil {
		t.Fatal(err)
	}
	assertPrincipal(t, "GetListing", w.seen.get("GetListing"), "service-team-order", "service", "listing.read")
}

// Scenario: Only GetListing gets a synthesized background identity
func TestOnlyGetListingGetsBackgroundIdentity(t *testing.T) {
	w := wire(t)
	ctx := context.Background()
	if _, err := w.flash.GetActiveFlashSale(ctx, &promotionv1.GetActiveFlashSaleRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.address.ListAddresses(ctx, &identityv1.ListAddressesRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.stock.GetListing(ctx, &listingv1.GetListingRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []string{"GetActiveFlashSale", "ListAddresses"} {
		md := w.seen.get(rpc)
		if md == nil {
			t.Fatalf("%s: upstream never received the call", rpc)
		}
		if v := md.Get("x-principal-id"); len(v) != 0 {
			t.Errorf("%s: synthesized principal id %v, want none", rpc, v)
		}
	}
	if got := one(w.seen.get("GetListing"), "x-principal-id"); got != "service-team-order" {
		t.Errorf("GetListing principal id = %q, want service-team-order", got)
	}
}
