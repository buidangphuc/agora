# tag-taxonomy-enrichment Specification

## Purpose
Defines how listings are enriched with classified tags: team-ai's internal tag classification and taxonomy mining and promotion, the SPU tags and per-variant SKU attributes the search read model stores, the filters and dynamic facets search offers on them, and the search page that shows them.

## Requirements

### Requirement: Online SPU and Granular SKU Tag Classification

The system SHALL provide high-performance online inference endpoints (`POST /api/v1/ai/tags/classify` and `POST /api/v1/ai/tags/classify-sku-hierarchy`) that normalize text, resolve synonyms, and extract canonical filter facets for both parent listings and child SKU variants.

#### Scenario: SPU Level Tag Classification

- **GIVEN** a seller provides a product title and description (e.g., "Tai nghe Bluetooth 5.3 chống ồn ANC sạc nhanh 65W GaN")
- **WHEN** the seller requests tag classification via `/api/v1/ai/tags/classify`
- **THEN** the system returns matching canonical tags (`bluetooth-5-3`, `chong-on-chu-dong-anc`, `sac-nhanh-65w-gan`) mapped to their respective facet groups (`connectivity`, `feature`, `power`) in under 10ms.

#### Scenario: Granular SKU Level Hierarchical Classification

- **GIVEN** a parent listing with multiple child SKU variants (e.g. "Titan Tự Nhiên / 256GB" and "Xanh Navy / 512GB")
- **WHEN** hierarchical classification is invoked via `/api/v1/ai/tags/classify-sku-hierarchy`
- **THEN** each SKU inherits common parent SPU tags while resolving its specific variant dimensions (`color`, `capacity`, `size`, `material`), and outputs an OpenSearch 2.11 nested document structure.

### Requirement: Offline Candidate Exploration and Mining

The system SHALL provide an offline exploration pipeline (`POST /api/v1/ai/tags/explore`) that processes batches of raw listings and SKU variants to mine emergent technical specifications and register them into a candidate exploration pool.

#### Scenario: Candidate Tag Discovery from Raw Batch

- **GIVEN** a batch of unclassified listings containing recurring technical specs (e.g. "Công suất 100W", "Dung lượng 20000mAh", "Vải Linen Tự Nhiên")
- **WHEN** offline exploration is executed with support threshold $N \ge 2$ and confidence $\ge 0.80$
- **THEN** the system discovers candidate clusters, calculates occurrence frequencies, and registers candidate tags with status `EXPLORING`.

### Requirement: Promotion Gating to Canonical Search Facet

The system SHALL provide a promotion gating engine (`POST /api/v1/ai/tags/promote`) that elevates validated candidate tags to canonical filter facets, immediately updating online classification and search facet aggregations.

#### Scenario: Candidate Tag Promoted to Canonical Filter Facet

- **GIVEN** an exploring candidate tag (e.g. `cong-suat-100w`) that satisfies catalog frequency and confidence criteria
- **WHEN** promotion is executed with target category and bound synonyms
- **THEN** the tag status transitions to `PROMOTED` (`is_canonical = true`), and every listing classified or re-indexed after the promotion carries the tag, so search facets include it for those listings.

### Requirement: Search Read-Model Carries Classified SPU Tags and Nested SKU Attributes

The search indexer SHALL classify every created or updated listing through team-ai's tag classifier (gRPC `AIService.ClassifyTags`, called as a service principal) and store, on the listing's OpenSearch document, its canonical SPU tags as `group:slug` keywords (`facet_tags`) and one object per variant in a `nested` field (`skus`) with that variant's own attributes (`attrs`), price, stock and `is_in_stock`. A listing whose variant carries `is_in_stock = true` only when that variant's stock is above zero. A classifier failure SHALL NOT fail or delay indexing: the document is written with `tags_pending` and keeps the tags it already had.

#### Scenario: Listing Events Index SPU Tags

- **GIVEN** a seller publishes a listing titled "Tai nghe Bluetooth 5.3 chống ồn ANC sạc nhanh 65W GaN"
- **WHEN** the listing event has been indexed
- **THEN** `SearchListings` with the filter `tag.connectivity` = `bluetooth-5-3` returns the listing, and the same search with `tag.connectivity` = `wifi-6` does not.

#### Scenario: Classifier Outage Does Not Block Indexing

