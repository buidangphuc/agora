# [W1-T1] recsys-auto-train

## Role
feature-run worker (change `openspec/changes/recsys-auto-train`)

## Objective
Implement OpenSpec change `recsys-auto-train` test-first per its tasks.md; context in plans/training-platform/00-spec.md and run/round2-decisions.md. Uses py-modelregistry from add-model-registry (already merged into the base).

## Write-set (EXCLUSIVE)
- platform-recsys/**
- platform-core/packages/py-modelregistry/src/modelregistry/policies.yaml (packaged policy file)
- platform-gitops/platform/recsys/**
- docker-compose.services.yaml
- .dockerignore (recsys builds from the monorepo root)
- .github/workflows/asdlc-gates.yml (tdd: install uv deps from the owning dir)
- openspec/changes/recsys-auto-train/** (human-approved clarifications 2026-10-13)
- platform-e2e/tests/e2e/features/recommendations/**
- platform-e2e/tests/e2e/step_definitions/** (re-pointed recsys steps off PROMOTION_FORCE)
- platform-e2e/tests/e2e/flows/** (recsys job drivers + registry_support.py)
- platform-e2e/src/api/**

## Read-only dependencies
- openspec/changes/recsys-auto-train/** (frozen by LOCK.md digest)
- platform-core/packages/py-modelregistry/src/** (contract from add-model-registry)
- team-ai/** (must not change)

## Verify
uv venv /tmp/recsys-venv -p 3.10 && uv pip install -p /tmp/recsys-venv/bin/python -r platform-recsys/requirements-dev.txt -e platform-recsys -e platform-core/packages/py-modelregistry && make -C platform-recsys compile lint check-env test-host PY=/tmp/recsys-venv/bin/python; platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py recsys-auto-train --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_recsys_auto_train.py)
