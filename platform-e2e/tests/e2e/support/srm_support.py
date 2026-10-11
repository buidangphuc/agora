"""Shared helpers for the port-search-read-model-correctness scenarios (area srm).

Everything is black box through the gateway (Connect JSON) with explicit tokens, so the order
of the Given steps never changes who acts. Search is eventually consistent (listing event ->
Kafka -> indexer), so every observation is a bounded poll (`eventually`), never a sleep, and an
absence is only asserted after a positive control proved the read path is live.
"""

from __future__ import annotations

import base64
import json
import time
import uuid
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.oic_inv_flow import Actor, fill_cart, make_buyer, register
from tests.e2e.support.world import World

LISTING = "/platform.listing.v1.ListingService/"
SEARCH = "/platform.search.v1.SearchService/"
ORDER = "/platform.order.v1.OrderService/"
VISIBLE_S = 30.0  # spec: a change is visible through the gateway within 30 seconds
DLQ_S = 60.0
CONSUME_S = 60.0


def post(path: str, body: dict, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(get_settings().gateway_url + path, json=body, headers=headers, timeout=30.0)


def subject(token: str) -> str:
    part = token.split(".")[1]
    part += "=" * (-len(part) % 4)
    return json.loads(base64.urlsafe_b64decode(part))["sub"]


def eventually(
    check: Callable[[], Any], what: str, timeout_s: float = VISIBLE_S, interval_s: float = 1.0
) -> Any:
    """Bounded poll: the first truthy `check()`; AssertionError naming `what` and the last value."""
    deadline = time.monotonic() + timeout_s
    last: Any = None
    while True:
        try:
            last = check()
        except AssertionError as exc:  # a not-yet-true expectation inside check
            last = f"AssertionError: {exc}"
            value = None
        else:
            value = last
        if value:
            return value
        if time.monotonic() >= deadline:
            raise AssertionError(f"{what} did not hold within {timeout_s:.0f}s; last: {last!r}")
        time.sleep(interval_s)


@dataclass
class Listed:
    id: str
    title: str


@dataclass
class Ctx:
    """Per-scenario bag in `world.state.extra["srm"]`."""

    keyword: str = field(default_factory=lambda: "zsrm" + uuid.uuid4().hex[:10])
    seller: Actor | None = None
    buyer: Actor | None = None
    buyer2: Actor | None = None
    listings: dict[str, Listed] = field(default_factory=dict)
    started_ms: int = field(default_factory=lambda: int(time.time() * 1000) - 5_000)
    orders: list[dict[str, Any]] = field(default_factory=list)
    resp: httpx.Response | None = None
    resps: list[httpx.Response] = field(default_factory=list)
    saved_id: str = ""
    event: tuple[int, int] | None = None  # (partition, offset) of the last crafted record
    extra: dict[str, Any] = field(default_factory=dict)


def ctx(world: World) -> Ctx:
    return world.state.extra.setdefault("srm", Ctx())


# ── actors and listings ──────────────────────────────────────────────────
def seller_of(world: World) -> Actor:
    c = ctx(world)
    if c.seller is None:
        c.seller = register(world, "seller")
    return c.seller


def buyer_of(world: World) -> Actor:
    c = ctx(world)
    if c.buyer is None:
        c.buyer = make_buyer(world)
    return c.buyer


def second_buyer(world: World) -> Actor:
    c = ctx(world)
    if c.buyer2 is None:
        c.buyer2 = make_buyer(world)
    return c.buyer2


def create_listing(
    world: World,
    label: str,
    *,
    stock: int,
    status: str = "LISTING_STATUS_PUBLISHED",
    title_suffix: str | None = None,
    price: int = 100_000,
    seller: Actor | None = None,
) -> Listed:
    c = ctx(world)
    seller = seller or seller_of(world)
    title = f"{c.keyword} {title_suffix or label}"
    resp = post(
        LISTING + "CreateListing",
        {
            "listing": {
                "title": title,
                "categoryId": "cat-laptop",
                "price": price,
                "stock": stock,
                "status": status,
                "currency": "VND",
                "description": "srm e2e",
            }
        },
        seller.token,
    )
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    listed = Listed(resp.json()["listing"]["id"], title)
    c.listings[label] = listed
    return listed


def listing(world: World, label: str) -> Listed:
    return ctx(world).listings[label]


# ── search ───────────────────────────────────────────────────────────────
def search(
    query: str,
    *,
    token: str | None = None,
    filters: dict[str, str] | None = None,
    sort_by: str | None = None,
    mode: str | None = None,
    min_rating: int | None = None,
    page_size: int | None = None,
) -> httpx.Response:
    body: dict[str, Any] = {"query": query}
    if filters:
        body["filters"] = filters
    if sort_by:
        body["sortBy"] = sort_by
    if mode:
        body["searchMode"] = mode
    if min_rating is not None:
        body["minRating"] = min_rating
    if page_size:
        body["page"] = {"pageSize": page_size}
    return post(SEARCH + "SearchListings", body, token)


def ok_json(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"{resp.request.url.path} {resp.status_code}: {resp.text}"
    return resp.json()


def hit_ids(resp: httpx.Response) -> list[str]:
    return [h["listingId"] for h in ok_json(resp).get("hits", [])]


def hit_of(resp: httpx.Response, listing_id: str) -> dict[str, Any] | None:
    for h in ok_json(resp).get("hits", []):
        if h["listingId"] == listing_id:
            return h
    return None


def total_of(resp: httpx.Response) -> int:
    return int((ok_json(resp).get("page") or {}).get("total") or 0)


def stock_of_hit(hit: dict[str, Any] | None) -> int | None:
    """`stock` of a hit as the gateway shows it: None when absent (unknown), 0 when present as 0."""
    if hit is None or "stock" not in hit:
        return None
    return int(hit["stock"])


def wait_indexed(world: World, label: str, query: str | None = None) -> None:
    """Positive control: the listing is returned by search (the indexer has caught up)."""
    listed = listing(world, label)
    q = query or listed.title
    eventually(
        lambda: hit_of(search(q), listed.id) is not None,
        f"listing {label} {listed.id} to be searchable",
        90.0,
    )


def wait_stock(world: World, label: str, expected: int, *, token: str | None = None) -> None:
    listed = listing(world, label)
    eventually(
        lambda: stock_of_hit(hit_of(search(listed.title, token=token), listed.id)) == expected,
        f"search stock of {label} to be {expected}",
    )


# ── ordering ─────────────────────────────────────────────────────────────
def checkout(world: World, buyer: Actor, label: str, quantity: int) -> dict[str, Any]:
    c = ctx(world)
    fill_cart(world, buyer, [(listing(world, label).id, quantity)])
    resp = post(ORDER + "CreateOrder", {"paymentMethod": "PAYMENT_METHOD_COD"}, buyer.token)
    assert resp.status_code == 200, f"CreateOrder {resp.status_code}: {resp.text}"
    orders = resp.json().get("orders", [])
    assert orders, f"CreateOrder returned no orders: {resp.text}"
    c.orders.append(orders[0])
    return orders[0]


def cancel_order(buyer: Actor, order_id: str) -> None:
    resp = post(ORDER + "CancelOrder", {"id": order_id, "reason": "srm e2e"}, buyer.token)
    assert resp.status_code == 200, f"CancelOrder {resp.status_code}: {resp.text}"


# ── saved searches ───────────────────────────────────────────────────────
def save_search(token: str, query: str, filters_json: str = "") -> httpx.Response:
    body: dict[str, Any] = {"query": query}
    if filters_json:
        body["filtersJson"] = filters_json
    return post(SEARCH + "SaveSearch", body, token)


def run_saved(token: str, saved_id: str) -> httpx.Response:
    return post(SEARCH + "RunSavedSearch", {"id": saved_id}, token)


def list_saved(token: str) -> list[dict[str, Any]]:
    return ok_json(post(SEARCH + "ListSavedSearches", {}, token)).get("savedSearches", [])


# ── real events on the topic ─────────────────────────────────────────────
def real_event(world: World, label: str, type_name: str, change_type: int | None = None):
    return ev.wait_for_record(
        listing(world, label).id,
        type_name=type_name,
        change_type=change_type,
        since_ms=ctx(world).started_ms,
    )


def last_real_stock_event(world: World, label: str, stock: int):
    """The real ListingStockChanged for `label` carrying `stock` (polled until written)."""
    lid = listing(world, label).id
    since = ctx(world).started_ms

    def _find():
        for rec in ev.records_for(lid, since):
            if rec.type == ev.STOCK_CHANGED and rec.stock == stock:
                return rec
        return None

    return eventually(_find, f"a ListingStockChanged with stock {stock} for {label}", 30.0)


def craft_listing(world: World, label: str, **fields: Any) -> bytes:
    """A Listing message for `label` (seller id and defaults filled in)."""
    fields.setdefault("seller_id", subject(seller_of(world).token))
    return ev.listing_msg(listing(world, label).id, **fields)


# ── deletes ──────────────────────────────────────────────────────────────
def delete_listing(world: World, label: str) -> ev.Record:
    """DeleteListing as the owner through the gateway; returns the domain's delete record
    (a ListingChanged / ListingBaseInfoChanged with change type DELETED) once it is on the topic."""
    lid = listing(world, label).id
    since = int(time.time() * 1000) - 2_000
    resp = post(LISTING + "DeleteListing", {"id": lid}, seller_of(world).token)
    assert resp.status_code == 200, f"DeleteListing {resp.status_code}: {resp.text}"

    def _find():
        for rec in ev.records_for(lid, since):
            if rec.change_type == ev.DELETED:
                return rec
        return None

    rec = eventually(_find, f"the delete event of {label} on {ev.TOPIC}", 30.0)
    ctx(world).extra[f"delete:{label}"] = rec
    return rec


def delete_and_wait_consumed(world: World, label: str) -> ev.Record:
    rec = delete_listing(world, label)
    ev.wait_consumed(rec.partition, rec.offset, CONSUME_S)
    return rec


def suggestions(query: str, limit: int = 20) -> list[str]:
    return ok_json(post(SEARCH + "Suggest", {"query": query, "limit": limit})).get(
        "suggestions", []
    )
