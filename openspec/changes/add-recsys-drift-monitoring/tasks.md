# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/monitoring/drift.py` (`calculate_psi`, `DriftLevel`, `FeatureDriftResult`, `DriftDetector`).
- [x] Implement `recsys/monitoring/__init__.py`.
- [x] Add unit tests in `platform-recsys/tests/test_drift.py`.

## 2. Code — the detector was never called (found 2026-10-09)
- [ ] `recsys/monitoring/generation.py`: quantile sketches of the run's features, comparison with the champion's stored
      sketches, the drift record, the Prometheus file. Unit tests in `tests/test_drift_generation.py`.
- [ ] Call it from `recsys/pipeline.py` before the structural gate: store `parameters.distribution` and `parameters.drift`
      and `metrics.drift_psi_max` on the model, report `drift` in the run summary, log, export when `DRIFT_METRICS_PATH`
      is set. New settings `DRIFT_ALERT_THRESHOLD`, `DRIFT_METRICS_PATH`. Spark-gated tests in
      `tests/test_pipeline_drift.py`.
- [ ] README: drift section and known gaps.

## 3. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-drift-monitoring --strict`).
- [x] Run `pytest -v tests/test_drift.py` in `platform-recsys/`.
- [ ] Scenario tests for the PSI math (`test_identical_distributions_have_zero_drift`,
      `test_shifted_distribution_triggers_significant_drift`) and `tests/test_drift_generation.py` in `make test-host`.
- [ ] e2e (`platform-e2e` `features/recommendations/mlr_drift.feature`, step prefix `mlr_`): the real job image run twice.
      Needs the rebuilt `platform-recsys` image.
