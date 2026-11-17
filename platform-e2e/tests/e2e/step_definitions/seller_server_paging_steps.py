"""Steps of seller/seller_server_paging.feature (change seller-server-paging-ui, initiative seller-centre).

Black box in the browser on the seller app. Orders and listings are seeded through the gateway with
real checkouts, payments, status updates and CreateListing calls; outages stop the real service
container through the docker CLI and the container is started again in teardown.
"""

from __future__ import annotations

import base64
import json
import os
import re
import subprocess
import time
import urllib.parse
import uuid
from concurrent.futures import ThreadPoolExecutor
from typing import Any

import httpx
import pytest
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import timeouts
from src.core.base_page import wait_streamed
from src.models import User
from src.pages import SellerPagingPage
from tests.e2e.flows import login_via_api
from tests.e2e.flows.stack_flow import stop_container
from tests.e2e.support import oic_inv_stack as stack
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

DESKTOP = {"width": 1280, "height": 720}
SEED_WORKERS = 8
SHIPPED_LABEL = "Đang giao hàng"
LIST_SELLER_ORDERS = s.ORDER + "/ListSellerOrders"
PROBE_ROUTE = "/platform.listing.v1.ListingService/ListCategories"
LOG_WAIT_S = 15.0
BASES = ["Áo thun", "Phụ kiện", "Giày", "Túi", "Mũ"]


@pytest.fixture
def ssp() -> s.OicWorld:
    w = s.OicWorld()
    w.data["ssp"] = {
        "orders": [],
        "listings": [],
        "tag": uuid.uuid4().hex[:6],
        "page1": [],
        "since": 0.0,
        "durations": [],
        "chosen": [],
    }
    return w


def _d(w: s.OicWorld) -> dict[str, Any]:
    return w.data["ssp"]


def _page(world: World) -> SellerPagingPage:
    return SellerPagingPage(world.page)


def _seller(w: s.OicWorld) -> s.Actor:
    if "seller" not in w.actors:
        s.register(w, "seller", "seller")
    return w.actors["seller"]


def _login(world: World, w: s.OicWorld) -> None:
    actor = _seller(w)
    user = User(username=actor.username, password=get_settings().seed_password, role="seller")
    world.page.set_viewport_size(DESKTOP)
    login_via_api(world, user)


def _listing_body(title: str, status: str, stock: int = 5) -> dict[str, Any]:
    return {
        "listing": {
            "title": title,
            "categoryId": "cat-electronics",
            "price": 100_000,
            "stock": stock,
            "status": status,
            "currency": "VND",
            "description": "seller server paging e2e",
        }
    }


def _create_listing(w: s.OicWorld, title: str, status: str = "LISTING_STATUS_PUBLISHED") -> str:
    resp = s.post(w, _seller(w), s.LISTING, "CreateListing", _listing_body(title, status))
    listing = s.ok(resp).get("listing") or {}
    assert listing.get("id"), resp.text
    return listing["id"]


def _buyer(w: s.OicWorld, idx: int) -> s.Actor:
    key = f"buyer-{idx}"
    if key not in w.actors:
        buyer = s.register(w, key, "buyer")
        s.ensure_address(w, buyer)
    return w.actors[key]


def _seed_orders(w: s.OicWorld, plan: list[str]) -> None:
    seller = _seller(w)
    tag = _d(w)["tag"]
    workers = min(SEED_WORKERS, max(1, len(plan)))
    buyers = [_buyer(w, i) for i in range(workers)]

    def place(args: tuple[int, int, str]) -> dict[str, str]:
        worker, n, status = args
        title = f"Hàng đơn {tag}{n:04d}"
        listing_id = _create_listing(w, title)
        buyer = buyers[worker]
        s.ok(s.post(w, buyer, s.CART, "AddToCart", {"listingId": listing_id, "quantity": 1}))
        orders = s.ok(s.checkout(w, buyer)).get("orders", [])
        assert len(orders) == 1, orders
        oid = orders[0]["id"]
        s.put_in_status(w, oid, buyer, seller, status)
        return {"id": oid, "status": status, "title": title}

    def run(worker: int) -> list[tuple[int, dict[str, str]]]:
        return [(n, place((worker, n, plan[n]))) for n in range(worker, len(plan), workers)]

    with ThreadPoolExecutor(workers) as pool:
        placed = [row for rows in pool.map(run, range(workers)) for row in rows]
    _d(w)["orders"].extend(order for _, order in sorted(placed, key=lambda x: x[0]))


