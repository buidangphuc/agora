"""Steps for engagement-listing-rating-events.

Black box through the gateway: reviews, orders and HideReview are Connect JSON calls; the rating
fact is observed on engagement.events (binary EventEnvelope, payload decoded without stubs) and
compared with GetListingRatingSummary. The backfill scenario seeds review rows in engagement_db
and applies the 0015 up migration file to it.
"""

from __future__ import annotations

import base64
import json
import struct
import time
from pathlib import Path

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import efe_support as e
from tests.e2e.support import review_order_support as ros
from tests.e2e.support.oic_order_support import AUTH, Actor, OicWorld, create_listing, register

KIND = "ListingRatingChanged"
MIGRATION_UP = "0015_listing_rating_backfill.up.sql"


@pytest.fixture
def lre() -> OicWorld:
    w = OicWorld()
    w.data["rp_stopped"] = False
    yield w
    if w.data["rp_stopped"]:
        e.start_redpanda_and_settle()


def _lid(w: OicWorld, name: str) -> str:
    return w.listings[name]["id"]


def _review_of(w: OicWorld, buyer: str) -> str:
    return w.data["reviews"][buyer]


def _sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def _rating_payload(payload: bytes) -> dict:
    out: dict = {}
    for number, wire, value in _fields(payload):
        if wire == 2 and number == 1:
            out["listing_id"] = bytes(value).decode("utf-8", "replace")
        elif wire == 0 and number == 3:
            out["review_count"] = int(value)
    for number, avg in _doubles(payload):
        if number == 2:
            out["average_rating"] = avg
    out.setdefault("average_rating", 0.0)
    out.setdefault("review_count", 0)
    return out


def _doubles(buf: bytes) -> list[tuple[int, float]]:
    found: list[tuple[int, float]] = []
    pos = 0

    def varint(p: int) -> tuple[int, int]:
        shift = result = 0
        while True:
            b = buf[p]
            p += 1
            result |= (b & 0x7F) << shift
            if not b & 0x80:
                return result, p
            shift += 7

    while pos < len(buf):
        key, pos = varint(pos)
        number, wire = key >> 3, key & 7
        if wire == 0:
            _, pos = varint(pos)
        elif wire == 2:
            size, pos = varint(pos)
            pos += size
        elif wire == 1:
            found.append((number, struct.unpack("<d", buf[pos : pos + 8])[0]))
            pos += 8
        elif wire == 5:
            pos += 4
        else:
            break
    return found


def rating_events(listing_id: str, timeout_s: float = 20.0) -> list[dict]:
    out = []
    for fact in e.facts(listing_id, KIND, timeout_s=timeout_s):
        if fact["key"] != listing_id:
            continue
        fact.update(_rating_payload(fact["payload_bytes"]))
        if fact.get("listing_id") == listing_id:
            out.append(fact)
    return out


def _matches(fact: dict, count: int, avg: float) -> bool:
    return fact["review_count"] == count and abs(fact["average_rating"] - avg) < 1e-9


def wait_rating(
    listing_id: str, count: int, avg: float, timeout_s: float = 60.0
) -> tuple[dict, list[dict]]:
    deadline = time.monotonic() + timeout_s
    seen: list[dict] = []
    while time.monotonic() < deadline:
        seen = rating_events(listing_id, timeout_s=6.0)
        hits = [f for f in seen if _matches(f, count, avg)]
        if hits:
            return hits[-1], seen
        time.sleep(1)
    got = [(f["review_count"], f["average_rating"]) for f in seen]
    raise AssertionError(
        f"no {KIND} for {listing_id} with count {count} avg {avg} on {e.TOPIC}; saw {got}"
    )


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.re(r'a seller with a published listing "(?P<name>[^"]+)"'))
def seller_listing(lre, name):
    create_listing(lre, name, register(lre, "seller", "seller"), 50)


@given(parsers.re(r'a buyer "(?P<name>[^"]+)" with a completed order of "(?P<listing>[^"]+)"'))
def buyer_completed(lre, name, listing):
    buyer = register(lre, name, "buyer")
    lre.orders[name] = ros.completed_order_for(lre, buyer, lre.actors["seller"], _lid(lre, listing))


