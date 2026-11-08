from __future__ import annotations

import re
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from typing import Any

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from src.core.base_page import goto_streamed
from src.pages import SearchPage
from tests.e2e.step_definitions.discovery_steps import _seed_indexed_listings
from tests.e2e.step_definitions.pdp_streaming_steps import _record_beacons
from tests.e2e.support import adq_support as adq
from tests.e2e.support import hrp_support as h
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import roe_support as roe
from tests.e2e.support import srm_support as s
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

REPORT = "/platform.analytics.v1.AnalyticsQueryService/GetSearchPerformance"
PLACEMENT = "search_results"
HEX32 = re.compile(r"^[0-9a-f]{32}$")
MODEL = re.compile(r"^(lexical|semantic|hybrid)(\+rerank)?@\S+$")
POLL_S = 60.0
BEACON_SETTLE_MS = 2600
HOLD_S = 8.0
BASE = get_settings().base_url.rstrip("/")


def _bag(world: World) -> dict[str, Any]:
    return s.ctx(world).extra.setdefault("sa", {})


def _page(world: World) -> SearchPage:
    return world.get_page(PageName.SEARCH)  # type: ignore[return-value]


def _kw(world: World) -> str:
    return s.ctx(world).keyword


def _search(world: World, **kw: Any) -> httpx.Response:
    token = kw.pop("token", None)
    query = kw.pop("query", _kw(world))
    token = token if token is not None else s.buyer_of(world).token
    resp = s.search(query, token=token, **kw)
    assert resp.status_code == 200, f"SearchListings {resp.status_code}: {resp.text[:300]}"
    return resp


def _fresh_model(tag: str) -> str:
    return f"e2e-sa-{uuid.uuid4().hex[:8]}-{tag}@v1"


def _report(token: str | None, window_hours: int | None = 1) -> httpx.Response:
    body = {} if window_hours is None else {"windowHours": window_hours}
    return pe.post_json(REPORT, body, token, timeout=30)


def _poll(token: str, until, what: str) -> dict[str, Any]:
    deadline = time.monotonic() + POLL_S
    last: Any = None
    while time.monotonic() < deadline:
        resp = _report(token)
        assert resp.status_code == 200, f"{what}: {resp.status_code} {resp.text[:300]}"
        last = resp.json()
        if until(last):
            return last
        time.sleep(2.0)
    raise AssertionError(f"{what}: never satisfied within {POLL_S:.0f}s, last={last!r}")


def _ids(prefix: str) -> dict[str, str]:
    return {"imp": roe.unique(f"{prefix}-imp"), "anon": roe.unique(f"{prefix}-anon")}


def _beacon(kind: str, listing: str, ids: dict[str, str], model: str, placement: str = PLACEMENT):
    return roe.beacon(
        kind,
        listing_id=listing,
        anonymous_id=ids["anon"],
        impression_id=ids["imp"],
        placement_id=placement,
        model_version=model,
    )


_SORTS = {"price ascending": "SORT_BY_PRICE_ASC"}


@given(parsers.parse("two published listings share a unique keyword and are searchable"))
def two_listings(world: World) -> None:
    for label in ("one", "two"):
        h.create(world, label, f"{_kw(world)} {label}")
    for label in ("one", "two"):
        h.wait_searchable(world, label, _kw(world))


@given("the buyer owns a saved search for that keyword")
def owns_saved_search(world: World) -> None:
    resp = s.save_search(s.buyer_of(world).token, _kw(world))
    saved = s.ok_json(resp)["savedSearch"]
    s.ctx(world).saved_id = saved["id"]


@when("a buyer searches for that keyword through the gateway")
def search_default(world: World) -> None:
    _bag(world)["resp"] = _search(world)


@when("a caller with no principal searches for that keyword through the gateway")
def search_anonymous(world: World) -> None:
    resp = s.search(_kw(world))
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    _bag(world)["resp"] = resp


