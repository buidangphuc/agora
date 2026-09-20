# ADR-0014 — Model Registry and Champion/Challenger Promotion Gate

**Status:** Accepted · **Date:** 2026-09-20 · **Relates to:** ADR-0011, ADR-0005

## Context

Recommendation and machine learning models in `platform-recsys` are retrained on scheduled batches. Previously, each training run immediately overwrote active serving keys or swapped versions without an automated validation gate.

Deploying unvalidated model artifacts introduces catastrophic risks:
1. **Model Degradation / Performance Regressions**: A noisy training run or corrupted warehouse batch could deploy an inferior model that degrades ranking metrics (NDCG@K, Recall@K).
2. **Popularity Collapse**: A model might optimize short-term loss while collapsing catalog coverage, hurting long-tail discovery.
3. **Lack of Auditability**: Serving artifacts in Redis/Qdrant need clear lineage linking them to training code git commit, warehouse dataset snapshot, and evaluation metrics.

## Decision

Establish an automated **Model Registry and Promotion Gate** in `platform-recsys`:

1. **Model Registry Metadata**: Every trained model produces a structured metadata manifest recording:
   - `model_name` & `model_version` (e.g., `als_20260920_090000`).
   - `model_type` (`als`, `two_tower`, `gbdt_ranker`).
   - `git_commit` & `training_timestamp`.
   - `eval_metrics` (`ndcg@10`, `recall@10`, `precision@10`, `coverage@10`, `map@10`).
   - `status`: `candidate`, `champion`, or `archived`.
   - `artifact_locations`: Qdrant collection name, Redis prefix.

2. **Automated Promotion Gate**:
   - Before a newly trained `candidate` is promoted to `champion`, it is evaluated against the current `champion` on holdout evaluation datasets using `ModelEvaluator.compare_models`.
   - **Promotion Criteria**:
     - Primary metric (`NDCG@10`) must not regress ($rel\_improvement \ge 0.0\%$, or $\ge \delta$ for challenger models).
     - Catalog coverage safety floor must be maintained ($coverage \ge 0.8 \times champion\_coverage$).
   - On passing the gate, the active champion version pointer in Redis (`recs:model:champion`) is updated atomically.
   - On failing the gate, the candidate remains flagged as `candidate` (or `rejected`), an alert/log is emitted, and the existing champion continues serving uninterrupted.

3. **Lineage and Rollback**:
   - Historical versions are preserved for $N$ generations (TTL/pruning policy) allowing zero-downtime rollbacks by pointing `recs:model:champion` back to a previous generation.

## Alternatives Rejected

- **Manual Approval Gate**: Incompatible with daily scheduled retraining and autonomous ML operations.
- **Blind Swap Without Metrics Gate**: High risk of silent degradation during data drift or transient pipeline errors.
- **Heavyweight External Model Registry (e.g., MLflow server / S3)**: Over-engineered for current architecture; a lightweight, Redis/metadata-backed registry in `platform-recsys` provides complete reproducibility with zero extra infra overhead.

## Consequences

- Retraining jobs will safely discard regressions without impacting live traffic.
- Serving services (`team-ai`) resolve model generation dynamically from the champion registry pointer.
- Model lineage is fully auditable.
