## ADDED Requirements

### Requirement: The finetune pipeline is imported with history as platform-finetune

The workspace SHALL contain `platform-finetune/`, created with `git subtree add --prefix=platform-finetune` from the MLOps repo branch `main`, so that its commit history is kept and `artifacts/` is not tracked. The import SHALL NOT contain `ops/docker-compose.yml`. Each of `serve/local.py`, `package.py`, `optimize.py`, `serve_analyzer.py`, `serve/triton.py` and `serve/deploy_sagemaker.py` SHALL start with a "NOT PORTED" notice and SHALL have no compose service and no gitops manifest. The offline test suite SHALL NOT import any of the six files. The imported tree and history SHALL contain no secrets.

#### Scenario: History is kept and the local stack is dropped
- **WHEN** `git log --oneline -- platform-finetune` is run after the import
- **THEN** it lists more than one commit from the MLOps history, and `platform-finetune/ops/docker-compose.yml` does not exist
- **VERIFIED BY**: `platform-e2e` `test_pft_import.py::test_history_kept_and_local_stack_dropped`

#### Scenario: Broken serving files are marked and not deployed
- **WHEN** the six serving files and the compose and gitops manifests are inspected
- **THEN** each file's first lines contain `NOT PORTED`, and no compose service or gitops manifest references any of them
- **VERIFIED BY**: `platform-e2e` `test_pft_import.py::test_serving_files_marked_not_ported`

#### Scenario: The offline suite does not import the not-ported files
- **WHEN** `pytest tests/` runs in `platform-finetune` and the imported module names are inspected
- **THEN** none of `datascience.serve.local`, `datascience.package`, `datascience.optimize`, `datascience.serve_analyzer`, `datascience.serve.triton` and `datascience.serve.deploy_sagemaker` is in `sys.modules`
- **VERIFIED BY**: `platform-finetune/tests/test_import_hygiene.py::test_not_ported_files_not_imported`

#### Scenario: Artifacts are not committed
- **WHEN** `git ls-files platform-finetune/artifacts` is run
- **THEN** it prints nothing
- **VERIFIED BY**: `platform-e2e` `test_pft_import.py::test_artifacts_untracked`

#### Scenario: The imported history has no secrets
- **WHEN** detect-secrets scans `platform-finetune` (tree and subtree history)
- **THEN** it reports zero findings
- **VERIFIED BY**: `platform-e2e` `test_pft_import.py::test_detect_secrets_zero_findings`

### Requirement: The finetune policy is declared in the shared policy file

`platform-core/packages/py-modelregistry/policies.yaml` SHALL contain the entry `finetune-classifier: {primary: macro_f1, min_relative_improvement: 0.01, floors: {macro_f1: 0.5}}`. `config/params.yaml` SHALL NOT contain `gate.enabled` or `gate.on_fail`, and `GateFailed` SHALL NOT be raised on the register path. The trainer SHALL resolve the policy file by `policies_path`, then env `MODELREGISTRY_POLICIES`, then the packaged file, and the job image SHALL set `MODELREGISTRY_POLICIES`.

#### Scenario: The entry has the agreed values
- **WHEN** `policies.yaml` is loaded through py-modelregistry
- **THEN** `finetune-classifier` has `primary` `macro_f1`, `min_relative_improvement` 0.01 and `floors` `{macro_f1: 0.5}`
- **VERIFIED BY**: `platform-finetune/tests/test_policy.py::test_finetune_policy_values`

#### Scenario: The policy file loads with every entry
- **WHEN** `policies.yaml` is loaded
- **THEN** it loads without `PolicyError` and contains `recsys-als`, `recsys-gbdt`, `recsys-two-tower` and `finetune-classifier`
- **VERIFIED BY**: `platform-finetune/tests/test_policy.py::test_policies_file_loads_all_entries`

#### Scenario: The gate settings are gone from params.yaml
- **WHEN** `config/params.yaml` is parsed
- **THEN** it has no `gate.enabled` and no `gate.on_fail` key
- **VERIFIED BY**: `platform-finetune/tests/test_policy.py::test_params_has_no_gate_switches`

#### Scenario: A malformed policy path is a config error
- **WHEN** `MODELREGISTRY_POLICIES` points to a file that does not exist and the pipeline reaches `submit`
- **THEN** the process exits 2 with `PolicyError` and no version is created
- **VERIFIED BY**: `platform-finetune/tests/test_cli_exit_codes.py::test_missing_policy_file_exits_2`

