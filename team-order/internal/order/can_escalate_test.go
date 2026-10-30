package order_test

import (
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/order"
	"github.com/buidangphuc/team-order/internal/store"
)

// Scenarios: Order receipt: A rejected return on a shipped order can escalate;
// A rejected return inside the window after completion can escalate;
// A rejected return cannot escalate after the window closes;
// A return that is not rejected cannot escalate
func TestCanEscalateTable(t *testing.T) {
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	cases := []struct {
		name   string
		rs     store.ReturnStatus
		os     store.OrderStatus
		window *time.Time
		want   bool
	}{
		{"rejected on shipped", store.ReturnStatusRejected, store.OrderStatusShipped, nil, true},
		{"rejected completed inside window", store.ReturnStatusRejected, store.OrderStatusCompleted, &end, true},
		{"rejected completed after window", store.ReturnStatusRejected, store.OrderStatusCompleted, &past, false},
		{"rejected completed at exact window end", store.ReturnStatusRejected, store.OrderStatusCompleted, &now, false},
		{"rejected completed without window", store.ReturnStatusRejected, store.OrderStatusCompleted, nil, false},
		{"rejected on paid", store.ReturnStatusRejected, store.OrderStatusPaid, nil, false},
		{"rejected on cancelled", store.ReturnStatusRejected, store.OrderStatusCancelled, nil, false},
		{"pending on shipped", store.ReturnStatusPending, store.OrderStatusShipped, nil, false},
		{"approved on shipped", store.ReturnStatusApproved, store.OrderStatusShipped, nil, false},
		{"refunded on shipped", store.ReturnStatusRefunded, store.OrderStatusShipped, nil, false},
		{"pending completed inside window", store.ReturnStatusPending, store.OrderStatusCompleted, &end, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := order.CanEscalate(store.OrderReturn{Status: c.rs}, store.Order{Status: c.os, ReturnWindowEndsAt: c.window}, now)
			if got != c.want {
				t.Fatalf("CanEscalate=%v want %v", got, c.want)
			}
		})
	}
}
