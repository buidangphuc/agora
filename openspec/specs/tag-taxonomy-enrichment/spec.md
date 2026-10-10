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

### Requirement: Tag routes require an authenticated principal

Every tag route of team-ai (`POST /api/v1/ai/tags/classify`, `/classify-sku-hierarchy`, `/explore`, `/promote` and `GET /api/v1/ai/tags`) SHALL require a bearer token that team-ai verifies itself. The read routes (`classify`, `classify-sku-hierarchy`, list) SHALL accept a principal holding scope `ai.classify` or `admin`; the mutating routes (`explore`, `promote`) SHALL accept only a principal holding scope `admin`. A request without a valid token SHALL be refused with 401 and a valid token that lacks the scope with 403, and a refused request SHALL NOT change the taxonomy. A user's JWT SHALL NOT be accepted (only the gateway verifies JWTs).

#### Scenario: Anonymous tag requests are refused

- **WHEN** a caller without an `Authorization` header calls each of the five tag routes, the mutating ones with a valid body
- **THEN** every call answers 401 and the taxonomy is unchanged

#### Scenario: A buyer's token is refused

- **WHEN** a signed-in buyer presents the session JWT the gateway accepts (or any other wrong bearer token) to a read route and to `promote`
- **THEN** both answer 401 and the taxonomy is unchanged

#### Scenario: A read-only service principal cannot mutate the taxonomy

- **WHEN** a service principal holding only `ai.classify` classifies and lists tags, then calls `explore` and `promote`
- **THEN** classify and list answer 200, `explore` and `promote` answer 403, and no candidate is registered or promoted

#### Scenario: An admin principal explores and promotes

- **WHEN** an admin principal explores a batch and promotes the discovered candidate
- **THEN** both answer 200 and the tag is promoted and canonical

#### Scenario: Admin access is off unless configured and strong

- **WHEN** `AUTH_ADMIN_BEARER_TOKEN` is unset, or outside dev/local/test is weak or equal to `AUTH_BEARER_TOKEN`
- **THEN** no principal holds `admin` on the REST routes, and startup refuses the weak or equal configuration
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_routes_authz.py › test_admin_token_unset_grants_no_admin, test_weak_or_shared_admin_token_is_refused_outside_local. Not verifiable end to end: the stack runs with the local environment and a configured admin token (design.md).

### Requirement: Tag taxonomy survives a restart

When taxonomy persistence is enabled, team-ai SHALL store its canonical tags and exploring candidates in Redis and load them at startup, so that a tag promoted through `POST /api/v1/ai/tags/promote` is still canonical, and an explored candidate still a candidate, after team-ai restarts. A promotion SHALL be durable before it is acknowledged: if the store cannot be written the call SHALL answer 503 and leave the taxonomy unchanged. The REST routes and gRPC `ClassifyTags` SHALL answer from the same registry. A store outage at startup SHALL NOT stop the service, which serves the seed taxonomy until the store can be read. With persistence disabled the registry stays in memory.

#### Scenario: Promoted and exploring tags survive a restart

- **WHEN** an admin promotes a tag, a second candidate is explored but left exploring, and team-ai is restarted
- **THEN** after the restart the promoted tag is canonical (listed as promoted and carried by a classification, with its bound synonym) and the other is still an exploring candidate

#### Scenario: A failed store write fails the promotion

- **WHEN** the taxonomy store cannot be written during `promote`
- **THEN** the call answers 503 and the tag is still only a candidate
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_promote_is_refused_and_unchanged_when_the_store_write_fails. Not verifiable end to end: failing Redis would break every service sharing the stack (design.md).

#### Scenario: A store outage at startup serves the seed taxonomy

- **WHEN** the store cannot be read at startup and later recovers
- **THEN** the service starts with the seed taxonomy, and the next `promote` or `explore` loads the stored state before applying its change
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_startup_outage_serves_seed_then_loads_before_the_next_mutation. Not verifiable end to end: same reason (design.md).

#### Scenario: REST and gRPC share one registry

- **WHEN** a tag is promoted through the REST route
- **THEN** gRPC `ClassifyTags` returns it for a matching title, and a restart restores it for both
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_rest_promotion_is_seen_by_grpc_classify_and_survives_a_new_process. Not verifiable end to end: the gateway does not route `ClassifyTags` (internal, service principal only).

#### Scenario: Persistence is off unless enabled

- **WHEN** `TAXONOMY_PERSISTENCE_ENABLED` is false
- **THEN** nothing is written to Redis and a restart returns to the seed taxonomy, and enabling it without `REDIS_ENABLED` or on the same Redis database as `REDIS_DATABASE` fails startup
- **VERIFIED BY**: team-ai/tests/unit/modules/test_tag_taxonomy_persistence.py › test_disabled_persistence_writes_nothing, test_enabling_persistence_requires_redis_on_another_database. Not verifiable end to end: the stack runs with persistence on (design.md).
