## ADDED Requirements

Terms. The job is `python -m recsys`. The model registry is MLflow through `py-modelregistry` (decisions D1 to D10 of the initiative). Its two-phase API is `evaluate(model, run_id, metrics, artifact_path, lineage)` (registers a version, tags `gate.outcome` as `promotable` or `rejected` plus `gate.reason`, moves no alias), `apply(model, version)` (moves `champion`, old holder becomes `previous`, tags `promoted`, refused unless `promotable`), `mark_rejected(model, version, reason)`, `register_rejected(model, run_id, reason, lineage)` and `seed(model, metrics, lineage, reason, artifact_path=None)`. "Serving" means `recs:v1:serving` and `recs:v1:previous` in Redis and what team-ai `Recommend` answers from them. Policies come from `platform-core/packages/py-modelregistry/policies.yaml`; both thresholds are inclusive. Exit codes: 0 = completed (a rejected decision completes), 1 = registry unavailable or an unexpected stage exception, 2 = configuration or usage error, refused rollback, stale dataset.

### Requirement: The ALS candidate is evaluated by the shared gate with the ADR-0014 numbers

The job SHALL register every evaluated ALS candidate as a new version of `recsys-als` in MLflow through `evaluate`, with params, metrics, lineage and the tag `lineage.eval_protocol`. The `recsys-als` policy SHALL require `ndcg@10` at least 1 % above the champion's and `coverage@10` at least 0.8 times the champion's. `evaluate` SHALL NOT move an alias. A model that has no `champion` SHALL be judged on its policy floors only (`recsys-als` has none, so its first candidate is `promotable`).

#### Scenario: RAT-01 A better ALS candidate is promotable and becomes champion after its publish
- **WHEN** the nightly job runs with champion metrics `ndcg@10=0.100, coverage@10=0.50` and the candidate scores `ndcg@10=0.102, coverage@10=0.45`
- **THEN** MLflow shows a new `recsys-als` version tagged `gate.outcome=promoted` after the publish, the alias `champion` names it and the old holder is under `previous`
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_als_promotable_then_applied_after_publish`

#### Scenario: RAT-02 A candidate below the improvement threshold is rejected
- **WHEN** the candidate scores `ndcg@10=0.1005` against a champion at `0.100`
- **THEN** the new version is tagged `gate.outcome=rejected` with a `gate.reason` naming `ndcg@10`, and the alias `champion` still names the old version
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_als_below_threshold_rejected`

#### Scenario: RAT-03 The coverage guard rejects a candidate that wins on ndcg
- **WHEN** the candidate scores `ndcg@10=0.150` and `coverage@10=0.39` against a champion with `coverage@10=0.50`
- **THEN** the version is tagged `gate.outcome=rejected` with a `gate.reason` naming `coverage@10`
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_als_coverage_guard`

#### Scenario: RAT-04 Boundary, exactly +1 % and exactly 0.8x passes
- **WHEN** the candidate scores `ndcg@10=0.101` and `coverage@10=0.40` against a champion at `0.100` and `0.50`
- **THEN** the version is tagged `gate.outcome=promotable` by `evaluate`
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_als_boundary_values_pass`

#### Scenario: RAT-05 A degenerate candidate is registered as rejected without a metric gate call
- **WHEN** the structural check finds an empty Top-N list for a user
- **THEN** the version is registered through `register_rejected` with `gate.outcome=rejected` and `gate.reason` carrying the structural reason, `evaluate` is not called, no alias moves, nothing is published and the job exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_structural_reject_uses_register_rejected`

#### Scenario: RAT-06 A run with no usable holdout is not registered
- **WHEN** the temporal split leaves no test events
- **THEN** the job registers no version, moves no alias, publishes nothing and exits 0 with decision `skipped`
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_no_holdout_not_a_candidate`

