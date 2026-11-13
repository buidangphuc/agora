from typing import Any

import grpc
import pytest

from app.modules.business.ai_assistant.adapters.errors import RetrievalError
from app.modules.business.ai_assistant.adapters.listing import GrpcListingReader
from app.modules.business.ai_assistant.adapters.search import GrpcListingSearch
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2
from app.transport.grpc._pb.platform.search.v1 import search_pb2


class _RpcError(grpc.aio.AioRpcError):
    def __init__(self, code: grpc.StatusCode) -> None:
        super().__init__(code, grpc.aio.Metadata(), grpc.aio.Metadata())


class FakeChannel:
    """Records unary calls by method name; replies come from ``replies``."""

    def __init__(self, replies: dict[str, Any]) -> None:
        self.replies = replies
        self.calls: list[tuple[str, Any, float | None, tuple]] = []

    def unary_unary(self, path: str, *args: Any, **kwargs: Any):
        method = path.rsplit("/", 1)[-1]

        async def call(request, timeout=None, metadata=None, **_: Any):
            self.calls.append((method, request, timeout, tuple(metadata or ())))
            reply = self.replies[method]
            if isinstance(reply, Exception):
                raise reply
            return reply

        return call


def _search(channel: FakeChannel) -> GrpcListingSearch:
    return GrpcListingSearch(
        channel,
        principal_id="team-ai",
        principal_scopes=["search:read"],  # type: ignore[arg-type]
    )


def _reader(channel: FakeChannel) -> GrpcListingReader:
    return GrpcListingReader(
        channel,
        principal_id="team-ai",
        principal_scopes=["listing.read"],  # type: ignore[arg-type]
    )


async def test_search_sends_hybrid_in_stock_budget_and_page_size_five_as_search_read():
    """Scenario: Published in-stock listing becomes a card (search request shape)."""
    channel = FakeChannel(
        {
            "SearchListings": search_pb2.SearchListingsResponse(
                hits=[
                    search_pb2.SearchHit(
                        listing_id="l-1", rating_avg=4.5, review_count=12
                    )
                ],
                model_version="hybrid@v1",
            )
        }
    )

    result = await _search(channel).search(
        "giày dưới 500k", max_price=500000, page_size=5, timeout_s=0.8
    )

    (method, request, timeout, metadata) = channel.calls[0]
    assert method == "SearchListings"
    assert request.query == "giày dưới 500k"
    assert request.search_mode == search_pb2.SEARCH_MODE_HYBRID
    assert dict(request.filters) == {"in_stock": "true"}
    assert request.max_price == 500000
    assert request.page.page_size == 5
    assert timeout == 0.8
    meta = dict(metadata)
    assert meta["x-principal-id"] == "team-ai"
    assert meta["x-principal-type"] == "service"
    assert meta["x-principal-scopes"] == "search:read"
    assert result.model_version == "hybrid@v1"
    assert [h.listing_id for h in result.hits] == ["l-1"]
    assert result.hits[0].rating_avg == 4.5
    assert result.hits[0].review_count == 12


async def test_search_without_a_budget_leaves_max_price_unset():
    """Scenario: Published in-stock listing becomes a card (no budget)."""
    channel = FakeChannel({"SearchListings": search_pb2.SearchListingsResponse()})

    await _search(channel).search("giày", max_price=None, page_size=5, timeout_s=0.8)

    assert channel.calls[0][1].max_price == 0


async def test_search_unavailable_raises_a_typed_retrieval_error():
    """Scenario: Search is down."""
    channel = FakeChannel({"SearchListings": _RpcError(grpc.StatusCode.UNAVAILABLE)})

    with pytest.raises(RetrievalError):
        await _search(channel).search("x", max_price=None, page_size=5, timeout_s=0.8)


async def test_search_permission_denied_raises_a_typed_retrieval_error():
    """Scenario: Search is down (scope rejected)."""
    channel = FakeChannel(
        {"SearchListings": _RpcError(grpc.StatusCode.PERMISSION_DENIED)}
    )

    with pytest.raises(RetrievalError):
        await _search(channel).search("x", max_price=None, page_size=5, timeout_s=0.8)


async def test_search_deadline_exceeded_raises_timeout():
    """Scenario: Search is down (deadline)."""
    channel = FakeChannel(
        {"SearchListings": _RpcError(grpc.StatusCode.DEADLINE_EXCEEDED)}
    )

    with pytest.raises(TimeoutError):
        await _search(channel).search("x", max_price=None, page_size=5, timeout_s=0.8)


async def test_listing_reader_returns_title_price_and_published_as_listing_read():
    """Scenario: Published in-stock listing becomes a card (GetListing)."""
    channel = FakeChannel(
        {
            "GetListing": listing_pb2.GetListingResponse(
                listing=listing_pb2.Listing(
                    id="l-1",
                    title="Giày chạy bộ",
                    price=450000,
                    currency="VND",
                    status=listing_pb2.LISTING_STATUS_PUBLISHED,
                )
            )
        }
    )

    facts = await _reader(channel).get("l-1", timeout_s=0.5)

    (method, request, timeout, metadata) = channel.calls[0]
    assert method == "GetListing"
    assert request.id == "l-1"
    assert timeout == 0.5
    meta = dict(metadata)
    assert meta["x-principal-id"] == "team-ai"
    assert meta["x-principal-type"] == "service"
    assert meta["x-principal-scopes"] == "listing.read"
    assert facts is not None
    assert (facts.listing_id, facts.title, facts.price, facts.currency) == (
        "l-1",
        "Giày chạy bộ",
        450000,
        "VND",
    )
    assert facts.published is True


async def test_listing_reader_marks_an_unpublished_listing_not_published():
    """Scenario: Published in-stock listing becomes a card (status mapping)."""
    channel = FakeChannel(
        {
            "GetListing": listing_pb2.GetListingResponse(
                listing=listing_pb2.Listing(
                    id="l-2", status=listing_pb2.LISTING_STATUS_DRAFT
                )
            )
        }
    )

    facts = await _reader(channel).get("l-2", timeout_s=0.5)

    assert facts is not None
    assert facts.published is False


async def test_listing_reader_returns_none_when_the_listing_is_missing():
    """Scenario: Published in-stock listing becomes a card (missing listing)."""
    channel = FakeChannel({"GetListing": _RpcError(grpc.StatusCode.NOT_FOUND)})

    assert await _reader(channel).get("gone", timeout_s=0.5) is None


async def test_listing_reader_unavailable_raises_a_typed_retrieval_error():
    """Scenario: Search is down (domain unavailable)."""
    channel = FakeChannel({"GetListing": _RpcError(grpc.StatusCode.UNAVAILABLE)})

    with pytest.raises(RetrievalError):
        await _reader(channel).get("l-1", timeout_s=0.5)


async def test_listing_reader_deadline_exceeded_raises_timeout():
    """Scenario: Search is down (domain deadline)."""
    channel = FakeChannel({"GetListing": _RpcError(grpc.StatusCode.DEADLINE_EXCEEDED)})

    with pytest.raises(TimeoutError):
        await _reader(channel).get("l-1", timeout_s=0.5)
