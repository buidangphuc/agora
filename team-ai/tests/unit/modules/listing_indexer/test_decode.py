import pytest
from google.protobuf.timestamp_pb2 import Timestamp

from app.modules.messaging.listing_indexer.decode import (
    UndecodableEventError,
    decode_listing_event,
)
from app.transport.grpc._pb.platform.events.v1 import events_pb2
from app.transport.grpc._pb.platform.listing.v1 import listing_pb2

LISTING_CHANGED = "platform.listing.v1.ListingChanged"


def envelope(
    listing_id: str,
    change: int = listing_pb2.CHANGE_TYPE_CREATED,
    *,
    event_id: str = "ev-1",
    title: str = "Tai nghe",
    description: str = "",
    price: int = 1_500_000,
    status: int = listing_pb2.LISTING_STATUS_PUBLISHED,
    seconds: int = 1_000,
    type_: str = LISTING_CHANGED,
) -> bytes:
    changed = listing_pb2.ListingChanged(
        listing=listing_pb2.Listing(
            id=listing_id,
            title=title,
            description=description,
            price=price,
            currency="VND",
            status=status,
            seller_id="seller-1",
            category_id="cat-electronics",
        ),
        change_type=change,
    )
    return events_pb2.EventEnvelope(
        event_id=event_id,
        type=type_,
        occurred_at=Timestamp(seconds=seconds),
        payload=changed.SerializeToString(),
    ).SerializeToString()


def test_maps_envelope_and_listing_fields() -> None:
    payload = decode_listing_event(
        envelope("l-1", listing_pb2.CHANGE_TYPE_UPDATED, description="d")
    )
    assert payload is not None
    assert (payload.listing_id, payload.action, payload.status) == (
        "l-1",
        "UPDATED",
        "PUBLISHED",
    )
    assert (payload.title, payload.description, payload.category) == (
        "Tai nghe",
        "d",
        "cat-electronics",
    )
    assert payload.price == 1_500_000.0 and isinstance(payload.price, float)
    assert (payload.seller_id, payload.currency) == ("seller-1", "VND")
    assert payload.event_id == "ev-1"
    assert payload.occurred_ns == 1_000 * 1_000_000_000


@pytest.mark.parametrize(
    ("change", "action"),
    [
        (listing_pb2.CHANGE_TYPE_CREATED, "CREATED"),
        (listing_pb2.CHANGE_TYPE_DELETED, "DELETED"),
        (listing_pb2.CHANGE_TYPE_UNSPECIFIED, "UNSPECIFIED"),
    ],
)
def test_maps_actions(change: int, action: str) -> None:
    payload = decode_listing_event(envelope("l-1", change))
    assert payload is not None and payload.action == action


@pytest.mark.parametrize(
    ("status", "name"),
    [
        (listing_pb2.LISTING_STATUS_DRAFT, "DRAFT"),
        (listing_pb2.LISTING_STATUS_REJECTED, "REJECTED"),
        (listing_pb2.LISTING_STATUS_UNSPECIFIED, ""),
    ],
)
def test_maps_statuses(status: int, name: str) -> None:
    payload = decode_listing_event(envelope("l-1", status=status))
    assert payload is not None and payload.status == name


def test_foreign_event_type_is_none() -> None:
    assert (
        decode_listing_event(envelope("l-1", type_="platform.order.v1.OrderPlaced"))
        is None
    )


def test_garbage_bytes_are_undecodable() -> None:
    with pytest.raises(UndecodableEventError):
        decode_listing_event(b"\xff\xff not a protobuf")


def test_empty_listing_id_is_undecodable() -> None:
    with pytest.raises(UndecodableEventError):
        decode_listing_event(envelope(""))


def test_new_statuses_are_decoded() -> None:
    for status, name in (
        (listing_pb2.LISTING_STATUS_PENDING_REVIEW, "PENDING_REVIEW"),
        (listing_pb2.LISTING_STATUS_BANNED, "BANNED"),
        (listing_pb2.LISTING_STATUS_HELD, "HELD"),
    ):
        payload = decode_listing_event(envelope("l-1", status=status))
        assert payload is not None and payload.status == name


def test_unknown_status_number_decodes_to_unknown() -> None:
    payload = decode_listing_event(envelope("l-1", status=99))
    assert payload is not None and payload.status == "UNKNOWN"


def test_unspecified_status_decodes_to_empty() -> None:
    payload = decode_listing_event(envelope("l-1", status=0))
    assert payload is not None and payload.status == ""
