# [W0-T1] add-model-registry

## Role
feature-run worker (change `openspec/changes/add-model-registry`)

## Objective
Implement OpenSpec change `add-model-registry` test-first per its tasks.md; context in plans/training-platform/00-spec.md and run/round2-decisions.md.

## Write-set (EXCLUSIVE)
- platform-core/docs/ADR/0017-unified-model-registry-and-publish-gate.md
- platform-core/docs/ADR/0014-model-registry-promotion.md
- platform-core/docs/ADR/0013-seller-demand-forecasting.md
- platform-core/infra/docker-compose.yaml
- platform-core/infra/mlflow/**
- platform-core/packages/py-modelregistry/**
- platform-core/tests/**
- platform-core/FEATURES.yaml
- platform-gitops/platform/mlflow/**
- platform-gitops/argocd/apps/mlflow.yaml
- platform-gitops/platform/infra/minio.yaml (mlflow-artifacts bucket)
- platform-gitops/platform/vault-config/vault-config.yaml (mlflow paths + role)
- platform-gitops/tests/test_mlflow_manifests.py
- .github/workflows/services-ci.yml
- platform-e2e/tests/e2e/features/model_registry/**
- platform-e2e/tests/e2e/step_definitions/model_registry_steps.py
- platform-e2e/tests/e2e/step_definitions/test_model_registry.py
- platform-e2e/pyproject.toml (add mlflow-skinny, boto3, pyyaml deps)
- platform-e2e/uv.lock

## Read-only dependencies
- openspec/changes/add-model-registry/** (frozen by LOCK.md digest)
- plans/training-platform/00-spec.md, plans/training-platform/run/round2-decisions.md

## Verify
(cd platform-core/packages/py-modelregistry && python -m pip install -e '.[dev]' -q && ruff check . && mypy src && pytest -q); pytest -q platform-core/tests platform-gitops/tests/test_mlflow_manifests.py; platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py add-model-registry --strict; (docker compose -p platform-core -f platform-core/infra/docker-compose.yaml up -d postgres-mlflow minio minio-init mlflow && cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_model_registry.py)
