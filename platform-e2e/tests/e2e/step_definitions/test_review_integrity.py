"""Binds engagement/review_integrity.feature (engagement-review-integrity).

Black box through the gateway: reviews, orders, HideReview and QueryAuditLog are Connect JSON
calls; only the legacy unverified row is inserted by direct SQL (the API can no longer create it).
"""

from __future__ import annotations

import time
import uuid

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import review_order_support as ros
from tests.e2e.support.oic_order_support import AUTH, Actor, OicWorld, create_listing, register

scenarios("engagement/review_integrity.feature")


@pytest.fixture
def ri() -> OicWorld:
    return OicWorld()


def _lid(ri: OicWorld, name: str) -> str:
    return ri.listings[name]["id"]


def _seller(ri: OicWorld) -> Actor:
    return ri.actors["seller"]


def _review_of(ri: OicWorld, buyer: str) -> str:
    return ri.data["reviews"][buyer]


def _status(ri: OicWorld, key: str) -> str:
    return ros.code_of(ri.responses[key])


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse('a seller with a published listing "{name}"'))
def seller_listing(ri, name):
    seller = register(ri, "seller", "seller")
    create_listing(ri, name, seller, 50)
    ri.data["listing_name"] = name


@given(parsers.re(r'a buyer "(?P<name>[^"]+)"'))
def a_buyer(ri, name):
    register(ri, name, "buyer")


@given(parsers.parse('a buyer "{name}" with a completed order of "{listing}"'))
def buyer_completed(ri, name, listing):
    buyer = register(ri, name, "buyer")
    ri.orders[name] = ros.completed_order_for(ri, buyer, _seller(ri), _lid(ri, listing))


@given(parsers.parse('a buyer "{name}" with a pending order of "{listing}"'))
def buyer_pending(ri, name, listing):
    buyer = register(ri, name, "buyer")
    ri.orders[name] = ros.pending_order_for(ri, buyer, _lid(ri, listing))


def _review(ri: OicWorld, name: str, listing: str, rating: int, order_id: str) -> None:
    resp = ros.create_review(
        ri.actors[name].token, _lid(ri, listing), rating, f"ri-{name}-{rating}", order_id
    )
    ri.responses["review"] = resp
    if resp.status_code == 200:
        ri.data.setdefault("reviews", {})[name] = resp.json()["review"]["id"]


@given(
    parsers.parse(
        '"{name}" has reviewed "{listing}" with rating {rating:d} from their completed order'
    )
)
def has_reviewed(ri, name, listing, rating):
    _review(ri, name, listing, rating, ri.orders[name])
    assert _status(ri, "review") == "ok", ri.responses["review"].text


@when(parsers.parse('a legacy unverified review of {rating:d} star exists for "{listing}"'))
@given(parsers.parse('a legacy unverified review of {rating:d} star exists for "{listing}"'))
def legacy_review(ri, rating, listing):
    ros.seed_legacy_unverified_review(
        _lid(ri, listing),
        _seller(ri).user_id,
        f"legacy-{uuid.uuid4().hex[:8]}",
        rating,
        "legacy unverified",
    )


@given("an admin")
def an_admin(ri):
    from config.settings import get_settings
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = ros.post(None, AUTH, "Login", {"username": admin.username, "password": admin.password})
    token = (ros.ok(resp).get("result") or {}).get("token", "")
    assert token, f"admin login failed: {resp.text[:200]} ({get_settings().gateway_url})"
    ri.actors["admin"] = Actor(admin.username, token, "admin", _sub(token))


def _sub(token: str) -> str:
    import base64
    import json

    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


# ── When ─────────────────────────────────────────────────────────────────
@when(
    parsers.parse('"{name}" reviews "{listing}" with rating {rating:d} from their completed order')
)
@when(parsers.parse('"{name}" reviews "{listing}" with rating {rating:d} from their pending order'))
def reviews_from_own_order(ri, name, listing, rating):
    _review(ri, name, listing, rating, ri.orders[name])


@when(parsers.parse('"{name}" reviews "{listing}" with rating {rating:d} and an empty order id'))
def reviews_empty_order(ri, name, listing, rating):
    _review(ri, name, listing, rating, "")


@when(
    parsers.parse(
        '"{name}" reviews "{listing}" with rating {rating:d} from the completed order of "{owner}"'
    )
)
def reviews_foreign_order(ri, name, listing, rating, owner):
    _review(ri, name, listing, rating, ri.orders[owner])


