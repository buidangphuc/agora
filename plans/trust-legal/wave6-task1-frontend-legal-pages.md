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
- openspec/changes/frontend-legal-pages/specs/frontend-legal-pages/spec.md (widened at build, P1: own OpenSpec folder: VERIFIED BY citations only)
- openspec/changes/frontend-legal-pages/tasks.md (widened at build, P1: own OpenSpec folder: task ticks)
- platform-e2e/pyproject.toml (widened at build, P1: pytest marker registration for this change's features)
- platform-e2e/tests/e2e/step_definitions/frontend_legal_steps.py (widened at build, P1: e2e step file of this change (named differently from packet))
- platform-e2e/tests/e2e/step_definitions/seller_terms_consent_steps.py (widened at build, P1: e2e step file of this change (named differently from packet))
- platform-e2e/tests/e2e/support/legal_ui_support.py (widened at build, P1: e2e support file of this change)
- platform-e2e/src/pages/admin_campaigns_page.py (widened at build, P1: page object broken by Vietnamese banner title)
- platform-e2e/src/pages/admin_moderation_page.py (widened at build, P1: page object broken by Vietnamese banner title)
- platform-e2e/src/pages/admin_vouchers_page.py (widened at build, P1: page object broken by Vietnamese banner title)
- platform-e2e/tests/e2e/features/tracking/consent.feature (widened at build, P1: existing e2e broken by Vietnamese banner labels)
- platform-e2e/tests/e2e/features/tracking/ga4_destination.feature (widened at build, P1: existing e2e broken by Vietnamese banner labels)
- platform-e2e/tests/e2e/step_definitions/consent_steps.py (widened at build, P1: existing steps broken by Vietnamese banner labels)
- platform-e2e/tests/e2e/step_definitions/ga4_destination_steps.py (widened at build, P1: existing steps broken by Vietnamese banner labels)
- team-frontend/src/features/tracking/AnalyticsProvider.test.tsx (widened at build, P1: existing unit test broken by Vietnamese banner labels)

## Verify
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py vitest team-frontend-seller` (`npm ci && npm run proto` first)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py tsc team-frontend`, `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py biome team-frontend` (same for team-frontend-seller)
- `openspec validate frontend-legal-pages --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: frontend/legal_pages, legal_signup, legal_reacceptance, consent_banner_ledger, seller/seller_terms_consent
