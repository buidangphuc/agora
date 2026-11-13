# platform-forecast

Nightly batch job (ADR-0013): reads the paid order lines that `team-analytics` exports to
`order_facts.parquet`, forecasts 14 days of p10/p50/p90 units per `(seller_id, listing_id)` and
writes them to Redis DB 3 under `fc:v1:*`. It is not a server: no port, proto, events or database.

`team-analytics` still computes `GetDemandForecast` itself (`duckdb_baseline_v1`); it does not read
these keys yet.

Bounded context: platform capability (precedent: `platform-featurestore`).

## 1. Contract

- Serves: nothing (no RPCs or endpoints, so no authorization rules).
- Input: `${INPUT_DIR}/order_facts.parquet` (read-only) with `seller_id`, `listing_id`, `quantity`,
  `occurred_at`, `status`. Only `status = 'PAID'` lines of complete days before `AS_OF` count; `buyer_id` is never read.
- Output (Redis, `SETEX`, never deletes a key):
  - `fc:v1:seller:{seller}:{listing}` = JSON `seller_id`, `listing_id`, `model_version`, `generated_at`,
    `is_cold_start`, `daily_forecasts[]` of `{date, p10, p50, p90}`; 14 days starting at `date(AS_OF)`,
    quantiles clipped to `0 <= p10 <= p50 <= p90`; TTL `REDIS_TTL_SECONDS`.
  - `fc:v1:meta`, written last: `model_version`, `generated_at`, `as_of`, `horizon_days`, `listings`,
    `cold_starts`, `wape`, `baseline_wape`, `status`. The same JSON is printed as one stdout line.
- Models: global quantile LightGBM (`lgbm_quantile_v1`) when its backtest WAPE is strictly lower than the
  seasonal-naive baseline's (`seasonal_naive_v1`); listings with fewer than `MIN_HISTORY_DAYS_FOR_ML` days are cold starts on the baseline.
- Exit codes: 0 ok or `empty` export (nothing written), 2 missing/invalid input or config, 3 Redis failure.

## 2. Events

None produced or consumed.

## 3. Data

No database, no migrations.

## 4. Configuration

`forecast/config.py` (`pydantic-settings`); `.env.example` is checked against it by `tests/test_env_example.py`.

| Variable | Default |
|---|---|
| `REDIS_URL` | `redis://localhost:6379/0` |
| `INPUT_DIR` | `/data/input` |
| `AS_OF` | empty (now, RFC 3339) |
| `BACKTEST_DAYS` | `14` |
| `HORIZON_DAYS` | `14` |
| `HISTORY_DAYS` | `90` |
| `MIN_HISTORY_DAYS_FOR_ML` | `14` |
| `REDIS_PREFIX` | `fc:v1:seller` |
| `REDIS_TTL_SECONDS` | `172800` |

## 5. Run locally

```bash
docker compose -f docker-compose.yaml -f docker-compose.services.yaml --profile forecast run --rm forecast-job
```

Mounts `analytics_data` read-only at `/data`, runs as uid 1001, writes Redis DB 3. In the cluster the
CronJob `platform-gitops/platform/forecast` runs it at `45 2 * * *`, after the analytics export.
Without the stack: `make run` (set `INPUT_DIR` and `REDIS_URL`).

## 6. Build, test and lint

```bash
cd platform-forecast
make test        # uv run --extra dev pytest
docker build --target test .
```

CI: `platform-forecast` job in `.github/workflows/services-ci.yml`. No lint tooling is configured.

## 7. Spec and verification

OpenSpec change `forecast-nightly-job`; behaviours are listed in `FEATURES.yaml` and verified by
`tests/` and `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature`.

## 8. Gotchas

- `publisher.py` imports pandas at the bottom of the file (`# noqa: E402`).
- The Redis client uses protocol 2.
- Series are zero-filled from each listing's first sale to the last complete day.

## 9. Links

- Rules: root `AGENTS.md`; root `README.md` (ASDLC).
- ADR: `platform-core/docs/ADR/0013-seller-demand-forecasting.md`.
