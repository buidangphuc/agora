# discovery-seo-contract Specification

## Purpose
TBD - created by archiving change discovery-seo-contract. Update Purpose after archive.

## Requirements

### Requirement: Best-selling sort value
The search proto SHALL add `SORT_BY_BEST_SELLING = 5` to `SortBy`, documented as sold_count descending, then created_at descending, then id, without altering values 0 to 4.

#### Scenario: Enum value present with stable numbers
- **WHEN** the descriptor of `platform.search.v1.SortBy` is inspected
- **THEN** it has exactly 6 values with `SORT_BY_UNSPECIFIED=0`, `SORT_BY_RELEVANCE=1`, `SORT_BY_PRICE_ASC=2`, `SORT_BY_PRICE_DESC=3`, `SORT_BY_NEWEST=4` and `SORT_BY_BEST_SELLING=5`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_SortByBestSellingIsFive"

#### Scenario: Ordering is documented
- **WHEN** the comment on `SORT_BY_BEST_SELLING` in search.proto is read
- **THEN** it contains the text `sold_count desc, created_at desc, id`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_BestSellingOrderingDocumented"

#### Scenario: Old consumer meets the new value
- **WHEN** a `SearchListingsRequest` with `sort_by=5` is parsed by the previous schema
- **THEN** parsing succeeds and `sort_by` reads as enum number 5 with no defined name with every other field unchanged, and a request that omits `sort_by` still reads `SORT_BY_UNSPECIFIED`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_OldSchemaParsesBestSelling"

#### Scenario: Undefined sort number is not a defined value
- **WHEN** the descriptor of `SortBy` is searched for number 6
- **THEN** no value exists, so a consumer must not treat 6 as best-selling and the check fails if a value is added without a spec change
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_NoUndefinedSortNumbers"

### Requirement: Search responses carry attribution
`SearchListingsResponse` and `RunSavedSearchResponse` SHALL each add `string request_id = 4` and `string model_version = 5`, and the comments SHALL state that `request_id` is minted per call, that `model_version` has the form `<mode>[+rerank]@<version>`, and that `<mode>` is the mode that actually ran.

#### Scenario: Both responses expose the same two fields at the same numbers
- **WHEN** the descriptors of `SearchListingsResponse` and `RunSavedSearchResponse` are inspected
- **THEN** each has `request_id=4 string` and `model_version=5 string`, and fields 1 to 3 are unchanged (`hits`, `page`, `facets`)
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_ResponsesCarryAttributionFields"

#### Scenario: Attribution round trip and empty default
- **WHEN** a `SearchListingsResponse` with `request_id="a1b2"` and `model_version="hybrid+rerank@v1"` is serialised and parsed, and a response without them is parsed
- **THEN** the first parse equals the original and the second reads both fields as empty strings
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_AttributionRoundTrip"

#### Scenario: Old payload stays readable and new payload is readable by the old schema
- **WHEN** bytes serialised by the previous response schema are parsed with the new one, and bytes with fields 4 and 5 set are parsed with the previous one
- **THEN** both parses succeed and `hits`, `page` and `facets` are unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_AttributionWireCompatible"

#### Scenario: model_version format is documented
- **WHEN** the comment on `model_version` in search.proto is read
- **THEN** it contains the text `<mode>[+rerank]@<version>` and the text `mode that actually ran`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_ModelVersionFormatDocumented"

### Requirement: SearchHit carries sold and rating numbers
The hit stays readable with the existing public search scope; no extra scope is needed. `SearchHit` SHALL add `int64 sold_count = 4`, `double rating_avg = 5`, `int64 review_count = 6` and `string seller_id = 7`, leaving `listing_id = 1`, `score = 2` and `optional int32 stock = 3` unchanged. The comments SHALL state that `rating_avg` counts verified, non-hidden reviews only, is 0 when `review_count` is 0 and stays within 0 to 5, that `sold_count` and `review_count` are never below 0, and that `seller_id` is the listing's seller, empty when not yet projected.

#### Scenario: Hit fields present with stable numbers
- **WHEN** the descriptor of `platform.search.v1.SearchHit` is inspected
- **THEN** it has exactly 7 fields: `listing_id=1 string`, `score=2 float`, `stock=3 optional int32`, `sold_count=4 int64`, `rating_avg=5 double`, `review_count=6 int64`, `seller_id=7 string`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_SearchHitFieldsStable"

#### Scenario: Unprojected listing reads as zero
- **WHEN** a `SearchHit` serialised by the previous schema (fields 1 to 3 only) is parsed with the new schema
- **THEN** parsing succeeds, `sold_count` is 0, `rating_avg` is 0, `review_count` is 0, `seller_id` is empty, and `listing_id`, `score` and `stock` are unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_OldHitReadsZeroes"

