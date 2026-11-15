# [W5-T1] payment-payout-kyc-gate

## Role
feature-run worker (change `openspec/changes/payment-payout-kyc-gate`)

## Objective
Implement OpenSpec change `payment-payout-kyc-gate` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-payment: vendor `verification.proto` byte-identical (task 1.1, open issue A3), `UPSTREAM_VERIFICATION_ADDR` required, `internal/upstream/verification.go` `NewPayoutGate` (single attempt, 2 s, non-VERIFIED -> FAILED_PRECONDITION, any other failure -> UNAVAILABLE), gate in `RequestPayout`/`RequestWalletPayout` after authz and validation; README.
- Compose: `UPSTREAM_VERIFICATION_ADDR=team-verification-svc:50064` + depends_on; seed scripts verify payout sellers through the real KYC RPCs (idempotent).
- E2E: `payout_kyc_gate.feature`, `flows/verify_seller_flow.py` called from the existing payout journeys so they stay green, outage steps.
- depends_on: verification-kyc-documents (wave 1)
- base: `asdlc/trust-legal` after wave 4
- hot file: `docker-compose.services.yaml`; `platform-core/tools/seed-marketplace.sh` + payout steps (seller-kyc-ui wave 7)

## Write-set (EXCLUSIVE)
- team-payment/**
- docker-compose.services.yaml
- platform-e2e/compose/payment-ledger.override.yaml
- platform-core/tools/seed-marketplace.sh
- platform-core/tools/seed-full-marketplace.sh
- platform-core/tools/seed_full_marketplace.py
- platform-core/tools/seed-full.sh
- platform-e2e/tests/e2e/features/payment/payout_kyc_gate.feature
- platform-e2e/tests/e2e/step_definitions/test_payout_kyc_gate.py
- platform-e2e/tests/e2e/step_definitions/payout_kyc_gate_steps.py
- platform-e2e/tests/e2e/flows/verify_seller_flow.py
- platform-e2e/tests/e2e/step_definitions/group_c_steps.py
- platform-e2e/tests/e2e/step_definitions/seller_steps.py
- platform-e2e/tests/e2e/step_definitions/pear_authz_payout_steps.py
- platform-e2e/tests/e2e/step_definitions/plp_steps.py
- platform-e2e/tests/e2e/step_definitions/seller_wallet_access_steps.py
- widened (P1):
  - openspec/changes/payment-payout-kyc-gate/specs/payout-kyc-gate/spec.md — the slice's own OpenSpec folder; VERIFIED BY citations only (TBD replaced with real test names), no requirement or scenario wording change
  - openspec/changes/payment-payout-kyc-gate/tasks.md — the slice's own OpenSpec folder; e2e task ticks 2.1-2.5
  - platform-e2e/README.md — doc updated by docs-sync (seed now KYC-verifies sellers via ReviewKyc)
  - platform-e2e/tests/e2e/flows/kyc_flow.py — e2e flow of this change (verify_seller; packet named it verify_seller_flow.py)
  - platform-e2e/tests/e2e/flows/__init__.py — e2e flow export of verify_seller for this change
  - platform-e2e/tests/e2e/flows/wallet_flow.py — e2e flow directly broken by the gate; payout helper must verify the seller first
  - platform-e2e/tests/e2e/support/plp_support.py — e2e support directly broken by the gate; payout helpers must verify the seller first
  - platform-e2e/tests/e2e/step_definitions/commerce_erasure_steps.py — existing e2e payout step directly broken by the gate; seller verified before payouts

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-payment --vet`
- `bash platform-core/tools/seed-marketplace.sh` twice on the env (idempotent)
- `openspec validate payment-payout-kyc-gate --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: payment/payout_kyc_gate.feature plus fintech/seller_payout, seller/fulfillment_and_payout, payment/payout_scope, payment/plp_payout_holdback, payment/seller_wallet_access
