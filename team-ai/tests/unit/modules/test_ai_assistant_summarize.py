"""SummarizeReviews (F8) — deterministic review summarization.

Exercises the heuristic ``AIAssistantService.summarize_reviews`` offline (no LLM,
no network) plus the gRPC servicer shape through a real in-process server, mirroring
the sibling MagicListing/ShoppingAssistant pattern and ``test_grpc_server.py``.
"""

from __future__ import annotations

import grpc

from app.modules.business.ai_assistant.schemas import (
    ReviewItem,
    SummarizeReviewsRequest,
)
from app.modules.business.ai_assistant.service import AIAssistantService
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc.chat_stream import build_chat_streamer
from app.transport.grpc.server import build_grpc_server
from tests.factories import build_test_settings

_AUTH = (("authorization", "bearer secret"),)


def _settings(**overrides):
    base = {
        "AUTH_BEARER_TOKEN": "secret",
        "AUTH_ROLES": "ai:read",
        "GRPC_REFLECTION_ENABLED": False,
        "CHAT_BACKEND": "mock",
    }
    base.update(overrides)
    return build_test_settings(**base)


async def _start(settings):
    server = build_grpc_server(
        settings=settings,
        rag_provider=lambda: None,
        chat_streamer=build_chat_streamer(settings),
    )
    port = server.add_insecure_port("localhost:0")
    await server.start()
    return server, port


async def test_summarize_reviews_happy_path():
    service = AIAssistantService()
    req = SummarizeReviewsRequest(
        listing_id="prod-101",
        reviews=[
            ReviewItem(rating=5, comment="Chất lượng tốt, giao hàng nhanh"),
            ReviewItem(rating=4, comment="Shop tư vấn nhiệt tình, giá hợp lý"),
        ],
    )

    result = await service.summarize_reviews(req)

    assert result.sentiment == "positive"
    assert "Chất lượng sản phẩm" in result.pros
    assert result.cons == []
    assert "2 đánh giá" in result.summary
    assert "4.5/5" in result.summary


async def test_summarize_reviews_empty_is_safe():
    service = AIAssistantService()

    result = await service.summarize_reviews(
        SummarizeReviewsRequest(listing_id="prod-empty", reviews=[])
    )

    assert result.sentiment == "unknown"
    assert result.pros == []
    assert result.cons == []
    assert result.summary  # non-empty, no panic on empty input


async def test_summarize_reviews_mixed_sentiment_splits_pros_cons():
    service = AIAssistantService()
    req = SummarizeReviewsRequest(
        reviews=[
            ReviewItem(rating=5, comment="Mẫu mã đẹp, đúng mô tả"),
            ReviewItem(rating=1, comment="Giao hàng chậm, đóng gói kém"),
        ],
    )

    result = await service.summarize_reviews(req)

    assert result.sentiment == "mixed"
    assert "Mẫu mã & thiết kế" in result.pros
    assert "Giao hàng & đóng gói" in result.cons
    # A negative review with no aspect keyword still never leaves cons empty.
    assert result.cons


async def test_summarize_reviews_grpc_shape():
    server, port = await _start(_settings())
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            stub = ai_pb2_grpc.AIServiceStub(channel)
            resp = await stub.SummarizeReviews(
                ai_pb2.SummarizeReviewsRequest(
                    listing_id="prod-101",
                    reviews=[
                        ai_pb2.ReviewInput(rating=5, comment="Chất lượng tốt"),
                        ai_pb2.ReviewInput(rating=2, comment="Giá hơi đắt"),
                    ],
                ),
                metadata=_AUTH,
            )
        assert isinstance(resp.summary, str) and resp.summary
        assert resp.sentiment == "mixed"
        assert list(resp.pros)  # populated for the 5-star review
        assert list(resp.cons)  # populated for the 2-star review
    finally:
        await server.stop(None)


async def test_summarize_reviews_grpc_requires_auth():
    server, port = await _start(_settings())
    try:
        async with grpc.aio.insecure_channel(f"localhost:{port}") as channel:
            stub = ai_pb2_grpc.AIServiceStub(channel)
            try:
                await stub.SummarizeReviews(
                    ai_pb2.SummarizeReviewsRequest(listing_id="x")
                )
                raise AssertionError("expected UNAUTHENTICATED")
            except grpc.aio.AioRpcError as exc:
                assert exc.code() == grpc.StatusCode.UNAUTHENTICATED
    finally:
        await server.stop(None)
