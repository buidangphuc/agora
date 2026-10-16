# [W0-T1] q5-flash-stock-contract

## Role
feature-run worker (change `openspec/changes/q5-flash-stock-contract`)

## Objective
Implement OpenSpec change `q5-flash-stock-contract` test-first per its tasks.md (scenario ids there); context in plans/q5-advertised-price/00-spec.md and the change's design.md Clarifications (Rounds 1-2).
- Additive FlashSaleService RPCs ReserveFlashSaleStock / CommitFlashSaleStock / ReleaseFlashSaleStock with documented semantics; vendor promotion.proto byte-identical into five repos (team-order's stale copy replaced).

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/promotion/v1/promotion.proto
- platform-core/packages/proto/platform/promotion/v1/flashstock_compat_test.go
- platform-core/packages/proto/platform/promotion/v1/*_test.go
- team-promotion/proto/platform/promotion/v1/promotion.proto
- team-order/proto/platform/promotion/v1/promotion.proto
- team-gateway/proto/platform/promotion/v1/promotion.proto
- team-analytics/proto/platform/promotion/v1/promotion.proto
- team-frontend/proto/platform/promotion/v1/promotion.proto
- platform-core/FEATURES.yaml

## Read-only dependencies
- openspec/changes/q5-flash-stock-contract/** (frozen by LOCK.md digest)
- platform-core/packages/proto/platform/analytics/v1/*_compat_test.go (pattern to mirror)
- plans/q5-advertised-price/00-spec.md, slices.yaml, LOCK.md

## Verify
make -C platform-core lint-proto breaking; (cd platform-core/packages/proto && buf lint && buf build && go test ./platform/promotion/v1/...); for r in team-promotion team-order team-gateway team-analytics team-frontend; do cmp platform-core/packages/proto/platform/promotion/v1/promotion.proto $r/proto/platform/promotion/v1/promotion.proto || exit 1; done; $PY platform-e2e/scripts/spec_sync.py q5-flash-stock-contract --strict
