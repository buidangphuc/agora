"""Pure recommend-module data types.

These carry no gRPC/proto dependency on purpose: the transport servicer maps the
generated ``recommendation_pb2`` messages to/from these, so the whole
retrieval/ranking/cache pipeline is importable and unit-testable without the
generated stubs. Keep it that way — proto types live only at the transport edge.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class RecommendQuery:
    """A normalized Recommend request (proto-free)."""

    user_id: str = ""
    anonymous_id: str = ""
    seed_listing_id: str = ""
    context: str = ""
    limit: int = 0

    @property
    def is_anonymous(self) -> bool:
        return not self.user_id


@dataclass(frozen=True)
class Candidate:
    """A retrieval candidate carrying the business-rule fields read at serve time.

    All fields come from the Qdrant point payload or the Redis cache value, so
    ranking/filtering needs no extra service call on the hot path.
    """

    listing_id: str
    score: float
    in_stock: bool = True
    category_id: str = ""
    seller_id: str = ""


@dataclass(frozen=True)
class RecommendedItem:
    listing_id: str
    score: float
    rank: int


@dataclass(frozen=True)
class RecommendResult:
    items: list[RecommendedItem]
    model_version: str
    # Which path produced the result — "cache" | "ann" | "popular". Useful for
    # observability and asserted by unit tests; not part of the wire contract.
    source: str
