package service

import (
	"fmt"
	"testing"
	"time"
)

const quoteCity = "Đà Nẵng"

func quoteSvc() *OrderService {
	now := time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC)
	return NewOrderService(nil, nil, nil, nil, nil, nil, nil, WithClock(func() time.Time { return now }))
}

func quoteGroupFor(t *testing.T, r quoteResult, seller string) quoteGroup {
	t.Helper()
	for _, g := range r.groups {
		if g.sellerID == seller {
			return g
		}
	}
	t.Fatalf("no group for seller %s in %+v", seller, r.groups)
	return quoteGroup{}
}

func TestQuoteCheckoutFreeShippingPerGroup(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, groups: []quoteGroupInput{
		{sellerID: "A", itemsSubtotal: 600000},
		{sellerID: "B", itemsSubtotal: 100000},
	}})
	if len(r.groups) != 2 {
		t.Fatalf("Multishop checkout: Free shipping is decided per group: groups=%d", len(r.groups))
	}
	a, b := quoteGroupFor(t, r, "A"), quoteGroupFor(t, r, "B")
	if a.shippingFee != 0 || !a.isFreeShipping {
		t.Errorf("Multishop checkout: Free shipping is decided per group: A fee=%d free=%v", a.shippingFee, a.isFreeShipping)
	}
	if b.shippingFee == 0 || b.isFreeShipping {
		t.Errorf("Multishop checkout: Free shipping is decided per group: B fee=%d free=%v", b.shippingFee, b.isFreeShipping)
	}
	if a.estimatedDeliveryAt.IsZero() || b.estimatedDeliveryAt.IsZero() {
		t.Errorf("Multishop checkout: Free shipping is decided per group: ETA must be set per group")
	}
}

func TestQuoteCheckoutProratesPlatformDiscount(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, platformDiscount: 10001, groups: []quoteGroupInput{
		{sellerID: "A", itemsSubtotal: 200000},
		{sellerID: "B", itemsSubtotal: 100000},
		{sellerID: "C", itemsSubtotal: 100000},
	}})
	want := map[string]int64{"A": 5001, "B": 2500, "C": 2500}
	var sum int64
	for s, w := range want {
		g := quoteGroupFor(t, r, s)
		if g.platformDiscount != w {
			t.Errorf("Multishop checkout: Platform discount splits across groups and sums back: %s share=%d want %d", s, g.platformDiscount, w)
		}
		sum += g.platformDiscount
	}
	if sum != 10001 {
		t.Errorf("Multishop checkout: Platform discount splits across groups and sums back: sum=%d want 10001", sum)
	}
}

func TestQuoteCheckoutRemainderTieLowestSellerID(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, platformDiscount: 100000, groups: []quoteGroupInput{
		{sellerID: "seller-c", itemsSubtotal: 100000},
		{sellerID: "seller-a", itemsSubtotal: 100000},
		{sellerID: "seller-b", itemsSubtotal: 100000},
	}})
	want := map[string]int64{"seller-a": 33334, "seller-b": 33333, "seller-c": 33333}
	for s, w := range want {
		if g := quoteGroupFor(t, r, s); g.platformDiscount != w {
			t.Errorf("Multishop checkout: Remainder ties go to the lowest seller id: %s share=%d want %d", s, g.platformDiscount, w)
		}
	}
}

func TestQuoteCheckoutSingleGroupWholeDiscount(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, platformDiscount: 50000, groups: []quoteGroupInput{
		{sellerID: "A", itemsSubtotal: 300000},
	}})
	if len(r.groups) != 1 || r.groups[0].platformDiscount != 50000 {
		t.Errorf("Multishop checkout: A single group takes the whole platform discount: %+v", r.groups)
	}
}

func TestQuoteCheckoutClampsGroupTotalAtZero(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, platformDiscount: 60000, groups: []quoteGroupInput{
		{sellerID: "A", itemsSubtotal: 100000, shopVoucherCode: "SHOPA", shopDiscount: 100000},
	}})
	if len(r.groups) != 1 {
		t.Fatalf("Multishop checkout: Stacked discounts never make a negative total: groups=%d", len(r.groups))
	}
	g := r.groups[0]
	if g.shippingFee != 35000 {
		t.Errorf("Multishop checkout: Stacked discounts never make a negative total: fee=%d want 35000", g.shippingFee)
	}
	if g.totalAmount != 0 {
		t.Errorf("Multishop checkout: Stacked discounts never make a negative total: group total=%d want 0", g.totalAmount)
	}
	if r.totalAmount != 0 {
		t.Errorf("Multishop checkout: Stacked discounts never make a negative total: response total=%d want 0", r.totalAmount)
	}
}

