# [W2-T2] featurestore-hidden-review-exclusion

## Role
feature-run worker (change `openspec/changes/featurestore-hidden-review-exclusion`)

## Objective
Implement OpenSpec change `featurestore-hidden-review-exclusion` test-first per its tasks.md (scenario ids there); context in plans/privacy-audit-hardening/00-spec.md and LOCK.md "Open issues" decisions.
- platform-featurestore adds item_popularity@v2 and als_interactions@v3 that exclude reviews hidden by AS_OF (review_id anti-join); facts gain a nullable review_id; locked versions untouched.
- depends_on: analytics-review-hidden

## Write-set (EXCLUSIVE)
- platform-featurestore/featurestore/inputs.py
- platform-featurestore/registry/sql/item_popularity_v2.sql
- platform-featurestore/registry/sql/als_interactions_v3.sql
- platform-featurestore/registry/features.yaml
- platform-featurestore/registry/features.lock
- platform-featurestore/tests/test_hidden_review_exclusion.py
- platform-featurestore/tests/conftest.py
- platform-featurestore/README.md
- platform-featurestore/FEATURES.yaml
- openspec/changes/featurestore-hidden-review-exclusion/**
- platform-featurestore/tests/test_materialize.py (# widened at build: task 4)
- plans/privacy-audit-hardening/wave2-task2-featurestore-hidden-review-exclusion.md (# widened at build: task 4; packet widening record)

## Read-only dependencies
- openspec/changes/featurestore-hidden-review-exclusion/** is frozen by the LOCK.md digest (only task ticks change)
- platform-featurestore/registry/sql/item_popularity_v1.sql, als_interactions_v2.sql (locked; never edit)
- plans/privacy-audit-hardening/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd platform-featurestore && uv sync --all-extras && uv run ruff check . && uv run pytest -q tests/); (cd platform-featurestore && uv run python -m featurestore lock) then git diff platform-featurestore/registry/features.lock adds only item_popularity@v2 and als_interactions@v3; $PY platform-e2e/scripts/spec_sync.py featurestore-hidden-review-exclusion --strict  (e2e: none, FEATURES not-testable naming tests/test_hidden_review_exclusion.py)
