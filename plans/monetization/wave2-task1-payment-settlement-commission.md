# [W2-T1] payment-settlement-commission

## Role
feature-run worker (change `openspec/changes/payment-settlement-commission`)

## Objective
Implement OpenSpec change `payment-settlement-commission` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-payment books `COMMISSION` (5% of subtotal minus shop voucher, `COMMISSION_RATE_BPS`) in the settlement transaction, credits paid + platform voucher, reverses on refunds/cancel with cumulative rounding, nets holdback, and serves `GetOrderSettlement`.
- Migration after payment-seller-fee-charge's: keep its `SUBSCRIPTION_FEE` CHECK arm. team-gateway forwards `GetOrderSettlement`; `ChargeSellerFee` stays unrouted.
- depends_on: order-paid-price-breakdown, payment-seller-fee-charge (hot file, serialised)
- base: `asdlc/monetization` after wave 1
- hot file: team-payment/internal/store/ledger.go + migrations (after payment-seller-fee-charge); team-gateway/internal/edge/payment.go (only this slice)

## Write-set (EXCLUSIVE)
- team-payment/internal/**
- team-payment/migrations/**
- team-payment/cmd/**
- team-payment/go.mod
- team-payment/go.sum
- team-payment/README.md
- team-payment/readme_test.go
- team-payment/.env.example
- team-payment/FEATURES.yaml
- team-gateway/internal/edge/payment.go
- team-gateway/internal/edge/payment_settlement_test.go
- platform-e2e/tests/e2e/features/payment/settlement_commission.feature
- platform-e2e/tests/e2e/step_definitions/settlement_commission_steps.py
- platform-e2e/tests/e2e/support/settlement_commission_support.py
- platform-e2e/tests/e2e/step_definitions/test_settlement_commission.py

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-payment` (equivalent: `cd team-payment && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway` (equivalent: `cd team-gateway && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-payment && make check`
- e2e: payment/settlement_commission.feature against env monetization (team-order, team-payment, team-gateway, team-promotion)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . payment-settlement-commission`
