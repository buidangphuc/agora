package settlement_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/protobuf/types/known/timestamppb"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/settlement"
	"github.com/buidangphuc/team-payment/internal/store"
)

var (
	metricsOnce   sync.Once
	metricsReader *sdkmetric.ManualReader
)

func skippedNoBreakdown(t *testing.T) int64 {
	t.Helper()
	metricsOnce.Do(func() {
		metricsReader = sdkmetric.NewManualReader()
		otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricsReader)))
	})
	var rm metricdata.ResourceMetrics
	if err := metricsReader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if !strings.HasPrefix(m.Name, "payment_commission_skipped") {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %s is %T, want an int64 counter", m.Name, m.Data)
			}
			for _, dp := range sum.DataPoints {
				if v, ok := dp.Attributes.Value("reason"); ok && v.AsString() == "no_breakdown" {
					total += dp.Value
				}
			}
		}
	}
	return total
}

type breakdown struct{ items, shipping, shop, platform *int64 }

func amt(v int64) *int64 { return &v }

func paidBreakdownRecord(t *testing.T, orderID string, total int64, b breakdown, seller string) rec {
	t.Helper()
	ev := &orderv1.OrderPaidEvent{
		OrderId: orderID, BuyerId: "buyer", TotalAmount: total, Currency: "VND", PaidAt: timestamppb.Now(),
		Items:         []*orderv1.OrderLineItemFact{{ListingId: "l", SellerId: seller, Quantity: 1, UnitPrice: total}},
		ItemsSubtotal: b.items, ShippingFee: b.shipping, ShopDiscountAmount: b.shop, PlatformDiscountAmount: b.platform,
	}
	return rec{Key: orderID, Value: envelope(t, settlement.OrderPaidEventType, "paid-"+orderID, ev)}
}

func TestConsumer_EventWithoutTheBreakdownBooksNoCommission(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o1", 320000)
	before := skippedNoBreakdown(t)
	dlq := feed(t, f, paidRecord(t, "o1", 320000, "s1"))
	wantNoDLQ(t, dlq)
	got := f.entries(t, "s1")
	if len(got[store.LedgerTypeCommission]) != 0 {
		t.Fatalf("commission booked without a breakdown: %+v", got[store.LedgerTypeCommission])
	}
	if len(got[store.LedgerTypeOrderSettlement]) != 1 || got[store.LedgerTypeOrderSettlement][0].Amount != 320000 {
		t.Fatalf("credits: %+v", got[store.LedgerTypeOrderSettlement])
	}
	if d := skippedNoBreakdown(t) - before; d != 1 {
		t.Fatalf("payment_commission_skipped_total{reason=no_breakdown} increased by %d, want 1", d)
	}
}

func TestConsumer_EventWithoutThePriceBreakdownCreditsThePaidAmountOnly(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o1", 320000)
	dlq := feed(t, f, paidRecord(t, "o1", 320000, "s1"))
	wantNoDLQ(t, dlq)
	got := f.entries(t, "s1")
	if len(got[store.LedgerTypeOrderSettlement]) != 1 || got[store.LedgerTypeOrderSettlement][0].Amount != 320000 {
		t.Fatalf("credits: %+v", got[store.LedgerTypeOrderSettlement])
	}
	if len(got[store.LedgerTypeCommission]) != 0 {
		t.Fatalf("commission: %+v", got[store.LedgerTypeCommission])
	}
}

func TestConsumer_PlatformVoucherIsCreditedToTheSellerOnTopOfThePaidAmount(t *testing.T) {
	f := newFixture(t)
	tx := f.paid(t, "o1", 320000)
	dlq := feed(t, f, paidBreakdownRecord(t, "o1", 320000,
		breakdown{items: amt(300000), shipping: amt(30000), shop: amt(0), platform: amt(10000)}, "s1"))
	wantNoDLQ(t, dlq)
	got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]
	if len(got) != 1 || got[0].Amount != 330000 || got[0].ReferenceID != tx.ID {
		t.Fatalf("credits: %+v, want one of 330000 referencing %s", got, tx.ID)
	}
}

func TestConsumer_InconsistentBreakdownIsParkedAndWritesNothing(t *testing.T) {
	cases := map[string]breakdown{
		"shop discount above subtotal": {items: amt(300000), shipping: amt(30000), shop: amt(400000), platform: amt(0)},
		"negative platform discount":   {items: amt(300000), shipping: amt(30000), shop: amt(0), platform: amt(-1)},
		"negative shipping":            {items: amt(300000), shipping: amt(-5), shop: amt(0), platform: amt(0)},
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.paid(t, "bad", 300000)
			f.paid(t, "good", 100000)
			dlq := feed(t, f,
				paidBreakdownRecord(t, "bad", 300000, b, "s1"),
				paidBreakdownRecord(t, "good", 100000,
					breakdown{items: amt(100000), shipping: amt(0), shop: amt(0), platform: amt(0)}, "s1"))
			if got := dlq.parked(); len(got) != 1 || got[0] != "bad" {
				t.Fatalf("dlq %v, want [bad]", got)
			}
			if n := dlq.loop.attemptsOf("bad"); n != 1 {
				t.Fatalf("bad record attempts %d, want 1 (permanent)", n)
			}
			credits := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]
			if len(credits) != 1 || credits[0].Amount != 100000 {
				t.Fatalf("credits %+v, want only the later paid order's 100000", credits)
			}
		})
	}
}
