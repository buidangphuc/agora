## Context

`Handler.GetDemandForecast` calls `repo.DemandForecast` (DuckDB or the in-memory repository) and then derives
reorder point and safety stock from the first `lead_time_days` points. The new source is a Redis string per listing
written by platform-forecast (`forecast-nightly-job`).

## Decisions

- Separate `ForecastStore` interface in `internal/forecaststore` (`Get(ctx, seller, listing) (Forecast, found bool, err)`),
  injected into `Handler` next to `repo`. `query.Repository.DemandForecast`, `ForecastResult` use in the DuckDB and
  memory repositories, and the `2.0` constants are removed. Reason: the data no longer lives in the warehouse, and a
  BigQuery parity burden for a Redis read makes no sense.
- Redis client: the existing go-sdk has no Redis package and no `team-*` Go service imports one, so this slice adds
  one client dependency to `team-analytics/go.mod` (open issue for the human: which client, `go-redis/v9` expected).
- Unset `FORECAST_REDIS_URL` builds a store that always errors, so the RPC returns `UNAVAILABLE`. This honours the
  no-dependency-enable-flags rule: importing the dependency means using it; there is no in-memory fallback.
- `fc:v1:meta` is not read per request. `model_version` and `generated_at` come from the per-listing payload, which the
  job stamps with the model that actually ran.
- Horizon: the job writes 14 or more points. The RPC returns the first `min(horizon, len(points))` points; it never
  pads. The reorder point uses the same points and the existing formula
  `p50(lead_time) + z * mean(p90 - p50)`; with zero points it is 0 and `safety_stock` is 0.
- Hot files: this slice lands before `analytics-experiment-reports`, which adds report methods to the same
  `handler.go`, `repository.go`, `duckdb.go` and `FEATURES.yaml`.

## Risks

- DB 3 is documented as taken by "ofb tests" in a compose comment; the job and the reader must agree on it and the
  e2e stack must not use it for anything else (flagged in open issues).
- Seeding for e2e: the step writes keys to Redis DB 3 directly; the key layout is the contract with the job.