@when("a client searches for that keyword with page size 1 and then with the next_cursor of page 1")
def search_two_pages(world: World) -> None:
    first = h.search(_kw(world), page_size=1)
    assert first.status_code == 200, first.text[:300]
    cursor = (first.json().get("page") or {}).get("nextCursor")
    assert cursor, f"page 1 offers no next_cursor: {first.text[:300]}"
    second = h.search(_kw(world), page_size=1, cursor=cursor)
    assert second.status_code == 200, second.text[:300]
    _bag(world)["resps"] = [first, second]


@when("a client searches for that keyword 1000 times with identical parameters")
def search_thousand(world: World) -> None:
    url = get_settings().gateway_url + s.SEARCH + "SearchListings"
    body = {"query": _kw(world), "searchMode": h.LEXICAL}
    with httpx.Client(timeout=30.0) as client:

        def call(_: int) -> dict[str, Any]:
            r = client.post(url, json=body)
            assert r.status_code == 200, f"{r.status_code}: {r.text[:200]}"
            return r.json()

        with ThreadPoolExecutor(max_workers=16) as pool:
            _bag(world)["bodies"] = list(pool.map(call, range(1000)))


@when("the owner runs the saved search twice")
def run_saved_twice(world: World) -> None:
    token = s.buyer_of(world).token
    runs = [s.run_saved(token, s.ctx(world).saved_id) for _ in range(2)]
    for r in runs:
        assert r.status_code == 200, f"RunSavedSearch {r.status_code}: {r.text[:300]}"
    _bag(world)["runs"] = runs


@when("another user runs that saved search")
def run_saved_other(world: World) -> None:
    _bag(world)["resp"] = s.run_saved(s.second_buyer(world).token, s.ctx(world).saved_id)


@when(parsers.parse('a buyer searches for that keyword with search mode "{mode}"'))
def search_mode(world: World, mode: str) -> None:
    _bag(world)["resp"] = _search(world, mode=mode)


@when(
    parsers.parse(
        'a buyer searches for that keyword with search mode "{mode}" and the directive "{directive}"'
    )
)
def search_mode_directive(world: World, mode: str, directive: str) -> None:
    _bag(world)["resp"] = _search(world, query=f"{_kw(world)} {directive}", mode=mode)


@when("a buyer searches for that keyword in hybrid mode sorted by price ascending")
def search_sorted(world: World) -> None:
    _bag(world)["resp"] = _search(world, mode=h.HYBRID, sort_by=_SORTS["price ascending"])


@when("a buyer searches for that keyword in hybrid mode with the cursor 250")
def search_deep(world: World) -> None:
    resp = h.search(_kw(world), mode=h.HYBRID, cursor="250")
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    _bag(world)["resp"] = resp


@when("a buyer searches with an empty query in hybrid mode")
def search_empty(world: World) -> None:
    _bag(world)["resp"] = _search(world, query="", mode=h.HYBRID)


@when("a buyer searches for that keyword without a search mode")
def search_unspecified(world: World) -> None:
    _bag(world)["resp"] = _search(world)


@then("the response request_id matches 32 lowercase hex characters")
def request_id_shape(world: World) -> None:
    rid = _bag(world)["resp"].json().get("requestId", "")
    assert HEX32.match(rid), f"request_id {rid!r}"


@then("the response carries a valid request_id and model_version")
def request_and_model(world: World) -> None:
    body = _bag(world)["resp"].json()
    assert HEX32.match(body.get("requestId", "")), body.get("requestId")
    assert MODEL.match(body.get("modelVersion", "")), body.get("modelVersion")


@then("the two responses carry different request_id values")
def two_differ(world: World) -> None:
    ids = [r.json().get("requestId", "") for r in _bag(world)["resps"]]
    assert all(HEX32.match(i) for i in ids), ids
    assert ids[0] != ids[1], ids