- **GIVEN** team-ai's tag classifier is unreachable or answers an error
- **WHEN** a listing event is indexed
- **THEN** the listing is searchable by its title, its document carries `tags_pending`, and the tags it had before stay stored.
- **VERIFIED BY**: Go test `team-search/internal/consumer/listing_test.go` › `TestListingEventHandler_ClassifierOutageDoesNotFailIngestion` and `team-search/internal/index/attributes_integration_test.go` › `TestIT_Attr_TagsPendingKeepsStoredTags`. Not verifiable end to end: it needs team-ai stopped, which no scenario may do to the shared stack.

### Requirement: SKU Filters Match One In-Stock Variant

A request filter `sku.<group>` (comma-separated slugs: OR inside a group, AND across groups) SHALL match a listing only when a SINGLE variant that is in stock carries every requested attribute. Filters `tag.<group>` SHALL match the listing's SPU tags. A malformed group (not `[a-z][a-z0-9_]{0,31}`) or slug (not `[a-z0-9][a-z0-9-]{0,63}`) SHALL be rejected with `invalid_argument` by `SearchListings`, `SaveSearch` and `RunSavedSearch`.

#### Scenario: SKU Filters Match One Variant, Not Across Variants

- **GIVEN** a published listing with variants "Titan Tự Nhiên / 256GB" and "Xanh Navy / 512GB" in stock, and a second listing with variants "Xanh Navy / 256GB" and "Titan Tự Nhiên / 512GB"
- **WHEN** a buyer searches with `sku.color` = `xanh-navy` and `sku.capacity` = `512gb`
- **THEN** only the first listing is returned (the second has both values, but on different variants).

#### Scenario: Sold-Out Variants Do Not Satisfy SKU Filters

- **GIVEN** a published listing whose only "Xanh Navy / 512GB" variant has stock 0, and another with that variant in stock
- **WHEN** a buyer searches with `sku.color` = `xanh-navy` and `sku.capacity` = `512gb`
- **THEN** only the listing whose matching variant is in stock is returned.

#### Scenario: Malformed Facet Filters Are Rejected

- **WHEN** a buyer calls `SearchListings` with the filter `sku.Colour Name` = `x` or `tag.color` = `den:other`
- **THEN** the call fails with `invalid_argument` and no search is run.

### Requirement: Search Responses Carry Dynamic Facets

`SearchListingsResponse.facets` (and `RunSavedSearchResponse.facets`) SHALL include `tags` and `skus`: one entry per facet group present in the matched set, each with value buckets (`key` = tag slug, `count` = number of matching LISTINGS). `skus` counts only listings reachable through an in-stock variant that also satisfies every active `sku.*` filter, so a bucket's count is the result size after adding that value.

#### Scenario: Search Response Carries Dynamic Facets

- **GIVEN** published listings with classified SPU tags and variants
- **WHEN** a buyer searches for their keyword through the gateway
- **THEN** `facets.tags` lists a `connectivity` group with the `bluetooth-5-3` bucket and `facets.skus` lists `color` and `capacity` groups, each bucket counting listings.

### Requirement: Search Page Offers Dynamic Facets

The `/search` page SHALL render one filter group per `facets.skus` and `facets.tags` group (a variant group hides a same-named SPU group), as links that toggle the tag slug in the URL parameter `sku.<group>` or `tag.<group>` (comma-separated), show the count beside each bucket, list each selected value as a removable active-filter chip, and keep every other URL parameter. The state is the URL; nothing is client-side.

#### Scenario: Selecting a Dynamic Facet Narrows Results and Reflects in the URL

- **GIVEN** published listings with different variant colors indexed for a keyword
- **WHEN** a buyer opens `/search` for that keyword and selects the `xanh-navy` bucket of the color facet
- **THEN** the URL contains `sku.color=xanh-navy`, only listings with an in-stock navy variant remain, and the selected value is shown as an active filter chip.

### Requirement: Tag Classification Is Internal

`AIService.ClassifyTags` SHALL be callable only by a service principal holding the scope `ai.classify`; it SHALL NOT be routed by the gateway, and no user role SHALL be granted the scope.

#### Scenario: Tag Classification Is Not Reachable At The Edge

- **WHEN** an anonymous caller and a signed-in buyer call `/platform.ai.v1.AIService/ClassifyTags` through the gateway
- **THEN** both calls fail with `unimplemented` (HTTP 501) and no classification is returned.
