package rpc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

func soldCountReachable(md protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) []string {
	if seen[md.FullName()] {
		return nil
	}
	seen[md.FullName()] = true
	var found []string
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		if strings.Contains(strings.ToLower(string(f.Name())), "sold_count") {
			found = append(found, string(md.FullName())+"."+string(f.Name()))
		}
		if f.Message() != nil && !f.IsMap() {
			found = append(found, soldCountReachable(f.Message(), seen)...)
		}
		if f.IsMap() && f.MapValue().Message() != nil {
			found = append(found, soldCountReachable(f.MapValue().Message(), seen)...)
		}
	}
	return found
}

func TestListingSales_NoRequestMessageCarriesSoldCount(t *testing.T) {
	msgs := orderv1.File_platform_order_v1_order_proto.Messages()
	requests := 0
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if !strings.HasSuffix(string(md.Name()), "Request") {
			continue
		}
		requests++
		if found := soldCountReachable(md, map[protoreflect.FullName]bool{}); len(found) > 0 {
			t.Errorf("request %s exposes %v", md.FullName(), found)
		}
	}
	if requests < 10 {
		t.Fatalf("expected the order service request messages, found %d", requests)
	}
}

func withUnknownSoldCount[M proto.Message](m M) M {
	b := protowire.AppendTag(nil, 9999, protowire.VarintType)
	b = protowire.AppendVarint(b, 1000000)
	m.ProtoReflect().SetUnknown(b)
	return m
}

func salesSnapshot(ctx context.Context, t *testing.T, o *pgOutbox) string {
	t.Helper()
	rows, err := o.pool.Query(ctx, `SELECT listing_id, sold_count FROM listing_sales ORDER BY listing_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sb, "%s=%d;", id, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func noopListingSalesRow(c store.ListingSalesChange) (outbox.Row, error) {
	return outbox.Row{
		EventID:       fmt.Sprintf("ls:%s:%d:%s", c.ListingID, c.SoldCount, c.SourceEventID),
		AggregateType: "Listing",
		AggregateID:   c.ListingID,
		EventType:     "platform.order.v1.ListingSalesChanged",
		Payload:       []byte(c.ListingID),
	}, nil
}

func TestListingSales_UnchangedAfterOrderRPCsWithArbitraryInput(t *testing.T) {
	ctx := context.Background()
	pg := newPGOutbox(ctx, t)
	orders := pg.orders(store.WithListingSalesOutbox(noopListingSalesRow))
	returns := pg.returns(store.WithReturnListingSalesOutbox(noopListingSalesRow))
	svc := newSvc(t, orders, nil, returns, nil, nil, nil, nil)
	h := newHandler(t, svc, nil, nil)

	paid, err := orders.CreateOrder(ctx, store.Order{
		BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 100, Currency: "VND",
		Items: []store.OrderItem{{ListingID: "L", Quantity: 2, UnitPrice: 50}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.UpdateOrderStatusFrom(ctx, paid.ID, store.OrderStatusPaid, []store.OrderStatus{store.OrderStatusPending}, ""); err != nil {
		t.Fatal(err)
	}
	pending, err := orders.CreateOrder(ctx, store.Order{
		BuyerID: "buyer_1", SellerID: "seller_1", TotalAmount: 100, Currency: "VND",
		Items: []store.OrderItem{{ListingID: "M", Quantity: 3, UnitPrice: 50}},
	})
	if err != nil {
		t.Fatal(err)
	}

	before := salesSnapshot(ctx, t, pg)
	if before != "L=2;" {
		t.Fatalf("snapshot before = %q, want L=2;", before)
	}

	buyer := incomingPrincipalCtxFrom(ctx, "buyer_1")
	_, _ = h.GetOrder(buyer, withUnknownSoldCount(&orderv1.GetOrderRequest{Id: paid.ID}))
	_, _ = h.ListBuyerOrders(buyer, withUnknownSoldCount(&orderv1.ListBuyerOrdersRequest{}))
	_, _ = h.CreateReturnRequest(buyer, withUnknownSoldCount(&orderv1.CreateReturnRequestRequest{OrderId: paid.ID, Reason: "x", RefundAmount: 1}))
	_, _ = h.CancelOrder(buyer, withUnknownSoldCount(&orderv1.CancelOrderRequest{Id: pending.ID, Reason: "changed mind"}))
	_, _ = h.CancelOrder(buyer, withUnknownSoldCount(&orderv1.CancelOrderRequest{Id: "does-not-exist"}))
	_, _ = h.UpdateOrderStatus(buyer, withUnknownSoldCount(&orderv1.UpdateOrderStatusRequest{Id: pending.ID}))

	if after := salesSnapshot(ctx, t, pg); after != before {
		t.Fatalf("listing_sales changed through RPCs: %q -> %q", before, after)
	}
}
