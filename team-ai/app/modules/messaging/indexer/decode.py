"""Decode ``listing.events`` Kafka values into indexer payloads.

The wire contract is ``platform.events.v1.EventEnvelope`` (ADR-0002) whose ``type``
discriminates the payload. Only ``platform.listing.v1.ListingChanged`` drives the RAG
index; every other listing event type is not ours and decodes to ``None``.
"""

from __future__ import annotations

from app.modules.messaging.indexer.handler import ListingEventPayload
from app.transport.grpc._pb.platform.events.v1 import events_pb2
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2

LISTING_CHANGED_TYPE = "platform.listing.v1.ListingChanged"


class UndecodableEventError(ValueError):
    """The value is not a decodable envelope/payload: retrying cannot help."""


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
        action=_action(changed.change_type),
        title=listing.title,
        description=listing.description,
        category=listing.category_id,
        price=float(listing.price),
        status=_status(listing.status),
        seller_id=listing.seller_id,
        currency=listing.currency,
        event_id=envelope.event_id,
        occurred_ns=occurred.seconds * 1_000_000_000 + occurred.nanos
        if envelope.HasField("occurred_at")
        else 0,
    )


def _action(change_type: listing_pb2.ChangeType) -> str:
    if change_type == listing_pb2.CHANGE_TYPE_CREATED:
        return "CREATED"
    if change_type == listing_pb2.CHANGE_TYPE_UPDATED:
        return "UPDATED"
    if change_type == listing_pb2.CHANGE_TYPE_DELETED:
        return "DELETED"
    return "UNSPECIFIED"


def _status(status: listing_pb2.ListingStatus) -> str:
    if status == listing_pb2.LISTING_STATUS_DRAFT:
        return "DRAFT"
    if status == listing_pb2.LISTING_STATUS_PUBLISHED:
        return "PUBLISHED"
    if status == listing_pb2.LISTING_STATUS_REJECTED:
        return "REJECTED"
    return ""
