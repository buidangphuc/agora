"""The analytics.events wire decoder (recsys.nearline.wire) against real protobuf bytes.

The GOLDEN_* hex strings were produced by the protobuf runtime from the contract in
``platform-core/packages/proto`` (``grpc_tools.protoc`` over analytics.proto, events.proto and
common.proto), so they pin the field numbers the decoder relies on.
"""

from __future__ import annotations

import pytest

from recsys.nearline.wire import DecodeError, to_interaction

# EventEnvelope(event_id="evt-1", type=TrackingEvent, occurred_at=1790000000.5s,
#   principal=USER "user-7" with scopes, traceparent, request_id,
#   payload=TrackingEvent(VIEW, listing-A, sess-1, anon-9, position 3, item_category electronics,
#                         page_path, properties))
GOLDEN_SIGNED_IN_VIEW = bytes.fromhex(
    "0a056576742d311223706c6174666f726d2e616e616c79746963732e76312e547261636b696e674576656e741a0c08"
    "80f7c4d5061080cab5ee0122100a06757365722d3710021a01611a01622a027470320272713a39080112096c697374"
    "696e672d411a06736573732d312206616e6f6e2d392a022f7838034a060a016b1201769a010b656c656374726f6e69"
    "6373"
)
# anonymous principal, CLICK on listing-B by visitor anon-9 (no session, no timestamp, no position)
GOLDEN_ANON_CLICK = bytes.fromhex(
    "0a056576742d321223706c6174666f726d2e616e616c79746963732e76312e547261636b696e674576656e74220d0a"
    "09616e6f6e796d6f757310013a15080212096c697374696e672d422206616e6f6e2d39"
)
# an envelope of another type (platform.listing.v1.ListingChanged)
GOLDEN_OTHER_TYPE = bytes.fromhex(
    "0a056576742d331222706c6174666f726d2e6c697374696e672e76312e4c697374696e674368616e6765643a030a0178"
)


def test_signed_in_view_decodes_to_the_users_interaction():
    ev = to_interaction(GOLDEN_SIGNED_IN_VIEW)
    assert ev is not None
    assert ev.user_id == "user-7"  # the principal, not the anonymous id
    assert ev.listing_id == "listing-A"
    assert ev.event_type == "view"
    assert ev.session_id == "sess-1"
    assert ev.category == "electronics"
    assert ev.position == 3
    assert ev.event_id == "evt-1"
    assert ev.timestamp == pytest.approx(1790000000.5)


def test_anonymous_click_is_keyed_like_the_warehouse_user_key():
    ev = to_interaction(GOLDEN_ANON_CLICK, now=123.0)
    assert ev is not None
    assert ev.user_id == "anon:anon-9"
    assert ev.event_type == "click" and ev.listing_id == "listing-B"
    assert ev.session_id == "" and ev.category == ""
    assert ev.timestamp == 123.0  # no occurred_at: the receipt time


def test_other_envelope_types_are_not_interactions():
    assert to_interaction(GOLDEN_OTHER_TYPE) is None


def test_event_types_nearline_does_not_use_are_skipped():
    # TrackingEvent(event_type=BEGIN_CHECKOUT(6), listing_id="l") inside a tracking envelope
    payload = bytes([0x08, 6, 0x12, 1]) + b"l"
    tracking_type = b"platform.analytics.v1.TrackingEvent"
    env = b"\x12" + bytes([len(tracking_type)]) + tracking_type + b"\x3a" + bytes([len(payload)]) + payload
    assert to_interaction(env) is None


@pytest.mark.parametrize(
    "garbage",
    [
        b"\x0a\x05ab",  # length-delimited field longer than the buffer
        b"\x0a",  # tag without a length
        b"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff",  # varint that never ends
        b"\x0b\x00",  # a group start tag (wire type 3)
        b"\x00\x01",  # field number 0
        b"\x0a\x02\xff\xfe",  # a string field that is not utf-8
    ],
)
def test_malformed_bytes_raise_decode_error(garbage):
    with pytest.raises(DecodeError):
        to_interaction(garbage)
