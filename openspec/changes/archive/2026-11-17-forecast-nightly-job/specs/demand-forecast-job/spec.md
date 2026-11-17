## ADDED Requirements

### Requirement: The job reads paid order lines from the warehouse export

`python -m forecast run` SHALL read only `seller_id`, `listing_id`, `quantity`, `occurred_at` and `status` from
`order_facts.parquet` in `INPUT_DIR`, keep lines whose status is `PAID` and whose `occurred_at` falls on a complete UTC day before
the date of `AS_OF`, and build one daily series per listing from that listing's first paid day, never reading `buyer_id`.
The job SHALL exit 0 on success or empty input, 2 on a configuration or input error and 3 on a Redis write failure.

#### Scenario: FNJ-1 Only paid lines before the as-of day are counted

- **WHEN** the job runs with `AS_OF` 2026-11-10T02:45:00Z over an export holding PAID lines on 2026-11-08 and 2026-11-09 for a listing, an unpaid line on 2026-11-09 and a PAID line on 2026-11-10
- **THEN** the listing's series ends on 2026-11-09 and its units equal the sum of the two PAID days only
- **VERIFIED BY**: `platform-forecast/tests/test_extract_paid_lines.py` `test_only_paid_complete_days`

#### Scenario: FNJ-2 A listing's series starts at its own first sale

- **WHEN** listing A has its first paid line 20 days before `AS_OF` and listing B has its first paid line 5 days before `AS_OF`
- **THEN** A's history is 20 days, B's history is 5 days and B has no leading zero days
- **VERIFIED BY**: `platform-forecast/tests/test_extract_paid_lines.py` `test_series_starts_at_first_sale`

#### Scenario: FNJ-3 A missing export file is a configuration error

- **WHEN** the job runs and `order_facts.parquet` is missing from `INPUT_DIR`
- **THEN** the process exits with code 2, names the missing file on stderr and writes no Redis key
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `A missing export file is a configuration error`

#### Scenario: FNJ-4 An export without a required column is rejected

- **WHEN** the job runs over an `order_facts.parquet` that has no `status` column
- **THEN** the process exits with code 2 and writes no Redis key
- **VERIFIED BY**: `platform-forecast/tests/test_extract_paid_lines.py` `test_missing_column_exits_2`

#### Scenario: FNJ-5 An unreachable Redis fails the run

- **WHEN** the job runs and Redis refuses the connection
- **THEN** the process exits with code 3 and `fc:v1:meta` is not written
- **VERIFIED BY**: `platform-forecast/tests/test_cli_exit_codes.py` `test_redis_down_exits_3`

### Requirement: The job writes one forecast key per listing

For every listing with at least one paid line in the window, the job SHALL write the JSON payload to
`fc:v1:seller:<seller_id>:<listing_id>` with a TTL of `REDIS_TTL_SECONDS` (default 172800 s). The payload SHALL hold `seller_id`,
`listing_id`, `model_version`, `generated_at` (UTC), `is_cold_start` and `daily_forecasts` of exactly `HORIZON_DAYS` (default 14)
`{date, p10, p50, p90}` entries starting on the date of `AS_OF`, with `0 <= p10 <= p50 <= p90`. The job SHALL never delete keys.

#### Scenario: FNJ-6 A listing with history gets a 14-day key

- **WHEN** the job runs over an export where listing L of seller S has 30 days of paid lines
- **THEN** Redis holds `fc:v1:seller:S:L` with 14 `daily_forecasts` dated `date(AS_OF)` to `date(AS_OF)+13`, each with `0 <= p10 <= p50 <= p90`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `A listing with history gets a 14-day key`

#### Scenario: FNJ-7 Keys expire after 48 hours

- **WHEN** the job has written a listing key
- **THEN** the key's TTL is between 172700 and 172800 seconds
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `Keys expire after 48 hours`

#### Scenario: FNJ-8 A listing without paid lines gets no key

- **WHEN** the job runs and listing X has no PAID line in the window
- **THEN** Redis holds no `fc:v1:seller:*:X` key after the run
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `A listing without paid lines gets no key`

#### Scenario: FNJ-9 A rerun with the same inputs gives the same numbers

- **WHEN** the job runs twice over the same export with the same `AS_OF`
- **THEN** the `daily_forecasts` of every key are identical between the two runs
- **VERIFIED BY**: `platform-forecast/tests/test_determinism.py` `test_same_input_same_output`

#### Scenario: FNJ-10 A failed run deletes no earlier key

- **WHEN** a run exits with code 2 or 3 after an earlier successful run
- **THEN** every key of the earlier run is still present with its earlier payload
- **VERIFIED BY**: `platform-forecast/tests/test_cli_exit_codes.py` `test_failed_run_keeps_old_keys`

### Requirement: The model label names the model that ran

