# ADR-0017 — Unified model registry and publish gate

**Status:** Accepted · **Date:** 2026-10-13 · **Supersedes:** ADR-0014 (whole), ADR-0013 (only the MLflow rejection in Alternatives) · **Relates to:** ADR-0011, ADR-0013, ADR-0014, ADR-0016

## Context

Every model agora trains needs one place to live and one rule for becoming the served version. Today there are two private registries. `platform-recsys` keeps champions in Redis (ADR-0014), and the imported MLOps fine-tune pipeline has its own `registry/{base,local,s3,stages}.py`. A promotion gate embedded in each trainer drifts per trainer: thresholds, metric handling and failure behaviour are re-implemented and diverge.

ADR-0013 rejected MLflow "for now" — three stateful components to version one nightly artifact — and recorded its own revisit trigger: more than one concurrently served model, or a need to roll back to a prior artifact. Both now hold. Four models are trained (`recsys-als`, `recsys-gbdt`, `recsys-two-tower`, `finetune-classifier`), and rollback to a prior artifact is a requirement, not a nicety. The ADR-0014 pointer-in-Redis design also cannot hold a fine-tuned transformer or give a uniform audit trail across model families.

Two properties must survive the move. A gate failure or a registry outage must never change what is served. And the gate decision must be reproducible: same metrics, same policy, same answer.

## Decision

One registry, one gate, one policy file.

- **Registry: MLflow Model Registry, server v3.2.0**, in-cluster on `:5050` (`MLFLOW_TRACKING_URI=http://mlflow:5050`). Backend store is its own `postgres-mlflow` (DB-per-service); artifacts live on the existing MinIO bucket `mlflow-artifacts`, created by `minio-init`. In Kubernetes it has a cluster-internal Service only, no ingress, credentials from Vault via External Secrets. Registered models: `recsys-als`, `recsys-gbdt`, `recsys-two-tower`, `finetune-classifier`.
- **Aliases carry serving intent.** `champion` is the version that should be served; `previous` is the prior holder. Stage-style transitions are not used.
- **Tags carry the audit trail.** `gate.outcome`, `gate.reason`, `gate.forced`, `lineage.data_version`, `lineage.eval_protocol`, `lineage.train_hw`.
- **The gate is a library, not a service**: `platform-core/packages/py-modelregistry` (package `modelregistry`; precedent `packages/go-sdk`, ADR-0016), run inside each trainer. There is no registry API of our own to operate. Policies live in one file, `policies.yaml`, resolved as the `policies_path` argument, then env `MODELREGISTRY_POLICIES`, then the packaged copy. Policy shape: `<model>: {primary, min_relative_improvement, guards: {metric: min_ratio}, floors: {metric: min}}`. Thresholds are inclusive, compared with a 1e-9 tolerance.
- **The ADR-0014 rule is kept as the default policy shape**: relative improvement on a primary metric plus guard ratios against the champion. `recsys-als` keeps `ndcg@10` at +1 % and `coverage@10` at 0.8x; the other models get their own numbers in `policies.yaml`.
- **Two-phase API, so publishing can sit between decision and switch.**
  - `evaluate(...)` registers a version, logs metrics and the artifact, tags `gate.outcome` as `promotable` or `rejected`, and moves no alias. It returns `Decision{version, outcome, reason}`.
  - `apply(model, version)` moves `champion` (old holder becomes `previous`) and sets `gate.outcome=promoted`. It is refused with `GateError` unless the version is `promotable`.
  - `mark_rejected` and `register_rejected` record a failure after a downstream stage or a structural problem. `submit` is `evaluate` then `apply`, for trainers with no publish step.
  - Operator verbs: `promote --force --reason` (override, recorded on the version), `rollback` (swap `champion` and `previous`, so a second rollback toggles back), and `seed` (bypass the gate, only for a model with no versions, used to import the current Redis champion as version 1).
- **eval_protocol.** If candidate and champion `lineage.eval_protocol` differ, or the champion has none, metrics are not compared; the candidate is judged on `floors` only and the reason says `eval_protocol changed`. This keeps the ADR-0014 "different protocol" behaviour, now bounded by floors.
- **Missing metrics never raise.** A missing or non-finite primary, guard or floor metric on the candidate makes it `rejected`, naming the metric. A guard missing on the champion is skipped and noted. A champion primary of 0 or below skips the ratio and requires a strict improvement plus floors.
- **Fail closed.** The registry is on the publish critical path. If it is unreachable no gate can run, the trainer publishes nothing and exits non-zero, and serving keeps the last champion. Each registry call times out in 10 s; each library operation has a 12 s total deadline across all its calls. Trainers run a reachability pre-flight before training starts.
- **Per-trainer registries are removed** (recsys `recsys/registry/`, finetune `registry/*`). Recsys serving keys and the Qdrant collection stay as they are; only the champion decision moves.

