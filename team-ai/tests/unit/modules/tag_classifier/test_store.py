"""RedisTaxonomyStore keeps team-ai's Redis format; the service restores it over the seed."""

from collections.abc import Awaitable
from typing import cast

import fakeredis
import pytest
from fakeredis import FakeAsyncRedis, FakeServer

from app.modules.business.tag_classifier.schemas import (
    ExploreTagsRequest,
    FacetGroup,
    ListTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    TagItem,
    TagStatus,
)
from app.modules.business.tag_classifier.service import TagClassifierService
from app.modules.business.tag_classifier.store import RedisTaxonomyStore

SLUG = "cong-suat-77w"


def _redis(server: FakeServer) -> FakeAsyncRedis:
    return fakeredis.FakeAsyncRedis(server=server, decode_responses=True)


async def _hash(redis: FakeAsyncRedis, key: str) -> dict[str, str]:
    return await cast("Awaitable[dict[str, str]]", redis.hgetall(key))


async def _hset(redis: FakeAsyncRedis, key: str, mapping: dict[str, str]) -> None:
    await cast("Awaitable[int]", redis.hset(key, mapping=mapping))


def _tag(slug: str, *, status: TagStatus = TagStatus.PROMOTED) -> TagItem:
    return TagItem(
        tag_id=f"t-{slug}",
        name=slug.upper(),
        slug=slug,
        facet_group=FacetGroup.POWER,
        category_id="cat-electronics",
        status=status,
        is_canonical=status is TagStatus.PROMOTED,
        synonyms=["a"],
    )


async def test_save_writes_the_v1_hashes_as_slug_to_json():
    server = FakeServer()
    redis = _redis(server)
    store = RedisTaxonomyStore(redis, prefix="tagtax")
    canonical = _tag("c1")
    candidate = _tag("k1", status=TagStatus.EXPLORING)
    await store.save(canonical=[canonical], candidates=[candidate])
    assert await _hash(redis, "tagtax:v1:canonical") == {
        "c1": canonical.model_dump_json()
    }
    assert await _hash(redis, "tagtax:v1:candidates") == {
        "k1": candidate.model_dump_json()
    }


async def test_save_drops_candidates_and_skips_an_empty_save():
    server = FakeServer()
    redis = _redis(server)
    store = RedisTaxonomyStore(redis, prefix="p")
    await store.save(candidates=[_tag("k1"), _tag("k2")])
    await store.save(canonical=[_tag("k1")], drop_candidates=["k1"])
    await store.save()
    assert set(await _hash(redis, "p:v1:candidates")) == {"k2"}
    assert set(await _hash(redis, "p:v1:canonical")) == {"k1"}


async def test_save_runs_in_one_multi_pipeline():
    server = FakeServer()
    redis = _redis(server)
    seen: list[bool] = []
    real = redis.pipeline

    def spy(transaction: bool = True, shard_hint: str | None = None):
        seen.append(transaction)
        return real(transaction=transaction, shard_hint=shard_hint)

    redis.pipeline = spy  # type: ignore[method-assign]
    await RedisTaxonomyStore(redis, prefix="p").save(
        canonical=[_tag("c")], candidates=[_tag("k")], drop_candidates=["z"]
    )
    assert seen == [True]


async def test_load_skips_an_unreadable_entry():
    server = FakeServer()
    redis = _redis(server)
    await _hset(redis, "p:v1:canonical", {"bad": "{not json"})
    store = RedisTaxonomyStore(redis, prefix="p")
    await store.save(canonical=[_tag("good")])
    canonical, candidates = await store.load()
    assert list(canonical) == ["good"]
    assert candidates == {}


async def test_restore_overlays_the_seed_and_survives_a_new_service():
    server = FakeServer()
    first = TagClassifierService(store=RedisTaxonomyStore(_redis(server), prefix="p"))
    await first.restore()
    await first.explore_tags(
        ExploreTagsRequest(
            batch_listings=[
                RawListingItem(listing_id="a", title="Sạc 77W"),
                RawListingItem(listing_id="b", title="Củ sạc 77W 1TB"),
                RawListingItem(listing_id="c", title="Củ sạc 90W 1TB"),
            ],
            min_frequency=2,
            min_confidence=0.8,
        )
    )
    await first.promote_tags(
        PromoteTagRequest(
            tag_slugs=[SLUG], target_category_id="cat-electronics", add_synonyms=["77w"]
        )
    )

    second = TagClassifierService(store=RedisTaxonomyStore(_redis(server), prefix="p"))
    assert second.list_tags(ListTagsRequest()).total == 25  # seed until restored
    await second.restore()

    promoted = {
        t.slug: t
        for t in second.list_tags(ListTagsRequest(status=TagStatus.PROMOTED)).tags
    }
    assert SLUG in promoted
    assert promoted[SLUG].synonyms == ["77w"]
    assert len(promoted) == 26  # seed + the promoted tag
    exploring = [
        t.slug
        for t in second.list_tags(ListTagsRequest(status=TagStatus.EXPLORING)).tags
    ]
    assert exploring == ["bo-nho-1tb"]


async def test_restore_failure_serves_the_seed():
    class Broken:
        async def load(self):
            raise ConnectionError("down")

        async def save(self, **_: object) -> None:
            raise ConnectionError("down")

    service = TagClassifierService(store=Broken())
    await service.restore()
    assert service.list_tags(ListTagsRequest()).total == 25


@pytest.mark.parametrize("prefix", ["tagtax", "other"])
async def test_prefix_namespaces_the_keys(prefix: str):
    redis = _redis(FakeServer())
    await RedisTaxonomyStore(redis, prefix=prefix).save(canonical=[_tag("c")])
    assert await redis.keys("*") == [f"{prefix}:v1:canonical"]
