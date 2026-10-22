package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/handler"
	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
	"github.com/buidangphuc/team-order/internal/upstream/upstreamtest"
)

type fakeAddrClient struct {
	identityv1.AddressServiceClient
	addrs []*identityv1.Address
	err   error
	calls int
}

func (f *fakeAddrClient) ListAddresses(_ context.Context, _ *identityv1.ListAddressesRequest, _ ...grpc.CallOption) (*identityv1.ListAddressesResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.ListAddressesResponse{Addresses: f.addrs}, nil
}

func addrA() *identityv1.Address {
	return &identityv1.Address{Id: "addr_A", UserId: "buyer_1", RecipientName: "An", Phone: "0900000001", Street: "1 A St", Ward: "W1", District: "D1", City: "Hà Nội", IsDefault: true}
}

func addrB() *identityv1.Address {
	return &identityv1.Address{Id: "addr_B", UserId: "buyer_1", RecipientName: "Binh", Phone: "0900000002", Street: "2 B St", Ward: "W2", District: "D2", City: "Đà Nẵng"}
}

func addressCheckout(t *testing.T, ac identityv1.AddressServiceClient) (*handler.OrderHandler, *upstreamtest.Domain, repository.CartRepository, *repository.InMemoryOrderRepository) {
	t.Helper()
	domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 10})
	orders := repository.NewInMemoryOrderRepository()
	carts := repository.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), repository.CartItem{
		UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1", Title: "item",
	}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewOrderService(orders, carts, nil, nil, domain, nil, nil)
	return handler.NewOrderHandler(svc, ac, nil), domain, carts, orders
}

func assertRefusedNothingReserved(t *testing.T, err error, want codes.Code, msg string, domain *upstreamtest.Domain, carts repository.CartRepository, orders *repository.InMemoryOrderRepository) {
	t.Helper()
	if status.Code(err) != want {
		t.Fatalf("want %v, got %v", want, err)
	}
	if msg != "" && status.Convert(err).Message() != msg {
		t.Fatalf("want message %q, got %q", msg, status.Convert(err).Message())
	}
	if domain.Calls.Reserve != 0 || domain.Stock("lst_1") != 10 {
		t.Fatalf("nothing may be reserved: calls %d stock %d", domain.Calls.Reserve, domain.Stock("lst_1"))
	}
	if items, _ := carts.GetCart(context.Background(), "buyer_1"); len(items) != 1 {
		t.Fatalf("cart must keep its lines, got %d", len(items))
	}
	if l, _ := orders.ListBuyerOrders(context.Background(), "buyer_1", 0); len(l) != 0 {
		t.Fatalf("no order may exist: %v", l)
	}
}

func TestCreateOrderUsesPickedAddress(t *testing.T) {
	// Scenario: Address and ETA: The picked address is the order's address
	h, _, _, _ := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
	resp, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B"})
	if err != nil {
		t.Fatal(err)
	}
	got := resp.GetOrders()[0].GetShippingAddress()
	if got.GetId() != "addr_B" || got.GetStreet() != "2 B St" {
		t.Fatalf("want address B, got %v", got)
	}
}

func TestCreateOrderEmptyAddressIDUsesDefault(t *testing.T) {
	// Scenario: Address and ETA: An empty address id uses the default address
	a, b := addrA(), addrB()
	a.IsDefault, b.IsDefault = false, true
	h, _, _, _ := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{a, b}})
	resp, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetOrders()[0].GetShippingAddress().GetId(); got != "addr_B" {
		t.Fatalf("want default address B, got %q", got)
	}
}

func TestCreateOrderUnknownAddressRefused(t *testing.T) {
	// Scenario: Address and ETA: An unknown address id is refused and nothing is reserved
	h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_missing"})
	assertRefusedNothingReserved(t, err, codes.FailedPrecondition, "address not found", domain, carts, orders)
}

func TestCreateOrderNoAddressRefused(t *testing.T) {
	// Scenario: Address and ETA: A buyer without any address is refused
	h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{})
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	assertRefusedNothingReserved(t, err, codes.FailedPrecondition, "address not found", domain, carts, orders)
}

func TestCreateOrderNoDefaultAddressRefused(t *testing.T) {
	// Scenario: Address and ETA: Without a default address an empty address id is refused
	a := addrA()
	a.IsDefault = false
	h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{a, addrB()}})
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	assertRefusedNothingReserved(t, err, codes.FailedPrecondition, "address not found", domain, carts, orders)
}

func TestCreateOrderEmptyCityAddressRefused(t *testing.T) {
	// Scenario: Address and ETA: An address with an empty city is refused
	for name, city := range map[string]string{"empty": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			a := addrA()
			a.City = city
			h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{a}})
			_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
			assertRefusedNothingReserved(t, err, codes.FailedPrecondition, "address not found", domain, carts, orders)
		})
	}
}

func TestCreateOrderIdentityUnavailableRefused(t *testing.T) {
	// Scenario: Address and ETA: An identity outage never places an order
	h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{err: errors.New("identity down")})
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	assertRefusedNothingReserved(t, err, codes.Unavailable, "", domain, carts, orders)
}

func TestCreateOrderNilAddressClientRefused(t *testing.T) {
	// Scenario: Address and ETA: A missing address client refuses the order
	h, domain, carts, orders := addressCheckout(t, nil)
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{})
	assertRefusedNothingReserved(t, err, codes.Unavailable, "", domain, carts, orders)
}

func TestCreateOrderNonUserNoAddressLookup(t *testing.T) {
	// Scenario: Address and ETA: A non-user principal is refused before any identity call
	fake := &fakeAddrClient{addrs: []*identityv1.Address{addrA()}}
	h, domain, _, _ := addressCheckout(t, fake)
	_, err := h.CreateOrder(serviceCtx("order.write"), &orderv1.CreateOrderRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if fake.calls != 0 || domain.Calls.Reserve != 0 {
		t.Fatalf("no identity call or reservation allowed: list %d reserve %d", fake.calls, domain.Calls.Reserve)
	}
}
