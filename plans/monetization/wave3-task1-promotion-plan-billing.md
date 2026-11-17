# [W3-T1] promotion-plan-billing

## Role
feature-run worker (change `openspec/changes/archive/2026-11-17-promotion-plan-billing`)

## Objective
Implement OpenSpec change `promotion-plan-billing` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-promotion: corrected plan prices (199000 / 499000) and `term_days`, `Subscribe` charges via `ChargeSellerFee` (key `sub:<id>:<period_seq+1>:<plan_id>`), deferred downgrade, `CancelSubscription`, `GetMySubscription`, renewal sweeper with retry window and `subscription_renewals_overdue`, grandfathering.
- team-gateway forwards the two subscription RPCs; `UPSTREAM_PAYMENT_ADDR` on team-promotion in compose. Renumber migration `0009_plan_billing` to the next free number.
- depends_on: payment-seller-fee-charge, promotion-ads-campaign-mgmt (hot file, serialised)
- base: `asdlc/monetization` after wave 2
- hot file: team-promotion migrations/sponsored files and edge/promotion.go (last of the promotion chain); docker-compose.services.yaml (after listing-plan-cap)

## Write-set (EXCLUSIVE)
- team-promotion/internal/**
- team-promotion/cmd/**
- team-promotion/go.mod
- team-promotion/go.sum
- team-promotion/README.md
- team-promotion/.env.example
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/promotion_subscription_test.go
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/promotion/seller_plan_billing.feature
- platform-e2e/tests/e2e/step_definitions/seller_plan_billing_steps.py
- platform-e2e/tests/e2e/support/seller_plan_billing_support.py
- platform-e2e/tests/e2e/step_definitions/test_seller_plan_billing.py
- widened (P1)
  - team-gateway/README.md — docs-sync: GetMySubscription/CancelSubscription now forwarded, line citations refreshed
  - openspec/changes/archive/2026-11-17-promotion-plan-billing/specs/promotion-plan-billing/spec.md — VERIFIED BY citations matched to real test names
  - openspec/changes/archive/2026-11-17-promotion-plan-billing/tasks.md — task 2.4 tick

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-promotion` (equivalent: `cd team-promotion && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway` (equivalent: `cd team-gateway && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-promotion && make check` and `cd team-gateway && make check`
- e2e: promotion/seller_plan_billing.feature against env monetization (team-promotion, team-payment, team-gateway)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . promotion-plan-billing`
