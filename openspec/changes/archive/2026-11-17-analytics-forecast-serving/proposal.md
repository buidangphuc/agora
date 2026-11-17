initiative: docs-ai-data

## Why

`GetDemandForecast` fabricates numbers. On a query error and on zero history `DuckDBRepository.DemandForecast`
(`team-analytics/internal/query/duckdb.go:163,169`) returns `avgDaily = 2.0`, and `AVG(quantity)` averages per order
line, not per day, so every forecast day carries the same value. A seller can be told to reorder against a number
nothing computed. ADR-0013 already decides that the forecast is written nightly by platform-forecast to Redis
`fc:v1:seller:*` and served by team-analytics; this slice is the serving half (user story 1 and decisions U8, U11 of
the initiative). The seller card in `seller-forecast-card` needs a response that is either real, explicitly "no
forecast", or an error.

## What changes

- `GetDemandForecast` reads one Redis key `fc:v1:seller:<seller>:<listing>` (DB 3) written by `forecast-nightly-job`
  and maps its JSON payload (`model_version`, `generated_at`, `is_cold_start`, `daily_forecasts[]` of
  `{date, p10, p50, p90}`) to the response. It sets the new `generated_at` field (from `docs-ai-data-contract`).
- No key: the response has `is_cold_start = true`, no daily points, reorder point 0, no `generated_at`.
- Redis error, timeout or an undecodable payload: the RPC returns `UNAVAILABLE`, never numbers.
- The DuckDB mean baseline and its `2.0` constants are deleted from `DuckDBRepository` and from the in-memory
  repository; `query.Repository.DemandForecast` is replaced by a separate `ForecastStore` capability interface
  (mirrors the `GetChannelFunnel` repository-capability pattern) backed by a Redis reader.
- Reorder point and safety stock math stays in `internal/rpc/handler.go`, computed only over real points.
- New settings `FORECAST_REDIS_URL` (default empty = the RPC returns `UNAVAILABLE`, no in-memory fallback) and
  `FORECAST_READ_TIMEOUT_MS` (default 25). `docker-compose.services.yaml` sets
  `FORECAST_REDIS_URL=redis://redis:6379/3` on `team-analytics`.
- `analytics.demand-forecast` in `team-analytics/FEATURES.yaml` is rewritten to the new behaviour (the e2e-author owns
  the file edit; this change names the scenarios).
- Not in scope: the writer (`forecast-nightly-job`), the seller card, the gateway route (already forwards
  `GetDemandForecast`), the three report RPCs (`analytics-experiment-reports`, which edits the same hot files after
  this slice).

## Capabilities

### Modified Capabilities
- `analytics-demand-forecast`: demand forecast served from the nightly job output, never fabricated.

## Impact

- team-analytics: `internal/rpc/handler.go`, `internal/query/{repository.go,duckdb.go,querytest/memory.go}`, new
  `internal/forecaststore/`, `internal/config/config.go`, `cmd/consumer` wiring, `.env.example`, README config table,
  `FEATURES.yaml`, `go.mod` (one Redis client dependency; see design.md).
- docker-compose.services.yaml (`team-analytics` env only).
- platform-e2e: `tests/e2e/features/analytics/forecast_serving.feature`, steps, and the replaced forecast scenario in
  `order_facts.feature`.
- Depends on `docs-ai-data-contract` (`generated_at = 8`) and `forecast-nightly-job` (key layout).

## NFR

- Latency: `GetDemandForecast` p95 <= 30 ms, one Redis GET, per-call read timeout 25 ms, no DuckDB query.
- Availability: a Redis outage fails only this RPC (`UNAVAILABLE`); funnel, revenue and report RPCs and the consumer
  are unaffected. No retries inside the RPC, the caller decides.
- Freshness: keys have a 48 h TTL set by the job; an expired key reads as no forecast (cold start), not as stale data.
- Limits: `horizon_days` clamps to 1..28 (default 14 when unset); payload larger than 64 KiB is treated as undecodable.
- Capacity: key count is bounded by listings (10k in the initiative sizing); no scan, no `KEYS`, point reads only.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `seller_id`, `listing_id` in key and response | business identifiers, not personal data | address one forecast | contract with the seller (own data) | key TTL 48 h, owned by the job |
| p10/p50/p90 per day | seller commercial data (aggregated demand) | forecast card, reorder advice | contract with the seller | key TTL 48 h |
| `model_version`, `generated_at` | operational metadata | provenance shown to the seller | n/a | key TTL 48 h |

No buyer identifier is read or returned. Logs carry no seller or listing id values beyond the existing
`log_key_fp` fingerprinting rules; no raw user id is logged.

## Threat model

- Assets: a seller's demand forecast (commercially sensitive); the Redis keyspace `fc:v1:*`.
- Abuse cases: (1) seller A reads seller B's forecast by passing B's `seller_id`; (2) key injection through
  `listing_id` containing `:` or `*` to reach another key; (3) a made-up number steering a stock decision when the
  store is empty or down; (4) a poisoned or malformed payload producing NaN or negative quantities.
- Authz rule: unchanged `requireSellerAccess(ctx, seller_id)`: the caller is that seller, or an admin. The key is built
  from the validated `seller_id` and `listing_id` only; either containing `:`, `*`, whitespace or control characters is
  `InvalidArgument`.
- Mitigations: no fallback numbers (cold start has no points); `UNAVAILABLE` on store faults; payload validation
  rejects non-finite or negative quantities and `p10 > p50` or `p50 > p90` as undecodable.
