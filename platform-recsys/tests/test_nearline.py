"""Unit tests for NearlineSignalAggregator and NearlineSignalStore."""

from __future__ import annotations

import time

from recsys.nearline.signals import (
    NearlineSignalAggregator,
    NearlineSignalStore,
    RawInteraction,
)


def test_nearline_recent_items_ordering() -> None:
    store = NearlineSignalStore()
    aggregator = NearlineSignalAggregator(store)

    t0 = time.time()
    aggregator.process_interaction(
        RawInteraction(user_id="u1", listing_id="item-A", event_type="view", timestamp=t0)
    )
    aggregator.process_interaction(
        RawInteraction(user_id="u1", listing_id="item-B", event_type="click", timestamp=t0 + 1)
    )
    aggregator.process_interaction(
        RawInteraction(user_id="u1", listing_id="item-C", event_type="view", timestamp=t0 + 2)
    )

    recents = store.get_recent_items("u1", limit=10)
    assert recents == ["item-C", "item-B", "item-A"]


def test_nearline_category_affinities() -> None:
    store = NearlineSignalStore()
    aggregator = NearlineSignalAggregator(store)

    aggregator.process_interaction(
        RawInteraction(user_id="u2", listing_id="item-1", event_type="view", category="Electronics")
    )
    aggregator.process_interaction(
        RawInteraction(user_id="u2", listing_id="item-2", event_type="add_to_cart", category="Electronics")
    )
    aggregator.process_interaction(
        RawInteraction(user_id="u2", listing_id="item-3", event_type="view", category="Books")
    )

    affinities = store.get_category_affinities("u2")
    assert affinities["Electronics"] == 6.0  # 1.0 view + 5.0 add_to_cart
    assert affinities["Books"] == 1.0


def test_nearline_realtime_coviews() -> None:
    store = NearlineSignalStore()
    aggregator = NearlineSignalAggregator(store)

    aggregator.process_interaction(
        RawInteraction(user_id="u3", listing_id="phone-1", event_type="view")
    )
    aggregator.process_interaction(
        RawInteraction(user_id="u3", listing_id="case-1", event_type="view")
    )

    coviews = store.get_coviewed_items("case-1")
    assert "phone-1" in coviews


def test_nearline_position_debiased_ctr() -> None:
    store = NearlineSignalStore()
    aggregator = NearlineSignalAggregator(store)

    # Item A displayed at pos 1 (weight = 1.0) and clicked
    aggregator.process_interaction(
        RawInteraction(user_id="u1", listing_id="item-A", event_type="click", position=1)
    )

    # Item B displayed at pos 4 (weight = sqrt(4) = 2.0) and clicked
    aggregator.process_interaction(
        RawInteraction(user_id="u2", listing_id="item-B", event_type="click", position=4)
    )

    ctr_a = store.get_debiased_ctr("item-A")
    ctr_b = store.get_debiased_ctr("item-B")

    assert ctr_a > 0.0
    assert ctr_b > 0.0
