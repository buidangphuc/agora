import asyncio
import time
from collections.abc import Callable
from typing import Any

import grpc
import pytest
import qdrant_client
from fakeredis import aioredis
from loguru import logger
from qdrant_client import models

from app.modules.business.recommendation.adapters.qdrant import QdrantCandidateSource
from app.modules.business.recommendation.adapters.redis_cache import RedisServingCache
from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.contract.qdrant import point_id
from app.modules.business.recommendation.ports import StockEntry, StockLookupError
from app.modules.business.recommendation.schemas import (
    Candidate,
    RecommendationQuery,
    RecommendationResult,
)
from app.transport.grpc._pb.platform.search.v1 import search_pb2, search_pb2_grpc
from tests.unit.recs_support import (
    FakeStock,
    Source,
    build_service,
    popular_entry,
    seed_popular,
    write_placements,
)

_IN_STOCK = """\
  p:
    candidate_ladder:
      - strategy: seed_vector_similarity
    ranking:
      model: cosine_rank
    filters:
      in_stock_only: true
"""

_NO_FILTER = """\
  p:
    candidate_ladder:
      - strategy: seed_vector_similarity
    ranking:
      model: cosine_rank
"""

_POPULAR = """\
  p:
    candidate_ladder:
      - strategy: global_popular
    ranking:
      model: cosine_rank
    filters:
      in_stock_only: true
"""

_TWO_STEPS = """\
  p:
    candidate_ladder:
      - strategy: seed_vector_similarity
      - strategy: global_popular
    ranking:
      model: cosine_rank
    filters:
      in_stock_only: true
"""


def _ids(n: int) -> list[str]:
    return [f"l{i:03d}" for i in range(n)]


def _cands(n: int, *, in_stock: bool | None = None) -> list[Candidate]:
    return [
        Candidate(lid, 1.0 - i / 1000, in_stock=in_stock)
        for i, lid in enumerate(_ids(n))
    ]


def _item_ids(result: RecommendationResult) -> list[str]:
    return [i.listing_id for i in result.items]


async def _serve(
    tmp_path: Any,
    stock: FakeStock,
    *,
    cands: list[Candidate] | None = None,
    limit: int = 8,
    body: str = _IN_STOCK,
    budget: float = 1.0,
    redis: Any = None,
    source: Any = None,
) -> RecommendationResult:
    source = source if source is not None else Source({"seed": cands or []})
    service = build_service(
        source,
        redis,
        write_placements(tmp_path, body),
        stock_lookup=stock,
        retrieve_timeout_s=budget,
    )
    query = RecommendationQuery(placement_id="p", seed_listing_id="seed", limit=limit)
    return await asyncio.wait_for(service.recommend(query), 5)


async def test_timeout_fails_closed_within_budget(tmp_path):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(redis, [popular_entry("p1", 0.9), popular_entry("p2", 0.8)])
    stock = FakeStock(hang_ids=_ids(5))
    started = time.monotonic()
    result = await _serve(
        tmp_path, stock, cands=_cands(5), limit=5, budget=0.05, redis=redis
    )
    elapsed = time.monotonic() - started
    assert result.items == []
    assert elapsed <= 0.05 + 0.05
    assert stock.calls != []


async def test_absent_stock_drops_only_that_candidate(tmp_path):
    stock = FakeStock({"l000": None, "l001": 3, "l002": None, "l003": 5}, default=None)
    result = await _serve(tmp_path, stock, cands=_cands(4), limit=4)
    assert _item_ids(result) == ["l001", "l003"]


async def test_stock_boundary_one_in_zero_out(tmp_path):
    stock = FakeStock({"l000": 1, "l001": 0, "l002": -1})
    result = await _serve(tmp_path, stock, cands=_cands(3), limit=3)
    assert _item_ids(result) == ["l000"]


async def test_one_failed_call_drops_all_candidates(tmp_path):
    stock = FakeStock(fail_ids=["l015"])
    result = await _serve(tmp_path, stock, cands=_cands(30), limit=10)
    assert result.items == []
    assert len(stock.calls) == 2


@pytest.mark.parametrize(
    "reply",
    [
        lambda entries: entries[:-1],
        lambda entries: [entries[1], entries[0], *entries[2:]],
        lambda entries: [*entries, StockEntry("extra", 5)],
        lambda entries: [StockEntry("other", e.stock) for e in entries],
    ],
    ids=["short", "reordered", "extra", "foreign_ids"],
)
async def test_malformed_response_empties_the_placement(
    tmp_path, reply: Callable[[list[StockEntry]], list[StockEntry]]
):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(redis, [popular_entry("p1", 0.9)])
    stock = FakeStock(reply=reply)
    result = await _serve(tmp_path, stock, cands=_cands(10), limit=5, redis=redis)
    assert result.items == []


