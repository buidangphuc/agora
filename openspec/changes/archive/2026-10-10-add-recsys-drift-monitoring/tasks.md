# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/monitoring/drift.py` (`calculate_psi`, `DriftLevel`, `FeatureDriftResult`, `DriftDetector`).
- [x] Implement `recsys/monitoring/__init__.py`.
- [x] Add unit tests in `platform-recsys/tests/test_drift.py`.

## 2. Code — the detector was never called (found 2026-10-09)
- [x] `recsys/monitoring/generation.py`: quantile sketches of the run's features, comparison with the champion's stored
      sketches, the drift record, the Prometheus file. Unit tests in `tests/test_drift_generation.py`.
- [x] Call it from `recsys/pipeline.py` before the structural gate: store `parameters.distribution` and `parameters.drift`
      and `metrics.drift_psi_max` on the model, report `drift` in the run summary, log, export when `DRIFT_METRICS_PATH`
      is set. New settings `DRIFT_ALERT_THRESHOLD`, `DRIFT_METRICS_PATH`. Spark-gated tests in
      `tests/test_pipeline_drift.py`.
- [x] README: drift section and known gaps.

## 3. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-drift-monitoring --strict`).
- [x] Run `pytest -v tests/test_drift.py` in `platform-recsys/`.
- [x] Scenario tests for the PSI math (`test_identical_distributions_have_zero_drift`,
      `test_shifted_distribution_triggers_significant_drift`) and `tests/test_drift_generation.py` in `make test-host`.
- [x] e2e (`platform-e2e` `features/recommendations/mlr_drift.feature`, step prefix `mlr_`): the real job image run twice.
      Needs the rebuilt `platform-recsys` image.

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.

- Final gate (2026-10-10): parallel lane 775/775 (w10-par) and 775/776 (w9-par; its one failure was the gateway-wide denylist gauge scenario, moved to the serial lane in e8373e00). Destructive lane 100/101 (w9-dfull); its one failure, backpressure, was fixed in 7f4ae454 and 4d325f2f and then passed twice in the outage-then-backpressure order.
