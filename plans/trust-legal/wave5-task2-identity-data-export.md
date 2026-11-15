# [W5-T2] identity-data-export

## Role
feature-run worker (change `openspec/changes/identity-data-export`)

## Objective
Implement OpenSpec change `identity-data-export` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-identity: `ExportMyData` (`auth.RequireUser` first, read-only tx, named columns, newest first, 1000-row caps with truncated flags, size shrink loop to <= 1 MiB, counts-only log), store export methods, PG tests incl. p95 and 1000/1001 boundary, authz coverage as user-only.
- team-gateway: forwarder method with read-retry and `Cache-Control: no-store`, anonymous refused before forward (`privacy_export_test.go` red first).
- depends_on: identity-account-deletion (wave 4)
- base: `asdlc/trust-legal` after wave 4
- hot file: `team-gateway/internal/edge/auth.go` (last identity writer)

## Write-set (EXCLUSIVE)
- team-identity/**
- team-gateway/internal/edge/auth*.go
- team-gateway/internal/edge/privacy*.go
- team-gateway/README.md
- platform-e2e/tests/e2e/features/identity/data_export.feature
- platform-e2e/tests/e2e/step_definitions/test_data_export.py
- platform-e2e/tests/e2e/step_definitions/data_export_steps.py
- widened (P1):
  - openspec/changes/identity-data-export/tasks.md — the slice's own OpenSpec folder; e2e task ticks 2.1 and 2.2 only
  - platform-e2e/pyproject.toml — pytest marker registration (data_export)
  - platform-e2e/tests/e2e/support/data_export_support.py — e2e support module of this change

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-identity --vet`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway --vet`
- `openspec validate identity-data-export --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal (only team-identity, team-gateway, Postgres, Kafka up): identity/data_export.feature
