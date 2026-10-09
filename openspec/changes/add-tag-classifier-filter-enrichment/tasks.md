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

- [ ] 2.1 team-search: additive mapping (`facet_tags`, nested `skus`, `tags_pending`), `tag.*`/`sku.*` filter clauses and dynamic facet aggregations (`internal/index/attributes.go`)
- [ ] 2.2 team-search: classify listings through team-ai at index time, `TAG_CLASSIFIER_URL`, outage keeps stored tags (`internal/taxonomy`, `internal/consumer/listing.go`)
- [ ] 2.3 team-search: validate `tag.*`/`sku.*` filters in the handler (`internal/handler/visibility.go`)
- [ ] 2.4 platform-core: additive `AttributeFacet` + `Facets.tags/skus` in `search.proto` (design D4), vendor into team-search/team-frontend/team-gateway, then map `index.Facets.Tags/SKUs` in `toFacets`
- [ ] 2.5 team-frontend: URL-driven dynamic facet groups and active-filter chips on `/search`
- [ ] 2.6 deploy: `TAG_CLASSIFIER_URL=http://team-ai-svc:8000` on `team-search-indexer` in the root compose / gitops values; replay `listing.events` to backfill
- [ ] 2.7 platform-e2e: scenarios of the search requirements (`mls_` steps)
