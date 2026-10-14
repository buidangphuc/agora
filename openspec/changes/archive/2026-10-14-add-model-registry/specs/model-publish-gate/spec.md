## ADDED Requirements

All scenario test names below are in `platform-core/packages/py-modelregistry/tests/` and run offline against an MLflow file store unless marked e2e. Metrics in scenarios are named by the policy, for example `ndcg@10`. Version states are the value of tag `gate.outcome`: `promotable` (passed the gate, no alias moved yet), `promoted` (holds or held `champion`), `rejected`. A version with no `gate.outcome` tag is `pending` (registered, not yet decided).

### Requirement: evaluate registers a version and decides without moving any alias

`evaluate(model, run_id, metrics, artifact_path, lineage, policies_path=None)` SHALL create a new version of registered model `model` (creating the model when absent), log `metrics` on run `run_id`, log the files at `artifact_path` to that run under `model/` and make that run the version's source (so the version's run id is `run_id`), set tags `lineage.data_version`, `lineage.eval_protocol` and, when given, `lineage.train_hw`, evaluate the model's policy, set tags `gate.outcome` (`promotable` or `rejected`) and `gate.reason`, and return `Decision{version, outcome, reason}`. It SHALL move no alias.

When the model has a `champion`, the candidate is compared with it only if the candidate's `lineage.eval_protocol` equals the champion version's `lineage.eval_protocol`. Then the candidate SHALL be `promotable` only if the relative improvement of `primary`, `(candidate - champion) / champion`, is at least `min_relative_improvement` and every guard satisfies `candidate[g] >= ratio * champion[g]`. Champion metrics are the latest logged value of each metric key on the champion version's source run. Thresholds are inclusive, and a threshold comparison SHALL treat values within 1e-9 (absolute, on the ratio) as equal, so a candidate mathematically at the threshold is accepted. The Decision is a pure function of candidate metrics, champion metrics, protocols and policy.

#### Scenario: Passing candidate is promotable and no alias moves
- **WHEN** the champion is version 1 with `ndcg@10=0.100` and `coverage@10=0.50`, the policy is `min_relative_improvement=0.01` with guard `coverage@10: 0.8`, and `evaluate` is called with `ndcg@10=0.102` and `coverage@10=0.45`
- **THEN** the Decision outcome is `promotable`, the new version has tag `gate.outcome=promotable`, alias `champion` still points at version 1 and no alias `previous` was created
- **VERIFIED BY**: `test_evaluate.py::test_pass_is_promotable_and_moves_no_alias`

#### Scenario: Insufficient improvement is rejected
- **WHEN** the same champion and policy are used and `evaluate` is called with `ndcg@10=0.1005`
- **THEN** the Decision outcome is `rejected`, the reason names `ndcg@10` and the required 0.01, the version has tags `gate.outcome=rejected` and a non-empty `gate.reason`, and alias `champion` is unchanged
- **VERIFIED BY**: `test_evaluate.py::test_insufficient_improvement_rejected`

#### Scenario: Guard violation is rejected
- **WHEN** the champion has `coverage@10=0.50` and `evaluate` is called with `ndcg@10=0.120` and `coverage@10=0.39` under guard `coverage@10: 0.8`
- **THEN** the Decision outcome is `rejected` and the reason names `coverage@10`
- **VERIFIED BY**: `test_evaluate.py::test_guard_violation_rejected`

#### Scenario: Thresholds are inclusive
- **WHEN** a candidate has relative improvement exactly equal to `min_relative_improvement` and a guard metric exactly equal to `ratio * champion[g]`
- **THEN** the Decision outcome is `promotable`
- **VERIFIED BY**: `test_evaluate.py::test_thresholds_are_inclusive`

#### Scenario: Floating-point rounding at the threshold is tolerated
- **WHEN** the champion `ndcg@10` is 0.5, `min_relative_improvement` is 0.005 and a candidate with `ndcg@10=0.5025` is evaluated
- **THEN** the Decision outcome is `promotable`
- **VERIFIED BY**: `test_evaluate.py::test_threshold_tolerance_1e9`

