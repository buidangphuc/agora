# [W2-T1] add-platform-finetune

## Role
feature-run worker (change `openspec/changes/add-platform-finetune`)

## Objective
Implement OpenSpec change `add-platform-finetune` test-first per its tasks.md; context in plans/training-platform/00-spec.md and run/round2-decisions.md. Imports /Users/phuc.buidang/Documents/work/MLOps via git subtree (operator step, task 1.1).

## Write-set (EXCLUSIVE)
- platform-finetune/**
- platform-core/packages/py-modelregistry/src/modelregistry/policies.yaml (packaged policy file)
- platform-gitops/platform/finetune/**
- platform-gitops/argocd/apps/finetune.yaml
- platform-gitops/platform/vault-config/vault-config.yaml (vault role platform-finetune)
- docker-compose.services.yaml
- .github/workflows/services-ci.yml
- platform-e2e/tests/e2e/static/** (static import/deploy checks)

## Read-only dependencies
- openspec/changes/add-platform-finetune/** (frozen by LOCK.md digest)
- platform-core/packages/py-modelregistry/src/**

## Verify
(cd platform-finetune && uv run --extra dev pytest -q); platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py add-platform-finetune --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_finetune.py)
