package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

var userScopes = []string{"listing.read", "engagement:read", "engagement:write"}

func principalCtx(id string, typ commonv1.PrincipalType, scopes ...string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{Id: id, Type: typ, Scopes: scopes})
}

func userCtx(id string) context.Context {
	return principalCtx(id, commonv1.PrincipalType_PRINCIPAL_TYPE_USER, userScopes...)
}

func adminCtx() context.Context {
	return principalCtx("admin-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, append(append([]string{}, userScopes...), "admin")...)
}

func anonCtx() context.Context {
	return principalCtx("anonymous", commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS, "listing.read")
}

type fixture struct {
	h        *handler.EngagementHandler
	disputes *countingDisputes
	orders   *orderFake
}

// countingDisputes records how many disputes the service tried to store.
type countingDisputes struct {
	*repository.InMemoryDisputeRepository
	creates int
}

func (c *countingDisputes) CreateDispute(ctx context.Context, d repository.Dispute) (repository.Dispute, error) {
	c.creates++
	return c.InMemoryDisputeRepository.CreateDispute(ctx, d)
}

type orderParties struct{ buyer, seller string }

// fakeOrders is a static OrderPartiesReader for tests that do not need call counts.
type fakeOrders map[string]orderParties

func (f fakeOrders) GetOrderParties(_ context.Context, id string) (string, string, error) {
	o, ok := f[id]
	if !ok {
		return "", "", upstream.ErrOrderNotFound
	}
	return o.buyer, o.seller, nil
}

// orderFake is fakeOrders with a forced error.
type orderFake struct {
	orders fakeOrders
	err    error
}

func (f *orderFake) GetOrderParties(ctx context.Context, id string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.orders.GetOrderParties(ctx, id)
}

// newFixture builds a handler over in-memory repos and seeds an OPEN dispute
// (buyer-1 vs seller-1) directly in the repo, so the read/resolve guards are
// exercised independently of CreateDispute's order verification.
func newFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	disputes := &countingDisputes{InMemoryDisputeRepository: repository.NewInMemoryDisputeRepository()}
	orders := &orderFake{orders: fakeOrders{"order-1": {buyer: "buyer-1", seller: "seller-1"}}}
	h := handler.NewEngagementHandler(
		repository.NewInMemoryRepository(),
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil),
		service.NewQAService(repository.NewInMemoryQARepository(), nil),
		service.NewDisputeService(disputes, nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil),
		handler.WithOrderParties(orders),
	)
	d, err := disputes.InMemoryDisputeRepository.CreateDispute(context.Background(), repository.Dispute{
		OrderID: "order-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "r", Status: repository.DisputeStatusOpen,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{h: h, disputes: disputes, orders: orders}, d.ID
}

func createDispute(f *fixture, ctx context.Context, orderID, defendant string) (*engagementv1.CreateDisputeResponse, error) {
	return f.h.CreateDispute(ctx, &engagementv1.CreateDisputeRequest{OrderId: orderID, DefendantId: defendant, Reason: "broken"})
}

func TestCreateDisputeVerifiesTheOrder(t *testing.T) {
	cases := []struct {
		name      string
		ctx       context.Context
		orderID   string
		defendant string
		orderErr  error
		want      codes.Code
	}{
		{"buyer disputes own order against its seller", userCtx("buyer-1"), "order-1", "seller-1", nil, codes.OK},
		{"unknown order", userCtx("buyer-1"), "order-404", "seller-1", nil, codes.NotFound},
		{"someone else's order", userCtx("buyer-2"), "order-1", "seller-1", nil, codes.NotFound},
		{"seller cannot open against own order", userCtx("seller-1"), "order-1", "buyer-1", nil, codes.NotFound},
		{"wrong defendant", userCtx("buyer-1"), "order-1", "seller-2", nil, codes.PermissionDenied},
		{"order service down", userCtx("buyer-1"), "order-1", "seller-1", status.Error(codes.Unavailable, "down"), codes.Unavailable},
		{"order lookup generic error", userCtx("buyer-1"), "order-1", "seller-1", errors.New("boom"), codes.Unavailable},
		{"anonymous", anonCtx(), "order-1", "seller-1", nil, codes.PermissionDenied},
		{"no principal", context.Background(), "order-1", "seller-1", nil, codes.Unauthenticated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, _ := newFixture(t)
			f.orders.err = c.orderErr
			before := f.disputes.creates
			res, err := createDispute(f, c.ctx, c.orderID, c.defendant)
			if got := status.Code(err); got != c.want {
				t.Fatalf("want %v, got %v (%v)", c.want, got, err)
			}
			if c.want == codes.OK {
				if res.GetDispute().GetClaimantId() != "buyer-1" || res.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN {
					t.Fatalf("unexpected dispute %v", res.GetDispute())
				}
				return
			}
			if res != nil {
				t.Fatalf("nothing may be returned on rejection, got %v", res)
			}
			if f.disputes.creates != before {
				t.Fatal("rejected CreateDispute stored a dispute")
			}
		})
	}
}

// Without an order client CreateDispute fails closed instead of trusting the request.
func TestCreateDisputeFailsClosedWithoutOrderClient(t *testing.T) {
	disputes := &countingDisputes{InMemoryDisputeRepository: repository.NewInMemoryDisputeRepository()}
	h := handler.NewEngagementHandler(repository.NewInMemoryRepository(),
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil),
		service.NewQAService(repository.NewInMemoryQARepository(), nil),
		service.NewDisputeService(disputes, nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil))
	_, err := h.CreateDispute(userCtx("buyer-1"), &engagementv1.CreateDisputeRequest{OrderId: "o", DefendantId: "s", Reason: "r"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want UNAVAILABLE, got %v", err)
	}
	if disputes.creates != 0 {
		t.Fatal("a dispute was stored")
	}
}
