package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/internal/store/fake"
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

func addressCheckout(t *testing.T, ac identityv1.AddressServiceClient) (*rpc.OrderHandler, *upstreamtest.Domain, store.CartRepository, *fake.InMemoryOrderRepository) {
	t.Helper()
	domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 10})
	orders := fake.NewInMemoryOrderRepository()
	carts := fake.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{
		UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1", Title: "item",
	}); err != nil {
		t.Fatal(err)
	}
	svc := newSvc(t, orders, carts, nil, nil, domain, nil, nil)
	return newHandler(t, svc, ac, nil), domain, carts, orders
}

func assertRefusedNothingReserved(t *testing.T, err error, want codes.Code, msg string, domain *upstreamtest.Domain, carts store.CartRepository, orders *fake.InMemoryOrderRepository) {
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

func confirmedOf(a *identityv1.Address) *identityv1.Address {
	return &identityv1.Address{Id: a.GetId(), UserId: a.GetUserId(), RecipientName: a.GetRecipientName(), Phone: a.GetPhone(), Street: a.GetStreet(), Ward: a.GetWard(), District: a.GetDistrict(), City: a.GetCity(), IsDefault: a.GetIsDefault()}
}

func TestCreateOrderConfirmedAddressMatches(t *testing.T) {
	// Scenario: Address and ETA: An unchanged confirmed address places the order
	h, _, _, _ := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
	resp, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B", ConfirmedAddress: confirmedOf(addrB())})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetOrders()[0].GetShippingAddress(); got.GetId() != "addr_B" || got.GetStreet() != "2 B St" {
		t.Fatalf("want address B snapshot, got %v", got)
	}
}

func TestCreateOrderEditedAddressRefused(t *testing.T) {
	// Scenario: Address and ETA: An address edited after confirm is refused
	h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
	c := confirmedOf(addrB())
	c.City = "Hà Nội"
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B", ConfirmedAddress: c})
	assertRefusedNothingReserved(t, err, codes.FailedPrecondition, "address changed", domain, carts, orders)
}

func TestCreateOrderEachAddressFieldChangeRefused(t *testing.T) {
	// Scenario: Address and ETA: Each delivery field counts as a change
	mutators := map[string]func(*identityv1.Address){
		"recipient_name": func(a *identityv1.Address) { a.RecipientName += "x" },
		"phone":          func(a *identityv1.Address) { a.Phone += "1" },
		"street":         func(a *identityv1.Address) { a.Street += "x" },
		"ward":           func(a *identityv1.Address) { a.Ward += "x" },
		"district":       func(a *identityv1.Address) { a.District += "x" },
		"city":           func(a *identityv1.Address) { a.City += "x" },
	}
	for name, mut := range mutators {
		t.Run(name, func(t *testing.T) {
			h, domain, carts, orders := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
			c := confirmedOf(addrB())
			mut(c)
			_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B", ConfirmedAddress: c})
			assertRefusedNothingReserved(t, err, codes.FailedPrecondition, "address changed", domain, carts, orders)
		})
	}
}

func TestCreateOrderConfirmedAddressIgnoresMetadata(t *testing.T) {
	// Scenario: Address and ETA: Metadata differences are not a change
	h, _, _, _ := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
	c := confirmedOf(addrB())
	c.IsDefault = true
	c.UserId = ""
	if _, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B", ConfirmedAddress: c}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateOrderNoConfirmedAddressSkipsCompare(t *testing.T) {
	// Scenario: Address and ETA: A request without a confirmed address is not compared
	h, _, _, _ := addressCheckout(t, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}})
	resp, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetOrders()[0].GetShippingAddress().GetStreet(); got != "2 B St" {
		t.Fatalf("want identity copy, got %q", got)
	}
}

func TestCreateOrderAddressRefusalLogsIDOnly(t *testing.T) {
	// Scenario: Address and ETA: Address details are never logged
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	domain := upstreamtest.NewDomain(map[string]int32{"lst_1": 10})
	orders := fake.NewInMemoryOrderRepository()
	carts := fake.NewInMemoryCartRepository()
	if _, err := carts.AddItem(context.Background(), store.CartItem{UserID: "buyer_1", ListingID: "lst_1", Quantity: 2, UnitPrice: 1000, SellerID: "s1", Title: "item"}); err != nil {
		t.Fatal(err)
	}
	svc := newSvc(t, orders, carts, nil, nil, domain, nil, nil)
	h := newHandler(t, svc, &fakeAddrClient{addrs: []*identityv1.Address{addrA(), addrB()}}, logger)
	c := confirmedOf(addrB())
	c.City = "Hà Nội"
	_, err := h.CreateOrder(incomingPrincipalCtx("buyer_1", "buyer"), &orderv1.CreateOrderRequest{AddressId: "addr_B", ConfirmedAddress: c})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "addr_B") {
		t.Fatalf("log must carry the address id, got %q", out)
	}
	for _, secret := range []string{"Binh", "0900000002", "2 B St"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log leaks %q: %q", secret, out)
		}
	}
}
