## Why

Every model agora trains needs one place to live and one rule for becoming the served version. Today there are two private registries: recsys keeps champions in Redis (ADR-0014), and the MLOps fine-tune pipeline has its own `registry/{base,local,s3,stages}.py`. The ADR-0013 revisit trigger for MLflow is met: four models are trained concurrently and rollback is needed. A gate that lives inside each trainer drifts per trainer. This change builds the shared foundation the other two slices (`recsys-auto-train`, `add-platform-finetune`) consume. It ships the registry server, the gate library and the policy file. It migrates no trainer.

initiative: training-platform

## What Changes

- **ADR-0017 "Unified model registry and publish gate"** in `platform-core/docs/ADR/`, following the structure of `0011-model-serving.md`. It supersedes ADR-0014 (Redis registry) and the MLflow rejection in ADR-0013 Alternatives. ADR-0014 and ADR-0013 get a "Superseded by ADR-0017" status line.
- **MLflow server v3.2.0** in `platform-core/infra/docker-compose.yaml`:
  - Service `mlflow` on `:5050`.
  - Its own `postgres-mlflow` as backend store (DB-per-service).
  - Artifacts on the existing MinIO bucket `mlflow-artifacts`, created by `minio-init`.
- **gitops** under `platform-gitops/platform/mlflow/` (raw manifests + kustomize, same shape as `platform/recsys`), plus `platform-gitops/argocd/apps/mlflow.yaml`. Cluster-internal Service only, no ingress; credentials via Vault refs (External Secrets).
- **`platform-core/packages/py-modelregistry`**, a shared Python library (precedent: `packages/go-sdk`, ADR-0016):
  - Two-phase API: `evaluate(model, run_id, metrics, artifact_path, lineage) -> Decision` (registers and tags the version `promotable` or `rejected`, moves no alias), `apply(model, version)` (moves `champion`, old holder becomes `previous`), `mark_rejected`, `register_rejected`, and the convenience `submit` = evaluate then apply. Also `promote(model, version, force, reason)`, `rollback(model)` (swap) and `seed(...)` (gate bypass for an empty model).
  - Aliases `champion` and `previous`.
  - Version tags `gate.outcome` (`promotable` | `promoted` | `rejected`), `gate.reason`, `gate.forced`, `lineage.data_version`, `lineage.eval_protocol`, `lineage.train_hw`.
  - Evaluation-protocol rule: a different or absent `eval_protocol` on the champion means floors-only judgement.
  - Exit codes: 0 completed (including `rejected`), 1 registry unavailable or unexpected error, 2 usage/config/policy/gate/rollback error.
  - Policy file resolved from the `policies_path` argument, then env `MODELREGISTRY_POLICIES`, then the packaged file.
  - CLI `python -m modelregistry promote <model> <version> --force --reason <text>` and `rollback <model>`.
  - `policies.yaml` schema `<model>: {primary, min_relative_improvement, guards, floors}`. The file ships with no model entries and one documented, commented example. `recsys-auto-train` and `add-platform-finetune` add their own entries.
  - Fail-closed: an unreachable registry raises an error (non-zero exit in the CLI). Each call is bounded by 10 s and each operation by a 12 s total deadline; a reachability check lets trainers fail before training.
  - Deterministic decisions: the same metrics and policy always give the same Decision.
- **services-ci** job `py-modelregistry` in `.github/workflows/services-ci.yml`: lint, types, offline pytest against a local MLflow file store.

## Non-goals

- Migrating recsys or finetune (the Redis seed migration, removal of the per-trainer registries). Those are `recsys-auto-train` and `add-platform-finetune`.
- A registry service with its own API. The registry is MLflow; the gate is a library run inside each trainer.
- Auth on MLflow (none locally; cluster-internal only in gitops), serving champions (modelserve), proto, RPC or Kafka changes.
- Policy entries for any real model.

## Impact

- Repos touched: `platform-core` (ADR, `infra/docker-compose.yaml`, `packages/py-modelregistry`), `platform-gitops`, `.github/workflows/services-ci.yml`.
- Capabilities: `model-registry` (new), `model-publish-gate` (new).
- Hot files: `.github/workflows/services-ci.yml` (shared with `add-platform-finetune`), `policies.yaml` (created here, extended by both later slices).
- New dependency: `mlflow-skinny` client in `py-modelregistry`, `pyyaml`.

## Non-functional requirements (NFR)

- Latency and bound: each registry call times out within 10 s; every operation (`evaluate`, `apply`, `submit`, `promote`, `rollback`, `seed`) has a 12 s total deadline across all its calls, including when the registry answers once and then stalls.
- Availability: the registry is on the publish critical path. An outage fails the run fast and leaves what is served untouched. Serving never reads MLflow in this initiative.
- Concurrency: single writer per model, assumed from `concurrencyPolicy: Forbid` on the consumer CronJobs and recorded in ADR-0017; no lock in the library. An outage mid-evaluate leaves the version `pending`; a crash between evaluate and apply leaves it `promotable`; neither moves an alias.
- Determinism: the Decision is a pure function of candidate metrics, champion metrics and the model's policy.
- Limits: policies are read from one file; a model without a policy entry is never promoted.
- Offline testability: the library's tests run against an MLflow file store with no network.

## Data classification

No personal data is introduced or stored by this slice. The fields it handles are all non-personal:

| Field | Purpose | Consent basis | Retention |
|---|---|---|---|
| model name, version, run id | registry identity | n/a (not personal) | life of the registered model |
| metrics (aggregate scalars) | gate input | n/a | life of the version |
| `gate.*` tags (outcome, reason, forced) | audit of the decision | n/a | life of the version |
| `lineage.data_version`, `lineage.train_hw` | provenance | n/a | life of the version |
| `--reason` free text | audit of a forced promotion | n/a | life of the version |

Constraint for consumers (recorded in ADR-0017): MLflow runs, params, tags and artifacts MUST NOT contain user ids, interaction rows or per-user lists. Only aggregate params and metrics and model artifacts are allowed. The recsys ALS item factors may be uploaded; user factors may not. The library does not inspect artifacts, so enforcement is each consumer's duty and is tested in the consumer's slice. Operators must not put personal data in `--reason`.

## Threat model

- **Assets:** the `champion` alias (it decides what gets published), the model artifacts in `mlflow-artifacts`, the audit tags.
- **Abuse cases and mitigations:**
  - A bad or regressed candidate gets published. Mitigation: the single gate; `apply` is refused unless the version is `promotable`; a failing candidate leaves alias and serving untouched. A forced promote or rollback changes the registry only, never a consumer's serving state.
  - Registry outage tempts a trainer to publish anyway. Mitigation: no in-memory fallback; error or non-zero exit.
  - A silent override. Mitigation: `--force` requires a non-empty `--reason` and records `gate.forced=true` and the reason on the version.
  - A model with no policy slips through. Mitigation: unknown model raises a policy error and nothing is promoted.
  - Personal data leaks into MLflow. Mitigation: consumer constraint above; MLflow is not exposed outside the cluster.
  - Unauthenticated access to MLflow. Mitigation: no ingress in gitops, ClusterIP only, credentials from Vault; locally the port is dev-only.
- **Authz rule:** there is no end-user authz. Anyone with network reach to MLflow can move aliases, so reach is limited to the trainers and engineers inside the cluster. Forced promotion is an engineer action through the CLI, always attributed by a mandatory reason. Hardening (MLflow auth, per-caller identity in the audit tags) is out of scope and listed in ADR-0017 as follow-up.
