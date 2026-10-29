# [W5-T1] deploy-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/deploy-go-sdk-v1`)

## Objective
Implement OpenSpec change `deploy-go-sdk-v1` test-first per its tasks.md sections 1 and 2: compose migrate jobs on the service image, removed keys gone, `GRPC_REFLECTION_ENABLED=true` locally, gateway `GRPC_PORT`, relay gRPC probes, new DLQ topics, chart PreSync migrate Job, liveness probes by name, strict-ENV overlays, raw manifest overlays, Vault seed, outbox rollout runbook, `migrate up` for team-sharing and team-referral, e2e/FEATURES edits. Built last, after every `<svc>-go-sdk-v1` change.

## Write-set (EXCLUSIVE)
- docker-compose.services.yaml
- platform-gitops/**
- scripts/asdlc/tests/**
- team-sharing/cmd/**
- team-sharing/internal/app/**
- team-referral/cmd/**
- team-referral/internal/app/**
- docs/runbooks/team-verification.md
- platform-e2e/tests/e2e/features/ops/**
- platform-e2e/tests/e2e/features/tracking/ga4_*.feature
- platform-e2e/tests/e2e/step_definitions/**
- platform-e2e/tests/e2e/support/ga4_relay_support.py
- platform-e2e/scripts/tests/test_gitops_ga4.py
- platform-e2e/compose/**
- openspec/changes/deploy-go-sdk-v1/**
- platform-gitops/platform/search-indexer-overlays/** (widened at build: task 1.6.1)
- platform-gitops/platform/team-analytics-overlays/** (widened at build: task 1.6.1)

## Verify
- python -m pytest platform-gitops/tests scripts/asdlc/tests -q
- cd team-sharing && GO_SDK_REQUIRE_DOCKER=1 go test ./... -count=1; same in team-referral
- docker compose -f docker-compose.yaml -f docker-compose.services.yaml config -q
- helm template for all 14 chart services (tasks.md 1.9.1)
- openspec validate deploy-go-sdk-v1 --strict
- make -C platform-e2e features-check
