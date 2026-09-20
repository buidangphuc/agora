# Tasks

## 1. Code — platform-recsys
- [x] Integrate temporal split metadata into `recsys/evals/evaluator.py:evaluate()`.
- [x] Update `recsys/evals/__main__.py` to run temporal split over raw interactions.
- [x] Write execution-proof tests in `tests/test_evals_temporal_cli.py`.

## 2. Verification
- [x] Run `pytest -v tests/test_evals_temporal_cli.py`.
- [x] `openspec validate wire-temporal-split-eval-cli --strict`.
