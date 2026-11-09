# [W3-T1] recsys-cart-cross-sell

## Role
feature-run worker (change `openspec/changes/recsys-cart-cross-sell`)

## Objective
Implement OpenSpec change `recsys-cart-cross-sell` test-first per its tasks.md; context in plans/discovery-seo/00-spec.md (C3, C14) and the change's design.md.
- team-ai: `context_listing_ids` (cap 50, INVALID_ARGUMENT), `Candidate` gains `seller_id`/`category_id`/`visible` (missing visible fails closed), `cart_cross_similarity`, placement `Filters` (in_stock, dedupe sellers, exclude seed/cart, drop non-visible), `category_popular`, strict placement keys; gateway rejects >50 and forwards; team-frontend cart cross-sell row.
- depends_on: discovery-seo-contract, recsys-candidate-attributes (merged)
- base: `feat/discovery-seo` after wave 2 merged

## Write-set (EXCLUSIVE)
- team-ai/app/modules/business/recommendation/**
- team-ai/app/transport/grpc/servicers/recommendation.py
- team-ai/tests/unit/modules/recommendation/**
- team-ai/tests/unit/transport/test_recommendation_servicer.py
- team-ai/README.md
- team-ai/FEATURES.yaml
- team-gateway/internal/edge/recommendation.go
- team-gateway/internal/edge/recommendation_test.go
- team-gateway/README.md
- team-gateway/FEATURES.yaml
- team-frontend/src/app/(shop)/cart/page.tsx
- team-frontend/src/app/(shop)/cart/crossSell.test.tsx
- team-frontend/src/features/cart/CrossSellRow.tsx
- team-frontend/src/features/recommendations/**
- team-frontend/src/lib/gateway/recommendations.ts
- team-frontend/src/lib/gateway/recommendations.test.ts
- team-frontend/FEATURES.yaml
- platform-e2e/tests/e2e/features/recsys/cart_cross_sell.feature
- platform-e2e/tests/e2e/features/frontend/cart_cross_sell_ui.feature
- platform-e2e/tests/e2e/step_definitions/cart_cross_sell_steps.py
- platform-e2e/tests/e2e/step_definitions/test_cart_cross_sell.py
- platform-e2e/tests/e2e/step_definitions/test_cart_cross_sell_ui.py
- platform-e2e/src/pages/cart_page.py
- platform-e2e/src/pages/components/recommendations_row_component.py

## Verify
- `cd team-ai && uv sync --dev --all-extras && uv run --all-extras python -m scripts.gen_proto && uv run --all-extras pytest -q && uv run --all-extras python -m scripts.run_eval --min-score 0.8`
- `cd team-gateway && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./...`
- `cd team-frontend && npm run proto && npm run check`
- e2e: `cd platform-e2e && HEADLESS=true $PY -m pytest tests/e2e/step_definitions/test_cart_cross_sell.py tests/e2e/step_definitions/test_cart_cross_sell_ui.py tests/e2e/step_definitions/test_candidate_attributes.py -q` (PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; candidate_attributes runs green here, see W2-T1)