def _hide(ri: OicWorld, token: str | None, review_id: str, reason: str) -> None:
    ri.responses["hide"] = ros.post(
        token, ros.ENGAGEMENT, "HideReview", {"reviewId": review_id, "reason": reason}
    )


@given(parsers.parse('the admin hides the review of "{name}" with the reason "{reason}"'))
@when(parsers.parse('the admin hides the review of "{name}" with the reason "{reason}"'))
def admin_hides(ri, name, reason):
    _hide(ri, ri.actors["admin"].token, _review_of(ri, name), reason)


@when(parsers.parse('"{name}" hides their own review with the reason "{reason}"'))
def buyer_hides(ri, name, reason):
    _hide(ri, ri.actors[name].token, _review_of(ri, name), reason)


@when(parsers.parse('an anonymous client hides the review of "{name}" with the reason "{reason}"'))
def anon_hides(ri, name, reason):
    _hide(ri, None, _review_of(ri, name), reason)


# ── Then ─────────────────────────────────────────────────────────────────
@then(parsers.parse('the review call fails with "{code}"'))
def review_fails(ri, code):
    assert _status(ri, "review") == code, ri.responses["review"].text


@then("the hide call succeeds")
def hide_ok(ri):
    assert _status(ri, "hide") == "ok", ri.responses["hide"].text


@then(parsers.parse('the hide call fails with "{code}"'))
def hide_fails(ri, code):
    assert _status(ri, "hide") == code, ri.responses["hide"].text


def _listed(ri: OicWorld, listing: str) -> tuple[list[dict], int]:
    res = ros.list_reviews(_lid(ri, listing))
    return res.get("reviews", []), int(res.get("page", {}).get("total", 0) or 0)


@then(parsers.parse('the listing "{listing}" lists {n:d} review that is a verified purchase'))
def lists_one_verified(ri, listing, n):
    reviews, total = _listed(ri, listing)
    assert len(reviews) == n and total == n, (reviews, total)
    assert all(r.get("verifiedPurchase") is True for r in reviews), reviews


@then(parsers.parse('the listing "{listing}" lists {n:d} reviews that are all verified purchases'))
def lists_all_verified(ri, listing, n):
    lists_one_verified(ri, listing, n)


@then(parsers.parse('the listing "{listing}" lists {n:d} reviews'))
def lists_n(ri, listing, n):
    reviews, total = _listed(ri, listing)
    assert len(reviews) == n and total == n, (reviews, total)


@then(
    parsers.parse(
        'the listing "{listing}" lists {n:d} reviews of which {u:d} is not a verified purchase'
    )
)
def lists_with_unverified(ri, listing, n, u):
    reviews, total = _listed(ri, listing)
    assert len(reviews) == n and total == n, (reviews, total)
    unverified = [r for r in reviews if not r.get("verifiedPurchase")]
    assert len(unverified) == u, reviews


@then(parsers.parse('the summary of "{listing}" shows an average of {avg:f} over {n:d} review'))
@then(parsers.parse('the summary of "{listing}" shows an average of {avg:f} over {n:d} reviews'))
def summary_is(ri, listing, avg, n):
    summary = ros.listing_summary(_lid(ri, listing))
    assert int(summary.get("reviewCount", 0) or 0) == n, summary
    assert float(summary.get("averageRating", 0) or 0) == pytest.approx(avg), summary


@then(
    parsers.parse(
        'the audit log holds {n:d} "{action}" event for the review of "{name}" by the admin '
        'with the reason "{reason}"'
    )
)
def audit_holds(ri, n, action, name, reason):
    admin = ri.actors["admin"]
    review_id = _review_of(ri, name)
    deadline = time.monotonic() + 15
    mine: list[dict] = []
    while time.monotonic() < deadline:
        res = ros.ok(
            ros.post(
                admin.token,
                ros.AUDIT,
                "QueryAuditLog",
                {"actorId": admin.user_id, "targetType": "review", "page": {"pageSize": 100}},
            )
        )
        mine = [
            e
            for e in res.get("events", [])
            if e.get("targetId") == review_id and e.get("action") == action
        ]
        if len(mine) >= n:
            break
        time.sleep(1)
    assert len(mine) == n, mine
    for event in mine:
        assert event.get("actorId") == admin.user_id, event
        assert event.get("targetType") == "review", event
        assert (event.get("metadata") or {}).get("reason") == reason, event
        assert (event.get("metadata") or {}).get("listing_id"), event