async def test_looks_up_only_limit_times_two_top_ranked_ids(tmp_path):
    stock = FakeStock()
    result = await _serve(tmp_path, stock, cands=_cands(100), limit=8)
    assert stock.looked_up == _ids(16)
    assert 1 <= len(stock.calls) <= 2
    assert all(len(call) <= 10 for call in stock.calls)
    assert _item_ids(result) == _ids(8)
    assert [i.rank for i in result.items] == list(range(1, 9))


async def test_lookup_capped_at_twenty_ids(tmp_path):
    stock = FakeStock()
    result = await _serve(tmp_path, stock, cands=_cands(100), limit=15)
    assert stock.looked_up == _ids(20)
    assert [len(call) for call in stock.calls] == [10, 10]
    assert len(result.items) == 15
    assert _item_ids(result) == _ids(15)


async def test_refill_from_next_ranked_candidates(tmp_path):
    zero = dict.fromkeys(_ids(12), 0)
    stock = FakeStock(zero)
    result = await _serve(tmp_path, stock, cands=_cands(40), limit=8)
    assert _item_ids(result) == _ids(20)[12:]
    looked_up = stock.looked_up
    assert len(looked_up) > 16
    assert looked_up == _ids(40)[: len(looked_up)]
    assert len(set(looked_up)) == len(looked_up)
    assert all(len(call) <= 10 for call in stock.calls)


async def test_refill_stops_when_candidates_are_exhausted(tmp_path):
    stock = FakeStock()
    result = await _serve(tmp_path, stock, cands=_cands(5), limit=8)
    assert _item_ids(result) == _ids(5)
    assert stock.calls == [_ids(5)]


async def test_refill_bounded_by_budget(tmp_path):
    zero = dict.fromkeys(_ids(16)[3:], 0)
    stock = FakeStock(zero, hang_ids=_ids(30)[16:])
    started = time.monotonic()
    result = await _serve(tmp_path, stock, cands=_cands(30), limit=8, budget=0.05)
    elapsed = time.monotonic() - started
    assert _item_ids(result) == _ids(3)
    assert elapsed <= 0.05 + 0.05
    assert any(lid in _ids(30)[16:] for lid in stock.looked_up)
    assert all(t <= 0.05 for t in stock.timeouts)


async def test_lookup_memoised_across_ladder_steps(tmp_path):
    redis = aioredis.FakeRedis(decode_responses=True)
    popular = [popular_entry(lid, 0.9 - i / 10) for i, lid in enumerate(_ids(7))]
    await seed_popular(redis, popular)
    stock = FakeStock(dict.fromkeys(_ids(5), 0))
    result = await _serve(
        tmp_path,
        stock,
        cands=_cands(5),
        limit=3,
        body=_TWO_STEPS,
        redis=redis,
    )
    assert _item_ids(result) == ["l005", "l006"]
    assert len(stock.looked_up) == len(set(stock.looked_up))
    assert set(stock.looked_up) == set(_ids(7))


async def test_default_in_stock_does_not_leak(tmp_path):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(redis, [popular_entry("a", 0.9), popular_entry("b", 0.8)])
    cached = await RedisServingCache(
        redis, ServingKeys("recs", "v1")
    ).popular_candidates(None)
    assert [c.in_stock for c in cached] == [None, None]

    client = qdrant_client.AsyncQdrantClient(location=":memory:")
    await client.create_collection(
        collection_name="item_als_vectors",
        vectors_config=models.VectorParams(size=2, distance=models.Distance.COSINE),
    )
    await client.upsert(
        collection_name="item_als_vectors",
        points=[
            models.PointStruct(
                id=point_id(lid),
                vector=vec,
                payload={"listing_id": lid, "visible": True},
            )
            for lid, vec in (("s", [1.0, 0.0]), ("n", [0.9, 0.1]))
        ],
    )
    source = QdrantCandidateSource(
        client,
        collection="item_als_vectors",
        vector_dim=2,
        distance="Cosine",
        timeout_s=1.0,
    )
    hits = await source.similar("s", top_k=5, generation=None)
    assert [(c.listing_id, c.in_stock) for c in hits] == [("n", None)]

    zero = await _serve(
        tmp_path, FakeStock(default=0), body=_POPULAR, redis=redis, source=Source()
    )
    assert zero.items == []
    down = await _serve(
        tmp_path,
        FakeStock(fail_ids=["a", "b"]),
        body=_POPULAR,
        redis=redis,
        source=Source(),
    )
    assert down.items == []


async def test_payload_in_stock_true_is_overwritten_by_live_stock(tmp_path):
    cands = [
        Candidate("a", 0.9, in_stock=True),
        Candidate("b", 0.8, in_stock=False),
        Candidate("c", 0.7, in_stock=True),
    ]
    stock = FakeStock({"a": 0, "b": 4, "c": 2})
    result = await _serve(tmp_path, stock, cands=cands, limit=3)
    assert _item_ids(result) == ["b", "c"]


