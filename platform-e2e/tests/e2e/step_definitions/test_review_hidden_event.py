"""Binds engagement/review_hidden_event.feature (engagement-review-hidden-event).

Black box through the gateway: reviews, orders and HideReview are Connect JSON calls; the fact is
observed on engagement.events (binary EventEnvelope) and in the analytics warehouse.
"""

from __future__ import annotations

import base64
import json
import time

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import efe_support as e
from tests.e2e.support import review_order_support as ros
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import AUTH, Actor, OicWorld, create_listing, register

scenarios("engagement/review_hidden_event.feature")


@pytest.fixture
def rhe() -> OicWorld:
    return OicWorld()


def _lid(w: OicWorld, name: str) -> str:
    return w.listings[name]["id"]


def _review_of(w: OicWorld, buyer: str) -> str:
    return w.data["reviews"][buyer]


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse('a seller with a published listing "{name}"'))
def seller_listing(rhe, name):
    create_listing(rhe, name, register(rhe, "seller", "seller"), 50)


@given(parsers.parse('a buyer "{name}" with a completed order of "{listing}"'))
def buyer_completed(rhe, name, listing):
    buyer = register(rhe, name, "buyer")
    rhe.orders[name] = ros.completed_order_for(
        rhe, buyer, rhe.actors["seller"], _lid(rhe, listing)
    )


@given(
    parsers.parse(
        '"{name}" has reviewed "{listing}" with rating {rating:d} from their completed order'
    )
)
def has_reviewed(rhe, name, listing, rating):
    resp = ros.create_review(
        rhe.actors[name].token, _lid(rhe, listing), rating, f"rhe-{name}-{rating}", rhe.orders[name]
    )
    assert resp.status_code == 200, resp.text
    rhe.data.setdefault("reviews", {})[name] = resp.json()["review"]["id"]


def _sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


@given("an admin")
def an_admin(rhe):
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = ros.post(None, AUTH, "Login", {"username": admin.username, "password": admin.password})
    token = (ros.ok(resp).get("result") or {}).get("token", "")
    assert token, f"admin login failed: {resp.text[:200]}"
    rhe.actors["admin"] = Actor(admin.username, token, "admin", _sub(token))


def _hide(rhe: OicWorld, token: str | None, review_id: str, reason: str) -> None:
    rhe.responses["hide"] = ros.post(
        token, ros.ENGAGEMENT, "HideReview", {"reviewId": review_id, "reason": reason}
    )


@given(parsers.parse('the admin hides the review of "{name}" with the reason "{reason}"'))
@when(parsers.parse('the admin hides the review of "{name}" with the reason "{reason}"'))
def admin_hides(rhe, name, reason):
    _hide(rhe, rhe.actors["admin"].token, _review_of(rhe, name), reason)


@when(parsers.parse('"{name}" hides their own review with the reason "{reason}"'))
def buyer_hides(rhe, name, reason):
    _hide(rhe, rhe.actors[name].token, _review_of(rhe, name), reason)


@given(
    parsers.parse(
        'the "ReviewCreated" envelope of the review of "{name}" has reached engagement.events'
    )
)
def created_reached(rhe, name):
    # positive control: the pipeline is live, so a missing ReviewHidden is not vacuous
    got = e.wait_facts(_review_of(rhe, name), "ReviewCreated", 1)
    assert got[0]["payload"]["review_id"] == _review_of(rhe, name), got


# ── Then ─────────────────────────────────────────────────────────────────
@then("the hide call succeeds")
def hide_ok(rhe):
    assert ros.code_of(rhe.responses["hide"]) == "ok", rhe.responses["hide"].text


@then(parsers.parse('the hide call is refused with HTTP {status:d} and "{code}"'))
def hide_refused(rhe, status, code):
    resp = rhe.responses["hide"]
    assert resp.status_code == status, (resp.status_code, resp.text)
    assert ros.code_of(resp) == code, resp.text


@then(
    parsers.parse(
        'exactly one "{kind}" envelope for the review of "{name}" and "{listing}" appears on engagement.events'
    )
)
def one_hidden(rhe, kind, name, listing):
    rid, lid = _review_of(rhe, name), _lid(rhe, listing)
    got = e.wait_facts(rid, kind, 1, timeout_s=30)[0]
    assert got["payload"]["review_id"] == rid, got
    assert got["payload"]["listing_id"] == lid, got
    assert got["key"] == lid, f"record key must be the listing id: {got['key']!r}"
    rhe.data["fact"] = got


@then(parsers.parse('that "{kind}" envelope does not contain "{text}"'))
def fact_without(rhe, kind, text):
    got = rhe.data["fact"]
    assert text.encode() not in got["raw"], "hide reason leaked into the envelope"


@then(
    parsers.parse(
        'no "{kind}" envelope for the review of "{name}" appears on engagement.events within {s:d} seconds'
    )
)
def none_hidden(rhe, kind, name, s):
    time.sleep(s)
    got = e.facts(_review_of(rhe, name), kind, timeout_s=8.0)
    assert not got, f"unexpected {kind} for a refused hide: {got}"


@then(
    parsers.parse(
        'within 30 seconds the warehouse holds one review_hidden record for the review of "{name}" and "{listing}"'
    )
)
def warehouse_hidden(rhe, name, listing):
    rid, lid = _review_of(rhe, name), _lid(rhe, listing)
    rows = tii.warehouse_rows(
        "SELECT review_id, listing_id FROM engagement_facts "
        "WHERE review_id = ? AND fact = 'review_hidden'",
        [rid],
        until=bool,
        timeout_s=30,
    )
    assert rows == [(rid, lid)], rows
