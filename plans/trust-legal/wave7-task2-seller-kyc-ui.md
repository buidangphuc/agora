# [W7-T2] seller-kyc-ui

## Role
feature-run worker (change `openspec/changes/seller-kyc-ui`)

## Objective
Implement OpenSpec change `seller-kyc-ui` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-frontend-seller: `src/lib/gateway/verification.ts`, `/seller/verification` page + upload form (sequential presign, PUT, `SubmitKyc`, consent checkbox, retry), `SELLER_NAV` entry, wallet status notice and payout error mapping.
- team-frontend: delete `/account/verification` and `features/account/verification/*`, remove the `AccountShell` item and the `ConsumerShell` link, `getPublicBadges` + badge on `/shop/[id]` with failure fallback.
- E2E approves/rejects through the real `ReviewKyc`; fault toggle overlay for PUT and badge failures (no route fulfils).
- depends_on: verification-kyc-documents (wave 1), payment-payout-kyc-gate (wave 5); after frontend-legal-pages (wave 6) for ConsumerShell
- base: `asdlc/trust-legal` after wave 6
- hot file: `team-frontend/src/components/shell/ConsumerShell.tsx` and `AccountShell.tsx` (frontend-account-privacy wave 8); `platform-core/tools/seed-marketplace.sh` (after payment-payout-kyc-gate)

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- team-frontend/src/app/(shop)/account/verification/**
- team-frontend/src/features/account/verification/**
- team-frontend/src/features/account/AccountShell.tsx
- team-frontend/src/features/account/AccountShell.test.tsx
- team-frontend/src/components/shell/ConsumerShell.tsx
- team-frontend/src/components/shell/ConsumerShell.test.tsx
- team-frontend/src/lib/gateway/verification.ts
- team-frontend/src/lib/gateway/verification.test.ts
- team-frontend/src/app/(shop)/shop/**
- team-frontend/FEATURES.yaml
- platform-core/tools/seed-marketplace.sh
- platform-e2e/compose/kyc-faults.override.yaml
- platform-e2e/tests/e2e/features/seller/kyc_upload.feature
- platform-e2e/tests/e2e/features/seller/kyc_payout_notice.feature
- platform-e2e/tests/e2e/features/buyer/shop_verified_badge.feature
- platform-e2e/tests/e2e/step_definitions/test_kyc_upload.py
- platform-e2e/tests/e2e/step_definitions/test_kyc_payout_notice.py
- platform-e2e/tests/e2e/step_definitions/test_shop_verified_badge.py
- platform-e2e/tests/e2e/step_definitions/seller_kyc_ui_steps.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend` (`npm ci && npm run proto` first)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc` and `biome` for both frontends
- `openspec validate seller-kyc-ui --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: seller/kyc_upload, seller/kyc_payout_notice, buyer/shop_verified_badge
