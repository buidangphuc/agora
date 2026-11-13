from collections.abc import Sequence
from dataclasses import dataclass

import grpc

from app.modules.business.ai_assistant.adapters.errors import RetrievalError
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2, listing_pb2_grpc


@dataclass(frozen=True, slots=True)
class ListingFacts:
    listing_id: str
    title: str
    price: int
    currency: str
    published: bool


class GrpcListingReader:
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

    async def get(self, listing_id: str, *, timeout_s: float) -> ListingFacts | None:
        try:
            reply = await listing_pb2_grpc.ListingServiceStub(self._channel).GetListing(
                listing_pb2.GetListingRequest(id=listing_id),
                timeout=timeout_s,
                metadata=self._metadata,
            )
        except grpc.aio.AioRpcError as exc:
            code = exc.code()
            if code == grpc.StatusCode.NOT_FOUND:
                return None
            if code == grpc.StatusCode.DEADLINE_EXCEEDED:
                raise TimeoutError("listing lookup deadline exceeded") from exc
            raise RetrievalError(f"listing lookup rejected: {code.name}") from exc
        listing = reply.listing
        return ListingFacts(
            listing_id=listing.id,
            title=listing.title,
            price=listing.price,
            currency=listing.currency,
            published=listing.status == listing_pb2.LISTING_STATUS_PUBLISHED,
        )
