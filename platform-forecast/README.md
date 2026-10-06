# platform-forecast

Offline probabilistic demand-forecasting **library** (ADR-0013, Stage 1): turns order-line facts into
per-`(seller_id, listing_id)` daily p10/p50/p90 forecasts and writes them to Redis.

**Status: unwired library. It is not deployed and nothing calls it.** It has no entrypoint, Dockerfile,
Makefile, compose service, GitOps CronJob, `.env.example`, `FEATURES.yaml` or CI. It is not in the
root `AGENTS.md` repo map. The only external reference is ADR-0013 (status "Proposed").

**`team-analytics` computes `GetDemandForecast` itself** (`team-analytics/internal/query/duckdb.go`
`DemandForecast`, `service.go` `GetDemandForecast`) from DuckDB `order_facts`: one mean/stddev per
listing, model version `duckdb_baseline_v1`. It does not read the Redis keys this repo writes, so
this library's output has no consumer today.

Bounded context: platform capability (precedent: `platform-recsys`). It owns no business tables,
defines no proto, serves no port and is not browser-reachable.

## 1. Contract

- Serves: nothing (no RPCs or endpoints, so no authorization rules).
- Consumes: no upstream RPCs and no warehouse. The entry point
  `forecast.pipeline.run_demand_forecast_pipeline(raw_facts, redis_client=None, settings=None)` takes
  in-memory order facts that the caller supplies: a list of dicts or a DataFrame.
  - Input columns: `seller_id`, `listing_id`, `quantity`, plus `occurred_at` or `date`. Revenue is
    `quantity * unit_price` if `unit_price` is present, else a `revenue` column, else 0.
  - `extract.py` is described as "from warehouse" but only aggregates what it is given.
- Output (Redis, when `redis_client` is passed; `SETEX`, with no client the key is only returned):
  - Key `{redis_prefix}:{seller_id}:{listing_id}` = `fc:v1:seller:{seller}:{listing}`, TTL `redis_ttl_seconds`.
  - JSON: `seller_id`, `listing_id`, `model_version`, `generated_at` (UTC ISO), `is_cold_start`,
    `daily_forecasts[]` of `{date, p10, p50, p90}`.
  - Forecasts start the day after the latest date in the input.
- Return value: `{status, forecast_count, forecast_start_date, horizon_days, published_keys, payloads}`;
  `{status: "empty", forecast_count: 0, model_version}` for empty input.

## 2. Events

None produced or consumed.

## 3. Data

No database. DuckDB is not read (`duckdb_path` is defined but unused). No migrations.

## 4. Configuration

`forecast/config.py` (`pydantic-settings`, reads `.env`, unknown keys ignored, env names are the
upper-cased field names).

| Variable | Default | Used by code |
|---|---|---|
| `ENV` | `local` | no |
| `REDIS_URL` | `redis://localhost:6379/0` | no (the caller injects the client) |
| `DUCKDB_PATH` | `/data/analytics.duckdb` | no |
| `MODEL_VERSION` | `lgbm_quantile_v1` | yes (label only, see Known gaps) |
| `HORIZON_DAYS` | `28` | yes |
| `HISTORY_DAYS` | `90` | no |
| `MIN_HISTORY_DAYS_FOR_ML` | `14` | yes (cold-start threshold) |
| `QUANTILES` | `[0.10, 0.50, 0.90]` | yes (only 0.10/0.50/0.90 map to p10/p50/p90 keys) |
| `REDIS_PREFIX` | `fc:v1:seller` | yes |
| `REDIS_TTL_SECONDS` | `172800` (48 h) | yes |

There is no `.env.example` and so no drift gate.

## 5. Run locally

Not in `docker-compose.yaml` or `docker-compose.services.yaml`, and no `jobs` profile. Standalone only:
call the pipeline from Python with your own facts and an optional Redis client.

## 6. Build, test and lint

```bash
cd platform-forecast
pip install -e '.[dev]'    # pandas, numpy, scipy, redis, pydantic(-settings); dev: pytest, pytest-cov
pytest                     # testpaths=tests, pythonpath=.
```

No lint, format or type-check tooling is configured, and there is no CI gate. Tests use a mock
Redis client and default `Settings()`.

## 7. Spec and verification

- No `FEATURES.yaml` and no e2e coverage for this repo. The user-facing RPC is covered under
  `team-analytics/FEATURES.yaml` (`GetDemandForecast serves probabilistic daily demand and restock points`,
  via `platform-e2e` `analytics/order_facts.feature`); that tests the team-analytics implementation, not this library.
- Changes go through OpenSpec (`openspec/changes/<id>`), per the root README's ASDLC. No change
  named forecast exists (Stage 0 was `archive/2026-09-20-add-order-warehouse-facts`). Verify with
  `make -C platform-e2e features-check` / `make -C platform-e2e spec-check CHANGE=<id>`.

## 8. Gotchas

- `publisher.py` imports pandas at the bottom of the file (`# noqa: E402`); it is used inside `format_forecast_payload`.
- `quantile_lgbm.QuantileRegressor` is a heuristic: rolling mean/lag blend, a weekend factor and a fixed z of 1.28. It is not LightGBM and not trained. `lightgbm` is not a dependency.
- `baseline.SeasonalNaiveQuantileForecaster` is seasonal-naive (period 7) plus a normal-approximation spread, not EWMA.
- Cold start (fewer than `min_history_days_for_ml` observed days after zero-filling) uses the baseline and appends `_baseline` to `model_version`.
- Series are zero-filled across the global min..max date of the whole input, so a listing first sold late gets leading zeros and is rarely "cold".
- `backtest.py` metrics (WAPE, pinball loss, interval coverage) are pure functions; nothing calls them or persists results.

## 9. Known gaps

- Unwired: no entrypoint, image, compose `jobs` profile, gitops CronJob/Argo app, or AGENTS.md row. ADR-0013 expects a nightly platform-gitops CronJob.
- Output has no reader: `team-analytics` does not read `fc:v1:seller:*`; it computes forecasts itself.
- ADR-0013 items not implemented: warehouse read (no DuckDB/BigQuery extract), category/seller-median cold-start floor, `forecast_runs` backtest rows, OTel gauges, `model_version` key, stale-generation prune, restock/reorder logic (that lives in `team-analytics`).
- `model_version` defaults to `lgbm_quantile_v1` although no LightGBM model exists.
- Unused settings: `env`, `redis_url`, `duckdb_path`, `history_days`.
- Percentile keys are derived as `int(q*100)`, so quantiles other than 0.1/0.5/0.9 break the payload mapping (z-scores are hard-coded for those three).

## 10. Links

- Rules: root `AGENTS.md`; root `README.md` (ASDLC).
- ADR: `platform-core/docs/ADR/0013-seller-demand-forecasting.md` (also `0011-model-serving.md`, `0014-model-registry-promotion.md` for the platform-model precedent).