#### Scenario: RAT-37 The first ALS candidate with no champion is promotable
- **WHEN** `recsys-als` has no `champion` and the candidate has finite `ndcg@10` and `coverage@10`
- **THEN** the version is tagged `gate.outcome=promotable` with a `gate.reason` saying there is no champion
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_first_candidate_without_champion`

#### Scenario: RAT-38 A missing or non-finite policy metric rejects the candidate
- **WHEN** the candidate's `coverage@10` is NaN
- **THEN** the version is registered with `gate.outcome=rejected` and a `gate.reason` naming `coverage@10`, no exception is raised and the job exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_nan_guard_metric_rejected`

#### Scenario: RAT-39 A changed evaluation protocol is judged on the floors only
- **WHEN** the champion's `lineage.eval_protocol` is `leave-last-new-item-v0` and the candidate's is `leave-last-new-item-v1`
- **THEN** the metrics are not compared, the version is judged on the policy floors only and its `gate.reason` is `eval_protocol changed: leave-last-new-item-v0 -> leave-last-new-item-v1`
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_protocol_change_floors_only`

### Requirement: GBDT and two-tower are trained nightly and evaluated under their own policies

With `ENABLE_GBDT=true` and `ENABLE_TWO_TOWER=true`, the job SHALL train both after the ALS candidate is `promotable`, register them as versions of `recsys-gbdt` and `recsys-two-tower`, and call `evaluate` on each under its own policy. The `recsys-gbdt` policy SHALL use primary `ndcg@10` with a minimum relative improvement of 0.5 % and guard `recall@10` ratio 0.95. The `recsys-two-tower` policy SHALL use primary `recall@10` with a minimum relative improvement of 0.5 %, guard `coverage@10` ratio 0.8 and floor `coverage@10` 0.5. Each model SHALL be compared with its own champion only. A guard metric missing on the champion SHALL be skipped and the skip noted in the reason.

#### Scenario: RAT-07 Three models become promotable and ride one generation
- **WHEN** the ALS, GBDT and two-tower candidates are all evaluated `promotable` (the e2e fixture is sized so that they are, against seeded weak champions) and the publish succeeds
- **THEN** each of `recsys-als`, `recsys-gbdt` and `recsys-two-tower` has a new version tagged `gate.outcome=promoted`, and the generation in Redis and Qdrant holds the ranker key and the two-tower collection
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_all_three_promoted_in_one_generation`

#### Scenario: RAT-08 A rejected GBDT leaves the ranker out of the generation
- **WHEN** ALS is `promotable` and the GBDT candidate's `ndcg@10` is below its champion's by the policy rule
- **THEN** `recsys-gbdt` has a version tagged `gate.outcome=rejected`, its `champion` alias is unchanged, the new generation has no `recs:v1:gen:<generation>:ranker` key, and the job exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_gbdt_rejected_generation_ships_without_ranker`

#### Scenario: RAT-09 A rejected two-tower leaves its vectors out of the generation
- **WHEN** ALS is `promotable` and the two-tower candidate's `recall@10` is below the champion's by the policy rule
- **THEN** `recsys-two-tower` has a version tagged `gate.outcome=rejected`, its `champion` alias is unchanged, the new generation has no two-tower Qdrant collection, and the job exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_two_tower_rejected_generation_ships_without_vectors`

#### Scenario: RAT-10 A rejected ALS candidate stops GBDT and two-tower
- **WHEN** the ALS candidate is rejected
- **THEN** no `recsys-gbdt` or `recsys-two-tower` version is created, and nothing is published
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_als_rejected_skips_other_stages`

#### Scenario: RAT-11 A stage that raises rejects the evaluated candidates and publishes nothing
- **WHEN** the GBDT or two-tower stage raises during training after ALS was evaluated `promotable`
- **THEN** `mark_rejected` is called on the ALS version with a reason starting `gbdt stage failed` or `two-tower stage failed`, every `champion` alias is unchanged, serving is unchanged and the job exits 1
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_stage_failure_marks_rejected_and_exits_1`