def _seed_bulk_orders(w: s.OicWorld, count: int) -> None:
    seller = _seller(w)
    workers = 16
    buyers = [_buyer(w, i) for i in range(workers)]
    stock = _create_listing_stock(w, f"Kho {_d(w)['tag']}", count + 50)

    def place(worker: int) -> list[str]:
        buyer = buyers[worker]
        placed = []
        for _ in range(worker, count, workers):
            s.ok(s.post(w, buyer, s.CART, "AddToCart", {"listingId": stock, "quantity": 1}))
            orders = s.ok(s.checkout(w, buyer)).get("orders", [])
            assert len(orders) == 1, orders
            placed.append(orders[0]["id"])
        return placed

    with ThreadPoolExecutor(workers) as pool:
        ids = [i for chunk in pool.map(place, range(workers)) for i in chunk]
    _d(w)["orders"].extend({"id": i, "status": s.PENDING, "title": "bulk"} for i in ids)
    assert seller


def _seed_sql_orders(w: s.OicWorld, count: int) -> None:
    seller = _seller(w)
    buyer = _buyer(w, 0)
    tag = _d(w)["tag"]
    sql = (
        "WITH o AS (INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, items_subtotal,"
        " created_at, updated_at) SELECT gen_random_uuid()::text, "
        f"{_lit(buyer.user_id)}, {_lit(seller.user_id)}, 1, 100000, 100000,"
        " now() - g * interval '1 second', now() - g * interval '1 second'"
        f" FROM generate_series(1, {int(count)}) AS g RETURNING id)"
        " INSERT INTO order_items (id, order_id, listing_id, title, quantity, unit_price)"
        f" SELECT gen_random_uuid()::text, o.id, {_lit('ssp-bulk-' + tag)},"
        f" {_lit('Hàng đơn ' + tag)}, 1, 100000 FROM o"
    )
    proc = stack.docker(
        "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", "order_db",
        "-v", "ON_ERROR_STOP=1", "-Atq", "-c", sql,
        check=False,
    )  # fmt: skip
    assert proc.returncode == 0, proc.stderr[-400:]
    _d(w)["orders"].extend({"id": "", "status": s.PENDING, "title": "sql"} for _ in range(count))


def _lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def _gateway_lines(since: int) -> list[dict[str, Any]]:
    proc = pe.docker("logs", "--since", str(since), pe.gateway_container(), check=False)
    lines = []
    for raw in (proc.stdout + proc.stderr).splitlines():
        if '"edge.request"' not in raw:
            continue
        try:
            line = json.loads(raw)
        except ValueError:
            continue
        if line.get("msg") == "edge.request":
            lines.append(line)
    return lines


def _seller_fingerprint(w: s.OicWorld) -> str:
    request_id = uuid.uuid4().hex
    since = int(time.time()) - 1
    resp = pe.post_json(
        PROBE_ROUTE,
        {},
        token=_seller(w).token,
        headers={"X-Request-Id": request_id},
    )
    assert resp.status_code == 200, resp.text[:300]
    deadline = time.monotonic() + LOG_WAIT_S
    while True:
        for line in _gateway_lines(since):
            if line.get("request_id") == request_id and line.get("principal_fp"):
                return line["principal_fp"]
        assert time.monotonic() < deadline, "no edge.request line for the seller probe"
        time.sleep(0.5)


def _list_calls(w: s.OicWorld) -> list[dict[str, Any]]:
    fp = _d(w)["fp"]
    return [
        line
        for line in _gateway_lines(_d(w)["log_since"])
        if line.get("method") == LIST_SELLER_ORDERS and line.get("principal_fp") == fp
    ]


def _cursor_id(href: str) -> str:
    query = urllib.parse.parse_qs(urllib.parse.urlsplit(href).query)
    raw = (query.get("cursor") or [""])[0]
    assert raw, href
    payload = base64.urlsafe_b64decode(raw + "=" * (-len(raw) % 4))
    return json.loads(payload)["id"]


def _create_listing_stock(w: s.OicWorld, title: str, stock: int) -> str:
    body = _listing_body(title, "LISTING_STATUS_PUBLISHED", stock)
    resp = s.post(w, _seller(w), s.LISTING, "CreateListing", body)
    listing = s.ok(resp).get("listing") or {}
    assert listing.get("id"), resp.text
    return listing["id"]


def _title_for(index: int, tag: str) -> str:
    base = BASES[index % 5] if index % 5 == 0 else ("Quần" if index % 7 == 3 else "Phụ kiện")
    if base == "Áo thun" and index % 2:
        base = "ÁO THUN"
    return f"{base} {tag}{index:02d}"


