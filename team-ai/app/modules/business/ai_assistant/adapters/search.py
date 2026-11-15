from collections.abc import Sequence
from dataclasses import dataclass

import grpc

from app.modules.business.ai_assistant.adapters.errors import RetrievalError
from app.transport.grpc._pb.platform.common.v1 import common_pb2
from app.transport.grpc._pb.platform.search.v1 import search_pb2, search_pb2_grpc


@dataclass(frozen=True, slots=True)
class SearchHitFacts:
    listing_id: str
    rating_avg: float
    review_count: int


@dataclass(frozen=True, slots=True)
class SearchOutcome:
    hits: list[SearchHitFacts]
    model_version: str


class GrpcListingSearch:
    def __init__(
        self,
        channel: grpc.aio.Channel,
        *,
        principal_id: str,
        principal_scopes: Sequence[str],
    ) -> None:
        self._channel = channel
        self._metadata = (
            ("x-principal-id", principal_id),
            ("x-principal-type", "service"),
            ("x-principal-scopes", ",".join(principal_scopes)),
        )

    async def search(
        self,
        query: str,
        *,
        max_price: int | None,
        page_size: int,
        timeout_s: float,
    ) -> SearchOutcome:
        try:
            reply = await search_pb2_grpc.SearchServiceStub(
                self._channel
            ).SearchListings(
                search_pb2.SearchListingsRequest(
                    query=query,
                    search_mode=search_pb2.SEARCH_MODE_HYBRID,
                    filters={"in_stock": "true"},
                    max_price=max_price or 0,
                    page=common_pb2.PageRequest(page_size=page_size),
                ),
                timeout=timeout_s,
                metadata=self._metadata,
            )
        except grpc.aio.AioRpcError as exc:
            if exc.code() == grpc.StatusCode.DEADLINE_EXCEEDED:
                raise TimeoutError("search deadline exceeded") from exc
            raise RetrievalError(f"search rejected: {exc.code().name}") from exc
        return SearchOutcome(
            hits=[
                SearchHitFacts(
                    listing_id=hit.listing_id,
                    rating_avg=hit.rating_avg,
                    review_count=hit.review_count,
                )
                for hit in reply.hits
            ],
            model_version=reply.model_version,
        )