#### Scenario: RAT-12 The first two-tower candidate must meet the policy floors
- **WHEN** `recsys-two-tower` has no `champion` and the candidate scores `coverage@10=0.49`
- **THEN** the version is tagged `gate.outcome=rejected` with a `gate.reason` naming the `coverage@10` floor of 0.5
- **VERIFIED BY**: `platform-recsys/tests/test_gate_two_tower.py::test_first_candidate_below_floor_rejected`

#### Scenario: RAT-40 An equal GBDT model does not churn the champion
- **WHEN** the GBDT candidate's `ndcg@10` equals its champion's
- **THEN** the version is tagged `gate.outcome=rejected` with a `gate.reason` naming `ndcg@10`, and `champion` is unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_gate_gbdt.py::test_equal_metric_rejected`

#### Scenario: RAT-41 Boundary, exactly +0.5 % passes the GBDT and two-tower policies
- **WHEN** the GBDT candidate has `ndcg@10=0.2010` against a champion at `0.2000` with `recall@10` at 0.95 times the champion's, and the two-tower candidate has `recall@10=0.2010` against `0.2000` with `coverage@10` 0.8 times the champion's and at least 0.5
- **THEN** both versions are tagged `gate.outcome=promotable`
- **VERIFIED BY**: `platform-recsys/tests/test_gate_gbdt.py::test_boundary_inclusive`, `platform-recsys/tests/test_gate_two_tower.py::test_boundary_inclusive`

#### Scenario: RAT-42 A champion without a guard metric skips that guard
- **WHEN** the GBDT champion (seeded from Redis) has no `recall@10`
- **THEN** the guard is skipped, the decision is made on `ndcg@10`, and the `gate.reason` notes the skipped guard
- **VERIFIED BY**: `platform-recsys/tests/test_gate_gbdt.py::test_champion_without_guard_metric`

### Requirement: The two-tower candidate is evaluated on a holdout before it can publish

The two-tower stage SHALL hold out a seeded fraction (`TWO_TOWER_HOLDOUT_FRACTION`, default 0.1, valid values strictly between 0 and 1) of its training pairs, train on the remainder, and compute `recall@10` and `coverage@10` of item-tower retrieval against the held-out pairs inside the job. Only these aggregate metrics SHALL leave the job. A run with fewer than 10 pairs, or whose holdout has no pairs, SHALL NOT produce a candidate. A `TWO_TOWER_HOLDOUT_FRACTION` of 0, 1 or outside that range SHALL be a configuration error (exit 2 before Spark starts).

#### Scenario: RAT-13 Metrics are computed on pairs the towers did not train on
- **WHEN** the stage runs on 1000 pairs with the default fraction and seed 42
- **THEN** exactly 100 pairs are held out, none of them appears in the training pairs, and the registered version carries numeric `recall@10` and `coverage@10`
- **VERIFIED BY**: `platform-recsys/tests/test_two_tower_holdout.py::test_holdout_is_disjoint_and_reproducible`

#### Scenario: RAT-14 Fewer than 10 pairs is not a candidate
- **WHEN** the stage has 9 pairs
- **THEN** no `recsys-two-tower` version is registered, the vectors are not published and the job exits 0 with `two_tower` decision `skipped`
- **VERIFIED BY**: `platform-recsys/tests/test_two_tower_holdout.py::test_too_few_pairs_skipped`

#### Scenario: RAT-43 Exactly 10 pairs is enough
- **WHEN** the stage has 10 pairs with the default fraction
- **THEN** one pair is held out and a `recsys-two-tower` version with numeric metrics is registered
- **VERIFIED BY**: `platform-recsys/tests/test_two_tower_holdout.py::test_ten_pairs_is_a_candidate`

#### Scenario: RAT-44 A holdout fraction outside (0, 1) is a configuration error
- **WHEN** `TWO_TOWER_HOLDOUT_FRACTION` is `0`, `1` or `1.5`
- **THEN** the job exits 2 before starting Spark and registers nothing
- **VERIFIED BY**: `platform-recsys/tests/test_config.py::test_holdout_fraction_bounds`

### Requirement: A generation is published in two phases and serving stays on the old one otherwise

The job SHALL run, in this order: (1) `evaluate` every candidate (ALS first; GBDT and two-tower only when ALS is `promotable`); (2) publish the generation made of the `promotable` models (Redis keys, Qdrant collections, then the Lua pointer swap `recs:v1:serving` / `recs:v1:previous`); (3) `apply` each published model, which moves its `champion` and keeps the old holder as `previous`. When no candidate is `promotable`, the job SHALL leave `recs:v1:serving`, `recs:v1:previous`, the Qdrant aliases and every `champion` alias unchanged. When a stage or the publish fails, the job SHALL call `mark_rejected` on every evaluated `promotable` version and SHALL NOT call `apply`; `previous` is never lost.

#### Scenario: RAT-15 Gate pass swaps the generation pointer
- **WHEN** the ALS candidate is `promotable` with generation `g2` while `recs:v1:serving` names `g1`
- **THEN** `recs:v1:serving` names `g2`, `recs:v1:previous` names `g1`, and team-ai `Recommend` answers from `g2`'s lists and vectors
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::Gate pass serves the new generation`

