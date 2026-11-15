"""Steps of order/seller_list_paging.feature (change order-seller-list-paging).

Black box through the gateway: orders are seeded by real checkouts, payments and seller status
updates; ListSellerOrders is called with raw Connect JSON so error codes and absent fields stay
observable.
"""

from __future__ import annotations

import os
import re
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime
from typing import Any

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import oic_order_support as s

STATUSES = ["PENDING", "PAID", "SHIPPED", "COMPLETED", "CANCELLED"]
SEED_WORKERS = 8


@pytest.fixture
def slp() -> s.OicWorld:
    w = s.OicWorld()
    w.data["slp"] = {
        "req": {},
        "orders": {},
        "resp": None,
        "pages": [],
        "cursor": {},
        "durations": [],
        "since": 0.0,
        "buyers": {},
    }
    return w


def _d(w: s.OicWorld) -> dict[str, Any]:
    return w.data["slp"]


def _status(name: str) -> str:
    return name if name.startswith("ORDER_STATUS_") else f"ORDER_STATUS_{name}"


def _short(status: str) -> str:
    return status.removeprefix("ORDER_STATUS_")


def _seller(w: s.OicWorld, name: str) -> s.Actor:
    if name not in w.actors:
        s.register(w, name, "seller")
        _d(w)["orders"][name] = []
    return w.actors[name]


def _buyer(w: s.OicWorld, owner: str, idx: int = 0) -> s.Actor:
    key = f"{owner}-buyer-{idx}"
    if key not in w.actors:
        buyer = s.register(w, key, "buyer")
        s.ensure_address(w, buyer)
    return w.actors[key]


def _create_listing(w: s.OicWorld, seller: s.Actor, title: str, stock: int) -> str:
    resp = s.post(
        w,
        seller,
        s.LISTING,
        "CreateListing",
        {
            "listing": {
                "title": title,
                "categoryId": "cat-electronics",
                "price": 100_000,
                "stock": stock,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
                "description": "seller list paging e2e",
            }
        },
    )
    listing = s.ok(resp).get("listing") or {}
    assert listing.get("id"), resp.text
    return listing["id"]


def _place(w: s.OicWorld, buyer: s.Actor, seller: s.Actor, listing_id: str, target: str) -> str:
    s.ok(s.post(w, buyer, s.CART, "AddToCart", {"listingId": listing_id, "quantity": 1}))
    orders = s.ok(s.checkout(w, buyer)).get("orders", [])
    assert len(orders) == 1, orders
    oid = orders[0]["id"]
    if target == s.CANCELLED:
        s.ok(s.post(w, buyer, s.ORDER, "CancelOrder", {"id": oid, "reason": "seed"}))
    else:
        s.put_in_status(w, oid, buyer, seller, target)
    return oid


def _seed_bulk(w: s.OicWorld, name: str, plan: list[str], title_prefix: str) -> None:
    """Place one order per entry of ``plan`` (a status each), spread over a pool of buyers."""
    seller = _seller(w, name)
    listing_id = _create_listing(w, seller, f"{title_prefix} bulk", len(plan) + 10)
    workers = min(SEED_WORKERS, max(1, len(plan)))
    buyers = [_buyer(w, name, i) for i in range(workers)]
    chunks = [plan[i::workers] for i in range(workers)]

    def run(i: int) -> list[dict[str, str]]:
        return [
            {
                "id": _place(w, buyers[i], seller, listing_id, st),
                "status": st,
                "title": title_prefix,
            }
            for st in chunks[i]
        ]

    with ThreadPoolExecutor(workers) as pool:
        for chunk in pool.map(run, range(workers)):
            _d(w)["orders"][name].extend(chunk)


def _seed_titled(w: s.OicWorld, name: str, status: str, title: str) -> None:
    seller = _seller(w, name)
    listing_id = _create_listing(w, seller, title, 5)
    oid = _place(w, _buyer(w, name), seller, listing_id, _status(status))
    _d(w)["orders"][name].append({"id": oid, "status": _status(status), "title": title})


# ── seeding steps ────────────────────────────────────────────────────────
@given(parsers.parse('a seller "{name}" owning no order'))
def seller_no_order(slp: s.OicWorld, name: str) -> None:
    _seller(slp, name)


@given(parsers.parse('a seller "{name}" owning {n:d} orders'))
def seller_n_orders(slp: s.OicWorld, name: str, n: int) -> None:
    _seed_bulk(slp, name, [s.PENDING] * n, "Seller paging item")


@given(parsers.parse('a seller "{name}" owning {n:d} orders of which {p:d} are PAID'))
def seller_n_orders_paid(slp: s.OicWorld, name: str, n: int, p: int) -> None:
    _seed_bulk(slp, name, [s.PAID] * p + [s.PENDING] * (n - p), "Seller paging item")


