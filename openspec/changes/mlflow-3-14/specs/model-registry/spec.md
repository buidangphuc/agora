## MODIFIED Requirements

### Requirement: MLflow registry runs on the infra stack

The infra compose stack SHALL run an MLflow server of version 3.14.0 reachable at `http://mlflow:5050` inside the stack and on host port 5050. Its backend store SHALL be a dedicated `postgres-mlflow` container, not the shared `postgres`. Its artifact root SHALL be `s3://mlflow-artifacts` on the existing MinIO. The `minio-init` job SHALL create the bucket `mlflow-artifacts` idempotently.

#### Scenario: Server is up with the pinned version
- **WHEN** the infra stack is started and `GET http://localhost:5050/version` is called
- **THEN** the response status is 200 and the body is `3.14.0`
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