The job SHALL backtest the global LightGBM quantile model and the seasonal-naive baseline on the same holdout of the last
`BACKTEST_DAYS` (default 14) complete days, pooling WAPE over listings with at least `MIN_HISTORY_DAYS_FOR_ML` days before the
holdout, and SHALL forecast non-cold listings with LightGBM only when its WAPE is strictly lower than the baseline's. Every
payload and `fc:v1:meta` SHALL carry `model_version` `lgbm_quantile_v1` when LightGBM produced the numbers and `seasonal_naive_v1`
otherwise. A listing with fewer than `MIN_HISTORY_DAYS_FOR_ML` days of history SHALL get the baseline and `is_cold_start=true`.

#### Scenario: FNJ-11 LightGBM is used only when it beats the baseline

- **WHEN** the backtest WAPE of LightGBM is lower than that of the baseline on a synthetic series with a strong weekly trend
- **THEN** non-cold payloads carry `model_version` `lgbm_quantile_v1` and meta carries the same value
- **VERIFIED BY**: `platform-forecast/tests/test_model_selection.py` `test_lgbm_wins_on_trend`

#### Scenario: FNJ-12 The baseline is labelled when LightGBM does not win

- **WHEN** the backtest WAPE of LightGBM is equal to or higher than the baseline's, or no listing is eligible for the backtest
- **THEN** every payload and meta carry `model_version` `seasonal_naive_v1` and no payload carries `lgbm_quantile_v1`
- **VERIFIED BY**: `platform-forecast/tests/test_model_selection.py` `test_baseline_labelled_when_not_beaten`

#### Scenario: FNJ-13 A short history is a cold start

- **WHEN** listing C has 5 days of history and `MIN_HISTORY_DAYS_FOR_ML` is 14
- **THEN** C's payload has `is_cold_start` true, `model_version` `seasonal_naive_v1` and 14 daily points
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `A short history is a cold start`

#### Scenario: FNJ-14 A label never overstates the model

- **WHEN** the job runs on the local stack export
- **THEN** a payload carries `lgbm_quantile_v1` only if meta `wape` is lower than meta `baseline_wape`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `A label never overstates the model`

### Requirement: The job records a run summary

After writing all listing keys, the job SHALL write `fc:v1:meta` (TTL `REDIS_TTL_SECONDS`) with `model_version`, `generated_at`,
`as_of`, `horizon_days`, `listings`, `cold_starts`, `wape` (chosen model, null when no listing was eligible), `baseline_wape`
and `status`, and SHALL print the same JSON as one stdout line. An export with no PAID line in the window SHALL give status
`empty`, no listing key, no `fc:v1:meta` write, and exit code 0.

#### Scenario: FNJ-15 The summary counts the run

- **WHEN** the job runs over an export with 3 listings of which 1 is a cold start
- **THEN** `fc:v1:meta` has `listings` 3, `cold_starts` 1, `status` `ok`, and the stdout JSON line equals it
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `The summary counts the run`

#### Scenario: FNJ-16 An empty export writes nothing and succeeds

- **WHEN** the job runs over an export with zero rows
- **THEN** the process exits 0 with stdout status `empty`, and Redis DB 3 holds no new `fc:v1` key
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `An empty export writes nothing and succeeds`

#### Scenario: FNJ-17 The summary carries no row data

- **WHEN** the job has finished a run
- **THEN** neither `fc:v1:meta` nor the stdout line contains a buyer id, order id or event id
- **VERIFIED BY**: `platform-forecast/tests/test_run_summary.py` `test_summary_has_only_aggregates`

### Requirement: The job runs as a hardened nightly batch

The repo SHALL ship a non-root image whose entrypoint is `python -m forecast`, a docker-compose service `forecast-job` in profile
`forecast` that mounts `analytics_data` read-only and writes Redis DB 3, and a platform-gitops CronJob `forecast` scheduled
`45 2 * * *` that forbids concurrent runs, runs non-root with a read-only root filesystem, mounts the export read-only and is
admitted by a NetworkPolicy allowing only DNS and valkey. The root `AGENTS.md` SHALL list platform-forecast and the repo SHALL
carry a `FEATURES.yaml`, a services-ci job and a `.env.example` that matches the settings.

#### Scenario: FNJ-18 The compose job runs as a non-root user

- **WHEN** `docker compose --profile forecast run --rm forecast-job` starts
- **THEN** the container user id is 1001 and the export mount is read-only
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `The compose job runs as a non-root user`

#### Scenario: FNJ-19 The CronJob is scheduled after the export and isolated

- **WHEN** the platform-gitops forecast manifests are rendered
- **THEN** the CronJob schedule is `45 2 * * *`, `concurrencyPolicy` is `Forbid`, the pod has `runAsNonRoot` true and `readOnlyRootFilesystem` true, the `analytics-data` volume is `readOnly`, and the NetworkPolicy has no ingress and egress only to DNS and valkey port 6379
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` `The CronJob is scheduled after the export and isolated`

#### Scenario: FNJ-20 A settings drift fails the gate

- **WHEN** a setting is added to `forecast/config.py` without a line in `.env.example`
- **THEN** the repo's drift test fails and names the missing variable
- **VERIFIED BY**: `platform-forecast/tests/test_env_example.py` `test_env_example_matches_settings`
