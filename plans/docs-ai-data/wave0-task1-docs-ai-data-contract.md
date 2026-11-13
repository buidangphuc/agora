# [W0-T1] docs-ai-data-contract

## Role
feature-run worker (change `openspec/changes/docs-ai-data-contract`)

## Objective
Implement OpenSpec change `docs-ai-data-contract` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md (Contracts).
- analytics.proto: `EVENT_TYPE_EXPERIMENT_EXPOSURE = 22`, `GetDemandForecastResponse.generated_at = 8`, rpcs `GetRetentionCohorts`, `GetExperimentReadout`, `GetAiSurfaceReport` + nine messages; ai.proto: `request_id`/`model_version` on the four AI responses. Refresh vendored consumer copies byte-for-byte (5 analytics.proto, 10 ai.proto consumers) and checked-in stubs.
- depends_on: none
- base: `asdlc/docs-ai-data`

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/analytics/v1/**
- platform-core/packages/proto/platform/ai/v1/**
- platform-core/packages/proto/aidata_consumer_copies_test.sh
- platform-core/packages/proto/Makefile
- platform-core/FEATURES.yaml
- platform-core/README.md
- team-*/proto/platform/analytics/v1/analytics.proto
- team-*/proto/platform/ai/v1/ai.proto
- fe-platform/packages/api-client/src/generated/platform/analytics/v1/**
- fe-platform/.changeset/**

## Verify
- `cd platform-core/packages/proto && go test ./platform/analytics/v1/... ./platform/ai/v1/... && bash aidata_consumer_copies_test.sh`
- `make -C platform-core proto` (buf lint + breaking against origin/main)
- `make -C platform-core check`
- `openspec validate docs-ai-data-contract --strict`