#### Scenario: Champion metric is the latest value
- **WHEN** the champion's source run logged `ndcg@10` at steps 0 and 1 with values 0.4 and 0.5
- **THEN** the comparison uses 0.5
- **VERIFIED BY**: `test_evaluate.py::test_champion_metric_uses_latest_value`

#### Scenario: Run, artifact and lineage are attached to the version
- **WHEN** `evaluate` is called with `run_id=R`, `artifact_path=P` (a directory with one file) and lineage `{data_version: "als_interactions@v1", eval_protocol: "p1", train_hw: "cpu"}`
- **THEN** the version's run id is `R`, run `R` holds that file under `model/`, and the version has tags `lineage.data_version=als_interactions@v1`, `lineage.eval_protocol=p1` and `lineage.train_hw=cpu`; tag `lineage.train_hw` is absent when `train_hw` was not given
- **VERIFIED BY**: `test_evaluate.py::test_artifact_run_and_lineage_tags`

#### Scenario: Decision is deterministic
- **WHEN** the same candidate metrics, champion metrics, protocols and policy are evaluated twice, each against an identical fresh registry
- **THEN** both Decisions have equal `outcome` and equal `reason` strings
- **VERIFIED BY**: `test_evaluate.py::test_decision_is_deterministic`

### Requirement: The comparison is skipped when the evaluation protocol differs

WHEN the candidate's `lineage.eval_protocol` differs from the champion's, or the champion version has no `lineage.eval_protocol` tag, THE SYSTEM SHALL NOT compare metrics with the champion. It SHALL judge the candidate on the policy's `floors` only, and the reason SHALL be `eval_protocol changed: <old> -> <new>` (with `<old>` equal to `none` when the champion has no tag), followed by the floor result.

#### Scenario: Protocol mismatch judges floors only
- **WHEN** the champion has protocol `p1` and `ndcg@10=0.300`, the policy has floors `{ndcg@10: 0.05}`, and a candidate with protocol `p2` and `ndcg@10=0.060` is evaluated
- **THEN** the Decision outcome is `promotable` and the reason starts with `eval_protocol changed: p1 -> p2`, even though the candidate is below the champion
- **VERIFIED BY**: `test_evaluate.py::test_protocol_mismatch_floors_only`

#### Scenario: Protocol mismatch below a floor is rejected
- **WHEN** the same setup is used with a candidate `ndcg@10=0.040`
- **THEN** the Decision outcome is `rejected` and the reason names `ndcg@10` and 0.05
- **VERIFIED BY**: `test_evaluate.py::test_protocol_mismatch_below_floor_rejected`

#### Scenario: Matching protocol is compared
- **WHEN** candidate and champion both have protocol `p1` and the candidate is below the champion by more than the policy allows
- **THEN** the Decision outcome is `rejected` with a reason that names the improvement shortfall and does not start with `eval_protocol changed`
- **VERIFIED BY**: `test_evaluate.py::test_protocol_match_is_compared`

#### Scenario: Champion without a protocol tag
- **WHEN** the champion version has no `lineage.eval_protocol` tag and a candidate with protocol `p1` is evaluated
- **THEN** metrics are not compared and the reason starts with `eval_protocol changed: none -> p1`
- **VERIFIED BY**: `test_evaluate.py::test_champion_without_protocol_tag`

### Requirement: Missing and non-finite metrics are decided, never raised

IF a candidate's `primary`, guard or floor metric is missing, NaN or infinite, THEN THE SYSTEM SHALL register the version as `rejected` with a reason naming the metric, and SHALL NOT raise. IF a guard metric is missing on the champion, THEN that guard SHALL be skipped and the skip SHALL be noted in the reason. WHEN the champion's `primary` is 0 or negative, the relative improvement SHALL NOT be computed; the candidate SHALL be `promotable` if `candidate primary > champion primary` and every floor is met.

