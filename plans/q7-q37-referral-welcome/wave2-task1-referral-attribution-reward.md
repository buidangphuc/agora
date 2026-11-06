# [W2-T1] referral-attribution-reward

## Role
feature-run worker (change `openspec/changes/referral-attribution-reward`)

## Objective
Implement OpenSpec change `referral-attribution-reward` test-first per its tasks.md; context in plans/q7-q37-referral-welcome/00-spec.md (Locked + Clarifications C1-C6) and the change's design.md.
- Consume identity.events UserRegistered and order.events OrderCompleted/ReturnRequested; attribution verdict events via outbox on referral.events; PENDING -> RELEASED | VOID rewards with sweeper and caps; Redeem rewritten without credit.
- depends_on: q7-referral-welcome-contract, identity-user-registered (both on feat/q7-q37)

## Write-set (EXCLUSIVE)
- team-referral/internal/**
- team-referral/cmd/**
- team-referral/migrations/**
- team-referral/.env.example
- team-referral/README.md
- team-referral/readme_test.go
- team-referral/Makefile
- team-referral/go.mod
- team-referral/go.sum
- team-referral/FEATURES.yaml
- docker-compose.services.yaml
- platform-gitops/envs/services/team-referral.yaml
- platform-gitops/envs/staging/services/team-referral.yaml
- platform-gitops/envs/prod/services/team-referral.yaml
- platform-gitops/envs/local/values.yaml
- platform-gitops/argocd/apps/team-referral.yaml
- platform-e2e/tests/e2e/features/referral/referral_reward.feature
- platform-e2e/tests/e2e/step_definitions/referral_reward_steps.py
- platform-e2e/tests/e2e/step_definitions/test_referral_reward.py
- platform-e2e/tests/e2e/support/referral_events_support.py
- platform-e2e/compose/referral-reward.override.yaml

## Verify
- `cd team-referral && buf generate && go vet ./... && go test ./... && make lint`
- e2e: deferred (local stack busy)
