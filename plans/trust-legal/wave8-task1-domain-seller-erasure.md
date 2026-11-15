# [W8-T1] domain-seller-erasure

## Role
feature-run worker (change `openspec/changes/domain-seller-erasure`)

## Objective
Implement OpenSpec change `domain-seller-erasure` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-domain: `PgTxWriter.EraseSellerListings` (dedupe in `processed_events`, batch `FOR UPDATE`, variants, delete, one `ListingChanged` DELETED outbox row per listing, storefront/bundles/`seller_status_applied` deleted, one commit), `ErasureConsumer` in `internal/events/erasure.go`, `KAFKA_ERASURE_GROUP` (default `team-domain-erasure`), DLQ `identity.events.team-domain.dlq`; `DeleteListing` owner check unchanged.
- E2E produces `UserErased` through the real identity deletion path (identity-account-deletion landed in wave 4).
- depends_on: trust-legal-contract (wave 0)
- base: `asdlc/trust-legal` after wave 7
- hot file: `docker-compose.services.yaml` (only compose writer in this wave)

## Write-set (EXCLUSIVE)
- team-domain/**
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/domain/seller_erasure.feature
- platform-e2e/tests/e2e/step_definitions/test_seller_erasure.py
- platform-e2e/tests/e2e/step_definitions/seller_erasure_steps.py
- widened (P1):
  - openspec/changes/domain-seller-erasure/specs/domain-seller-erasure/spec.md — VERIFIED BY citations replaced with real test names (no requirement/scenario wording change)
  - openspec/changes/domain-seller-erasure/tasks.md — task ticks for this change's own OpenSpec folder
  - platform-e2e/tests/e2e/support/seller_erasure_support.py — e2e support file of this change, named differently from the packet

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-domain --vet`
- `openspec validate domain-seller-erasure --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: domain/seller_erasure.feature
