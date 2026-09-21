## ADDED Requirements

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
- **THEN** the tag status transitions to `PROMOTED` (`is_canonical = true`), and subsequent search queries dynamically include this facet.
