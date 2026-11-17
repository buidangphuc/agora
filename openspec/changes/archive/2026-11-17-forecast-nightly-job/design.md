## Context

ADR-0013 stage 1 and 2 are both needed: the baseline is the floor and LightGBM ships only if it beats it. The existing
library has the baseline, the metrics and the payload formatter; it lacks a warehouse reader, an entrypoint, a real model
and any wiring. `platform-featurestore` is the closest precedent (batch, Parquet in, Redis out, compose profile, CronJob).

## Decisions

- **Entry point `python -m forecast run`** (`forecast/__main__.py`, `cli.py`), a single subcommand. Settings from env:
  `INPUT_DIR` (default `/data`), `REDIS_URL`, `REDIS_PREFIX`, `REDIS_TTL_SECONDS`, `HORIZON_DAYS=14`,
  `MIN_HISTORY_DAYS_FOR_ML=14`, `BACKTEST_DAYS=14`, `HISTORY_DAYS=90`, `AS_OF` (RFC 3339, empty = now, same parser rules as
  featurestore). Unused settings are removed so `.env.example` can be gated.
- **Reader**: DuckDB over `order_facts.parquet` selecting only `seller_id, listing_id, quantity, occurred_at, status`;
  `status = 'PAID'`, `occurred_at < date(AS_OF)` (complete UTC days only), within `HISTORY_DAYS`. `duckdb` is the same
  dependency featurestore already uses. A missing file or a missing required column is a configuration error (exit 2).
- **Series**: per listing, zero-filled from the listing's own first paid day to the last complete day (fixes the global
  min..max leading-zero bug). History days = length of that series. The forecast starts at `date(AS_OF)` and spans
  `HORIZON_DAYS` days.
- **Backtest**: the last `BACKTEST_DAYS` complete days are held out. Listings with at least
  `MIN_HISTORY_DAYS_FOR_ML` days before the holdout are eligible. Both models forecast the holdout from the earlier days;
  WAPE is `sum|y - p50| / sum(y)` pooled over all eligible listing-days (existing `compute_wape`). LightGBM wins only when
  its WAPE is strictly lower. No eligible listing: baseline, `wape` null in meta. Then the winning model is refit on all
  history and forecasts the horizon.
- **Models**: LightGBM = one global model per quantile (objective `quantile`, alpha 0.1/0.5/0.9) on lag, rolling and
  calendar features, `seed` fixed, `num_threads=1`. Output is clipped at 0 and made monotone (`p10 <= p50 <= p90`).
  Baseline = existing seasonal-naive (period 7). Quantiles are fixed at 0.1/0.5/0.9; the `QUANTILES` setting goes away
  because other values broke the key mapping.
- **Labels**: constants `lgbm_quantile_v1` and `seasonal_naive_v1`, set by the code path that ran. Cold listings always
  carry `seasonal_naive_v1` and `is_cold_start=true` and still get points; that differs from "no key" which team-analytics
  serves as cold start without points.
- **Output**: `SETEX fc:v1:seller:<seller>:<listing>` JSON (existing payload shape) with TTL 48 h, in one pipeline per
  500 listings; then `fc:v1:meta` JSON `{model_version, generated_at, as_of, horizon_days, listings, cold_starts, wape,
  baseline_wape, status}`. No stale prune beyond TTL: a listing that stops selling ages out in 48 h.
- **Empty export**: zero PAID rows in the window -> status `empty` on stdout, no listing keys, no `fc:v1:meta` write, exit 0.
  The previous night's keys age out by TTL, so a prolonged empty export ends in "no key" rather than stale numbers.
- **Redis DB 3** as decided by the initiative. The compose file's comment says DB 3 is used by "ofb tests"; see open issue.
- **Packaging**: `python:3.12-slim` (+ `libgomp1` for LightGBM), uid 1001, entrypoint `python -m forecast`, `CMD ["run"]`,
  `test` stage. Compose `forecast-job` with `analytics_data:/data:ro`, profile `forecast`, `depends_on: redis`. CronJob
  `45 2 * * *`, `concurrencyPolicy: Forbid`, non-root 10001, read-only root FS, drop ALL, NetworkPolicy to valkey and DNS only,
  PVC `analytics-data` read-only, ArgoCD wave 4. 02:45 is after the 02:15 featurestore run and the analytics export (U11).

## Alternatives rejected

- Keep the heuristic and relabel it honestly: it is neither LightGBM nor better than seasonal-naive by any measure; deleting
  it removes a model nobody can defend.
- Fall back to a mean baseline when no key exists: rejected by U8.
- Write meta first: a crash mid-run would announce a model for keys that are missing.

## Risks

- Demo data is thin, so the baseline will usually win locally; the LightGBM branch is exercised by a synthetic-series unit
  test, and e2e asserts the label matches the meta WAPE comparison, not which model wins.
- `libgomp1` enlarges the image; accepted.
