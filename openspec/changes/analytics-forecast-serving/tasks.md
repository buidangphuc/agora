## 1. Code track (team-analytics)

- [x] 1.1 Tests first: handler tests for scenarios "Reorder point and safety stock are derived from the stored points", "An invalid listing or seller id is rejected", "A key marked cold start carries no invented numbers", "Redis unreachable returns UNAVAILABLE", "An unset store address returns UNAVAILABLE"; `duckdb_test.go` "The DuckDB baseline is gone"; verify red on the current code
- [ ] 1.2 `internal/forecaststore`: Redis reader (`Get`), payload decode and validation, 25 ms timeout, 64 KiB cap, key built from validated ids; unit tests for the malformed cases of "A malformed payload returns UNAVAILABLE"
- [ ] 1.3 `internal/config/config.go`: `FORECAST_REDIS_URL`, `FORECAST_READ_TIMEOUT_MS`; `.env.example` and README config table (drift gates)
- [x] 1.4 `handler.go`: inject the store, validate ids, map to response with `generated_at`, keep reorder-point math, return `UNAVAILABLE` on faults, cold start on no key ("A forecast key is served as written", "The horizon truncates and never pads", "No key means cold start without points")
- [x] 1.5 Delete `DemandForecast` from `query.Repository`, `DuckDBRepository` and `querytest/memory.go` (scenario "The DuckDB baseline is gone")
- [x] 1.6 Wire the store in `cmd/consumer`; add one Redis client to `go.mod`; `docker-compose.services.yaml` sets `FORECAST_REDIS_URL=redis://redis:6379/3` on `team-analytics`
- [ ] 1.7 `go test ./...`, lint, features-check, `openspec validate analytics-forecast-serving --strict`

## 2. e2e track (platform-e2e, team-analytics/FEATURES.yaml by the e2e-author)

- [ ] 2.1 `forecast_serving.feature` + steps that seed and clear `fc:v1:seller:*` keys in Redis DB 3: "A forecast key is served as written", "The horizon truncates and never pads", "No key means cold start without points", "A malformed payload returns UNAVAILABLE", "Another seller's forecast is denied"
- [ ] 2.2 "A read stays within the latency budget" (200 calls, p95 <= 30 ms)
- [ ] 2.3 Replace the old "GetDemandForecast serves probabilistic daily demand and restock points" scenario in `order_facts.feature` with a link to the new feature; rewrite the `analytics.demand-forecast` FEATURES entry (`covered_by` the new scenarios)
- [ ] 2.4 Run on the `mainx` stack with `forecast-nightly-job` output or seeded keys; analytics group green
