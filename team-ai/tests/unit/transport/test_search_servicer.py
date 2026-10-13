"""SearchService servicer: scope, allowlisted filters, hit mapping, UNAVAILABLE and UNIMPLEMENTED."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import grpc
import pytest
from llama_index.core.schema import NodeWithScore, TextNode

from app.core.errors import ServiceUnavailableError
from app.transport.grpc._pb.platform.common.v1 import common_pb2
from app.transport.grpc._pb.platform.search.v1 import search_pb2, search_pb2_grpc
from app.transport.grpc.server import build_grpc_server
from app.transport.grpc.servicers.search import search_service_registration
from tests.factories import allow_all_limiter


def _md(*scopes: str):
    return (
        ("x-principal-id", "u1"),
        ("x-principal-type", "user"),
        ("x-principal-scopes", ",".join(scopes)),
    )


class FakeRag:
    def __init__(self, *, error: Exception | None = None) -> None:
        self.calls: list[tuple[str, int | None, dict | None]] = []
        self.error = error

    async def search(
        self,
        query: str,
        *,
        top_k: int | None = None,
        filters: dict[str, str | int | float | bool] | None = None,
    ) -> list[NodeWithScore]:
        self.calls.append((query, top_k, filters))
        if self.error:
            raise self.error
        return [
            NodeWithScore(
                node=TextNode(text="x", metadata={"document_id": "L1"}), score=0.9
            ),
            NodeWithScore(
                node=TextNode(text="y", metadata={"document_id": "L2"}), score=0.5
            ),
        ]


@asynccontextmanager
async def _serving(
    rag: FakeRag | None,
) -> AsyncIterator[search_pb2_grpc.SearchServiceStub]:
    server = build_grpc_server(
        registrations=(search_service_registration(rag),),
        rate_limiter=allow_all_limiter(),
    )
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            yield search_pb2_grpc.SearchServiceStub(channel)
    finally:
        await server.stop(None)


async def _fails(call) -> grpc.aio.AioRpcError:
    with pytest.raises(grpc.aio.AioRpcError) as exc:
        await call
    return exc.value


async def test_maps_hits_and_total() -> None:
    rag = FakeRag()
    async with _serving(rag) as stub:
        reply = await stub.SearchListings(
            search_pb2.SearchListingsRequest(query="tai nghe"),
            metadata=_md("search:read"),
        )

    assert [(h.listing_id, round(h.score, 1)) for h in reply.hits] == [
        ("L1", 0.9),
        ("L2", 0.5),
    ]
    assert reply.page.total == 2
    assert reply.page.next_cursor == ""


async def test_page_size_is_top_k_and_zero_means_default() -> None:
    rag = FakeRag()
    async with _serving(rag) as stub:
        await stub.SearchListings(
            search_pb2.SearchListingsRequest(
                query="a", page=common_pb2.PageRequest(page_size=7)
            ),
            metadata=_md("search:read"),
        )
        await stub.SearchListings(
            search_pb2.SearchListingsRequest(query="b"), metadata=_md("search:read")
        )

    assert [c[1] for c in rag.calls] == [7, None]


async def test_filters_are_restricted_to_the_allowlist() -> None:
    rag = FakeRag()
    async with _serving(rag) as stub:
        await stub.SearchListings(
            search_pb2.SearchListingsRequest(
                query="a", filters={"category": "cat-electronics", "price": "100"}
            ),
            metadata=_md("search:read"),
        )
        await stub.SearchListings(
            search_pb2.SearchListingsRequest(query="a", filters={"price": "100"}),
            metadata=_md("search:read"),
        )

    assert rag.calls[0][2] == {"category": "cat-electronics"}
    assert rag.calls[1][2] is None


async def test_requires_search_read_scope() -> None:
    async with _serving(FakeRag()) as stub:
        error = await _fails(
            stub.SearchListings(
                search_pb2.SearchListingsRequest(query="a"),
                metadata=_md("listing.read"),
            )
        )

    assert error.code() == grpc.StatusCode.PERMISSION_DENIED


async def test_disabled_rag_is_unavailable() -> None:
    async with _serving(None) as stub:
        error = await _fails(
            stub.SearchListings(
                search_pb2.SearchListingsRequest(query="a"),
                metadata=_md("search:read"),
            )
        )

    assert error.code() == grpc.StatusCode.UNAVAILABLE
    assert error.details() == "service unavailable"


async def test_store_failure_is_unavailable() -> None:
    rag = FakeRag(error=ServiceUnavailableError("qdrant down"))
    async with _serving(rag) as stub:
        error = await _fails(
            stub.SearchListings(
                search_pb2.SearchListingsRequest(query="a"),
                metadata=_md("search:read"),
            )
        )

    assert error.code() == grpc.StatusCode.UNAVAILABLE
    assert error.details() == "service unavailable"


async def test_other_rpcs_are_unimplemented() -> None:
    async with _serving(FakeRag()) as stub:
        error = await _fails(
            stub.Suggest(search_pb2.SuggestRequest(), metadata=_md("search:read"))
        )

    assert error.code() == grpc.StatusCode.UNIMPLEMENTED
