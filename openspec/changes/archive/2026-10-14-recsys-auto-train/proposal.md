## Why

platform-recsys decides "champion" in a Redis-backed `recsys/registry/` (ADR-0014), with the in-memory fallback when Redis is down. Initiative `training-platform` puts every model behind one MLflow Model Registry and one shared publish gate (`py-modelregistry`, ADR-0017, change `add-model-registry`). Recsys is the first consumer and today falls short in four ways:

- The champion pointer lives in Redis next to the serving keys. It is not visible with the other models, and a registry outage silently turns into "every run is a bootstrap" (`ModelRegistry` falls back to memory, so the gate is bypassed).
- GBDT and two-tower are off in the nightly CronJob (`ENABLE_GBDT`, `ENABLE_TWO_TOWER` default `false`; `platform-gitops/platform/recsys/cronjob.yaml` sets neither).
- GBDT is gated against its own Redis pointer (`recs:model:champion:gbdt`), but two-tower has no evaluation at all: it only needs non-degenerate vectors to publish.
- The "stale dataset refused" rule exists only as a k8s init container, so it cannot be tested on the compose stack.

initiative: training-platform

## What Changes

- **Registry swap.** `recsys/registry/` (`ModelRegistry`, `ModelMetadata`, `PromotionDecision`) is deleted. `pipeline.py`, `publish.py` (rollback), `ranker/stage.py`, `ranker/contract.py` and `__main__.py` call `py-modelregistry` through one adapter, `recsys/registry_client.py`. Three registered models: `recsys-als`, `recsys-gbdt`, `recsys-two-tower`. Library API and semantics are the two-phase ones fixed by the initiative (round-2 decisions D1 to D10): `evaluate` registers a version and moves no alias, `apply` moves `champion` (old holder becomes `previous`), `mark_rejected`, `register_rejected`, `seed`.
- **Run order (two-phase).** (1) pre-flight registry reachability, then dataset checks, then train ALS and `evaluate`; (2) only when ALS is `promotable`, train GBDT and two-tower and `evaluate` each; (3) publish the generation made of the `promotable` models; (4) `apply` each published model. A stage or publish failure calls `mark_rejected` on the evaluated versions and applies nothing, so `previous` is never lost. A crash between evaluate and apply leaves aliases unchanged and the version `promotable`; later runs ignore stale `promotable` versions. Structural (degenerate) rejections use `register_rejected`.
- **Gate policies.** This slice adds three entries to `platform-core/packages/py-modelregistry/policies.yaml` (created by `add-model-registry`; hot file, build is serialised), thresholds inclusive:
  - `recsys-als`: `primary: ndcg@10`, `min_relative_improvement: 0.01`, `guards: {coverage@10: 0.8}` (ADR-0014 numbers).
  - `recsys-gbdt`: `primary: ndcg@10`, `min_relative_improvement: 0.005`, `guards: {recall@10: 0.95}`.
  - `recsys-two-tower`: `primary: recall@10`, `min_relative_improvement: 0.005`, `guards: {coverage@10: 0.8}`, `floors: {recall@10: 0.000001}` (the first champion needs `recall@10` > 0; a coverage floor is unreachable on a real catalogue, see design.md Clarifications).
  - The +0.5 % step stops an equal model from churning `champion` nightly. A missing or non-finite policy metric on the candidate rejects it; a guard metric missing on the champion is skipped with a note (D3). The evaluation protocol is the version tag `lineage.eval_protocol`; a changed or absent champion protocol means the candidate is judged on floors only (D2).
