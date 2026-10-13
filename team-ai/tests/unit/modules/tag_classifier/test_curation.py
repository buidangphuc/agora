"""Read-only classify, serialized and persisted curation."""

from collections.abc import Sequence

import pytest

from app.core.errors import ServiceUnavailableError
from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest,
    ExploreTagsRequest,
    ListTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    SkuVariantInput,
    TagItem,
    TagStatus,
)
from app.modules.business.tag_classifier.service import TagClassifierService

SLUG = "cong-suat-77w"


class MemoryStore:
    def __init__(self, *, fail_save: bool = False, fail_load: bool = False) -> None:
        self.fail_save = fail_save
        self.fail_load = fail_load
        self.loads = 0
        self.canonical: dict[str, TagItem] = {}
        self.candidates: dict[str, TagItem] = {}

    async def load(self) -> tuple[dict[str, TagItem], dict[str, TagItem]]:
        self.loads += 1
        if self.fail_load:
            raise ConnectionError("redis down")
        return dict(self.canonical), dict(self.candidates)

    async def save(
        self,
        *,
        canonical: Sequence[TagItem] = (),
        candidates: Sequence[TagItem] = (),
        drop_candidates: Sequence[str] = (),
    ) -> None:
        if self.fail_save:
            raise ConnectionError("redis down")
        self.canonical.update({t.slug: t for t in canonical})
        self.candidates.update({t.slug: t for t in candidates})
        for slug in drop_candidates:
            self.candidates.pop(slug, None)


def _batch() -> ExploreTagsRequest:
    return ExploreTagsRequest(
        batch_listings=[
            RawListingItem(listing_id="a", title="Sạc GaN công suất 77W"),
            RawListingItem(listing_id="b", title="Củ sạc laptop 77W"),
        ],
        min_frequency=2,
        min_confidence=0.8,
    )


def _exploring(service: TagClassifierService) -> list[TagItem]:
    return service.list_tags(ListTagsRequest(status=TagStatus.EXPLORING)).tags


async def test_repeated_classify_is_stable():
    service = TagClassifierService(store=None)
    request = ClassifyTagsRequest(
        title="Củ sạc 120W pin 20000mAh 256GB", category_id="cat-electronics"
    )
    first = service.classify_tags(request)
    second = service.classify_tags(request)
    assert first.candidate_tags
    assert first.canonical_tags == second.canonical_tags
    assert first.candidate_tags == second.candidate_tags
    assert [t.occurrence_count for t in second.candidate_tags] == [1] * len(
        second.candidate_tags
    )
    assert _exploring(service) == []


async def test_sku_classify_does_not_register_candidates():
    service = TagClassifierService(store=None)
    service.classify_sku_hierarchy(
        ClassifySkuHierarchyRequest(
            spu_title="Sạc 120W",
            variants=[SkuVariantInput(name="256GB 65w")],
        )
    )
    assert _exploring(service) == []


async def test_promote_with_a_failing_store_leaves_memory_unchanged():
    store = MemoryStore()
    service = TagClassifierService(store=store)
    await service.restore()
    await service.explore_tags(_batch())
    before = service.list_tags(ListTagsRequest())
    store.fail_save = True
    with pytest.raises(ServiceUnavailableError):
        await service.promote_tags(
            PromoteTagRequest(tag_slugs=[SLUG, "bluetooth-5-3"], add_synonyms=["x77"])
        )
    assert service.list_tags(ListTagsRequest()) == before
    assert [t.slug for t in _exploring(service)] == [SLUG]


async def test_promote_with_an_unreadable_store_is_unavailable():
    store = MemoryStore(fail_load=True)
    service = TagClassifierService(store=store)
    await service.restore()  # logged, not raised: the seed is served
    assert service.list_tags(ListTagsRequest()).canonical_count == 25
    with pytest.raises(ServiceUnavailableError):
        await service.promote_tags(PromoteTagRequest(tag_slugs=["x"]))
    store.fail_load = False
    await service.promote_tags(PromoteTagRequest(tag_slugs=["x"]))
    assert store.loads == 3  # restore, the refused promote, the retry
    assert "x" in store.canonical


async def test_duplicate_synonyms_are_added_once():
    service = TagClassifierService(store=None)
    await service.explore_tags(_batch())
    response = await service.promote_tags(
        PromoteTagRequest(tag_slugs=[SLUG, SLUG], add_synonyms=["sac 77w", "sac 77w"])
    )
    tag = service.list_tags(ListTagsRequest(status=TagStatus.PROMOTED)).tags[-1]
    assert tag.slug == SLUG
    assert tag.synonyms == ["sac 77w"]
    assert response.total_promoted == 2

    existing = await service.promote_tags(
        PromoteTagRequest(tag_slugs=["bluetooth-5-3"], add_synonyms=["bt 5.3", "bt53"])
    )
    assert existing.promoted_tags[0].synonyms.count("bt 5.3") == 1
    assert existing.promoted_tags[0].synonyms[-1] == "bt53"

    unknown = await service.promote_tags(
        PromoteTagRequest(tag_slugs=["moi"], add_synonyms=["a", "a"])
    )
    assert unknown.promoted_tags[0].synonyms == ["a"]


async def test_promote_persists_before_applying_and_drops_the_candidate():
    store = MemoryStore()
    service = TagClassifierService(store=store)
    await service.explore_tags(_batch())
    assert SLUG in store.candidates
    await service.promote_tags(
        PromoteTagRequest(tag_slugs=[SLUG], add_synonyms=["sac 77w"])
    )
    assert SLUG not in store.candidates
    assert store.canonical[SLUG].synonyms == ["sac 77w"]
    assert _exploring(service) == []


async def test_explore_persistence_is_best_effort():
    store = MemoryStore(fail_save=True)
    service = TagClassifierService(store=store)
    response = await service.explore_tags(_batch())
    assert response.new_candidate_count == 1
    assert [t.slug for t in _exploring(service)] == [SLUG]
    assert store.candidates == {}


async def test_explore_persists_registered_candidates():
    store = MemoryStore()
    service = TagClassifierService(store=store)
    await service.explore_tags(_batch())
    assert list(store.candidates) == [SLUG]
    assert store.candidates[SLUG].occurrence_count == 2
