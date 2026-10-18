# [W3-T1] frontend-listing-moderation

## Role
feature-run worker (change `openspec/changes/frontend-listing-moderation`)

## Objective
Implement OpenSpec change `frontend-listing-moderation` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- Admin moderation queue (/admin/listings) and cases inbox with reports (/admin/cases), buyer report button on the PDP, seller status tags and moderation banner (no screen flags on seller surfaces).
- depends_on: domain-listing-moderation, engagement-listing-reports

## Write-set (EXCLUSIVE)
- team-frontend/src/generated/platform/**
- team-frontend/src/lib/gateway/listings.ts
- team-frontend/src/lib/gateway/listings.test.ts
- team-frontend/src/features/seller/status.tsx
- team-frontend/src/features/seller/status.test.tsx
- team-frontend/src/features/listing/ModerationBanner.tsx
- team-frontend/src/features/listing/ModerationBanner.test.tsx
- team-frontend/src/features/listing/reasonLabels.ts
- team-frontend/src/app/(shop)/seller/[id]/edit/page.tsx
- team-frontend/src/features/listing/ListingForm.tsx
- team-frontend/src/features/listing/actions.ts
- team-frontend/src/features/listing/actions.test.ts
- team-frontend/src/lib/gateway/moderation.ts
- team-frontend/src/lib/gateway/moderation.test.ts
- team-frontend/src/features/admin/moderationActions.ts
- team-frontend/src/features/admin/moderationActions.test.ts
- team-frontend/src/lib/gateway/reports.ts
- team-frontend/src/lib/gateway/reports.test.ts
- team-frontend/src/features/admin/reportActions.ts
- team-frontend/src/features/admin/reportActions.test.ts
- team-frontend/src/features/listing/reportActions.ts
- team-frontend/src/features/listing/reportActions.test.ts
- team-frontend/src/features/admin/ModerationQueueView.tsx
- team-frontend/src/features/admin/ModerationQueueView.test.tsx
- team-frontend/src/features/admin/ModerateDialog.tsx
- team-frontend/src/features/admin/ModerateDialog.test.tsx
- team-frontend/src/features/admin/ReportsView.tsx
- team-frontend/src/features/admin/ReportsView.test.tsx
- team-frontend/src/features/admin/CloseReportDialog.tsx
- team-frontend/src/features/admin/CloseReportDialog.test.tsx
- team-frontend/src/features/admin/AdminCasesTabs.tsx
- team-frontend/src/features/admin/AdminCasesTabs.test.tsx
- team-frontend/src/features/admin/casesParams.ts
- team-frontend/src/features/admin/casesParams.test.ts
- team-frontend/src/app/(shop)/admin/listings/**
- team-frontend/src/app/(shop)/admin/cases/**
- team-frontend/src/features/listing/ReportListingButton.tsx
- team-frontend/src/features/listing/ReportListingButton.test.tsx
- team-frontend/src/app/(shop)/listing/[id]/page.tsx
- team-frontend/src/app/(shop)/listing/[id]/page.test.tsx
- team-frontend/src/features/admin/CockpitView.tsx
- team-frontend/src/features/admin/CockpitView.test.tsx
- team-frontend/src/components/shell/ConsumerShell.tsx
- team-frontend/src/components/shell/ConsumerShell.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/admin_listing_moderation_ui.feature
- platform-e2e/tests/e2e/step_definitions/admin_listing_moderation_steps.py
- platform-e2e/tests/e2e/step_definitions/test_admin_listing_moderation_ui.py
- platform-e2e/tests/e2e/features/frontend/admin_cases_ui.feature
- platform-e2e/tests/e2e/step_definitions/admin_cases_steps.py
- platform-e2e/tests/e2e/step_definitions/test_admin_cases_ui.py
- platform-e2e/tests/e2e/features/frontend/listing_report_ui.feature
- platform-e2e/tests/e2e/step_definitions/listing_report_steps.py
- platform-e2e/tests/e2e/step_definitions/test_listing_report_ui.py
- platform-e2e/tests/e2e/features/frontend/seller_listing_status_ui.feature
- platform-e2e/tests/e2e/step_definitions/seller_listing_status_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_listing_status_ui.py
- openspec/changes/frontend-listing-moderation/**

## Read-only dependencies
- openspec/changes/frontend-listing-moderation/** is frozen by the LOCK.md digest (only task ticks change)
- team-frontend/proto/** (copied by q6-trust-contract; never edit)
- platform-e2e/tests/e2e/support/audit_support.py, listing_moderation_support.py, report_support.py (reuse)
- team-frontend/src/app/guards.test.ts (must stay green)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-frontend && npm run proto && npm test && npm run check); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_admin_listing_moderation_ui.py tests/e2e/step_definitions/test_admin_cases_ui.py tests/e2e/step_definitions/test_listing_report_ui.py tests/e2e/step_definitions/test_seller_listing_status_ui.py -m 'not destructive') then the @destructive audit-down scenarios alone; $PY platform-e2e/scripts/spec_sync.py frontend-listing-moderation --strict
