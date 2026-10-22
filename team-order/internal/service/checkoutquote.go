package service

import (
	"sort"
	"time"
)

type quoteGroupInput struct {
	sellerID        string
	itemIDs         []string
	itemsSubtotal   int64
	shopVoucherCode string
	shopDiscount    int64
}

type quoteInput struct {
	city             string
	platformDiscount int64
	groups           []quoteGroupInput
}

type quoteGroup struct {
	sellerID            string
	itemIDs             []string
	itemsSubtotal       int64
	shippingFee         int64
	isFreeShipping      bool
	shopVoucherCode     string
	shopDiscount        int64
	platformDiscount    int64
	totalAmount         int64
	estimatedDeliveryAt time.Time
}

type quoteResult struct {
	groups        []quoteGroup
	itemsSubtotal int64
	shippingFee   int64
	totalAmount   int64
}

func (s *OrderService) quoteCheckout(in quoteInput) quoteResult {
	inputs := append([]quoteGroupInput(nil), in.groups...)
	sort.SliceStable(inputs, func(i, j int) bool { return inputs[i].sellerID < inputs[j].sellerID })

	var itemsTotal int64
	for _, g := range inputs {
		itemsTotal += g.itemsSubtotal
	}

	shares := make([]int64, len(inputs))
	var allocated int64
	if itemsTotal > 0 {
		for i, g := range inputs {
			shares[i] = in.platformDiscount * g.itemsSubtotal / itemsTotal
			allocated += shares[i]
		}
	}
	remainder := in.platformDiscount - allocated
	for i := 0; remainder > 0 && len(inputs) > 0; i = (i + 1) % len(inputs) {
		shares[i]++
		remainder--
	}

	res := quoteResult{groups: make([]quoteGroup, 0, len(inputs))}
	for i, g := range inputs {
		fee, free, _, _, eta := s.CalculateShippingFee(in.city, g.itemsSubtotal)
		total := g.itemsSubtotal + fee - g.shopDiscount - shares[i]
		if total < 0 {
			total = 0
		}
		res.groups = append(res.groups, quoteGroup{
			sellerID:            g.sellerID,
			itemIDs:             g.itemIDs,
			itemsSubtotal:       g.itemsSubtotal,
			shippingFee:         fee,
			isFreeShipping:      free,
			shopVoucherCode:     g.shopVoucherCode,
			shopDiscount:        g.shopDiscount,
			platformDiscount:    shares[i],
			totalAmount:         total,
			estimatedDeliveryAt: eta,
		})
		res.itemsSubtotal += g.itemsSubtotal
		res.shippingFee += fee
		res.totalAmount += total
	}
	return res
}
