"""Decode the ``analytics.events`` wire format without generated protobuf code.

A Kafka message is a ``platform.events.v1.EventEnvelope`` whose ``payload`` is a
``platform.analytics.v1.TrackingEvent`` (spec ``tracking``). The nearline consumer needs six fields of
the event and three of the envelope, so it reads the protobuf wire format directly instead of vendoring
the contract and a protobuf runtime into a Spark image. The field numbers below are copied from
``platform-core/packages/proto`` and are additive-only there; ``tests/test_nearline_wire.py`` pins them.

  EventEnvelope  1 event_id (string)  2 type (string)  3 occurred_at (Timestamp: 1 seconds, 2 nanos)
                 4 principal (Principal: 1 id, 2 type enum)  7 payload (bytes)
  TrackingEvent  1 event_type (enum)  2 listing_id  3 session_id  4 anonymous_id  7 position  19 item_category
"""

from __future__ import annotations

import time
from dataclasses import dataclass

from recsys.nearline.signals import RawInteraction

TRACKING_EVENT_TYPE = "platform.analytics.v1.TrackingEvent"
PRINCIPAL_TYPE_USER = 2
# EventType enum values of analytics.proto that feed nearline signals.
_EVENT_NAMES = {1: "view", 2: "click", 3: "add_to_cart", 4: "impression"}


class DecodeError(ValueError):
    """The bytes are not a well-formed protobuf message."""


def _varint(buf: bytes, pos: int) -> tuple[int, int]:
    result = 0
    shift = 0
    while True:
        if pos >= len(buf):
            raise DecodeError("truncated varint")
        byte = buf[pos]
        pos += 1
        result |= (byte & 0x7F) << shift
        if not byte & 0x80:
            return result, pos
        shift += 7
        if shift > 63:
            raise DecodeError("varint longer than 10 bytes")


def _fields(buf: bytes):
    """Yield (field_number, value): an int for varint fields, bytes for the rest."""
    pos = 0
    while pos < len(buf):
        tag, pos = _varint(buf, pos)
        number, wire = tag >> 3, tag & 7
        if number == 0:
            raise DecodeError("field number 0")
        if wire == 0:
            value, pos = _varint(buf, pos)
        elif wire == 1:
            value, pos = buf[pos : pos + 8], pos + 8
        elif wire == 5:
            value, pos = buf[pos : pos + 4], pos + 4
        elif wire == 2:
            size, pos = _varint(buf, pos)
            value, pos = buf[pos : pos + size], pos + size
            if len(value) != size:
                raise DecodeError("truncated length-delimited field")
        else:
            raise DecodeError(f"unsupported wire type {wire}")
        if wire in (1, 5) and len(value) != (8 if wire == 1 else 4):
            raise DecodeError("truncated fixed-width field")
        yield number, value


def _str(value) -> str:
    if not isinstance(value, bytes):
        raise DecodeError("string field has a varint wire type")
    try:
        return value.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise DecodeError(f"invalid utf-8: {exc}") from None


@dataclass
class Envelope:
    event_id: str = ""
    type: str = ""
    occurred_at: float | None = None
    principal_id: str = ""
    principal_type: int = 0
    payload: bytes = b""


@dataclass
class Tracking:
    event_type: int = 0
    listing_id: str = ""
    session_id: str = ""
    anonymous_id: str = ""
    position: int = 0
    item_category: str = ""


def decode_envelope(data: bytes) -> Envelope:
    env = Envelope()
    for number, value in _fields(data):
        if number == 1:
            env.event_id = _str(value)
        elif number == 2:
            env.type = _str(value)
        elif number == 3 and isinstance(value, bytes):
            seconds = nanos = 0
            for n, v in _fields(value):
                if n == 1 and isinstance(v, int):
                    seconds = v - (1 << 64) if v >= 1 << 63 else v
                elif n == 2 and isinstance(v, int):
                    nanos = v
            env.occurred_at = seconds + nanos / 1e9
        elif number == 4 and isinstance(value, bytes):
            for n, v in _fields(value):
                if n == 1:
                    env.principal_id = _str(v)
                elif n == 2 and isinstance(v, int):
                    env.principal_type = v
        elif number == 7 and isinstance(value, bytes):
            env.payload = value
    return env


def decode_tracking(payload: bytes) -> Tracking:
    t = Tracking()
    for number, value in _fields(payload):
        if number == 1 and isinstance(value, int):
            t.event_type = value
        elif number == 2:
            t.listing_id = _str(value)
        elif number == 3:
            t.session_id = _str(value)
        elif number == 4:
            t.anonymous_id = _str(value)
        elif number == 7 and isinstance(value, int):
            t.position = value
        elif number == 19:
            t.item_category = _str(value)
    return t


def to_interaction(data: bytes, now: float | None = None) -> RawInteraction | None:
    """The nearline interaction a Kafka message carries; None for anything that is not one.

    Raises DecodeError for bytes that are not a protobuf message. Non-tracking envelopes, event types
    nearline does not use (checkout steps, favourites, ...) and events without a listing give None.
    """
    env = decode_envelope(data)
    if env.type != TRACKING_EVENT_TYPE:
        return None
    tracking = decode_tracking(env.payload)
    name = _EVENT_NAMES.get(tracking.event_type)
    if name is None or not tracking.listing_id:
        return None
    # The warehouse's user_key: the signed-in principal, else the visitor's anonymous id.
    if env.principal_type == PRINCIPAL_TYPE_USER and env.principal_id:
        actor = env.principal_id
    elif tracking.anonymous_id:
        actor = f"anon:{tracking.anonymous_id}"
    else:
        actor = ""
    return RawInteraction(
        user_id=actor,
        listing_id=tracking.listing_id,
        event_type=name,
        session_id=tracking.session_id,
        category=tracking.item_category,
        position=tracking.position,
        timestamp=env.occurred_at if env.occurred_at is not None else (now or time.time()),
        event_id=env.event_id,
    )
