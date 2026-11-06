# [W1-T2] order-has-completed-order

## Role
feature-run worker (change `openspec/changes/order-has-completed-order`)

## Objective
Implement OpenSpec change `order-has-completed-order` test-first per its tasks.md; context in plans/q7-q37-referral-welcome/00-spec.md and the change's design.md.
- OrderService.HasCompletedOrder: SERVICE principal with `order.read` only, one EXISTS query, not routed at the gateway (501).
- depends_on: q7-referral-welcome-contract (merged)

## Write-set (EXCLUSIVE)
- team-order/internal/**
- team-order/tests/**
- team-order/FEATURES.yaml
- team-order/README.md
- team-gateway/internal/edge/q7_order_unrouted_test.go
- platform-e2e/tests/e2e/features/order/has_completed_order.feature
- platform-e2e/tests/e2e/step_definitions/order_q7_steps.py
- platform-e2e/tests/e2e/step_definitions/test_order_has_completed_order.py

## Verify
- `cd team-order && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./... && make lint`
- `cd team-gateway && go test ./internal/edge/ -run Q7`
- e2e: deferred (local stack busy)
