# [W4-T1] identity-account-deletion

## Role
feature-run worker (change `openspec/changes/identity-account-deletion`)

## Objective
Implement OpenSpec change `identity-account-deletion` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-identity: migration (`deletion_requested_at`, `erase_after`, status `deleted`), config `ACCOUNT_DELETION_CANCEL_WINDOW`, `ACCOUNT_ERASURE_SWEEP_INTERVAL`, `UPSTREAM_ORDER_ADDR`, `UPSTREAM_PAYMENT_ADDR` (boot fails when missing); obligation clients (service principal, 2 s, one attempt); `RequestAccountDeletion` (password re-check, obligations, revoke other sessions in one tx), `CancelAccountDeletion`, `GetAccountDeletionStatus`; eraser tx with `FOR UPDATE SKIP LOCKED` + `UserErased` outbox; sweeper; `deleted` refused in Login/Refresh/handoff.
- team-gateway: route policy and forwarder for the three RPCs; obligation RPCs stay 501.
- Compose: identity env for the new keys; e2e overlay with a short cancel window.
- depends_on: identity-legal-consent (wave 2), commerce-user-erasure (wave 3)
- base: `asdlc/trust-legal` after wave 3
- hot file: `team-gateway/internal/edge/auth.go` (+ `policy.go`, after verification-kyc-documents wave 1); `docker-compose.services.yaml`

## Write-set (EXCLUSIVE)
- team-identity/**
- team-gateway/internal/edge/auth*.go
- team-gateway/internal/edge/privacy*.go
- team-gateway/internal/edge/policy*.go
- team-gateway/internal/app/**
- team-gateway/README.md
- docker-compose.services.yaml
- platform-e2e/compose/account-deletion.override.yaml
- platform-e2e/tests/e2e/features/identity/account_deletion.feature
- platform-e2e/tests/e2e/step_definitions/test_account_deletion.py
- platform-e2e/tests/e2e/step_definitions/account_deletion_steps.py
- openspec/changes/identity-account-deletion/design.md (widened at build, P1: the slice's own OpenSpec folder; "Clarifications" entry on admin-scope self-deletion audit (no requirement or scenario wording change))
- openspec/changes/identity-account-deletion/specs/identity-account-deletion/spec.md (widened at build, P1: the slice's own OpenSpec folder; only VERIFIED BY citations added (no requirement or scenario wording change))
- platform-e2e/compose/README.md (widened at build, P1: doc updated by docs-sync (account-deletion overlay row))
- platform-e2e/pyproject.toml (widened at build, P1: pytest marker registration (account_deletion, deletion_default, deletion_sweep_override))
- platform-e2e/tests/e2e/features/identity/account_deletion_erasure.feature (widened at build, P1: e2e feature of this change (packet named it account_deletion.feature))
- platform-e2e/tests/e2e/features/identity/account_deletion_gateway.feature (widened at build, P1: e2e feature of this change (packet named it account_deletion.feature))
- platform-e2e/tests/e2e/features/identity/account_deletion_obligations.feature (widened at build, P1: e2e feature of this change (packet named it account_deletion.feature))
- platform-e2e/tests/e2e/features/identity/account_deletion_request.feature (widened at build, P1: e2e feature of this change (packet named it account_deletion.feature))
- platform-e2e/tests/e2e/features/identity/account_deletion_sessions.feature (widened at build, P1: e2e feature of this change (packet named it account_deletion.feature))
- platform-e2e/tests/e2e/support/account_deletion_support.py (widened at build, P1: e2e support module of this change)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-identity --vet`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway --vet`
- `openspec validate identity-account-deletion --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal with `platform-e2e/compose/account-deletion.override.yaml`: identity/account_deletion.feature
