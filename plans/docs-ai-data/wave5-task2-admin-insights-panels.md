# [W5-T2] admin-insights-panels

## Role
feature-run worker (change `openspec/changes/admin-insights-panels`)

## Objective
Implement OpenSpec change `admin-insights-panels` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- Four server-rendered panels on /admin/cockpit (placements, retention, experiments, AI surfaces) over five AnalyticsQueryService RPCs, GET-form selectors with allowlisted window/weeks/range/flag, 5 s timeout, no error text to the browser.
- depends_on: analytics-experiment-reports (merged)
- base: `asdlc/docs-ai-data` after wave 4
- Generated stubs are gitignored: `npm ci && npm run proto` in the worktree.
- MINOR to fold in: page.test.tsx redirect case; align model_version wording in design.md:11 with 00-spec; fixed clock for the range scenario.

## Write-set (EXCLUSIVE)
- team-frontend-admin/src/**
- team-frontend-admin/README.md
- team-frontend-admin/FEATURES.yaml
- platform-e2e/tests/e2e/features/admin/insights_panels.feature
- platform-e2e/tests/e2e/step_definitions/test_insights_panels.py
- platform-e2e/tests/e2e/step_definitions/insights_panels_steps.py
- platform-e2e/tests/e2e/flows/tracking_flow.py
- widened (P1):
  - platform-e2e/tests/e2e/step_definitions/admin_insights_steps.py — e2e step file of this change (admin/insights_panels.feature)
  - platform-e2e/tests/e2e/step_definitions/test_admin_insights_panels.py — e2e binding of this change (admin/insights_panels.feature)

## Verify
- `cd team-frontend-admin && npm ci && npm run proto && npm run check`
- `openspec validate admin-insights-panels --strict`
- `make -C platform-e2e features-check`
- e2e: admin/insights_panels.feature (seeded through the real collector)
