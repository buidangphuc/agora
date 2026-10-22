# [W1-T2] order-address-eta

## Role
feature-run worker (change `openspec/changes/order-address-eta`)

## Objective
Implement OpenSpec change `order-address-eta` test-first per its tasks.md (scenario ids there); context in plans/q11-checkout-coherence/00-spec.md and the change's design.md Clarifications.
- Mock ETA (HCM/HN +2d else +4d, calendar date in Asia/Ho_Chi_Minh) on `CalculateShippingFee`; fail-closed address resolver (id or default, no first-address fallback, "address not found" / UNAVAILABLE); confirmed-address compare ("address changed"); address details never logged.
- depends_on: q11-checkout-contract. Hot files `internal/service/order.go`, `internal/handler/order.go` are shared with order-multishop-checkout (builds after this).

## Write-set (EXCLUSIVE)
- team-order/proto/platform/order/v1/order.proto
- team-order/generated/**
- team-order/internal/service/order.go
- team-order/internal/service/order_test.go
- team-order/internal/handler/order.go
- team-order/internal/handler/order_test.go
- team-order/internal/handler/address_test.go
- team-order/README.md
- team-order/FEATURES.yaml
- platform-e2e/tests/e2e/support/address_support.py
- platform-e2e/tests/e2e/features/order/address_eta.feature
- platform-e2e/tests/e2e/step_definitions/test_order_address_eta.py
- openspec/changes/order-address-eta/**

## Read-only dependencies
- openspec/changes/order-address-eta/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/step_definitions/address_steps.py, oic_inv_checkout_steps.py, cart_checkout_steps.py (reuse; check for an existing address helper before adding address_support.py)
- plans/q11-checkout-coherence/00-spec.md, slices.yaml, LOCK.md

## Verify
cmp platform-core/packages/proto/platform/order/v1/order.proto team-order/proto/platform/order/v1/order.proto; bash $BUILD/gen.sh $WT team-order; (cd team-order && gofmt -l . && go vet ./... && go test ./... -race); bash $BUILD/e2e.sh $WT tests/e2e/step_definitions/test_order_address_eta.py; $PY platform-e2e/scripts/spec_sync.py order-address-eta --strict

Vars: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; BUILD=/Users/phuc.buidang/Library/Caches/ai-first-runs/c4q11-build; WT=<slice worktree>. Run every test/gate via scripts/asdlc/gate_digest.py (gate-digest skill).
