"""Steps for search/listing_stock_lookup.feature (change search-listing-stock-lookup)."""

from __future__ import annotations

import time
import uuid
from typing import Any

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.flows import listing_stock_support as lsl
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

LATENCY_CALLS = 200
LATENCY_P95_MS = 20.0
SLOW_DEADLINE_MS = 400.0
GATEWAY_PATHS = [
    "/platform.search.v1.SearchService/GetListingStock",
    "/platform.search.v1.SearchService/GetListingStock/",
]


def _state(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("lsl", {})


def _ids(world: World, *labels: str) -> list[str]:
    return [s.listing(world, label).id for label in labels]


def _ok(code: str, body: Any) -> Any:
    assert code == "ok", f"GetListingStock failed with {code}: {body}"
    return body


def _create(world: World, label: str, stock: int) -> None:
    s.create_listing(world, label, stock=stock)


def _await_ready(world: World, label: str, stock: int) -> None:
    s.wait_indexed(world, label)
    s.wait_stock(world, label, stock)


def _fresh_id() -> str:
    return str(uuid.uuid4())


# ── givens ───────────────────────────────────────────────────────────────
@given("published listings A with stock 5, B with stock 0 and C with stock 12 in the read-model")
def listings_abc(world: World) -> None:
    for label, stock in (("A", 5), ("B", 0), ("C", 12)):
        _create(world, label, stock)
    for label, stock in (("A", 5), ("B", 0), ("C", 12)):
        _await_ready(world, label, stock)


@given("a published listing with stock 1 that appears in search")
def listing_stock_one(world: World) -> None:
    _create(world, "A", 1)
    _await_ready(world, "A", 1)


@given("listing A with stock 5 and an id U that was never indexed")
def listing_a_and_unknown(world: World) -> None:
    _create(world, "A", 5)
    _await_ready(world, "A", 5)
    _state(world)["unknown"] = _fresh_id()


@given("a listing that was indexed with stock 5 and then deleted")
def listing_deleted(world: World) -> None:
    _create(world, "A", 5)
    _await_ready(world, "A", 5)
    s.delete_and_wait_consumed(world, "A")
    listed = s.listing(world, "A")
    s.eventually(
        lambda: s.hit_of(s.search(listed.title), listed.id) is None,
        f"deleted listing {listed.id} to leave search",
    )


@given("the search index is unreachable")
def index_unreachable(world: World) -> None:
    lsl.stop_opensearch()
    world.add_cleanup(lambda: (lsl.start_opensearch(), lsl.wait_search_healthy()))


@given("the search index answers slower than 200 ms")
def index_slow(world: World) -> None:
    lsl.pause_opensearch()
    world.add_cleanup(lambda: (lsl.unpause_opensearch(), lsl.wait_search_healthy()))


@given("the mainx stack with the OpenSearch overlay and 10 indexed listings")
def ten_indexed(world: World) -> None:
    labels = [f"L{i}" for i in range(10)]
    for label in labels:
        _create(world, label, 5)
    for label in labels:
        _await_ready(world, label, 5)
    _state(world)["ten"] = _ids(world, *labels)


# ── whens ────────────────────────────────────────────────────────────────
@when(parsers.parse("a service calls GetListingStock with [C, A, B]"))
def call_cab(world: World) -> None:
    _state(world)["resp"] = lsl.call(_ids(world, "C", "A", "B"))


@when(parsers.parse("a service calls GetListingStock with [A, U]"))
def call_a_u(world: World) -> None:
    _state(world)["resp"] = lsl.call([*_ids(world, "A"), _state(world)["unknown"]])


@when("a service calls GetListingStock with its id")
def call_its_id(world: World) -> None:
    _state(world)["resp"] = lsl.call(_ids(world, "A"))


@when("a service calls GetListingStock with one valid id")
def call_one_valid(world: World) -> None:
    ids = _ids(world, "A") if "A" in s.ctx(world).listings else [_fresh_id()]
    t0 = time.monotonic()
    _state(world)["resp"] = lsl.call(ids)
    _state(world)["elapsed_ms"] = (time.monotonic() - t0) * 1000


@when("a service calls GetListingStock with exactly 10 ids, then with 11 ids")
def call_ten_then_eleven(world: World) -> None:
    ids = [_fresh_id() for _ in range(11)]
    _state(world)["ten"] = lsl.call(ids[:10])
    _state(world)["eleven"] = lsl.call(ids)


@when(parsers.parse('a service calls GetListingStock with no ids, then with ["A", ""]'))
def call_empty_then_malformed(world: World) -> None:
    _state(world)["empty"] = lsl.call([])
    _state(world)["malformed"] = lsl.call([_fresh_id(), ""])


@when(
    "an anonymous caller, then a user principal with search:read, then a service principal "
    "without search:read, calls GetListingStock with one valid id"
)
def call_three_callers(world: World) -> None:
    ids = _ids(world, "A")
    _state(world)["anonymous"] = lsl.call(ids, principal=None)
    _state(world)["user"] = lsl.call(ids, principal="user")
    _state(world)["service-no-scope"] = lsl.call(ids, principal="service-no-scope")


@when("a client sends an HTTP request to the gateway for any path derived from SearchService/GetListingStock")
def call_gateway(world: World) -> None:
    st = _state(world)
    gateway = get_settings().gateway_url
    st["gw_started"] = lsl.now()
    results = []
    for path in GATEWAY_PATHS:
        request_id = uuid.uuid4().hex
        resp = httpx.post(
            gateway + path,
            json={"listingIds": [_fresh_id()]},
            headers={"Content-Type": "application/json", "X-Request-Id": request_id},
            timeout=30.0,
        )
        results.append((path, resp.status_code, resp.text[:200], request_id))
    st["gateway"] = results


@when(parsers.parse("a service calls GetListingStock with ids [A, U]"))
def call_logged(world: World) -> None:
    st = _state(world)
    st["since"] = lsl.now() - 1
    st["ids"] = [*_ids(world, "A"), st["unknown"]]
    st["resp"] = lsl.call(st["ids"])
    time.sleep(1.0)


@when("a buyer checks out quantity 1 of it through the gateway")
def buyer_checks_out(world: World) -> None:
    s.checkout(world, s.buyer_of(world), "A", 1)


@when("a service makes 200 sequential GetListingStock calls with those 10 ids")
def call_two_hundred(world: World) -> None:
    _state(world)["report"] = lsl.ghz(_state(world)["ten"], LATENCY_CALLS)


# ── thens ────────────────────────────────────────────────────────────────
@then("the response has exactly three entries [C: 12, A: 5, B: 0], with B's stock present as 0")
def check_order(world: World) -> None:
    code, body = _state(world)["resp"]
    got = lsl.stock_by_entry(_ok(code, body))
    c, a, b = _ids(world, "C", "A", "B")
    assert got == [(c, 12), (a, 5), (b, 0)], got


@then(parsers.parse("within 30 seconds GetListingStock for that listing returns stock 0"))
def check_checkout_visible(world: World) -> None:
    lid = _ids(world, "A")[0]

    def _stock() -> bool:
        code, body = lsl.call([lid])
        return lsl.stock_by_entry(_ok(code, body)) == [(lid, 0)]

    s.eventually(_stock, f"GetListingStock stock of {lid} to be 0", s.VISIBLE_S, 1.0)


@then("the response has two entries, A with stock 5 and U with stock absent")
def check_unknown(world: World) -> None:
    code, body = _state(world)["resp"]
    got = lsl.stock_by_entry(_ok(code, body))
    assert got == [(_ids(world, "A")[0], 5), (_state(world)["unknown"], None)], got


@then("the response has one entry for it with stock absent")
def check_deleted(world: World) -> None:
    code, body = _state(world)["resp"]
    got = lsl.stock_by_entry(_ok(code, body))
    assert got == [(_ids(world, "A")[0], None)], got


@then("the first call returns 10 entries and the second fails with invalid_argument")
def check_ten_eleven(world: World) -> None:
    st = _state(world)
    code, body = st["ten"]
    assert len(lsl.entries(_ok(code, body))) == 10
    code, body = st["eleven"]
    assert code == "invalid_argument", f"11 ids: expected invalid_argument, got {code}: {body}"


@then("both calls fail with invalid_argument")
def check_empty_malformed(world: World) -> None:
    st = _state(world)
    for key in ("empty", "malformed"):
        code, body = st[key]
        assert code == "invalid_argument", f"{key}: expected invalid_argument, got {code}: {body}"


@then(
    "the first fails with unauthenticated, the second and third with permission_denied, "
    "and no response carries stock"
)
def check_authz(world: World) -> None:
    st = _state(world)
    for key, want in (
        ("anonymous", "unauthenticated"),
        ("user", "permission_denied"),
        ("service-no-scope", "permission_denied"),
    ):
        code, body = st[key]
        assert code == want, f"{key}: expected {want}, got {code}: {body}"
        assert "stock" not in str(body), f"{key}: error carries stock: {body}"


@then("the gateway answers HTTP 404 or 501 for every path")
def check_gateway_not_exposed(world: World) -> None:
    for path, status, text, _ in _state(world)["gateway"]:
        assert status in (404, 501), f"{path}: expected HTTP 404 or 501, got {status}: {text}"


@then("team-search records no GetListingStock call")
def check_no_call_recorded(world: World) -> None:
    time.sleep(1.0)
    calls = lsl.search_calls_after(_state(world)["gw_started"])
    assert not calls, f"team-search logged a {lsl.METHOD} call after the gateway request: {calls[-3:]}"


@then("the call fails with unavailable and returns no entries")
def check_unavailable(world: World) -> None:
    code, body = _state(world)["resp"]
    assert code == "unavailable", f"expected unavailable, got {code}: {body}"
    assert "listings" not in str(body), f"failed call returned entries: {body}"


@then("the call fails with unavailable within 400 ms and returns no entries")
def check_slow_unavailable(world: World) -> None:
    report = lsl.ghz([_fresh_id()], 1)
    codes = lsl.status_codes(report)
    assert codes == {"Unavailable": 1}, f"expected one Unavailable, got {codes}"
    took = report["total"] / 1e6
    assert took <= SLOW_DEADLINE_MS, f"unavailable after {took:.0f} ms (> {SLOW_DEADLINE_MS:.0f} ms)"
    check_unavailable(world)


@then("the captured team-search log lines for that call contain the id count 2 and neither id value")
def check_not_logged(world: World) -> None:
    st = _state(world)
    code, body = st["resp"]
    _ok(code, body)
    logs = lsl.logs_since(st["since"])
    lines = [ln for ln in logs.splitlines() if lsl.METHOD in ln]
    assert lines, f"no {lsl.METHOD} log line captured:\n{logs[-1000:]}"
    import re

    assert any(re.search(r"\"?id_count\"?\s*[=:]\s*\"?2\b", ln) for ln in lines), f"no log line carries id_count=2: {lines}"
    for listing_id in st["ids"]:
        assert listing_id not in logs, f"id {listing_id} found in team-search logs"


@then("the p95 latency is at most 20 ms")
def check_p95(world: World) -> None:
    report = _state(world)["report"]
    assert lsl.status_codes(report) == {"OK": LATENCY_CALLS}, lsl.status_codes(report)
    p95 = lsl.percentile_ms(report, 95)
    assert p95 <= LATENCY_P95_MS, f"p95 {p95:.1f} ms > {LATENCY_P95_MS:.0f} ms"