def _seed_listings(w: s.OicWorld, published: int, drafts: int) -> None:
    _seller(w)
    total = published + drafts
    tag = _d(w)["tag"]
    pattern = [
        ("LISTING_STATUS_DRAFT" if i % 3 == 2 else "LISTING_STATUS_PUBLISHED") for i in range(total)
    ]
    assert pattern.count("LISTING_STATUS_DRAFT") == drafts

    def create(i: int) -> dict[str, Any]:
        title = _title_for(i, tag)
        return {
            "id": _create_listing(w, title, pattern[i]),
            "title": title,
            "status": pattern[i],
            "index": i,
        }

    with ThreadPoolExecutor(SEED_WORKERS) as pool:
        _d(w)["listings"].extend(pool.map(create, range(total)))


def _ids_on_page(world: World) -> list[str]:
    hrefs = _page(world).order_links.evaluate_all("els => els.map(e => e.getAttribute('href'))")
    return [h.rstrip("/").rsplit("/", 1)[-1].split("?")[0] for h in hrefs]


def _row_index(text: str, tag: str) -> int:
    m = re.search(rf"{tag}(\d\d)", text)
    assert m, f"row without a seeded title: {text!r}"
    return int(m.group(1))


def _product_indexes(world: World, w: s.OicWorld) -> list[int]:
    tag = _d(w)["tag"]
    rows = _page(world).product_rows(tag)
    return [_row_index(t, tag) for t in rows.all_inner_texts()]


def _listing_by_index(w: s.OicWorld) -> dict[int, dict[str, Any]]:
    return {x["index"]: x for x in _d(w)["listings"]}


def _wait_orders_loaded(world: World) -> None:
    page = _page(world)
    wait_streamed(world.page, timeouts.NAVIGATION)
    expect(page.tabs.or_(page.orders_error).first).to_be_visible(timeout=timeouts.NAVIGATION)


def _wait_dashboard_loaded(world: World) -> None:
    page = _page(world)
    wait_streamed(world.page, timeouts.NAVIGATION)
    expect(page.kpi_row).to_be_visible(timeout=timeouts.NAVIGATION)


# ── seeding ──────────────────────────────────────────────────────────────
@given(
    parsers.parse(
        "a logged-in seller owns {p:d} PENDING, {pa:d} PAID, {sh:d} SHIPPED and {c:d} COMPLETED orders"
    )
)
def seller_with_orders(world: World, ssp: s.OicWorld, p: int, pa: int, sh: int, c: int) -> None:
    _seed_orders(ssp, [s.PENDING] * p + [s.PAID] * pa + [s.SHIPPED] * sh + [s.COMPLETED] * c)
    _login(world, ssp)


@given(parsers.parse("a logged-in seller owns {n:d} PENDING orders"))
def seller_with_pending(world: World, ssp: s.OicWorld, n: int) -> None:
    _seed_orders(ssp, [s.PENDING] * n)
    _login(world, ssp)


@given(parsers.parse("a logged-in seller owns {n:d} PENDING orders seeded in the order database"))
def seller_with_sql_orders(world: World, ssp: s.OicWorld, n: int) -> None:
    _seed_sql_orders(ssp, n)
    _d(ssp)["fp"] = _seller_fingerprint(ssp)
    _d(ssp)["log_since"] = int(time.time()) - 1
    _login(world, ssp)


@given(parsers.parse("a logged-in seller owns {n:d} orders in one listing"))
def seller_with_bulk_orders(world: World, ssp: s.OicWorld, n: int) -> None:
    _seed_bulk_orders(ssp, n)
    _login(world, ssp)


@given(parsers.parse("a logged-in seller owns {pub:d} published and {dr:d} draft listings"))
def seller_with_listings(world: World, ssp: s.OicWorld, pub: int, dr: int) -> None:
    _seed_listings(ssp, pub, dr)
    _login(world, ssp)


@given(parsers.parse("the seller also owns {n:d} SHIPPED orders"))
def seller_also_orders(ssp: s.OicWorld, n: int) -> None:
    _seed_orders(ssp, [s.SHIPPED] * n)


@given("a logged-in buyer without the listing.write scope")
def buyer_without_scope(world: World, ssp: s.OicWorld) -> None:
    actor = s.register(ssp, "plain-buyer", "buyer")
    user = User(username=actor.username, password=get_settings().seed_password, role="buyer")
    world.page.set_viewport_size(DESKTOP)
    login_via_api(world, user)


# ── navigation ───────────────────────────────────────────────────────────
@when(parsers.parse('the seller opens "/seller/orders" with a search text of {n:d} characters'))
def open_long_query(world: World, n: int) -> None:
    _page(world).open("/seller/orders?q=" + "a" * n)
    _wait_orders_loaded(world)


