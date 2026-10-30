package order_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
)

type mdListing struct {
	listingv1.UnimplementedListingServiceServer
	mu   sync.Mutex
	seen metadata.MD
}

func (m *mdListing) CommitReservation(ctx context.Context, _ *listingv1.CommitReservationRequest) (*listingv1.CommitReservationResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	m.mu.Lock()
	m.seen = md
	m.mu.Unlock()
	return &listingv1.CommitReservationResponse{}, nil
}

func TestResyncCommitsAsServicePrincipal(t *testing.T) {
	ctx := context.Background()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fl := &mdListing{}
	srv := grpc.NewServer()
	listingv1.RegisterListingServiceServer(srv, fl)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := sdkupstream.NewClient(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	sagas := fake.NewInMemorySagaRepository()
	if _, err := sagas.CreateReservation(ctx, store.Reservation{
		ID: "res-1", SagaID: "s", OrderID: "ord-1", ListingID: "lst_1", Quantity: 1,
		Status: store.ReservationStatusCommitted, ExpiresAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	rep, err := order.ResyncCommittedReservations(ctx, sagas, listingv1.NewListingServiceClient(conn), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Committed != 1 {
		t.Fatalf("report: %+v", rep)
	}
	fl.mu.Lock()
	md := fl.seen
	fl.mu.Unlock()
	if md == nil {
		t.Fatal("team-domain never received the call")
	}
	for k, want := range map[string]string{
		"x-principal-id":     "service-team-order",
		"x-principal-type":   "service",
		"x-principal-scopes": "listing.write",
	} {
		if got := md.Get(k); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want exactly [%s]", k, got, want)
		}
	}
}
