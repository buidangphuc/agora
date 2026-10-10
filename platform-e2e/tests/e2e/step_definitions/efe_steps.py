"""Steps for engagement-fact-events (area efe-e2e), shared by the Kafka and warehouse features.

Real, distinct users per scenario (one seller, its buyers); every RPC goes through the gateway.
Facts are asserted on engagement.events and in the DuckDB warehouse, never through mocks.
"""

from __future__ import annotations

import time
import uuid

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import efe_support as e
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import Actor, OicWorld, register


@pytest.fixture
def efe():
    w = OicWorld()
    w.data["rp_stopped"] = False
    yield w
    if w.data["rp_stopped"]:  # never leave redpanda stopped for the next scenario
        e.start_redpanda_and_settle()


def _buyer(w: OicWorld, name: str) -> Actor:
    return w.actors[name]


def _seller(w: OicWorld) -> Actor:
    return w.actors["seller"]


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse('a seller with {n:d} published listing "{name}"'))
def seller_one(efe, n, name):
    e.seller_with_listings(efe, n)
    efe.listings[name] = efe.listings.pop("L1")


@given(parsers.re(r"a seller with (?P<n>\d+) published listings"))
def seller_many(efe, n):
    e.seller_with_listings(efe, int(n))


@given(parsers.parse('a buyer "{name}"'))
def a_buyer(efe, name):
    register(efe, name, "buyer")


@given(parsers.parse('a buyer "{name}" with a delivered order of "{listing}"'))
def buyer_delivered(efe, name, listing):
    buyer = register(efe, name, "buyer")
    efe.orders[name] = e.delivered_order(efe, buyer, _seller(efe), listing)


# ── When / Given (action) ────────────────────────────────────────────────
@given(parsers.parse('"{name}" adds "{listing}" to their favourites through the gateway'))
@when(parsers.parse('"{name}" adds "{listing}" to their favourites through the gateway'))
def add_fav(efe, name, listing):
    e.favorite(efe, _buyer(efe, name), listing)


@when(parsers.parse('"{name}" removes "{listing}" from their favourites through the gateway'))
def remove_fav(efe, name, listing):
    e.unfavorite(efe, _buyer(efe, name), listing)


@when(parsers.parse('"{name}" adds "L1" and "L2" to their favourites and then removes "L1"'))
def add_two_remove_first(efe, name):
    b = _buyer(efe, name)
    e.favorite(efe, b, "L1")
    e.favorite(efe, b, "L2")
    e.unfavorite(efe, b, "L1")


@when(parsers.parse('"{name}" follows the seller'))
def follow_seller(efe, name):
    e.follow(efe, _buyer(efe, name), _seller(efe))


@when(parsers.parse('"{name}" reviews "{listing}" with rating {rating:d} and the text "{text}"'))
def create_review(efe, name, listing, rating, text):
    efe.data["review_text"] = text
    e.review(efe, _buyer(efe, name), listing, rating, text, efe.orders[name])


@when(
    parsers.parse('Redpanda is stopped, "{name}" follows the seller, and Redpanda is started again')
)
def kafka_down_follow(efe, name):
    efe.data["rp_stopped"] = True
    e.stop_redpanda()
    e.follow(efe, _buyer(efe, name), _seller(efe))
    e.start_redpanda_and_settle()
    efe.data["rp_stopped"] = False


@when("a malformed record is produced to engagement.events")
def malformed(efe):
    marker = f"efe-malformed-{uuid.uuid4().hex}"
    efe.data["malformed"] = marker
    stack.produce(e.TOPIC, marker.encode(), b"\x00not-an-event-envelope:" + marker.encode())


# ── Then: Kafka ──────────────────────────────────────────────────────────
@then(
    parsers.parse('one "{kind}" envelope for "{name}" and "{listing}" appears on engagement.events')
)
@then(
    parsers.parse(
        'exactly one "{kind}" envelope for "{name}" and "{listing}" appears on engagement.events'
    )
)
def one_fact_listing(efe, kind, name, listing):
    buyer = _buyer(efe, name)
    got = e.wait_facts(buyer.user_id, kind, 1)[0]
    lid = e.listing_id(efe, listing)
    assert got["payload"]["user_id"] == buyer.user_id, got
    assert got["payload"]["listing_id"] == lid, got
    assert got["principal_id"] == buyer.user_id, f"envelope principal is not the caller: {got}"
    assert got["key"] == lid, f"record key must be the listing id: {got['key']!r}"