#### Scenario: RAT-16 Gate fail keeps the previous generation
- **WHEN** the ALS candidate is rejected while `recs:v1:serving` names `g1`
- **THEN** `recs:v1:serving` still names `g1`, `recs:v1:previous` is unchanged, no `g2` keys or collections exist, and `Recommend` answers are identical to before the run
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::Gate fail keeps the previous generation`

#### Scenario: RAT-17 A failed publish rejects the versions and leaves every alias alone
- **WHEN** the Qdrant write fails after the evaluation returned `promotable`
- **THEN** the pointer is not swapped, `apply` is never called, each evaluated version is tagged `gate.outcome=rejected` with `gate.reason` `publish failed: <ExceptionType>`, every `champion` and `previous` alias is as before, and the job exits 1
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_publish_failure_marks_rejected_aliases_untouched`

#### Scenario: RAT-45 A crash between evaluate and publish leaves aliases unchanged
- **WHEN** the process is killed after `evaluate` returned `promotable` and before the publish step
- **THEN** every `champion` and `previous` alias is unchanged, the version stays `promotable`, serving is unchanged, and the next run's gate compares its candidate against the old champion and ignores the stale `promotable` version
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_crash_after_evaluate_leaves_aliases`

#### Scenario: RAT-46 The registry going down at apply time is a non-zero exit with a named gap
- **WHEN** the pointer swap succeeded and `apply` fails with the registry unavailable
- **THEN** the job exits 1 and logs the models whose `champion` was not moved; models already applied stay applied, the rest stay `promotable`, and the next run compares against each model's current champion
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_apply_failure_exits_1_and_logs_gap`

#### Scenario: RAT-18 A rejected or skipped run still refreshes the serving TTL
- **WHEN** the gate rejects the candidate
- **THEN** the keys of the serving and previous generations have a TTL of 172800 s again
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_rejected_run_refreshes_ttl`

### Requirement: The job fails closed when the registry is unreachable

At job start, before any training, the job SHALL run a registry reachability pre-flight; when MLflow does not answer, the job SHALL exit 1 without starting Spark. Each registry operation has a total deadline of 12 s. When the registry becomes unavailable later, the job SHALL stop at that call with exit 1, write nothing further to Redis or Qdrant, and SHALL NOT fall back to an in-memory registry. A registry call that fails after the candidates were evaluated and before the pointer swap SHALL leave serving unchanged.

#### Scenario: RAT-19 Registry down at start
- **WHEN** `MLFLOW_TRACKING_URI` points at a port with no listener
- **THEN** the job exits 1 within 20 s of start, Spark is not started, `recs:v1:serving` and `recs:v1:previous` are unchanged, and no new Qdrant collection or Redis generation key exists
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::Registry down fails closed`