@given(parsers.parse('a seller "{name}" owning {n:d} PAID orders'))
def seller_n_paid(slp: s.OicWorld, name: str, n: int) -> None:
    _seed_bulk(slp, name, [s.PAID] * n, "Seller paging item")


@given(
    parsers.parse('a seller "{name}" owning {p:d} PAID, {sh:d} SHIPPED and {c:d} CANCELLED orders')
)
def seller_mixed(slp: s.OicWorld, name: str, p: int, sh: int, c: int) -> None:
    _seed_bulk(slp, name, [s.PAID] * p + [s.SHIPPED] * sh + [s.CANCELLED] * c, "Seller paging item")


@given(parsers.parse('a seller "{name}" who has a {status} order titled "{title}"'))
def seller_titled(slp: s.OicWorld, name: str, status: str, title: str) -> None:
    _seed_titled(slp, name, status, title)


@given(parsers.parse('"{name}" has a {status} order titled "{title}"'))
def more_titled(slp: s.OicWorld, name: str, status: str, title: str) -> None:
    _seed_titled(slp, name, status, title)


# ── request building ─────────────────────────────────────────────────────
@given(parsers.parse("the request has page_size {n:d}"))
def req_page_size(slp: s.OicWorld, n: int) -> None:
    _d(slp)["req"].setdefault("page", {})["pageSize"] = n


@given("the request has an empty page")
def req_empty_page(slp: s.OicWorld) -> None:
    _d(slp)["req"].setdefault("page", {})


@given(parsers.parse('the request has cursor "{cursor}"'))
def req_cursor(slp: s.OicWorld, cursor: str) -> None:
    _d(slp)["req"].setdefault("page", {})["cursor"] = cursor


@given(parsers.parse('the request has statuses "{values}"'))
def req_statuses(slp: s.OicWorld, values: str) -> None:
    _d(slp)["req"]["statuses"] = [
        "ORDER_STATUS_UNSPECIFIED" if v == "UNSPECIFIED" else _status(v) for v in values.split(",")
    ]


@given(parsers.parse('the request has status_filter "{value}"'))
def req_status_filter(slp: s.OicWorld, value: str) -> None:
    _d(slp)["req"]["statusFilter"] = _status(value)


@given(parsers.parse('the request has query "{text}"'))
@when(parsers.parse('the request has query "{text}"'))
def req_query(slp: s.OicWorld, text: str) -> None:
    _d(slp)["req"]["query"] = text


@given(parsers.parse("the request has a query of {n:d} runes"))
@when(parsers.parse("the request has a query of {n:d} runes"))
def req_query_runes(slp: s.OicWorld, n: int) -> None:
    _d(slp)["req"]["query"] = "á" * n


@given(parsers.parse('the request has the first 8 characters of an order id of "{name}" as query'))
def req_id_prefix(slp: s.OicWorld, name: str) -> None:
    oid = _d(slp)["orders"][name][0]["id"]
    _d(slp)["prefix"] = oid[:8]
    _d(slp)["req"]["query"] = oid[:8]


@when(parsers.parse('the request has the id of the order of "{name}" as query'))
def req_full_id(slp: s.OicWorld, name: str) -> None:
    _d(slp)["req"]["query"] = _d(slp)["orders"][name][0]["id"]


@given(parsers.parse('"{name}" holds a next_cursor from a first page of {n:d}'))
def hold_cursor(slp: s.OicWorld, name: str, n: int) -> None:
    resp = _call(slp, slp.actors[name], {"page": {"pageSize": n}})
    cursor = (s.ok(resp).get("page") or {}).get("nextCursor", "")
    assert cursor, resp.text
    _d(slp)["cursor"][name] = cursor


@given(parsers.parse('the request uses the cursor of "{name}"'))
def req_foreign_cursor(slp: s.OicWorld, name: str) -> None:
    _d(slp)["req"].setdefault("page", {})["cursor"] = _d(slp)["cursor"][name]


# ── calls ────────────────────────────────────────────────────────────────
def _call(w: s.OicWorld, actor: s.Actor | None, body: dict[str, Any]) -> httpx.Response:
    return s.post(w, actor, s.ORDER, "ListSellerOrders", body)


def _json(resp: httpx.Response) -> dict[str, Any]:
    return resp.json() if resp.content else {}


@when(parsers.parse('"{name}" lists seller orders'))
def list_orders(slp: s.OicWorld, name: str) -> None:
    d = _d(slp)
    d["since"] = time.time() - 1
    d["resp"] = _call(slp, slp.actors[name], d["req"])


@when("an unauthenticated caller lists seller orders")
def list_orders_anon(slp: s.OicWorld) -> None:
    d = _d(slp)
    d["resp"] = _call(slp, None, d["req"])