func TestQuoteCheckoutStacksShopAndPlatform(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, platformDiscount: 60000, groups: []quoteGroupInput{
		{sellerID: "A", itemsSubtotal: 300000, shopVoucherCode: "SHOPA", shopDiscount: 30000},
		{sellerID: "B", itemsSubtotal: 300000},
	}})
	a := quoteGroupFor(t, r, "A")
	if a.shopDiscount != 30000 || a.platformDiscount != 30000 || a.shopVoucherCode != "SHOPA" {
		t.Errorf("Multishop checkout: Platform and shop vouchers stack on one group: %+v", a)
	}
	if a.totalAmount != 275000 {
		t.Errorf("Multishop checkout: Platform and shop vouchers stack on one group: total=%d want 275000", a.totalAmount)
	}
	if b := quoteGroupFor(t, r, "B"); b.totalAmount != 300000+35000-30000 {
		t.Errorf("Multishop checkout: Platform and shop vouchers stack on one group: B total=%d", b.totalAmount)
	}
}

func TestQuoteCheckoutGroupsSortedBySellerID(t *testing.T) {
	r := quoteSvc().quoteCheckout(quoteInput{city: quoteCity, groups: []quoteGroupInput{
		{sellerID: "B", itemsSubtotal: 1000}, {sellerID: "A", itemsSubtotal: 1000},
	}})
	if len(r.groups) != 2 || r.groups[0].sellerID != "A" || r.groups[1].sellerID != "B" {
		t.Errorf("Multishop checkout: Totals always add up: groups must be sorted by seller id: %+v", r.groups)
	}
}

func TestQuoteCheckoutTableTotalsInvariant(t *testing.T) {
	subtotals := [][]int64{
		{1000},
		{600000, 100000},
		{100000, 100000, 100000},
		{999999, 1, 500000, 250000},
		{7, 11, 13, 17, 19},
		{500000, 499999, 3, 3, 3},
	}
	shop := [][]int64{{0}, {5000, 0}, {100000, 0, 50}}
	platforms := []int64{0, 1, 10001, 99999, 250000}
	cities := []string{"Hà Nội", quoteCity}
	s := quoteSvc()
	for si, subs := range subtotals {
		for pi, d := range platforms {
			for ci, city := range cities {
				name := fmt.Sprintf("Multishop checkout: Totals always add up/%d-%d-%d", si, pi, ci)
				t.Run(name, func(t *testing.T) {
					var in quoteInput
					in.city, in.platformDiscount = city, d
					var sel int64
					for i, st := range subs {
						sd := shop[(si+i)%len(shop)][i%len(shop[(si+i)%len(shop)])]
						if sd > st {
							sd = st
						}
						in.groups = append(in.groups, quoteGroupInput{sellerID: fmt.Sprintf("s%d", i), itemsSubtotal: st, shopDiscount: sd})
						sel += st
					}
					r := s.quoteCheckout(in)
					if len(r.groups) != len(subs) {
						t.Fatalf("groups=%d want %d", len(r.groups), len(subs))
					}
					var sumTotal, sumShare, sumItems, sumFee int64
					for i, g := range r.groups {
						exp := g.itemsSubtotal + g.shippingFee - g.shopDiscount - g.platformDiscount
						if exp < 0 {
							exp = 0
						}
						if g.totalAmount != exp || g.totalAmount < 0 {
							t.Errorf("group %d total=%d want %d", i, g.totalAmount, exp)
						}
						sumTotal += g.totalAmount
						sumShare += g.platformDiscount
						sumItems += g.itemsSubtotal
						sumFee += g.shippingFee
					}
					if r.totalAmount != sumTotal {
						t.Errorf("response total=%d sum of groups=%d", r.totalAmount, sumTotal)
					}
					if sumShare != d {
						t.Errorf("platform shares sum=%d want %d", sumShare, d)
					}
					if r.itemsSubtotal != sumItems || r.itemsSubtotal != sel || r.shippingFee != sumFee {
						t.Errorf("response items=%d fee=%d want %d / %d", r.itemsSubtotal, r.shippingFee, sumItems, sumFee)
					}
				})
			}
		}
	}
}
