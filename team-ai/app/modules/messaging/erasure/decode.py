"""Decode ``identity.events`` values into the erased user id.

The wire contract is ``platform.events.v1.EventEnvelope`` (ADR-0002); only
``platform.identity.v1.UserErased`` matters and its field 1 is the user id.
"""

from app.transport.grpc._pb.platform.events.v1 import events_pb2

USER_ERASED_TYPE = "platform.identity.v1.UserErased"


class UndecodableEventError(Exception):
    """The value is not a decodable UserErased event: retrying cannot help."""


def _varint(data: bytes, pos: int) -> tuple[int, int]:
    shift = result = 0
    while True:
        if pos >= len(data) or shift > 63:
            raise UndecodableEventError("truncated varint")
        byte = data[pos]
        pos += 1
        result |= (byte & 0x7F) << shift
        if not byte & 0x80:
            return result, pos
        shift += 7


def _user_id(payload: bytes) -> str:
    pos = 0
    found = b""
    while pos < len(payload):
        tag, pos = _varint(payload, pos)
        number, wire = tag >> 3, tag & 7
        if wire == 0:
            _, pos = _varint(payload, pos)
        elif wire == 1:
            pos += 8
        elif wire == 5:
            pos += 4
        elif wire == 2:
            size, pos = _varint(payload, pos)
            if pos + size > len(payload):
                raise UndecodableEventError("truncated field")
            if number == 1:
                found = payload[pos : pos + size]
            pos += size
        else:
            raise UndecodableEventError("unsupported wire type")
    try:
        return found.decode()
    except UnicodeDecodeError as exc:
        raise UndecodableEventError("user id is not utf-8") from exc


def decode_user_erased(value: bytes) -> str | None:
    """Return the user id of a UserErased envelope, ``None`` for other types."""
    envelope = events_pb2.EventEnvelope()
    try:
        envelope.ParseFromString(value)
    except Exception as exc:
        raise UndecodableEventError("envelope") from exc
    if envelope.type != USER_ERASED_TYPE:
        return None
    user_id = _user_id(envelope.payload)
    if not user_id:
        raise UndecodableEventError("UserErased has no user id")
    return user_id
