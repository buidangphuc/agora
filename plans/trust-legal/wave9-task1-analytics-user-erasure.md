# [W9-T1] analytics-user-erasure

## Role
feature-run worker (change `openspec/changes/analytics-user-erasure`)

## Objective
Implement OpenSpec change `analytics-user-erasure` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-analytics: erasure config (`IDENTITY_EVENTS_TOPIC`, `ERASURE_CONSUMER_GROUP`, `ERASURE_DLQ_TOPIC`, `ERASURE_TOMBSTONE_SECRET`), DuckDB `erased_users` tombstone, `internal/events/erasure.go`, one-tx purge/re-key, writers re-key tombstoned users, export files clean after one cycle.
- platform-recsys `recsys.erasure` consumer (Redis + Qdrant deletes); platform-featurestore inputs exclude `erased:` keys and online reconcile in `materialize`; team-ai erasure addon (DLQ `identity.events.team-ai.dlq`).
- Compose: three DLQs, consumer env, recsys erasure service (non-root).
- depends_on: trust-legal-contract (wave 0)
- base: `asdlc/trust-legal` after wave 8
- hot file: `docker-compose.services.yaml` (only compose writer in this wave)

## Write-set (EXCLUSIVE)
- team-analytics/**
- platform-recsys/**
- platform-featurestore/**
- team-ai/**
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/analytics/user_erasure.feature
- platform-e2e/tests/e2e/step_definitions/test_analytics_user_erasure.py
- platform-e2e/tests/e2e/step_definitions/analytics_erasure_steps.py
- openspec/changes/analytics-user-erasure/specs/analytics-warehouse-erasure/spec.md (widened at build, P1: VERIFIED BY citations matching the real test names (no requirement/scenario wording change))
- openspec/changes/analytics-user-erasure/specs/ml-stores-erasure/spec.md (widened at build, P1: VERIFIED BY citations matching the real test names (no requirement/scenario wording change))
- platform-core/FEATURES.yaml (widened at build, P1: entries core.aue-33 and core.aue-34 (user decision 2026-11-15))
- platform-e2e/tests/e2e/features/analytics/user_erasure_stores.feature (widened at build, P1: e2e feature file of this change, named differently from the packet)
- platform-e2e/tests/e2e/step_definitions/test_analytics_user_erasure_stores.py (widened at build, P1: e2e step file of this change, named differently from the packet)
- platform-e2e/tests/e2e/support/analytics_erasure_support.py (widened at build, P1: e2e support file of this change, named differently from the packet)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-analytics --vet`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py pytest platform-recsys -- -q tests/` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py pytest platform-featurestore -- -q tests/` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py pytest team-ai -- -q`
- `openspec validate analytics-user-erasure --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: analytics/user_erasure.feature (warehouse re-key, redelivery/poison, Redis+Qdrant erasure, late event)