#### Scenario: Maximum count round trips
- **WHEN** a `SearchHit` with `sold_count=9223372036854775807`, `rating_avg=5`, `review_count=9223372036854775807` and `seller_id="s-1"` is serialised and parsed
- **THEN** the parsed hit equals the original
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_HitBoundaryValuesRoundTrip"

#### Scenario: Rating semantics are documented
- **WHEN** the comments on `sold_count`, `rating_avg`, `review_count` and `seller_id` in search.proto are read
- **THEN** they contain the texts `verified, non-hidden reviews`, `0 when review_count is 0`, `within 0 to 5`, `never below 0` and `empty when not yet projected`
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_RatingSemanticsDocumented"

#### Scenario: Existing hit fields are not renumbered
- **WHEN** `listing_id`, `score` and `stock` are compared with their previous names, types and numbers
- **THEN** the check fails and names the field if any differs
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_ExistingHitFieldsNotRenumbered"

#### Scenario: Seller id round trips
- **WHEN** a `SearchHit` with `seller_id="s-1"` is serialised and parsed, and a hit serialised by the previous schema is parsed with the new one
- **THEN** the first parse equals the original and the second reads `seller_id` as an empty string
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_SellerIdRoundTrip"

#### Scenario: Hit exposes no private data
- **WHEN** the fields of `SearchHit` are listed
- **THEN** none of `user_id`, `buyer_id`, `email`, `phone` or `rating_by_user` is present, and the check fails if one is added
- **VERIFIED BY**: `platform-core/packages/proto/platform/search/v1/discovery_compat_test.go` "TestDiscovery_HitHasNoPrivateFields"

### Requirement: ListingSalesChanged event
The order proto SHALL define `message ListingSalesChanged` with `string listing_id = 1`, `int64 sold_count = 2` and `google.protobuf.Timestamp changed_at = 3`, published on `order.events` as an `EventEnvelope` payload. `sold_count` SHALL be an absolute value, never below 0, and the comment SHALL name the topic `order.events`, the key `listing_id` and the envelope type `platform.order.v1.ListingSalesChanged`, and the message SHALL carry no personal or order identifier.

#### Scenario: Message shape is fixed
- **WHEN** the descriptor of `platform.order.v1.ListingSalesChanged` is inspected
- **THEN** it has exactly 3 fields `listing_id=1 string`, `sold_count=2 int64`, `changed_at=3 google.protobuf.Timestamp`
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/discovery_compat_test.go` "TestDiscovery_ListingSalesChangedShape"

#### Scenario: No personal or order fields
- **WHEN** the fields of `ListingSalesChanged` are listed
- **THEN** none of `buyer_id`, `user_id`, `seller_id`, `order_id` or `delta` is present
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/discovery_compat_test.go` "TestDiscovery_ListingSalesChangedHasNoForbiddenFields"

#### Scenario: Round trip and absolute-value documentation
- **WHEN** a `ListingSalesChanged` with `listing_id="l-1"`, `sold_count=0` and a `changed_at` is serialised and parsed, and the comment on `sold_count` is read
- **THEN** the parsed message equals the original (zero is kept as a valid absolute value) and the comment contains the text `absolute` and the text `never below 0`
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/discovery_compat_test.go` "TestDiscovery_ListingSalesChangedRoundTrip"

#### Scenario: Topic, key and envelope type are documented
- **WHEN** the comment on `ListingSalesChanged` in order.proto is read
- **THEN** it contains the texts `order.events`, `key = listing_id` and `platform.order.v1.ListingSalesChanged`
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/discovery_compat_test.go` "TestDiscovery_ListingSalesChangedTopicKeyTypeDocumented"

#### Scenario: Existing order messages are untouched
- **WHEN** the descriptors of `OrderPaidEvent`, `OrderCancelled` and `ReturnRefunded` are compared with their previous field lists
- **THEN** every previous field keeps its name, type and number, and the check reports an error naming the message if one differs
- **VERIFIED BY**: `platform-core/packages/proto/platform/order/v1/discovery_compat_test.go` "TestDiscovery_OrderEventsUnchanged"

### Requirement: ListingRatingChanged event
The engagement events proto SHALL define `message ListingRatingChanged` with `string listing_id = 1`, `double average_rating = 2`, `int64 review_count = 3` and `google.protobuf.Timestamp changed_at = 4`, published on `engagement.events` keyed by listing id as an `EventEnvelope` payload of type `platform.engagement.v1.ListingRatingChanged`, all three named in the comment. The values SHALL cover verified, non-hidden reviews only, `review_count` SHALL never be below 0, `average_rating` SHALL be 0 when `review_count` is 0 and otherwise within 0 to 5, and the message SHALL carry no personal identifier, review id or review text.

