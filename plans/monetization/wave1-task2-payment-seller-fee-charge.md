# [W1-T2] payment-seller-fee-charge

## Role
feature-run worker (change `openspec/changes/archive/2026-11-17-payment-seller-fee-charge`)

## Objective
Implement OpenSpec change `payment-seller-fee-charge` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-payment `ChargeSellerFee`: service-principal only, idempotent `SUBSCRIPTION_FEE` debit via `AppendDebitOnce` (seller lock, reference lookup, withdrawable check with hold-back), migration extending the ledger CHECKs, ledger mapper surfaces the type.
- No team-gateway edit; the e2e asserts the gateway does not expose the RPC.
- depends_on: monetization-contract
- base: `asdlc/monetization` after wave 0
- hot file: team-payment/internal/store/ledger.go and team-payment/migrations/ (ledger CHECK) shared with payment-settlement-commission (wave 2, merges this CHECK arm additively)

## Write-set (EXCLUSIVE)
- team-payment/internal/**
- team-payment/migrations/**
- team-payment/cmd/**
- team-payment/go.mod
- team-payment/go.sum
- team-payment/README.md
- team-payment/readme_test.go
- team-payment/FEATURES.yaml
- platform-e2e/tests/e2e/features/payment/seller_fee_charge.feature
- platform-e2e/tests/e2e/step_definitions/seller_fee_charge_steps.py
- platform-e2e/tests/e2e/support/seller_fee_charge_support.py
- platform-e2e/tests/e2e/step_definitions/test_seller_fee_charge.py

### widened (P1)
- team-gateway/README.md (docs-sync README: ChargeSellerFee is service-to-service, not forwarded by the edge)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-payment` (equivalent: `cd team-payment && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-payment && make check`
- e2e: payment/seller_fee_charge.feature against env monetization (team-gateway, team-payment)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . payment-seller-fee-charge`
