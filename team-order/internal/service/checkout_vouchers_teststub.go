package service

import orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"

// TEST-STUB: compile-only. The implementer replaces this with the real option in order.go and deletes this file.
func WithVouchers(shop []*orderv1.ShopVoucher, platformCode string) CheckoutOption {
	return func(*checkoutConfig) {}
}
