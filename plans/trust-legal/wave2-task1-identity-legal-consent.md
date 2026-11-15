# [W2-T1] identity-legal-consent

## Role
feature-run worker (change `openspec/changes/identity-legal-consent`)

## Objective
Implement OpenSpec change `identity-legal-consent` test-first per its tasks.md; context in plans/trust-legal/00-spec.md.
- team-identity: `LegalVersions` config (`LEGAL_TERMS_VERSIONS`, `PRIVACY_POLICY_VERSIONS`), migration `0022_consents`, `AppendConsents`/`ListConsents`, `Register` validates versions and writes two consent rows in the same tx, `PrivacyHandler` (`RecordConsent`, `ListMyConsents`; other four UNIMPLEMENTED), user-principal authz rule.
- team-gateway: `PrivacyForwarder` in `internal/edge/auth.go` for the two RPCs, wired in the gateway app.
- Every existing `Register` caller (platform-e2e helpers, seed scripts, compose seed) sends both current versions.
- depends_on: trust-legal-contract (wave 0)
- base: `asdlc/trust-legal` after wave 1
- hot file: `team-gateway/internal/edge/auth.go` (identity-account-deletion wave 4, identity-data-export wave 5); `docker-compose.services.yaml`; `platform-core/tools/seed-marketplace.sh` (payment-payout-kyc-gate wave 5, seller-kyc-ui wave 7)

## Write-set (EXCLUSIVE)
- team-identity/**
- team-gateway/internal/edge/auth*.go
- team-gateway/internal/edge/privacy*.go
- team-gateway/internal/app/**
- team-gateway/README.md
- docker-compose.services.yaml
- platform-core/tools/seed-marketplace.sh
- platform-core/tools/seed-full-marketplace.sh
- platform-core/tools/seed_full_marketplace.py
- platform-core/tools/seed-full.sh
- platform-core/tools/seed/**
- platform-e2e/compose/legal-versions.override.yaml
- platform-e2e/tests/e2e/features/identity/legal_consent.feature
- platform-e2e/tests/e2e/step_definitions/test_legal_consent.py
- platform-e2e/tests/e2e/step_definitions/legal_consent_steps.py
- platform-e2e/src/api/services/auth_service.py
- platform-e2e/src/constants/gateway_endpoints.py
- platform-e2e/tests/e2e/support/world.py
- platform-e2e/tests/e2e/support/tii_support.py
- platform-e2e/tests/e2e/support/d2_support.py
- platform-e2e/tests/e2e/support/new_user_voucher_support.py
- platform-e2e/tests/e2e/support/oic_order_support.py
- platform-e2e/tests/e2e/support/user_enforcement_support.py
- platform-e2e/tests/e2e/flows/seed_listing_flow.py
- platform-e2e/tests/e2e/flows/auth_flow.py
- platform-e2e/tests/e2e/step_definitions/register_steps.py
- platform-e2e/tests/e2e/step_definitions/seller_analytics_access_steps.py
- platform-e2e/tests/e2e/step_definitions/c1_steps.py
- platform-e2e/tests/e2e/step_definitions/identity_q7_steps.py
- platform-e2e/tests/e2e/step_definitions/notification_alert_steps.py
- platform-e2e/tests/e2e/step_definitions/test_new_services.py
- platform-e2e/tests/e2e/step_definitions/prm_rma_steps.py
- platform-e2e/tests/e2e/step_definitions/plp_steps.py
- platform-e2e/tests/e2e/step_definitions/test_order_receipt_completion.py
- platform-e2e/tests/e2e/step_definitions/session_continuity_steps.py
- platform-e2e/tests/e2e/step_definitions/referral_reward_steps.py
- widened (P1):
  - openspec/changes/identity-legal-consent/specs/identity-consent/spec.md — the slice's own OpenSpec folder; only VERIFIED BY citations set to the real legal_consent.feature scenario names (no requirement or scenario wording change)
  - platform-e2e/pyproject.toml — pytest marker registration (legal_consent, legal_versions_override)
  - platform-e2e/src/constants/legal_versions.py — e2e support constant of this change; current Terms/Privacy versions every Register body now carries
  - platform-e2e/tests/e2e/step_definitions/listing_moderation_ui_steps.py — existing e2e step directly broken by the new rule (Register requires terms_version/privacy_version)
  - platform-e2e/tests/e2e/step_definitions/referral_invite_steps.py — existing e2e step directly broken by the new rule (Register requires terms_version/privacy_version)
  - platform-e2e/tests/e2e/step_definitions/session_client_context_steps.py — existing e2e step directly broken by the new rule (Register requires terms_version/privacy_version)
  - platform-e2e/tests/e2e/step_definitions/session_refresh_steps.py — existing e2e step directly broken by the new rule (Register requires terms_version/privacy_version)
  - platform-e2e/tests/e2e/step_definitions/sso_handoff_steps.py — existing e2e step directly broken by the new rule (Register requires terms_version/privacy_version)
- widened (user-approved 2026-11-15):
  - team-gateway/internal/edge/server.go — PrivacyService mux registration needed by the forwarder
  - team-gateway/internal/upstream/clients.go — PrivacyService client dial needed by the forwarder

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-identity --vet`
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway --vet` (stubs: `buf generate --template buf.gen.local.yaml`)
- `openspec validate identity-legal-consent --strict`
- e2e (pytest-bdd through gate_digest.py pytest platform-e2e) against env trust-legal: identity/legal_consent.feature plus identity/register_referral.feature and auth/session_continuity.feature (Register callers still green)
