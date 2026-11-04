# [W1-T2] identity-sso-handoff

## Role
feature-run worker (change `openspec/changes/identity-sso-handoff`)

## Objective
Implement OpenSpec change `identity-sso-handoff` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- Single-use 60 s hand-off codes (Create/Peek/Redeem) in team-identity, gateway forwarding (peek/redeem public), fe-platform session startHandoff + confirmation handler, buyer<->seller links and route handlers, access-log redaction, remove G8 from gaps.md.

## Write-set (EXCLUSIVE)
- team-identity/internal/store/**
- team-identity/internal/identity/handoff.go
- team-identity/internal/identity/handoff_test.go
- team-identity/internal/identity/auth.go (widened at build: task 1.2.1)
- team-identity/internal/identity/session_sweeper.go
- team-identity/internal/identity/session_sweeper_test.go
- team-identity/internal/config/**
- team-identity/internal/rpc/**
- team-identity/internal/app/**
- team-identity/README.md
- team-identity/FEATURES.yaml
- team-gateway/internal/edge/auth.go
- team-gateway/internal/edge/auth_*_test.go
- platform-core/packages/go-sdk/pkg/auth/** (widened at build: task 1.3.1)
- team-gateway/internal/edge/forward.go (widened at build: task 1.3.1)
- team-gateway/internal/edge/forward_test.go (widened at build: task 1.3.1)
- team-gateway/internal/edge/forward_sid_test.go (widened at build: task 1.3.1)
- fe-platform/packages/session/src/handoff.ts
- fe-platform/packages/session/src/handoff.test.ts
- fe-platform/packages/session/src/index.ts
- fe-platform/packages/session/src/config.ts
- fe-platform/docs/gaps.md
- team-frontend/src/app/auth/handoff/**
- team-frontend/src/components/shell/ConsumerShell.tsx
- team-frontend/src/components/shell/ConsumerShell.test.tsx
- team-frontend/src/features/handoff/**
- team-frontend/src/middleware.ts
- team-frontend/vendor/fe-platform-session.tgz
- team-frontend/package-lock.json
- team-frontend-seller/src/app/auth/handoff/**
- team-frontend-seller/src/app/seller/layout.tsx
- team-frontend-seller/src/app/seller/layout.test.tsx
- team-frontend-seller/src/features/handoff/**
- team-frontend-seller/src/middleware.ts
- team-frontend-seller/vendor/fe-platform-session.tgz
- team-frontend-seller/package-lock.json
- team-frontend-admin/src/app/auth/handoff/**
- platform-gitops/envs/services/team-frontend.yaml
- platform-gitops/envs/services/team-frontend-seller.yaml
- platform-e2e/tests/e2e/features/auth/sso_handoff.feature
- platform-e2e/tests/e2e/step_definitions/sso_handoff_steps.py
- platform-e2e/tests/e2e/step_definitions/test_sso_handoff.py
- platform-e2e/tests/e2e/support/sso_handoff_support.py
- platform-e2e/src/pages/handoff_confirm_page.py
- openspec/changes/identity-sso-handoff/**
- team-frontend-admin/src/app/handoffAbsent.test.ts (widened at build: task 1.6.1)
- team-frontend-seller/src/app/seller/layout.handoff.test.tsx (widened at build: task 1.6.1)
- platform-gitops/charts/service/templates/ingress.yaml # widened at build: task 1.6.2
- platform-gitops/tests/test_handoff_log_redaction_manifests.py # widened at build: task 1.6.2
- team-frontend-seller/src/access-log.test.ts # widened at build: task 1.6.2
- team-frontend-seller/src/lib/access-log.ts # widened at build: task 1.6.2
- team-frontend/src/access-log.test.ts # widened at build: task 1.6.2
- team-frontend/src/lib/access-log.ts # widened at build: task 1.6.2

## Read-only dependencies
- team-identity login session issuer (pattern)
- generated identity stubs from p10-gaps-contract
- fe-platform/packages/session existing setSession/safeNext
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- (cd team-identity && go test -race ./...) with a throwaway identity_test DB (a skip is not a pass)
- make -C team-identity check
- (cd team-gateway && go test ./... -race)
- (cd fe-platform && npm test -- packages/session)
- (cd team-frontend && npm run vendor:check && npm test)
- (cd team-frontend-seller && npm run vendor:check && npm test)
- bash /Users/phuc.buidang/Library/Caches/ai-first-runs/p10-build/e2e.sh <slice-worktree> platform-e2e/tests/e2e/step_definitions/test_sso_handoff.py
- $PY platform-e2e/scripts/spec_sync.py identity-sso-handoff --strict
