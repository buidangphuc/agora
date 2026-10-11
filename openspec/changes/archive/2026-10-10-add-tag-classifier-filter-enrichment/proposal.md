## Why

In an e-commerce marketplace (Shopee / Amazon scale), sellers frequently omit structured technical attributes, materials, and specification tags (e.g. *Bluetooth 5.3*, *Inox 304*, *UPF50+*, *512GB*), resulting in sparse product catalog metadata. Furthermore, search discovery suffers because buyers rely on granular faceted filters (`filter.connectivity`, `filter.capacity`, `filter.color`, `filter.material`), but without standardized taxonomy tags at both the parent (SPU) and variant (SKU) levels, search recall drops and filters produce inaccurate cross-variant results.

This change introduces a comprehensive **Two-Stage Tag Classifier, Discovery & Filter Taxonomy Enrichment System**:
1. **Online Fast Inference:** SPU & SKU-level tag classification providing 1-click auto-tagging for sellers in `<5ms`.
2. **Offline Exploration:** Continuous extraction and clustering of emergent candidate tags from uncataloged listings and search queries.
3. **Promotion Gating:** Statistical criteria ($N_{\min} \ge 5, \theta_{\text{conf}} \ge 0.80$) to elevate candidate tags to **Official Canonical Filter Facets** in OpenSearch.
4. **Granular SKU Nested Filtering:** OpenSearch 2.11 nested document structure matching exact in-stock variants without cross-variant false positives.

## What Changes

- **team-ai** (`app/modules/business/tag_classifier/`):
  - `TagClassifierService`: SPU & SKU hierarchical classification, pattern extraction, synonym indexing, offline exploration, and promotion gating.
  - REST endpoints: `POST /api/v1/ai/tags/classify`, `POST /api/v1/ai/tags/classify-sku-hierarchy`, `POST /api/v1/ai/tags/explore`, `POST /api/v1/ai/tags/promote`, `GET /api/v1/ai/tags`.
  - Comprehensive unit and API test suites in `team-ai/tests/unit/modules/`.
- **platform-core**:
  - `tools/tag_taxonomy_pipeline.py`: Standalone CLI demonstrating the complete 4-phase MLOps pipeline.
  - `docs/TAG_CLASSIFIER_AND_FILTER_ENRICHMENT.md`: End-to-end architecture specification, sequence diagrams, and mathematical formulation.
- **team-search & team-frontend**:
  - Dynamic facet aggregation mapping supporting SPU common tags and nested SKU variant facets.

## Non-goals

- No hardcoded cross-database SQL queries (Rule 3) — taxonomy synchronization flows through gRPC APIs and event envelopes.
