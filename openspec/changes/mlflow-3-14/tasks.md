## 1. Tests first

- [x] 1.1 `test_infra_config.py` pins `mlflow:3.14.0`; `model_registry.feature` expects version `3.14.0`

## 2. Change

- [x] 2.1 Compose `mlflow` on `burakince/mlflow:3.14.0` with `mlflow db upgrade` and `--allowed-hosts`; drop `infra/mlflow/Dockerfile`
- [x] 2.2 platform-gitops `platform/mlflow` and `deploy/images/services.map`
- [x] 2.3 Clients: py-modelregistry, platform-e2e, platform-finetune (pyproject, Dockerfile, CI), lock files

## 3. Verification

- [x] 3.1 A 3.2.0 database upgraded by `mlflow db upgrade` serves its runs on 3.14.0; `GET /version` = `3.14.0`
- [ ] 3.2 `model_registry` e2e on mainx after merge (`agora_env.py promote`)
- [x] 3.3 `openspec validate mlflow-3-14 --strict`, py-modelregistry tests, pre-commit-ci
