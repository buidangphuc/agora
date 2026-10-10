## Context

`recsys/monitoring/drift.py` (PSI, `DriftDetector`) was a library no run called. This change wires it into
`recsys/pipeline.py` against the generation being replaced.

Reconciled with the archived AI-first changes (`recsys-generation-publish`, `featurestore-datasets`):

- The job trains only on the governed dataset, so the "training distribution" is the dataset's pairs, read from the same
  Spark frame the job already built. There is no second data source and no raw-events path.
- A generation is a `model_version`; the generation a run replaces is the registry champion at run time (the one that
  is serving, or the one `python -m recsys rollback` restored). Drift is therefore computed against `champion`, not
  against a stored "previous" pointer, and follows a rollback.

## Decisions

### D1. What is compared
Four features: `weight` (the dataset pairs' weight), `user_items` (distinct listings per user), `item_users` (distinct users
per listing) and `top_score` (each user's best recommendation score). The ALS factors are deliberately not compared: two
trainings of the same data differ by an arbitrary rotation, so PSI over factor components would flag every run.

### D2. A quantile sketch is the stored baseline
A run stores 101 equally spaced quantiles per feature (`parameters["distribution"]`, about 400 numbers). A sketch is an
equal-mass sample, so `calculate_psi(baseline_sketch, current_sketch)` works unchanged and nothing row-level is kept.
Dataset-side sketches use Spark `approxQuantile` (relative error 0.001), so memory on the driver stays constant.

### D3. Observational, never a gate
The verdict is logged (WARNING when flagged), written to the metadata and summary, and optionally exported. It does not
change `decision`. A real distribution shift is often exactly why a retrain is wanted; blocking it would stop the model
from adapting. Alerting on `recsys_model_drift_alert` is an operations concern.

### D4. Where it is recorded
- `parameters.distribution`: the run's sketches (the next run's baseline).
- `parameters.drift`: `{status: no_baseline|ok|drifted, baseline_version, threshold, is_drifted, num_features_drifted,
  max_psi, features: {name: {psi, drift_level}}}`.
- `metrics.drift_psi_max`: the largest PSI (only with a baseline). The promotion gate reads only the metrics it names, so
  this is inert for it.
- The run summary carries the same `drift` record. Rejected candidates carry it too (it explains a degenerate run).
- `DRIFT_METRICS_PATH` (default empty): Prometheus text via `DriftDetector.to_prometheus_metrics`, written atomically. The
  job is a batch with no HTTP surface, so a textfile collector is the way to expose it.

## Risks

- A champion registered before this change has no stored distribution: its successor reports `no_baseline`, and the
  next run after that has a baseline.
- Two datasets of very different size can differ in `weight` legitimately (a longer window): the flag is a prompt to look,
  by design not an action.
