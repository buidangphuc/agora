## Why

The fine-tuning pipeline (ingest, validate, transform, train, evaluate) lives in a standalone repo, `/Users/phuc.buidang/Documents/work/MLOps`, with its own registry (`registry/{base,local,s3,stages}.py`), its own quality gate (`gate.py`, which raises and registers nothing on failure) and its own `ops/docker-compose.yml`. Agora trains other models, and the training-platform initiative puts every trained model behind one MLflow registry and one publish gate (`py-modelregistry`, delivered by `add-model-registry`). The finetune pipeline has to join it, run on a schedule with no manual step, and never move a champion except through that gate.

initiative: training-platform

## What Changes

- **New repo `platform-finetune`**, imported with `git subtree add --prefix=platform-finetune` from the MLOps repo branch `main` (a one-time operator step from a local clone; history kept; `artifacts/` is gitignored and not imported). It is a platform capability, not a `team-*` repo: no business tables, no proto.
- **Registry replaced.** `registry/{base,local,s3,stages}.py` are deleted. Both register sites (`pipeline/stages.py` `run_evaluate`, `tune/promote.py` `promote`) call py-modelregistry `submit` (= `evaluate`, then `apply` if `promotable`, per round-2 decision D1) for model `finetune-classifier`. `promote-stage`, `rollback` and `stage-path` CLI commands are removed in favour of `python -m modelregistry promote|rollback` (add-model-registry).
- **Policy (D10).** `platform-core/packages/py-modelregistry/policies.yaml` gets exactly `finetune-classifier: {primary: macro_f1, min_relative_improvement: 0.01, floors: {macro_f1: 0.5}}`. In the monorepo this file is at `platform-core/packages/py-modelregistry/policies.yaml`; the trainer resolves it by D8 (`policies_path` argument, then env `MODELREGISTRY_POLICIES`, then the packaged file). The job image sets `MODELREGISTRY_POLICIES` explicitly. A test asserts the three values and that the file loads with all four entries.
- **Gate config removed.** `gate.enabled` and `gate.on_fail` are removed from `config/params.yaml`, `GateConfig`/`gate.py`/`QualityGate` are removed, and the `tune/loop.py` use of the gate is replaced by reading the objective metric only. `GateFailed` is never raised on the register path. The old `gate.rules` floor now lives only in `policies.yaml`.
- **Decision semantics.** The model is always saved to disk. `submit` registers every candidate as a version. Pass: `champion` moves, old holder becomes `previous`. Reject (below floor, insufficient improvement, or a missing or non-finite `macro_f1`, D3): version tagged `gate.outcome=rejected` with a reason, aliases untouched, exit 0.
- **Exit codes (D4):** 0 = completed including `rejected`; 1 = `RegistryUnavailable` or an unexpected stage exception; 2 = usage or config error, `PolicyError`, `GateError`, unknown version.
- **Pre-flight (D5).** The pipeline checks registry reachability at the start, before ingest and training; unreachable means exit 1 with no training. Each library operation has a 12 s total deadline; at the end of a run the submit fails with exit 1 within 15 s of a closed port (12 s library deadline plus 3 s process margin).
- **Lineage (D9, D11).** The trainer creates the MLflow run and passes its `run_id` to `submit`; the version's source is that run (no run-id tag). Tags: `lineage.data_version`, `lineage.train_hw` (the `_hw_id` string, or `cpu` on a CPU-only host) and `lineage.eval_protocol` (D2: first 12 hex chars of sha256 over dataset name, split, eval-set `data_version` and metric config; a champion with a different or absent value means the candidate is judged on floors only).
- **Packaging and scheduling.** A training `Dockerfile`; compose service `platform-finetune` under profile `jobs` in `docker-compose.services.yaml` (MLflow `http://mlflow:5050`, MinIO from agora infra); a CronJob under `platform-gitops/platform/finetune/` with schedule `0 4 * * 0`, `backoffLimit: 1`, `restartPolicy: Never`, `concurrencyPolicy: Forbid`, GPU nodeSelector and toleration (ADR-0011), `nvidia.com/gpu: 1`, a NetworkPolicy (egress: DNS, mlflow, minio, TCP 443 to `0.0.0.0/0` except cluster CIDRs for the HF dataset and base model) and an ArgoCD Application.
- **CI.** Job `platform-finetune` in `.github/workflows/services-ci.yml`, `timeout-minutes: 10`, runs the offline pytest (no GPU, no dataset).
- **Dropped:** `ops/docker-compose.yml`. **Not ported, kept as is:** `serve/local.py`, `package.py`, `optimize.py`, `serve_analyzer.py`, `serve/triton.py`, `serve/deploy_sagemaker.py` get a "NOT PORTED" header; no compose service or manifest; the offline suite never imports them. `perf/` is imported unchanged.
- **Removed tests and their replacement.** `tests/test_registry_stages.py` (`test_assign_resolve_and_path`, `test_rollback_restores_previous`, `test_guards`) is replaced by py-modelregistry's own promote/rollback tests (add-model-registry) and `test_registry_gate.py::test_pass_moves_champion`. `tests/test_registry_remote.py` (`test_register_local_then_s3_in_lockstep`, `test_s3_pull_resolves_latest`, `test_s3_backend_is_registered_in_seam`) is replaced by `test_register_sites_write_to_registry` (both sites, file store) and `test_registry_unreachable_fails_closed`. The assertion on `cm.gate().rules` in `tests/test_finetune.py` (line 88) becomes an assertion on `cm.registry().name`.
- `FEATURES.yaml` is created for `platform-finetune` by the e2e-author.