### Version states

The state of a version is the value of tag `gate.outcome`.

| State | Meaning | Reached by |
|---|---|---|
| `pending` | registered, no `gate.outcome` tag yet | version creation inside `evaluate` |
| `promotable` | passed the gate, no alias moved | `evaluate` |
| `promoted` | holds or held `champion` | `apply`, `promote --force`, `seed` |
| `rejected` | failed the gate or a later stage | `evaluate`, `mark_rejected`, `register_rejected` |

A crash between `evaluate` and `apply` leaves the version `promotable` and every alias unchanged, so serving is unaffected; later runs ignore stale `promotable` versions and never apply them.

### Single-writer assumption

The library assumes a single writer per model. That is guaranteed operationally by `concurrencyPolicy: Forbid` on the trainer CronJobs, one CronJob per model family. No lock is taken in the library, and no compare-and-set on aliases: MLflow offers none, and a lock service would add a component to protect a case the scheduler already excludes. Manual `promote`, `rollback` and `seed` are operator actions and must not be run while that model's trainer job is active.

### Exit codes (library CLI and trainers)

| Code | Meaning |
|---|---|
| 0 | completed, including a `rejected` decision |
| 1 | `RegistryUnavailable`, or an unexpected stage exception |
| 2 | usage or config error, `PolicyError`, `GateError`, `RollbackError`, unknown version, stale dataset |

A `rejected` decision is a normal outcome, not a failed job; the workflow stays green and the previous generation keeps serving.

### Consumer rule: no person identifiers in MLflow

MLflow runs, tags and artifacts must not contain person identifiers or per-person data. That means no interaction rows, no per-person lists (per-user recommendation lists, per-user histories), and no raw user or buyer ids in runs, tags, artifacts or params. Only aggregate params and metrics, lineage tags, and model artifacts are allowed; ALS item factors are item-side and permitted. Trainers that need to log long values log them as JSON artifacts, which are subject to the same rule. MLflow has no authentication in this stage, which is why the rule is strict.

### Out of scope, recorded as follow-ups

- MLflow authentication and per-model authorization; today access is network-level only (ADR-0010 NetworkPolicy).
- `platform-modelserve` and `team-ai` loading registry champions; serving is unchanged by this ADR.
- Mirroring a forced promote or rollback into recsys serving: recsys serving changes only through the recsys job or its own rollback command, which calls the library.

## Alternatives rejected

- **Keep ADR-0014's Redis registry and add one per new trainer.** No new infrastructure, but each registry and gate drifts, and Redis cannot hold large artifacts or a uniform lineage record. This is the status quo the ADR replaces.
- **A bespoke registry service with its own API.** Full control, but we would own an API, storage and a UI to rebuild what MLflow already provides.
- **MLflow stage transitions (`Staging` / `Production`).** Deprecated upstream in favour of aliases; aliases also give us `previous` for free.
- **A gate inside each trainer.** Rejected for drift; the library makes the rule single-sourced and testable offline.
- **A distributed lock or alias compare-and-set.** Unnecessary while `concurrencyPolicy: Forbid` holds; revisit if trainers ever run concurrently per model.
- **Single-phase `submit` only.** Simpler, but recsys must publish to Redis/Qdrant between the decision and the alias switch, and a failed publish would leave `champion` pointing at something not served. Two phases keep `previous` intact.
- **Manual approval gate.** Incompatible with nightly retraining.

## Consequences

- One more stateful stack to run (MLflow, `postgres-mlflow`, a MinIO bucket), the cost ADR-0013 deferred; it is now justified by four models and the need to roll back.
- The registry is on the publish critical path. Outages stop publishing (exit 1) but never change serving.
- ADR-0014 is superseded; its gate numbers survive as policy. ADR-0013 stays in force except for its MLflow rejection, whose revisit trigger is met.
- Recsys and finetune gain a `mlflow-skinny` client dependency through `py-modelregistry`.
- The single-writer guarantee is operational, not enforced in code; a CronJob misconfigured without `Forbid` could race on aliases.
- Metrics comparison depends on `lineage.eval_protocol` being set honestly; a changed protocol drops the comparison to floors only.
- No proto, gateway or Kafka changes.