@then(
    parsers.parse(
        'a "FavoriteRemoved" envelope for "{name}" and "{listing}" follows the "FavoriteAdded" on engagement.events'
    )
)
def removal_follows_add(efe, name, listing):
    buyer = _buyer(efe, name)
    lid = e.listing_id(efe, listing)
    added = e.wait_facts(buyer.user_id, "FavoriteAdded", 1)[0]
    removed = e.wait_facts(buyer.user_id, "FavoriteRemoved", 1)[0]
    assert removed["payload"]["user_id"] == buyer.user_id
    assert removed["payload"]["listing_id"] == lid
    assert removed["key"] == added["key"] == lid
    assert removed["offset"] > added["offset"], (added["offset"], removed["offset"])


@then(
    parsers.parse(
        'a "ReviewCreated" envelope with rating {rating:d}, "{listing}" and its seller appears on engagement.events'
    )
)
def review_fact(efe, rating, listing):
    buyer = efe.actors["b1"]
    got = e.wait_facts(buyer.user_id, "ReviewCreated", 1)[0]
    p = got["payload"]
    assert p["rating"] == rating, got
    assert p["listing_id"] == e.listing_id(efe, listing), got
    assert p["seller_id"] == _seller(efe).user_id, got
    assert p["user_id"] == buyer.user_id, got
    efe.data["review_fact"] = got


@then(parsers.parse('the "ReviewCreated" payload does not contain "{text}"'))
def review_no_text(efe, text):
    got = efe.data["review_fact"]
    assert text.encode() not in got["payload_bytes"], "review text leaked into the payload"
    assert text.encode() not in got["raw"], "review text leaked into the envelope"


@then(
    parsers.parse(
        'one "SellerFollowed" envelope for "{name}" and the seller appears on engagement.events'
    )
)
def follow_fact(efe, name):
    buyer, seller = _buyer(efe, name), _seller(efe)
    got = e.wait_facts(buyer.user_id, "SellerFollowed", 1, timeout_s=180)[0]
    assert got["payload"]["user_id"] == buyer.user_id, got
    assert got["payload"]["seller_id"] == seller.user_id, got
    assert got["key"] == seller.user_id, f"follow record key must be the seller id: {got['key']!r}"


# ── Then: warehouse ──────────────────────────────────────────────────────
@then(
    parsers.parse(
        'engagement_facts holds three rows for "{name}" and favorites_current lists only "L2" for "{name2}"'
    )
)
def warehouse_favorites(efe, name, name2):
    uid = _buyer(efe, name).user_id
    rows = tii.warehouse_rows(
        "SELECT fact, listing_id FROM engagement_facts WHERE user_id = ?",
        [uid],
        until=lambda r: len(r) >= 3,
        settle_s=4.0,
    )
    l1, l2 = e.listing_id(efe, "L1"), e.listing_id(efe, "L2")
    assert sorted(rows) == sorted(
        [("favorite_added", l1), ("favorite_added", l2), ("favorite_removed", l1)]
    ), rows
    current = tii.warehouse_rows(
        "SELECT listing_id FROM favorites_current WHERE user_id = ?",
        [uid],
        until=lambda r: r == [(l2,)],
    )
    assert current == [(l2,)], current


@then(parsers.parse('follows_current lists "{name}" and the seller'))
def warehouse_follow(efe, name):
    uid, sid = _buyer(efe, name).user_id, _seller(efe).user_id
    rows = tii.warehouse_rows(
        "SELECT user_id, seller_id FROM follows_current WHERE user_id = ?",
        [uid],
        until=lambda r: (uid, sid) in r,
    )
    assert rows == [(uid, sid)], rows


@then("the malformed record appears on engagement.events.analytics.dlq")
def malformed_on_dlq(efe):
    marker = efe.data["malformed"].encode()
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        if stack.find_records(e.DLQ_TOPIC, marker, timeout_s=5):
            return
    raise AssertionError(f"malformed record {marker!r} is not on {e.DLQ_TOPIC}")


@then(parsers.parse('the favourite of "{name}" and "{listing}" reaches engagement_facts'))
def favourite_in_facts(efe, name, listing):
    uid, lid = _buyer(efe, name).user_id, e.listing_id(efe, listing)
    rows = tii.warehouse_rows(
        "SELECT fact FROM engagement_facts WHERE user_id = ? AND listing_id = ?",
        [uid, lid],
        until=lambda r: bool(r),
    )
    assert rows == [("favorite_added",)], rows
