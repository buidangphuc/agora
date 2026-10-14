# 00-spec — Training platform (one model registry + publish gate, auto-train recsys, MLOps import)

initiative: training-platform

## Goal
Every model agora trains lives in **one model registry** (MLflow Model Registry, artifacts on the existing
MinIO). A model version becomes `champion` only by passing **one shared publish gate**, whose per-model
policy is declared in one file. Training runs on a schedule end to end with no manual step: recsys (ALS,
GBDT, two-tower) nightly, and the fine-tune pipeline imported from the standalone MLOps repo as
`platform-finetune` on its own schedule. A gate failure or a registry outage never changes what is served.

## User stories
- **One registry.** Given the infra stack is up, when any trainer (recsys ALS, GBDT, two-tower, finetune)
  finishes a run, then MLflow on :5050 shows the run (params, metrics) and a new version of that registered
  model, tagged with its gate outcome (`promoted` | `rejected` + reason) and lineage (data version, run id,
  train hardware where known).
- **Publish gate.** Given a registered model with a `champion`, when a candidate version is submitted, then
  the shared gate compares it with the champion using that model's policy from the one policy file (primary
  metric + minimum relative improvement + guard ratios). Pass → `champion` moves to the candidate (the old
  holder becomes `previous`) and the trainer publishes it. Fail → the version stays `rejected`; alias and
  serving are untouched.
  - Edge: no champion yet → the candidate must meet the policy's absolute floors to become the first champion.
  - Edge: registry unreachable → no gate can run → the trainer publishes nothing and exits non-zero
    (fail-closed; no in-memory fallback).
- **Override and roll back.** When an engineer runs `python -m modelregistry promote <model> <version> --force
  --reason <text>`, then the alias moves and the override is recorded on the version; `rollback <model>` moves
  `champion` back to `previous`.
- **Recsys auto-train, end to end.** Given the nightly schedule, when the featurestore dataset job finishes,
  then the recsys job trains ALS, GBDT and two-tower, submits each to the gate, and publishes only the
  promoted ones (new generation pointer in Redis; two-tower vectors in Qdrant); team-ai then serves
  recommendations from that generation with no change on its side.
  - Edge: a candidate fails its gate → the previous generation keeps serving.
  - Edge: the dataset is stale (> 25 h) → the job refuses to train (existing init-container guard).
- **Finetune auto-train.** Given the finetune schedule, when the job runs, then the pipeline (ingest →
  validate → transform → train → evaluate) produces a candidate that is always saved, registered as a
  version of `finetune-classifier`, and becomes champion only via the gate (`macro_f1` floor + improvement).

## Non-goals
- **No LLM or chatbot wiring this initiative.** The finetune champion has no consumer yet; nothing in team-ai,
  team-gateway or platform-modelserve changes.
- No serving of finetune models. The GLiNER-shaped serving files from MLOps (`serve/local.py`, `package.py`,
  `optimize.py`, `serve_analyzer.py`, `serve/triton.py`, `serve/deploy_sagemaker.py`) are imported but marked
  "not ported" and get no compose service or manifest.
- No orchestrator (Airflow, Kubeflow, Ray), no feature-store change, no HPO for ALS.
- No registry service with its own API: the registry is MLflow; the gate is a library run inside each trainer.
- The recsys serving pointer swap in Redis (`recs:v1:serving` / `recs:v1:previous`) stays as recsys's publish
  step; only the champion decision moves to the registry.
- platform-forecast stays unwired. The GPU perf toolkit (`perf/`) is imported as-is with no deploy wiring.

## Architecture decisions
- **ADR-0017 "Unified model registry and publish gate"** (new, platform-core/docs/ADR). It **supersedes ADR-0014**
  (Redis registry) and the MLflow rejection in ADR-0013 (Alternatives). Why: the ADR-0013 revisit trigger is met
  (four models trained concurrently; rollback needed), and all AI models must live in one place behind one gate.
- **Registry = MLflow Model Registry** (v3.2.0 server on :5050). Backend store: its own `postgres-mlflow`
  (DB-per-service). Artifacts: the existing MinIO (`s3://mlflow-artifacts`). Champion = alias `champion`;
  previous holder = alias `previous`. Gate outcome, reason and lineage are version tags.
- **Gate = shared Python library `platform-core/packages/py-modelregistry`** (precedent: `packages/go-sdk`,
  ADR-0016). API: `submit(model, run_id, metrics, artifact, lineage) → Decision`, `promote(force, reason)`,
  `rollback`. Policies in one file `platform-core/packages/py-modelregistry/policies.yaml`. The ADR-0014 rule
  (primary-metric relative improvement + guard ratio) is the default policy shape, so recsys behaviour is kept.
- Per-trainer registries are **removed**: recsys `recsys/registry/` (Redis) and finetune
  `registry/{base,local,s3,stages}.py`. A one-shot migration seeds each recsys model's current Redis champion
  as version 1 `@champion`, so the first gated run compares against it instead of auto-promoting.
- **New repo `platform-finetune`** (a platform capability, not `team-*`: no business tables, no proto), imported
  with `git subtree add` from `/Users/phuc.buidang/Documents/work/MLOps` (branch `main`) to keep history. Its own
  `ops/docker-compose.yml` (MinIO, MySQL, MLflow) is dropped in favour of agora infra.
