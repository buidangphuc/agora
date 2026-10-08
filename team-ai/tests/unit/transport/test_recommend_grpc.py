"""RecommendationService over a real in-process gRPC server (serve-trained-recs-locally).

The servicer used to be skipped at startup because the recommendation stubs were not
vendored; these tests pin that it is registered, maps the module result onto the
contract, and answers UNAVAILABLE while recommendations are disabled.
"""

from __future__ import annotations

from types import SimpleNamespace

import grpc
import pytest

from app.transport.grpc._pb.platform.recommendation.v1 import (
    recommendation_pb2,
    recommendation_pb2_grpc,
)
from app.transport.grpc.chat_stream import build_chat_streamer
from app.transport.grpc.server import build_grpc_server
from tests.factories import build_test_settings

_AUTH = (("authorization", "bearer secret"),)


class _FakeRecs:
    def __init__(self) -> None:
        self.queries = []

    async def recommend(self, query):
        self.queries.append(query)
        return SimpleNamespace(
            items=[SimpleNamespace(listing_id="lst-1", score=0.8, rank=1)],
            model_version="als-test",
        )


async def _start(recs, roles="listing.read"):
    settings = build_test_settings(
        AUTH_BEARER_TOKEN="secret",
        AUTH_ROLES=roles,
        GRPC_BEARER_FALLBACK_ENABLED=True,
        GRPC_REFLECTION_ENABLED=False,
        CHAT_BACKEND="mock",
    )
    server = build_grpc_server(
        settings=settings,
        rag_provider=lambda: None,
        chat_streamer=build_chat_streamer(settings),
        recommendation_provider=lambda: recs,
    )
    port = server.add_insecure_port("localhost:0")
    await server.start()
    return server, port


async def test_recommend_maps_the_module_result():
    recs = _FakeRecs()
    server, port = await _start(recs)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as ch:
            stub = recommendation_pb2_grpc.RecommendationServiceStub(ch)
            res = await stub.Recommend(
                recommendation_pb2.RecommendRequest(
                    user_id="u-1",
                    limit=5,
                    context=recommendation_pb2.RECOMMENDATION_CONTEXT_HOMEPAGE,
                ),
                metadata=_AUTH,
            )
    finally:
        await server.stop(None)
    assert [(r.listing_id, r.rank) for r in res.items] == [("lst-1", 1)]
    assert res.model_version == "als-test"
    q = recs.queries[0]
    assert (q.user_id, q.limit, q.placement_id) == ("u-1", 5, "home_feed")


async def test_recommend_is_unavailable_while_disabled():
    server, port = await _start(None)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as ch:
            stub = recommendation_pb2_grpc.RecommendationServiceStub(ch)
            with pytest.raises(grpc.aio.AioRpcError) as exc:
                await stub.Recommend(
                    recommendation_pb2.RecommendRequest(), metadata=_AUTH
                )
    finally:
        await server.stop(None)
    assert exc.value.code() == grpc.StatusCode.UNAVAILABLE


async def test_recommend_requires_the_listing_read_scope():
    server, port = await _start(_FakeRecs(), roles="search:read")
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as ch:
            stub = recommendation_pb2_grpc.RecommendationServiceStub(ch)
            with pytest.raises(grpc.aio.AioRpcError) as exc:
                await stub.Recommend(
                    recommendation_pb2.RecommendRequest(), metadata=_AUTH
                )
    finally:
        await server.stop(None)
    assert exc.value.code() == grpc.StatusCode.PERMISSION_DENIED


def _forwarded(pid: str, ptype: str, scopes: str = "listing.read"):
    return (
        ("x-principal-id", pid),
        ("x-principal-type", ptype),
        ("x-principal-scopes", scopes),
    )


async def _recommend_as(metadata, **request_fields):
    recs = _FakeRecs()
    server, port = await _start(recs)
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as ch:
            stub = recommendation_pb2_grpc.RecommendationServiceStub(ch)
            await stub.Recommend(
                recommendation_pb2.RecommendRequest(**request_fields),
                metadata=metadata,
            )
    finally:
        await server.stop(None)
    return recs.queries[0]


async def test_user_principal_cannot_request_another_users_recommendations():
    q = await _recommend_as(
        _forwarded("buyer-1", "user"), user_id="victim", anonymous_id="dev-1"
    )
    assert (q.user_id, q.anonymous_id) == ("buyer-1", "")


async def test_anonymous_principal_cannot_claim_a_user_id():
    q = await _recommend_as(
        _forwarded("anonymous", "anonymous"), user_id="victim", anonymous_id="dev-1"
    )
    assert (q.user_id, q.anonymous_id) == ("", "dev-1")


async def test_admin_and_service_principals_may_request_on_behalf_of_a_user():
    admin = await _recommend_as(
        _forwarded("admin-1", "user", "listing.read,admin"), user_id="u-9"
    )
    service = await _recommend_as(_forwarded("svc", "service"), user_id="u-9")
    assert admin.user_id == "u-9"
    assert service.user_id == "u-9"