#### Scenario: RAT-20 Registry goes down after the ALS evaluation
- **WHEN** the registry stops answering after `evaluate` of the ALS candidate returned `promotable`, during the GBDT evaluation
- **THEN** the job exits 1 within 12 s of that call, publishes nothing, no alias moves, and versions may remain `promotable` (`mark_rejected` cannot run while the registry is down), and a later run with the registry back compares against the unchanged champion
- **VERIFIED BY**: `platform-recsys/tests/test_registry_fail_closed.py::test_outage_after_als_evaluate_publishes_nothing`

#### Scenario: RAT-21 A missing tracking URI is a configuration error
- **WHEN** `MLFLOW_TRACKING_URI` is unset or empty
- **THEN** the job exits 2 before starting Spark and registers nothing
- **VERIFIED BY**: `platform-recsys/tests/test_config.py::test_mlflow_uri_required`

#### Scenario: RAT-47 An unexpected stage exception exits 1
- **WHEN** the ALS trainer raises an unexpected exception
- **THEN** the job exits 1, registers no version and publishes nothing
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_gates.py::test_unexpected_exception_exit_code`

### Requirement: A stale dataset is refused

`MAX_DATASET_AGE_MINUTES` SHALL be an integer of 0 or more (a negative or non-numeric value is a configuration error, exit 2). When it is positive, the job SHALL refuse to train if the newest `als_interactions` snapshot's `as_of` is older than that many minutes: it SHALL exit with status 2 before Spark starts, register no model version and publish nothing. The value 0 disables the check. The gitops init container `dataset-fresh` SHALL remain.

#### Scenario: RAT-22 A 26-hour-old dataset is refused
- **WHEN** the newest snapshot is 26 hours old and `MAX_DATASET_AGE_MINUTES=1500`
- **THEN** the job exits 2, MLflow has no new version of any `recsys-*` model, and serving is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::Stale dataset is refused`

#### Scenario: RAT-23 Boundary, a dataset just inside the window trains
- **WHEN** the newest snapshot is 1499 minutes old and `MAX_DATASET_AGE_MINUTES=1500`
- **THEN** the job proceeds to training
- **VERIFIED BY**: `platform-recsys/tests/test_dataset.py::test_age_guard_boundary`

#### Scenario: RAT-24 The guard is off when the setting is 0
- **WHEN** `MAX_DATASET_AGE_MINUTES=0` and the snapshot is 30 days old
- **THEN** the job proceeds to training
- **VERIFIED BY**: `platform-recsys/tests/test_dataset.py::test_age_guard_disabled`

#### Scenario: RAT-48 A negative age setting is a configuration error
- **WHEN** `MAX_DATASET_AGE_MINUTES=-5`
- **THEN** the job exits 2 before starting Spark
- **VERIFIED BY**: `platform-recsys/tests/test_config.py::test_max_dataset_age_bounds`

### Requirement: Nothing personal reaches MLflow

The job SHALL send to MLflow only numeric metrics, an allow-listed set of params (hyperparameters, dataset lineage, feature list, counts), the drift sketches as a JSON artifact `drift_sketches.json`, and item-side artifacts (ALS item factors, GBDT ranker artifact, two-tower item vectors). It SHALL NOT send user ids, interaction rows, per-user lists or user factors, in params, metrics, tags (system tags and run name included) or artifact contents. The ALS artifact SHALL be written by the job as an item-factors file only; a Spark `ALSModel.save` directory (which contains `userFactors`) SHALL NOT be uploaded. Retention of everything sent follows the registry: kept for the life of the version, with no pruning.

#### Scenario: RAT-25 A full run leaks no user id
- **WHEN** a full run completes on a dataset whose user keys are `u-0001` ... `u-0050`
- **THEN** a scan of every param, metric, tag (system tags and run name included) and the contents of every artifact file of the run's MLflow runs finds none of those keys, and no artifact named like user factors exists
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::Registry holds no user data`

#### Scenario: RAT-49 The PII scan detects a planted key
- **WHEN** the scan runs against a run in which the key `u-0001` was deliberately logged as a param and as artifact text
- **THEN** the scan reports both hits (negative control: the scan can fail)
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::PII scan negative control`

