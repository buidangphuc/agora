# [W4-T1] engagement-dispute-escalation

## Role
feature-run worker (change `openspec/changes/engagement-dispute-escalation`)

## Objective
Implement OpenSpec change `engagement-dispute-escalation` test-first per its tasks.md (scenario ids there); context in plans/q6-trust-enforcement/00-spec.md and the change's design.md Clarifications.
- HELD until q10-order-lifecycle merges to main. Escalate a return to a dispute (return_id, can_escalate), tightened dispute state machine with ruling, audit-first ResolveDispute + DisputeResolved fact, ListOrderDisputes/ListDisputes; team-order service-principal GetReturnRequest; gateway routes.

**HELD:** build only after q10-order-lifecycle has merged to main.
- depends_on: engagement-listing-reports; serialised after (write-set overlap): domain-listing-moderation, identity-user-enforcement

## Write-set (EXCLUSIVE)
- team-engagement/migrations/*_dispute_escalation.up.sql
- team-engagement/migrations/*_dispute_escalation.down.sql
- team-engagement/internal/repository/dispute.go
- team-engagement/internal/repository/dispute_test.go
- team-engagement/internal/repository/pgtest_dispute_test.go
- team-engagement/internal/upstream/order.go
- team-engagement/internal/upstream/order_test.go
- team-engagement/internal/service/dispute.go
- team-engagement/internal/service/dispute_test.go
- team-engagement/internal/handler/engagement.go
- team-engagement/internal/handler/disputes_escalation_test.go
- team-engagement/internal/handler/disputes_audit_test.go
- team-engagement/internal/handler/authz_test.go
- team-engagement/internal/events/relayer.go
- team-engagement/internal/events/relayer_test.go
- team-engagement/cmd/server/main.go
- team-engagement/generated/**
- team-engagement/README.md
- team-engagement/FEATURES.yaml
- team-order/internal/handler/order.go
- team-order/internal/handler/order_return_read_test.go
- team-order/FEATURES.yaml
- team-gateway/internal/edge/engagement.go
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/policy_edge_test.go
- team-gateway/internal/edge/unrouted_test.go
- team-gateway/internal/edge/policy_edge_disputes_test.go
- team-gateway/generated/**
- team-gateway/FEATURES.yaml
- platform-e2e/tests/e2e/features/engagement/dispute_escalation.feature
- platform-e2e/tests/e2e/step_definitions/test_dispute_escalation.py
- platform-e2e/tests/e2e/support/dde_support.py
- platform-e2e/tests/e2e/features/engagement/dispute_audit.feature
- platform-e2e/tests/e2e/features/engagement/disputes.feature
- platform-e2e/tests/e2e/step_definitions/group_b_steps.py
- platform-e2e/tests/e2e/step_definitions/disputes_access_steps.py
- platform-e2e/tests/e2e/step_definitions/test_dispute_audit.py
- platform-e2e/tests/e2e/step_definitions/test_disputes.py
- platform-e2e/tests/e2e/step_definitions/test_dispute_access.py
- openspec/changes/engagement-dispute-escalation/**
- team-engagement/internal/grpcserver/server_test.go (# widened at build: task 4)
- team-engagement/internal/handler/engagement_test.go (# widened at build: task 4)
- plans/q6-trust-enforcement/wave4-task1-engagement-dispute-escalation.md (# widened at build: task 4; packet widening record)

## Read-only dependencies
- openspec/changes/engagement-dispute-escalation/** is frozen by the LOCK.md digest (only task ticks change)
- platform-e2e/tests/e2e/support/prm_rma_support.py, audit_support.py (reuse, do not edit)
- q10-order-lifecycle compose overlay (short seller answer time)
- plans/q6-trust-enforcement/00-spec.md, slices.yaml, LOCK.md

## Verify
(cd team-engagement && make proto && go vet ./... && go test ./... -race); (cd team-order && go vet ./... && go test ./internal/handler/... -race); (cd team-gateway && go vet ./... && go test ./internal/edge/... -race); (cd platform-e2e && HEADLESS=true $PY -m pytest -q tests/e2e/step_definitions/test_dispute_escalation.py tests/e2e/step_definitions/test_dispute_audit.py tests/e2e/step_definitions/test_disputes.py tests/e2e/step_definitions/test_dispute_access.py -m 'not destructive') then the @destructive audit-down scenario alone; $PY platform-e2e/scripts/spec_sync.py engagement-dispute-escalation --strict
