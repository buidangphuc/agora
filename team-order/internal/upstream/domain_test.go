package upstream

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

// recorder is a gRPC server that accepts any unary RPC and records the metadata
// each method was called with.
type recorder struct {
	mu   sync.Mutex
	seen map[string]metadata.MD
}

func startRecorder(t *testing.T) (addr string, rec *recorder) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rec = &recorder{seen: map[string]metadata.MD{}}
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		method, _ := grpc.MethodFromServerStream(stream)
		rec.mu.Lock()
		rec.seen[method] = md
		rec.mu.Unlock()
		var in emptypb.Empty
		if err := stream.RecvMsg(&in); err != nil {
			return err
		}
		return stream.SendMsg(&emptypb.Empty{})
	}))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), rec
}

// outgoingMD calls method through a real go-sdk connection behind scopedConn with
// ctx as the caller's context and returns the metadata that reached the upstream.
func outgoingMD(ctx context.Context, t *testing.T, method string) metadata.MD {
	t.Helper()
	addr, rec := startRecorder(t) //nolint:contextcheck // the recorder serves its own connections
	conn, scoped, err := dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := scoped.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Fatalf("invoke %s: %v", method, err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.seen[method]
}

// userCtx is the context of a handler serving the buyer buyer-1: the principal the
// auth interceptor resolved and the request id.
func userCtx() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "buyer-1", Type: "user", Scopes: []string{"listing.read"}})
	return reqctx.WithRequestID(ctx, "req-1")
}

func userMD() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", "buyer-1",
		"x-principal-type", "user",
		"x-principal-scopes", "listing.read",
		"x-request-id", "req-1",
	)
}

func TestStockCallsAlwaysUseServicePrincipal(t *testing.T) {
	for _, method := range []string{
		listingv1.ListingService_ReserveStock_FullMethodName,
		listingv1.ListingService_ReleaseStock_FullMethodName,
		listingv1.ListingService_CommitReservation_FullMethodName,
	} {
		t.Run(method, func(t *testing.T) {
			md := outgoingMD(userCtx(), t, method)
			if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "service-team-order" {
				t.Fatalf("principal id = %v, want service-team-order", v)
			}
			if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "service" {
				t.Fatalf("principal type = %v, want service", v)
			}
			if v := md.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.write" {
				t.Fatalf("scopes = %v, want listing.write", v)
			}
			if v := md.Get("x-request-id"); len(v) != 1 || v[0] != "req-1" {
				t.Fatalf("request id not preserved: %v", v)
			}
		})
	}
}

func TestStockCallsWithoutIncomingUseServicePrincipal(t *testing.T) {
	md := outgoingMD(context.Background(), t, listingv1.ListingService_ReleaseStock_FullMethodName)
	if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "service" {
		t.Fatalf("principal type = %v, want service", v)
	}
}

func TestOtherDomainCallsKeepForwardedUser(t *testing.T) {
	md := outgoingMD(userCtx(), t, listingv1.ListingService_GetListing_FullMethodName)
	if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "buyer-1" {
		t.Fatalf("GetListing principal id = %v, want forwarded buyer-1", v)
	}
	if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "user" {
		t.Fatalf("GetListing principal type = %v, want user", v)
	}
}

func TestForwardedUserScopesAreNotWidened(t *testing.T) {
	md := outgoingMD(userCtx(), t, listingv1.ListingService_GetListing_FullMethodName)
	if v := md.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.read" {
		t.Fatalf("forwarded scopes = %v, want unchanged listing.read", v)
	}
}

func TestUnmarkedBackgroundCallIsReadOnlyService(t *testing.T) {
	md := outgoingMD(context.Background(), t, listingv1.ListingService_GetListing_FullMethodName)
	if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "service-team-order" {
		t.Fatalf("principal id = %v", v)
	}
	if v := md.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.read" {
		t.Fatalf("scopes = %v, want only listing.read", v)
	}
}

func TestPromotionMethodsCarryOnlyPromotionReserve(t *testing.T) {
	for _, m := range []string{
		promotionv1.VoucherService_ValidateAndReserve_FullMethodName,
		promotionv1.VoucherService_CommitReservation_FullMethodName,
		promotionv1.VoucherService_ReleaseReservation_FullMethodName,
	} {
		md := outgoingMD(userCtx(), t, m)
		if v := md.Get("x-principal-scopes"); len(v) != 1 || v[0] != "promotion.reserve" {
			t.Fatalf("%s scopes = %v, want only promotion.reserve", m, v)
		}
		if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "service" {
			t.Fatalf("%s type = %v", m, v)
		}
	}
}

type recordingVoucherServer struct {
	promotionv1.UnimplementedVoucherServiceServer
	mu   sync.Mutex
	seen map[string]metadata.MD
}

