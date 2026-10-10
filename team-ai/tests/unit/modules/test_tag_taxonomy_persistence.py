"""The tag taxonomy registry survives a restart (change tag-taxonomy-persistence)."""

from __future__ import annotations

import fakeredis
import pytest
from fakeredis import FakeServer
from fastapi import FastAPI

from app.bootstrap.resources import ApplicationResources
from app.core.errors import ServiceUnavailableError
from app.modules.business.tag_classifier.factory import TagTaxonomyAddon
from app.modules.business.tag_classifier.schemas import (
    ClassifyTagsRequest,
    ExploreTagsRequest,
    ListTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    TagStatus,
)
from app.modules.business.tag_classifier.service import (
    TagClassifierService,
    shared_tag_classifier,
)
from app.modules.business.tag_classifier.store import RedisTaxonomyStore
from app.modules.platform.identity.schemas import Principal
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2
from app.transport.grpc.context import bind_principal, reset_principal
from app.transport.grpc.servicers.ai import AIServicer
from tests.factories import build_test_settings

SLUG = "cong-suat-77w"


def _redis(server: FakeServer):
    return fakeredis.FakeAsyncRedis(server=server, decode_responses=True)


def _batch() -> ExploreTagsRequest:
    return ExploreTagsRequest(
        batch_listings=[
            RawListingItem(
                listing_id="a",
                title="Sạc nhanh GaN công suất 77W Anker",
                category_id="c",
            ),
            RawListingItem(
                listing_id="b",
                title="Củ sạc laptop công suất 77W Baseus",
                category_id="c",
            ),
        ],
        min_frequency=2,
        min_confidence=0.8,
    )


async def _service(server: FakeServer) -> TagClassifierService:
    """A 'process': a fresh service that attaches the store the previous one wrote."""
    svc = TagClassifierService()
    assert await svc.attach_store(RedisTaxonomyStore(_redis(server)))
    return svc


async def _statuses(svc: TagClassifierService) -> dict[str, TagStatus]:
    return {t.slug: t.status for t in (await svc.list_tags(ListTagsRequest())).tags}


async def test_promoted_and_exploring_tags_survive_a_new_process():
    server = FakeServer()
    first = await _service(server)
    await first.explore_tags(_batch())
    assert (await _statuses(first))[SLUG] == TagStatus.EXPLORING
    await first.promote_tags(
        PromoteTagRequest(
            tag_slugs=[SLUG],
            target_category_id="cat-electronics",
            add_synonyms=["sac 77w"],
        )
    )
    other = ExploreTagsRequest(
        batch_listings=[
            RawListingItem(
                listing_id="c", title="Pin dự phòng công suất 88W", category_id="c"
            ),
            RawListingItem(
                listing_id="d", title="Sạc dự phòng công suất 88W", category_id="c"
            ),
        ],
        min_frequency=2,
        min_confidence=0.8,
    )
    await first.explore_tags(other)

    second = await _service(server)  # restart

    statuses = await _statuses(second)
    assert statuses[SLUG] == TagStatus.PROMOTED
    assert statuses["cong-suat-88w"] == TagStatus.EXPLORING
    res = await second.classify_tags(
        ClassifyTagsRequest(
            title="Củ sạc nhanh công suất 77W", category_id="cat-electronics"
        )
    )
    assert SLUG in [t.slug for t in res.canonical_tags]
    assert (
        "sac 77w"
        in next(
            t
            for t in (await second.list_tags(ListTagsRequest())).tags
            if t.slug == SLUG
        ).synonyms
    )


async def test_a_new_process_without_the_store_returns_to_the_seed():
    first = TagClassifierService()
    await first.explore_tags(_batch())
    await first.promote_tags(PromoteTagRequest(tag_slugs=[SLUG]))

    assert SLUG not in await _statuses(TagClassifierService())


async def test_promote_is_refused_and_unchanged_when_the_store_write_fails():
    class Broken(RedisTaxonomyStore):
        async def save(self, **_):  # type: ignore[override]
            raise ConnectionError("redis down")

    server = FakeServer()
    svc = TagClassifierService()
    await svc.attach_store(Broken(_redis(server)))
    await svc.explore_tags(_batch())

    with pytest.raises(ServiceUnavailableError):
        await svc.promote_tags(PromoteTagRequest(tag_slugs=[SLUG]))

    assert (await _statuses(svc))[SLUG] == TagStatus.EXPLORING
    assert SLUG not in await _statuses(await _service(server))  # nothing was stored