@then("the 1000 responses carry 1000 distinct request_id values")
def thousand_distinct(world: World) -> None:
    ids = [b.get("requestId", "") for b in _bag(world)["bodies"]]
    assert all(HEX32.match(i) for i in ids), [i for i in ids if not HEX32.match(i)][:3]
    assert len(set(ids)) == 1000, f"{len(set(ids))} distinct of {len(ids)}"


@then("each run response request_id matches 32 lowercase hex characters")
def runs_shape(world: World) -> None:
    for r in _bag(world)["runs"]:
        rid = r.json().get("requestId", "")
        assert HEX32.match(rid), f"request_id {rid!r}"


@then("the two run responses carry different request_id values")
def runs_differ(world: World) -> None:
    a, b = (r.json().get("requestId", "") for r in _bag(world)["runs"])
    assert a and a != b, (a, b)


@then(parsers.parse('each run response model_version equals "{expected}"'))
def runs_model(world: World, expected: str) -> None:
    got = [r.json().get("modelVersion") for r in _bag(world)["runs"]]
    assert got == [expected, expected], got


@then("the call fails with NOT_FOUND and no request_id is returned")
def not_found(world: World) -> None:
    resp = _bag(world)["resp"]
    assert resp.status_code == 404, f"{resp.status_code}: {resp.text[:300]}"
    assert resp.json().get("code") == "not_found", resp.text[:300]
    assert "requestId" not in resp.json(), resp.text[:300]


@then(parsers.parse('the model_version equals "{expected}"'))
def model_equals(world: World, expected: str) -> None:
    got = _bag(world)["resp"].json().get("modelVersion")
    assert got == expected, f"model_version {got!r}, want {expected!r}"


@then(parsers.parse('the model_version does not contain "{part}"'))
def model_lacks(world: World, part: str) -> None:
    got = _bag(world)["resp"].json().get("modelVersion", "")
    assert got and part not in got, f"model_version {got!r}"


@then(parsers.parse('the model_version ends with "{suffix}"'))
def model_ends(world: World, suffix: str) -> None:
    got = _bag(world)["resp"].json().get("modelVersion", "")
    assert got.endswith(suffix), f"model_version {got!r}"


@given("the seeded admin for search performance")
def seeded_admin(world: World) -> None:
    _bag(world)["admin"] = adq.admin_token(world)


@given("a seller and a buyer for search performance")
def seller_and_buyer(world: World) -> None:
    bag = _bag(world)
    bag["seller"] = s.seller_of(world).token
    bag["buyer"], _ = tii.register_buyer()


@when("a visitor posts a search_results impression under a fresh model version")
def post_one_impression(world: World) -> None:
    bag = _bag(world)
    bag["model"] = _fresh_model("hybrid")
    roe.post([_beacon("impression", roe.unique("listing"), _ids("gw"), bag["model"])])


@then(
    'every row of the admin\'s search performance report has placement_id "search_results" and '
    "the fresh model version has a row"
)
def rows_are_search_results(world: World) -> None:
    bag = _bag(world)
    report = _poll(
        bag["admin"], lambda r: bool(roe.row(r, PLACEMENT, bag["model"])), "search performance"
    )
    wrong = [r for r in report.get("rows", []) if r.get("placementId") != PLACEMENT]
    assert not wrong, wrong


@when(parsers.parse("the admin calls GetSearchPerformance with window_hours {hours:d}"))
def admin_calls(world: World, hours: int) -> None:
    bag = _bag(world)
    bag["resp"] = _report(bag["admin"], hours)


@then("the gateway answers HTTP 400 with code invalid_argument for search performance")
def invalid_argument(world: World) -> None:
    resp = _bag(world)["resp"]
    assert resp.status_code == 400, f"{resp.status_code}: {resp.text[:300]}"
    assert resp.json().get("code") == "invalid_argument", resp.text[:300]