@when(
    parsers.parse(
        'the seller opens the "{kind}" orders filtered by the id prefix of one {status} order'
    )
)
def open_prefix(world: World, ssp: s.OicWorld, kind: str, status: str) -> None:
    wanted = getattr(s, status)
    order = next(o for o in _d(ssp)["orders"] if o["status"] == wanted)
    _d(ssp)["prefix"] = order["id"][:8]
    _d(ssp)["target"] = order["id"]
    _page(world).open(f"/seller/orders?status={kind}&q={order['id'][:8]}")
    _wait_orders_loaded(world)


@when(parsers.parse('the buyer opens "{path}"'))
def buyer_opens(world: World, path: str) -> None:
    _page(world).open(path)


@when("the seller opens the orders page")
def open_orders(world: World) -> None:
    _page(world).open("/seller/orders")
    _wait_orders_loaded(world)


@when(parsers.parse('the seller opens the orders page on the "{status}" tab'))
def open_orders_status(world: World, status: str) -> None:
    _page(world).open(f"/seller/orders?status={status}")
    _wait_orders_loaded(world)


@when("the seller opens the dashboard")
def open_dashboard(world: World) -> None:
    _page(world).open("/seller")
    _wait_dashboard_loaded(world)


@when(parsers.parse('the seller opens the dashboard with "{query}"'))
def open_dashboard_query(world: World, query: str) -> None:
    _page(world).open("/seller?" + urllib.parse.quote(query, safe="=&"))
    _wait_dashboard_loaded(world)


@when(parsers.parse('the seller opens "/seller" with the cursor "{cursor}"'))
def open_dashboard_cursor(world: World, cursor: str) -> None:
    _page(world).open(f"/seller?cursor={cursor}")
    expect(_page(world).products_error).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the seller opens "/seller/orders" with the cursor "{cursor}"'))
def open_orders_cursor(world: World, cursor: str) -> None:
    _page(world).open(f"/seller/orders?cursor={cursor}")
    _wait_orders_loaded(world)


