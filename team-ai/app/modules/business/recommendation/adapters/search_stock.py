from collections.abc import Sequence

import grpc

from app.modules.business.recommendation.ports import StockEntry, StockLookupError
from app.transport.grpc._pb.platform.search.v1 import search_pb2, search_pb2_grpc


class GrpcStockLookup:
    def __init__(
        self,
        channel: grpc.aio.Channel,
        *,
        principal_id: str,
        principal_scopes: Sequence[str],
    ) -> None:
        self._stub = search_pb2_grpc.SearchServiceStub(channel)
        self._metadata = (
            ("x-principal-id", principal_id),
            ("x-principal-type", "service"),
            ("x-principal-scopes", ",".join(principal_scopes)),
        )

    async def lookup(
        self, listing_ids: Sequence[str], *, timeout_s: float
    ) -> list[StockEntry]:
        try:
            reply = await self._stub.GetListingStock(
                search_pb2.GetListingStockRequest(listing_ids=list(listing_ids)),
                timeout=timeout_s,
                metadata=self._metadata,
            )
        except grpc.aio.AioRpcError as exc:
            if exc.code() == grpc.StatusCode.DEADLINE_EXCEEDED:
                raise TimeoutError("stock lookup deadline exceeded") from exc
            raise StockLookupError(f"stock lookup rejected: {exc.code().name}") from exc
        return [
            StockEntry(
                listing_id=entry.listing_id,
                stock=entry.stock if entry.HasField("stock") else None,
            )
            for entry in reply.listings
        ]
