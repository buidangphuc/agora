# Tasks

## 1. Code — platform-recsys
- [x] Implement `recsys/evals/metrics.py` (ndcg, recall, precision, map, mrr, hit_rate, catalog_coverage).
- [x] Implement `recsys/evals/evaluator.py` (`ModelEvaluator` class aggregating user metrics and reporting dataset summary).
- [x] Implement `recsys/evals/__init__.py` and `recsys/evals/__main__.py` CLI.
- [x] Add `make eval` target in `platform-recsys/Makefile`.
- [x] Write comprehensive unit tests in `platform-recsys/tests/test_evals.py`.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-recsys-offline-eval --strict`).
- [x] Run `pytest -v tests/test_evals.py` in `platform-recsys/`.
