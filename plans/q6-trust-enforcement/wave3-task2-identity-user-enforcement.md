# [W3-T2] identity-user-enforcement

## Role
feature-run worker (change `openspec/changes/identity-user-enforcement`)

## Objective
Implement OpenSpec change `identity-user-enforcement` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- team-identity user status (active/restricted/suspended/banned): AccountAdminService ListUsers/SetUserStatus (audit-first, session revoke, UserStatusChanged), login/refresh gate, restricted drops listing.write, lift of expired suspension at login; gateway admin routes.
- depends_on: q6-trust-contract; serialised after (write-set overlap): domain-listing-moderation, engagement-listing-reports

## Write-set (EXCLUSIVE)
- team-identity/migrations/0007_user_status.up.sql
- team-identity/migrations/0007_user_status.down.sql
- team-identity/internal/repository/users.go
- team-identity/internal/repository/users_pg.go
- team-identity/internal/repository/sessions_pg.go
- team-identity/internal/repository/**
- team-identity/internal/events/publisher.go
- team-identity/internal/events/*_test.go
- team-identity/internal/upstream/audit.go
- team-identity/internal/upstream/audit_test.go
- team-identity/internal/config/config.go
- team-identity/internal/config/*_test.go
- team-identity/internal/bootstrap/lifecycle.go
- team-identity/internal/bootstrap/resources.go
- team-identity/cmd/server/**
- team-identity/internal/authz/scopes.go
- team-identity/internal/authz/*_test.go
- team-identity/internal/service/auth.go
- team-identity/internal/service/enforcement.go
- team-identity/internal/service/refresh.go
- team-identity/internal/service/*_test.go
- team-identity/internal/handler/accountadmin.go
- team-identity/internal/handler/accountadmin_test.go
- team-identity/internal/handler/auth.go
- team-identity/internal/handler/*_test.go
- team-identity/internal/grpcserver/server.go
- team-identity/generated/**
- team-identity/.env.example
- team-identity/README.md
- team-identity/FEATURES.yaml
- team-gateway/internal/edge/accountadmin.go
- team-gateway/internal/edge/accountadmin_test.go
- team-gateway/internal/edge/server.go
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/policy_edge_test.go
- team-gateway/internal/edge/unrouted_test.go
- team-gateway/generated/**
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/support/user_enforcement_support.py
- platform-e2e/tests/e2e/features/admin/user_enforcement.feature
- platform-e2e/tests/e2e/step_definitions/test_user_enforcement.py
- openspec/changes/identity-user-enforcement/**

## Read-only dependencies
- openspec/changes/identity-user-enforcement/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/audit_support.py (reuse, do not edit)
- team-identity/proto/platform/identity/v1/identity.proto (copied by q6-trust-contract)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-identity && make proto && go vet ./... && go test ./... -race); (cd team-gateway && go vet ./... && go test ./internal/edge/... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_user_enforcement.py -m 'not destructive') then the @destructive audit-down scenario alone; $PY platform-e2e/scripts/spec_sync.py identity-user-enforcement --strict
