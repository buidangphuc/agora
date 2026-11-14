# [W3-T1] commerce-user-erasure

## Role
feature-run worker (change `openspec/changes/commerce-user-erasure`)

## Objective
Implement OpenSpec change `commerce-user-erasure` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-order: `GetUserObligations` (SERVICE + `order.internal.read`) and `UserErased` consumer masking shipping address / `order_returns.reason`, deleting cart (DLQ `identity.events.order.dlq`).
- team-payment: `GetPayoutObligations` (`payment.internal.read`, reuse `computeWithdrawal`) and erasure consumer masking bank fields (DLQ `identity.events.payment.dlq`).
- team-promotion: erasure on group `team-promotion.erasure`, DLQ `identity.events.promotion.dlq`. team-referral: erasure on `team-referral-erasure`, DLQ `identity.events.referral.dlq`; `UserRegistered` consumer skips other types.
- team-gateway: test only, both obligation RPCs answer 501 with and without a token (no routing code). Compose: three DLQs + erasure env for four services.
- depends_on: trust-legal-contract (wave 0)
- base: `asdlc/trust-legal` after wave 2
- hot file: `docker-compose.services.yaml`; team-payment (payment-payout-kyc-gate wave 5)

## Write-set (EXCLUSIVE)
- team-order/**
- team-payment/**
- team-promotion/**
- team-referral/**
- team-gateway/internal/edge/obligations_501_test.go
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/order/user_obligations.feature
- platform-e2e/tests/e2e/features/payment/payout_obligations.feature
- platform-e2e/tests/e2e/features/order/user_erasure.feature
- platform-e2e/tests/e2e/features/payment/user_erasure.feature
- platform-e2e/tests/e2e/features/promotion/user_erasure.feature
- platform-e2e/tests/e2e/features/referral/user_erasure.feature
- platform-e2e/tests/e2e/step_definitions/test_user_obligations.py
- platform-e2e/tests/e2e/step_definitions/test_payout_obligations.py
- platform-e2e/tests/e2e/step_definitions/user_obligations_steps.py
- platform-e2e/tests/e2e/step_definitions/test_order_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/test_payment_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/test_promotion_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/test_referral_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/commerce_erasure_steps.py
- platform-e2e/tests/e2e/support/erasure_events_support.py
- widened (P1):
  - openspec/changes/commerce-user-erasure/specs/order-user-erasure/spec.md — the slice's own OpenSpec folder; adds only "- **VERIFIED BY**: <test>" citation lines (no requirement or scenario wording change)
  - openspec/changes/commerce-user-erasure/specs/order-user-obligations/spec.md — the slice's own OpenSpec folder; adds only "- **VERIFIED BY**: <test>" citation lines (no requirement or scenario wording change)
  - openspec/changes/commerce-user-erasure/specs/payment-payout-obligations/spec.md — the slice's own OpenSpec folder; adds only "- **VERIFIED BY**: <test>" citation lines (no requirement or scenario wording change)
  - openspec/changes/commerce-user-erasure/specs/payment-user-erasure/spec.md — the slice's own OpenSpec folder; adds only "- **VERIFIED BY**: <test>" citation lines (no requirement or scenario wording change)
  - openspec/changes/commerce-user-erasure/specs/promotion-user-erasure/spec.md — the slice's own OpenSpec folder; adds only "- **VERIFIED BY**: <test>" citation lines (no requirement or scenario wording change)
  - openspec/changes/commerce-user-erasure/specs/referral-user-erasure/spec.md — the slice's own OpenSpec folder; adds only "- **VERIFIED BY**: <test>" citation lines (no requirement or scenario wording change)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-order --vet` (same for team-payment, team-promotion, team-referral)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway -- ./internal/edge/...`
- `openspec validate commerce-user-erasure --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: order/user_obligations, payment/payout_obligations, order|payment|promotion|referral/user_erasure; the four DLQs empty after a normal erasure
