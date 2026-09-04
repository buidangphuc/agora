package query

import (
	"context"
	"sort"
	"time"
)

// Event is a single flattened warehouse row as the in-memory repository sees it.
// It mirrors the subset of `tracking_events` (+ its properties bag) the seller
// queries read, letting unit tests build fixtures without a live warehouse.
type Event struct {
	SellerID   string
	EventType  string // "impression" | "view" | "add_to_cart" | "click"
	OrderID    string // non-empty marks a purchase/order
	Revenue    int64  // minor units (0 when not an order line)
	SKU        string
	ListingID  string
	Units      int64
	OccurredAt time.Time
}

// MemoryRepository is an in-memory Repository over a fixed set of Events, used
// by the unit tests (and as a safe, warehouse-free fallback).
type MemoryRepository struct {
	events []Event
}

// NewMemoryRepository builds a repository over the given events.
func NewMemoryRepository(events ...Event) *MemoryRepository {
	return &MemoryRepository{events: events}
}

// inWindow reports whether e belongs to sellerID and falls in [from, to].
func (e Event) inWindow(sellerID string, from, to time.Time) bool {
	if e.SellerID != sellerID {
		return false
	}
	t := e.OccurredAt.UTC()
	if t.Before(from) || t.After(to) {
		return false
	}
	return true
}

// SellerFunnel counts impressions/views/adds by event type and orders as the
// distinct set of non-empty order_ids.
func (m *MemoryRepository) SellerFunnel(_ context.Context, sellerID string, from, to time.Time) (Funnel, error) {
	var f Funnel
	orders := map[string]struct{}{}
	for _, e := range m.events {
		if !e.inWindow(sellerID, from, to) {
			continue
		}
		switch e.EventType {
		case "impression":
			f.Impressions++
		case "view":
			f.Views++
		case "add_to_cart":
			f.Adds++
		}
		if e.OrderID != "" {
			orders[e.OrderID] = struct{}{}
		}
	}
	f.Orders = int64(len(orders))
	return f, nil
}

// RevenueBreakdown aggregates revenue by UTC day (over order rows) and the
// top-N SKUs by revenue.
func (m *MemoryRepository) RevenueBreakdown(_ context.Context, sellerID string, from, to time.Time, topN int) (Breakdown, error) {
	type dayAgg struct {
		revenue int64
		orders  map[string]struct{}
	}
	type skuAgg struct {
		listingID string
		revenue   int64
		units     int64
	}
	days := map[string]*dayAgg{}
	skus := map[string]*skuAgg{}

	for _, e := range m.events {
		if !e.inWindow(sellerID, from, to) {
			continue
		}
		// Revenue-by-day is keyed on order rows (a purchase carries an order_id).
		if e.OrderID != "" {
			key := e.OccurredAt.UTC().Format("2006-01-02")
			d := days[key]
			if d == nil {
				d = &dayAgg{orders: map[string]struct{}{}}
				days[key] = d
			}
			d.revenue += e.Revenue
			d.orders[e.OrderID] = struct{}{}
		}
		if e.SKU != "" {
			s := skus[e.SKU]
			if s == nil {
				s = &skuAgg{listingID: e.ListingID}
				skus[e.SKU] = s
			}
			s.revenue += e.Revenue
			s.units += e.Units
		}
	}

	var b Breakdown
	for day, agg := range days {
		b.Days = append(b.Days, DayRevenue{
			Day:        day,
			Revenue:    agg.revenue,
			OrderCount: int64(len(agg.orders)),
		})
	}
	sort.Slice(b.Days, func(i, j int) bool { return b.Days[i].Day < b.Days[j].Day })

	for sku, agg := range skus {
		b.TopSkus = append(b.TopSkus, TopSku{
			SKU:       sku,
			ListingID: agg.listingID,
			Revenue:   agg.revenue,
			UnitsSold: agg.units,
		})
	}
	// Deterministic top-N: revenue desc, then SKU asc as a tiebreak.
	sort.Slice(b.TopSkus, func(i, j int) bool {
		if b.TopSkus[i].Revenue != b.TopSkus[j].Revenue {
			return b.TopSkus[i].Revenue > b.TopSkus[j].Revenue
		}
		return b.TopSkus[i].SKU < b.TopSkus[j].SKU
	})
	if topN > 0 && len(b.TopSkus) > topN {
		b.TopSkus = b.TopSkus[:topN]
	}
	return b, nil
}

// compile-time assertion that the fake satisfies the seam.
var _ Repository = (*MemoryRepository)(nil)
