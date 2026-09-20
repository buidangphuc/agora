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

## 2. Code — the CLI evaluates a fixture, not a model (found 2026-09-20)
- [ ] Read interactions from the warehouse instead of the literal at `evals/__main__.py:15-22`
      (`u1`/`item1`…), reusing `recsys/warehouse.py:read_tracking_events`.
- [ ] Take predictions from a trained model rather than the hand-written `predicted` dict at
      `evals/__main__.py:26-29`. Until this lands, `make eval` cannot report on any model and
      cannot feed the promotion gate.
- [ ] Emit the report in a form the registry can consume as `ModelMetadata.metrics`.

## 3. Verification
- [x] Run `pytest -v tests/test_evals_temporal_cli.py`.
- [ ] `make eval` over a sample warehouse produces NDCG@10 for the ALS run, not for a fixture.
- [x] `openspec validate wire-temporal-split-eval-cli --strict`.
