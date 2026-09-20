# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/monitoring/drift.py` (`calculate_psi`, `DriftLevel`, `FeatureDriftResult`, `DriftDetector`).
- [x] Implement `recsys/monitoring/__init__.py`.
- [x] Add unit tests in `platform-recsys/tests/test_drift.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-drift-monitoring --strict`).
- [x] Run `pytest -v tests/test_drift.py` in `platform-recsys/`.
