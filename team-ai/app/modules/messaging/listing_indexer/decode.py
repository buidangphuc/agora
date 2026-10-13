"""Decode ``listing.events`` Kafka values into indexer payloads.

The wire contract is ``platform.events.v1.EventEnvelope`` (ADR-0002) whose ``type``
discriminates the payload. Only ``platform.listing.v1.ListingChanged`` drives the RAG
index; every other event type decodes to ``None``.
"""

from dataclasses import dataclass

from app.transport.grpc._pb.platform.events.v1 import events_pb2
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2

LISTING_CHANGED_TYPE = "platform.listing.v1.ListingChanged"

_ACTIONS = {
    listing_pb2.CHANGE_TYPE_CREATED: "CREATED",
    listing_pb2.CHANGE_TYPE_UPDATED: "UPDATED",
    listing_pb2.CHANGE_TYPE_DELETED: "DELETED",
}
_STATUSES = {
    listing_pb2.LISTING_STATUS_DRAFT: "DRAFT",
    listing_pb2.LISTING_STATUS_PUBLISHED: "PUBLISHED",
    listing_pb2.LISTING_STATUS_REJECTED: "REJECTED",
}


class UndecodableEventError(Exception):
    """The value is not a decodable envelope/payload: retrying cannot help."""


@dataclass(frozen=True, slots=True)
class ListingEventPayload:
    listing_id: str
    action: str  # CREATED | UPDATED | DELETED | UNSPECIFIED
    title: str
    description: str
    category: str
    price: float
    status: str  # DRAFT | PUBLISHED | REJECTED | "" (unknown)
    seller_id: str
    currency: str
    # Envelope identity: a redelivered event repeats event_id; occurred_ns orders
    # the events of one listing (0 = unknown).
    event_id: str
    occurred_ns: int


def decode_listing_event(value: bytes) -> ListingEventPayload | None:
    """Return the payload for a ListingChanged envelope, ``None`` for other types."""
    envelope = events_pb2.EventEnvelope()
    try:
        envelope.ParseFromString(value)
    except Exception as exc:
        raise UndecodableEventError(f"envelope: {exc}") from exc
    if envelope.type != LISTING_CHANGED_TYPE:
        return None

    changed = listing_pb2.ListingChanged()
    try:
        changed.ParseFromString(envelope.payload)
    except Exception as exc:
        raise UndecodableEventError(f"ListingChanged: {exc}") from exc
    listing = changed.listing
    if not listing.id:
        raise UndecodableEventError("ListingChanged has no listing id")

    occurred = envelope.occurred_at
    return ListingEventPayload(
        listing_id=listing.id,
        action=_ACTIONS.get(changed.change_type, "UNSPECIFIED"),
        title=listing.title,
        description=listing.description,
        category=listing.category_id,
        price=float(listing.price),
        status=_STATUSES.get(listing.status, ""),
        seller_id=listing.seller_id,
        currency=listing.currency,
        event_id=envelope.event_id,
        occurred_ns=occurred.seconds * 1_000_000_000 + occurred.nanos
        if envelope.HasField("occurred_at")
        else 0,
    )