#### Scenario: Message shape is fixed
- **WHEN** the descriptor of `platform.engagement.v1.ListingRatingChanged` is inspected
- **THEN** it has exactly 4 fields `listing_id=1 string`, `average_rating=2 double`, `review_count=3 int64`, `changed_at=4 google.protobuf.Timestamp`
- **VERIFIED BY**: `platform-core/packages/proto/platform/engagement/v1/discovery_compat_test.go` "TestDiscovery_ListingRatingChangedShape"

#### Scenario: No reviewer or text fields
- **WHEN** the fields of `ListingRatingChanged` are listed
- **THEN** none of `user_id`, `reviewer_id`, `review_id`, `text`, `title` or `rating` is present
- **VERIFIED BY**: `platform-core/packages/proto/platform/engagement/v1/discovery_compat_test.go` "TestDiscovery_ListingRatingChangedHasNoForbiddenFields"

#### Scenario: Last review hidden leaves zero
- **WHEN** a `ListingRatingChanged` with `average_rating=0` and `review_count=0` is serialised and parsed, and the comment on `average_rating` is read
- **THEN** the parsed message equals the original and the comment contains the text `0 when review_count is 0`
- **VERIFIED BY**: `platform-core/packages/proto/platform/engagement/v1/discovery_compat_test.go` "TestDiscovery_ListingRatingChangedZeroReviews"

#### Scenario: Topic, key and envelope type are documented
- **WHEN** the comment on `ListingRatingChanged` in events.proto is read
- **THEN** it contains the texts `engagement.events`, `key = listing_id` and `platform.engagement.v1.ListingRatingChanged`
- **VERIFIED BY**: `platform-core/packages/proto/platform/engagement/v1/discovery_compat_test.go` "TestDiscovery_ListingRatingChangedTopicKeyTypeDocumented"

#### Scenario: Existing review events are untouched
- **WHEN** the descriptors of `ReviewCreated` and `ReviewHidden` are compared with their previous field lists
- **THEN** every previous field keeps its name, type and number, and the check reports an error naming the message if one differs
- **VERIFIED BY**: `platform-core/packages/proto/platform/engagement/v1/discovery_compat_test.go` "TestDiscovery_ReviewEventsUnchanged"

### Requirement: Recommendation context from the cart
`RecommendRequest` SHALL add `repeated string context_listing_ids = 6` for cart lines, documented as at most 50 ids with longer lists rejected with INVALID_ARGUMENT at both the gateway and team-ai, and SHALL keep `seed_listing_id = 3` and fields 1 to 5 unchanged.

#### Scenario: Field present with stable number
- **WHEN** the descriptor of `platform.recommendation.v1.RecommendRequest` is inspected
- **THEN** `context_listing_ids` is `repeated string` at number 6 and `user_id=1`, `anonymous_id=2`, `seed_listing_id=3`, `context=4`, `limit=5` are unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/recommendation/v1/discovery_compat_test.go` "TestDiscovery_ContextListingIdsIsSix"

#### Scenario: Old request reads an empty context
- **WHEN** a `RecommendRequest` serialised by the previous schema is parsed with the new one, and one with `context_listing_ids=["a","b"]` is parsed by the previous schema
- **THEN** the first parse yields an empty `context_listing_ids` with the other fields unchanged, and the second parse succeeds with fields 1 to 5 unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/recommendation/v1/discovery_compat_test.go` "TestDiscovery_RecommendRequestWireCompatible"

#### Scenario: The 50-id cap is documented
- **WHEN** the comment on `context_listing_ids` in recommendation.proto is read
- **THEN** it contains the texts `max 50` and `INVALID_ARGUMENT`, and states that both the gateway and team-ai reject longer lists, with no truncation
- **VERIFIED BY**: `platform-core/packages/proto/platform/recommendation/v1/discovery_compat_test.go` "TestDiscovery_ContextCapDocumented"

### Requirement: Search performance query
The analytics proto SHALL add `rpc GetSearchPerformance(GetSearchPerformanceRequest) returns (GetSearchPerformanceResponse)` to `AnalyticsQueryService`, mirroring `GetRecommendationPerformance`. `GetSearchPerformanceRequest` SHALL have `uint32 window_hours = 1` (1 to 168, 0 means 24). `GetSearchPerformanceResponse` SHALL have `repeated RecommendationPerformanceRow rows = 1`, `uint32 window_hours = 2` (the effective window) and `uint32 attribution_window_hours = 3`, with the comments stating that every row has `placement_id = search_results`, that the call is admin-only (scope "admin") and that it carries no per-user data. The gateway SHALL route it with the admin scope exactly as `GetRecommendationPerformance`.

