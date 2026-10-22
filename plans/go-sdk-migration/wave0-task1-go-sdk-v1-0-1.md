# [W0-T1] go-sdk-v1-0-1

## Role
feature-run worker (change `openspec/changes/go-sdk-v1-0-1`)

## Objective
Implement OpenSpec change `go-sdk-v1-0-1` test-first per its tasks.md (G1–G6: module path + tag distribution, no enable flags, pkg/mockdata, pkg/auditx, key_fp fingerprint, 23505 → AlreadyExists); context plans/go-sdk-migration/PLAN.md and the change's design.md Clarifications.

## Write-set (EXCLUSIVE)
- platform-core/packages/go-sdk/**
- platform-core/make/go-service.mk
- platform-core/templates/_service-go/**
- platform-core/tools/sync-go-sdk.sh
- platform-core/tools/sync-go-sdk_test.sh
- platform-core/tools/check-go-sdk-vendor.sh
- platform-core/tools/check-go-sdk-vendor_test.sh
- platform-core/tools/template_smoke_test.sh
- platform-core/docs/ADR/0016-go-service-convention-and-sdk.md
- platform-core/docs/ADR/0018-*.md
- platform-core/FEATURES.yaml
- .github/workflows/asdlc-gates.yml
- scripts/asdlc/ci_local.sh
- openspec/changes/go-sdk-v1-0-1/**

## Verify
- cd platform-core/packages/go-sdk && go build ./... && go vet ./... && go test ./... -count=1 && go test -race ./pkg/consume/
- make -C platform-core/packages/go-sdk check
- openspec validate go-sdk-v1-0-1 --strict
- make -C platform-e2e features-check
