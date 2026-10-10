## Tasks

- [x] 1.1 Implement `TagClassifierService` with SPU & SKU hierarchical classification (`team-ai/app/modules/business/tag_classifier/service.py`)
- [x] 1.2 Define Pydantic schemas for `TagItem`, `ClassifyTagsRequest/Response`, `ClassifySkuHierarchyRequest/Response`, `ExploreTagsRequest/Response`, `PromoteTagRequest/Response` (`team-ai/app/modules/business/tag_classifier/schemas.py`)
- [x] 1.3 Add FastAPI REST endpoints in `team-ai/app/api/v1/ai/router.py`
- [x] 1.4 Implement offline candidate exploration & promotion gating logic
- [x] 1.5 Implement OpenSearch 2.11 nested document payload generator for SKU-level filtering
- [x] 1.6 Add unit and API test suites in `team-ai/tests/unit/modules/test_tag_classifier*.py`
- [x] 1.7 Add runnable CLI MLOps pipeline tool in `platform-core/tools/tag_taxonomy_pipeline.py`
- [x] 1.8 Add full architecture documentation in `platform-core/docs/TAG_CLASSIFIER_AND_FILTER_ENRICHMENT.md`

## 2. Search read-model and facet UI

- [x] 2.1 team-search: additive mapping (`facet_tags`, nested `skus`, `tags_pending`), `tag.*`/`sku.*` filter clauses and dynamic facet aggregations (`internal/index/attributes.go`)
- [x] 2.2 team-search: classify listings through team-ai gRPC `ClassifyTags` at index time (`UPSTREAM_AI_ADDR`), outage keeps stored tags (`internal/taxonomy`, `internal/consumer/listing.go`)
- [x] 2.3 team-search: validate `tag.*`/`sku.*` filters in the handler (`internal/handler/visibility.go`)
- [x] 2.4 platform-core: additive `AttributeFacet` + `Facets.tags/skus` in `search.proto` and `AIService.ClassifyTags` in `ai.proto` (design D1, D4), vendored; team-search `toFacets` maps them
- [x] 2.4b team-ai: `ClassifyTags` servicer gated by scope `ai.classify` + service principal; team-gateway does not route it
- [x] 2.5 team-frontend: URL-driven dynamic facet groups and active-filter chips on `/search`
- [x] 2.6 deploy: `UPSTREAM_AI_ADDR=team-ai-svc:50060` on `team-search-indexer` in the root compose / gitops values; replay `listing.events` to backfill
- [x] 2.7 platform-e2e: scenarios of the search requirements (`mls_` steps)

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.

- Final gate (2026-10-10): parallel lane 775/775 (w10-par) and 775/776 (w9-par; its one failure was the gateway-wide denylist gauge scenario, moved to the serial lane in e8373e00). Destructive lane 100/101 (w9-dfull); its one failure, backpressure, was fixed in 7f4ae454 and 4d325f2f and then passed twice in the outage-then-backpressure order.
