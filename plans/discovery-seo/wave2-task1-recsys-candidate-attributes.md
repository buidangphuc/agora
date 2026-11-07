# [W2-T1] recsys-candidate-attributes

## Role
feature-run worker (change `openspec/changes/recsys-candidate-attributes`)

## Objective
Implement OpenSpec change `recsys-candidate-attributes` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md (C14, C15) and the change's design.md.
- featurestore `item_attributes` v2 carries the optional export `status`; recsys publish writes `seller_id`, `category_id`, `visible` (128-char rule) onto item points and cache entries; `backfill-attributes` command patches the serving and previous generations (Qdrant `set_payload` x256, Redis rewrite keeping TTL).
- depends_on: analytics-listing-status-export (merged; status column + replay)
- base: `feat/discovery-seo` after wave 1 merged
- e2e note: the gateway-entry assertions of `candidate_attributes.feature` need recsys-cart-cross-sell (wave 3); author the feature red here, run it green after wave 3.

## Write-set (EXCLUSIVE)
- platform-featurestore/featurestore/inputs.py
- platform-featurestore/registry/sql/item_attributes_v2.sql
- platform-featurestore/registry/features.yaml
- platform-featurestore/registry/features.lock
- platform-featurestore/tests/test_item_attributes.py
- platform-featurestore/tests/test_materialize.py
- platform-featurestore/README.md
- platform-featurestore/FEATURES.yaml
- platform-recsys/recsys/config.py
- platform-recsys/recsys/load/**
- platform-recsys/recsys/publish.py
- platform-recsys/recsys/pipeline.py
- platform-recsys/recsys/backfill.py
- platform-recsys/recsys/__main__.py
- platform-recsys/tests/test_candidate_attributes.py
- platform-recsys/tests/test_config.py
- platform-recsys/tests/test_env_drift.py
- platform-recsys/tests/test_compose_wiring.py
- platform-recsys/Makefile
- platform-recsys/.env.example
- platform-recsys/README.md
- platform-recsys/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/recsys/candidate_attributes.feature
- platform-e2e/tests/e2e/step_definitions/candidate_attributes_steps.py
- platform-e2e/tests/e2e/step_definitions/test_candidate_attributes.py

## Verify
- `cd platform-featurestore && ruff check . && python -m pytest -q tests/`
- `cd platform-recsys && make compile lint check-env test-host eval` (add `tests/test_candidate_attributes.py` to `test-host`)
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_candidate_attributes.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; green only after recsys-cart-cross-sell)
