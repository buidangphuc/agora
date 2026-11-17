package events_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

const skippedCounter = "order_paid_breakdown_skipped_total"

func breakdownOrder(subtotal, shipping, discount, platform, total int64) store.Order {
	return store.Order{
		ID:                     "ord-secret-1",
		BuyerID:                "buyer-secret-1",
		SellerID:               "seller-1",
		Currency:               "VND",
		TotalAmount:            total,
		ItemsSubtotal:          subtotal,
		ShippingFee:            shipping,
		DiscountAmount:         discount,
		PlatformDiscountAmount: platform,
		Items:                  []store.OrderItem{{ListingID: "l1", VariantID: "v1", Quantity: 2, UnitPrice: subtotal / 2}},
	}
}

func buildPaid(t *testing.T, o store.Order) (*orderv1.OrderPaidEvent, string) {
	t.Helper()
	eid := events.OrderPaidEventID(o.ID)
	raw, err := events.BuildOrderPaidEnvelope(eid, o, time.Unix(1700000000, 0).UTC(), "req-1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var paid orderv1.OrderPaidEvent
	if err := proto.Unmarshal(env.Payload, &paid); err != nil {
		t.Fatalf("unmarshal paid: %v", err)
	}
	return &paid, env.EventId
}

func assertBreakdown(t *testing.T, p *orderv1.OrderPaidEvent, sub, ship, shop, plat int64) {
	t.Helper()
	if p.ItemsSubtotal == nil || p.ShippingFee == nil || p.ShopDiscountAmount == nil || p.PlatformDiscountAmount == nil {
		t.Fatalf("breakdown fields must all be present: %v", p)
	}
	if *p.ItemsSubtotal != sub || *p.ShippingFee != ship || *p.ShopDiscountAmount != shop || *p.PlatformDiscountAmount != plat {
		t.Fatalf("got sub=%d ship=%d shop=%d plat=%d, want %d %d %d %d",
			*p.ItemsSubtotal, *p.ShippingFee, *p.ShopDiscountAmount, *p.PlatformDiscountAmount, sub, ship, shop, plat)
	}
}

func assertAbsent(t *testing.T, p *orderv1.OrderPaidEvent) {
	t.Helper()
	if p.ItemsSubtotal != nil || p.ShippingFee != nil || p.ShopDiscountAmount != nil || p.PlatformDiscountAmount != nil {
		t.Fatalf("breakdown fields must be absent: %v", p)
	}
}

func TestPaidEventCarriesBreakdownShopVoucher(t *testing.T) {
	p, _ := buildPaid(t, breakdownOrder(200000, 30000, 20000, 0, 210000))
	assertBreakdown(t, p, 200000, 30000, 20000, 0)
	if p.TotalAmount != 210000 {
		t.Fatalf("total = %d", p.TotalAmount)
	}
}

func TestPaidEventSplitsShopAndPlatformDiscount(t *testing.T) {
	p, _ := buildPaid(t, breakdownOrder(200000, 30000, 50000, 30000, 180000))
	assertBreakdown(t, p, 200000, 30000, 20000, 30000)
}

func TestPaidEventZeroDiscountIsPresent(t *testing.T) {
	p, _ := buildPaid(t, breakdownOrder(200000, 0, 0, 0, 200000))
	assertBreakdown(t, p, 200000, 0, 0, 0)
}

func TestPaidEventLegacyFieldsUnchanged(t *testing.T) {
	o := breakdownOrder(200000, 30000, 50000, 30000, 180000)
	p, eid := buildPaid(t, o)
	if eid != events.OrderPaidEventID(o.ID) {
		t.Fatalf("event id changed: %s", eid)
	}
	if p.OrderId != o.ID || p.BuyerId != o.BuyerID || p.SellerId != o.SellerID ||
		p.TotalAmount != 180000 || p.Currency != "VND" || len(p.Items) != 1 ||
		p.Items[0].ListingId != "l1" || p.Items[0].Quantity != 2 || p.PaidAt == nil {
		t.Fatalf("legacy fields changed: %v", p)
	}
}

type capture struct {
	reader *sdkmetric.ManualReader
	logs   *bytes.Buffer
}

func newCapture(t *testing.T) *capture {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	buf := &bytes.Buffer{}
	prevLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		otel.SetMeterProvider(prevMP)
		slog.SetDefault(prevLog)
	})
	return &capture{reader: reader, logs: buf}
}

func (c *capture) skipped(t *testing.T) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := c.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var n int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != skippedCounter {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is not an int64 sum", skippedCounter)
			}
			for _, dp := range sum.DataPoints {
				n += dp.Value
			}
		}
	}
	return n
}

func assertSkipObserved(t *testing.T, c *capture) {
	t.Helper()
	if got := c.skipped(t); got != 1 {
		t.Fatalf("%s = %d, want 1", skippedCounter, got)
	}
	out := c.logs.String()
	if n := strings.Count(out, "level=ERROR"); n != 1 {
		t.Fatalf("want exactly one error log line, got %d: %q", n, out)
	}
	if strings.Contains(out, "ord-secret-1") || strings.Contains(out, "buyer-secret-1") {
		t.Fatalf("log leaks raw ids: %q", out)
	}
}

func TestPaidEventSkipsNegativeShopShare(t *testing.T) {
	c := newCapture(t)
	o := breakdownOrder(200000, 30000, 10000, 30000, 190000)
	p, _ := buildPaid(t, o)
	assertAbsent(t, p)
	if p.TotalAmount != 190000 || p.OrderId != o.ID {
		t.Fatalf("legacy fields must still be published: %v", p)
	}
	assertSkipObserved(t, c)
}

func TestPaidEventSkipsIncoherentTotal(t *testing.T) {
	c := newCapture(t)
	o := breakdownOrder(200000, 30000, 20000, 0, 999999)
	p, _ := buildPaid(t, o)
	assertAbsent(t, p)
	if p.TotalAmount != 999999 {
		t.Fatalf("total must be published as is: %d", p.TotalAmount)
	}
	assertSkipObserved(t, c)
}
