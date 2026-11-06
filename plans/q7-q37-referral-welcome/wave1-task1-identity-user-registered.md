# [W1-T1] identity-user-registered

## Role
feature-run worker (change `openspec/changes/identity-user-registered`)

## Objective
Implement OpenSpec change `identity-user-registered` test-first per its tasks.md; context in plans/q7-q37-referral-welcome/00-spec.md (Locked + Clarifications C1-C6) and the change's design.md.
- RegisterRequest.referral_code normalised and stored; UserRegistered written to the outbox in the same transaction as the user row; AuthService.GetMyAccount (principal-scoped) routed authenticated-any-role at the gateway.
- depends_on: q7-referral-welcome-contract (merged)

## Write-set (EXCLUSIVE)
- team-identity/internal/**
- team-identity/cmd/**
- team-identity/migrations/**
- team-identity/tests/**
- team-identity/FEATURES.yaml
- team-identity/README.md
- team-gateway/internal/edge/auth.go
- team-gateway/internal/edge/server.go
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/q7_identity_test.go
- team-gateway/README.md
- platform-e2e/tests/e2e/features/identity/register_referral.feature
- platform-e2e/tests/e2e/features/identity/get_my_account.feature
- platform-e2e/tests/e2e/step_definitions/identity_q7_steps.py
- platform-e2e/tests/e2e/step_definitions/test_identity_q7.py

## Verify
- `cd team-identity && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./... && make lint`
- `cd team-gateway && buf generate --template buf.gen.local.yaml && go test ./internal/edge/...`
- e2e: deferred (local stack busy)
