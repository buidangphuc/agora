"""Binds seller/seller_enforcement_hold.feature (change domain-seller-enforcement-hold,
initiative q6-trust-enforcement).

Black box through the gateway: an admin sets a seller's status (AccountAdminService), the
observable effect is listing visibility in search / GetListing and the owner's ListMyListings.
"""

from __future__ import annotations

import time
import uuid

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import srm_support as s
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

scenarios("seller/seller_enforcement_hold.feature")

LISTING = "/platform.listing.v1.ListingService/"
SEARCH = "/platform.search.v1.SearchService/SearchListings"
INDEX_S = 90.0
EFFECT_S = 60.0


def _x(world: World) -> dict:
    return world.state.extra.setdefault("seh", {})


def _who(name: str) -> str:
    return {"first": "a", "second": "b"}[name]


def _search_hits(query: str) -> list[str]:
    resp = s.post(SEARCH, {"query": query})
    assert resp.status_code == 200, f"SearchListings {resp.status_code}: {resp.text}"
    return [h["listingId"] for h in resp.json().get("hits", [])]


def _get(token: str | None, listing_id: str) -> httpx.Response:
    return s.post(LISTING + "GetListing", {"id": listing_id}, token)


def _make_seller(world: World, key: str) -> None:
    st = _x(world)
    user = ue.register_user("seller", f"seh_{key}")
    unique = f"hold{uuid.uuid4().hex[:12]}"
    resp = s.post(
        LISTING + "CreateListing",
        {
            "listing": {
                "title": f"Clean laptop stand {unique}",
                "description": "seller enforcement hold e2e",
                "categoryId": "cat-laptop",
                "price": 100000,
                "stock": 3,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
            }
        },
        user["token"],
    )
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    listing_id = resp.json()["listing"]["id"]
    st[key] = {"user": user, "listing": listing_id, "unique": unique}

    def cleanup() -> None:
        try:
            s.post(LISTING + "DeleteListing", {"id": listing_id}, ue.admin_token(world))
        except Exception:  # best effort: a held listing refuses owner deletes
            pass

    world.add_cleanup(cleanup)
    # Positive control: the listing is public before any enforcement.
    s.eventually(
        lambda: listing_id in _search_hits(unique), "published listing in search", timeout_s=INDEX_S
    )
    got = _get(None, listing_id)
    assert got.status_code == 200, f"published listing not readable by a non-owner: {got.text}"
    assert got.json()["listing"]["status"] == "LISTING_STATUS_PUBLISHED", got.text


def _seller(world: World, who: str) -> dict:
    return _x(world)[_who(who)]


# ── Givens ───────────────────────────────────────────────────────────────
@given("an enforcement-hold seller with a published listing that is findable")
def first_seller(world: World) -> None:
    _make_seller(world, "a")


@given("a second enforcement-hold seller with a published listing that is findable")
def second_seller(world: World) -> None:
    _make_seller(world, "b")


# ── Whens ────────────────────────────────────────────────────────────────
def _set(world: World, who: str, status: str, reason: str, until=None) -> None:
    seller = _seller(world, who)
    resp = ue.set_user_status(world, seller["user"]["id"], status, reason, until)
    assert resp.status_code == 200, f"SetUserStatus({status}) failed: {resp.status_code} {resp.text}"


@when(parsers.parse("the admin suspends the {who} enforcement-hold seller"))
def suspend(world: World, who: str) -> None:
    _set(world, who, "suspended", "scam", ue.in_future(3600))


@when(parsers.parse("the admin suspends the {who} enforcement-hold seller until {sec:d} seconds from now"))
def suspend_short(world: World, who: str, sec: int) -> None:
    _set(world, who, "suspended", "scam", ue.in_future(sec))
    _x(world)["until_ts"] = time.time() + sec


@when(parsers.parse("the admin restricts the {who} enforcement-hold seller"))
def restrict(world: World, who: str) -> None:
    _set(world, who, "restricted", "scam")


@when(parsers.parse("the admin sets the {who} enforcement-hold seller back to active"))
def reinstate(world: World, who: str) -> None:
    _set(world, who, "active", "appeal_accepted")


@when(parsers.parse("the suspension end time has passed and the {who} seller logs in"))
def login_after_expiry(world: World, who: str) -> None:
    st = _x(world)
    time.sleep(max(0.0, st["until_ts"] - time.time()) + 2.0)
    user = _seller(world, who)["user"]
    resp = ue.login_raw(user["username"], user["password"])
    assert resp.status_code == 200, f"login after the suspension ended failed: {resp.status_code} {resp.text}"


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse("within {seconds:d} seconds the {who} seller's listing is absent from search"))
def absent_from_search(world: World, seconds: int, who: str) -> None:
    seller = _seller(world, who)
    deadline = time.monotonic() + seconds
    hits: list[str] = []
    while time.monotonic() < deadline:
        hits = _search_hits(seller["unique"])
        if seller["listing"] not in hits:
            return
        time.sleep(1.0)
    raise AssertionError(f"listing still in search {seconds}s after the enforcement; hits={hits}")


@then(parsers.parse("within {seconds:d} seconds the {who} seller's listing is returned by search"))
def found_in_search(world: World, seconds: int, who: str) -> None:
    seller = _seller(world, who)
    s.eventually(
        lambda: seller["listing"] in _search_hits(seller["unique"]),
        "reinstated listing in search",
        timeout_s=float(seconds),
    )


@then(parsers.parse("a non-owner gets not_found for the {who} seller's listing"))
def non_owner_not_found(world: World, who: str) -> None:
    resp = _get(None, _seller(world, who)["listing"])
    assert resp.status_code == 404 and resp.json().get("code") == "not_found", (
        f"held listing readable by a non-owner: {resp.status_code} {resp.text}"
    )


@then(parsers.parse('a non-owner gets the {who} seller\'s listing with status "{status}"'))
def non_owner_status(world: World, who: str, status: str) -> None:
    resp = _get(None, _seller(world, who)["listing"])
    assert resp.status_code == 200, f"GetListing {resp.status_code}: {resp.text}"
    assert resp.json()["listing"]["status"] == status, resp.text


def _still_visible(world: World, who: str) -> None:
    seller = _seller(world, who)
    assert seller["listing"] in _search_hits(seller["unique"]), f"{who} listing missing from search"
    resp = _get(None, seller["listing"])
    assert resp.status_code == 200, f"{who} listing not readable by a non-owner: {resp.text}"
    assert resp.json()["listing"]["status"] == "LISTING_STATUS_PUBLISHED", resp.text


@then("the second seller's listing is still returned by search and by GetListing")
def second_visible(world: World) -> None:
    _still_visible(world, "second")


@then("the first seller's listing is still returned by search and by GetListing")
def first_visible(world: World) -> None:
    _still_visible(world, "first")
