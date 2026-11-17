# [W8-T2] frontend-account-privacy

## Role
feature-run worker (change `openspec/changes/frontend-account-privacy`)

## Objective
Implement OpenSpec change `frontend-account-privacy` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-frontend `/account/privacy`: `src/lib/gateway/privacy.ts` extended (consents, deletion status, export; no user id in requests), `src/features/privacy/actions.ts` server actions, `ConsentForm`/`ConsentHistory`, `ExportButton` (`agora-du-lieu-<date>.json`), `DeletionCard` + `DeleteAccountModal`, page + loading with guest redirect and paging, menu entries in `AccountShell` and `ConsumerShell`; never log passwords or export content.
- depends_on: identity-data-export (wave 5), frontend-legal-pages (wave 6); after seller-kyc-ui (wave 7) for ConsumerShell/AccountShell
- base: `asdlc/trust-legal` after wave 7
- hot file: `team-frontend/src/components/shell/ConsumerShell.tsx`, `AccountShell.tsx` (last writer)

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/privacy.ts
- team-frontend/src/lib/gateway/privacy.test.ts
- team-frontend/src/features/privacy/**
- team-frontend/src/app/(shop)/account/privacy/**
- team-frontend/src/features/account/AccountShell.tsx
- team-frontend/src/features/account/AccountShell.test.tsx
- team-frontend/src/components/shell/ConsumerShell.tsx
- team-frontend/src/components/shell/ConsumerShell.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature
- platform-e2e/tests/e2e/step_definitions/test_account_privacy_ui.py
- platform-e2e/tests/e2e/step_definitions/account_privacy_steps.py
- platform-e2e/pyproject.toml (widened at build, P1: pytest marker registration (account_privacy_ui))
- platform-e2e/src/pages/privacy_page.py (widened at build, P1: e2e page object of this change)

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend` (`npm ci && npm run proto` first)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend`
- `openspec validate frontend-account-privacy --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: frontend/account_privacy_ui.feature