@then(parsers.parse("the search performance response window_hours equals {hours:d}"))
def window_equals(world: World, hours: int) -> None:
    resp = _bag(world)["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    assert int(resp.json().get("windowHours", 0)) == hours, resp.text[:300]


@when(
    "a visitor posts 10 item impressions and 2 clicks under one model version and 10 item "
    "impressions and 1 click under another"
)
def post_ctr(world: World) -> None:
    bag = _bag(world)
    bag["a"], bag["b"] = _fresh_model("hybrid"), _fresh_model("lexical")
    for model, clicks in ((bag["a"], 2), (bag["b"], 1)):
        ids = _ids("ctr")
        listings = [roe.unique("listing") for _ in range(10)]
        roe.post([_beacon("impression", lid, ids, model) for lid in listings])
        roe.post([_beacon("click", lid, ids, model) for lid in listings[:clicks]])


@then(
    "the admin's search performance report has a row with ctr 0.2 for the first and ctr 0.1 for "
    "the second"
)
def ctr_rows(world: World) -> None:
    bag = _bag(world)

    def ready(r: dict[str, Any]) -> bool:
        return (
            roe.num(roe.row(r, PLACEMENT, bag["a"]), "clicks") >= 2
            and roe.num(roe.row(r, PLACEMENT, bag["b"]), "clicks") >= 1
        )

    report = _poll(bag["admin"], ready, "ctr rows")
    a, b = roe.row(report, PLACEMENT, bag["a"]), roe.row(report, PLACEMENT, bag["b"])
    assert abs(float(a.get("ctr", 0)) - 0.2) < 1e-9, a
    assert abs(float(b.get("ctr", 0)) - 0.1) < 1e-9, b


@when(
    'a visitor posts impressions under placement "similar_items" with a fresh model version and a '
    "search_results impression under another"
)
def post_other_placement(world: World) -> None:
    bag = _bag(world)
    bag["other"], bag["model"] = _fresh_model("als"), _fresh_model("hybrid")
    roe.post(
        [_beacon("impression", roe.unique("l"), _ids("sim"), bag["other"], "similar_items")]
        + [_beacon("impression", roe.unique("l"), _ids("sr"), bag["model"])]
    )


@then("the admin's search performance report has no row for the similar_items model version")
def no_other_rows(world: World) -> None:
    bag = _bag(world)
    report = _poll(
        bag["admin"], lambda r: bool(roe.row(r, PLACEMENT, bag["model"])), "control search row"
    )
    leaked = [r for r in report.get("rows", []) if r.get("modelVersion") == bag["other"]]
    assert not leaked, leaked


@when(
    "a visitor posts a search impression of listing A and a click on listing B with the same impression_id"
)
def post_foreign_click(world: World) -> None:
    bag = _bag(world)
    bag["model"], ids = _fresh_model("hybrid"), _ids("foreign")
    a, b = roe.unique("listing-a"), roe.unique("listing-b")
    roe.post([_beacon("impression", a, ids, bag["model"]), _beacon("click", b, ids, bag["model"])])


@then("the search performance row of that model version has 1 item impression and 0 clicks")
def foreign_not_credited(world: World) -> None:
    bag = _bag(world)
    report = _poll(
        bag["admin"],
        lambda r: roe.num(roe.row(r, PLACEMENT, bag["model"]), "itemImpressions") >= 1,
        "row",
    )
    time.sleep(HOLD_S)
    again = _report(bag["admin"])
    assert again.status_code == 200, again.text[:300]
    mine = roe.row(again.json(), PLACEMENT, bag["model"])
    assert roe.num(mine, "itemImpressions") == 1 and roe.num(mine, "clicks") == 0, (report, mine)


@when("a visitor posts impressions with one impression_id under two model versions")
def post_two_models(world: World) -> None:
    bag = _bag(world)
    bag["a"], bag["b"], ids = _fresh_model("hybrid"), _fresh_model("lexical"), _ids("reuse")
    listing = roe.unique("listing")
    roe.post(
        [
            _beacon("impression", listing, ids, bag["a"]),
            _beacon("impression", listing, ids, bag["b"]),
        ]
    )


@then("the admin's search performance report has two rows, one per model version")
def two_rows(world: World) -> None:
    bag = _bag(world)
    report = _poll(
        bag["admin"],
        lambda r: all(roe.row(r, PLACEMENT, bag[k]) for k in ("a", "b")),
        "one row per model version",
    )
    rows = [r for r in report["rows"] if r.get("modelVersion") in (bag["a"], bag["b"])]
    assert len(rows) == 2, rows


@when(
    "a logged-in buyer clicks a search result and posts a purchase tracking event for it but pays "
    "no order"
)
def post_forged_purchase(world: World) -> None:
    bag = _bag(world)
    bag["model"], ids = _fresh_model("hybrid"), _ids("forged")
    token, _ = tii.register_buyer()
    listing = roe.unique("listing")
    roe.post(
        [
            _beacon("impression", listing, ids, bag["model"]),
            _beacon("click", listing, ids, bag["model"]),
            _beacon("purchase", listing, ids, bag["model"]),
        ],
        token,
    )


@then("the search performance row of that model version has 1 click and 0 purchases")
def click_no_purchase(world: World) -> None:
    bag = _bag(world)
    report = _poll(
        bag["admin"], lambda r: roe.num(roe.row(r, PLACEMENT, bag["model"]), "clicks") >= 1, "row"
    )
    mine = roe.row(report, PLACEMENT, bag["model"])
    assert roe.num(mine, "clicks") == 1 and roe.num(mine, "purchases") == 0, mine


@when("the seller and then the buyer call GetSearchPerformance through the gateway")
def seller_buyer_call(world: World) -> None:
    bag = _bag(world)
    bag["seller_resp"], bag["buyer_resp"] = _report(bag["seller"]), _report(bag["buyer"])


@then("the gateway answers HTTP 403 for both")
def both_403(world: World) -> None:
    bag = _bag(world)
    for who in ("seller_resp", "buyer_resp"):
        assert bag[who].status_code == 403, f"{who}: {bag[who].status_code} {bag[who].text[:300]}"


@when("an anonymous client calls GetSearchPerformance through the gateway")
def anonymous_call(world: World) -> None:
    _bag(world)["resp"] = _report(None)


@then("the gateway answers HTTP 401 for search performance")
def anonymous_401(world: World) -> None:
    resp = _bag(world)["resp"]
    assert resp.status_code == 401, f"{resp.status_code}: {resp.text[:300]}"


@given(parsers.parse("{count:d} listings are indexed under one keyword for the search page"))
def seed_for_page(world: World, count: int) -> None:
    _bag(world)["keyword"] = _seed_indexed_listings(world, count, "zsa")


def _search_beacons(world: World, kind: str, start: int = 0) -> list[dict]:
    return [
        b
        for b in world.state.extra["beacons"][start:]
        if b.get("type") == kind and b.get("placementId") == PLACEMENT
    ]


def _wait_beacon(world: World, kind: str, start: int = 0) -> list[dict]:
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        found = _search_beacons(world, kind, start)
        if found:
            return found
        world.page.wait_for_timeout(500)
    return []


def _open_recording(world: World, query: str, **params: str) -> None:
    _record_beacons(world)
    page = _page(world)
    page.open_query(query, **params)
    page.scroll_first_result_into_view()


@when("the buyer opens the search results for that keyword recording the beacons")
def open_recording(world: World) -> None:
    _open_recording(world, _bag(world)["keyword"])
    _bag(world)["first"] = _wait_beacon(world, "impression")


@when(
    "the buyer opens the search results for that keyword with a failing embedding directive recording the beacons"
)
def open_recording_failing(world: World) -> None:
    _open_recording(world, f"{_bag(world)['keyword']} [[fake status=500]]")
    _bag(world)["first"] = _wait_beacon(world, "impression")


@when("the buyer clicks the first search result")
def click_first(world: World) -> None:
    _page(world).open_first_result()
    _bag(world)["clicks"] = _wait_beacon(world, "click")


@when(parsers.parse('the buyer picks the sort "{label}"'))
def pick_sort(world: World, label: str) -> None:
    bag = _bag(world)
    bag["mark"] = len(world.state.extra["beacons"])
    _page(world).sort_link(label).click()
    world.page.wait_for_url(re.compile(r"sort="), timeout=timeouts.NAVIGATION)
    _page(world).scroll_first_result_into_view()
    bag["second"] = _wait_beacon(world, "impression", bag["mark"])


@when("the buyer moves to page 2 of the results")
def go_page_two(world: World) -> None:
    bag = _bag(world)
    pager = _page(world).page_link(2)
    pager.scroll_into_view_if_needed()
    world.page.wait_for_timeout(BEACON_SETTLE_MS)
    bag["mark"] = len(world.state.extra["beacons"])
    pager.click()
    world.page.wait_for_url(re.compile(r"page=2"), timeout=timeouts.NAVIGATION)
    _page(world).scroll_first_result_into_view()
    bag["second"] = _wait_beacon(world, "impression", bag["mark"])


@when("the buyer searches the page for a term with no matches recording the beacons")
def search_no_match(world: World) -> None:
    _record_beacons(world)
    _page(world).open_query(h.word())


@then(
    "the search_results impression beacons and the click beacon carry one 32 hex impression_id, "
    'one model_version and placement_id "search_results"'
)
def beacons_carry_identity(world: World) -> None:
    bag = _bag(world)
    impressions, clicks = bag["first"], bag["clicks"]
    assert impressions, f"no search_results impression beacon: {world.state.extra['beacons']}"
    assert clicks, f"no search_results click beacon: {world.state.extra['beacons']}"
    every = impressions + clicks
    ids = {b.get("impressionId") for b in every}
    models = {b.get("modelVersion") for b in every}
    assert len(ids) == 1 and HEX32.match(str(next(iter(ids)))), ids
    assert len(models) == 1 and MODEL.match(str(next(iter(models)))), models


@then(
    "the second page of search_results beacons carries an impression_id different from the first page's"
)
def second_id_differs(world: World) -> None:
    bag = _bag(world)
    first = {b.get("impressionId") for b in bag["first"]}
    second = {b.get("impressionId") for b in bag["second"]}
    assert second, "no search_results impression beacon after the second navigation"
    assert all(HEX32.match(str(i)) for i in first | second), (first, second)
    assert not (first & second), (first, second)


@then('the search_results beacons carry a model_version that starts with "lexical@"')
def beacons_lexical(world: World) -> None:
    beacons = _bag(world)["first"]
    assert beacons, f"no search_results impression beacon: {world.state.extra['beacons']}"
    wrong = [b for b in beacons if not str(b.get("modelVersion", "")).startswith("lexical@")]
    assert not wrong, wrong


@then("the page renders the empty state and sends no search_results impression beacon")
def empty_no_impression(world: World) -> None:
    expect(_page(world).empty_state).to_be_visible(timeout=timeouts.NAVIGATION)
    world.page.wait_for_load_state("networkidle")
    assert not _search_beacons(world, "impression"), _search_beacons(world, "impression")


@then(
    "a following search for the indexed keyword sends impression beacons carrying an impression_id"
)
def control_search(world: World) -> None:
    bag = _bag(world)
    mark = len(world.state.extra["beacons"])
    page = _page(world)
    goto_streamed(world.page, f"{BASE}/search?q={bag['keyword']}")
    page.scroll_first_result_into_view()
    found = _wait_beacon(world, "impression", mark)
    assert found, f"no search_results impression beacon: {world.state.extra['beacons'][mark:]}"
    assert all(HEX32.match(str(b.get("impressionId", ""))) for b in found), found
