## Why

The local stack ran two MLflow servers of different versions: agora's `mlflow` service on a self-built
`platform-core/mlflow:3.2.0` (official `ghcr.io/mlflow/mlflow:v3.2.0` plus psycopg2 and boto3), and
`~/Documents/localinfra` on `burakince/mlflow:3.14.0`, the image prod `mlflow.propertyguru.vn` runs. One version
and one image keep the registry behaviour the same everywhere and drop a 1.2 GB local build.

## What Changes

- The infra compose `mlflow` service uses `burakince/mlflow:3.14.0` (ships psycopg2 and boto3); the
  `platform-core/infra/mlflow/Dockerfile` build is removed. The server runs `mlflow db upgrade` on its backend
  store before `mlflow server`, so an existing 3.2.0 database migrates in place, and passes `--allowed-hosts` for
  the host names clients use (`localhost`, `127.0.0.1`, `mlflow`).
- platform-gitops `platform/mlflow`: same upgrade step and allowed hosts; `deploy/images/services.map` retags
  `burakince/mlflow:3.14.0` as `platform-mlflow`.
- Clients move with the server: `py-modelregistry` `mlflow-skinny>=3.14,<3.15`, platform-e2e `mlflow-skinny==3.14.*`,
  platform-finetune and its CI/Dockerfile `mlflow>=3.14,<3.15`.
- ADR-0017 records the new server version.

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `model-registry`: "MLflow registry runs on the infra stack" pins version 3.14.0 instead of 3.2.0.

## Impact

- `platform-core/infra/docker-compose.yaml`, `platform-core/infra/mlflow/Dockerfile` (deleted),
  `platform-core/packages/py-modelregistry/{pyproject.toml,README.md,tests/test_infra_config.py}`,
  `platform-gitops/platform/mlflow/mlflow.yaml`, `deploy/images/services.map`,
  `platform-e2e/{pyproject.toml,uv.lock,tests/e2e/features/model_registry/model_registry.feature}`,
  `platform-recsys/uv.lock`, `platform-finetune/{pyproject.toml,Dockerfile}`, `.github/workflows/services-ci.yml`,
  `docs/agents/project.md`, `platform-core/README.md`, ADR-0017.
- Data: the `pg_mlflow_data` schema is upgraded on first start; there is no downgrade back to 3.2.0.

## Non-goals

Moving localinfra and agora onto one shared MLflow instance; changing the backend store or artifact layout.

## Non-functional requirements

- `GET /version` returns `3.14.0`; existing runs, registered models and artifacts stay readable after the upgrade.
- Server start adds only the `mlflow db upgrade` step (a no-op on an up-to-date schema).

## Data classification

No new data is collected, stored, logged or sent. Existing registry metadata is migrated in place.

## Threat model

- Assets: model registry metadata (postgres-mlflow) and model artifacts (MinIO `mlflow-artifacts`).
- Abuse cases: (1) DNS rebinding against the unauthenticated local server -> `--allowed-hosts` limits the accepted
  Host headers to the in-stack and loopback names; (2) a third-party image replaces the official one -> pinned tag
  `burakince/mlflow:3.14.0`, the same image prod already runs.
- Authz rule: unchanged; the registry has no ingress and is reached in-cluster or on 127.0.0.1 only.
