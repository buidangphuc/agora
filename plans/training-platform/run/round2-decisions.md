# Round 2 — cross-slice decisions (orchestrator, from spec-quality review round 1)

All three changes MUST follow these. Items marked (human-confirm) are listed in LOCK.md for sign-off.

- **D1 Two-phase gate API (py-modelregistry).**
  - `evaluate(model, run_id, metrics, artifact_path, lineage) -> Decision{version, outcome: promotable|rejected, reason}`
    registers the version, tags `gate.outcome` (`promotable` or `rejected`) and `gate.reason`, and moves NO alias.
  - `apply(model, version)` moves `champion` (old holder → `previous`) and sets `gate.outcome=promoted`.
    It is refused (GateError, exit 2) unless the version is `promotable`.
  - `mark_rejected(model, version, reason)` sets `gate.outcome=rejected` plus the reason and touches no alias.
    Use it after a downstream stage or publish failure.
  - `register_rejected(model, run_id, reason, lineage)` registers a version as rejected without evaluating
    the policy. Use it for structural rejections.
  - `submit(...)` = `evaluate`, then `apply` if promotable (convenience for finetune).
  - A crash between evaluate and apply leaves the version `promotable` and the aliases unchanged.
    Later runs ignore stale `promotable` versions.
  - recsys: evaluate all → publish → apply each published model → on publish failure, `mark_rejected`.
    `previous` is never lost.
- **D2 eval_protocol.** Recorded in the lineage tag `lineage.eval_protocol`. If the candidate's and the
  champion's values differ, or the champion has no such tag, metrics are not compared: the candidate is
  judged on `floors` only, with reason `eval_protocol changed: <old> -> <new>`. This keeps the existing
  recommendations "different protocol" behaviour, now bounded by floors. MODIFY that requirement; do not drop it.
- **D3 Missing metrics.**
  - A missing or non-finite `primary`, guard or floor metric on the candidate → `rejected` (version registered,
    reason names the metric). Nothing raises.
  - A guard metric missing on the champion → that guard is skipped, and the skip is noted in the reason.
  - A champion `primary` ≤ 0 → relative improvement is not computed; the candidate passes if
    candidate > champion and meets the floors.
- **D4 Exit codes (library CLI and both trainers):**
  - 0 = completed, including a `rejected` decision.
  - 1 = RegistryUnavailable, or an unexpected stage exception.
  - 2 = usage/config error, PolicyError, GateError, RollbackError, unknown version, stale dataset.
- **D5 Deadline.**
  - Each library operation (`evaluate`, `apply`, `submit`, `promote`, `rollback`, `seed`) has a total
    deadline of 12 s across all its registry calls.
  - Both trainers run a registry reachability pre-flight at job start, before training (exit 1 if unreachable).
- **D6 Rollback = swap.** `champion` ↔ `previous`, so a second rollback toggles back. Rollback with no
  `previous` → RollbackError (exit 2). A forced promote or a rollback in the registry is NOT mirrored into
  recsys serving; recsys serving changes only through the recsys job or `python -m recsys rollback`, which
  calls the library.
- **D7 Seed.** `seed(model, metrics, lineage, reason, artifact_path=None)` bypasses the gate. It is allowed
  only when the model has no versions, and tags `gate.outcome=promoted` and `gate.reason=seed: <reason>`.
  A metrics-only v1 (no artifact) is allowed. recsys rollback to a version with no artifact is refused (exit 2).
  The recsys migration uses `seed` and copies `eval_protocol` from the Redis meta into the lineage.
- **D8 Policy file resolution.** The `policies_path` argument, then env `MODELREGISTRY_POLICIES`, then the
  packaged `policies.yaml`.
- **D9 Artifacts and params.** `artifact_path` is logged to `run_id` under `model/`, and the version's source
  is that run, so the run id is the version's run (no tag needed). Callers log params and long values
  (sketches) to the run themselves; anything over the MLflow param limit goes in as a JSON artifact.
- **D10 Policy numbers (human-confirm).**
  - `finetune-classifier`: `{primary: macro_f1, min_relative_improvement: 0.01, floors: {macro_f1: 0.5}}`.
    Remove `gate.enabled` and `gate.on_fail` from params.yaml.
  - `recsys-als`: as in ADR-0014 (+1 % ndcg@10, guard coverage@10 0.8).
  - `recsys-gbdt`: `{primary: ndcg@10, min_relative_improvement: 0.005, guards: {recall@10: 0.95}}`.
  - `recsys-two-tower`: `{primary: recall@10, min_relative_improvement: 0.005, guards: {coverage@10: 0.8},
    floors: {coverage@10: 0.5}}`.
  - Both thresholds are inclusive (≥). The +0.5 % improvement threshold stops a merely equal model from
    churning `champion` every night.
- **D11 Finetune specifics.**
  - The trainer creates the MLflow run and passes `run_id` to evaluate/submit.
  - `train_hw` is the `_hw_id` string, or `cpu` on a CPU-only host.
  - CronJob: `0 4 * * 0` (Sunday 04:00), `backoffLimit: 1`, `concurrencyPolicy: Forbid`.
  - Egress: DNS, mlflow, minio, and TCP 443 to `0.0.0.0/0` except cluster CIDRs (for the HF dataset and the
    base model). This is asserted statically in the manifest test.
  - CI job `timeout-minutes: 10`.
  - The offline suite must not import the six NOT-PORTED files.
- **D12 recsys spec deltas.** Add `## MODIFIED` and `## REMOVED` requirements for every contradicted existing
  requirement: recsys-generations, recommendations, gbdt-ranking-training, observability (drift baseline now
  from the champion run in MLflow), and recs-on-cluster (egress gains mlflow). Each comes with a migration
  note. Re-point the e2e features `pipeline_eval_registry.feature`, `generation_publish.feature` and
  `trained_recs_local.feature`.
- **D13 E2E determinism.**
  - Scenarios seed state explicitly (registry seed via `seed`, prior generation g1, snapshots present) and are
    `@destructive` with a reset step.
  - Pass and fail are driven by seeding a weak or strong champion.
  - The fixture is sized so every model passes in the "all three promoted" scenario.
  - The PII scan covers params, metrics, system tags, run name and artifact contents, and includes a planted
    key as a negative control.