#### Scenario: Candidate missing the primary metric
- **WHEN** `evaluate` is called without the `primary` metric
- **THEN** no exception is raised, the version is registered with `gate.outcome=rejected`, and the reason names the metric as missing
- **VERIFIED BY**: `test_evaluate.py::test_missing_primary_rejected`

#### Scenario: Candidate metric is NaN or infinite
- **WHEN** a guard metric is NaN, or a floor metric is infinite
- **THEN** the Decision outcome is `rejected` and the reason names that metric as not finite
- **VERIFIED BY**: `test_evaluate.py::test_non_finite_metrics_rejected`

#### Scenario: Candidate missing a floor metric
- **WHEN** the model has no champion, the floors are `{coverage@10: 0.5}` and the candidate has no `coverage@10`
- **THEN** the Decision outcome is `rejected` and the reason names `coverage@10`
- **VERIFIED BY**: `test_evaluate.py::test_missing_floor_metric_rejected`

#### Scenario: Guard missing on the champion is skipped and noted
- **WHEN** the policy guards `recall@10`, the champion run has no `recall@10`, and the candidate clears the primary threshold
- **THEN** the Decision outcome is `promotable` and the reason contains `recall@10` and `skipped`
- **VERIFIED BY**: `test_evaluate.py::test_guard_missing_on_champion_skipped`

#### Scenario: Champion primary is zero or negative
- **WHEN** the champion `primary` is 0 (or -0.2) and a candidate with `primary=0.05` meeting all floors is evaluated
- **THEN** no arithmetic error is raised and the Decision outcome is `promotable`; a candidate with `primary` equal to the champion's is `rejected`
- **VERIFIED BY**: `test_evaluate.py::test_champion_primary_not_positive`

### Requirement: The first version needs to meet the floors

WHEN a model has no `champion`, THE SYSTEM SHALL mark a candidate `promotable` only if every metric in the policy's `floors` is present, finite and at least its floor value, and `primary` is present and finite. A policy whose `floors` is empty SHALL make any candidate with a finite `primary` promotable.

#### Scenario: First champion via floors
- **WHEN** model `m` has no champion, the floors are `{ndcg@10: 0.05}`, `evaluate` is called with `ndcg@10=0.06` and then `apply` is called on the returned version
- **THEN** the Decision outcome is `promotable`, after `apply` alias `champion` points at that version and alias `previous` does not exist
- **VERIFIED BY**: `test_evaluate.py::test_first_champion_meets_floors`

#### Scenario: Candidate below a floor is rejected and no champion is set
- **WHEN** model `m` has no champion, the floors are `{ndcg@10: 0.05}`, and `evaluate` is called with `ndcg@10=0.049`
- **THEN** the Decision outcome is `rejected`, the reason names `ndcg@10` and 0.05, and the model has no alias `champion`
- **VERIFIED BY**: `test_evaluate.py::test_first_candidate_below_floor_rejected`

#### Scenario: Candidate exactly at the floor passes
- **WHEN** the candidate `ndcg@10` equals the floor 0.05 exactly
- **THEN** the Decision outcome is `promotable`
- **VERIFIED BY**: `test_evaluate.py::test_floor_is_inclusive`

### Requirement: The champion can be read

`get_champion(model)` SHALL return `{version, run_id, metrics, tags}` for the version holding alias `champion`, with `metrics` the latest value of each metric key on its source run, or `None` when the model has no `champion`. It SHALL NOT write to the registry.

#### Scenario: Read the champion
- **WHEN** a model was seeded or promoted with run `R` holding `ndcg@10=0.5`, and `get_champion` is called
- **THEN** it returns the champion's version number, `run_id=R`, `metrics` containing `ndcg@10=0.5`, and the version tags including `gate.outcome=promoted`
- **VERIFIED BY**: `test_apply.py::test_get_champion_returns_run_and_metrics`

#### Scenario: No champion returns None, not an error
- **WHEN** `get_champion` is called for a model with no `champion` alias, or no versions
- **THEN** it returns `None` and does not raise and creates nothing
- **VERIFIED BY**: `test_apply.py::test_get_champion_none_without_champion`

### Requirement: apply moves the alias only for a promotable version

