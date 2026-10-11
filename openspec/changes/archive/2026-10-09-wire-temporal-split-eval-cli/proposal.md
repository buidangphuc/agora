## Why

`add-recsys-offline-eval` (P1-T2) delivered ranking metrics, and a later change added
`temporal_train_test_split` and `user_leave_k_out_temporal_split`. Grep shows both are referenced
only by `recsys/evals/__init__.py` (re-export) and their own tests. `evaluator.py` and
`evals/__main__.py` never call them: the CLI still receives ground-truth sets prepared by the
caller.

So `make eval` produces numbers without owning how the data was divided. That is the precise
condition under which a random or overlapping split silently leaks future interactions into
training and inflates every metric. **P1-T4's promotion gate is built on top of this**, so an
unsound split would propagate into model-promotion decisions.

## What Changes

- **platform-recsys** (`recsys/evals/__main__.py`, `evaluator.py`):
  - The CLI reads raw interactions and performs the temporal split itself.
  - The chosen cutoff, train event count and test event count are logged and returned in the
    report — the split becomes an auditable part of the result, not an assumption.
  - The report records the split strategy name so two reports are only comparable when the
    strategy and cutoff match.
  - Passing pre-split ground truth remains possible but is explicitly labelled in the report.

## Non-goals

- No new metrics.
- No change to the promotion gate itself (it consumes the report).