@given(parsers.re(r'a buyer "(?P<name>[^"]+)"$'))
def plain_buyer(lre, name):
    register(lre, name, "buyer")


def _review(w: OicWorld, name: str, listing: str, rating: int, order: str) -> None:
    resp = ros.create_review(
        w.actors[name].token, _lid(w, listing), rating, f"lre-{name}-{rating}", order
    )
    w.responses["review"] = resp
    if resp.status_code == 200:
        w.data.setdefault("reviews", {})[name] = resp.json()["review"]["id"]


@given(
    parsers.re(
        r'"(?P<name>[^"]+)" has reviewed "(?P<listing>[^"]+)" with rating (?P<rating>\d+) from their completed order'
    )
)
def has_reviewed(lre, name, listing, rating):
    _review(lre, name, listing, int(rating), lre.orders[name])
    assert lre.responses["review"].status_code == 200, lre.responses["review"].text


@given("an admin")
def an_admin(lre):
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = ros.post(None, AUTH, "Login", {"username": admin.username, "password": admin.password})
    token = (ros.ok(resp).get("result") or {}).get("token", "")
    assert token, f"admin login failed: {resp.text[:200]}"
    lre.actors["admin"] = Actor(admin.username, token, "admin", _sub(token))


@given(
    parsers.re(
        r'the rating event of "(?P<listing>[^"]+)" with review count (?P<count>\d+) and average (?P<avg>[\d.]+) has reached engagement.events'
    )
)
def control_reached(lre, listing, count, avg):
    _, seen = wait_rating(_lid(lre, listing), int(count), float(avg))
    lre.data["rating_seen"] = len(seen)


@given(
    parsers.re(
        r'listing "(?P<listing>[^"]+)" has (?P<n>\d+) verified review rows seeded directly in the database'
    )
)
def seed_rows(lre, listing, n):
    ros.seed_verified_reviews_sql(_lid(lre, listing), lre.actors["seller"].user_id, int(n))


# ── When ─────────────────────────────────────────────────────────────────
@when(
    parsers.re(
        r'"(?P<name>[^"]+)" reviews "(?P<listing>[^"]+)" with rating (?P<rating>\d+) from their completed order'
    )
)
def reviews_own_order(lre, name, listing, rating):
    _review(lre, name, listing, int(rating), lre.orders[name])
    assert lre.responses["review"].status_code == 200, lre.responses["review"].text


@when(
    parsers.re(
        r'"(?P<name>[^"]+)" reviews "(?P<listing>[^"]+)" with rating (?P<rating>\d+) from the completed order of "(?P<owner>[^"]+)"'
    )
)
def reviews_other_order(lre, name, listing, rating, owner):
    _review(lre, name, listing, int(rating), lre.orders[owner])


def _hide(w: OicWorld, token: str, review_id: str) -> None:
    w.responses["hide"] = ros.post(
        token, ros.ENGAGEMENT, "HideReview", {"reviewId": review_id, "reason": "lre"}
    )


@given(parsers.re(r'the admin hides the review of "(?P<name>[^"]+)"$'))
@when(parsers.re(r'the admin hides the review of "(?P<name>[^"]+)"$'))
def admin_hides(lre, name):
    _hide(lre, lre.actors["admin"].token, _review_of(lre, name))


@when(parsers.re(r'"(?P<name>[^"]+)" hides their own review$'))
def buyer_hides(lre, name):
    _hide(lre, lre.actors[name].token, _review_of(lre, name))


@when(
    parsers.re(
        r'Redpanda is stopped, "(?P<name>[^"]+)" reviews "(?P<listing>[^"]+)" with rating (?P<rating>\d+) from their completed order, and Redpanda is started again'
    )
)
def kafka_down_review(lre, name, listing, rating):
    lre.data["rp_stopped"] = True
    e.stop_redpanda()
    _review(lre, name, listing, int(rating), lre.orders[name])
    assert lre.responses["review"].status_code == 200, lre.responses["review"].text
    e.start_redpanda_and_settle()
    lre.data["rp_stopped"] = False


@when("migration 0015 is applied to the engagement database")
def apply_migration(lre):
    path = Path(__file__).resolve().parents[4] / "team-engagement" / "migrations" / MIGRATION_UP
    assert path.is_file(), f"{path} does not exist"
    ros.engagement_psql(path.read_text("utf-8"))


