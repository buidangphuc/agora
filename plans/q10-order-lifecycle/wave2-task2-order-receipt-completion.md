# [W2-T2] order-receipt-completion

## Role
feature-run worker (change `openspec/changes/archive/2026-10-18-order-receipt-completion`, owner team-order)

## Objective
Implement OpenSpec change `order-receipt-completion` test-first per its tasks.md (scenario ids there); context in plans/q10-order-lifecycle/00-spec.md and the change's design.md Clarifications.
- team-order ConfirmReceipt (buyer only), auto-complete 7d after shipped, OrderCompleted, 15d return window + ReturnRequested, return auto-approve after 3d, can_escalate, ship_by stamps; team-gateway ConfirmReceipt forwarder. Builds after order-unpaid-expiry (hot files).

## Write-set (EXCLUSIVE)
- team-order/proto/platform/order/v1/order.proto
- team-order/generated/**
- team-gateway/proto/platform/order/v1/order.proto
- team-gateway/generated/**
- team-order/internal/config/config.go
- team-order/internal/config/envcheck.go
- team-order/internal/config/config_test.go
- team-order/.env.example (widened at build: task 2)
- team-order/migrations/*_order_lifecycle_completion.up.sql
- team-order/migrations/*_order_lifecycle_completion.down.sql
- team-order/internal/repository/order.go
- team-order/internal/repository/placer.go
- team-order/internal/repository/return.go
- team-order/internal/repository/migration_test.go
- team-order/internal/repository/*_test.go
- team-order/internal/events/publisher.go
- team-order/internal/events/*_test.go
- team-order/internal/service/transitions.go
- team-order/internal/service/order.go (widened at build: task 5)
- team-order/internal/service/saga.go
- team-order/internal/service/lifecycle.go
- team-order/internal/service/*_test.go
- team-order/internal/handler/order.go
- team-order/internal/handler/*_test.go
- team-order/internal/grpcserver/server.go
- team-order/cmd/server/main.go
- team-order/README.md
- team-order/tests/readme_docs_test.sh (widened at build: task 11)
- team-order/FEATURES.yaml
- team-gateway/internal/edge/order.go
- team-gateway/internal/edge/order_test.go
- team-gateway/internal/edge/confirm_receipt_test.go
- team-gateway/FEATURES.yaml
- platform-e2e/compose/order-inventory.override.yaml
- platform-e2e/tests/e2e/support/order_lifecycle_support.py
- platform-e2e/tests/e2e/support/prm_rma_support.py
- platform-e2e/tests/e2e/features/order/order_receipt_completion.feature
- platform-e2e/tests/e2e/step_definitions/test_order_receipt_completion.py
- platform-e2e/tests/e2e/features/order/return_refund_settlement.feature
- platform-e2e/tests/e2e/features/seller/return_refund_ui.feature
- platform-e2e/tests/e2e/step_definitions/prm_rma_steps.py
- platform-e2e/tests/e2e/step_definitions/prm_rma_ui_steps.py
- platform-e2e/tests/e2e/step_definitions/test_prm_rma_return_refund_settlement.py
- platform-e2e/tests/e2e/step_definitions/test_prm_rma_return_refund_ui.py
- openspec/changes/archive/2026-10-18-order-receipt-completion/**

## Read-only dependencies
- openspec/changes/archive/2026-10-18-order-receipt-completion/** is frozen by its LOCK.md digest except tasks.md ticks
- order-unpaid-expiry's lifecycle sweep, migration 0008 and OrderPlaced path (merged before this slice)
- team-gateway/internal/edge CancelOrder forwarder (pattern)
- plans/q10-order-lifecycle/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-order && test -z "$(gofmt -l . | grep -v generated)" && go vet ./... && go test ./... -race); make -C team-gateway check; (cd team-gateway && go test ./internal/edge/... -race); for r in team-order team-gateway; do cmp platform-core/packages/proto/platform/order/v1/order.proto $r/proto/platform/order/v1/order.proto || exit 1; done; (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_order_receipt_completion.py tests/e2e/step_definitions/test_prm_rma_return_refund_settlement.py tests/e2e/step_definitions/test_prm_rma_return_refund_ui.py); $PY platform-e2e/scripts/spec_sync.py order-receipt-completion --strict