@when(parsers.parse('"{name}" lists seller orders {n:d} times'))
def list_orders_many(slp: s.OicWorld, name: str, n: int) -> None:
    d = _d(slp)
    d["durations"] = []
    for _ in range(n):
        t0 = time.perf_counter()
        resp = _call(slp, slp.actors[name], d["req"])
        d["durations"].append((time.perf_counter() - t0) * 1000)
        s.ok(resp)
    d["resp"] = resp


@when(parsers.parse('"{name}" follows next_cursor until it is empty'))
def follow_cursor(slp: s.OicWorld, name: str) -> None:
    d = _d(slp)
    body = {k: v for k, v in d["req"].items()}
    d["pages"] = []
    for _ in range(100):
        resp = _call(slp, slp.actors[name], body)
        data = s.ok(resp)
        d["pages"].append(data)
        cursor = (data.get("page") or {}).get("nextCursor", "")
        if not cursor:
            return
        body = {**d["req"], "page": {**d["req"]["page"], "cursor": cursor}}
    raise AssertionError("next_cursor never became empty")


# ── assertions ───────────────────────────────────────────────────────────
def _data(w: s.OicWorld) -> dict[str, Any]:
    return s.ok(_d(w)["resp"])


def _orders(w: s.OicWorld) -> list[dict[str, Any]]:
    return _data(w).get("orders", [])


def _total(data: dict[str, Any]) -> int:
    return int((data.get("page") or {}).get("total", 0) or 0)


@then("the call succeeds")
def call_succeeds(slp: s.OicWorld) -> None:
    resp = _d(slp)["resp"]
    assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:300]}"


@then(parsers.parse("the call fails with {code}"))
def call_fails(slp: s.OicWorld, code: str) -> None:
    assert s.code_of(_d(slp)["resp"]) == code, _d(slp)["resp"].text[:300]


@then("no order is returned")
def no_order(slp: s.OicWorld) -> None:
    resp = _d(slp)["resp"]
    assert resp.status_code >= 400, "expected a failed call"
    assert not _json(resp).get("orders"), resp.text[:300]


@then("the response carries no status_counts")
def no_counts(slp: s.OicWorld) -> None:
    resp = _d(slp)["resp"]
    assert resp.status_code >= 400, "expected a failed call"
    assert not _json(resp).get("statusCounts"), resp.text[:300]


@then(parsers.parse("the response holds {n:d} orders"))
def holds_n(slp: s.OicWorld, n: int) -> None:
    assert len(_orders(slp)) == n, f"{len(_orders(slp))} orders, expected {n}"


@then("the next_cursor is non-empty")
def cursor_non_empty(slp: s.OicWorld) -> None:
    assert (_data(slp).get("page") or {}).get("nextCursor"), _data(slp).get("page")


@then("the next_cursor is empty")
def cursor_empty(slp: s.OicWorld) -> None:
    assert not (_data(slp).get("page") or {}).get("nextCursor"), _data(slp).get("page")


@then(parsers.parse("page.total is {n:d}"))
def total_is(slp: s.OicWorld, n: int) -> None:
    assert _total(_data(slp)) == n, _data(slp).get("page")


def _counts(data: dict[str, Any]) -> list[tuple[str, int]]:
    return [
        (_short(c.get("status", "")), int(c.get("count", 0) or 0))
        for c in data.get("statusCounts", [])
    ]


@then(
    parsers.parse(
        "status_counts is PENDING {a:d}, PAID {b:d}, SHIPPED {c:d}, COMPLETED {d:d}, CANCELLED {e:d}"
    )
)
def counts_are(slp: s.OicWorld, a: int, b: int, c: int, d: int, e: int) -> None:
    expected = list(zip(STATUSES, [a, b, c, d, e], strict=True))
    assert _counts(_data(slp)) == expected, _data(slp).get("statusCounts")


@then(parsers.parse('the response holds exactly the {kinds} orders of "{name}"'))
def holds_exactly(slp: s.OicWorld, kinds: str, name: str) -> None:
    wanted = {_status(k) for k in kinds.split(" and ")}
    expected = {o["id"] for o in _d(slp)["orders"][name] if o["status"] in wanted}
    got = [o["id"] for o in _orders(slp)]
    assert expected and set(got) == expected and len(got) == len(expected), (got, expected)


@then(parsers.parse('the response holds only the order titled "{title}"'))
def holds_titled(slp: s.OicWorld, title: str) -> None:
    expected = [o["id"] for v in _d(slp)["orders"].values() for o in v if o["title"] == title]
    assert len(expected) == 1, expected
    assert [o["id"] for o in _orders(slp)] == expected, _orders(slp)
    assert _total(_data(slp)) == 1 or "page" not in _data(slp)