### Requirement: A passing candidate becomes champion through the shared gate

After `evaluate`, and in `promote <variant>`, the pipeline SHALL save the model to disk and then call py-modelregistry `submit` with model `finetune-classifier`, the `run_id` of the MLflow run the trainer created, the scalar metrics, the model directory and lineage. WHEN the decision is promotable, alias `champion` SHALL point to the new version and the previous holder SHALL carry alias `previous`. Thresholds are inclusive.

#### Scenario: Candidate passing floor and improvement becomes champion
- **WHEN** a candidate with `macro_f1=0.90` is submitted while champion v1 (`macro_f1=0.80`) was submitted with the same `lineage.eval_protocol`
- **THEN** `finetune-classifier` v2 has alias `champion`, v1 has alias `previous`, v2 carries tag `gate.outcome=promoted`, and the process exits 0
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_pass_moves_champion`

#### Scenario: First candidate at or above the floor becomes the first champion
- **WHEN** a candidate with `macro_f1=0.55` is submitted and the model has no champion
- **THEN** v1 has alias `champion` and tag `gate.outcome=promoted`
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_first_candidate_meets_floor`

#### Scenario: Exactly at the floor passes
- **WHEN** a first candidate with `macro_f1` equal to 0.5 is submitted
- **THEN** it is promoted and becomes `champion`
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_boundary_at_floor_passes`

#### Scenario: One percent better passes per the library's comparison rule
- **WHEN** champion v1 has `macro_f1=0.5` and the same `lineage.eval_protocol`, and a candidate with `macro_f1` equal to `0.5 * 1.01` (computed in the test) is submitted
- **THEN** it is promoted and v2 has alias `champion`
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_boundary_one_percent_passes`

#### Scenario: A candidate that is not promotable never moves the alias
- **WHEN** `apply` is called for a version tagged `gate.outcome=rejected`
- **THEN** it is refused with `GateError` (exit 2) and alias `champion` is unchanged
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_apply_refused_for_rejected_version`

#### Scenario: Both register sites write to the registry at runtime
- **WHEN** `run_evaluate` and `tune.promote.promote` each run against a local MLflow file store with a passing metrics file
- **THEN** each creates a new version of `finetune-classifier` whose source is the run passed by the trainer, and no import of `registry.base`, `registry.local`, `registry.s3`, `registry.stages` or `gate` remains in `src/datascience`
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_register_sites_write_to_registry`

### Requirement: A failing candidate is saved and registered as rejected

IF the candidate's `macro_f1` is below the floor, fails the improvement rule, or is missing or not finite, THEN the system SHALL keep the model on disk, register it as a version tagged `gate.outcome=rejected` with a `gate.reason`, leave aliases `champion` and `previous` unchanged (and create no `champion` alias when none existed), and exit 0. Nothing SHALL raise on the register path.

#### Scenario: Below the floor is rejected and champion is unchanged
- **WHEN** a candidate with `macro_f1=0.40` is submitted while champion v1 exists
- **THEN** v2 has tag `gate.outcome=rejected` and a `gate.reason` naming `macro_f1`, alias `champion` still points to v1, the model directory exists on disk, and the process exits 0
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_below_floor_rejected_champion_unchanged`

#### Scenario: First candidate below the floor with no champion is rejected
- **WHEN** a candidate with `macro_f1=0.40` is submitted and the model has no versions
- **THEN** v1 has tag `gate.outcome=rejected`, the model has no `champion` alias, the model directory exists, and the process exits 0
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_first_candidate_below_floor_no_champion`

#### Scenario: Not improving enough is rejected
- **WHEN** a candidate with `macro_f1` equal to `0.5 * 1.005` (computed in the test) is submitted while champion v1 (`macro_f1=0.5`) has the same `lineage.eval_protocol`
- **THEN** the new version is tagged `gate.outcome=rejected` and alias `champion` still points to v1
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_insufficient_improvement_rejected`

#### Scenario: Missing or NaN macro_f1 is rejected, not raised
- **WHEN** a candidate is submitted with metrics that lack `macro_f1`, and another with `macro_f1=NaN`
- **THEN** each registers a version tagged `gate.outcome=rejected` with a reason naming `macro_f1`, alias `champion` is unchanged, and nothing raises
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_missing_or_nan_primary_rejected`

