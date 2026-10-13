"""Tag classification service: read-only classify, locked and persisted curation."""

from loguru import logger

from app.core.errors import ServiceUnavailableError
from app.modules.business.tag_classifier.classifier import (
    classify_sku_hierarchy,
    classify_spu,
)
from app.modules.business.tag_classifier.curation import (
    apply_promotion,
    explore,
    plan_promotion,
)
from app.modules.business.tag_classifier.registry import TaxonomyRegistry
from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifySkuHierarchyResponse,
    ClassifyTagsRequest,
    ClassifyTagsResponse,
    ExploreTagsRequest,
    ExploreTagsResponse,
    ListTagsRequest,
    ListTagsResponse,
    PromoteTagRequest,
    PromoteTagResponse,
    TagItem,
)
from app.modules.business.tag_classifier.seed import SEED_CANONICAL_TAGS
from app.modules.business.tag_classifier.store import TaxonomyStore


class TagClassifierService:
    """Online SPU/SKU classification plus offline explore and promote curation."""

    def __init__(self, *, store: TaxonomyStore | None) -> None:
        self._store = store
        self._store_loaded = False
        self._registry = TaxonomyRegistry(
            TagItem(**item) for item in SEED_CANONICAL_TAGS
        )

    async def restore(self) -> None:
        """Overlay the saved taxonomy on the seed; a failure is retried before the next mutation."""
        async with self._registry.lock:
            await self._load_from_store()

    async def _load_from_store(self) -> bool:
        if self._store is None or self._store_loaded:
            return True
        try:
            canonical, candidates = await self._store.load()
        except Exception as exc:
            logger.error("taxonomy.load_failed error={}", type(exc).__name__)
            return False
        self._registry.apply_overlay(canonical, candidates)
        self._store_loaded = True
        logger.info(
            "taxonomy.loaded canonical={} candidates={}",
            len(canonical),
            len(candidates),
        )
        return True

    def classify_tags(self, request: ClassifyTagsRequest) -> ClassifyTagsResponse:
        return classify_spu(self._registry, request)

    def classify_sku_hierarchy(
        self, request: ClassifySkuHierarchyRequest
    ) -> ClassifySkuHierarchyResponse:
        return classify_sku_hierarchy(self._registry, request)

    def list_tags(self, request: ListTagsRequest) -> ListTagsResponse:
        registry = self._registry
        filtered = [*registry.canonical.values(), *registry.candidates.values()]
        if request.category_id:
            filtered = [
                t for t in filtered if t.category_id in ("all", request.category_id)
            ]
        if request.facet_group:
            filtered = [t for t in filtered if t.facet_group == request.facet_group]
        if request.status:
            filtered = [t for t in filtered if t.status == request.status]
        return ListTagsResponse(
            tags=filtered,
            total=len(filtered),
            canonical_count=len(registry.canonical),
            candidate_count=len(registry.candidates),
        )

    async def explore_tags(self, request: ExploreTagsRequest) -> ExploreTagsResponse:
        async with self._registry.lock:
            await self._load_from_store()
            outcome = explore(self._registry, request)
            await self._persist_candidates(outcome.registered)
        logger.info(
            "Explore tags completed: processed {} listings, found {} candidates (new: {})",
            len(request.batch_listings),
            len(outcome.response.discovered_candidates),
            outcome.response.new_candidate_count,
        )
        return outcome.response

    async def promote_tags(self, request: PromoteTagRequest) -> PromoteTagResponse:
        """Persist first, then apply: a store failure leaves the registry unchanged."""
        async with self._registry.lock:
            if not await self._load_from_store():
                raise ServiceUnavailableError("tag taxonomy store unavailable")
            promotion = plan_promotion(self._registry, request)
            if self._store is not None and promotion.tags:
                try:
                    await self._store.save(
                        canonical=list(promotion.tags.values()),
                        drop_candidates=promotion.from_candidate,
                    )
                except Exception as exc:
                    logger.error(
                        "taxonomy.save_failed op=promote error={}", type(exc).__name__
                    )
                    raise ServiceUnavailableError(
                        "tag taxonomy store unavailable"
                    ) from exc
            apply_promotion(self._registry, promotion)
            return PromoteTagResponse(
                promoted_tags=promotion.promoted,
                total_promoted=len(promotion.promoted),
                active_canonical_count=len(self._registry.canonical),
            )

    async def _persist_candidates(self, slugs: list[str]) -> None:
        """Best effort: candidates are re-discoverable, so a store failure is only logged."""
        if self._store is None:
            return
        tags = [
            self._registry.candidates[s]
            for s in dict.fromkeys(slugs)
            if s in self._registry.candidates
        ]
        if not tags:
            return
        try:
            await self._store.save(candidates=tags)
        except Exception as exc:
            logger.warning(
                "taxonomy.save_failed op=explore error={}", type(exc).__name__
            )
