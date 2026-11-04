# [W2-T1] identity-seller-onboarding

## Role
feature-run worker (change `openspec/changes/identity-seller-onboarding`)

## Objective
Implement OpenSpec change `identity-seller-onboarding` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- AuthService.BecomeSeller: ACTIVE buyer accepts a configured seller terms version, role granted atomically with one UserRoleGranted outbox event, fresh access token on the same sid; gateway routes it for user principals only.

## Write-set (EXCLUSIVE)
- team-identity/internal/store/**
- team-identity/internal/identity/auth.go
- team-identity/internal/identity/auth*_test.go
- team-identity/internal/identity/seller*.go
- team-identity/internal/config/**
- team-identity/internal/events/**
- team-identity/internal/rpc/**
- team-identity/internal/app/**
- team-identity/README.md
- team-identity/FEATURES.yaml
- team-gateway/internal/edge/auth.go
- team-gateway/internal/edge/auth_*_test.go
- platform-e2e/tests/e2e/features/identity/seller_onboarding.feature
- platform-e2e/tests/e2e/step_definitions/seller_onboarding_steps.py
- platform-e2e/tests/e2e/step_definitions/test_seller_onboarding.py
- platform-e2e/tests/e2e/support/seller_onboarding_support.py
- openspec/changes/identity-seller-onboarding/**
- team-identity/.env.example (widened at build: task 1.1)
- team-identity/internal/identity/become_seller.go (widened at build: task 1.3)
- team-identity/internal/identity/become_seller_test.go (widened at build: task 1.3)
- team-identity/internal/identity/become_seller.go (widened at build: task 1.4)
- team-identity/cmd/server/main_test.go (widened at build: task 1.5)

## Read-only dependencies
- team-identity ScopesForUser and session token signer
- platform-e2e/tests/e2e/support/c2_identity_events.py (Kafka consumer pattern)
- generated identity stubs from p10-gaps-contract
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- (cd team-identity && go test -race ./... && go vet ./...) with a throwaway identity_test DB (a skip is not a pass)
- make -C team-identity check
- (cd team-gateway && go test ./... -race && go vet ./...)
- make -C team-gateway check
- bash /Users/phuc.buidang/Library/Caches/ai-first-runs/p10-build/e2e.sh <slice-worktree> platform-e2e/tests/e2e/step_definitions/test_seller_onboarding.py
- openspec validate identity-seller-onboarding --strict
- $PY platform-e2e/scripts/spec_sync.py identity-seller-onboarding --strict
