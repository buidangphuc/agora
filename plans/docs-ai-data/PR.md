## Summary

Initiative `docs-ai-data`: 10 OpenSpec slices that put real data behind the AI and analytics surfaces (seller demand forecast, recs experiment arms, grounded assistant and copilot, admin insight reports). Plan and lock: `plans/docs-ai-data/` (LOCK re-approved 2026-11-16, all 10 digests match).

```mermaid
flowchart LR
  C[docs-ai-data-contract<br/>platform-core proto] --> AFS
  FNJ[forecast-nightly-job<br/>platform-forecast] --> AFS[analytics-forecast-serving<br/>team-analytics]
  AFS --> AER[analytics-experiment-reports<br/>team-analytics]
  C --> AER
  RXA[recs-experiment-arms<br/>team-ai] --> AAG[ai-assistant-grounding<br/>team-ai]
  C --> AAG --> ACG[ai-copilot-grounding<br/>team-ai]
  AER --> AIP[admin-insights-panels<br/>team-frontend-admin]
  AFS --> SFC[seller-forecast-card<br/>team-frontend-seller]
  ACG --> SFC
  AAG --> BUI[buyer-ai-surfaces-ui<br/>team-frontend]
  ACG --> BUI
  AER --> BUI
```

Data path of the seller forecast:

```text
forecast-job (nightly, python -m forecast run)
  read order_facts.parquet (analytics export volume, read-only)
  fit / select model per listing -> p10/p50/p90
  write Redis DB 3  fc:v1:seller:<seller_id>:<listing_id>  (TTL 48 h) + fc:v1:meta
team-analytics GetDemandForecast
  validate ids -> one Redis read (FORECAST_READ_TIMEOUT_MS)
  no key -> is_cold_start; bad/slow store -> Unavailable
  reorder_point / safety_stock from P50/P90 over lead time
team-gateway -> team-frontend-seller ForecastCard
```

Contract additions (`platform-core/packages/proto`):

```diff
 service AnalyticsQueryService {
   rpc GetDemandForecast(...)
+  rpc GetRetentionCohorts(...)
+  rpc GetExperimentReadout(...)
+  rpc GetAiSurfaceReport(...)
 }
 platform/ai/v1/ai.proto
+  provenance fields (request_id, model_version) on assistant / copilot replies
```

Per repo:

```text
platform-core/        ai.proto, analytics.proto, compat tests, ADR-0013 status
platform-forecast/    nightly LightGBM/baseline job -> Redis DB 3 (new compose service forecast-job, gitops CronJob 45 2 * * *)
team-analytics/       forecaststore (go-redis), GetDemandForecast, retention / readout / AI-surface reports
team-ai/              Flipt experiment arms on home_feed, tier/ranker/arm in model_version,
                      assistant grounded on search + GetListing, copilot grounded on listing stock
team-gateway/         admin procedures for the 3 reports, exposure beacon policy, analytics forwarding
team-frontend/        assistant chat + home block + review summary provenance, experiment exposure beacon
team-frontend-seller/ ForecastCard on /seller/analytics, Magic Listing provenance beacons
team-frontend-admin/  retention, experiments, placements, AI-surface panels on /admin/cockpit
fe-platform/          api-client regenerated (analytics), changeset; apps re-vendor the tarball
platform-e2e/         56 e2e files: features, steps, recsys/forecast fixtures
```

## Evidence

- **Before:** e2e suite written red first per slice (`test(e2e): <slice> red` commits); FEATURES entries `planned`.
  **After:** 70 FEATURES entries flipped to `automated` after their scenarios passed on env `docs-ai-data`; renamed modules re-ran green (2 + 33 passed, b881d721f).
- **rgp module re-run after the converge decisions (env docs-ai-data):** 7/8. All 7 scenarios pass; 1 error at teardown:
  `platform-e2e/tests/e2e/step_definitions/test_rgp_generation_publish.py::test_serving_follows_a_promotion_within_seconds[chromium]`
  (`rgp_restore_serving` left no serving generation: the tooling recsys seed data shape is rejected by the structural gate, or skipped for having no holdout). Fixed by decision 8 (agora_env.py seeds recsys data) in the tooling PR.
- **Local CI (base 06c44c17, head of this branch):**

| check | result | evidence |
|---|---|---|
| ci_local specs | pass | openspec validate --strict, spec_lint (warnings only), spec_sync, cross_change_check |
| ci_local features | pass | `make -C platform-e2e features-check` |
| ci_local repo | pass | repo_doctor, validate_plan (known exceptions below), asdlc_config, check_gates |
| ci_local proto | pass | buf lint + buf breaking vs base |
| ci_local tests-asdlc | pass | pytest scripts/asdlc/tests, guard and red_first shell tests |
| ci_local tdd red_first (go, ts) | pass | every Go and TS test red at base and green at head, or exempt with a trailer |
| ci_local tdd red_first (python) | tooling red | runs team-ai / platform-forecast tests with the platform-e2e venv, which lacks their deps; same suites green with their own venvs (rows below). Fixed by the tooling PR (decision 16) |
| ci_local tdd red_first e2e order | fixed by chore/asdlc-tooling | first impl commit 2f9e3f1e (vendored proto copies) precedes the first e2e commit; is_contract widening is decision 10 |
| ci_local security | not run | gitleaks / trivy not installed locally |
| team-ai ruff format, ruff check, check_env_example | pass | 322 files formatted, all checks passed, .env.example in sync |
| team-ai pytest + coverage | pass | 1292 passed, coverage 96.69% (gate 80%) |
| team-ai eval gate | pass | 7 cases, 5 scored, pass_rate 1.0, 2 judge cases skipped |
| team-ai core-only import | pass | `uv sync --frozen --no-dev` + `import main` |
| team-ai pyright | pre-existing red | 2 errors in tests/unit/transport/test_search_servicer.py:267,282 (file and deps untouched by this branch) |
| team-ai real Redis tests | not run | needs a Redis on localhost:6379 (CI service container) |
| platform-forecast pytest | pass | 22 passed |
| Go team-analytics, team-gateway, team-domain, team-order | pass | go vet + go test |
| Go team-search | pass (db-tests skipped) | 65 skipped Docker-backed tests; only a proto copy changed |
| Go team-notification | pre-existing red | TestDockerRequiredNeverSkips needs GO_SDK_REQUIRE_DOCKER; only a proto copy changed |
| Go platform-core/packages/proto | pre-existing red | checkout_compat_test.go:162 pin 17 vs 18 (decision 11, other branch), audit catalogue heading (25 vs 17), Q6Gate tests need a local `main` ref; none of these protos are touched here |
| team-frontend, -seller, -admin | pass | npm ci, proto, check (biome, tsc, tokens, vitest), build |
| fe-platform | pass | check (incl. vendor:check), template build, release:status |
| platform-gitops unittest | pass | 111 tests OK |

