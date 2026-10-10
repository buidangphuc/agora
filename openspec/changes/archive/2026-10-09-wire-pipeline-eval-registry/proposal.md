## Why

`recsys/evals/` and `recsys/registry/` both exist, both have passing unit tests, and neither has
a production call site. `recsys/pipeline.py:13-22` is the complete list of what the batch job
imports; neither module appears in it. Nothing writes a `ModelMetadata`, and
`evaluate_and_promote` has never been called by anything but a test.

The consequence is not cosmetic. `ModelEvaluator.compare_models` defaults to
`min_relative_improvement=0.0` and reads `metrics`, a dict the pipeline never populates — so the
promotion gate compares two empty dicts against a zero threshold. **A gate in that state always
opens.** ADR-0014 describes a champion/challenger promotion discipline that has never adjudicated
anything.

This blocks the whole track. `plan-mlops/INDEX.md` sets the rule *"ALS không bị xoá cho tới P3-T3
thắng trên metric. Nó là baseline duy nhất."* — correct, but there is no metric. Until a run
produces a real NDCG@10 for ALS, every later claim that two-tower or a GBDT ranker "improves"
something is unverifiable, and the promotion gate that is supposed to protect serving is
decorative.

Measurement is therefore the first wire, not the last.

## What Changes

**platform-recsys** (`recsys/pipeline.py`, `recsys/evals/`, `recsys/registry/`, `recsys/config.py`):

- `run()` gains an evaluation stage after ALS training: a temporal holdout is scored with the
  existing `ModelEvaluator`, producing NDCG/Recall/Precision/MAP/MRR and `coverage@k` for the
  generation just trained. No new metric is written — `evals/` already computes all of them.
- The run is registered as a `ModelMetadata` candidate carrying those metrics, its
  `model_version`, `model_type: "als"`, `git_commit`, and the Qdrant/Redis locations it wrote.
- `evaluate_and_promote` is called with the candidate. The champion pointer
  (`recs:model:champion`) moves only when the gate passes.
- **Serving artifacts follow the decision, not the run.** Today the pipeline upserts to Qdrant,
  prunes the previous generation, and flips `recs:v1:model_version` unconditionally — so a bad
  model is already serving before any gate could object. The write order changes so a rejected
  candidate leaves the previous generation intact and serving.
- New settings for the holdout window and the promotion thresholds, added to `_FIELDS` and
  `.env.example` under the existing drift gate.
- The pipeline summary reports the metrics and the promotion decision, so a CronJob log answers
  "did this run ship, and why".

## Non-goals

- **No new metrics.** `evals/metrics.py` and `evaluator.py` are used as they are.
- **No model replacement.** ALS stays the baseline and is not removed; this change exists to
  give it a number, which is the precondition for anything replacing it.
- **No external registry.** ADR-0014 rejects MLflow/S3 as over-engineered for this scale; the
  Redis-backed registry already in the repo is the target.
- No change to the shape of the Qdrant payload or the Redis key schema — only to *when* they
  are written.
- No serving-side change. team-ai reading the real `model_version` is a separate concern.
