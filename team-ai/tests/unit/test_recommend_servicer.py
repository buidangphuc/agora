from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import cast

import grpc
import pytest
from loguru import logger

from app.modules.business.recommendation.schemas import (
    RecommendationQuery,
    RecommendationResult,
    RecommendedItem,
)
from app.modules.business.recommendation.service import RecommendationService
from app.transport.grpc._pb.platform.recommendation.v1 import (
    recommendation_pb2 as pb,
    recommendation_pb2_grpc,
)
from app.transport.grpc.server import build_grpc_server
from app.transport.grpc.servicers.recommendation import (
    recommendation_service_registration,
)
from tests.factories import allow_all_limiter

_MD = (
    ("x-principal-id", "u1"),
    ("x-principal-type", "user"),
    ("x-principal-scopes", "listing.read"),
)


class _Fake:
    def __init__(self) -> None:
        self.queries: list[RecommendationQuery] = []

    async def recommend(self, query: RecommendationQuery) -> RecommendationResult:
        self.queries.append(query)
        return RecommendationResult(
            items=[RecommendedItem(listing_id="l1", score=0.9, rank=1)],
            model_version="als-1",
            placement_id="cart_cross_sell",
        )


@asynccontextmanager
async def _serving(
    service: _Fake,
) -> AsyncIterator[recommendation_pb2_grpc.RecommendationServiceStub]:
    server = build_grpc_server(
        registrations=(
            recommendation_service_registration(cast("RecommendationService", service)),
        ),
        rate_limiter=allow_all_limiter(),
    )
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            yield recommendation_pb2_grpc.RecommendationServiceStub(channel)
    finally:
        await server.stop(None)


def _ids(n: int) -> list[str]:
    return [f"ctx-listing-{i:03d}" for i in range(n)]


async def test_context_listing_ids_are_mapped_onto_the_query():
    service = _Fake()
    async with _serving(service) as stub:
        await stub.Recommend(
            pb.RecommendRequest(
                context=pb.RECOMMENDATION_CONTEXT_CART,
                context_listing_ids=["a", "b"],
            ),
            metadata=_MD,
        )
    assert service.queries[0].placement_id == "cart_cross_sell"
    assert list(service.queries[0].context_listing_ids) == ["a", "b"]


async def test_exactly_50_context_ids_are_served():
    service = _Fake()
    async with _serving(service) as stub:
        reply = await stub.Recommend(
            pb.RecommendRequest(context_listing_ids=_ids(50)), metadata=_MD
        )
    assert [i.listing_id for i in reply.items] == ["l1"]
    assert len(service.queries[0].context_listing_ids) == 50


async def test_context_over_the_limit_is_rejected():
    service = _Fake()
    async with _serving(service) as stub:
        with pytest.raises(grpc.aio.AioRpcError) as exc:
            await stub.Recommend(
                pb.RecommendRequest(context_listing_ids=_ids(51)), metadata=_MD
            )
    assert exc.value.code() == grpc.StatusCode.INVALID_ARGUMENT
    assert service.queries == []


async def test_no_context_ids_maps_to_an_empty_context():
    service = _Fake()
    async with _serving(service) as stub:
        await stub.Recommend(pb.RecommendRequest(), metadata=_MD)
    assert list(service.queries[0].context_listing_ids) == []


async def test_logs_carry_context_count_and_no_ids():
    service = _Fake()
    ids = _ids(3)
    lines: list[str] = []
    sink = logger.add(
        lambda m: lines.append(str(m)),
        format="{message} | {extra}",
        level="DEBUG",
    )
    try:
        async with _serving(service) as stub:
            await stub.Recommend(
                pb.RecommendRequest(
                    context=pb.RECOMMENDATION_CONTEXT_CART, context_listing_ids=ids
                ),
                metadata=_MD,
            )
    finally:
        logger.remove(sink)
    text = "\n".join(lines)
    assert "context_count=3" in text or "'context_count': 3" in text
    for listing_id in ids:
        assert listing_id not in text