`apply(model, version)` SHALL, for a version whose `gate.outcome` is `promotable`, set alias `previous` to the current champion (when one exists), set alias `champion` to `version` and set tag `gate.outcome=promoted`. IF the version's `gate.outcome` is anything else (`rejected`, `promoted`, or no tag), THEN `apply` SHALL raise `GateError` and change nothing. `mark_rejected(model, version, reason)` SHALL set `gate.outcome=rejected` and `gate.reason=<reason>` and move no alias. `register_rejected(model, run_id, reason, lineage)` SHALL register a version for run `run_id` as `rejected` with that reason and lineage, without evaluating the policy and without moving an alias. `submit(...)` SHALL equal `evaluate` followed by `apply` when the outcome is `promotable`, and return the Decision of the evaluate (with outcome `promoted` after the apply).

#### Scenario: Apply after evaluate moves the aliases
- **WHEN** `evaluate` returned `promotable` for version 2 while version 1 is champion, then `apply(m, 2)` runs
- **THEN** alias `champion` points at version 2, alias `previous` points at version 1 and version 2 has tag `gate.outcome=promoted`
- **VERIFIED BY**: `test_apply.py::test_apply_moves_champion_and_sets_previous`

#### Scenario: Apply is refused on a rejected version
- **WHEN** `apply(m, 3)` runs and version 3 has `gate.outcome=rejected`
- **THEN** `GateError` is raised and aliases and tags are unchanged
- **VERIFIED BY**: `test_apply.py::test_apply_refused_on_rejected`

#### Scenario: Apply is refused on a pending version
- **WHEN** `apply(m, 4)` runs and version 4 has no `gate.outcome` tag
- **THEN** `GateError` is raised and aliases are unchanged
- **VERIFIED BY**: `test_apply.py::test_apply_refused_on_pending`

#### Scenario: Crash between evaluate and apply changes nothing served
- **WHEN** `evaluate` returns `promotable` and the process ends before `apply`
- **THEN** alias `champion` and alias `previous` are unchanged, the version stays `promotable`, and a later `evaluate` of a new candidate compares with the unchanged champion and ignores the stale `promotable` version
- **VERIFIED BY**: `test_apply.py::test_crash_between_evaluate_and_apply_leaves_aliases`

#### Scenario: mark_rejected after a downstream failure
- **WHEN** `mark_rejected(m, 2, "publish failed")` runs on a `promotable` version
- **THEN** version 2 has `gate.outcome=rejected` and `gate.reason=publish failed`, and the aliases are unchanged
- **VERIFIED BY**: `test_apply.py::test_mark_rejected_touches_no_alias`

#### Scenario: register_rejected records a structural rejection
- **WHEN** `register_rejected(m, run_id, "dataset stale", lineage)` runs
- **THEN** a new version exists with `gate.outcome=rejected`, `gate.reason=dataset stale` and the lineage tags, the policy was not read, and the aliases are unchanged
- **VERIFIED BY**: `test_apply.py::test_register_rejected_skips_policy`

#### Scenario: Submit is evaluate then apply
- **WHEN** `submit` is called twice, the second candidate better than the first by more than the policy minimum
- **THEN** the second Decision has outcome `promoted`, alias `champion` is on version 2 and alias `previous` on version 1; a failing third candidate returns `rejected` and leaves both aliases
- **VERIFIED BY**: `test_apply.py::test_submit_is_evaluate_then_apply`

#### Scenario: Submit against a real MLflow server
- **WHEN** two versions of `e2e-gate-probe` are submitted to `http://localhost:5050`, the second better than the first by more than the policy minimum
- **THEN** the registry shows alias `champion` on version 2 and alias `previous` on version 1
- **VERIFIED BY**: e2e `platform-e2e` feature `model_registry` scenario `gate_against_real_server`

### Requirement: Policies are validated, resolved by a fixed order, and a model without a policy is never decided