- **Two-tower gets an evaluation.** The stage holds out a seeded fraction (`TWO_TOWER_HOLDOUT_FRACTION`, default 0.1, valid range (0, 1)) of its (user, item) pairs, trains on the rest, and scores `recall@10` and `coverage@10` of item-tower retrieval on the held-out pairs in process. Fewer than 10 pairs means no candidate. Only aggregate metrics go to the gate.
- **Generation rule.** ALS decides first. ALS rejected: GBDT and two-tower are not trained and nothing is published. ALS `promotable`: a rejected GBDT or two-tower leaves that component out of the generation; a stage exception exits 1 with nothing published.
- **Fail-closed and exit codes (D4, D5).** Pre-flight reachability check at job start, before Spark (exit 1 when MLflow does not answer). Each library operation has a 12 s deadline across its registry calls. Exit 0 = completed (rejection included), 1 = registry unavailable or unexpected stage exception, 2 = configuration error, stale dataset or refused rollback. No in-memory fallback.
- **One-shot seed (D7).** `python -m recsys migrate-registry` reads `recs:model:champion`, `recs:model:champion:gbdt` and `recs:model:meta:<version>` and calls `seed` per model: v1, `@champion`, `gate.outcome=promoted`, `gate.reason=seed: ...`, metrics as stored (a metrics-only v1 without artifact is allowed), `eval_protocol` copied into `lineage.eval_protocol`. A model that already has versions is skipped. A Redis champion lacking a guard metric still seeds, and the guard is then skipped with a note. Two-tower has no Redis champion and starts at the floors.
- **Rollback (D6).** `python -m recsys rollback` swaps each carried model's `champion` and `previous` through the library first, then swaps the serving pointer. All preconditions are checked before the first change (previous generation present, every model has a `previous` with an artifact); any refusal exits 2 and changes nothing. A registry failure mid-way swaps back what was swapped, leaves the pointer alone and exits 1. A forced promote or rollback made directly in the registry is not mirrored into serving.
- **Freshness guard in the job.** `MAX_DATASET_AGE_MINUTES` (integer >= 0; 0 disables; default 0 in code, 1500 in gitops): when positive and the newest `als_interactions` snapshot is older, exit 2 before Spark starts and register nothing. The init container stays.
- **Drift baseline (D9).** Quantile sketches become the JSON artifact `drift_sketches.json` of the run (too long for an MLflow param); the next run reads it from the run behind `recsys-als@champion`.
- **Wiring.**
  - `platform-gitops/platform/recsys/cronjob.yaml`: `ENABLE_GBDT=true`, `ENABLE_TWO_TOWER=true`, `MLFLOW_TRACKING_URI=http://mlflow:5050`, egress to `app: mlflow` TCP 5050, `activeDeadlineSeconds: 3600`.
  - `docker-compose.services.yaml` `platform-recsys` service: `MLFLOW_TRACKING_URI`, `ENABLE_GBDT`, `ENABLE_TWO_TOWER`, `depends_on: mlflow`.
  - `platform-recsys`: `py-modelregistry` (brings `mlflow-skinny`), settings `MLFLOW_TRACKING_URI`, `MAX_DATASET_AGE_MINUTES`, `TWO_TOWER_HOLDOUT_FRACTION`, `.env.example`.
- **Lineage.** Tags `lineage.data_version` (dataset `name@version as_of sha256`), `lineage.eval_protocol`, `lineage.train_hw=cpu`; the run id is the version's run.
- No proto, RPC, Kafka or team-ai change. The Redis serving keys (`recs:v1:*`) and Qdrant collections read by team-ai are unchanged.
- **BREAKING (internal):** `recs:model:champion*` and `recs:model:meta:*` are no longer read or written after migration. `PROMOTION_PRIMARY_METRIC`, `PROMOTION_MIN_RELATIVE_IMPROVEMENT`, `PROMOTION_MIN_COVERAGE_RATIO` and `PROMOTION_FORCE` are removed.
- **Existing specs changed (D12, MODIFIED deltas in this change).** Migration notes are inside each modified requirement; summary:

| Capability | Requirement | Change | Migration note |
|---|---|---|---|
| recsys-generations | Degenerate candidates are rejected before the metric gate | rejection recorded via `register_rejected` | Redis meta ignored |
| recsys-generations | The previous generation can be restored | registry swap then pointer; exit 2 | `restore_champion` replaced |
| recommendations | The training run SHALL evaluate the generation it produced | protocol rule via `lineage.eval_protocol` and floors (D2) | rule kept, bounded by floors |
| recommendations | A candidate SHALL reach serving only after passing the promotion gate | `evaluate` then `apply` after publish | `ModelMetadata` and Redis champion gone |
| recommendations | Batch pipeline produces two-tower vectors | published only when two-tower is `promotable` | two-tower now gated |
| recommendations | The local stack serves trained recommendations | compose job needs MLflow | e2e re-pointed |
| gbdt-ranking-training | The GBDT candidate is gated and published with its generation | `recsys-gbdt` policy, no `PROMOTION_*` | Redis GBDT champion seeded |
| observability | Each run is compared with the generation it replaces | baseline from champion run's `drift_sketches.json` | first run after seed is `no_baseline` |
| recs-on-cluster | Recommendation pods reach only the stores they need | recsys egress gains mlflow | none |

No whole requirement is removed, so no `## REMOVED` delta is needed: the obsolete scenarios (bootstrap via the Redis registry, `recs:model:champion` pointer) are rewritten inside the MODIFIED requirements. The e2e features `pipeline_eval_registry.feature`, `generation_publish.feature` and `trained_recs_local.feature` are re-pointed to MLflow aliases and tags (tasks 2.x).

## Non-goals

- No change to team-ai, platform-featurestore, platform-modelserve, team-gateway or proto.
- No change to the generation pointer protocol (`recs:v1:serving` / `recs:v1:previous`, Lua swap, retention).
- No HPO, no orchestrator, no new CronJob schedule (featurestore 02:15, recsys 03:00 stay).
- The registry server and `py-modelregistry` themselves belong to `add-model-registry`; this change only consumes them.