@then("the response holds that order and every own order sharing the prefix")
def holds_prefix(slp: s.OicWorld) -> None:
    prefix = _d(slp)["prefix"].lower()
    own = [o["id"] for v in _d(slp)["orders"].values() for o in v]
    expected = {i for i in own if i.lower().startswith(prefix)}
    assert expected
    assert {o["id"] for o in _orders(slp)} == expected


@then(parsers.parse('every returned order belongs to "{name}"'))
def belongs_to(slp: s.OicWorld, name: str) -> None:
    orders = _orders(slp)
    assert orders, "expected at least one order"
    assert {o.get("sellerId") for o in orders} == {slp.actors[name].user_id}, orders


@then(parsers.parse("the pages hold {sizes}"))
def pages_hold(slp: s.OicWorld, sizes: str) -> None:
    expected = [int(x) for x in re.findall(r"\d+", sizes)]
    assert [len(p.get("orders", [])) for p in _d(slp)["pages"]] == expected


@then("no order id repeats across the pages")
def no_repeat(slp: s.OicWorld) -> None:
    ids = [o["id"] for p in _d(slp)["pages"] for o in p.get("orders", [])]
    assert len(ids) == len(set(ids)) and ids


def _created(order: dict[str, Any]) -> str:
    return order.get("createdAt", "")


def _instant(stamp: str) -> tuple[datetime, int]:
    base, _, frac = stamp.rstrip("Z").partition(".")
    nanos = int((frac + "000000000")[:9])
    return datetime.fromisoformat(base).replace(tzinfo=UTC), nanos


@then("created_at never increases across the pages")
def created_desc(slp: s.OicWorld) -> None:
    stamps = [_created(o) for p in _d(slp)["pages"] for o in p.get("orders", [])]
    assert stamps and all(stamps), "orders carry no createdAt"
    instants = [_instant(x) for x in stamps]
    assert all(a >= b for a, b in zip(instants, instants[1:], strict=False)), stamps


@then(parsers.parse("every page reports page.total {n:d}"))
def every_total(slp: s.OicWorld, n: int) -> None:
    assert [_total(p) for p in _d(slp)["pages"]] == [n] * len(_d(slp)["pages"])


@then(parsers.parse('the response holds all {n:d} PAID orders of "{name}" newest first'))
def holds_all_paid(slp: s.OicWorld, n: int, name: str) -> None:
    expected = {o["id"] for o in _d(slp)["orders"][name] if o["status"] == s.PAID}
    assert len(expected) == n
    orders = _orders(slp)
    assert {o["id"] for o in orders} == expected and len(orders) == n
    stamps = [_created(o) for o in orders]
    assert all(a >= b for a, b in zip(stamps, stamps[1:], strict=False))


@then(parsers.parse('the response holds all {n:d} orders of "{name}" newest first'))
def holds_all(slp: s.OicWorld, n: int, name: str) -> None:
    expected = {o["id"] for o in _d(slp)["orders"][name]}
    orders = _orders(slp)
    assert {o["id"] for o in orders} == expected and len(orders) == n
    stamps = [_created(o) for o in orders]
    assert all(a >= b for a, b in zip(stamps, stamps[1:], strict=False))


@then("the response equals the response of the same request with an empty query")
def equals_empty_query(slp: s.OicWorld) -> None:
    d = _d(slp)
    name = next(iter(slp.actors))
    seller = next(a for k, a in slp.actors.items() if k in d["orders"])
    other = _call(slp, seller, {k: v for k, v in d["req"].items() if k != "query"})
    first, second = s.ok(d["resp"]), s.ok(other)
    assert [o["id"] for o in first.get("orders", [])] == [o["id"] for o in second.get("orders", [])]
    assert first.get("page") == second.get("page")
    assert first.get("statusCounts") == second.get("statusCounts")
    assert first.get("orders"), f"no order to compare for {name}"


@then(parsers.parse("the 95th percentile of the call durations is at most {ms:d} ms"))
def p95(slp: s.OicWorld, ms: int) -> None:
    durations = sorted(_d(slp)["durations"])
    assert len(durations) == 50
    value = durations[int(0.95 * len(durations)) - 1]
    assert value <= ms, f"p95 {value:.0f} ms > {ms} ms"


@then(parsers.parse('no team-order-svc log line since the call contains "{needle}"'))
def log_clean(slp: s.OicWorld, needle: str) -> None:
    container = os.getenv("ORDER_CONTAINER") or os.getenv(
        "PLP_TEAM_ORDER_CONTAINER", "team-order-svc"
    )
    since = str(int(_d(slp)["since"]))
    proc = subprocess.run(
        ["docker", "logs", "--since", since, container],
        capture_output=True,
        text=True,
        timeout=60,
        check=True,
    )
    lines = (proc.stdout + proc.stderr).splitlines()
    assert lines, f"no log line from {container} since the call: the check would be vacuous"
    assert not [ln for ln in lines if needle in ln]
