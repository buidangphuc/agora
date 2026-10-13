"""SearchService gRPC servicer: ``SearchListings`` over the RAG store.

Only ``SearchListings`` is implemented; the other RPCs keep the generated
UNIMPLEMENTED default. Embedding and vector search live in the RAG service.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Final, Protocol

import grpc

from app.core.errors import ServiceUnavailableError
from app.transport.grpc._pb.platform.common.v1 import common_pb2
from app.transport.grpc._pb.platform.search.v1 import search_pb2, search_pb2_grpc
from app.transport.grpc.context import ensure_scopes
from app.transport.grpc.registry import GrpcServiceRegistration

if TYPE_CHECKING:
    from llama_index.core.schema import NodeWithScore

_SERVICE_NAME = "platform.search.v1.SearchService"
SEARCH_READ_SCOPE: Final = "search:read"
_UNAVAILABLE_MESSAGE: Final = "service unavailable"

# Metadata keys the indexer writes that callers may filter on.
FILTERABLE_KEYS: Final = frozenset({"category", "currency", "seller_id", "status"})


class ListingSearch(Protocol):
    async def search(
        self,
        query: str,
        *,
        top_k: int | None = None,
        filters: dict[str, str | int | float | bool] | None = None,
    ) -> list[NodeWithScore]: ...


class SearchServicer(search_pb2_grpc.SearchServiceServicer):
    def __init__(self, rag: ListingSearch | None) -> None:
        self._rag = rag

    async def SearchListings(
        self,
        request: search_pb2.SearchListingsRequest,
        context: grpc.aio.ServicerContext,
    ) -> search_pb2.SearchListingsResponse:
        await ensure_scopes(context, SEARCH_READ_SCOPE)
        if self._rag is None:
            await context.abort(grpc.StatusCode.UNAVAILABLE, _UNAVAILABLE_MESSAGE)
            raise AssertionError("unreachable")  # abort raises

        filters: dict[str, str | int | float | bool] = {
            key: value
            for key, value in request.filters.items()
            if key in FILTERABLE_KEYS
        }
        try:
            nodes = await self._rag.search(
                request.query,
                top_k=request.page.page_size or None,
                filters=filters or None,
            )
        except ServiceUnavailableError as exc:
            await context.abort(grpc.StatusCode.UNAVAILABLE, _UNAVAILABLE_MESSAGE)
            raise AssertionError("unreachable") from exc  # abort raises

        hits = [
            search_pb2.SearchHit(
                listing_id=node.node.metadata["document_id"], score=node.get_score()
            )
            for node in nodes
        ]
        return search_pb2.SearchListingsResponse(
            hits=hits, page=common_pb2.PageResponse(next_cursor="", total=len(hits))
        )


def search_service_registration(rag: ListingSearch | None) -> GrpcServiceRegistration:
    """``rag`` is None while ``RAG_ENABLED=false``: SearchListings then fails UNAVAILABLE."""
    servicer = SearchServicer(rag)

    def register(server: grpc.aio.Server) -> None:
        search_pb2_grpc.add_SearchServiceServicer_to_server(servicer, server)

    return GrpcServiceRegistration(_SERVICE_NAME, register)
