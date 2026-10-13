# modelregistry

Unified model registry client and publish gate on MLflow (ADR-0017). Python >= 3.10,
depends on `mlflow-skinny` 3.2.x and `pyyaml`.

## Usage

```python
from modelregistry import check_reachable, evaluate, apply, submit, mark_rejected

check_reachable()                       # trainer pre-flight, raises RegistryUnavailable
d = evaluate("my-model", run_id, {"ndcg@10": 0.31}, "/tmp/model_dir", {
    "data_version": "ds@v3", "eval_protocol": "p1", "train_hw": "cpu"})
if d.outcome == "promotable":
    apply("my-model", d.version)        # or submit(...) = evaluate then apply
```

Other calls: `register_rejected`, `mark_rejected`, `promote(model, version, force=False, reason=None)`,
`rollback(model)` (swaps `champion` and `previous`), `seed(...)` (first champion, bypasses the gate,
only for a model with no versions), `get_champion(model)`.

CLI: `python -m modelregistry promote <model> <version> [--force --reason TEXT]` and
`python -m modelregistry rollback <model>`. A JSON decision goes to stdout. Exit codes: 0 done
(including a `rejected` decision), 1 registry unavailable or unexpected error, 2 usage, policy,
gate or rollback error.

Configuration is read at call time: `MLFLOW_TRACKING_URI`, `MLFLOW_HTTP_REQUEST_TIMEOUT` (capped at
10 s per call), `MODELREGISTRY_POLICIES` (policy file; otherwise the packaged `policies.yaml`).
Each operation has a 12 s total deadline; when the registry does not answer the gate fails closed
with `RegistryUnavailable`.

Metric names containing `@` are stored in MLflow as `_at_` (`ndcg@10` -> `ndcg_at_10`) and decoded
when read; callers always use the `@` form.

## Version states

`pending` (registered, not yet decided) -> `promotable` or `rejected` (by `evaluate`) -> `promoted`
(by `apply`, `promote` or `seed`). `evaluate` never moves an alias; a crash between `evaluate` and
`apply` leaves the version `promotable` and the aliases unchanged. The library takes no lock: it
assumes a single writer per model.

## Policy

One entry per model in the policy file: `primary` (required), `min_relative_improvement` (default 0),
`guards` (candidate >= ratio x champion), `floors` (candidate >= value). Thresholds are inclusive.

## Privacy rule for callers

MLflow must not contain person identifiers: no interaction rows and no per-person lists in runs,
tags or artifacts. Callers log params and long values (for example sketches) to the run themselves;
anything over the MLflow param limit goes in as a JSON artifact.