#### Scenario: A below-floor run through the CLI exits 0
- **WHEN** `python main.py run` finishes `evaluate` with a below-floor candidate against a local file-store registry (training stage stubbed by a pre-built model and metrics file)
- **THEN** the process exits 0, the version is `rejected`, and no `GateFailed` appears in the output
- **VERIFIED BY**: `platform-finetune/tests/test_cli_exit_codes.py::test_below_floor_run_exits_0`

### Requirement: An unreachable registry publishes nothing

IF the registry cannot be reached, THEN `submit` SHALL raise `RegistryUnavailable` within its 12 s deadline, the process SHALL exit 1 within 15 s (the 12 s library deadline plus 3 s process margin), the model SHALL stay on disk, and no alias SHALL change. The system SHALL NOT fall back to a local or in-memory decision. At pipeline start, before ingest, the system SHALL check registry reachability and exit 1 without training when it is down.

#### Scenario: Submit against a closed port fails closed
- **WHEN** `submit` is called with `MLFLOW_TRACKING_URI` set to a closed local port, after the model was saved
- **THEN** `RegistryUnavailable` is raised, the process exits 1 within 15 s, the model directory exists, and the alias state read back from the file store is unchanged
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_registry_unreachable_fails_closed`

#### Scenario: The pre-flight stops the job before training
- **WHEN** `python main.py run` starts with `MLFLOW_TRACKING_URI` set to a closed local port
- **THEN** the process exits 1 within 15 s and no stage (ingest, train) has started
- **VERIFIED BY**: `platform-finetune/tests/test_cli_exit_codes.py::test_preflight_unreachable_exits_1_before_training`

#### Scenario: An unexpected stage exception exits 1
- **WHEN** a pipeline stage raises an unexpected exception in a `main.py run` subprocess
- **THEN** the process exits 1 and no registry alias changes
- **VERIFIED BY**: `platform-finetune/tests/test_cli_exit_codes.py::test_stage_exception_exits_1`

### Requirement: Every version carries lineage

The trainer SHALL create the MLflow run and pass its `run_id` to `submit`, and the version's source SHALL be that run. Every version, promoted or rejected, SHALL carry tags `lineage.data_version` (from `read_data_version()`), `lineage.train_hw` (the `_hw_id` string, or `cpu` on a CPU-only host) and `lineage.eval_protocol`. `lineage.eval_protocol` is the first 12 hex characters of the sha256 of the test-split identity (dataset name, split, and the `data_version` of the eval set) plus the metric config (`metrics.compute` and `metrics.objective`); it SHALL be identical across runs that use the same evaluation and differ when any of those inputs change. Per D2, when the champion's `lineage.eval_protocol` differs from the candidate's or is absent, the candidate is judged on the floor only.

#### Scenario: Lineage tags are present on promoted and rejected versions
- **WHEN** one passing and one failing candidate are submitted from a trainer-created run
- **THEN** both versions have non-empty `lineage.data_version`, `lineage.train_hw` and a 12-character `lineage.eval_protocol`, and each version's source run id equals the trainer's run id
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_lineage_tags_on_every_version`

#### Scenario: The eval protocol id is stable and sensitive
- **WHEN** the identifier is computed twice from the same dataset name, split, eval-set `data_version` and metric config, and then once with a changed `metrics.compute` list
- **THEN** the first two values are equal, the third differs, and each is 12 hex characters
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_eval_protocol_id_stable_and_sensitive`

#### Scenario: A changed eval protocol is judged on the floor only
- **WHEN** a candidate with `macro_f1=0.90` and `lineage.eval_protocol=bbbbbbbbbbbb` is submitted while champion v1 (`macro_f1=0.95`) was submitted with `lineage.eval_protocol=aaaaaaaaaaaa`
- **THEN** the candidate is judged on the floor `macro_f1` 0.5 only and is promoted, its `gate.reason` starts with `eval_protocol changed`, and v1 holds alias `previous`
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_eval_protocol_changed_floor_only`

#### Scenario: A changed eval protocol below the floor is rejected
- **WHEN** a candidate with `macro_f1=0.40` and a different `lineage.eval_protocol` from champion v1 is submitted
- **THEN** it is tagged `gate.outcome=rejected`, its `gate.reason` names `macro_f1`, and alias `champion` still points to v1
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_eval_protocol_changed_below_floor_rejected`

#### Scenario: A CPU-only host records cpu
- **WHEN** a candidate is submitted from a host with no GPU
- **THEN** `lineage.train_hw` is `cpu`
- **VERIFIED BY**: `platform-finetune/tests/test_registry_gate.py::test_train_hw_cpu_fallback`

### Requirement: The training image and compose job run the pipeline against agora infra

`platform-finetune/Dockerfile` SHALL build the training image, and `docker-compose.services.yaml` SHALL define service `platform-finetune` under profile `jobs` with `MLFLOW_TRACKING_URI=http://mlflow:5050` and MinIO credentials from agora infra. The service SHALL NOT start without the `jobs` profile.

