# [W6-T1] frontend-legal-pages

## Role
feature-run worker (change `openspec/changes/frontend-legal-pages`)

## Objective
Implement OpenSpec change `frontend-legal-pages` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-frontend: seven `content/legal/<slug>.md` placeholders + reader in `src/features/legal/`, static route group `(shop)/(legal)/[slug]`, sitemap, footer legal column in `ConsumerShell.tsx`, `NEXT_PUBLIC_LEGAL_TERMS_VERSION`/`NEXT_PUBLIC_PRIVACY_POLICY_VERSION` build args equal to identity's, `PrivacyService` client + `src/lib/gateway/privacy.ts`, `RegisterForm` checkboxes sending both versions, `LegalReacceptance` dialog, Vietnamese `ConsentBanner` + `recordBannerConsent`.
- team-frontend-seller: terms link, `acceptSellerTerms` records SELLER_TERMS after BecomeSeller.
- depends_on: identity-legal-consent (wave 2)
- base: `asdlc/trust-legal` after wave 5
- hot file: `team-frontend/src/components/shell/ConsumerShell.tsx` (seller-kyc-ui wave 7, frontend-account-privacy wave 8); `docker-compose.services.yaml` (frontend build args)

## Write-set (EXCLUSIVE)
- team-frontend/content/legal/**
- team-frontend/src/features/legal/**
- team-frontend/src/app/(shop)/(legal)/**
- team-frontend/src/app/sitemap.ts
- team-frontend/src/app/robots.ts
- team-frontend/src/components/shell/ConsumerShell.tsx
- team-frontend/src/components/shell/ConsumerShell.test.tsx
- team-frontend/src/lib/gateway/client.ts
- team-frontend/src/lib/gateway/privacy.ts
- team-frontend/src/lib/gateway/privacy.test.ts
- team-frontend/src/features/auth/**
- team-frontend/src/features/consent/**
- team-frontend/Dockerfile
- team-frontend/README.md
- team-frontend/FEATURES.yaml
- team-frontend-seller/src/features/seller/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/compose/legal-frontend-stale.override.yaml
- platform-e2e/tests/e2e/features/frontend/legal_pages.feature
- platform-e2e/tests/e2e/features/frontend/legal_signup.feature
- platform-e2e/tests/e2e/features/frontend/legal_reacceptance.feature
- platform-e2e/tests/e2e/features/frontend/consent_banner_ledger.feature
- platform-e2e/tests/e2e/features/seller/seller_terms_consent.feature
- platform-e2e/tests/e2e/step_definitions/test_legal_pages.py
- platform-e2e/tests/e2e/step_definitions/test_legal_signup.py
- platform-e2e/tests/e2e/step_definitions/test_legal_reacceptance.py
- platform-e2e/tests/e2e/step_definitions/test_consent_banner_ledger.py
- platform-e2e/tests/e2e/step_definitions/test_seller_terms_consent.py
- platform-e2e/tests/e2e/step_definitions/legal_ui_steps.py
- platform-e2e/src/pages/register_page.py

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller` (`npm ci && npm run proto` first)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend`, `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend` (same for team-frontend-seller)
- `openspec validate frontend-legal-pages --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: frontend/legal_pages, legal_signup, legal_reacceptance, consent_banner_ledger, seller/seller_terms_consent
