"""Unit tests for NearlineSignalAggregator and NearlineSignalStore."""

from __future__ import annotations

import time

import pytest

from recsys.nearline.signals import (
    MAX_RECENT_ITEMS,
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

    aggregator.process_interaction(RawInteraction(user_id="u3", listing_id="phone-1", event_type="view"))
    aggregator.process_interaction(RawInteraction(user_id="u3", listing_id="case-1", event_type="view"))

    coviews = store.get_coviewed_items("case-1")
    assert "phone-1" in coviews


def test_nearline_position_debiased_ctr() -> None:
    store = NearlineSignalStore()
    aggregator = NearlineSignalAggregator(store)

    for item, pos in (("item-A", 1), ("item-B", 4)):
        aggregator.process_interaction(
            RawInteraction(user_id="u0", listing_id=item, event_type="impression", position=pos)
        )

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


# ── Redis-backed layout (the contract team-ai reads) and the spec scenarios ─────────────────────


def _redis_store():
    fakeredis = pytest.importorskip("fakeredis")
    client = fakeredis.FakeRedis(decode_responses=True)
    return client, NearlineSignalStore(client)


def test_redis_recent_views_and_category_affinities():
    """Scenario: User recent views update nearline signals."""
    client, store = _redis_store()
    agg = NearlineSignalAggregator(store, ttl_seconds=3600)
    t0 = time.time()
    agg.process_interaction(
        RawInteraction("u1", "item-A", "view", session_id="s1", category="phones", timestamp=t0)
    )
    agg.process_interaction(
        RawInteraction("u1", "item-B", "view", session_id="s1", category="laptops", timestamp=t0 + 1)
    )
    assert store.get_recent_items("u1") == ["item-B", "item-A"]
    assert store.get_category_affinities("u1") == {"phones": 1.0, "laptops": 1.0}
    # the layout, as documented in the change's design.md
    assert client.zrevrange("recs:nearline:user:u1:items", 0, -1) == ["item-B", "item-A"]
    assert client.hgetall("recs:nearline:user:u1:cats") == {"phones": "1", "laptops": "1"}
    assert 0 < client.ttl("recs:nearline:user:u1:items") <= 3600
    assert 0 < client.ttl("recs:nearline:user:u1:cats") <= 3600


def test_redis_coview_counts_accumulate_across_users_in_their_sessions():
    """Scenario: Real-time item co-occurrence is tracked."""
    client, store = _redis_store()
    agg = NearlineSignalAggregator(store, ttl_seconds=3600)
    t0 = time.time()
    for n, user in enumerate(("u1", "u2")):
        sid = f"session-{user}"
        agg.process_interaction(RawInteraction(user, "item-A", "view", session_id=sid, timestamp=t0 + n))
        agg.process_interaction(RawInteraction(user, "item-B", "view", session_id=sid, timestamp=t0 + n + 1))
    assert store.get_coview_counts("item-A") == [("item-B", 2.0)]
    assert store.get_coview_counts("item-B") == [("item-A", 2.0)]
    assert store.get_coviewed_items("item-A") == ["item-B"]
    assert 0 < client.ttl("recs:nearline:coview:item-A") <= 3600


def test_listings_viewed_in_different_sessions_are_not_coviewed():
    _, store = _redis_store()
    agg = NearlineSignalAggregator(store)
    agg.process_interaction(RawInteraction("u1", "item-A", "view", session_id="s1"))
    agg.process_interaction(RawInteraction("u2", "item-B", "view", session_id="s2"))
    assert store.get_coview_counts("item-A") == []


@pytest.mark.parametrize("redis_backed", [True, False])
def test_recents_and_coviews_are_bounded(redis_backed):
    store = _redis_store()[1] if redis_backed else NearlineSignalStore()
    agg = NearlineSignalAggregator(store)
    t0 = time.time()
    for i in range(60):
        agg.process_interaction(RawInteraction("u1", f"item-{i}", "view", session_id="s1", timestamp=t0 + i))
    recents = store.get_recent_items("u1", limit=100)
    assert len(recents) == MAX_RECENT_ITEMS
    assert recents[0] == "item-59"


@pytest.mark.parametrize("redis_backed", [True, False])
def test_impressions_feed_ctr_only(redis_backed):
    store = _redis_store()[1] if redis_backed else NearlineSignalStore()
    agg = NearlineSignalAggregator(store)
    agg.process_interaction(RawInteraction("u1", "item-A", "impression", session_id="s1", category="x"))
    agg.process_interaction(RawInteraction("u1", "item-A", "click", session_id="s1", category="x"))
    assert store.get_recent_items("u1") == ["item-A"]  # the click, not the impression, put it there
    assert store.get_category_affinities("u1") == {"x": 2.0}
    assert 0.0 < store.get_debiased_ctr("item-A") <= 1.0


@pytest.mark.parametrize("redis_backed", [True, False])
def test_a_redelivered_event_is_applied_once(redis_backed):
    store = _redis_store()[1] if redis_backed else NearlineSignalStore()
    agg = NearlineSignalAggregator(store)
    ev = RawInteraction("u1", "item-A", "view", category="x", event_id="evt-1")
    assert agg.process_interaction(ev) is True
    assert agg.process_interaction(ev) is False
    assert store.get_category_affinities("u1") == {"x": 1.0}


@pytest.mark.parametrize("redis_backed", [True, False])
def test_events_older_than_the_window_are_ignored(redis_backed):
    store = _redis_store()[1] if redis_backed else NearlineSignalStore()
    agg = NearlineSignalAggregator(store, ttl_seconds=60)
    old = RawInteraction("u1", "item-A", "view", timestamp=time.time() - 3600)
    assert agg.process_interaction(old) is False
    assert store.get_recent_items("u1") == []


@pytest.mark.parametrize("redis_backed", [True, False])
def test_equal_raw_ctr_at_worse_positions_gets_higher_debiased_ctr(redis_backed):
    store = _redis_store()[1] if redis_backed else NearlineSignalStore()
    agg = NearlineSignalAggregator(store)
    for listing, position in (("top", 1), ("deep", 9)):
        for _ in range(4):
            agg.process_interaction(RawInteraction("u", listing, "impression", position=position))
        agg.process_interaction(RawInteraction("u", listing, "click", position=position))
    # raw CTR is 1/4 for both; the click at position 9 counts sqrt(9)=3 times
    assert store.get_debiased_ctr("top") == pytest.approx(0.25)
    assert store.get_debiased_ctr("deep") == pytest.approx(0.75)
