package service

import "time"

// TEST-STUB: compile-only placeholder for task 3. The implementer moves these
// types and quoteCheckout into checkoutquote.go and deletes this file.

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
	return quoteResult{}
}
