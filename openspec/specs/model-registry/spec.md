# model-registry Specification

## Purpose
TBD - created by archiving change add-model-registry. Update Purpose after archive.

## Requirements

### Requirement: MLflow registry runs on the infra stack

The infra compose stack SHALL run an MLflow server of version 3.2.0 reachable at `http://mlflow:5050` inside the stack and on host port 5050. Its backend store SHALL be a dedicated `postgres-mlflow` container, not the shared `postgres`. Its artifact root SHALL be `s3://mlflow-artifacts` on the existing MinIO. The `minio-init` job SHALL create the bucket `mlflow-artifacts` idempotently.

#### Scenario: Server is up with the pinned version
- **WHEN** the infra stack is started and `GET http://localhost:5050/version` is called
- **THEN** the response status is 200 and the body is `3.2.0`
- **VERIFIED BY**: e2e `platform-e2e` feature `model_registry` scenario `mlflow_server_up`

#### Scenario: A run and a registered model version are stored with artifacts in MinIO
- **WHEN** a client logs one run with a model artifact and creates a version of registered model `e2e-registry-probe` against `http://localhost:5050`
- **THEN** the version exists in the registry and the artifact object exists in bucket `mlflow-artifacts`
- **VERIFIED BY**: e2e `platform-e2e` feature `model_registry` scenario `run_and_artifact_stored`

#### Scenario: Bucket creation is idempotent
- **WHEN** `minio-init` runs a second time on a stack where `mlflow-artifacts` already exists
- **THEN** the job exits with status 0 and the bucket and its objects are unchanged
- **VERIFIED BY**: e2e `platform-e2e` feature `model_registry` scenario `bucket_init_idempotent`

#### Scenario: Backend store is not the shared postgres
- **WHEN** the rendered infra compose config is inspected
- **THEN** the `mlflow` backend store URI points at host `postgres-mlflow` and no `mlflow` database exists in the shared `postgres` init script
- **VERIFIED BY**: `platform-core/packages/py-modelregistry/tests/test_infra_config.py::test_backend_store_is_dedicated`

### Requirement: Registry is deployed cluster-internal through gitops

The gitops repo SHALL contain `platform/mlflow` manifests and an ArgoCD application `mlflow`. The manifests SHALL define a ClusterIP Service on port 5050, a `postgres-mlflow` backend, and SHALL define no Ingress. Database and S3 credentials SHALL come from Vault references, never as literals in a manifest.

#### Scenario: No ingress is rendered
- **WHEN** `kustomize build platform-gitops/platform/mlflow` is rendered
- **THEN** the output contains zero resources of kind `Ingress` and exactly one Service of type ClusterIP exposing port 5050
- **VERIFIED BY**: `platform-gitops/tests/test_mlflow_manifests.py::test_no_ingress_clusterip_only`

#### Scenario: Manifests without a literal secret
- **WHEN** the rendered manifests are scanned for `password`, `secret` and `access_key` values
- **THEN** every such value is a reference to a Secret or ExternalSecret and a literal credential string fails the test
- **VERIFIED BY**: `platform-gitops/tests/test_mlflow_manifests.py::test_credentials_from_vault_refs`

#### Scenario: ArgoCD application points at the manifests
- **WHEN** `platform-gitops/argocd/apps/mlflow.yaml` is parsed
- **THEN** its source path is `platform/mlflow` and it defines a destination namespace
- **VERIFIED BY**: `platform-gitops/tests/test_mlflow_manifests.py::test_argocd_app_source_path`

### Requirement: ADR-0017 records the decision

`platform-core/docs/ADR/0017-unified-model-registry-and-publish-gate.md` SHALL exist with Status, Context, Decision, Alternatives and Consequences, SHALL state that it supersedes ADR-0014 and the MLflow rejection in ADR-0013, and SHALL record the consumer rule that MLflow must not store person identifiers.

#### Scenario: Supersession is visible from the old ADRs
- **WHEN** ADR-0014 and ADR-0013 are read
- **THEN** each carries a status line naming ADR-0017 as superseding it (ADR-0013 for its MLflow alternative only)
- **VERIFIED BY**: `platform-core/packages/py-modelregistry/tests/test_adr.py::test_supersession_lines`

#### Scenario: Consumer privacy rule is recorded
- **WHEN** ADR-0017 is read
- **THEN** it contains a statement that MLflow runs, tags and artifacts must not contain person identifiers, interaction rows or per-person lists
- **VERIFIED BY**: `tests/test_adr.py::test_privacy_rule_present`

### Requirement: CI runs the library checks

`.github/workflows/services-ci.yml` SHALL contain a job `py-modelregistry` that runs lint, type check and the offline pytest suite of `platform-core/packages/py-modelregistry`, and SHALL trigger on changes under that path.

#### Scenario: Job triggers on library changes
- **WHEN** a pull request changes a file under `platform-core/packages/py-modelregistry/**`
- **THEN** the `py-modelregistry` job is scheduled by the workflow path filter
- **VERIFIED BY**: `tests/test_ci_workflow.py::test_job_and_path_filter_present`

#### Scenario: A failing test fails the job
- **WHEN** one library test fails
- **THEN** the job exits non-zero and the workflow run is red
- **VERIFIED BY**: `tests/test_ci_workflow.py::test_job_runs_pytest_without_continue_on_error`

### Requirement: ADR-0017 records the concurrency and state model

ADR-0017 SHALL state the version states (`pending`, `promotable`, `promoted`, `rejected`), that a crash between `evaluate` and `apply` leaves aliases unchanged, and that the library assumes a single writer per model, guaranteed by `concurrencyPolicy: Forbid` on the trainer CronJobs, without a lock.

#### Scenario: Single-writer assumption is written down
- **WHEN** ADR-0017 is read
- **THEN** it names `concurrencyPolicy: Forbid`, the four version states and the statement that no lock is taken
- **VERIFIED BY**: `tests/test_adr.py::test_single_writer_and_states_recorded`

#### Scenario: ADR without the state model fails the check
- **WHEN** the ADR text lacks `concurrencyPolicy: Forbid` or any of the four state names
- **THEN** the check fails and names the missing term
- **VERIFIED BY**: `tests/test_adr.py::test_adr_rejects_missing_sections`