async def test_startup_outage_serves_seed_then_loads_before_the_next_mutation():
    server = FakeServer()
    seeded = await _service(server)
    await seeded.explore_tags(_batch())
    await seeded.promote_tags(PromoteTagRequest(tag_slugs=[SLUG]))

    class Flaky(RedisTaxonomyStore):
        down = True

        async def load(self):
            if self.down:
                raise ConnectionError("redis down")
            return await super().load()

    store = Flaky(_redis(server))
    svc = TagClassifierService()
    assert await svc.attach_store(store) is False
    assert SLUG not in await _statuses(svc)  # seed only, but it serves

    store.down = False
    await svc.promote_tags(PromoteTagRequest(tag_slugs=["x-moi"]))  # loads first

    statuses = await _statuses(svc)
    assert statuses[SLUG] == TagStatus.PROMOTED  # stored state was not written over
    assert statuses["x-moi"] == TagStatus.PROMOTED


async def test_rest_promotion_is_seen_by_grpc_classify_and_survives_a_new_process():
    server = FakeServer()
    rest = await _service(server)  # the one registry both transports use
    await rest.explore_tags(_batch())
    await rest.promote_tags(
        PromoteTagRequest(tag_slugs=[SLUG], target_category_id="cat-electronics")
    )

    async def grpc_tags(svc: TagClassifierService) -> list[str]:
        servicer = AIServicer(tag_classifier_provider=lambda: svc)
        token = bind_principal(
            Principal(id="idx", type="service", scopes=("ai.classify",))
        )
        try:
            resp = await servicer.ClassifyTags(
                ai_pb2.ClassifyTagsRequest(
                    title="Củ sạc nhanh công suất 77W", category_id="cat-electronics"
                ),
                object(),  # type: ignore[arg-type]
            )
        finally:
            reset_principal(token)
        return [t.slug for t in resp.tags]

    assert SLUG in await grpc_tags(rest)
    assert SLUG in await grpc_tags(await _service(server))  # restart


async def test_disabled_persistence_writes_nothing():
    settings = build_test_settings(REDIS_ENABLED=True)
    assert not TagTaxonomyAddon().is_enabled(settings)
    server = FakeServer()
    svc = TagClassifierService()  # no store attached
    await svc.explore_tags(_batch())
    await svc.promote_tags(PromoteTagRequest(tag_slugs=[SLUG]))

    assert await _redis(server).dbsize() == 0
    assert SLUG not in await _statuses(TagClassifierService())


def test_enabling_persistence_requires_redis_on_another_database():
    with pytest.raises(ValueError, match="REDIS_ENABLED"):
        build_test_settings(TAXONOMY_PERSISTENCE_ENABLED=True, REDIS_ENABLED=False)
    with pytest.raises(ValueError, match="differ from REDIS_DATABASE"):
        build_test_settings(
            TAXONOMY_PERSISTENCE_ENABLED=True,
            REDIS_ENABLED=True,
            REDIS_DATABASE=5,
            TAXONOMY_REDIS_DATABASE=5,
        )
    build_test_settings(TAXONOMY_PERSISTENCE_ENABLED=True, REDIS_ENABLED=True)


async def test_store_writes_only_changed_tags_under_the_versioned_keys():
    server = FakeServer()
    svc = await _service(server)
    await svc.explore_tags(_batch())
    await svc.promote_tags(PromoteTagRequest(tag_slugs=[SLUG]))

    raw = _redis(server)
    assert await raw.hkeys("tagtax:v1:canonical") == [SLUG]  # the seed is not copied in
    assert (
        await raw.hkeys("tagtax:v1:candidates") == []
    )  # promoted candidates are removed


async def test_addon_attaches_the_shared_registry_and_detaches(monkeypatch):
    server = FakeServer()
    monkeypatch.setattr(
        "app.modules.business.tag_classifier.factory.build_redis_client",
        lambda settings: _redis(server),
    )
    shared = shared_tag_classifier()
    addon = TagTaxonomyAddon()
    resources = ApplicationResources()
    settings = build_test_settings(
        TAXONOMY_PERSISTENCE_ENABLED=True, REDIS_ENABLED=True
    )
    assert addon.is_enabled(settings)

    await addon.open(FastAPI(), resources, settings)
    try:
        assert resources.tag_classifier_service is shared
        await shared.explore_tags(_batch())
        await shared.promote_tags(PromoteTagRequest(tag_slugs=[SLUG]))
        assert await _redis(server).hexists("tagtax:v1:canonical", SLUG)
    finally:
        await addon.close(FastAPI(), resources)
        shared._canonical_tags.pop(SLUG, None)
        shared._synonym_index = {
            k: v for k, v in shared._synonym_index.items() if v != SLUG
        }

    assert shared._store is None