#### Scenario: RPC and messages present
- **WHEN** the descriptors of `AnalyticsQueryService` and the two new messages are inspected
- **THEN** the service has `GetSearchPerformance` taking `GetSearchPerformanceRequest` and returning `GetSearchPerformanceResponse`, the request has `window_hours=1 uint32`, the response has `rows=1 repeated RecommendationPerformanceRow`, `window_hours=2 uint32` and `attribution_window_hours=3 uint32`, and `GetRecommendationPerformance*` messages are unchanged
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/discovery_compat_test.go` "TestDiscovery_GetSearchPerformanceShape"

#### Scenario: Window and admin semantics are documented
- **WHEN** the comments on the RPC and on `window_hours` are read
- **THEN** they contain the texts `Admin-only (scope "admin")`, `search_results`, `0 = 24`, `1..168` and `values above 168 are rejected with INVALID_ARGUMENT`
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/discovery_compat_test.go` "TestDiscovery_GetSearchPerformanceDocumented"

#### Scenario: Non-admin caller is denied at the gateway
- **WHEN** an anonymous client, a logged-in buyer without the admin scope, and an admin caller each call `GetSearchPerformance` through the gateway policy check
- **THEN** the anonymous call is rejected as unauthenticated, the buyer call as permission denied, and the admin caller passes the policy check (scope admin)
- **VERIFIED BY**: `team-gateway/internal/edge/policy_test.go` "TestPolicy_GetSearchPerformanceAdminOnly"

#### Scenario: Admin procedure set includes the new RPC
- **WHEN** the pinned set of admin procedures in the gateway policy test is read
- **THEN** it lists `AnalyticsQueryServiceGetSearchPerformanceProcedure`, and the test fails if the procedure is absent from the policy map
- **VERIFIED BY**: `team-gateway/internal/edge/policy_test.go` "TestAdminProceduresPinnedSet"

#### Scenario: Response semantics are documented
- **WHEN** the comments on `GetSearchPerformanceResponse.rows` and `attribution_window_hours` are read
- **THEN** they contain the texts `rows is empty when the window has no search impressions` and `attribution_window_hours is the attribution window used for purchases`
- **VERIFIED BY**: `platform-core/packages/proto/platform/analytics/v1/discovery_compat_test.go` "TestDiscovery_GetSearchPerformanceResponseDocumented"

### Requirement: Contract is additive and consumers are in sync
The change SHALL pass `buf lint` and `buf breaking` (FILE rules) against `origin/main`, and every consumer repo that vendors one of the five edited protos SHALL hold a byte-identical copy at `<repo>/proto/platform/<path>`.

#### Scenario: Lint and breaking check pass
- **WHEN** `buf lint` and `buf breaking --against` the `origin/main` module run in `platform-core/packages/proto`
- **THEN** both exit 0
- **VERIFIED BY**: `platform-core/packages/proto/discovery_seo_consumer_copies_test.sh` "Discovery SEO contract: buf lint and breaking pass"

#### Scenario: Copies match the source
- **WHEN** every vendored copy of `search.proto`, `order.proto`, engagement `events.proto`, `recommendation.proto` and `analytics.proto` is compared with the platform-core source
- **THEN** all copies are byte-identical
- **VERIFIED BY**: `platform-core/packages/proto/discovery_seo_consumer_copies_test.sh` "Discovery SEO contract: Copies match the source"

#### Scenario: Drifted or missing copy is rejected
- **WHEN** the check runs on a synthetic tree where one copy has an extra byte and another copy is missing
- **THEN** the check fails and names both paths
- **VERIFIED BY**: `platform-core/packages/proto/discovery_seo_consumer_copies_test.sh` "Discovery SEO contract: Drifted or missing copy is rejected"

#### Scenario: Renumbering is rejected
- **WHEN** `buf breaking` runs against a scratch copy of the module where `SearchHit.stock` is renumbered
- **THEN** it exits non-zero and names `SearchHit`
- **VERIFIED BY**: `platform-core/packages/proto/discovery_seo_consumer_copies_test.sh` "Discovery SEO contract: Renumbering is rejected"

#### Scenario: Consumer protos still build
- **WHEN** `buf build` runs in every consumer repo that holds a refreshed copy of any of the five edited protos (the union of the repos in the proposal, no sampling)
- **THEN** each exits 0 and the check lists the repos it built, failing if any repo in the union is missing from the list
- **VERIFIED BY**: `platform-core/packages/proto/discovery_seo_consumer_copies_test.sh` "Discovery SEO contract: Consumer protos build"
