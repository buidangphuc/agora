"""Nearline cart and checkout signals (spec recommendations: cart and checkout session signals).

EventType 11 = view_cart (weight 3.0), 6 = begin_checkout (weight 6.0); both of consented actors only.
Aggregator-level cases assume RawInteraction.event_type names ``"view_cart"`` / ``"begin_checkout"``.
"""

from __future__ import annotations

import pytest

from recsys.nearline.signals import NearlineSignalAggregator, NearlineSignalStore, RawInteraction
from tests.test_nearline_consent import _dump, _run, consent, event
from tests.test_nearline_consumer import Msg

fakeredis = pytest.importorskip("fakeredis")

VIEW_CART, BEGIN_CHECKOUT = 11, 6
GRANT = consent(True)


def _agg():
    client = fakeredis.FakeRedis(decode_responses=True)
    store = NearlineSignalStore(client)
    return client, store, NearlineSignalAggregator(store, ttl_seconds=10**12)


def test_checkout_events_raise_category_affinity():
    """Scenario: Checkout events raise category affinity (3.0 + 6.0 = 9.0, via the decoder)."""
    client, store, _ = _run(
        [
            Msg(event("u-buy", "L1", VIEW_CART, consent_bytes=GRANT, event_id="e1")),
            Msg(event("u-buy", "L1", BEGIN_CHECKOUT, consent_bytes=GRANT, event_id="e2")),
        ]
    )
    assert client.hget("recs:nearline:user:u-buy:cats", "books") == pytest.approx(9.0)
    assert store.get_recent_items("u-buy") == ["L1"]


def test_checkout_weights_per_event_type():
    """Scenario: Checkout events raise category affinity (weights 3.0 and 6.0)."""
    _, store, agg = _agg()
    agg.process_interaction(
        RawInteraction("u1", "L1", "view_cart", category="books", timestamp=1.0, event_id="a")
    )
    assert store.get_category_affinities("u1") == {"books": pytest.approx(3.0)}
    agg.process_interaction(
        RawInteraction("u1", "L1", "begin_checkout", category="books", timestamp=2.0, event_id="b")
    )
    assert store.get_category_affinities("u1") == {"books": pytest.approx(9.0)}


def test_checkout_of_denied_actor_changes_nothing():
    """Scenario: Checkout events raise category affinity (consented actors only)."""
    client, _, _ = _run([Msg(event("u-deny", "L1", BEGIN_CHECKOUT, consent_bytes=consent(False)))])
    assert _dump(client) == {}


def test_cart_event_without_listing_is_skipped():
    """Scenario: Cart event without a listing is skipped."""
    client, _, source = _run(
        [
            Msg(event("u-buy", "", VIEW_CART, consent_bytes=GRANT, event_id="e1")),
            Msg(event("u-buy", "L2", VIEW_CART, consent_bytes=GRANT, event_id="e2")),
        ]
    )
    # the empty-listing event changed nothing; the consumer kept running and handled the next one
    assert client.zrevrange("recs:nearline:user:u-buy:items", 0, -1) == ["L2"]
    assert client.hget("recs:nearline:user:u-buy:cats", "books") == pytest.approx(3.0)
    assert source.commits >= 1


def test_checkout_redelivery_applied_once():
    """Scenario: Redelivery of a checkout event is applied once."""
    client, _, _ = _run(
        [
            Msg(event("u-buy", "L1", BEGIN_CHECKOUT, consent_bytes=GRANT, event_id="dup")),
            Msg(event("u-buy", "L1", BEGIN_CHECKOUT, consent_bytes=GRANT, event_id="dup")),
        ]
    )
    assert client.hget("recs:nearline:user:u-buy:cats", "books") == pytest.approx(6.0)
