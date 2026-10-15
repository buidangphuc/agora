# [W2-T3] frontend-audit-viewer

## Role
feature-run worker (change `openspec/changes/frontend-audit-viewer`)

## Objective
Implement OpenSpec change `frontend-audit-viewer` test-first per its tasks.md (scenario ids there); context in plans/q3-audit-log/00-spec.md and the change's design.md Clarifications.
- Admin audit viewer page at /admin/audit plus the cockpit link; regenerates (never edits) the audit TS stubs from the vendored proto.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/client.ts
- team-frontend/src/lib/gateway/audit.ts
- team-frontend/src/lib/gateway/audit.test.ts
- team-frontend/src/features/admin/auditFilters.ts
- team-frontend/src/features/admin/auditFilters.test.ts
- team-frontend/src/features/admin/AuditLogView.tsx
- team-frontend/src/features/admin/AuditLogView.test.tsx
- team-frontend/src/app/(shop)/admin/audit/**
- team-frontend/src/features/admin/CockpitView.tsx
- team-frontend/src/features/admin/CockpitView.test.tsx
- team-frontend/src/app/(shop)/admin/cockpit/page.test.tsx
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/frontend/audit_viewer_ui.feature
- platform-e2e/tests/e2e/step_definitions/audit_viewer_steps.py
- platform-e2e/tests/e2e/step_definitions/test_audit_viewer_ui.py

## Read-only dependencies
- openspec/changes/frontend-audit-viewer/** (frozen by LOCK.md digest)
- team-frontend/proto/platform/audit/v1/audit.proto (vendored by q3-audit-log-contract; `npm run proto` regenerates gitignored `src/generated/platform/audit/**`)
- platform-e2e/tests/e2e/support/audit_support.py (audit-query-filters output; import only, never edit)
- team-frontend/src/app/(shop)/admin/cockpit/page.tsx (gate order to mirror)
- plans/q3-audit-log/00-spec.md, plans/q3-audit-log/slices.yaml, plans/q3-audit-log/LOCK.md

## Verify
(cd team-frontend && npm run proto && npx vitest run src/lib/gateway/audit.test.ts src/features/admin/auditFilters.test.ts src/features/admin/AuditLogView.test.tsx 'src/app/(shop)/admin/audit' src/features/admin/CockpitView.test.tsx 'src/app/(shop)/admin/cockpit/page.test.tsx' && npx tsc --noEmit && npm run check); (cd platform-e2e && HEADLESS=true /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python -m pytest -q tests/e2e/features/frontend/audit_viewer_ui.feature); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py frontend-audit-viewer --strict