#### Scenario: RAT-26 The adapter rejects a payload keyed by user id
- **WHEN** code passes a param dict or artifact path holding a user key to the recsys registry adapter
- **THEN** the adapter raises before any MLflow call is made
- **VERIFIED BY**: `platform-recsys/tests/test_registry_client.py::test_user_keyed_payload_refused`

#### Scenario: RAT-50 The ALS artifact holds item factors only
- **WHEN** the ALS candidate's artifact is built from a trained model with 50 users and 20 items
- **THEN** the artifact directory contains only the item factors file with 20 rows, and no file or column holds a user key or a user factor
- **VERIFIED BY**: `platform-recsys/tests/test_registry_client.py::test_als_artifact_is_item_factors_only`

#### Scenario: RAT-35 The nightly job is denied a force promotion
- **WHEN** the gate rejects the candidate and the job environment sets the removed `PROMOTION_FORCE=true`
- **THEN** the override is denied: the version stays `gate.outcome=rejected`, no `gate.forced` tag is set and the `champion` alias is unchanged (only `python -m modelregistry promote --force --reason` by an engineer can force)
- **VERIFIED BY**: `platform-recsys/tests/test_gate_als.py::test_nightly_job_cannot_force`

### Requirement: The Redis registry is replaced and its champion seeded once

