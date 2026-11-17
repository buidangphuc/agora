## 1. Code track (platform-forecast, platform-gitops, compose, docs)

- [x] 1.1 Tests first (red): `test_extract_paid_lines.py`, `test_cli_exit_codes.py`, `test_determinism.py`, `test_model_selection.py`, `test_run_summary.py`, `test_env_example.py` in `platform-forecast/tests/`. Cite FNJ-1, FNJ-2, FNJ-4, FNJ-5, FNJ-9, FNJ-10, FNJ-11, FNJ-12, FNJ-17, FNJ-20.
- [x] 1.2 Reader: DuckDB over `order_facts.parquet`, paid and complete-day filter, per-listing series from first sale, no `buyer_id`; exit 2 on missing file or column. FNJ-1, FNJ-2, FNJ-3, FNJ-4.
- [x] 1.3 Models and selection: add `lightgbm` and `duckdb` dependencies, global quantile LightGBM (fixed seed, one thread), delete `QuantileRegressor`, pooled-WAPE backtest over the holdout using `evals/backtest.py`, constants `lgbm_quantile_v1` and `seasonal_naive_v1`, cold-start rule. FNJ-9, FNJ-11, FNJ-12, FNJ-13.
- [x] 1.4 Output: per-listing `SETEX` with TTL, 14-day horizon starting at `date(AS_OF)`, monotone clipped quantiles, `fc:v1:meta` last, stdout summary, exit 3 on Redis failure, `empty` path writes nothing, never delete keys. FNJ-5, FNJ-6, FNJ-7, FNJ-8, FNJ-10, FNJ-15, FNJ-16, FNJ-17.
- [x] 1.5 Entry point and settings: `forecast/__main__.py`, `cli.py`, trim `config.py` (drop `env`, `duckdb_path`, `quantiles`, `model_version`), `HORIZON_DAYS=14`, add `INPUT_DIR`, `AS_OF`, `BACKTEST_DAYS`, `.env.example`, `Makefile`. FNJ-20.
- [x] 1.6 `Dockerfile` (non-root uid 1001, `libgomp1`, `test` and `job` stages) and `docker-compose.services.yaml` service `forecast-job`, profile `forecast`, Redis DB 3, `analytics_data:/data:ro`. FNJ-18.
- [x] 1.7 platform-gitops: `platform/forecast/` (ConfigMap, CronJob `45 2 * * *`, NetworkPolicy, kustomization) and `argocd/apps/forecast.yaml`. FNJ-19.
- [x] 1.8 `.github/workflows/services-ci.yml` job for platform-forecast (like platform-featurestore); root `AGENTS.md` repo row and schedule line; flip `platform-core/docs/ADR/0013-seller-demand-forecasting.md` status to Accepted; rewrite `platform-forecast/README.md` from "unwired library" to the job (delete the Known gaps this slice closes).
- [x] 1.9 Green: `cd platform-forecast && uv run pytest -q`; `docker compose --profile forecast run --rm forecast-job` against the local stack.

## 2. e2e track (platform-e2e, run by the e2e-author)

- [x] 2.1 `platform-e2e/tests/e2e/features/forecast/nightly_forecast.feature` with scenarios named in the VERIFIED BY lines of FNJ-3, FNJ-6, FNJ-7, FNJ-8, FNJ-13, FNJ-14, FNJ-15, FNJ-16, FNJ-18, FNJ-19; seed paid order lines through the existing order flow and wait out the export cycle once (mirror `featurestore/feature_materialization.feature`). Static render steps for FNJ-19.
- [x] 2.2 `platform-forecast/FEATURES.yaml` (owner `platform-ml-team`) entries for the job behaviours, `covered_by` the feature file; `make -C platform-e2e features-check` green.
- [x] 2.3 Unit-verified scenarios (FNJ-1, FNJ-2, FNJ-4, FNJ-5, FNJ-9 to FNJ-12, FNJ-17, FNJ-20) recorded in FEATURES.yaml `notes` as unit-verified with the test path.
