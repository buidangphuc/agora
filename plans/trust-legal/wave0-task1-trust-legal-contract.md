# [W0-T1] trust-legal-contract

## Role
feature-run worker (change `openspec/changes/trust-legal-contract`)

## Objective
Implement OpenSpec change `trust-legal-contract` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- Additive proto contract per 00-spec Contracts: `PrivacyService` (6 RPCs), `RegisterRequest.terms_version/privacy_version`, `USER_STATUS_DELETED`, `UserErased`; `OrderService.GetUserObligations`, `PaymentService.GetPayoutObligations` (+ payout gate comments); verification `NOT_SUBMITTED` + 4 KYC RPCs and fields; audit catalogue 25 -> 26 (`kyc.document_view`).
- Pin with `trust_legal_compat_test.go`; copy the five source protos byte-identical into every consumer listed in the proposal (+ new team-verification `identity.proto`/`events.proto`), guarded by `trust_legal_consumer_copies_test.sh`.
- E2E track: none (FEATURES.yaml `not-testable` entries by the e2e-author).
- depends_on: none
- base: `asdlc/trust-legal`
- hot file: none in this wave (solo contract wave; merges first)

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/identity/v1/**
- platform-core/packages/proto/platform/order/v1/**
- platform-core/packages/proto/platform/payment/v1/**
- platform-core/packages/proto/platform/verification/v1/**
- platform-core/packages/proto/platform/audit/v1/**
- platform-core/packages/proto/gaps_contract_docs_test.go
- platform-core/packages/proto/trust_legal_consumer_copies_test.sh
- platform-core/packages/proto/Makefile
- platform-core/FEATURES.yaml
- platform-core/README.md
- team-*/proto/platform/identity/v1/identity.proto
- team-*/proto/platform/order/v1/order.proto
- team-*/proto/platform/payment/v1/payment.proto
- team-*/proto/platform/verification/v1/verification.proto
- team-*/proto/platform/audit/v1/audit.proto
- team-verification/proto/platform/events/v1/events.proto
- team-frontend*/package-lock.json
- team-frontend*/vendor/fe-platform-api-client.tgz
- fe-platform/packages/api-client/src/generated/**
- fe-platform/.changeset/**
- widened (P1):
  - platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go — pinned Q7 compat test directly broken by the new RegisterRequest terms_version/privacy_version fields and OrderService.GetUserObligations RPC

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go platform-core/packages/proto -- ./platform/identity/v1/... .`
- `cd platform-core/packages/proto && bash trust_legal_consumer_copies_test.sh`
- `make -C platform-core lint-proto breaking` (buf lint + breaking against origin/main) and the mutated-copy breaking check of task 1.6
- `buf lint && buf build` in `team-verification/proto` and in every consumer proto dir with a `buf.yaml`
- `make -C platform-core check`
