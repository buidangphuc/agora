# [W1-T1] verification-kyc-documents

## Role
feature-run worker (change `openspec/changes/verification-kyc-documents`)

## Objective
Implement OpenSpec change `verification-kyc-documents` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-verification: MinIO `internal/objects`, migration `002_kyc_documents`, `CreateKycUploadUrl`, `SubmitKyc`, `GetVerificationStatus` (`NOT_SUBMITTED`, service `verification.read`), `ReviewKyc` reason codes, `ListKycSubmissions`, audit-first `GetKycDocumentUrl`, `GetPublicBadges`, 90-day retention sweeper, `UserErased` consumer (DLQ `identity.events.team-verification.dlq`).
- team-gateway: four forwarders in `verification.go`, `adminProcedures` + pinned-set test in `policy.go`, anonymous `GetPublicBadges`.
- Compose: `kyc-documents` bucket + encryption in MinIO init, DLQ topic, team-verification env/depends_on.
- depends_on: trust-legal-contract (wave 0)
- base: `asdlc/trust-legal` after wave 0
- hot file: `docker-compose.services.yaml` (only compose writer in this wave); `team-gateway/internal/edge/policy.go` (identity-account-deletion edits it in wave 4)

## Write-set (EXCLUSIVE)
- team-verification/**
- team-gateway/internal/edge/verification*.go
- team-gateway/internal/edge/policy*.go
- team-gateway/README.md
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/verification/kyc_documents.feature
- platform-e2e/tests/e2e/step_definitions/test_kyc_documents.py
- platform-e2e/tests/e2e/step_definitions/kyc_documents_steps.py
- platform-e2e/tests/e2e/flows/kyc_flow.py
- platform-e2e/tests/e2e/features/services/verification_access.feature
- platform-e2e/tests/e2e/features/services/verification_kyc_audit.feature
- platform-e2e/tests/e2e/step_definitions/verification_access_steps.py
- platform-e2e/tests/e2e/step_definitions/verification_kyc_audit_steps.py
- platform-e2e/tests/e2e/step_definitions/test_verification_access.py
- platform-e2e/tests/e2e/step_definitions/test_verification_kyc_audit.py

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-verification --vet`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway --vet` (generate stubs first: `buf generate --template buf.gen.local.yaml`)
- `openspec validate verification-kyc-documents --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: verification/kyc_documents.feature, services/verification_access.feature, services/verification_kyc_audit.feature
