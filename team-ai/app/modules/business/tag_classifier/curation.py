"""Candidate exploration and promotion; callers hold the registry lock."""

from collections import Counter
from dataclasses import dataclass

from loguru import logger

from app.modules.business.tag_classifier.extractors import extract_emergent_patterns
from app.modules.business.tag_classifier.matching import strip_accents
from app.modules.business.tag_classifier.registry import TaxonomyRegistry
from app.modules.business.tag_classifier.schemas import (
    ExploreTagsRequest,
    ExploreTagsResponse,
    FacetGroup,
    PromoteTagRequest,
    TagItem,
    TagStatus,
)


@dataclass(frozen=True)
class Exploration:
    response: ExploreTagsResponse
    registered: list[str]


@dataclass(frozen=True)
class Promotion:
    """The outcome of a promote request, computed on copies and not yet applied."""

    tags: dict[str, TagItem]
    from_candidate: list[str]
    promoted: list[TagItem]


def _add_synonyms(tag: TagItem, synonyms: list[str]) -> None:
    for synonym in synonyms:
        if synonym not in tag.synonyms:
            tag.synonyms.append(synonym)


def explore(registry: TaxonomyRegistry, request: ExploreTagsRequest) -> Exploration:
    """Mine listings and their variants; register candidates seen often enough."""
    initial_candidate_count = len(registry.candidates)
    extracted_slugs: Counter[str] = Counter()
    candidate_map: dict[str, TagItem] = {}

    for item in request.batch_listings:
        texts = [
            (f"{item.title} {item.description}"),
            *(
                f"{var.name} {' '.join(var.options.values())} {var.sku_code}"
                for var in item.variants
            ),
        ]
        for text in texts:
            for tag in extract_emergent_patterns(strip_accents(text), item.category_id):
                extracted_slugs[tag.slug] += 1
                if tag.slug not in candidate_map:
                    candidate_map[tag.slug] = tag
                else:
                    candidate_map[tag.slug].occurrence_count += 1

    registered: list[str] = []
    for slug, freq in extracted_slugs.items():
        if freq >= request.min_frequency:
            tag = candidate_map[slug]
            if tag.confidence >= request.min_confidence:
                registry.register_candidate(tag)
                registered.append(slug)

    discovered = [
        registry.candidates[slug]
        for slug in extracted_slugs
        if slug in registry.candidates
    ]
    response = ExploreTagsResponse(
        total_processed=len(request.batch_listings),
        discovered_candidates=discovered,
        clusters_found=len(extracted_slugs),
        new_candidate_count=len(registry.candidates) - initial_candidate_count,
    )
    return Exploration(response=response, registered=registered)


def plan_promotion(registry: TaxonomyRegistry, request: PromoteTagRequest) -> Promotion:
    """Resolve each slug to its promoted tag without touching the registry."""
    working: dict[str, TagItem] = {}
    from_candidate: list[str] = []
    promoted: list[TagItem] = []

    for slug in request.tag_slugs:
        if slug in working:
            tag = working[slug]
            _add_synonyms(tag, request.add_synonyms)
        elif slug in registry.candidates:
            tag = registry.candidates[slug].model_copy(deep=True)
            tag.status = TagStatus.PROMOTED
            tag.is_canonical = True
            tag.confidence = 1.0
            if request.target_category_id:
                tag.category_id = request.target_category_id
            if request.target_facet_group:
                tag.facet_group = request.target_facet_group
            _add_synonyms(tag, request.add_synonyms)
            working[slug] = tag
            from_candidate.append(slug)
            logger.info(
                "Promoted candidate tag '{}' to canonical facet ({})",
                slug,
                tag.facet_group.value,
            )
        elif slug in registry.canonical:
            tag = registry.canonical[slug].model_copy(deep=True)
            _add_synonyms(tag, request.add_synonyms)
            working[slug] = tag
        else:
            tag = TagItem(
                tag_id=f"tag-{slug}",
                name=slug.replace("-", " ").title(),
                slug=slug,
                facet_group=request.target_facet_group or FacetGroup.FEATURE,
                category_id=request.target_category_id or "all",
                status=TagStatus.PROMOTED,
                confidence=1.0,
                is_canonical=True,
                synonyms=list(dict.fromkeys(request.add_synonyms)),
            )
            working[slug] = tag
        promoted.append(tag)

    return Promotion(tags=working, from_candidate=from_candidate, promoted=promoted)


def apply_promotion(registry: TaxonomyRegistry, promotion: Promotion) -> None:
    for slug, tag in promotion.tags.items():
        registry.drop_candidate(slug)
        registry.register_canonical(tag)
