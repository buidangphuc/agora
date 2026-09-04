"""Interaction-mapping logic (event→weight, principal-vs-anonymous, empty drop).

Pure Python — runs on the host without PySpark. These same rules drive the Spark
column pipeline in interactions.py.
"""

from recsys import weights as W
from recsys.config import DEFAULT_EVENT_WEIGHTS


def test_event_weight_known_types():
    assert W.event_weight("view", DEFAULT_EVENT_WEIGHTS) == 1.0
    assert W.event_weight("click", DEFAULT_EVENT_WEIGHTS) == 2.0
    assert W.event_weight("add_to_cart", DEFAULT_EVENT_WEIGHTS) == 5.0
    assert W.event_weight("impression", DEFAULT_EVENT_WEIGHTS) == 0.5


def test_event_weight_case_insensitive():
    assert W.event_weight("VIEW", DEFAULT_EVENT_WEIGHTS) == 1.0
    assert W.event_weight(" Click ", DEFAULT_EVENT_WEIGHTS) == 2.0


def test_event_weight_unknown_gets_floor():
    assert W.event_weight("share", DEFAULT_EVENT_WEIGHTS) == W.UNKNOWN_EVENT_WEIGHT
    assert W.event_weight(None, DEFAULT_EVENT_WEIGHTS) == W.UNKNOWN_EVENT_WEIGHT


def test_user_key_prefers_principal():
    assert W.choose_user_key("user-1", "anon-9") == "user-1"


def test_user_key_falls_back_to_anonymous():
    assert W.choose_user_key("", "anon-9") == "anon-9"
    assert W.choose_user_key(None, "anon-9") == "anon-9"
    assert W.choose_user_key("   ", "anon-9") == "anon-9"


def test_user_key_none_when_both_empty():
    assert W.choose_user_key("", "") is None
    assert W.choose_user_key(None, None) is None


def test_empty_listing_dropped():
    assert W.is_valid_listing("listing-a") is True
    assert W.is_valid_listing("") is False
    assert W.is_valid_listing("   ") is False
    assert W.is_valid_listing(None) is False


def test_recency_multiplier():
    assert W.recency_multiplier(0, 7) == 1.0
    assert W.recency_multiplier(7, 7) == 0.5
    assert W.recency_multiplier(10, 0) == 1.0  # decay disabled