The library SHALL resolve the policy file as: the `policies_path` argument, else env `MODELREGISTRY_POLICIES`, else the packaged `policies.yaml`. It SHALL validate it against the schema `<model>: {primary: str, min_relative_improvement: float >= 0, guards: {<metric>: ratio > 0}, floors: {<metric>: float}}`, where `primary` is required and the other keys optional. An unknown key, a wrong type or an out-of-range value SHALL raise `PolicyError` naming the model and key. `evaluate` and `submit` for a model with no policy entry SHALL raise `PolicyError` before any registry write.

#### Scenario: Unknown model has no policy
- **WHEN** `evaluate` is called for model `not-in-policies`
- **THEN** `PolicyError` is raised naming the model, and no version is created
- **VERIFIED BY**: `test_policy.py::test_unknown_model_raises_before_any_write`

#### Scenario: Malformed policy is rejected
- **WHEN** a policy entry has `guards: {coverage@10: 0}` or an unknown key `min_improvment`
- **THEN** loading raises `PolicyError` whose message names the model and the offending key
- **VERIFIED BY**: `test_policy.py::test_malformed_entries_raise`

#### Scenario: Resolution order
- **WHEN** a model is defined in the argument file, in the file named by `MODELREGISTRY_POLICIES` with a different `primary`, and in the packaged file
- **THEN** the argument file wins; without the argument the env file wins; without both the packaged file is used
- **VERIFIED BY**: `test_policy.py::test_resolution_order`

#### Scenario: Shipped policies.yaml loads and holds no live model
- **WHEN** the `policies.yaml` shipped in the package is loaded
- **THEN** loading succeeds and the file holds no model entries other than those the later slices add; the documented example is in a comment and is not loaded
- **VERIFIED BY**: `test_policy.py::test_shipped_file_loads`

#### Scenario: Omitted optional keys take defaults
- **WHEN** an entry has only `primary: f1`
- **THEN** it loads with `min_relative_improvement=0.0`, no guards and no floors
- **VERIFIED BY**: `test_policy.py::test_defaults`

### Requirement: Forced promotion is explicit and recorded

`promote(model, version, force=True, reason=...)` SHALL move alias `champion` to `version` without evaluating the policy, move the old holder to alias `previous`, and set the version tags `gate.outcome=promoted`, `gate.forced=true` and `gate.reason=<reason>`. A forced promotion SHALL require a non-empty `reason` and raise `ValueError` otherwise. `promote(model, version, force=False)` SHALL re-run the gate on the version's recorded run metrics and protocol and, when the result is `promotable`, apply it. A forced promote or a rollback changes the registry only and is not mirrored into any consumer's serving state.

#### Scenario: Force moves the alias and records the override
- **WHEN** `python -m modelregistry promote m 3 --force --reason "hotfix for stale data"` runs and version 3 was `rejected`
- **THEN** the exit code is 0, alias `champion` points at version 3, alias `previous` points at the old champion, and version 3 has tags `gate.forced=true`, `gate.outcome=promoted` and `gate.reason=hotfix for stale data`
- **VERIFIED BY**: `test_cli.py::test_force_promote_records_override`

#### Scenario: Force without a reason is refused
- **WHEN** `python -m modelregistry promote m 3 --force` runs with no `--reason`, or with an empty one
- **THEN** the exit code is 2, nothing is written to the registry and the error text names `--reason`
- **VERIFIED BY**: `test_cli.py::test_force_without_reason_refused`

#### Scenario: Promoting an unknown version fails
- **WHEN** `promote m 99 --force --reason x` runs and version 99 does not exist
- **THEN** the exit code is 2, the error names version 99 and the aliases are unchanged
- **VERIFIED BY**: `test_cli.py::test_unknown_version_exit_2`

#### Scenario: Unforced promote that fails the gate exits 0 with the decision
- **WHEN** `python -m modelregistry promote m 3` runs (no `--force`) and version 3 fails the policy against the champion
- **THEN** the exit code is 0, stdout is a JSON object with `outcome` equal to `rejected`, the `version` and the `reason`, and alias `champion` is unchanged
- **VERIFIED BY**: `test_cli.py::test_unforced_promote_rejected_exits_0_with_json`