`recsys/registry/` SHALL be removed and no code path SHALL read or write `recs:model:champion`, `recs:model:champion:gbdt` or `recs:model:meta:*` after migration. `python -m recsys migrate-registry` SHALL, for each model that has a Redis champion, call `seed` to create version 1 of the matching registered model with its metrics, lineage (the Redis meta's `eval_protocol` copied into `lineage.eval_protocol`) and the alias `champion`, without an artifact when the Redis meta holds none (a metrics-only v1 is allowed). A model that already has a version SHALL be left alone. Seeding SHALL NOT touch the serving pointer.

#### Scenario: RAT-27 Migration seeds the current champion as v1
- **WHEN** Redis holds a champion `als-2026-10-12` with `ndcg@10=0.100`, `coverage@10=0.5` and `eval_protocol=leave-last-new-item-v1`, the registry has no `recsys-als` version, and `migrate-registry` runs
- **THEN** `recsys-als` has version 1 with those metrics, `lineage.eval_protocol=leave-last-new-item-v1`, `gate.outcome=promoted`, `gate.reason` starting `seed:` and `@champion`; the next nightly candidate is compared against version 1 instead of auto-promoting
- **VERIFIED BY**: `platform-recsys/tests/test_migrate_registry.py::test_seeds_v1_champion`

#### Scenario: RAT-28 Migration is idempotent
- **WHEN** `migrate-registry` runs a second time
- **THEN** `recsys-als` still has exactly one version, the `champion` alias is unchanged, and the command exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_migrate_registry.py::test_second_run_is_noop`

#### Scenario: RAT-29 Migration with the registry down fails and changes nothing
- **WHEN** `migrate-registry` runs while MLflow is unreachable
- **THEN** it exits 1 and the Redis keys are not deleted
- **VERIFIED BY**: `platform-recsys/tests/test_migrate_registry.py::test_registry_down_exit_nonzero`

#### Scenario: RAT-30 No code path touches the old registry
- **WHEN** the recsys package is imported and a full in-memory run executes
- **THEN** `recsys.registry` does not exist and no Redis key beginning `recs:model:` is written
- **VERIFIED BY**: `platform-recsys/tests/test_no_redis_registry.py::test_no_model_keys_written`

#### Scenario: RAT-51 A Redis champion lacking a guard metric still seeds and the guard is skipped
- **WHEN** the Redis champion's metrics have `ndcg@10` but no `coverage@10`, migration runs, and the next ALS candidate is evaluated
- **THEN** version 1 is seeded with the metrics it has, and the candidate's `gate.reason` notes the skipped `coverage@10` guard
- **VERIFIED BY**: `platform-recsys/tests/test_migrate_registry.py::test_seed_without_guard_metric`

#### Scenario: RAT-52 A Redis champion with no eval_protocol is judged on floors on the first run
- **WHEN** the Redis meta has no `eval_protocol` and the next ALS candidate is evaluated
- **THEN** the champion has no `lineage.eval_protocol`, the candidate is judged on the floors only, and the `gate.reason` starts `eval_protocol changed:`
- **VERIFIED BY**: `platform-recsys/tests/test_migrate_registry.py::test_seed_without_protocol_floors_first_run`

#### Scenario: RAT-53 Seeding a model that already has versions is refused and skipped
- **WHEN** `recsys-gbdt` already has a version and `migrate-registry` runs
- **THEN** `seed` is refused for that model, the model is left unchanged, the other models are still seeded, and the command exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_migrate_registry.py::test_existing_versions_left_alone`

### Requirement: Rollback follows the registry, registry first and then the pointer

`python -m recsys rollback` SHALL swap `champion` and `previous` through the shared library for each model carried by both the serving and the previous generation (ALS always, GBDT when the generation has a ranker key, two-tower when it has a two-tower collection), and only then swap `recs:v1:serving` and `recs:v1:previous` and repoint the aliases. A model carried by only one of the two generations SHALL be left untouched and the omission logged. Before changing anything it SHALL check that there is a previous generation, that its keys and collections exist, that every model to roll back has a `previous` version with an artifact, and that the version each carried model of the serving generation was built from is that model's registry `champion` (serving and registry out of step, for example after RAT-46, is refused); otherwise it SHALL change nothing and exit 2. If a registry call fails after some models were swapped, it SHALL swap those back, leave the pointer unchanged and exit 1. A forced promote or rollback made directly in the registry is not mirrored into serving.

#### Scenario: RAT-31 Rollback restores serving and the champion alias
- **WHEN** `recs:v1:serving` names `g2`, `previous` names `g1`, and `rollback` runs
- **THEN** `recs:v1:serving` names `g1`, `recs:v1:previous` names `g2`, and `recsys-als` `@champion` names the version that built `g1`
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_rollback_moves_registry_champion`

#### Scenario: RAT-32 Rollback with no previous generation is refused
- **WHEN** `recs:v1:previous` is unset and `rollback` runs
- **THEN** it exits 2 and no alias or pointer changes
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_rollback_without_previous_refused`

#### Scenario: RAT-54 Rollback to an artifact-less version is refused
- **WHEN** the `previous` version of `recsys-als` is the metrics-only v1 created by `seed`
- **THEN** `rollback` exits 2 and `champion`, `previous` and the pointer are unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_rollback_to_artifactless_refused`

#### Scenario: RAT-55 A model that cannot roll back stops the whole rollback
- **WHEN** the serving generation carries a GBDT ranker and `recsys-gbdt` has no `previous`
- **THEN** `rollback` exits 2 before any alias moves, and `recsys-als` is not swapped
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_partial_rollback_refused_before_any_change`

#### Scenario: RAT-60 Rollback is refused when serving and the registry are out of step
- **WHEN** serving names `g2` but `recsys-als@champion` still names the version that built `g1` (for example after RAT-46)
- **THEN** `rollback` exits 2 and `champion`, `previous` and the pointer are unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_rollback_refused_when_serving_ahead_of_registry`

#### Scenario: RAT-61 Models present in only one generation are not swapped
- **WHEN** serving `g2` has a GBDT ranker, the previous `g1` has none, and `rollback` runs
- **THEN** `recsys-als` is swapped, `recsys-gbdt` `champion` and `previous` are unchanged, the log notes the omission, the pointer is swapped and the command exits 0
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_rollback_swaps_only_models_in_both_generations`

#### Scenario: RAT-56 Registry down during rollback changes nothing
- **WHEN** MLflow becomes unreachable after `recsys-als` was swapped and before `recsys-gbdt` is
- **THEN** `recsys-als` is swapped back, `recs:v1:serving` is unchanged, and the command exits 1
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_registry_down_mid_rollback_restores`

#### Scenario: RAT-57 Registry down at the start of rollback
- **WHEN** MLflow is unreachable when `rollback` starts
- **THEN** the command exits 1 within 20 s and nothing changes
- **VERIFIED BY**: `platform-recsys/tests/test_generations.py::test_rollback_registry_down_exit_1`

### Requirement: Drift baselines come from the champion's MLflow run

Each run SHALL write its quantile sketches (`weight`, `user_items`, `item_users`, `top_score`, 101 points each) as the JSON artifact `drift_sketches.json` of its MLflow run. The next run SHALL read the artifact of the run behind the `recsys-als` `champion` version as its drift baseline. A missing champion or missing artifact SHALL give the status `no_baseline` and SHALL NOT affect the gate decision.

#### Scenario: RAT-58 The second run reads the first run's sketches
- **WHEN** the job runs twice on the same dataset and the first run's version is `champion`
- **THEN** the second run's drift names the first run's version as baseline and reports a PSI for each of the four features
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_drift.py::test_baseline_from_champion_artifact`

#### Scenario: RAT-59 A seeded champion has no sketches and gives no_baseline
- **WHEN** the champion is the seeded v1 without an artifact
- **THEN** drift is `no_baseline` and the gate decision is unchanged
- **VERIFIED BY**: `platform-recsys/tests/test_pipeline_drift.py::test_seeded_champion_no_baseline`

### Requirement: The nightly schedule runs the full chain unattended

The gitops CronJob `platform-recsys` SHALL set `ENABLE_GBDT=true`, `ENABLE_TWO_TOWER=true` and `MLFLOW_TRACKING_URI=http://mlflow:5050`, SHALL allow egress to MLflow TCP 5050, SHALL keep the schedule `0 3 * * *` after the featurestore `15 2 * * *` run, and SHALL set `activeDeadlineSeconds: 3600` so that the whole job is bounded to 60 minutes. The compose `platform-recsys` job service SHALL set the same three variables.

#### Scenario: RAT-33 The manifest enables all three models and reaches MLflow
- **WHEN** the manifests under `platform-gitops/platform/recsys/` are rendered
- **THEN** the `recsys-config` ConfigMap has `ENABLE_GBDT=true`, `ENABLE_TWO_TOWER=true` and `MLFLOW_TRACKING_URI=http://mlflow:5050`, the NetworkPolicy has an egress rule to pods labelled `app: mlflow` on TCP 5050, the schedule is `0 3 * * *` and `activeDeadlineSeconds` is 3600
- **VERIFIED BY**: `platform-recsys/tests/test_gitops_manifest.py::test_cronjob_enables_chain`

#### Scenario: RAT-34 The nightly chain end to end on the stack
- **WHEN** the featurestore dataset job finishes and then the recsys job runs on the compose stack with the `jobs` profile against seeded weak champions
- **THEN** MLflow on :5050 shows a new version of each of `recsys-als`, `recsys-gbdt` and `recsys-two-tower` tagged `gate.outcome=promoted`, and the job exits 0
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/recsys_auto_train.feature::Nightly chain registers all three models`

#### Scenario: RAT-36 A manifest without the MLflow URI is rejected by the manifest check
- **WHEN** the CronJob `recsys-config` lacks `MLFLOW_TRACKING_URI` or sets `ENABLE_GBDT=false`
- **THEN** the manifest test fails naming the missing or wrong key
- **VERIFIED BY**: `platform-recsys/tests/test_gitops_manifest.py::test_manifest_missing_key_fails`
