# [W1-T1] promotion-voucher-scope

## Role
feature-run worker (change `openspec/changes/promotion-voucher-scope`)

## Objective
Implement OpenSpec change `promotion-voucher-scope` test-first per its tasks.md (scenario ids there); context in plans/q11-checkout-coherence/00-spec.md and the change's design.md Clarifications.
- `ValidateAndReserve` echoes the voucher's `scope` and `seller_id` on valid, rejected and replay paths; no authz change; buyer preview namespace keeps working.
- depends_on: q11-checkout-contract

## Write-set (EXCLUSIVE)
- team-promotion/proto/**
- team-promotion/generated/**
- team-promotion/internal/service/voucher.go
- team-promotion/internal/service/voucher_test.go
- team-promotion/internal/handler/voucher.go
- team-promotion/internal/handler/authz_test.go
- team-promotion/internal/handler/voucher_scope_test.go
- team-promotion/internal/handler/errors_internal_test.go
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- platform-e2e/tests/e2e/support/voucher_scope_support.py
- platform-e2e/tests/e2e/features/promo/voucher_scope.feature
- platform-e2e/tests/e2e/step_definitions/voucher_scope_steps.py
- platform-e2e/tests/e2e/step_definitions/test_voucher_scope.py
- openspec/changes/promotion-voucher-scope/**

## Read-only dependencies
- openspec/changes/promotion-voucher-scope/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/flash_hold_support.py (grpcurl pattern), step_definitions/promo_steps.py (seller/admin seeding), support/voucher_seed.py (ensure_known_voucher for SAVE10)
- plans/q11-checkout-coherence/00-spec.md, slices.yaml, LOCK.md

## Verify
cmp platform-core/packages/proto/platform/promotion/v1/promotion.proto team-promotion/proto/platform/promotion/v1/promotion.proto; bash $BUILD/gen.sh $WT team-promotion; make -C team-promotion check; (cd team-promotion && go test ./... -race); bash $BUILD/e2e.sh $WT tests/e2e/step_definitions/test_voucher_scope.py; $PY platform-e2e/scripts/spec_sync.py promotion-voucher-scope --strict

Vars: PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; BUILD=/Users/phuc.buidang/Library/Caches/ai-first-runs/c4q11-build; WT=<slice worktree>. Run every test/gate via scripts/asdlc/gate_digest.py (gate-digest skill).
