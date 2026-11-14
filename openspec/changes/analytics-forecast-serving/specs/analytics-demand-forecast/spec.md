## ADDED Requirements

### Requirement: Forecast is served from the nightly job output

`AnalyticsQueryService.GetDemandForecast` SHALL read exactly one Redis key `fc:v1:seller:<seller_id>:<listing_id>`
(DB 3) and return its `daily_forecasts`, `model_version` and `generated_at` unchanged, truncated to the requested
`horizon_days` (default 14, clamped to 1..28). The RPC SHALL NOT query DuckDB or compute any demand number itself.

#### Scenario: A forecast key is served as written

- **WHEN** a seller calls `GetDemandForecast` for a listing whose key holds 14 days of p10/p50/p90, model_version `seasonal_naive_v1` and generated_at 2026-11-13T02:45Z
- **THEN** the response has 14 `daily_forecasts` equal to the stored points, `model_version = seasonal_naive_v1`, `generated_at = 2026-11-13T02:45Z` and `is_cold_start = false`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/forecast_serving.feature` `A forecast key is served as written`

#### Scenario: The horizon truncates and never pads

- **WHEN** a seller calls `GetDemandForecast` with `horizon_days = 7` on a key holding 14 points, then with `horizon_days = 40` on the same key
- **THEN** the first response has the first 7 stored points and the second has 14 points, none invented
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/forecast_serving.feature` `The horizon truncates and never pads`

#### Scenario: Reorder point and safety stock are derived from the stored points

- **WHEN** a seller calls `GetDemandForecast` with `lead_time_days = 3` and `service_level = 0.95` on a key whose first three days have p50 = 4 and p90 = 6
- **THEN** `safety_stock = 3.3` (z 1.65 times mean spread 2) and `suggested_reorder_point = 15.3` (12 plus 3.3)
- **VERIFIED BY**: `team-analytics/internal/rpc/forecast_test.go` `TestGetDemandForecastReorderPoint`

#### Scenario: An invalid listing or seller id is rejected

- **WHEN** a caller sends `listing_id = "x:*"`, or an empty `seller_id`, or an empty `listing_id`
- **THEN** the RPC returns `InvalidArgument` and issues no Redis read
- **VERIFIED BY**: `team-analytics/internal/rpc/forecast_test.go` `TestGetDemandForecastRejectsBadIDs`

#### Scenario: Another seller's forecast is denied

- **WHEN** seller A calls `GetDemandForecast` with `seller_id` of seller B, and separately an unauthenticated caller does the same
- **THEN** the first call returns `PermissionDenied` and the second `Unauthenticated`, both without a Redis read, while an admin caller receives B's forecast
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/forecast_serving.feature` `Another seller's forecast is denied`

### Requirement: A missing forecast is a cold start with no numbers

When no key exists for the listing, `GetDemandForecast` SHALL return `is_cold_start = true`, no `daily_forecasts`,
`suggested_reorder_point = 0`, `safety_stock = 0` and no `generated_at`. It SHALL NOT serve a mean, floor or
default value, even when `order_facts` holds history for that listing.

#### Scenario: No key means cold start without points

- **WHEN** a seller calls `GetDemandForecast` for a listing that has no `fc:v1` key but has paid order lines in `order_facts`
- **THEN** the response has `is_cold_start = true`, zero `daily_forecasts`, `suggested_reorder_point = 0` and no `generated_at`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/forecast_serving.feature` `No key means cold start without points`

#### Scenario: A key marked cold start carries no invented numbers

- **WHEN** a seller calls `GetDemandForecast` for a listing whose key has `is_cold_start = true` and a baseline model_version
- **THEN** the response echoes the stored baseline points with `is_cold_start = true` and the stored `model_version`
- **VERIFIED BY**: `team-analytics/internal/rpc/forecast_test.go` `TestGetDemandForecastStoredColdStart`

#### Scenario: The DuckDB baseline is gone

- **WHEN** `GetDemandForecast` is called on a build where the warehouse query path is disabled
- **THEN** the response is identical to the one from a normal build, because no code path reads `order_facts` for this RPC
- **VERIFIED BY**: `team-analytics/internal/query/duckdb_test.go` `TestRepositoryHasNoDemandForecast`

### Requirement: Store faults fail closed

When Redis cannot be read within `FORECAST_READ_TIMEOUT_MS`, is not configured, or returns a payload that fails
validation (invalid JSON, non-finite or negative quantity, p10 > p50 or p50 > p90, over 64 KiB), `GetDemandForecast`
SHALL return gRPC `UNAVAILABLE` with no forecast fields.

#### Scenario: Redis unreachable returns UNAVAILABLE

- **WHEN** a seller calls `GetDemandForecast` while the Redis read fails or times out
- **THEN** the RPC returns `UNAVAILABLE` and the response carries no daily points and no reorder point
- **VERIFIED BY**: `team-analytics/internal/rpc/forecast_test.go` `TestGetDemandForecastStoreDown`

#### Scenario: A malformed payload returns UNAVAILABLE

- **WHEN** a seller calls `GetDemandForecast` for a listing whose key holds invalid JSON, or a point with p10 greater than p50
- **THEN** the RPC returns `UNAVAILABLE` and no numbers
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/forecast_serving.feature` `A malformed payload returns UNAVAILABLE`

#### Scenario: An unset store address returns UNAVAILABLE

- **WHEN** team-analytics starts with `FORECAST_REDIS_URL` empty and a seller calls `GetDemandForecast`
- **THEN** the RPC returns `UNAVAILABLE`, and the other query RPCs keep answering
- **VERIFIED BY**: `team-analytics/internal/rpc/forecast_test.go` `TestGetDemandForecastStoreUnconfigured`

#### Scenario: A read stays within the latency budget

- **WHEN** 200 sequential `GetDemandForecast` calls hit a stored key on the local stack
- **THEN** the p95 call latency is at most 30 ms
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/analytics/forecast_serving.feature` `A read stays within the latency budget`
