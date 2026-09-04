"""Two-stage recommend pipeline — pure, no proto, no real Qdrant/Redis.

Uses in-memory fakes so the retrieval/ranking/cache logic is exercised offline.
The gRPC servicer (which imports generated ``recommendation_pb2``) is not tested
here — it runs only in CI once the stubs exist; see the module docstring in
``app/transport/grpc/servicers/recommend.py`` and its search.py sibling.
"""

from __future__ import annotations

import asyncio

import pytest

from app.core.errors import ServiceUnavailableError
from app.modules.business.recommend.cache import PrecomputedCache
from app.modules.business.recommend.schemas import Candidate, RecommendQuery
from app.modules.business.recommend.service import RecommendationService


class FakeBackend:
    def __init__(
        self,
        *,
        similar: dict[str, list[Candidate]] | None = None,
        popular: list[Candidate] | None = None,
        raise_on_retrieve: bool = False,
        retrieve_delay: float = 0.0,
    ) -> None:
        self._similar = similar or {}
        self._popular = popular or []
        self._raise_on_retrieve = raise_on_retrieve
        self._retrieve_delay = retrieve_delay
        self.retrieve_calls: list[str] = []
        self.popular_calls = 0

    async def retrieve_similar(self, seed_listing_id, *, top_k):
        self.retrieve_calls.append(seed_listing_id)
        if self._retrieve_delay:
            await asyncio.sleep(self._retrieve_delay)
        if self._raise_on_retrieve:
            raise RuntimeError("qdrant down")
        return list(self._similar.get(seed_listing_id, [])[:top_k])

    async def popular(self, *, top_k):
        self.popular_calls += 1
        return list(self._popular[:top_k])

    async def collection_ok(self):
        return True


class FakeRedis:
    """Minimal async redis stub: preset values per key, optional raise."""

    def __init__(self, values: dict[str, str] | None = None, *, raise_on_get=False):
        self._values = values or {}
        self._raise = raise_on_get
        self.get_calls: list[str] = []

    async def get(self, key):
        self.get_calls.append(key)
        if self._raise:
            raise ConnectionError("redis unreachable")
        return self._values.get(key)


def _cache(redis=None) -> PrecomputedCache:
    return PrecomputedCache(redis, prefix="recs", schema_version="v1")


def _service(backend, cache, **kw) -> RecommendationService:
    return RecommendationService(
        backend=backend,
        cache=cache,
        candidate_top_k=100,
        result_top_k=10,
        retrieve_timeout_ms=kw.pop("retrieve_timeout_ms", 50),
        model_version="test-v1",
        **kw,
    )


async def test_cache_hit_skips_qdrant():
    redis = FakeRedis({"recs:v1:user:u1": '["listing-1","listing-2","listing-3"]'})
    backend = FakeBackend(similar={"seed": [Candidate("x", 1.0)]})
    svc = _service(backend, _cache(redis))

    result = await svc.recommend(RecommendQuery(user_id="u1", seed_listing_id="seed"))

    assert result.source == "cache"
    assert [i.listing_id for i in result.items] == [
        "listing-1",
        "listing-2",
        "listing-3",
    ]
    assert result.items[0].rank == 1
    assert result.model_version == "test-v1"
    # No Qdrant round trip on a cache hit.
    assert backend.retrieve_calls == []


async def test_cache_miss_falls_back_to_qdrant():
    redis = FakeRedis({})  # no cached list for u1
    backend = FakeBackend(
        similar={"seed": [Candidate("listing-9", 0.9), Candidate("listing-8", 0.8)]}
    )
    svc = _service(backend, _cache(redis))

    result = await svc.recommend(RecommendQuery(user_id="u1", seed_listing_id="seed"))

    assert result.source == "ann"
    assert backend.retrieve_calls == ["seed"]
    assert [i.listing_id for i in result.items] == ["listing-9", "listing-8"]


async def test_redis_error_is_fail_open_and_falls_back():
    redis = FakeRedis(raise_on_get=True)
    backend = FakeBackend(similar={"seed": [Candidate("listing-9", 0.9)]})
    svc = _service(backend, _cache(redis))

    result = await svc.recommend(RecommendQuery(user_id="u1", seed_listing_id="seed"))

    assert result.source == "ann"  # cache error treated as a miss, RPC not failed
    assert [i.listing_id for i in result.items] == ["listing-9"]


