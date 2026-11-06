# [W3-T1] promotion-new-user-vouchers

## Role
feature-run worker (change `openspec/changes/promotion-new-user-vouchers`)

## Objective
Implement OpenSpec change `promotion-new-user-vouchers` test-first per its tasks.md; context in plans/q7-q37-referral-welcome/00-spec.md (Locked + C1-C6) and the change's design.md.
- Welcome / invited NEW_USER voucher issued from identity.events + referral.events (verdict wait, 30-minute fallback), user-bound vouchers with per_user_limit and new_user_only in ValidateAndReserve, hold expires_at, ListMyVouchers routed at the gateway, HasCompletedOrder client to team-order.
- depends_on: contract, identity-user-registered, referral-attribution-reward, order-has-completed-order (all on feat/q7-q37, rebased on main incl. PR #97)

## Write-set (EXCLUSIVE)
- team-promotion/internal/**
- team-promotion/cmd/**
- team-promotion/README.md
- team-promotion/go.mod
- team-promotion/go.sum
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/q7_promotion_test.go
- team-gateway/README.md
- docker-compose.services.yaml
- platform-gitops/envs/services/team-promotion.yaml
- platform-gitops/argocd/apps/team-promotion.yaml
- platform-gitops/envs/services/team-order.yaml
- platform-gitops/argocd/apps/team-order.yaml
- platform-e2e/tests/e2e/features/promotion/**
- platform-e2e/tests/e2e/step_definitions/new_user_voucher_steps.py
- platform-e2e/tests/e2e/step_definitions/test_new_user_vouchers.py
- platform-e2e/tests/e2e/support/new_user_voucher_support.py
- platform-e2e/compose/new-user-voucher.override.yaml

## Verify
- `cd team-promotion && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./... && make lint` (PG/Redpanda via testcontainers on the OrbStack socket)
- `cd team-gateway && go test ./internal/edge/...`
- `python -m unittest discover -s platform-gitops/tests`
- e2e: deferred
