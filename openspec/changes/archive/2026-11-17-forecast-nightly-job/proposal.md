## Why

Sellers need a 14-day demand forecast they can trust, but nothing produces one that is honest. `platform-forecast` is an
unwired library (no entrypoint, image, compose job, CronJob or `FEATURES.yaml`); it labels its output `lgbm_quantile_v1`
although no LightGBM model runs (`quantile_lgbm.QuantileRegressor` is a rolling-mean heuristic); it zero-fills every
listing from the global first date so a late-starting listing is rarely cold; its backtest metrics are never called; and it
takes in-memory facts instead of reading the warehouse export. `team-analytics` meanwhile fabricates `avgDaily = 2.0` on
errors and zero history (`team-analytics/internal/query/duckdb.go:163,169`). ADR-0013 (Proposed) already decides the shape:
a nightly job writes Redis `fc:v1:seller:*`, team-analytics serves it. This slice builds the writer half so the serving
slice (`analytics-forecast-serving`) has real, honestly labelled keys to read.

initiative: docs-ai-data

## What changes

- `platform-forecast` gets `python -m forecast run`: reads `order_facts.parquet` from the shared analytics volume
  (read-only), keeps paid lines up to `AS_OF`, builds one daily series per listing from its own first sale, and writes one
  Redis key per listing plus `fc:v1:meta`.
- Model choice per run: a global LightGBM quantile model (p10/p50/p90) is used only when its backtest WAPE beats the
  seasonal-naive baseline on the same holdout, otherwise the baseline is used. `model_version` names the model that
  actually ran (`lgbm_quantile_v1` or `seasonal_naive_v1`). The heuristic `QuantileRegressor` is deleted and `lightgbm`
  becomes a real dependency.
- Listings with fewer than `MIN_HISTORY_DAYS_FOR_ML` days of history get the baseline and `is_cold_start=true`.
- Horizon is 14 days (U11). Run summary (listings, cold starts, WAPE of the chosen model and of the baseline) is written to
  `fc:v1:meta` and printed as one JSON line.
- Packaging: non-root `Dockerfile`, compose profile `forecast` with service `forecast-job` (mirrors `featurestore-job`),
  platform-gitops CronJob at 02:45 UTC with NetworkPolicy and ArgoCD Application, a `services-ci` job, a root `AGENTS.md`
  row, ADR-0013 flipped to Accepted, and the README rewritten from "unwired library" to the job.
- Deleted: unused settings (`env`, `duckdb_path`, `quantiles`, `model_version` label setting), the heuristic model, the
  in-memory `raw_facts` entry as the job's input path.

## Capabilities

### New Capabilities
- `demand-forecast-job`: the nightly forecast job, its Redis output contract, its model-honesty rule and its packaging.

### Modified Capabilities
none

## Impact

- `platform-forecast/` (code, tests, Dockerfile, Makefile, README, `.env.example`), `platform-gitops/platform/forecast/` and
  `platform-gitops/argocd/apps/forecast.yaml`, `docker-compose.services.yaml` (service `forecast-job`, profile `forecast`),
  `.github/workflows/services-ci.yml`, root `AGENTS.md`, `platform-core/docs/ADR/0013-seller-demand-forecasting.md` (status line only).
- Redis DB 3 on the shared Redis/valkey. No proto, no events, no new port.
- Hot file `docker-compose.services.yaml` is shared with `analytics-forecast-serving` (adds `REDIS` DB 3 env to team-analytics)
  and `ai-assistant-grounding`; this slice lands first.
- Serving side is out of scope: `analytics-forecast-serving` reads these keys and deletes the DuckDB baseline.

## Non-functional requirements

- Run time at most 10 minutes for 10 000 listings on 1 CPU and 2 GiB (the CronJob limits).
- Deterministic: the same export and the same `AS_OF` produce byte-identical forecast values (fixed LightGBM seed, single thread).
- Atomic enough for readers: listing keys are written first and `fc:v1:meta` last, so meta never describes a run whose keys
  were not all written; a failed run leaves no new meta.
- Availability: a failed or skipped night leaves the previous keys readable for up to 48 h (TTL `172800`); the job never
  deletes keys.
- Exit codes: 0 success or `empty`, 2 configuration or input error, 3 Redis write failure.
- Logs carry counts and ids of the run only, never row data; no buyer data is read at all.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `seller_id`, `listing_id` (from `order_facts`) | business identifiers, not personal data | key the forecast per listing | contract performance (seller analytics) | key TTL 48 h, overwritten nightly |
| `quantity`, `unit_price`, `occurred_at` | business facts | daily demand series | same | not stored; only forecast aggregates are written |
| `buyer_id`, `order_id`, `event_id`, `variant_id` | personal or linkable | NOT read: the job selects only the columns it needs | n/a | n/a |
| Forecast payload and `fc:v1:meta` | derived aggregates | read by team-analytics | same | TTL 48 h |

No personal data enters Redis, logs or the run summary.

## Threat model

- Assets: forecast keys (seller-visible numbers that drive restock decisions), the analytics Parquet export, Redis DB 3.
- Abuse cases: (1) a forged or tampered key makes a seller over-order; mitigations: only this job and team-analytics reach DB 3
  (NetworkPolicy egress to valkey and DNS only, no ingress), the job image is non-root with a read-only root filesystem,
  the export volume is mounted read-only. (2) A model that does not beat the baseline is shipped under a flattering label;
  mitigation: the label is derived from the backtest result and tested. (3) Fabricated numbers on failure; mitigation: errors
  exit non-zero and write nothing, the empty export writes nothing. (4) Cross-seller leakage; mitigation: one key per
  `(seller, listing)`, payload carries only that listing.
- Authz rule: the job serves no RPC and no port. Authorization stays in team-analytics (`GetDemandForecast` scopes the key to
  the caller's own `seller_id`); this slice only guarantees keys are namespaced `fc:v1:seller:<seller>:<listing>`.
