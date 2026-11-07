# [W0-T1] discovery-seo-contract

## Role
feature-run worker (change `openspec/changes/discovery-seo-contract`)

## Objective
Implement OpenSpec change `discovery-seo-contract` test-first per its tasks.md (scenario names there); context in plans/discovery-seo/00-spec.md (Locked + Clarifications) and the change's design.md.
- Additive contract: `SORT_BY_BEST_SELLING = 5`; `request_id`/`model_version` on both search responses; `sold_count`, `rating_avg`, `review_count`, `seller_id` on `SearchHit`; `order.events/ListingSalesChanged`; `engagement.events/ListingRatingChanged`; `RecommendRequest.context_listing_ids = 6`; `AnalyticsQueryService.GetSearchPerformance`. Byte-identical copies into the consumer repos; gateway admin-scope policy + forwarder for `GetSearchPerformance`.
- depends_on: none
- base: `feat/discovery-seo` (wave 0 runs alone)

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/search/v1/search.proto
- platform-core/packages/proto/platform/search/v1/discovery_compat_test.go
- platform-core/packages/proto/platform/order/v1/order.proto
- platform-core/packages/proto/platform/order/v1/discovery_compat_test.go
- platform-core/packages/proto/platform/engagement/v1/events.proto
- platform-core/packages/proto/platform/engagement/v1/discovery_compat_test.go
- platform-core/packages/proto/platform/recommendation/v1/recommendation.proto
- platform-core/packages/proto/platform/recommendation/v1/discovery_compat_test.go
- platform-core/packages/proto/platform/analytics/v1/analytics.proto
- platform-core/packages/proto/platform/analytics/v1/discovery_compat_test.go
- platform-core/packages/proto/discovery_seo_consumer_copies_test.sh
- platform-core/gen/**
- platform-core/FEATURES.yaml
- team-*/proto/platform/search/v1/search.proto
- team-*/proto/platform/order/v1/order.proto
- team-*/proto/platform/engagement/v1/events.proto
- team-*/proto/platform/recommendation/v1/recommendation.proto
- team-*/proto/platform/analytics/v1/analytics.proto
- fe-platform/packages/api-client/src/generated/platform/search/v1/*
- fe-platform/packages/api-client/src/generated/platform/order/v1/*
- fe-platform/packages/api-client/src/generated/platform/analytics/v1/*
- team-gateway/internal/edge/policy.go
- team-gateway/internal/edge/policy_test.go
- team-gateway/internal/edge/analytics.go
- team-gateway/internal/edge/analytics_test.go
- team-gateway/FEATURES.yaml

## Verify
- `cd platform-core/packages/proto && go vet ./... && go test ./...`
- `bash platform-core/packages/proto/discovery_seo_consumer_copies_test.sh`
- `make -C platform-core check` (buf lint + breaking vs origin/main)
- `make -C platform-core proto`
- `cd team-gateway && buf generate --template buf.gen.local.yaml && go vet ./... && go test ./...`
- e2e: none (tasks 2.1: no user-observable behaviour; FEATURES entries point at the Go and shell contract tests). End-to-end smoke is covered by search-attribution and search-bestseller-rating.
