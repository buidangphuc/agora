# Tasks

## 1. Code — platform-featurestore
- [x] Scaffold `platform-featurestore/` repo: `pyproject.toml`, `requirements.txt`, `Makefile`, `Dockerfile`, `README.md`, `FEATURES.yaml`.
- [x] Implement `featurestore/definitions.py` (`UserFeatures`, `ItemFeatures`).
- [x] Implement `featurestore/online.py` (`OnlineFeatureStore` with Redis backend and in-memory fallback).
- [x] Implement `featurestore/offline.py` (`OfflineFeatureStore` for batch training datasets).
- [x] Implement `featurestore/parity.py` (`validate_parity`).
- [x] Implement `featurestore/__init__.py`.
- [x] Write unit and parity tests in `platform-featurestore/tests/`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-platform-featurestore --strict`).
- [x] Run `pytest -v tests/` in `platform-featurestore/`.
