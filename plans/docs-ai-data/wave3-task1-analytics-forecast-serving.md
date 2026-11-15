# [W3-T1] analytics-forecast-serving

## Role
feature-run worker (change `openspec/changes/analytics-forecast-serving`)

## Objective
Implement OpenSpec change `analytics-forecast-serving` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- team-analytics `GetDemandForecast` reads `fc:v1:seller:*` from Redis DB 3 via new `internal/forecaststore` (25 ms timeout, 64 KiB cap), serves `generated_at`, cold start on no key, UNAVAILABLE on faults; delete the DuckDB baseline; `FORECAST_REDIS_URL` in config and compose; go-redis/v9 dependency.
- depends_on: docs-ai-data-contract, forecast-nightly-job (merged)
- base: `asdlc/docs-ai-data` after wave 2
- hot files: team-analytics/internal/rpc/handler.go, internal/query/{repository.go,duckdb.go}, team-analytics/FEATURES.yaml (analytics-experiment-reports W4); docker-compose.services.yaml

## Write-set (EXCLUSIVE)
- team-analytics/internal/forecaststore/**
- team-analytics/internal/config/**
- team-analytics/internal/rpc/**
- team-analytics/internal/query/**
- team-analytics/cmd/consumer/**
- team-analytics/go.mod
- team-analytics/go.sum
- team-analytics/.env.example
- team-analytics/README.md
- team-analytics/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/analytics/forecast_serving.feature
- platform-e2e/tests/e2e/features/analytics/order_facts.feature
- platform-e2e/tests/e2e/step_definitions/test_forecast_serving.py
- platform-e2e/tests/e2e/step_definitions/forecast_serving_steps.py
- platform-e2e/tests/e2e/step_definitions/test_order_facts.py
- platform-e2e/tests/e2e/step_definitions/order_facts_steps.py
- widened (user-approved 2026-11-15):
  - team-analytics/internal/app/consumer.go — server wiring of the forecast store (the packet named cmd/consumer/** by mistake)
- widened (P1):
  - platform-e2e/tests/e2e/step_definitions/fsv_steps.py — e2e step file of this change (forecast_serving.feature), named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_fsv_forecast_serving.py — e2e scenario binding of this change, named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/journey_steps.py — e2e step broken by the new behaviour: a missing forecast key is now a cold start, so the seller cockpit journey seeds a key
  - platform-forecast/README.md — doc updated by docs-sync (team-analytics now reads the fc:v1 keys)
  - openspec/changes/analytics-forecast-serving/tasks.md — task ticks of this change
  - openspec/changes/analytics-forecast-serving/specs/analytics-demand-forecast/spec.md — VERIFIED BY citations matching the real test file (forecast_test.go)

## Verify
- `cd team-analytics && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...` and `go vet ./...`
- `openspec validate analytics-forecast-serving --strict`
- `make -C platform-e2e features-check`
- e2e: analytics/forecast_serving.feature, analytics/order_facts.feature (mainx, seeded `fc:v1` keys)
