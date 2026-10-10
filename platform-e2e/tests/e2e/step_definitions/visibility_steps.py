"""Listing visibility (port-security-hardening / listing-read-visibility).

Black box through the gateway Connect API with two real users: a seller who owns the
draft and a buyer who does not. Every call names its token explicitly, so the order
of the Given steps never changes who is acting. Search is eventually consistent
(listing event -> Kafka -> indexer), so the search scenario waits with a bounded poll
on the published counterpart before it asserts that the draft is absent.
"""

from __future__ import annotations

import base64
import json
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"
LISTING = "/platform.listing.v1.ListingService/"
SEARCH = "/platform.search.v1.SearchService/"
INDEX_DEADLINE_S = 90.0
POLL_INTERVAL_S = 1.0


def _post(path: str, body: dict, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(get_settings().gateway_url + path, json=body, headers=headers, timeout=30.0)


def _subject(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))["sub"]


def _register(world: World, role: str) -> str:
    username = f"e2e_{role}_{uuid.uuid4().hex[:10]}"
    return world.service_factory.auth.register(username, PASSWORD, role=role)


def _create(token: str, title: str, status: str) -> str:
    resp = _post(
        LISTING + "CreateListing",
        {
            "listing": {
                "title": title,
                "categoryId": "cat-laptop",
                "price": 100000,
                "stock": 3,
                "status": status,
                "currency": "VND",
                "description": "visibility e2e",
            }
        },
        token,
    )
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    return resp.json()["listing"]["id"]


def _ensure_seller(world: World) -> str:
    extra = world.state.extra
    if "seller_token" not in extra:
        token = _register(world, "seller")
        extra["seller_token"] = token
        extra["seller_id"] = _subject(token)
    return extra["seller_token"]


def _hit_ids(resp: httpx.Response) -> list[str]:
    assert resp.status_code == 200, f"search {resp.status_code}: {resp.text}"
    return [h["listingId"] for h in resp.json().get("hits", [])]


@given("a seller with a draft listing")
def seller_with_draft(world: World) -> None:
    token = _ensure_seller(world)
    token_tag = uuid.uuid4().hex[:8]
    world.state.extra["draft_id"] = _create(token, f"vis draft {token_tag}", "LISTING_STATUS_DRAFT")


@given("a logged-in buyer")
def logged_in_buyer(world: World) -> None:
    world.state.extra["buyer_token"] = _register(world, "buyer")


@given("a seller with a draft and a published listing sharing a unique title token")
def seller_with_draft_and_published(world: World) -> None:
    token = _ensure_seller(world)
    unique = "zqvis" + uuid.uuid4().hex[:10]
    extra = world.state.extra
    extra["unique"] = unique
    extra["draft_id"] = _create(token, f"{unique} draft", "LISTING_STATUS_DRAFT")
    extra["published_id"] = _create(token, f"{unique} published", "LISTING_STATUS_PUBLISHED")


@when("the buyer reads that draft through GetListing")
def buyer_reads_draft(world: World) -> None:
    extra = world.state.extra
    extra["resp"] = _post(LISTING + "GetListing", {"id": extra["draft_id"]}, extra["buyer_token"])


@when("the seller reads that draft through GetListing")
def seller_reads_draft(world: World) -> None:
    extra = world.state.extra
    extra["resp"] = _post(LISTING + "GetListing", {"id": extra["draft_id"]}, extra["seller_token"])


@when("the client lists listings without a status filter")
def anonymous_lists(world: World) -> None:
    world.state.extra["resp"] = _post(LISTING + "ListListings", {})


@when("the published listing is indexed in search")
def published_is_indexed(world: World) -> None:
    """Bounded poll: the absence of the draft proves nothing until its published twin is indexed."""
    extra = world.state.extra
    deadline = time.monotonic() + INDEX_DEADLINE_S
    last: list[str] = []
    while time.monotonic() < deadline:
        last = _hit_ids(_post(SEARCH + "SearchListings", {"query": extra["unique"]}))
        if extra["published_id"] in last:
            return
        time.sleep(POLL_INTERVAL_S)
    raise AssertionError(
        f"published listing {extra['published_id']} not indexed within {INDEX_DEADLINE_S}s; "
        f"hits={last}"
    )


@when("the client searches for the unique title token")
def anonymous_searches(world: World) -> None:
    extra = world.state.extra
    extra["resp"] = _post(SEARCH + "SearchListings", {"query": extra["unique"]})


@when("the buyer searches with status draft and the seller's seller_id")
def buyer_searches_foreign_drafts(world: World) -> None:
    extra = world.state.extra
    extra["resp"] = _post(
        SEARCH + "SearchListings",
        {"query": "vis", "filters": {"status": "draft", "seller_id": extra["seller_id"]}},
        extra["buyer_token"],
    )


@when("the client calls SaveSearch")
def anonymous_save_search(world: World) -> None:
    world.state.extra["resp"] = _post(SEARCH + "SaveSearch", {"query": "laptop"})


@then(parsers.parse("the gateway answers HTTP {status:d}"))
def gateway_answers(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


@then("the returned listing has status draft")
def returned_is_draft(world: World) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    listing = resp.json()["listing"]
    assert listing["id"] == world.state.extra["draft_id"], listing
    assert listing["status"] == "LISTING_STATUS_DRAFT", listing


@then("every returned listing has status published")
def all_published(world: World) -> None:
    listings = world.state.extra["resp"].json().get("listings", [])
    assert listings, "ListListings returned nothing; the check would be vacuous"
    statuses = {item.get("status") for item in listings}
    assert statuses == {"LISTING_STATUS_PUBLISHED"}, statuses


@then("the seller's draft is not among them")
def draft_not_listed(world: World) -> None:
    ids = {item.get("id") for item in world.state.extra["resp"].json().get("listings", [])}
    assert world.state.extra["draft_id"] not in ids


@then("the search returns the published listing")
def search_has_published(world: World) -> None:
    extra = world.state.extra
    assert extra["published_id"] in _hit_ids(extra["resp"])


@then("the search returns no hit for the draft")
def search_has_no_draft(world: World) -> None:
    extra = world.state.extra
    assert extra["draft_id"] not in _hit_ids(extra["resp"])


@then("the owner can find the draft by asking for their own drafts")
def owner_finds_draft(world: World) -> None:
    """Proves the draft IS indexed, so its absence above is the visibility filter."""
    extra = world.state.extra
    deadline = time.monotonic() + INDEX_DEADLINE_S
    last: list[str] = []
    while time.monotonic() < deadline:
        last = _hit_ids(
            _post(
                SEARCH + "SearchListings",
                {
                    "query": extra["unique"],
                    "filters": {"status": "draft", "seller_id": extra["seller_id"]},
                },
                extra["seller_token"],
            )
        )
        if extra["draft_id"] in last:
            return
        time.sleep(POLL_INTERVAL_S)
    raise AssertionError(f"draft {extra['draft_id']} never indexed for its owner; hits={last}")
