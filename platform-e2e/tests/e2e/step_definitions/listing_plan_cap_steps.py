"""Seller plan listing cap (listing-plan-cap): black box through the gateway."""

from __future__ import annotations

import base64
import json

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services import ListingService
from src.constants import gateway_endpoints as ep
from src.models import Listing
from tests.e2e.step_definitions.commerce_steps import _call, _user
from tests.e2e.support.world import World

_SUBSCRIPTION = "/platform.promotion.v1.SubscriptionService/"
_STATUSES = ("published", "draft")


_EXTRA_PAYLOAD = {
    "listing": {
        "title": "[E2E] plan-cap extra",
        "categoryId": "cat-electronics",
        "price": 1_000_000,
        "stock": 5,
        "status": "LISTING_STATUS_PUBLISHED",
        "currency": "VND",
        "description": "plan cap probe",
    }
}


def _x(world: World) -> dict:
    return world.state.extra


def _new_listing(index: int, status: str) -> Listing:
    return Listing(
        title=f"[E2E] plan-cap {index}",
        category_id="cat-electronics",
        price=1_000_000,
        stock=5,
        status=status,
    )


def _subscribe(token: str, plan_id: str) -> None:
    resp = _call(token, _SUBSCRIPTION + "Subscribe", {"planId": plan_id})
    assert resp.status_code == 200, f"Subscribe {plan_id}: {resp.status_code} {resp.text}"


def _seed_seller(plan_id: str, count: int) -> dict:
    seller = _user("seller")
    _subscribe(seller["token"], plan_id)
    svc = ListingService(token=seller["token"])
    ids = [svc.create_listing(_new_listing(i, _STATUSES[i % len(_STATUSES)])) for i in range(count)]
    assert all(ids), "seeding a listing failed"
    seller["listing_ids"] = ids
    return seller


def _my_listing_ids(token: str) -> list[str]:
    ids: list[str] = []
    cursor = ""
    for _ in range(50):
        resp = _call(token, ep.LISTING_LIST_MINE, {"page": {"cursor": cursor, "pageSize": 100}})
        assert resp.status_code == 200, f"ListMyListings: {resp.status_code} {resp.text}"
        body = resp.json()
        ids += [li["id"] for li in body.get("listings", [])]
        cursor = (body.get("page") or {}).get("nextCursor", "")
        if not cursor:
            break
    return ids


def _error_mentions(resp: httpx.Response, reason: str) -> bool:
    if reason in resp.text:
        return True
    try:
        details = resp.json().get("details") or []
    except json.JSONDecodeError:
        return False
    for detail in details:
        raw = detail.get("value", "")
        raw += "=" * (-len(raw) % 4)
        try:
            if reason.encode() in base64.b64decode(raw):
                return True
        except ValueError:
            continue
    return False


@given(
    parsers.parse(
        'a plan-cap seller on the plan "{plan_id}" who owns {count:d} listings of mixed statuses'
    )
)
def plan_cap_seller(world: World, plan_id: str, count: int) -> None:
    _x(world)["pc_seller"] = _seed_seller(plan_id, count)


@given(
    parsers.parse(
        'another plan-cap seller on the plan "{plan_id}" who owns {count:d} listings '
        "of mixed statuses"
    )
)
def other_plan_cap_seller(world: World, plan_id: str, count: int) -> None:
    _x(world)["pc_other"] = _seed_seller(plan_id, count)


@given(parsers.parse('the plan-cap seller moves to the plan "{plan_id}"'))
def plan_cap_seller_moves(world: World, plan_id: str) -> None:
    _subscribe(_x(world)["pc_seller"]["token"], plan_id)


@when("the plan-cap seller creates another listing")
def plan_cap_create(world: World) -> None:
    _x(world)["pc_resp"] = _call(_x(world)["pc_seller"]["token"], ep.LISTING_CREATE, _EXTRA_PAYLOAD)


@when(
    parsers.parse(
        'a plan-cap control seller on the plan "{plan_id}" with {count:d} listings '
        "creates another listing"
    )
)
def control_seller_creates(world: World, plan_id: str, count: int) -> None:
    control = _seed_seller(plan_id, count)
    _x(world)["pc_control_resp"] = _call(control["token"], ep.LISTING_CREATE, _EXTRA_PAYLOAD)


@then(parsers.parse('the control seller\'s create is rejected with reason "{reason}"'))
def control_rejected(world: World, reason: str) -> None:
    _assert_rejected(_x(world)["pc_control_resp"], reason)


@when("the plan-cap seller opens their listings")
def plan_cap_open(world: World) -> None:
    _x(world)["pc_listed"] = _my_listing_ids(_x(world)["pc_seller"]["token"])


@then(parsers.parse('the create is rejected as RESOURCE_EXHAUSTED with reason "{reason}"'))
def create_rejected(world: World, reason: str) -> None:
    _assert_rejected(_x(world)["pc_resp"], reason)


def _assert_rejected(resp: httpx.Response, reason: str) -> None:
    assert (
        resp.status_code == 429
    ), f"expected 429 RESOURCE_EXHAUSTED, got {resp.status_code}: {resp.text}"
    assert resp.json().get("code") == "resource_exhausted", resp.text
    assert _error_mentions(resp, reason), f"reason {reason} missing: {resp.text}"


@then("the create succeeds")
def create_succeeds(world: World) -> None:
    resp: httpx.Response = _x(world)["pc_resp"]
    assert resp.status_code == 200, f"create failed: {resp.status_code} {resp.text}"
    assert (resp.json().get("listing") or {}).get("id"), resp.text


@then(parsers.parse("the plan-cap seller's listing list has {count:d} entries"))
def listing_list_count(world: World, count: int) -> None:
    ids = _my_listing_ids(_x(world)["pc_seller"]["token"])
    assert len(ids) == count, f"expected {count} listings, got {len(ids)}"


@then(parsers.parse("all {count:d} listings are returned and none is hidden or removed"))
def all_listings_returned(world: World, count: int) -> None:
    listed = set(_x(world)["pc_listed"])
    seeded = set(_x(world)["pc_seller"]["listing_ids"])
    assert len(seeded) == count
    assert seeded <= listed, f"missing after downgrade: {sorted(seeded - listed)}"
    assert len(listed) == count, f"expected {count} listings, got {len(listed)}"


@then(
    parsers.parse(
        "only the plan-cap seller's own {count:d} listings are returned "
        "and none of the other seller's"
    )
)
def only_own_listings(world: World, count: int) -> None:
    listed = set(_x(world)["pc_listed"])
    own = set(_x(world)["pc_seller"]["listing_ids"])
    other = set(_x(world)["pc_other"]["listing_ids"])
    assert listed == own, f"listing set differs from own: extra={sorted(listed - own)}"
    assert len(listed) == count
    assert not (listed & other), "another seller's listings leaked"