#### Scenario: Unforced promote that passes applies it
- **WHEN** `promote m 3` runs and version 3 passes the policy
- **THEN** the exit code is 0, stdout JSON has `outcome` equal to `promoted` and alias `champion` points at version 3
- **VERIFIED BY**: `test_cli.py::test_unforced_promote_passes_applies`

### Requirement: Rollback swaps champion and previous

`rollback(model)` SHALL swap aliases `champion` and `previous`, so that a second rollback in a row toggles back. It SHALL set tag `gate.reason` on the restored champion to a text starting `rollback`. IF alias `previous` does not exist, THEN `rollback` SHALL raise `RollbackError` and change nothing.

#### Scenario: Rollback swaps the aliases
- **WHEN** `python -m modelregistry rollback m` runs with champion version 2 and previous version 1
- **THEN** the exit code is 0, alias `champion` points at version 1 and alias `previous` points at version 2
- **VERIFIED BY**: `test_cli.py::test_rollback_swaps_champion_and_previous`

#### Scenario: A second rollback toggles back
- **WHEN** `rollback m` runs twice in a row
- **THEN** after the second call alias `champion` points at version 2 and alias `previous` at version 1
- **VERIFIED BY**: `test_promote.py::test_double_rollback_toggles`

#### Scenario: Rollback without a previous is refused
- **WHEN** `rollback m` runs on a model that has a champion and no `previous` alias
- **THEN** `RollbackError` is raised, the CLI exit code is 2 and alias `champion` is unchanged
- **VERIFIED BY**: `test_cli.py::test_rollback_without_previous_exit_2`

### Requirement: Seed bypasses the gate for an empty model only

`seed(model, metrics, lineage, reason, artifact_path=None)` SHALL create version 1 of a model that has no versions, set alias `champion` to it, log `metrics` on a new run, and set tags `gate.outcome=promoted`, `gate.reason=seed: <reason>` and the lineage tags, without reading the policy. A version with metrics and no artifact SHALL be allowed. IF the model already has any version, THEN `seed` SHALL raise `GateError` and change nothing.

#### Scenario: Seed creates the first champion
- **WHEN** `seed("m", {"ndcg@10": 0.1}, {"data_version": "d1", "eval_protocol": "p1"}, "migrate from redis")` runs on a model with no versions
- **THEN** version 1 exists with alias `champion`, tags `gate.outcome=promoted`, `gate.reason=seed: migrate from redis` and `lineage.eval_protocol=p1`, and its run holds `ndcg@10=0.1`
- **VERIFIED BY**: `test_seed.py::test_seed_creates_first_champion`

#### Scenario: Seed without an artifact is allowed
- **WHEN** `seed` runs with `artifact_path=None`
- **THEN** the version is created and has no files under `model/` on its run
- **VERIFIED BY**: `test_seed.py::test_seed_metrics_only`

#### Scenario: Seed refused when versions exist
- **WHEN** `seed` runs for a model that already has a version, including a `rejected` one
- **THEN** `GateError` is raised and no version, alias or tag is written
- **VERIFIED BY**: `test_seed.py::test_seed_refused_when_versions_exist`

#### Scenario: The next evaluate compares against the seeded champion
- **WHEN** a model was seeded with `ndcg@10=0.1` and protocol `p1`, and a candidate with `ndcg@10=0.2` and protocol `p1` is evaluated
- **THEN** the Decision is `promotable` with a reason showing the relative improvement, not a floors-only reason
- **VERIFIED BY**: `test_seed.py::test_evaluate_after_seed_compares`

### Requirement: The gate fails closed when the registry is unavailable

