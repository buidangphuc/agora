"""RecommendationService servicer: scope, identity binding, context map and error mapping, over a real channel."""

from __future__ import annotations

import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import cast

import grpc
import pytest
from loguru import logger

from app.core.errors import ServiceUnavailableError
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


class _FakeRecommendations:
    def __init__(self, *, unavailable: bool = False) -> None:
        self.queries: list[RecommendationQuery] = []
        self._unavailable = unavailable

    async def recommend(self, query: RecommendationQuery) -> RecommendationResult:
        self.queries.append(query)
        if self._unavailable:
            raise ServiceUnavailableError("collection contract mismatch")
        return RecommendationResult(
            items=[
                RecommendedItem(listing_id="l1", score=0.9, rank=1),
                RecommendedItem(listing_id="l2", score=0.5, rank=2),
            ],
            model_version="als-1",
            placement_id="home_feed",
        )


def _md(principal_id: str, ptype: str = "user", scopes: str = "listing.read"):
    return (
        ("x-principal-id", principal_id),
        ("x-principal-type", ptype),
        ("x-principal-scopes", scopes),
    )


@asynccontextmanager
async def _serving(
    service: _FakeRecommendations,
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


async def _recommend(
    metadata, **fields
) -> tuple[pb.RecommendResponse, _FakeRecommendations]:
    service = _FakeRecommendations()
    async with _serving(service) as stub:
        reply = await stub.Recommend(pb.RecommendRequest(**fields), metadata=metadata)
    return reply, service


async def _fails(call) -> grpc.aio.AioRpcError:
    with pytest.raises(grpc.aio.AioRpcError) as exc:
        await call
    return exc.value


def test_recommend_is_not_rate_limited():
    registration = recommendation_service_registration(
        cast("RecommendationService", _FakeRecommendations())
    )

    assert (
        registration.service_name == "platform.recommendation.v1.RecommendationService"
    )
    assert registration.rate_limited_methods == frozenset()


async def test_response_maps_items_rank_model_version_and_request_id():
    reply, _ = await _recommend(_md("u1"))

    assert [(i.listing_id, i.rank) for i in reply.items] == [("l1", 1), ("l2", 2)]
    assert reply.items[0].score == pytest.approx(0.9)
    assert reply.model_version == "als-1"
    assert reply.placement_id == "home_feed"
    assert re.fullmatch(r"[0-9a-f]{32}", reply.request_id)


async def test_each_response_gets_its_own_request_id():
    service = _FakeRecommendations()
    async with _serving(service) as stub:
        first = await stub.Recommend(pb.RecommendRequest(), metadata=_md("u1"))
        second = await stub.Recommend(pb.RecommendRequest(), metadata=_md("u1"))

    assert first.request_id != second.request_id


async def test_requires_listing_read():
    async with _serving(_FakeRecommendations()) as stub:
        error = await _fails(
            stub.Recommend(pb.RecommendRequest(), metadata=_md("u1", scopes="search"))
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED


async def test_user_cannot_impersonate():
    _, service = await _recommend(_md("u1"), user_id="u2", anonymous_id="device-1")

    assert (service.queries[0].user_id, service.queries[0].anonymous_id) == ("u1", "")


async def test_anonymous_keeps_only_anonymous_id():
    _, service = await _recommend(
        _md("anonymous", "anonymous"), user_id="u2", anonymous_id="device-1"
    )

    assert (service.queries[0].user_id, service.queries[0].anonymous_id) == (
        "",
        "device-1",
    )


@pytest.mark.parametrize(
    "metadata",
    [_md("svc", "service"), _md("ops", "user", "listing.read,admin")],
)
async def test_admin_and_service_pass_identity_through(metadata):
    _, service = await _recommend(metadata, user_id="u2", anonymous_id="device-1")

    assert (service.queries[0].user_id, service.queries[0].anonymous_id) == (
        "u2",
        "device-1",
    )


@pytest.mark.parametrize(
    ("context", "placement"),
    [
        (pb.RECOMMENDATION_CONTEXT_HOMEPAGE, "home_feed"),
        (pb.RECOMMENDATION_CONTEXT_SIMILAR_ITEMS, "similar_items"),
        (pb.RECOMMENDATION_CONTEXT_CART, "cart_cross_sell"),
        (pb.RECOMMENDATION_CONTEXT_SEARCH, ""),
        (pb.RECOMMENDATION_CONTEXT_UNSPECIFIED, ""),
    ],
)
async def test_context_maps_to_placement(context: int, placement: str):
    _, service = await _recommend(
        _md("u1"), context=context, seed_listing_id="l9", limit=3
    )

    query = service.queries[0]
    assert query.placement_id == placement
    assert (query.seed_listing_id, query.limit) == ("l9", 3)


async def test_service_unavailable_maps_to_unavailable():
    async with _serving(_FakeRecommendations(unavailable=True)) as stub:
        error = await _fails(stub.Recommend(pb.RecommendRequest(), metadata=_md("u1")))

    assert error.code() == grpc.StatusCode.UNAVAILABLE
    assert error.details() == "collection contract mismatch"


async def test_logs_one_served_line_per_response():
    records = []
    sink = logger.add(lambda m: records.append(m.record), level="INFO")
    try:
        reply, _ = await _recommend(_md("u1"))
    finally:
        logger.remove(sink)

    served = [r for r in records if r["message"] == "recs.served"]
    assert len(served) == 1
    extra = served[0]["extra"]
    assert extra["request_id"] == reply.request_id
    assert extra["placement_id"] == "home_feed"
    assert extra["model_version"] == "als-1"
    assert extra["listing_ids"] == ["l1", "l2"]
    assert extra["fallback"] is False
    assert extra["principal_type"] == "user"
