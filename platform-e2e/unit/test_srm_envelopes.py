"""Unit tests (no stack): the srm envelope encoder round-trips through the e2e decoders."""

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.oic_inv_events_flow import STOCK_CHANGED_TYPE, decode_stock_changed

T = 1_760_000_000_123_456_789


def test_stock_changed_decodes_with_the_existing_stock_decoder():
    raw = ev.stock_changed("lst-1", 8, T)
    assert decode_stock_changed(raw) == {
        "type": STOCK_CHANGED_TYPE,
        "listing_id": "lst-1",
        "stock": 8,
    }
    info = ev.decode(raw)
    assert (info["type"], info["listing_id"], info["stock"]) == (ev.STOCK_CHANGED, "lst-1", 8)
    assert info["occurred_at_ns"] == T


def test_negative_stock_survives_the_round_trip():
    info = ev.decode(ev.stock_changed("lst-1", -1, T))
    assert info["stock"] == (1 << 64) - 1  # int32 -1 is a 10-byte varint


def test_missing_occurred_at_and_empty_listing_id():
    assert ev.decode(ev.stock_changed("lst-1", 5, None))["occurred_at_ns"] is None
    assert ev.decode(ev.stock_changed("", 5, T))["listing_id"] == ""


def test_listing_changed_carries_change_type_status_and_stock():
    listing = ev.listing_msg("lst-2", title="t", status=ev.PUBLISHED, stock=10)
    info = ev.decode(ev.listing_changed(ev.CREATED, listing, T))
    assert info["type"] == ev.LISTING_CHANGED
    assert (info["listing_id"], info["change_type"], info["status"], info["stock"]) == (
        "lst-2",
        ev.CREATED,
        ev.PUBLISHED,
        10,
    )
    assert info["occurred_at_ns"] == T


def test_status_and_base_info_events():
    st = ev.decode(ev.status_changed("lst-3", ev.PUBLISHED, T))
    assert (st["type"], st["listing_id"], st["status"]) == (
        ev.STATUS_CHANGED,
        "lst-3",
        ev.PUBLISHED,
    )
    base = ev.decode(
        ev.base_info_changed("lst-4", title="x", change_type=ev.DELETED, occurred_at_ns=T)
    )
    assert (base["listing_id"], base["change_type"]) == ("lst-4", ev.DELETED)
