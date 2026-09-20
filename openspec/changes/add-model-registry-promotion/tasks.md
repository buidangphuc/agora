# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/registry/metadata.py` (`ModelMetadata` data class with serialization).
- [x] Implement `recsys/registry/registry.py` (`ModelRegistry` supporting candidate registration, promotion gate execution, champion pointer updates).
- [x] Implement `recsys/registry/__init__.py`.
- [x] Write unit tests in `platform-recsys/tests/test_registry.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-model-registry-promotion --strict`).
- [x] Run `pytest -v tests/test_registry.py` in `platform-recsys/`.