## NFR

- Registry: each operation has a 12 s total deadline; the pre-flight at job start fails an unreachable registry with exit 1 within 20 s and before Spark starts; a mid-run outage stops the job at that call (exit 1, within 12 s). Whole job bounded to 60 minutes (`activeDeadlineSeconds: 3600` in the CronJob; proposed value).
- Gate decision is deterministic for the same metrics and policy; the nightly run stays inside the current CronJob window (recsys 03:00, `backoffLimit: 1`, `concurrencyPolicy: Forbid`). Added cost per night: two-tower holdout training of one extra pass on at most `TWO_TOWER_MAX_PAIRS` pairs, plus at most 3 `evaluate` and 3 `apply` calls (each within 12 s).
- Availability: serving (team-ai) never depends on MLflow; only the nightly job does. A failed or skipped run leaves serving on the last generation and refreshes its TTL (48 h > nightly cadence).
- Limits: MLflow artifacts per version are item-side only (item factors, GBDT artifact JSON, two-tower item vectors). The size of the biggest artifact is not capped by this change.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `user_key` (pseudonymous user id) in training pairs, ALS user factors, per-user Top-N lists | Personal (pseudonymous) | Train and serve recommendations | Existing recsys consent (`recsys_consent_context`, behavioural analytics consent) | Redis generation keys 48 h TTL; Qdrant user collection per generation (2 kept). Unchanged. **Never sent to MLflow.** |
| Interaction rows (`als_interactions`, `rank_training`) | Personal (pseudonymous) | Training input | Same | Read-only from the featurestore volume. **Never sent to MLflow.** |
| Aggregate metrics (ndcg@10, coverage@10, recall@10, drift_psi_max, counts) | Not personal | Gate, audit | n/a | MLflow run history, kept for the life of the version (no pruning) |
| Params: hyperparameters, dataset lineage (name, version, as_of, sha256), feature list, counts | Not personal (aggregates) | Lineage | n/a | MLflow, same as above (the sketches are a JSON artifact `drift_sketches.json`) |
| Drift sketches (`drift_sketches.json`: quantiles of weight, items per user, users per item, top score; 101 points each) | Not personal (aggregates) | Drift baseline | n/a | Life of the version |
| Item-side artifacts: ALS item factors, two-tower item vectors, GBDT ranker JSON | Not personal (listing-level) | Reproduce/rollback a version | n/a | MLflow artifact store (MinIO `mlflow-artifacts`, life of the version) |
| Refused listing ids (two-tower `refused`) | Not personal | Diagnostics | n/a | Count only is logged to MLflow, ids stay in the job log |

Rule enforced in code: a single allow-list in the recsys MLflow adapter (`recsys/registry_client.py`) is the only path to `py-modelregistry`; it accepts numeric metrics, a fixed set of param keys, and item-side artifacts, and raises on anything keyed by user id. Covered by a test that fails if a user id appears in any logged param, metric, tag or artifact.

## Threat model

- **Assets:** serving generation (what buyers see), the `champion` alias per model, pseudonymous interaction data, MLflow artifacts.
- **Abuse and failure cases:**
  1. Registry outage turns the gate off and a bad candidate ships (old behaviour: in-memory fallback). Mitigation: fail-closed, exit 1, publish nothing.
  2. Personal data leaks to MLflow (no auth locally, cluster-internal in gitops). Mitigation: the allow-list above (also covers system tags and run name), the item-factors-only ALS artifact (never a Spark `ALSModel.save` directory, which holds `userFactors`), its leak test, and an e2e scan of params, metrics, tags, run name and artifact contents with a planted-key negative control.
  3. A crash between champion move and publish leaves the registry naming an unserved model (a known gap in the Redis registry, README §251). Mitigation: two-phase gate. `evaluate` moves no alias; `apply` runs only after a successful publish; on failure `mark_rejected` with reason `publish failed`. A crash between evaluate and apply leaves the alias unchanged. Residual gap: if the registry is down at `apply` time after the pointer swap, serving is ahead of the registry (exit 1, logged; the next run compares against the old champion).
  4. Stale data trains and ships. Mitigation: freshness guard in the job and the init container.
  5. Manual override abuse: `promote --force` is an engineer action with `--reason`, recorded on the version (`gate.forced`); the nightly job never forces and the removed `PROMOTION_FORCE` is ignored.
- **Authorisation:** the job has no inbound surface. Write access to MLflow is network-level only (cluster-internal service, NetworkPolicy egress from the recsys pod; no ingress to MLflow). Only the recsys pod and engineers with cluster access can move aliases. No new user-facing permission, so no 401/403 scenario applies; the permission-like rule is "the job must not force-promote".
