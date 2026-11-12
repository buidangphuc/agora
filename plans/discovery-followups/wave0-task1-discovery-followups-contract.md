# [W0-T1] discovery-followups-contract

## Role
feature-run worker (change `openspec/changes/discovery-followups-contract`)

## Objective
Implement OpenSpec change `discovery-followups-contract` test-first per its tasks.md; context in plans/discovery-followups/00-spec.md (Contracts).
- `LISTING_STATUS_DELETED = 7` in listing.proto; `SearchService.GetListingStock` + messages in search.proto; refresh vendored consumer copies byte-for-byte.
- depends_on: none
- base: `asdlc/discovery-followups`

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/listing/v1/**
- platform-core/packages/proto/platform/search/v1/**
- platform-core/packages/proto/followups_consumer_copies_test.sh
- platform-core/packages/proto/Makefile
- platform-core/FEATURES.yaml
- platform-core/README.md
- team-*/proto/platform/listing/v1/listing.proto
- team-*/proto/platform/search/v1/search.proto
- team-frontend*/package-lock.json
- team-frontend*/vendor/fe-platform-api-client.tgz
- fe-platform/packages/api-client/src/generated/**
- fe-platform/.changeset/**
- platform-core/packages/proto/platform/listing/v1/moderation_compat_test.go
- team-*/README.md

## Verify
- `cd platform-core/packages/proto && go test ./platform/listing/v1/... ./platform/search/v1/... && bash followups_consumer_copies_test.sh`
- `make -C platform-core proto` (buf lint + breaking against origin/main)
- `make -C platform-core check`