async def test_no_lookup_without_in_stock_only(tmp_path):
    stock = FakeStock(fail_ids=_ids(10))
    result = await _serve(tmp_path, stock, cands=_cands(10), limit=4, body=_NO_FILTER)
    assert stock.calls == []
    assert _item_ids(result) == _ids(4)


@pytest.mark.parametrize("trigger", ["ranking_raises", "ladder_floor"])
async def test_popular_fallback_is_stock_checked(tmp_path, trigger):
    class _Raises(Source):
        async def similar(self, seed_listing_id, *, top_k, generation):
            raise RuntimeError("ranking input failed")

    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(
        redis,
        [popular_entry("p1", 0.9), popular_entry("p2", 0.8), popular_entry("p3", 0.7)],
    )
    source = _Raises() if trigger == "ranking_raises" else Source()
    result = await _serve(
        tmp_path, FakeStock({"p2": 0}), redis=redis, source=source, limit=3
    )
    assert _item_ids(result) == ["p1", "p3"]
    down = await _serve(
        tmp_path, FakeStock(fail_ids=["p1"]), redis=redis, source=source, limit=3
    )
    assert down.items == []


class _Search(search_pb2_grpc.SearchServiceServicer):
    def __init__(
        self,
        stock: dict[str, int] | None = None,
        abort: grpc.StatusCode | None = None,
    ) -> None:
        self.stock = stock or {}
        self.abort = abort
        self.metadata: list[dict[str, str]] = []
        self.requests: list[list[str]] = []

    async def GetListingStock(self, request, context):
        self.metadata.append(dict(context.invocation_metadata()))
        self.requests.append(list(request.listing_ids))
        if self.abort is not None:
            await context.abort(self.abort, "rejected")
        reply = search_pb2.GetListingStockResponse()
        for lid in request.listing_ids:
            entry = reply.listings.add(listing_id=lid)
            if lid in self.stock:
                entry.stock = self.stock[lid]
        return reply


async def _search_lookup(servicer: _Search, **kw: Any):
    from app.modules.business.recommendation.adapters.search_stock import (
        GrpcStockLookup,
    )

    server = grpc.aio.server()
    search_pb2_grpc.add_SearchServiceServicer_to_server(servicer, server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = grpc.aio.insecure_channel(f"127.0.0.1:{port}")
    lookup = GrpcStockLookup(
        channel,
        principal_id=kw.pop("principal_id", "team-ai"),
        principal_scopes=kw.pop("principal_scopes", ("search:read",)),
    )
    return server, channel, lookup


async def test_lookup_carries_service_principal_with_search_read():
    servicer = _Search({"a": 3})
    server, channel, lookup = await _search_lookup(servicer, principal_id="team-ai-x")
    try:
        entries = await lookup.lookup(["a", "b"], timeout_s=2.0)
    finally:
        await channel.close()
        await server.stop(None)
    assert servicer.requests == [["a", "b"]]
    assert [(e.listing_id, e.stock) for e in entries] == [("a", 3), ("b", None)]
    [md] = servicer.metadata
    assert md["x-principal-type"] == "service"
    assert md["x-principal-id"] == "team-ai-x"
    scopes = {s.strip() for s in md["x-principal-scopes"].split(",")}
    assert scopes == {"search:read"}


@pytest.mark.parametrize(
    "code", [grpc.StatusCode.UNAUTHENTICATED, grpc.StatusCode.PERMISSION_DENIED]
)
async def test_lookup_carries_service_principal_rejection_raises_lookup_error(code):
    servicer = _Search(abort=code)
    server, channel, lookup = await _search_lookup(servicer)
    try:
        with pytest.raises(StockLookupError):
            await lookup.lookup(["a"], timeout_s=2.0)
    finally:
        await channel.close()
        await server.stop(None)


@pytest.mark.parametrize("outcome", ["ok", "timeout", "error"])
async def test_logs_carry_outcome_and_no_ids(tmp_path, outcome):
    ids = _ids(5)
    stock = {
        "ok": FakeStock(),
        "timeout": FakeStock(hang_ids=ids),
        "error": FakeStock(fail_ids=ids),
    }[outcome]
    lines: list[str] = []
    sink = logger.add(
        lambda m: lines.append(str(m)),
        format="{message} | {extra}",
        level="DEBUG",
    )
    try:
        await _serve(tmp_path, stock, cands=_cands(5), limit=5, budget=0.05)
    finally:
        logger.remove(sink)
    text = "\n".join(lines)
    assert f"outcome={outcome}" in text or f"'outcome': '{outcome}'" in text
    assert "candidates=5" in text or "'candidates': 5" in text
    for listing_id in ids:
        assert listing_id not in text