- **Scheduling stays gitops CronJobs** (no orchestrator): featurestore 02:15 → recsys 03:00 nightly (existing),
  finetune weekly on the GPU node pool (ADR-0011 selector/toleration).
- The registry is on the publish critical path: unreachable → no publish, non-zero exit; serving keeps the last
  champion. Registry calls time out within 10 s.
- **LLM principle recorded for later initiatives:** every LLM call goes through team-ai, and the `mock` provider
  always exists and is the default in compose and e2e (`CHAT_BACKEND=mock`, `llm-fake`). A self-trained model
  will be one more provider behind team-ai, never a replacement for mock.
- New dependency: `mlflow-skinny` client (via py-modelregistry) in platform-recsys and platform-finetune.

## Contracts
No proto, RPC or Kafka event changes.
- MLflow tracking + registry API at `http://mlflow:5050` (env `MLFLOW_TRACKING_URI`). Registered models
  (= experiments): `recsys-als`, `recsys-gbdt`, `recsys-two-tower`, `finetune-classifier`. Aliases `champion`,
  `previous`. Version tags `gate.outcome`, `gate.reason`, `gate.forced`, `lineage.data_version`, `lineage.train_hw`.
- `py-modelregistry` Python API (above) and `policies.yaml` schema:
  `<model>: {primary: <metric>, min_relative_improvement: float, guards: {<metric>: min_ratio}, floors: {<metric>: min}}`.
- MinIO bucket `mlflow-artifacts` (created by `minio-init`).

Existing contracts consumed unchanged: featurestore datasets `als_interactions@v1`, `rank_training@v1`; recsys
Redis serving keys (`recs:v1:*`) and Qdrant collection read by team-ai.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| One model registry + shared publish gate (ADR-0017, MLflow server, py-modelregistry, policies.yaml) | add-model-registry | platform-core | – | provides: MLflow :5050, py-modelregistry API, policies.yaml | platform-core (ADR, infra compose, packages/py-modelregistry), platform-gitops, .github/workflows/services-ci.yml |
| Recsys auto-train end to end on the shared registry + gate; GBDT/two-tower on; two-tower gated | recsys-auto-train | platform-recsys | add-model-registry | uses: py-modelregistry | platform-recsys, platform-gitops, docker-compose.services.yaml, platform-e2e |
| MLOps fine-tune pipeline in agora, auto-trained weekly, registered + gated | add-platform-finetune | platform-finetune | add-model-registry | uses: py-modelregistry | platform-finetune (new), docker-compose.services.yaml, platform-gitops, .github/workflows/services-ci.yml |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `docker-compose.services.yaml` — recsys-auto-train, add-platform-finetune
- `.github/workflows/services-ci.yml` — add-model-registry, add-platform-finetune
- `platform-core/packages/py-modelregistry/policies.yaml` — created by add-model-registry; recsys-auto-train and
  add-platform-finetune each add their entries

Build order: add-model-registry → recsys-auto-train (priority) → add-platform-finetune.

## Out of initiative
- Docs: AGENTS.md §2 row for platform-finetune, §5 ports (MLflow :5050, postgres-mlflow), a short "Training"
  subsection; fix stale `platform-recsys/README.md` §10 and `platform-modelserve/README.md`; point
  `plan-mlops/INDEX.md` at this initiative; add the finetune feedback loop/check to `docs/agents/project.md`.
- Next initiative (not here): modelserve loads registry champions + streaming; team-ai `registry:` provider
  behind the mock-default ModelRouter; chatbot on a self-trained model.

## NFR and data (initiative level; each slice's proposal.md refines them)
- NFR: registry calls time out within 10 s; an outage fails the run fast (non-zero) and leaves serving on the last
  champion. The gate decision is deterministic for the same metrics and policy. Recsys nightly end-to-end stays
  within the current CronJob window.
- Personal data: recsys trains on pseudonymous user ids. MLflow runs and artifacts **must not** contain user ids,
  interaction rows or per-user lists — only aggregate params/metrics and model artifacts (ALS item factors are
  allowed; user factors are not uploaded). Finetune data (AG News) is public. No new personal data stored.
- Risk routing expected: pii (recsys → MLflow boundary). None for auth, payment or proto. MLflow has no auth
  locally; in gitops it is cluster-internal only (no ingress).

## Conventions
- New ADR follows `platform-core/docs/ADR/0011-model-serving.md` structure.
- Gitops manifests copy the shape of `platform-gitops/platform/{modelserve,recsys}/`; secrets via Vault refs.
- Tests: platform-recsys `make -C platform-recsys compile lint check-env test-host eval` (py3.10);
  py-modelregistry offline pytest against a local MLflow file store; platform-finetune offline pytest
  (no GPU, no dataset; `tmp_artifacts` fixture). E2e via platform-e2e on the compose stack (jobs profile).
- Keep the ADR-0014 gate numbers for recsys (`ndcg@10` ≥ +1 %, `coverage@10` ≥ 0.8× champion) as its policy.
- Don't touch platform-featurestore, team-ai, team-gateway, platform-modelserve or proto.

## Locked
2026-10-13 — Confirmed by Bùi Đăng phúc. One MLflow Model Registry + shared py-modelregistry gate (supersedes ADR-0014); recsys auto-train first, then MLOps import as platform-finetune (weekly, gated, no consumer); LLM calls only via team-ai with mock always default; chatbot/modelserve wiring deferred.
