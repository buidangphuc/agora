package upstream

// Scenarios (order-advertised-price): Hold RPCs carry the service principal,
// Price read keeps the forwarded principal, Unset promotion address yields no client.
//
// Interface fixed by these tests (implementer must provide):
//   - field `Flash promotionv1.FlashSaleServiceClient` on *PromotionClients, set by
//     DialPromotion (non-empty addr); ReserveFlashSaleStock / CommitFlashSaleStock /
//     ReleaseFlashSaleStock are sent as x-principal-id=service-team-order,
//     x-principal-type=service, scope exactly promotion.reserve, replacing any
//     forwarded buyer principal; GetActiveFlashSale (and other reads) keep the
//     forwarded principal.
//   - DialPromotion("") still returns (nil, nil).

import (
	"context"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

type recordingFlashServer struct {
	promotionv1.UnimplementedFlashSaleServiceServer
	mu   sync.Mutex
	seen map[string]metadata.MD
}

func (s *recordingFlashServer) rec(ctx context.Context, rpc string) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	s.seen[rpc] = md
	s.mu.Unlock()
}

func (s *recordingFlashServer) ReserveFlashSaleStock(ctx context.Context, _ *promotionv1.ReserveFlashSaleStockRequest) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	s.rec(ctx, "Reserve")
	return &promotionv1.ReserveFlashSaleStockResponse{}, nil
}

func (s *recordingFlashServer) CommitFlashSaleStock(ctx context.Context, _ *promotionv1.CommitFlashSaleStockRequest) (*promotionv1.CommitFlashSaleStockResponse, error) {
	s.rec(ctx, "Commit")
	return &promotionv1.CommitFlashSaleStockResponse{}, nil
}

func (s *recordingFlashServer) ReleaseFlashSaleStock(ctx context.Context, _ *promotionv1.ReleaseFlashSaleStockRequest) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	s.rec(ctx, "Release")
	return &promotionv1.ReleaseFlashSaleStockResponse{}, nil
}

func (s *recordingFlashServer) GetActiveFlashSale(ctx context.Context, _ *promotionv1.GetActiveFlashSaleRequest) (*promotionv1.GetActiveFlashSaleResponse, error) {
	s.rec(ctx, "Get")
	return &promotionv1.GetActiveFlashSaleResponse{}, nil
}

func dialFlash(t *testing.T) (*PromotionClients, *recordingFlashServer) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rec := &recordingFlashServer{seen: map[string]metadata.MD{}}
	srv := grpc.NewServer()
	promotionv1.RegisterFlashSaleServiceServer(srv, rec)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	pc, err := DialPromotion(lis.Addr().String())
	if err != nil || pc == nil {
		t.Fatalf("dial: %v %v", pc, err)
	}
	t.Cleanup(pc.Close)
	if pc.Flash == nil {
		t.Fatal("PromotionClients.Flash is nil for a non-empty address")
	}
	return pc, rec
}

func first(md metadata.MD, k string) string {
	if v := md.Get(k); len(v) > 0 {
		return v[0]
	}
	return ""
}

func TestServiceFlashClientSendsServicePrincipal(t *testing.T) {
	pc, rec := dialFlash(t)
	buyer := metadata.NewOutgoingContext(
		metadata.NewIncomingContext(context.Background(), userMD()), userMD())

	if _, err := pc.Flash.ReserveFlashSaleStock(buyer, &promotionv1.ReserveFlashSaleStockRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pc.Flash.CommitFlashSaleStock(buyer, &promotionv1.CommitFlashSaleStockRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := pc.Flash.ReleaseFlashSaleStock(buyer, &promotionv1.ReleaseFlashSaleStockRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, rpc := range []string{"Reserve", "Commit", "Release"} {
		md, ok := rec.seen[rpc]
		if !ok {
			t.Fatalf("%s never reached the server", rpc)
		}
		if got := md.Get("x-principal-id"); len(got) != 1 || got[0] != "service-team-order" {
			t.Errorf("%s principal id = %v, want only service-team-order", rpc, got)
		}
		if got := first(md, "x-principal-type"); got != "service" {
			t.Errorf("%s principal type = %q, want service", rpc, got)
		}
		if got := md.Get("x-principal-scopes"); len(got) != 1 || got[0] != "promotion.reserve" {
			t.Errorf("%s scopes = %v, want only promotion.reserve", rpc, got)
		}
	}
}

func TestServiceFlashClientReadNotWrapped(t *testing.T) {
	pc, rec := dialFlash(t)
	buyer := metadata.NewOutgoingContext(
		metadata.NewIncomingContext(context.Background(), userMD()), userMD())

	if _, err := pc.Flash.GetActiveFlashSale(buyer, &promotionv1.GetActiveFlashSaleRequest{ListingId: "l1"}); err != nil {
		t.Fatal(err)
	}
	md := rec.seen["Get"]
	if got := first(md, "x-principal-id"); got != "buyer-1" {
		t.Errorf("GetActiveFlashSale principal id = %q, want forwarded buyer-1", got)
	}
	if got := first(md, "x-principal-type"); got != "user" {
		t.Errorf("GetActiveFlashSale principal type = %q, want user", got)
	}
	for _, s := range md.Get("x-principal-scopes") {
		if s == "promotion.reserve" {
			t.Errorf("GetActiveFlashSale must not carry promotion.reserve, got %v", md.Get("x-principal-scopes"))
		}
	}
}

func TestDialPromotionEmptyAddrNil(t *testing.T) {
	pc, err := DialPromotion("")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if pc != nil {
		t.Fatalf("client set = %v, want nil", pc)
	}
}
