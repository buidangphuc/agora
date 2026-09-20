## Why

Following **ADR-0014**, model retraining must not blindly overwrite active serving models. We need a model registry and an automated champion/challenger promotion gate to ensure that candidate models meet offline ranking metrics and catalog coverage standards before being promoted to live traffic.

## What Changes

- **platform-recsys** (`recsys/registry/`):
  - `ModelMetadata`: Dataclass tracking model name, version, type, commit hash, timestamp, evaluation metrics, and champion/candidate status.
  - `ModelRegistry`: Registry storing and retrieving model metadata manifests (in Redis or local metadata store), managing candidate registration, and executing automated promotion gates.
  - `PromotionGate`: Validates candidate models against active champions using `ModelEvaluator.compare_models` (primary metric `ndcg@10`, coverage floor).
  - Atomic pointer updates: Updates `recs:model:champion` in Redis only upon successful promotion.
  - Unit tests in `platform-recsys/tests/test_registry.py`.
- **platform-gitops**:
  - Reference configuration for scheduled retraining promotion.

## Non-goals

- No external SaaS dependencies (e.g. SageMaker / Comet / MLflow).