## Non-goals

- No consumer: `team-ai`, `platform-modelserve` and `team-gateway` do not change. Nothing serves the finetune champion.
- No LLM or chatbot wiring; the `mock` provider stays the default everywhere.
- No change to py-modelregistry itself, MLflow infra or the ADR (add-model-registry). This slice only adds its policy entry.
- No orchestrator, no HPO changes, no change to the training logic, the perf toolkit or the tune loop.
- No migration of historic local registry versions from MLOps `artifacts/registry`.
- No change to the exit-code or deadline semantics of py-modelregistry; this slice follows them.

## NFR

- Each py-modelregistry operation has a 12 s total deadline (D5); a registry outage fails the submit with exit 1 within 15 s, with the champion untouched. The pre-flight fails the job before training when the registry is down.
- The gate decision is deterministic for the same metrics, policy and champion.
- The weekly job is unattended: concurrent runs are forbidden, `backoffLimit` is 1, and a failed run leaves the champion unchanged.
- CI offline pytest needs no GPU, no network and no dataset; the CI job has `timeout-minutes: 10`.
- Availability: the finetune job is batch only; nothing depends on it being up.

## Data classification

| Field | Purpose | Class | Consent basis | Retention |
|---|---|---|---|---|
| AG News articles and labels (HF `fancyzhx/ag_news`) | training and evaluation text | public dataset, no personal data | n/a (public) | training sample is ephemeral in the job pod volume |
| Model weights (Qwen2.5-0.5B fine-tune) | registered artifact in `s3://mlflow-artifacts` | not personal | n/a | keep all versions; pruning is out of scope |
| Metrics, params, `data_version`, `train_hw` (GPU model, compute capability, VRAM, count) | gate input and lineage | not personal | n/a | same as the MLflow run |
| `misclassified examples` dump (`error_dump`) | error analysis | public text | n/a | stays on local disk, not uploaded to MLflow |

No user ids, interaction rows or other personal data enter this repo, MLflow or MinIO from this slice.

## Threat model

- **Assets:** the `champion` alias of `finetune-classifier` (integrity); training compute on the GPU pool; MLflow and MinIO credentials.
- **Abuse cases:**
  1. A weak or poisoned candidate becomes champion. Mitigation: only `submit` moves the alias, and a version below the floor or the improvement rule is `rejected`.
  2. A registry outage makes the job publish anyway. Mitigation: fail closed, no in-memory or local fallback for the decision.
  3. A manual run overlaps the scheduled one and races the alias. Mitigation: `concurrencyPolicy: Forbid`.
  4. The training pod reaches unrelated services. Mitigation: NetworkPolicy allowing DNS, MLflow, MinIO and egress for the dataset and base-model download only (ADR-0010).
  5. Secrets leak through the image or git history. Mitigation: MinIO/MLflow credentials come from env sourced from Vault refs in gitops; detect-secrets over the imported tree and history reports zero findings.
- **Authz rule:** no end-user surface. Only the job's service identity writes to MLflow. Manual `promote --force` is an engineer action defined by add-model-registry and is recorded on the version. MLflow is cluster-internal only (no ingress).
