# [W2-T2] admin-kyc-queue

## Role
feature-run worker (change `openspec/changes/admin-kyc-queue`)

## Objective
Implement OpenSpec change `admin-kyc-queue` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-frontend-admin: `tab=kyc` on `/admin/cases` ("Xác minh người bán"), `verification` client in `client.ts`, `src/lib/gateway/kyc.ts` (5 s timeout, never throws, Connect code mapping, no URL/id in logs), shared reason-code list + Vietnamese labels, `kycActions.ts` server actions, `KycQueueView.tsx`, `KycRejectDialog.tsx`, page branch with guest redirect / 403 / unavailable.
- E2E creates pending submissions through the real seller RPCs (reuse `flows/kyc_flow.py` read-only), audit query on `kyc.document_view`, audit-down fail-closed case, two-admin race and self-review.
- depends_on: verification-kyc-documents (wave 1; also uses trust-legal-contract stubs: `npm ci && npm run proto`)
- base: `asdlc/trust-legal` after wave 1
- hot file: none (team-frontend-admin is touched by no other slice)

## Write-set (EXCLUSIVE)
- team-frontend-admin/src/**
- team-frontend-admin/README.md
- team-frontend-admin/FEATURES.yaml
- platform-e2e/tests/e2e/features/admin/kyc_queue.feature
- platform-e2e/tests/e2e/step_definitions/test_admin_kyc_queue.py
- platform-e2e/tests/e2e/step_definitions/admin_kyc_queue_steps.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-admin`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend-admin` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend-admin`
- `openspec validate admin-kyc-queue --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: admin/kyc_queue.feature
