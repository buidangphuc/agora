package service_test

import (
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
	"github.com/buidangphuc/team-order/internal/service"
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
		rs     repository.ReturnStatus
		os     repository.OrderStatus
		window *time.Time
		want   bool
	}{
		{"rejected on shipped", repository.ReturnStatusRejected, repository.OrderStatusShipped, nil, true},
		{"rejected completed inside window", repository.ReturnStatusRejected, repository.OrderStatusCompleted, &end, true},
		{"rejected completed after window", repository.ReturnStatusRejected, repository.OrderStatusCompleted, &past, false},
		{"rejected completed at exact window end", repository.ReturnStatusRejected, repository.OrderStatusCompleted, &now, false},
		{"rejected completed without window", repository.ReturnStatusRejected, repository.OrderStatusCompleted, nil, false},
		{"rejected on paid", repository.ReturnStatusRejected, repository.OrderStatusPaid, nil, false},
		{"rejected on cancelled", repository.ReturnStatusRejected, repository.OrderStatusCancelled, nil, false},
		{"pending on shipped", repository.ReturnStatusPending, repository.OrderStatusShipped, nil, false},
		{"approved on shipped", repository.ReturnStatusApproved, repository.OrderStatusShipped, nil, false},
		{"refunded on shipped", repository.ReturnStatusRefunded, repository.OrderStatusShipped, nil, false},
		{"pending completed inside window", repository.ReturnStatusPending, repository.OrderStatusCompleted, &end, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := service.CanEscalate(repository.OrderReturn{Status: c.rs}, repository.Order{Status: c.os, ReturnWindowEndsAt: c.window}, now)
			if got != c.want {
				t.Fatalf("CanEscalate=%v want %v", got, c.want)
			}
		})
	}
}
