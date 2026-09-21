## Tasks

- [x] 1.1 Implement `TagClassifierService` with SPU & SKU hierarchical classification (`team-ai/app/modules/business/tag_classifier/service.py`)
- [x] 1.2 Define Pydantic schemas for `TagItem`, `ClassifyTagsRequest/Response`, `ClassifySkuHierarchyRequest/Response`, `ExploreTagsRequest/Response`, `PromoteTagRequest/Response` (`team-ai/app/modules/business/tag_classifier/schemas.py`)
- [x] 1.3 Add FastAPI REST endpoints in `team-ai/app/api/v1/ai/router.py`
- [x] 1.4 Implement offline candidate exploration & promotion gating logic
- [x] 1.5 Implement OpenSearch 2.11 nested document payload generator for SKU-level filtering
- [x] 1.6 Add unit and API test suites in `team-ai/tests/unit/modules/test_tag_classifier*.py`
- [x] 1.7 Add runnable CLI MLOps pipeline tool in `platform-core/tools/tag_taxonomy_pipeline.py`
- [x] 1.8 Add full architecture documentation in `platform-core/docs/TAG_CLASSIFIER_AND_FILTER_ENRICHMENT.md`