#### Scenario: The service is in the jobs profile only
- **WHEN** `docker compose -f docker-compose.services.yaml config --services` is run without profiles, and then with `--profile jobs`
- **THEN** `platform-finetune` is absent in the first output and present in the second, with `MLFLOW_TRACKING_URI` equal to `http://mlflow:5050`
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_compose_service_in_jobs_profile`

#### Scenario: The image builds without a GPU
- **WHEN** `docker build platform-finetune` runs on a host with no GPU
- **THEN** the build exits 0 and the image entrypoint runs `python main.py run`
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_image_builds_cpu_host`

### Requirement: The weekly CronJob trains unattended on the GPU pool

`platform-gitops/platform/finetune/` SHALL define a CronJob with schedule `0 4 * * 0`, `concurrencyPolicy: Forbid`, `backoffLimit: 1`, `restartPolicy: Never`, the ADR-0011 GPU `nodeSelector` and `toleration`, and a `nvidia.com/gpu: 1` limit. An ArgoCD Application SHALL sync it. Its NetworkPolicy SHALL have no ingress and egress only to DNS, `mlflow`, MinIO, and TCP 443 to `0.0.0.0/0` excluding cluster CIDRs (HF dataset and base model). No Service or Ingress SHALL exist for it.

#### Scenario: The manifest is weekly, non-overlapping and bounded
- **WHEN** the CronJob manifest is rendered
- **THEN** `schedule` is `0 4 * * 0`, `concurrencyPolicy` is `Forbid`, `backoffLimit` is 1, `restartPolicy` is `Never`, and `MLFLOW_TRACKING_URI` targets the in-cluster `mlflow` Service
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_cronjob_weekly_forbid`

#### Scenario: The job lands on a GPU node
- **WHEN** the CronJob pod template is rendered
- **THEN** it has the ADR-0011 GPU `nodeSelector` and `toleration`, and the container limit `nvidia.com/gpu` is `1`
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_cronjob_gpu_pool`

#### Scenario: The pod is isolated and unreachable
- **WHEN** the NetworkPolicy and kustomization are rendered
- **THEN** the policy has no ingress rule, egress allows only DNS, `mlflow`, MinIO and TCP 443 to `0.0.0.0/0` with the cluster CIDRs in `except`, and no Service or Ingress exists for the job
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_networkpolicy_egress_and_no_service`

#### Scenario: A rejected run completes and an outage run fails by exit code
- **WHEN** `main.py run` is executed in a subprocess with a rejected candidate, then with an unreachable registry
- **THEN** the exit codes are 0 and 1 respectively, and the champion alias is unchanged in both
- **VERIFIED BY**: `platform-finetune/tests/test_cli_exit_codes.py::test_rejected_exits_0_outage_exits_1`

### Requirement: CI runs the offline tests without a GPU or dataset

`.github/workflows/services-ci.yml` SHALL have a job `platform-finetune` with `timeout-minutes: 10` that installs the CPU dev extras and runs `pytest tests/` in `platform-finetune`. The tests SHALL pass with no GPU, no dataset download and no running MLflow server (a local file store is used).

#### Scenario: The CI job exists and is bounded
- **WHEN** the workflow file is parsed
- **THEN** a job `platform-finetune` runs `pytest` with working directory `platform-finetune`, has `timeout-minutes` 10 and no GPU runner label
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_ci_job_offline`

#### Scenario: The suite passes with the network blocked
- **WHEN** `pytest tests/` runs in `platform-finetune` with network access disabled and no `artifacts/` dataset
- **THEN** all tests pass
- **VERIFIED BY**: `platform-finetune/tests` full run via `make -C platform-finetune test`

#### Scenario: A missing dependency fails the job
- **WHEN** a test imports a module that the CPU dev extras do not install
- **THEN** the CI job fails with a non-zero status instead of skipping the test
- **VERIFIED BY**: `platform-e2e` `test_pft_deploy.py::test_ci_job_offline`
