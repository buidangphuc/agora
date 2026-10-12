# [W2-T2] recsys-consent-and-context

## Role
feature-run worker (change `openspec/changes/recsys-consent-and-context`)

## Objective
Implement OpenSpec change `recsys-consent-and-context` test-first per its tasks.md (scenario ids there); context in plans/ga4-analytics/00-spec.md.

## Write-set (EXCLUSIVE)
- team-analytics/internal/export/export.go
- team-analytics/internal/export/export_test.go
- platform-featurestore/featurestore/inputs.py
- platform-featurestore/featurestore/dataset.py
- platform-featurestore/registry/sql/als_interactions_v2.sql
- platform-featurestore/registry/sql/rank_training_v2.sql
- platform-featurestore/registry/features.yaml
- platform-featurestore/registry/features.lock
- platform-featurestore/tests/test_consent_filter.py (new; may also touch existing test_dataset.py/test_rank_dataset.py)
- platform-featurestore/FEATURES.yaml
- platform-recsys/recsys/nearline/wire.py
- platform-recsys/recsys/nearline/signals.py
- platform-recsys/recsys/nearline/consumer.py
- platform-recsys/recsys/ranker/training.py
- platform-recsys/recsys/ranker/features.py
- platform-recsys/recsys/ranker/model.py
- platform-recsys/recsys/ranker/**
- platform-recsys/recsys/evals/evaluator.py
- platform-recsys/recsys/pipeline.py
- platform-recsys/recsys/recommend.py
- platform-recsys/tests/test_nearline*.py
- platform-recsys/tests/test_ranker*.py
- platform-recsys/tests/test_evals.py
- platform-recsys/tests/test_recommend.py
- platform-recsys/tests/test_pipeline_*.py
- platform-recsys/FEATURES.yaml
- platform-recsys/Makefile
- platform-e2e/tests/e2e/features/recommendations/recsys_consent_context.feature
- platform-e2e/tests/e2e/step_definitions/recsys_consent_context_steps.py
- platform-e2e/tests/e2e/step_definitions/test_recsys_consent_context.py
- openspec/changes/recsys-consent-and-context/**

## Read-only dependencies
- openspec/changes/recsys-consent-and-context/** (frozen by LOCK.md digest)
- plans/ga4-analytics/00-spec.md, plans/ga4-analytics/slices.yaml

## Verify
(cd team-analytics && go vet ./... && go test ./internal/export/...); (cd platform-featurestore && uv run --frozen --all-extras pytest -q); uv venv <scratch> && uv pip install -p <scratch> -r platform-recsys/requirements-dev.txt -e platform-recsys && make -C platform-recsys compile lint check-env test-host PY=<scratch>/bin/python; platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py recsys-consent-and-context --strict; (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_recsys_consent_context.py)
