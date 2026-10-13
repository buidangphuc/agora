"""Pure classification of an SPU and its SKU variants against a registry snapshot."""

import time

from app.modules.business.tag_classifier.extractors import extract_emergent_patterns
from app.modules.business.tag_classifier.matching import matching_slugs, strip_accents
from app.modules.business.tag_classifier.projection import (
    spu_document,
    variant_document,
)
from app.modules.business.tag_classifier.registry import TaxonomyRegistry
from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifySkuHierarchyResponse,
    ClassifyTagsRequest,
    ClassifyTagsResponse,
    FacetGroup,
    SkuClassificationResult,
    TagItem,
)

VARIANT_FACET_GROUPS = (
    FacetGroup.COLOR,
    FacetGroup.SIZE,
    FacetGroup.CAPACITY,
    FacetGroup.RAM,
    FacetGroup.POWER,
    FacetGroup.MATERIAL,
    FacetGroup.EDITION,
)


def classify_spu(
    registry: TaxonomyRegistry, request: ClassifyTagsRequest
) -> ClassifyTagsResponse:
    """Top-K canonical tags and candidate tags for a title and description.

    Read-only: emergent candidates are returned, never registered.
    """
    start_time = time.perf_counter()
    norm_text = strip_accents(f"{request.title} {request.description}")

    matched_canonical: list[tuple[TagItem, float]] = []
    matched_candidates: list[tuple[TagItem, float]] = []

    matched_slugs = matching_slugs(registry.synonym_matchers(), norm_text)
    canonical_slugs = set(matched_slugs)
    for slug in matched_slugs:
        tag = registry.canonical[slug]
        cat_boost = 1.0
        if request.category_id and tag.category_id not in (
            "all",
            request.category_id,
        ):
            cat_boost = 0.6
        matched_canonical.append((tag, tag.confidence * cat_boost))

    if request.include_candidates:
        for slug in matching_slugs(registry.candidate_matchers(), norm_text):
            if slug in canonical_slugs:
                continue
            cand = registry.candidates[slug]
            matched_candidates.append((cand, cand.confidence * 0.85))

        for cand in extract_emergent_patterns(norm_text, request.category_id):
            if cand.slug not in canonical_slugs and cand.slug not in [
                c.slug for c, _ in matched_candidates
            ]:
                matched_candidates.append((cand, cand.confidence * 0.8))

    matched_canonical.sort(key=lambda x: x[1], reverse=True)
    matched_candidates.sort(key=lambda x: x[1], reverse=True)

    final_canonical = [item[0] for item in matched_canonical[: request.top_k]]
    final_candidates = [item[0] for item in matched_candidates[: request.top_k]]

    facet_filters: dict[str, list[str]] = {}
    for tag in final_canonical:
        facet_filters.setdefault(tag.facet_group.value, []).append(tag.slug)

    latency_ms = (time.perf_counter() - start_time) * 1000.0

    return ClassifyTagsResponse(
        canonical_tags=final_canonical,
        candidate_tags=final_candidates,
        suggested_facet_filters=facet_filters,
        category_id=request.category_id or "cat-general",
        execution_time_ms=round(latency_ms, 2),
    )


def classify_sku_hierarchy(
    registry: TaxonomyRegistry, request: ClassifySkuHierarchyRequest
) -> ClassifySkuHierarchyResponse:
    """SPU common tags once, then each variant's color/size/capacity-style facets."""
    start_time = time.perf_counter()

    spu_res = classify_spu(
        registry,
        ClassifyTagsRequest(
            title=request.spu_title,
            description=request.spu_description,
            category_id=request.category_id,
            top_k=10,
            include_candidates=False,
        ),
    )
    spu_tags = spu_res.canonical_tags
    spu_facet_map: dict[str, set[str]] = {
        k: set(v) for k, v in spu_res.suggested_facet_filters.items()
    }

    sku_results: list[SkuClassificationResult] = []
    variant_docs: list[dict[str, object]] = []

    for v in request.variants:
        options_text = " ".join(f"{k} {val}" for k, val in v.options.items())
        norm_v_text = strip_accents(f"{v.name} {options_text} {v.sku_code}")

        sku_specific_tags: list[TagItem] = []
        variant_facets: dict[str, str] = {}
        for slug in matching_slugs(registry.synonym_matchers(), norm_v_text):
            tag = registry.canonical[slug]
            if tag.facet_group not in VARIANT_FACET_GROUPS:
                continue
            sku_specific_tags.append(tag)
            variant_facets[tag.facet_group.value] = tag.slug
            spu_facet_map.setdefault(tag.facet_group.value, set()).add(tag.slug)

        all_effective = list(
            {t.slug: t for t in (spu_tags + sku_specific_tags)}.values()
        )

        sku_res = SkuClassificationResult(
            variant_id=v.variant_id or f"var-{len(sku_results) + 1}",
            sku_code=v.sku_code,
            name=v.name,
            price=v.price,
            stock=v.stock,
            is_in_stock=(v.stock > 0),
            sku_specific_tags=sku_specific_tags,
            inherited_spu_tags=spu_tags,
            all_effective_tags=all_effective,
            variant_facets=variant_facets,
        )
        sku_results.append(sku_res)
        variant_docs.append(variant_document(sku_res))

    final_spu_facets = {k: sorted(v) for k, v in spu_facet_map.items()}
    latency_ms = (time.perf_counter() - start_time) * 1000.0

    return ClassifySkuHierarchyResponse(
        spu_title=request.spu_title,
        category_id=request.category_id or "cat-general",
        spu_canonical_tags=spu_tags,
        sku_results=sku_results,
        spu_facet_filters=final_spu_facets,
        nested_opensearch_doc=spu_document(
            request, spu_tags, final_spu_facets, variant_docs
        ),
        total_skus_processed=len(request.variants),
        execution_time_ms=round(latency_ms, 2),
    )