@when("the seller records the orders shown")
def record_page1(world: World, ssp: s.OicWorld) -> None:
    expect(_page(world).order_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    _d(ssp)["page1"] = _ids_on_page(world)


@when(parsers.parse('the seller clicks "{name}"'))
def click_link(world: World, name: str) -> None:
    link = world.page.get_by_role("link", name=name).first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.click()


@when(parsers.parse("the seller goes to page {n:d} of the orders"))
def go_to_page(world: World, n: int) -> None:
    page = _page(world)
    for step in range(2, n + 1):
        expect(page.next_link).to_be_visible(timeout=timeouts.NAVIGATION)
        page.next_link.click()
        expect(world.page).to_have_url(re.compile(rf"page={step}"), timeout=timeouts.NAVIGATION)
        expect(page.order_links.first).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the seller records the product titles shown")
def record_products(world: World, ssp: s.OicWorld) -> None:
    _wait_dashboard_loaded(world)
    expect(_page(world).product_rows(_d(ssp)["tag"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    _d(ssp)["page1"] = _product_indexes(world, ssp)


@when(parsers.parse('the seller searches the orders for "{text}"'))
def search_orders(world: World, text: str) -> None:
    _page(world).open("/seller/orders?q=" + urllib.parse.quote(text))
    _wait_orders_loaded(world)


@when(parsers.parse('the seller searches the orders for the text "{text}" through the search box'))
def search_orders_box(world: World, text: str) -> None:
    page = _page(world)
    page.search_box.fill(text)
    page.search_box.press("Enter")
    expect(world.page).to_have_url(re.compile("q="), timeout=timeouts.NAVIGATION)


@when(parsers.parse('the seller starts the order search "{text}" and the browser storage is read'))
def search_and_capture(world: World, ssp: s.OicWorld, text: str) -> None:
    _d(ssp)["since"] = time.time() - 1
    _d(ssp)["needle"] = text
    _page(world).open("/seller/orders?q=" + urllib.parse.quote(text))
    _wait_orders_loaded(world)


# ── orders assertions ────────────────────────────────────────────────────
def _count_of(ssp: s.OicWorld, *statuses: str) -> int:
    return sum(1 for o in _d(ssp)["orders"] if o["status"] in statuses)


@then(parsers.parse("the orders table shows {n:d} rows"))
def rows_are(world: World, n: int) -> None:
    expect(_page(world).order_links).to_have_count(n, timeout=timeouts.NAVIGATION)


@then(parsers.parse("the orders table shows between 1 and {n:d} rows"))
def rows_between(world: World, n: int) -> None:
    links = _page(world).order_links
    expect(links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    assert 1 <= links.count() <= n, links.count()


@then(parsers.parse('the "{label}" badge shows {n:d}'))
def badge_is(world: World, label: str, n: int) -> None:
    expect(_page(world).tab(label)).to_contain_text(str(n), timeout=timeouts.NAVIGATION)


@then(parsers.parse('the "{label}" badge equals the number of {status} orders of the seller'))
def badge_equals(world: World, ssp: s.OicWorld, label: str, status: str) -> None:
    n = _count_of(ssp, getattr(s, status))
    expect(_page(world).tab(label)).to_contain_text(str(n), timeout=timeouts.NAVIGATION)


@then(parsers.parse("the seller app issued exactly one ListSellerOrders call with page_size {n:d}"))
def one_list_call(world: World, ssp: s.OicWorld, n: int) -> None:
    page = _page(world)
    expect(page.order_links).to_have_count(n, timeout=timeouts.NAVIGATION)
    ids = _ids_on_page(world)
    expect(page.next_link).to_be_visible(timeout=timeouts.NAVIGATION)
    assert _cursor_id(page.next_link.get_attribute("href") or "") == ids[-1]
    deadline = time.monotonic() + LOG_WAIT_S
    while not _list_calls(ssp) and time.monotonic() < deadline:
        time.sleep(0.5)
    time.sleep(2)
    calls = _list_calls(ssp)
    assert len(calls) == 1, [c.get("request_id") for c in calls]


@then(parsers.parse('every order row shows the status "{label}"'))
def rows_show_status(world: World, label: str) -> None:
    page = _page(world)
    expect(page.order_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    rows = page.order_rows
    for text in rows.all_inner_texts():
        assert label in text, text


@then(parsers.parse("the orders shown are exactly the {status} orders of the seller"))
def shown_exactly_status(world: World, ssp: s.OicWorld, status: str) -> None:
    expect(_page(world).order_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expected = {o["id"] for o in _d(ssp)["orders"] if o["status"] == getattr(s, status)}
    assert set(_ids_on_page(world)) == expected


@then("the table shows only the order that matches the prefix")
def shows_only_prefix(world: World, ssp: s.OicWorld) -> None:
    expect(_page(world).order_links).to_have_count(1, timeout=timeouts.NAVIGATION)
    assert _ids_on_page(world) == [_d(ssp)["target"]]


@then("the URL holds a cursor and page 2")
def url_cursor_page2(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r"[?&]cursor=[^&]+"), timeout=timeouts.NAVIGATION)
    expect(world.page).to_have_url(re.compile(r"[?&]page=2(&|$)"))


@then("the orders shown share no order with the recorded orders")
def disjoint_orders(world: World, ssp: s.OicWorld) -> None:
    expect(_page(world).order_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    now = _ids_on_page(world)
    assert _d(ssp)["page1"] and now
    assert not set(now) & set(_d(ssp)["page1"]), set(now) & set(_d(ssp)["page1"])


@then('the "Trang sau" control is not rendered')
def no_next(world: World) -> None:
    expect(_page(world).next_link).to_have_count(0, timeout=timeouts.NAVIGATION)


@then("the page renders without an error")
def renders_without_error(world: World) -> None:
    page = _page(world)
    expect(page.orders_error).to_have_count(0)
    expect(page.tabs).to_be_visible()


@then(
    parsers.parse(
        'the page shows the error Alert "{text}" with a "Thử lại" link to "{path}" without a cursor'
    )
)
def error_alert_retry(world: World, text: str, path: str) -> None:
    page = _page(world)
    expect(world.page.get_by_text(text, exact=True)).to_be_visible(timeout=timeouts.NAVIGATION)
    retry = page.retry_links.first
    expect(retry).to_be_visible()
    href = retry.get_attribute("href") or ""
    assert href.split("?")[0].rstrip("/") == path.rstrip("/"), href
    assert "cursor" not in href, href


@then("the URL contains neither a cursor nor a page")
def url_clean(world: World) -> None:
    expect(world.page).not_to_have_url(
        re.compile(r"[?&](cursor|page)="), timeout=timeouts.NAVIGATION
    )


@then(parsers.parse("the orders shown are only {status} orders"))
def only_status(world: World, ssp: s.OicWorld, status: str) -> None:
    expect(_page(world).order_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    allowed = {o["id"] for o in _d(ssp)["orders"] if o["status"] == getattr(s, status)}
    assert set(_ids_on_page(world)) <= allowed


@then(parsers.parse('the page shows "{text}" with a "{link}" link to "{path}"'))
def empty_state(world: World, text: str, link: str, path: str) -> None:
    expect(world.page.get_by_text(text, exact=True)).to_be_visible(timeout=timeouts.NAVIGATION)
    target = world.page.get_by_role("link", name=link)
    expect(target).to_be_visible()
    href = target.get_attribute("href") or ""
    assert href.rstrip("/").endswith(path.rstrip("/")) and "?" not in href, href


@then("the page renders no order row and no order tabs")
def no_order_rows(world: World) -> None:
    page = _page(world)
    expect(page.onboarding_button).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.order_links).to_have_count(0)
    expect(page.tabs).to_have_count(0)


@then("the page renders no listing row and no order row")
def no_rows_dashboard(world: World, ssp: s.OicWorld) -> None:
    page = _page(world)
    expect(page.onboarding_button).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.order_links).to_have_count(0)
    expect(page.kpi_row).to_have_count(0)
    expect(page.product_rows(_d(ssp)["tag"])).to_have_count(0)


# ── dashboard assertions ─────────────────────────────────────────────────
@then(parsers.parse('the KPI "{title}" shows {n:d}'))
def kpi_shows(world: World, title: str, n: int) -> None:
    expect(_page(world).kpi(title)).to_contain_text(str(n), timeout=timeouts.NAVIGATION)


@then(parsers.parse("the product table shows {n:d} rows"))
def product_rows_are(world: World, ssp: s.OicWorld, n: int) -> None:
    expect(_page(world).product_rows(_d(ssp)["tag"])).to_have_count(n, timeout=timeouts.NAVIGATION)


@then("every product row is a draft listing and all draft listings are shown")
def only_drafts(world: World, ssp: s.OicWorld) -> None:
    expect(_page(world).product_rows(_d(ssp)["tag"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    by_index = _listing_by_index(ssp)
    shown = _product_indexes(world, ssp)
    drafts = {i for i, x in by_index.items() if x["status"] == "LISTING_STATUS_DRAFT"}
    assert set(shown) == drafts and len(shown) == len(drafts), (shown, drafts)


@then(
    parsers.parse(
        'every product row title contains "{text}" ignoring case and all matching listings are shown'
    )
)
def only_matching(world: World, ssp: s.OicWorld, text: str) -> None:
    expect(_page(world).product_rows(_d(ssp)["tag"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    by_index = _listing_by_index(ssp)
    wanted = {i for i, x in by_index.items() if text.casefold() in x["title"].casefold()}
    shown = _product_indexes(world, ssp)
    assert wanted and set(shown) == wanted and len(shown) == len(wanted), (shown, wanted)


@then("the product table shows listings not shown before")
def products_not_before(world: World, ssp: s.OicWorld) -> None:
    expect(_page(world).product_rows(_d(ssp)["tag"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    now = _product_indexes(world, ssp)
    assert now and _d(ssp)["page1"]
    assert not set(now) & set(_d(ssp)["page1"]), set(now) & set(_d(ssp)["page1"])


@then('the product block shows the error Alert with a "Thử lại" link to "/seller" without a cursor')
def product_error_retry(world: World) -> None:
    page = _page(world)
    expect(page.products_error).to_be_visible(timeout=timeouts.NAVIGATION)
    href = page.retry_links.first.get_attribute("href") or ""
    assert href.split("?")[0].rstrip("/") == "/seller" and "cursor" not in href, href


@then(
    'the product block shows the error Alert "Không tải được danh sách sản phẩm" with a "Thử lại" link'
)
def product_error(world: World) -> None:
    page = _page(world)
    expect(page.products_error).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.retry_links.first).to_be_visible()


@then("the order blocks still render")
def orders_blocks_render(world: World, ssp: s.OicWorld) -> None:
    page = _page(world)
    expect(page.kpi("Cần gửi")).to_be_visible(timeout=timeouts.NAVIGATION)
    shown = "|".join(re.escape(f"#{o['id'][:8]}") for o in _d(ssp)["orders"])
    expect(world.page.get_by_role("link", name=re.compile(f"^({shown})$")).first).to_be_visible()


@then("the page answered HTTP 200")
def http_200(world: World, ssp: s.OicWorld) -> None:
    assert _d(ssp)["status"] == 200, _d(ssp)["status"]


@then(
    'the "Cần gửi" KPI is omitted and the recent-orders block shows an error state with a retry link'
)
def orders_block_degraded(world: World) -> None:
    page = _page(world)
    expect(page.retry_links.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.kpi_row.get_by_text("Cần gửi")).to_have_count(0)
    expect(page.products_error).to_have_count(0)


@then("the product table still renders")
def product_table_renders(world: World, ssp: s.OicWorld) -> None:
    expect(_page(world).product_rows(_d(ssp)["tag"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


# ── outages ──────────────────────────────────────────────────────────────
def _wait_ready(w: s.OicWorld, timeout_s: float = 90.0) -> None:
    seller = _seller(w)
    probes = [
        lambda: s.post(w, seller, s.ORDER, "ListSellerOrders", {}).status_code == 200,
        lambda: s.post(w, seller, s.LISTING, "ListMyListings", {}).status_code == 200,
        lambda: (
            s.post(
                w, seller, s.CART, "AddToCart", {"listingId": str(uuid.uuid4()), "quantity": 1}
            ).status_code
            not in (502, 503, 504)
        ),
    ]
    deadline = time.monotonic() + timeout_s
    for probe in probes:
        while True:
            try:
                if probe():
                    break
            except httpx.HTTPError:
                pass
            if time.monotonic() >= deadline:
                raise TimeoutError("stopped service did not come back through the gateway")
            time.sleep(1)


def _container(env: str, service: str) -> str:
    key = service.upper().replace("-", "_")
    return os.getenv(env) or os.getenv(f"PLP_{key}_CONTAINER") or f"{service}-svc"


def _stop(world: World, w: s.OicWorld, env: str, service: str) -> None:
    restore = stop_container(_container(env, service))

    def back() -> None:
        restore()
        _wait_ready(w)

    world.add_cleanup(back)


@given("team-order is stopped")
def order_down(world: World, ssp: s.OicWorld) -> None:
    _stop(world, ssp, "ORDER_CONTAINER", "team-order")


@given("team-domain is stopped")
@when("team-domain is stopped")
def domain_down(world: World, ssp: s.OicWorld) -> None:
    _stop(world, ssp, "DOMAIN_CONTAINER", "team-domain")


@when("the seller opens the dashboard while the orders source is down")
def open_dashboard_orders_down(world: World, ssp: s.OicWorld) -> None:
    response = world.page.goto(
        f"{_page(world).app_base_url()}/seller", wait_until="domcontentloaded"
    )
    _d(ssp)["status"] = response.status if response else 0
    wait_streamed(world.page, timeouts.NAVIGATION)
    expect(_page(world).product_rows(_d(ssp)["tag"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the seller opens the dashboard while the listings source is down")
def open_dashboard_listings_down(world: World, ssp: s.OicWorld) -> None:
    response = world.page.goto(
        f"{_page(world).app_base_url()}/seller", wait_until="domcontentloaded"
    )
    _d(ssp)["status"] = response.status if response else 0
    wait_streamed(world.page, timeouts.NAVIGATION)
    expect(_page(world).products_error).to_be_visible(timeout=timeouts.NAVIGATION)


# ── pickers ──────────────────────────────────────────────────────────────
@when(parsers.parse('the seller opens the "{kind}" page'))
def open_picker_page(world: World, kind: str) -> None:
    _page(world).open(f"/seller/{kind}")
    expect(_page(world).picker.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_page(world).picker_options.first).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse("the picker lists {n:d} published listings"))
def picker_lists_n(world: World, n: int) -> None:
    expect(_page(world).picker_options).to_have_count(n, timeout=timeouts.NAVIGATION)


@then("every picker option is an own published listing")
def picker_published(world: World, ssp: s.OicWorld) -> None:
    published = {
        x["title"] for x in _d(ssp)["listings"] if x["status"] == "LISTING_STATUS_PUBLISHED"
    }
    texts = _page(world).picker_options.all_inner_texts()
    assert texts
    for text in texts:
        assert any(title in text for title in published), text


@when(parsers.parse('the seller types "{text}" in the picker and waits for the options to settle'))
def picker_type(world: World, ssp: s.OicWorld, text: str) -> None:
    page = _page(world)
    page.picker_search.fill(text)
    _d(ssp)["typed"] = text


@then(
    parsers.parse(
        'the picker lists only own published listings whose title contains "{text}" ignoring case'
    )
)
def picker_only_matching(world: World, ssp: s.OicWorld, text: str) -> None:
    wanted = {
        x["title"]
        for x in _d(ssp)["listings"]
        if x["status"] == "LISTING_STATUS_PUBLISHED" and text.casefold() in x["title"].casefold()
    }
    assert wanted
    options = _page(world).picker_options
    expect(options).to_have_count(len(wanted), timeout=timeouts.NAVIGATION)
    texts = options.all_inner_texts()
    assert {t for t in wanted if any(t in x for x in texts)} == wanted, (texts, wanted)
    assert len(texts) <= 20


@when(parsers.parse("the seller selects {n:d} listings in the picker"))
def picker_select(world: World, ssp: s.OicWorld, n: int) -> None:
    options = _page(world).picker_options
    expect(options.first).to_be_visible(timeout=timeouts.NAVIGATION)
    page = _page(world)
    titles = []
    for i in range(n):
        option = options.nth(i)
        titles.append(option.inner_text())
        page.picker_checkbox(option).click()
    _d(ssp)["chosen"] = titles
    expect(page.picker_checked).to_have_count(n, timeout=timeouts.DEFAULT)
    expect(page.selection_count(n)).to_be_visible(timeout=timeouts.DEFAULT)


@then("the two selected listings remain selected")
def picker_chosen_remain(world: World, ssp: s.OicWorld) -> None:
    page = _page(world)
    chosen = _d(ssp)["chosen"]
    expect(page.selection_count(len(chosen))).to_be_visible(timeout=timeouts.DEFAULT)
    titles = [next(x["title"] for x in _d(ssp)["listings"] if x["title"] in raw) for raw in chosen]
    options = page.picker_options
    for i in range(options.count()):
        option = options.nth(i)
        text = option.inner_text()
        checked = page.picker_checkbox(option).is_checked()
        assert checked == any(t in text for t in titles), (text, titles)


@when(
    parsers.parse('the seller fills the bundle "{name}" priced {price:d} and presses "Tạo combo"')
)
def create_bundle(world: World, name: str, price: int) -> None:
    page = _page(world)
    page.bundle_name.fill(name)
    page.bundle_price.fill(str(price))
    page.create_bundle.click()


@then("the seller has a stored bundle of exactly the two selected listings")
def bundle_stored(ssp: s.OicWorld) -> None:
    seller = _seller(ssp)
    expected = {
        next(x["id"] for x in _d(ssp)["listings"] if x["title"] in raw) for raw in _d(ssp)["chosen"]
    }
    deadline = time.monotonic() + 30
    bundles: list[dict[str, Any]] = []
    while time.monotonic() < deadline:
        resp = s.post(ssp, seller, s.LISTING, "ListBundlesBySeller", {"sellerId": seller.user_id})
        bundles = s.ok(resp).get("bundles") or []
        if bundles:
            break
        time.sleep(1)
    assert [set(b.get("listingIds", [])) for b in bundles] == [expected], bundles


@then('the picker shows "Không tìm thấy sản phẩm" and offers no option')
def picker_empty(world: World) -> None:
    page = _page(world)
    expect(page.picker_empty).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.picker_options).to_have_count(0)


@then(
    'the picker shows "Không tải được sản phẩm" with a retry control and keeps the current selection'
)
def picker_error_kept(world: World, ssp: s.OicWorld) -> None:
    page = _page(world)
    expect(page.picker_error).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.picker_retry).to_be_visible()
    expect(page.selection_count(len(_d(ssp)["chosen"]))).to_be_visible()


# ── logs and storage ─────────────────────────────────────────────────────
@then("the seller app server output since the request does not contain the search text")
def logs_clean(ssp: s.OicWorld) -> None:
    container = _container("SELLER_CONTAINER", "team-frontend-seller")
    proc = subprocess.run(
        ["docker", "logs", "--since", str(int(_d(ssp)["since"])), container],
        capture_output=True,
        text=True,
        timeout=60,
        check=True,
    )
    out = proc.stdout + proc.stderr
    assert _d(ssp)["needle"] not in out


@then("no cookie or browser storage of the seller app contains the search text")
def storage_clean(world: World, ssp: s.OicWorld) -> None:
    needle = _d(ssp)["needle"]
    cookies = world.context.cookies()
    assert not [c for c in cookies if needle in c["name"] or needle in c["value"]], cookies
    stored = world.page.evaluate(
        "() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)])"
    )
    assert needle not in stored, stored


# ── NFR ──────────────────────────────────────────────────────────────────
@when(parsers.parse("the orders page is requested {n:d} times"))
def timed_requests(world: World, ssp: s.OicWorld, n: int) -> None:
    url = f"{_page(world).app_base_url()}/seller/orders"
    durations = []
    for _ in range(n):
        t0 = time.perf_counter()
        resp = world.context.request.get(url)
        durations.append((time.perf_counter() - t0) * 1000)
        assert resp.status == 200, resp.status
    _d(ssp)["durations"] = durations


@then(parsers.parse("the 95th percentile of the orders page render is at most {ms:d} ms"))
def p95(ssp: s.OicWorld, ms: int) -> None:
    durations = sorted(_d(ssp)["durations"])
    assert durations
    value = durations[max(0, int(0.95 * len(durations)) - 1)]
    assert value <= ms, f"p95 {value:.0f} ms > {ms} ms"
