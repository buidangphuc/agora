# [W6-T1] seller-forecast-card

## Role
feature-run worker (change `openspec/changes/seller-forecast-card`)

## Objective
Implement OpenSpec change `seller-forecast-card` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- /seller/analytics forecast card for the top 5 listings of the range (14-day GetDemandForecast, `Promise.allSettled`, three states + empty), seller id from the principal only; Magic Listing beacons carry `request_id`/`model_version` provenance.
- depends_on: analytics-forecast-serving, ai-copilot-grounding (merged)
- base: `asdlc/docs-ai-data` after wave 5
- Generated stubs are gitignored: `npm ci && npm run proto` in the worktree.

## Write-set (EXCLUSIVE)
- team-frontend-seller/src/**
- team-frontend-seller/README.md
- team-frontend-seller/FEATURES.yaml
- team-analytics/FEATURES.yaml
- platform-e2e/tests/e2e/features/seller/seller_forecast_card.feature
- platform-e2e/tests/e2e/step_definitions/test_seller_forecast_card.py
- platform-e2e/tests/e2e/step_definitions/seller_forecast_card_steps.py
- platform-e2e/tests/e2e/step_definitions/seller_analytics_steps.py
Widened (P1):
  - platform-e2e/src/pages/seller_analytics_page.py — e2e page object of this change (seller/seller_forecast_card.feature)
  - openspec/changes/seller-forecast-card/specs/seller-demand-forecast-card/spec.md — own OpenSpec folder: VERIFIED BY citations matched to real e2e scenario names
  - openspec/changes/seller-forecast-card/specs/seller-magic-listing-provenance/spec.md — own OpenSpec folder: VERIFIED BY citation matched to real e2e scenario name
  - openspec/changes/seller-forecast-card/tasks.md — own OpenSpec folder: task 2.1 scenario names matched to the real feature

## Verify
- `cd team-frontend-seller && npm ci && npm run proto && npm run check`
- `openspec validate seller-forecast-card --strict`
- `make -C platform-e2e features-check`
- e2e: seller/seller_forecast_card.feature (forecast profile job output; store-down scenario is destructive)