func (s *recordingVoucherServer) rec(ctx context.Context, rpc string) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	s.seen[rpc] = md
	s.mu.Unlock()
}

func (s *recordingVoucherServer) ValidateAndReserve(ctx context.Context, _ *promotionv1.ValidateAndReserveRequest) (*promotionv1.ValidateAndReserveResponse, error) {
	s.rec(ctx, "ValidateAndReserve")
	return &promotionv1.ValidateAndReserveResponse{}, nil
}

func (s *recordingVoucherServer) CommitReservation(ctx context.Context, _ *promotionv1.CommitReservationRequest) (*promotionv1.CommitReservationResponse, error) {
	s.rec(ctx, "CommitReservation")
	return &promotionv1.CommitReservationResponse{}, nil
}

func (s *recordingVoucherServer) ReleaseReservation(ctx context.Context, _ *promotionv1.ReleaseReservationRequest) (*promotionv1.ReleaseReservationResponse, error) {
	s.rec(ctx, "ReleaseReservation")
	return &promotionv1.ReleaseReservationResponse{}, nil
}

// The real DialPromotion client must present SERVICE + promotion.reserve for the
// three saga RPCs, from a buyer request context and a background one alike.
func TestDialPromotionSagaCallsPresentPromotionReserveOverTheWire(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rec := &recordingVoucherServer{seen: map[string]metadata.MD{}}
	srv := grpc.NewServer()
	promotionv1.RegisterVoucherServiceServer(srv, rec)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	pc, err := DialPromotion(lis.Addr().String())
	if err != nil || pc == nil {
		t.Fatalf("dial: %v %v", pc, err)
	}
	defer pc.Close()

	buyer := metadata.NewOutgoingContext(
		metadata.NewIncomingContext(context.Background(), userMD()), userMD())
	if _, err := pc.Voucher.ValidateAndReserve(buyer, &promotionv1.ValidateAndReserveRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pc.Voucher.CommitReservation(context.Background(), &promotionv1.CommitReservationRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pc.Voucher.ReleaseReservation(buyer, &promotionv1.ReleaseReservationRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []string{"ValidateAndReserve", "CommitReservation", "ReleaseReservation"} {
		md := rec.seen[rpc]
		get := func(k string) string {
			if v := md.Get(k); len(v) > 0 {
				return v[0]
			}
			return ""
		}
		if get("x-principal-id") != "service-team-order" || get("x-principal-type") != "service" || get("x-principal-scopes") != "promotion.reserve" {
			t.Errorf("%s: promotion received %v", rpc, md)
		}
	}
}

// recordingDomain captures the context each stock RPC is invoked with, so the
// wrapper's marking can be run through the real interceptor.
type recordingDomain struct {
	DomainClient
	ctxs map[string]context.Context
}

func (r *recordingDomain) GetListing(ctx context.Context, _ *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	r.ctxs[listingv1.ListingService_GetListing_FullMethodName] = ctx
	return &listingv1.GetListingResponse{}, nil
}

func (r *recordingDomain) CommitReservation(ctx context.Context, _ *listingv1.CommitReservationRequest, _ ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	r.ctxs[listingv1.ListingService_CommitReservation_FullMethodName] = ctx
	return &listingv1.CommitReservationResponse{}, nil
}

// A buyer-context CommitReservation through the service-stock wrapper carries the
// service principal with exactly listing.write; GetListing through the same wrapper
// still forwards the buyer.
func TestServiceStockClientCommitIsServiceAndGetListingForwardsBuyer(t *testing.T) {
	inner := &recordingDomain{ctxs: map[string]context.Context{}}
	client := NewServiceStockClient(inner)
	buyer := userCtx()
	if _, err := client.CommitReservation(buyer, &listingv1.CommitReservationRequest{ReservationId: "r1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetListing(buyer, &listingv1.GetListingRequest{Id: "l1"}); err != nil {
		t.Fatal(err)
	}
	run := func(method string) metadata.MD {
		// Use an unrelated method name so only the context marking (not the
		// methodScopes safety net) decides the principal.
		return outgoingMD(inner.ctxs[method], t, "/x.Y/Z")
	}
	commit := run(listingv1.ListingService_CommitReservation_FullMethodName)
	if v := commit.Get("x-principal-id"); len(v) != 1 || v[0] != "service-team-order" {
		t.Fatalf("commit principal = %v", v)
	}
	if v := commit.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.write" {
		t.Fatalf("commit scopes = %v, want exactly listing.write", v)
	}
	get := run(listingv1.ListingService_GetListing_FullMethodName)
	if v := get.Get("x-principal-id"); len(v) != 1 || v[0] != "buyer-1" {
		t.Fatalf("GetListing principal = %v, want forwarded buyer-1", v)
	}
	if v := get.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.read" {
		t.Fatalf("GetListing scopes = %v, want unchanged listing.read", v)
	}
}
