# Tasks

> **Reality check 2026-09-20** — tasks below were done as written: `evals/__main__.py` does call
> `evaluator.evaluate(raw_interactions=..., split_k=1)`, and `evaluate()` (`evaluator.py:29-41`)
> does take `split_strategy` / `cutoff_timestamp` / `train_events_count` / `test_events_count`
> and record them. The gap is that the CLI still runs on a 6-row literal, so `make eval`
> reports a sound split over data that means nothing.

## 1. Code — platform-recsys
- [x] Integrate temporal split metadata into `recsys/evals/evaluator.py:evaluate()`.
- [x] Update `recsys/evals/__main__.py` to run temporal split over raw interactions.
- [x] Write execution-proof tests in `tests/test_evals_temporal_cli.py`.

## 2. CLI over the warehouse and a trained model — retired (2026-10-09)
The batch pipeline already evaluates each trained generation on the warehouse dataset, with the temporal protocol,
in `pipeline.evaluate_generation`. It feeds the promotion gate and `ModelMetadata.metrics`, as the archived
wire-pipeline-eval-registry specifies. A second, CLI-side path to the same report is therefore dropped.
`evals/__main__.py` stays a fixture demo of the split metadata.

## 3. Verification
- [x] Run `pytest -v tests/test_evals_temporal_cli.py`.
- [x] `openspec validate wire-temporal-split-eval-cli --strict`.