## Merge Danger

**Door:** two-way

Additive proto fields and RPCs (buf breaking passes), new optional env vars with safe defaults (empty `FORECAST_REDIS_URL` -> `Unavailable`, empty `RECS_FLIPT_URL` -> experiments off, `ASSISTANT_BACKEND` / `COPILOT_BACKEND` default `mock`). Reverting the merge restores the old behaviour; Redis DB 3 keys expire after 48 h.

**Blast Radius:** cross-service

- Recs `model_version` format changes to `<model_version>/<tier>/<ranker>/<arm>`; any consumer that parses it (analytics, dashboards) sees the new shape.
- team-ai now depends on team-search and team-domain at boot (compose `depends_on`) and needs a service principal with `listing.read` and `search:read` in every environment.
- Assistant reply streaming route `POST /api/assistant/stream` is removed from team-frontend.
- The three apps re-vendor `fe-platform-api-client.tgz`; a stale tarball breaks their build.
- Hot files shared with the seller-centre / trust-legal / tooling PRs: `docker-compose.services.yaml`, `scripts/asdlc/red_first.sh` (529796f42, BSR plugin drop), `scripts/devenv/agora_env.py` (FLIPT_CONTAINER export), `.github/workflows/services-ci.yml`. Expect merge conflicts with the tooling PR.

## Needs human

### Deploy order
1. platform-core contract (proto) and fe-platform api-client release.
2. platform-forecast job (Redis DB 3, CronJob after the analytics export) — confirm DB 3 is free (LOCK open issue 1).
3. team-analytics with `FORECAST_REDIS_URL` / `FORECAST_READ_TIMEOUT_MS`.
4. team-ai with `RECS_FLIPT_URL`, `RECS_EXPERIMENT_SALT` (required outside dev/local/test), `ASSISTANT_LISTING_ADDR`, and the `listing.read` + `search:read` principal accepted by team-domain and team-search.
5. Flipt flag `recs-home-feed-ranker` per environment (only the local seed script creates it).
6. team-gateway, then team-frontend, team-frontend-seller, team-frontend-admin.

### Privacy / DPO (from LOCK.md and the slices' design.md)
- U6 consent basis: aggregate reports count all first-party events without `consent_analytics` filtering (contract, reports, recs entity hash, admin panels, seller beacons).
- Experiment readouts have no small-sample suppression threshold; admin-only access and aggregate shape are the only mitigation.
- Retention cohorts anchor on authenticated user ids only; anonymous sign-ups fall outside every cohort.
- Beacon retention and consent basis for `impressionId` / `modelVersion`.
- Recs experiment assignment sends user / anonymous ids to Flipt only as a salted SHA-256 hash; salt management per environment.
- Anonymous visitors get no flag variant (server-readable anonymous-id cookie would raise a consent and Secure-cookie question).

### Red-First-Exempt commits (review the reasons)
- 0f80611a5 team-frontend/src/features/assistant/actions.test.ts — regression guard
- e5bfe33cd seller-forecast-card guest/no-scope guard tests green at base
- 67ad44fed seller-forecast-card FEATURES.yaml metadata only
- 8e4256143 admin-insights-panels formatting only
- 08cc7ddff analytics-experiment-reports lint/format only
- aa74dc4c5 team-gateway/internal/edge/analytics_test.go — wrong int64 JSON expectation corrected
- b87cafaef team-ai test_flag_trim.py, test_team_ai_runtime.py — follow config
- 751ab9dd7 team-ai test_golden.py — follows the new factory signature
- 7baa0907c platform-forecast test_forecast_pipeline.py — regression guard

### LOCK re-approvals
- Batch re-approval 2026-11-16 (decision 5) covers all 10 slices; digests verified at this head. Two scenario titles were renamed (ai-copilot-grounding, admin-insights-panels) to match one FEATURES manifest.

### Accepted exceptions (plans/docs-ai-data/LOCK.md "Known exceptions")
- validate_plan wave 4/5/6 widened conflicts (decision 9): the `widened (P1)` bullet was parsed as a write-set entry; widened paths do not overlap within a wave. Parser fix in the tooling PR.
- decide.py was ENGINE_DOWN for every slice; spec verdicts are the spec-quality-reviewer's checklist.

### Open spec questions
- LOCK.md "Open issues / NEEDS_HUMAN" items 1-42 (Redis DB 3, new deps go-redis / lightgbm / duckdb, cold-start shape, report limits, recs tier list and fallback `model_version`, copilot claims and impersonation, admin 403 UX) remain for confirmation.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
