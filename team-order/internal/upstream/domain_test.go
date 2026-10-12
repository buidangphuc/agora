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

// dialRecorder returns a real go-sdk connection to a recording upstream.
func dialRecorder(t *testing.T) (*grpc.ClientConn, *recorder) {
	t.Helper()
	addr, rec := startRecorder(t) //nolint:contextcheck // the recorder serves its own connections
	conn, err := dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, rec
}

func (r *recorder) md(method string) metadata.MD {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[method]
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
	conn, rec := dialRecorder(t)
	client := NewServiceStockClient(listingv1.NewListingServiceClient(conn))
	for name, call := range map[string]func(context.Context) error{
		listingv1.ListingService_ReserveStock_FullMethodName: func(ctx context.Context) error {
			_, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{})
			return err
		},
		listingv1.ListingService_ReleaseStock_FullMethodName: func(ctx context.Context) error {
			_, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{})
			return err
		},
		listingv1.ListingService_CommitReservation_FullMethodName: func(ctx context.Context) error {
			_, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(userCtx()); err != nil {
				t.Fatal(err)
			}
			md := rec.md(name)
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
	conn, rec := dialRecorder(t)
	client := NewServiceStockClient(listingv1.NewListingServiceClient(conn))
	if _, err := client.ReleaseStock(context.Background(), &listingv1.ReleaseStockRequest{}); err != nil {
		t.Fatal(err)
	}
	md := rec.md(listingv1.ListingService_ReleaseStock_FullMethodName)
	if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "service" {
		t.Fatalf("principal type = %v, want service", v)
	}
}

func TestOtherDomainCallsKeepForwardedUser(t *testing.T) {
	conn, rec := dialRecorder(t)
	client := NewServiceStockClient(listingv1.NewListingServiceClient(conn))
	if _, err := client.GetListing(userCtx(), &listingv1.GetListingRequest{Id: "l1"}); err != nil {
		t.Fatal(err)
	}
	md := rec.md(listingv1.ListingService_GetListing_FullMethodName)
	if v := md.Get("x-principal-id"); len(v) != 1 || v[0] != "buyer-1" {
		t.Fatalf("GetListing principal id = %v, want forwarded buyer-1", v)
	}
	if v := md.Get("x-principal-type"); len(v) != 1 || v[0] != "user" {
		t.Fatalf("GetListing principal type = %v, want user", v)
	}
	if v := md.Get("x-principal-scopes"); len(v) != 1 || v[0] != "listing.read" {
		t.Fatalf("forwarded scopes = %v, want unchanged listing.read", v)
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