WHEN the registry cannot be reached, THE SYSTEM SHALL raise `RegistryUnavailable` from `evaluate`, `apply`, `submit`, `promote`, `rollback` and `seed`, SHALL NOT return a `promotable` or `promoted` Decision, and SHALL NOT fall back to any in-memory or local decision. Each operation SHALL have a total deadline of 12 s across all its registry calls, and each single call SHALL time out within 10 s. The library SHALL provide `check_reachable()`, which raises `RegistryUnavailable` within the same 12 s deadline (a trainer exits 1 on it) and is run by trainers before training. WHEN an outage interrupts `evaluate`, THE SYSTEM SHALL leave the version `pending` (no `gate.outcome` tag) or `promotable`, never `promoted`, and no alias SHALL have moved. The single-writer assumption (one trainer per model at a time, enforced by `concurrencyPolicy: Forbid` on the consumers' CronJobs) is recorded in ADR-0017; the library takes no lock.

#### Scenario: Unreachable registry raises and decides nothing
- **WHEN** `evaluate` runs with `MLFLOW_TRACKING_URI` pointing at a closed port
- **THEN** `RegistryUnavailable` is raised and no Decision is returned
- **VERIFIED BY**: `test_failclosed.py::test_evaluate_raises_when_unreachable`

#### Scenario: CLI exits 1 when the registry is down
- **WHEN** `python -m modelregistry rollback m` runs with the registry unreachable
- **THEN** the exit code is 1 and stderr contains `RegistryUnavailable`
- **VERIFIED BY**: `test_failclosed.py::test_cli_exit_1_when_unreachable`

#### Scenario: A hanging registry times out within the deadline
- **WHEN** `evaluate` runs against a TCP listener that accepts connections and never replies
- **THEN** `RegistryUnavailable` is raised after no more than 12 s of wall time
- **VERIFIED BY**: `test_failclosed.py::test_hanging_server_times_out_within_deadline`

#### Scenario: First call answers, then the registry stalls
- **WHEN** a stub server answers the first request normally and then accepts connections without replying, and `submit` runs
- **THEN** `RegistryUnavailable` is raised within 12 s total, not 10 s per remaining call
- **VERIFIED BY**: `test_failclosed.py::test_total_deadline_across_calls`

#### Scenario: Timeout is capped at 10 s per call
- **WHEN** the environment sets `MLFLOW_HTTP_REQUEST_TIMEOUT=300` and the client is built
- **THEN** the effective request timeout is 10 s
- **VERIFIED BY**: `test_failclosed.py::test_timeout_is_capped`

#### Scenario: Reachability check
- **WHEN** the reachability check runs against a closed port and then against a live file or server store
- **THEN** the first raises `RegistryUnavailable` within 12 s and the second returns without error
- **VERIFIED BY**: `test_failclosed.py::test_reachability_check`

#### Scenario: Outage mid-evaluate leaves no champion change and no promoted version
- **WHEN** the registry becomes unreachable after the version is created and before the tags are written
- **THEN** `RegistryUnavailable` is raised, the version has no `gate.outcome=promoted`, alias `champion` points at the old champion once the registry is back, and `apply` on that version raises `GateError` if it is still `pending`
- **VERIFIED BY**: `test_failclosed.py::test_outage_mid_evaluate_keeps_champion`

### Requirement: The CLI exit codes are fixed

The CLI `python -m modelregistry` SHALL exit 0 when the operation completed, including a `rejected` Decision (printed as JSON on stdout); 1 on `RegistryUnavailable` or an unexpected exception; and 2 on a usage or configuration error, `PolicyError`, `GateError`, `RollbackError` or an unknown model or version.

#### Scenario: PolicyError exits 2
- **WHEN** `python -m modelregistry promote m 3` runs and `m` has no policy entry
- **THEN** the exit code is 2 and stderr names `PolicyError` and the model
- **VERIFIED BY**: `test_cli.py::test_policy_error_exit_2`

#### Scenario: Unknown subcommand or missing argument exits 2
- **WHEN** `python -m modelregistry rollback` runs with no model argument
- **THEN** the exit code is 2 and usage text is printed
- **VERIFIED BY**: `test_cli.py::test_usage_error_exit_2`

#### Scenario: Unexpected exception exits 1
- **WHEN** the client layer raises an unexpected `RuntimeError` during `rollback m`
- **THEN** the exit code is 1 and stderr contains the exception text
- **VERIFIED BY**: `test_cli.py::test_unexpected_exception_exit_1`
