# [W1-T1] go-sdk-v1-0-2

## Role
feature-run worker (change `openspec/changes/go-sdk-v1-0-2`)
## Objective
Implement OpenSpec change `go-sdk-v1-0-2` test-first per its tasks.md: remove postgres.OpenOrLocal, gRPC reflection off by default (GRPC_REFLECTION_ENABLED, refused in strict ENV or without Runtime), healthx `liveness` service independent of checks, chart opt-in probes.livenessService; context design.md Clarifications.
## Write-set (EXCLUSIVE)
- platform-core/packages/go-sdk/**
- platform-core/templates/_service-go/**
- platform-core/FEATURES.yaml
- platform-gitops/charts/service/**
- platform-gitops/tests/golden/**
- platform-gitops/FEATURES.yaml
- platform-e2e/tests/e2e/features/ops/gitops_render.feature
- platform-e2e/tests/e2e/step_definitions/pear_authz_gitops_steps.py
- platform-e2e/tests/e2e/step_definitions/test_pear_authz_gitops.py
- openspec/changes/go-sdk-v1-0-2/**
## Verify
- cd platform-core/packages/go-sdk && go build ./... && go vet ./... && go test ./... -count=1
- GO_SDK_REQUIRE_DOCKER=1 make -C platform-core/packages/go-sdk check
- (cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/step_definitions/test_pear_authz_gitops.py)
- openspec validate go-sdk-v1-0-2 --strict
- make -C platform-e2e features-check
