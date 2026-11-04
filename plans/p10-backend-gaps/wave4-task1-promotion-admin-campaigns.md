# [W4-T1] promotion-admin-campaigns

## Role
feature-run worker (change `openspec/changes/promotion-admin-campaigns`)

## Objective
Implement OpenSpec change `promotion-admin-campaigns` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- Admin campaign list/filter/paging, cancel and edit-if-scheduled with audit-first, idempotent admin create (key + payload hash), LISTING_UNAVAILABLE validation via ListingService.GetListing; gateway admin exposure; admin campaign screens; remove G12-G14 from gaps.md.

## Write-set (EXCLUSIVE)
- team-promotion/migrations/*campaign*
- team-promotion/migrations/*idempoten*
- team-promotion/internal/store/**
- team-promotion/internal/repository/**
- team-promotion/internal/service/flashsale*.go
- team-promotion/internal/flashsale/**
- team-promotion/internal/rpc/flashsale*.go
- team-promotion/internal/rpc/campaign*.go
- team-promotion/internal/rpc/common.go
- team-promotion/internal/rpc/common_test.go
- team-promotion/internal/bootstrap/**
- team-promotion/cmd/server/main.go
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/promotion*_test.go
- team-frontend-admin/src/features/campaigns/**
- team-frontend-admin/src/app/admin/campaigns/**
- team-frontend-admin/src/lib/gateway/campaigns.ts
- team-frontend-admin/src/lib/gateway/campaigns.test.ts
- team-frontend-admin/src/lib/gateway/errors.ts
- fe-platform/docs/gaps.md
- platform-e2e/tests/e2e/features/promo/promotion_admin_campaigns.feature
- platform-e2e/tests/e2e/step_definitions/promotion_admin_campaigns_steps.py
- platform-e2e/tests/e2e/step_definitions/test_promotion_admin_campaigns.py
- platform-e2e/tests/e2e/support/admin_campaigns_support.py
- platform-e2e/src/pages/admin_campaigns_page.py
- openspec/changes/promotion-admin-campaigns/**
- team-promotion/internal/listing/owner.go (widened at build: task 5)

## Read-only dependencies
- promotion-admin-vouchers shared admin error mapping
- platform-e2e/src/core/base_page.py wait_streamed
- generated promotion and listing stubs
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- make -C team-promotion check
- (cd team-promotion && go test ./... -race) with PROMOTION_TEST_DATABASE_URL set to a throwaway DB (a skip is not a pass)
- (cd team-promotion && go test -run xxx -bench BenchmarkListCampaignsPage50 ./...) informational
- (cd team-gateway && go test ./... -race)
- (cd team-frontend-admin && npm test && npm run lint && npx tsc --noEmit)
- bash /Users/phuc.buidang/Library/Caches/ai-first-runs/p10-build/e2e.sh <slice-worktree> platform-e2e/tests/e2e/step_definitions/test_promotion_admin_campaigns.py
- $PY platform-e2e/scripts/spec_sync.py promotion-admin-campaigns --strict