async def test_out_of_stock_dedupe_and_seed_are_filtered():
    backend = FakeBackend(
        similar={
            "seed": [
                Candidate("seed", 1.0),  # the request's own seed -> dropped
                Candidate("listing-1", 0.9),
                Candidate("listing-1", 0.5),  # duplicate -> dropped (keep best)
                Candidate("listing-2", 0.8, in_stock=False),  # out of stock -> dropped
                Candidate("listing-3", 0.7),
            ]
        }
    )
    svc = _service(backend, _cache(None))

    result = await svc.recommend(
        RecommendQuery(anonymous_id="anon", seed_listing_id="seed")
    )

    ids = [i.listing_id for i in result.items]
    assert ids == ["listing-1", "listing-3"]  # in-stock, distinct, seed excluded
    assert result.items[0].score == pytest.approx(0.9)  # kept the higher dup score
    assert [i.rank for i in result.items] == [1, 2]


async def test_anonymous_with_seed_returns_similar_items():
    backend = FakeBackend(
        similar={"seed": [Candidate("listing-1", 0.9), Candidate("seed", 1.0)]}
    )
    redis = FakeRedis({"recs:v1:user:u1": '["should-not-read"]'})
    svc = _service(backend, _cache(redis))

    result = await svc.recommend(
        RecommendQuery(user_id="", anonymous_id="anon", seed_listing_id="seed")
    )

    assert result.source == "ann"
    assert redis.get_calls == []  # anonymous skips the user cache read entirely
    assert [i.listing_id for i in result.items] == ["listing-1"]  # seed excluded


async def test_no_user_no_seed_falls_back_to_popular():
    backend = FakeBackend(popular=[Candidate("pop-1", 5.0), Candidate("pop-2", 4.0)])
    svc = _service(backend, _cache(None))

    result = await svc.recommend(RecommendQuery())

    assert result.source == "popular"
    assert [i.listing_id for i in result.items] == ["pop-1", "pop-2"]
    assert backend.retrieve_calls == []  # no seed -> no ANN


async def test_popular_prefers_redis_list():
    redis = FakeRedis({"recs:v1:popular": '["hot-1","hot-2"]'})
    backend = FakeBackend(popular=[Candidate("pop-1", 5.0)])
    svc = _service(backend, _cache(redis))

    result = await svc.recommend(RecommendQuery())

    assert result.source == "popular"
    assert [i.listing_id for i in result.items] == ["hot-1", "hot-2"]
    assert backend.popular_calls == 0  # redis popular list preferred over backend


async def test_retrieval_timeout_yields_popular_fallback():
    backend = FakeBackend(
        similar={"seed": [Candidate("listing-1", 0.9)]},
        popular=[Candidate("pop-1", 5.0)],
        retrieve_delay=0.2,  # 200ms > 5ms budget
    )
    svc = _service(backend, _cache(None), retrieve_timeout_ms=5)

    result = await svc.recommend(
        RecommendQuery(anonymous_id="anon", seed_listing_id="seed")
    )

    assert result.source == "popular"  # timed out, degraded to popular
    assert [i.listing_id for i in result.items] == ["pop-1"]


async def test_retrieval_error_is_fail_open():
    backend = FakeBackend(raise_on_retrieve=True, popular=[Candidate("pop-1", 5.0)])
    svc = _service(backend, _cache(None))

    result = await svc.recommend(
        RecommendQuery(anonymous_id="anon", seed_listing_id="seed")
    )

    assert result.source == "popular"  # datastore error -> fallback, not a raise


async def test_collection_mismatch_raises_unavailable():
    backend = FakeBackend(popular=[Candidate("pop-1", 5.0)])
    svc = _service(backend, _cache(None), collection_ok=False)

    with pytest.raises(ServiceUnavailableError):
        await svc.recommend(RecommendQuery(user_id="u1"))


async def test_limit_overrides_result_top_k():
    backend = FakeBackend(
        popular=[Candidate(f"pop-{i}", float(10 - i)) for i in range(10)]
    )
    svc = _service(backend, _cache(None))

    result = await svc.recommend(RecommendQuery(limit=3))

    assert len(result.items) == 3