# ── Then ─────────────────────────────────────────────────────────────────
@then(parsers.parse('the review call fails with "{code}"'))
def review_fails(lre, code):
    assert ros.code_of(lre.responses["review"]) == code, lre.responses["review"].text


@then("the hide call succeeds")
def hide_ok(lre):
    assert ros.code_of(lre.responses["hide"]) == "ok", lre.responses["hide"].text


@then(parsers.parse('the hide call is refused with HTTP {status:d} and "{code}"'))
def hide_refused(lre, status, code):
    resp = lre.responses["hide"]
    assert resp.status_code == status, (resp.status_code, resp.text)
    assert ros.code_of(resp) == code, resp.text


@then(
    parsers.re(
        r'a "ListingRatingChanged" envelope for "(?P<listing>[^"]+)" with review count (?P<count>\d+) and average (?P<avg>[\d.]+) appears on engagement.events$'
    )
)
def rating_appears(lre, listing, count, avg):
    lid = _lid(lre, listing)
    fact, _ = wait_rating(lid, int(count), float(avg))
    lre.data["rating_fact"] = fact


@then(
    parsers.re(
        r'a "ListingRatingChanged" envelope for "(?P<listing>[^"]+)" with review count (?P<count>\d+) and average (?P<avg>[\d.]+) appears on engagement.events after the "ReviewHidden" of the review of "(?P<name>[^"]+)"'
    )
)
def rating_after_hidden(lre, listing, count, avg, name):
    lid = _lid(lre, listing)
    hidden = e.wait_facts(_review_of(lre, name), "ReviewHidden", 1, timeout_s=60)[0]
    fact, _ = wait_rating(lid, int(count), float(avg))
    assert fact["key"] == hidden["key"] == lid, (fact["key"], hidden["key"])
    assert fact["offset"] > hidden["offset"], (fact["offset"], hidden["offset"])


@then(
    parsers.re(
        r'exactly one "ListingRatingChanged" envelope for "(?P<listing>[^"]+)" with review count (?P<count>\d+) and average (?P<avg>[\d.]+) appears on engagement.events'
    )
)
def rating_exactly_one(lre, listing, count, avg):
    lid = _lid(lre, listing)
    wait_rating(lid, int(count), float(avg))
    time.sleep(4)
    seen = rating_events(lid, timeout_s=6.0)
    assert len(seen) == 1, [(f["review_count"], f["average_rating"]) for f in seen]
    assert _matches(seen[0], int(count), float(avg)), seen[0]


@then(
    parsers.re(
        r'GetListingRatingSummary of "(?P<listing>[^"]+)" returns review count (?P<count>\d+) and average (?P<avg>[\d.]+)'
    )
)
def summary_equals(lre, listing, count, avg):
    got = ros.listing_summary(_lid(lre, listing))
    assert int(got.get("reviewCount", 0)) == int(count), got
    assert abs(float(got.get("averageRating", 0)) - float(avg)) < 1e-9, got


@then(
    parsers.re(r'no "ListingRatingChanged" envelope for "(?P<listing>[^"]+)" carries a principal')
)
def no_principal(lre, listing):
    seen = rating_events(_lid(lre, listing))
    assert seen, "no ListingRatingChanged to inspect"
    for fact in seen:
        assert fact["principal_id"] == "", fact["principal_id"]


@then(
    parsers.re(
        r'no further "ListingRatingChanged" envelope for "(?P<listing>[^"]+)" appears on engagement.events within (?P<s>\d+) seconds'
    )
)
def no_further(lre, listing, s):
    time.sleep(int(s))
    seen = rating_events(_lid(lre, listing), timeout_s=8.0)
    assert len(seen) == lre.data["rating_seen"], [
        (f["review_count"], f["average_rating"]) for f in seen
    ]


@then(
    parsers.re(
        r'the outbox row of that rating event of "(?P<listing>[^"]+)" is published and was never parked'
    )
)
def outbox_published(lre, listing):
    lid = ros.sql_lit(_lid(lre, listing))
    out = ros.engagement_psql(
        "SELECT status FROM engagement_outbox_events "
        f"WHERE aggregate_id = {lid} AND event_type LIKE '%ListingRatingChanged'"
    )
    assert out.splitlines() == ["published"], out
