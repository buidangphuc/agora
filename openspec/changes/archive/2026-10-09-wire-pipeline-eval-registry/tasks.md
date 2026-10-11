# Tasks

## 1. Code — platform-recsys (evaluation stage)
- [x] Build the temporal holdout from the same `tracking_events` window the triples come from,
      reusing `recsys/evals/split.py`; do not re-read the warehouse.
- [x] Score the trained factors with `ModelEvaluator.evaluate`, producing `ndcg@k`, `recall@k`,
      `precision@k`, `map@k`, `mrr@k` and `coverage@k`. Add no new metric.
- [x] Return `None` (not an empty dict) when the split yields no test events, so "unevaluated"
      is distinguishable from "evaluated at zero".

## 2. Code — platform-recsys (registry + gate)
- [x] Populate `ModelMetadata` with the metrics, `model_version`, `model_type: "als"`,
      `git_commit`, and the Qdrant collections / Redis prefix the run targets.
- [x] Call `ModelRegistry.evaluate_and_promote` with the candidate before publishing anything.
- [x] Reorder `pipeline.py:run()` so Qdrant upsert, `_prune_stale` and
      `redis_cache.load_cache` happen only on a promoted candidate (see `design.md` §2).
      Keep `recs:v1:model_version` as the last write.
- [x] Pass a real Redis client to `ModelRegistry`; the in-memory fallback must not be what runs
      in the CronJob.

## 3. Code — platform-recsys (configuration)
- [x] Add to `_FIELDS`: holdout window / `split_k`, `PROMOTION_PRIMARY_METRIC`,
      `PROMOTION_MIN_RELATIVE_IMPROVEMENT`, `PROMOTION_MIN_COVERAGE_RATIO`.
- [x] Ship a non-zero `PROMOTION_MIN_RELATIVE_IMPROVEMENT` default — `0.0` promotes noise.
- [x] Update `.env.example` (the `make check-env` drift gate enforces both directions).
- [x] Surface the same keys in `platform-gitops/platform/recsys/cronjob.yaml` so the deployed
      thresholds are visible in the manifest, not implicit in code defaults.

## 4. Code — platform-recsys (observability)
- [x] Extend the pipeline summary with `metrics`, `champion_before`, `candidate_version`,
      `decision` and `reason`.
- [x] Log the decision at INFO; a rejection is a normal outcome and must not exit non-zero.

## 5. Verification — execution proof, not module test
- [x] A test drives `pipeline.run()` end to end on a sample warehouse and asserts the summary
      carries a non-empty `metrics` dict — the pipeline, not the evaluator, is under test.
- [x] A test asserts a rejected candidate leaves Qdrant points, Redis keys and
      `recs:v1:model_version` untouched, and that no prune ran.
- [x] A test asserts the first run with an empty registry is promoted.
- [x] `make -C platform-recsys compile lint test-host check-env`.
- [x] `openspec validate wire-pipeline-eval-registry --strict`.

## 6. E2E (platform-e2e)
- [x] **Known gate tension — resolved before archiving.** Option (a) + (b) implemented: added automated scenario tests in `platform-e2e/tests/e2e/features/recommendations/pipeline_eval_registry.feature` and registered automated features in `platform-recsys/FEATURES.yaml`.
- [x] Note for the record: Unblocked `make -C platform-e2e spec-check CHANGE=wire-pipeline-eval-registry`.

## 7. Archive
- [x] `make -C platform-e2e features-check` green.
- [x] `make -C platform-e2e spec-check CHANGE=wire-pipeline-eval-registry` green.
- [x] Update the `plan-mlops/INDEX.md` status row.
- [x] `openspec archive wire-pipeline-eval-registry`.
