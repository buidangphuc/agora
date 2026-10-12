"""GA4 purchase relay steps (tracking/ga4_purchase_relay.feature, change add-ga4-purchase-relay).

Black box: Kafka in (order.events, analytics.events), a GA4 MP mock out, the relay container's
logs / env / probe port and the relay DLQ topic. Spec scenario names are echoed by the feature.
See support/ga4_relay_support.py for the stack knobs.
"""

from __future__ import annotations

import atexit
import os
import tempfile
import threading
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import ga4_relay_support as ga
from tests.e2e.support.world import World

_MOCK = ga.Ga4Mock()
_STARTED = False
_QUIET_S = 10.0  # an "exactly N" check lets the topic go quiet this long first
_SECRET_MARK = "api_secret"


def _ensure_mock() -> None:
    global _STARTED
    if not _STARTED:
        _MOCK.start()
        atexit.register(_MOCK.stop)
        _STARTED = True


def _x(world: World) -> dict:
    return world.state.extra


def _rid(world: World) -> str:
    x = _x(world)
    return x.setdefault("ga4_rid", uuid.uuid4().hex[:8])


def _n(world: World, name: str) -> str:
    """Scenario-unique id for a feature-level name (o1, b1 ...)."""
    return f"{name}-{_rid(world)}"


def _ready_path() -> str:
    return os.getenv("GA4_RELAY_READY_PATH", "/readyz")


def _wait_ready(url: str, timeout_s: float = 60.0) -> None:
    deadline = time.monotonic() + timeout_s
    last = None
    while time.monotonic() < deadline:
        try:
            r = httpx.get(url + _ready_path(), timeout=3)
            if r.status_code == 200:
                return
            last = r.status_code
        except httpx.HTTPError as exc:
            last = repr(exc)
        time.sleep(1)
    raise AssertionError(f"relay readiness probe {url}{_ready_path()} never returned 200: {last}")


# ── background ───────────────────────────────────────────────────────────
@given("the GA4 MP mock is listening and the relay targets it")
def mock_and_relay(world: World) -> None:
    _ensure_mock()
    name = ga.relay_container()
    state = ga.docker("inspect", name, "--format", "{{.State.Running}}", check=False)
    assert (
        state.stdout.strip() == "true"
    ), f"relay container {name!r} is not running (service missing): {state.stderr.strip()}"
    env = ga.container_env(name)
    assert env.get("GA4_API_SECRET"), "relay container has no GA4_API_SECRET"
    assert env.get("GA4_MEASUREMENT_ID"), "relay container has no GA4_MEASUREMENT_ID"
    endpoint = env.get("GA4_ENDPOINT", "")
    assert f":{ga.MOCK_PORT}{ga.MOCK_PATH}" in endpoint, f"relay GA4_ENDPOINT is {endpoint!r}"
    _wait_ready(ga.relay_url())
    _x(world)["ga4_secret"] = env["GA4_API_SECRET"]
    _x(world)["ga4_mock_start"] = len(_MOCK.requests)


# ── mapping events ───────────────────────────────────────────────────────
@given(parsers.parse('buyer "{buyer}" emits consented events with GA client ids "{ids}"'))
def consented_events(world: World, buyer: str, ids: str) -> None:
    for cid in (i.strip() for i in ids.split(",")):
        ga.produce_tracking(_n(world, buyer), cid, True)
    ga.wait_mapping_caught_up()


@given(parsers.parse('buyer "{buyer}" then emits an event with analytics consent denied'))
def denied_event(world: World, buyer: str) -> None:
    ga.produce_tracking(_n(world, buyer), "999.9", False)
    ga.wait_mapping_caught_up()


@given(parsers.parse('buyer "{buyer}" emits only events with analytics consent denied'))
def only_denied(world: World, buyer: str) -> None:
    ga.produce_tracking(_n(world, buyer), "999.9", False)
    ga.wait_mapping_caught_up()


@given(parsers.parse('an anonymous event with GA client id "{cid}" and consent granted is emitted'))
def anonymous_event(world: World, cid: str) -> None:
    payload = ga.tracking_event(cid, True, ga.uid("sess"))
    ga.produce(
        ga.get_settings().kafka_analytics_topic,
        ga.uid("anon"),
        ga.envelope(ga.TRACKING_TYPE, payload, principal_id=""),
    )
    ga.wait_mapping_caught_up()


# ── order events ─────────────────────────────────────────────────────────
def _items(spec: str) -> list[dict]:
    out = []
    for part in spec.split(","):
        listing, variant, qty, price = part.strip().split(":")
        out.append(
            {
                "listing_id": listing,
                "variant_id": variant,
                "quantity": int(qty),
                "unit_price": int(price),
            }
        )
    return out


def _send_paid(
    world: World, order: str, buyer: str, total: int, items: list[dict], paid: bool = True
) -> None:
    x = _x(world)
    oid, bid = _n(world, order), _n(world, buyer)
    payload = ga.order_paid(oid, bid, items, total, paid_at=int(time.time()) if paid else None)
    env = ga.envelope(ga.ORDER_PAID_TYPE, payload)
    x.setdefault("ga4_env", {})[order] = env
    x["ga4_t0"] = time.monotonic()
    ga.produce(ga.get_settings().kafka_order_topic, oid, env)


@when(
    parsers.parse(
        'order "{order}" for buyer "{buyer}" is paid with total {total:d} and items "{items}"'
    )
)
def order_paid(world: World, order: str, buyer: str, total: int, items: str) -> None:
    _send_paid(world, order, buyer, total, _items(items))


@when(
    parsers.parse(
        'order "{order}" for buyer "{buyer}" is paid with total {total:d} and items "{items}" '
        "and no paid_at"
    )
)
def order_paid_no_paid_at(world: World, order: str, buyer: str, total: int, items: str) -> None:
    _send_paid(world, order, buyer, total, _items(items), paid=False)


@when(
    parsers.parse(
        'order "{order}" for buyer "{buyer}" is paid with total {total:d} and {n:d} distinct '
        "line items"
    )
)
def order_paid_many(world: World, order: str, buyer: str, total: int, n: int) -> None:
    items = [
        {"listing_id": f"lm-{i}", "variant_id": "", "quantity": 1, "unit_price": total // n}
        for i in range(n)
    ]
    _send_paid(world, order, buyer, total, items)


@when(
    parsers.parse(
        'an order.events record of type "{type_name}" for order "{order}" and buyer "{buyer}" '
        "is produced"
    )
)
def other_type(world: World, type_name: str, order: str, buyer: str) -> None:
    oid, bid = _n(world, order), _n(world, buyer)
    payload = ga.order_paid(oid, bid, _items("l1::1:500000"), 500000, paid_at=int(time.time()))
    ga.produce_order_paid(payload, oid, type_name=type_name)


@when(parsers.parse('an OrderPaidEvent with an empty order id for buyer "{buyer}" is produced'))
def empty_order_id(world: World, buyer: str) -> None:
    bid = _n(world, buyer)
    payload = ga.order_paid("", bid, _items("l1::1:500000"), 500000, paid_at=int(time.time()))
    ga.produce_order_paid(payload, key=bid)
    _x(world)["ga4_malformed"] = [bid]


@when(
    parsers.parse(
        'an order.events record of type "{type_name}" with an undecodable payload for buyer '
        '"{buyer}" is produced'
    )
)
def undecodable(world: World, type_name: str, buyer: str) -> None:
    bid = _n(world, buyer)
    junk = b"\xff\xff\xff\xff" + bid.encode()
    ga.produce_order_paid(junk, key=bid, type_name=type_name)
    _x(world)["ga4_malformed"].append(bid)


@when(
    parsers.parse(
        'the same OrderPaidEvent for order "{order}" and buyer "{buyer}" is delivered {n:d} times'
    )
)
def redeliver(world: World, order: str, buyer: str, n: int) -> None:
    x = _x(world)
    if order not in x.get("ga4_env", {}):
        _send_paid(world, order, buyer, 500000, _items("l1::1:500000"))
        n -= 1
    for _ in range(n):
        ga.produce(ga.get_settings().kafka_order_topic, _n(world, order), x["ga4_env"][order])


@when(
    parsers.parse(
        'two OrderPaidEvent records for order "{order}" and buyer "{buyer}" are produced '
        "concurrently"
    )
)
def concurrent(world: World, order: str, buyer: str) -> None:
    oid, bid = _n(world, order), _n(world, buyer)
    payload = ga.order_paid(oid, bid, _items("l1::1:500000"), 500000, paid_at=int(time.time()))
    barrier = threading.Barrier(2)
    errors: list = []

    def run() -> None:
        try:
            env = ga.envelope(ga.ORDER_PAID_TYPE, payload)
            barrier.wait(10)
            ga.produce(ga.get_settings().kafka_order_topic, oid, env)
        except Exception as exc:  # noqa: BLE001
            errors.append(exc)

    threads = [threading.Thread(target=run) for _ in range(2)]
    [t.start() for t in threads]
    [t.join(30) for t in threads]
    assert not errors, errors


@when("the relay is killed and restarted")
def restart_relay(world: World) -> None:
    name = ga.relay_container()
    ga.docker("kill", name)
    ga.docker("start", name)
    _wait_ready(ga.relay_url(), 90)


# ── mock scripting ───────────────────────────────────────────────────────
def _script(value: str) -> list:
    out: list = []
    for item in (v.strip() for v in value.split(",")):
        out.append(int(item) if item.isdigit() else item)
    return out


@given(parsers.parse('the GA4 mock answers order "{order}" with "{replies}"'))
def mock_answers(world: World, order: str, replies: str) -> None:
    _ensure_mock()
    _MOCK.script(_n(world, order), _script(replies))


@when(parsers.parse('the GA4 mock recovers for order "{order}"'))
def mock_recovers(world: World, order: str) -> None:
    _MOCK.script(_n(world, order), [])


@when(parsers.re(r'the GA4 mock has received (?P<n>\d+) requests? for order "(?P<order>[^"]+)"'))
def mock_has_received(world: World, n: str, order: str) -> None:
    got = _MOCK.wait_for(_n(world, order), int(n), 90)
    assert len(got) >= int(n), f"mock saw {len(got)} requests for {order}, wanted {n}"
    _MOCK.quiet_for(_n(world, order), 3)


# ── request-count assertions ─────────────────────────────────────────────
def _sentinel(world: World) -> None:
    """Relay one more order of a mapped buyer: proves the relay consumed past earlier records."""
    x = _x(world)
    if x.get("ga4_sentinel_ok"):
        return
    buyer = ga.uid("sent-b")
    ga.produce_tracking(buyer, "777.7", True)
    ga.wait_mapping_caught_up()
    oid = ga.uid("sent-o")
    payload = ga.order_paid(oid, buyer, _items("ls::1:1000"), 1000, paid_at=int(time.time()))
    ga.produce(ga.get_settings().kafka_order_topic, oid, ga.envelope(ga.ORDER_PAID_TYPE, payload))
    got = _MOCK.wait_for(oid, 1, 90)
    assert got, "relay did not relay a following order of a mapped buyer (consumption stalled)"
    x["ga4_sentinel_ok"] = True


@then(parsers.re(r'the GA4 mock receives 0 requests for order "(?P<order>[^"]+)"'))
def zero_requests(world: World, order: str) -> None:
    _sentinel(world)
    got = _MOCK.quiet_for(_n(world, order), 5)
    assert not got, f"expected 0 requests for {order}, got {len(got)}: {got[0]['raw'][:300]}"


@then(
    parsers.re(r'the GA4 mock receives exactly (?P<n>\d+) requests? for order "(?P<order>[^"]+)"')
)
def exact_requests(world: World, n: str, order: str) -> None:
    oid = _n(world, order)
    _MOCK.wait_for(oid, int(n), 120)
    got = _MOCK.quiet_for(oid, _QUIET_S)
    assert len(got) == int(n), f"expected exactly {n} requests for {order}, got {len(got)}"
    _x(world)["ga4_reqs"] = got


@then(
    parsers.parse("the relay commits past the record by relaying the next order of a mapped buyer")
)
def commits_past(world: World) -> None:
    _sentinel(world)


def _last(world: World, order: str) -> dict:
    got = _MOCK.for_order(_n(world, order))
    assert got, f"no request recorded for {order}"
    return got[-1]


# ── payload assertions ───────────────────────────────────────────────────
@then(parsers.parse('the request for order "{order}" has client id "{cid}"'))
def has_client_id(world: World, order: str, cid: str) -> None:
    body = _last(world, order)["body"]
    assert body.get("client_id") == cid, body


@then(
    parsers.parse(
        'the request for order "{order}" has one purchase event with transaction id "{tx}", '
        'currency "{cur}", value {value:d} and {n:d} items each with item id, quantity and price'
    )
)
def purchase_event(world: World, order: str, tx: str, cur: str, value: int, n: int) -> None:
    body = _last(world, order)["body"]
    events = body.get("events") or []
    assert len(events) == 1 and events[0].get("name") == "purchase", events
    p = events[0].get("params") or {}
    assert p.get("transaction_id") == _n(world, tx), p
    assert p.get("currency") == cur, p
    assert int(p.get("value")) == value, p
    items = p.get("items") or []
    assert len(items) == n, items
    for it in items:
        assert it.get("item_id") and int(it.get("quantity", 0)) >= 1 and "price" in it, it


@then(parsers.parse('the request for order "{order}" body and URL contain none of "{names}"'))
def no_personal_data(world: World, order: str, names: str) -> None:
    req = _last(world, order)
    text = (req["raw"] + " " + req["raw_url"]).lower()
    for name in (n.strip() for n in names.split(",")):
        assert name not in text, f"{name!r} found in request for {order}: {text[:400]}"
    assert _n(world, "b1") not in text


@then(
    parsers.parse(
        'the URL of the request for order "{order}" carries measurement_id and api_secret only'
    )
)
def url_params(world: World, order: str) -> None:
    keys = set(_last(world, order)["query"])
    assert keys == {"measurement_id", _SECRET_MARK}, keys


@then(
    parsers.parse(
        'the items of the request for order "{order}" are "{items}" and its value is {value:d}'
    )
)
def items_value(world: World, order: str, items: str, value: int) -> None:
    p = _last(world, order)["body"]["events"][0]["params"]
    got = [(it["item_id"], int(it["quantity"]), int(it["price"])) for it in p["items"]]
    want = []
    for part in items.split(","):
        listing, variant, qty, price = part.split(":")
        item_id = listing if variant in ("-", "") else f"{listing}:{variant}"
        want.append((item_id, int(qty), int(price)))
    assert got == want, (got, want)
    assert int(p["value"]) == value, p


@then(parsers.parse('the request for order "{order}" has no timestamp_micros'))
def no_timestamp(world: World, order: str) -> None:
    body = _last(world, order)["body"]
    assert "timestamp_micros" not in body, body
    assert all("timestamp_micros" not in e for e in body.get("events", [])), body


@then(
    parsers.parse(
        'the GA4 mock receives its request for order "{order}" within {s:d} seconds of paid_at'
    )
)
def within_budget(world: World, order: str, s: int) -> None:
    got = _MOCK.wait_for(_n(world, order), 1, s)
    assert got, f"no request within {s}s"
    assert got[0]["t"] - _x(world)["ga4_t0"] <= s


@then(parsers.parse('the request for order "{order}" has exactly {n:d} items and value {value:d}'))
def capped(world: World, order: str, n: int, value: int) -> None:
    p = _last(world, order)["body"]["events"][0]["params"]
    assert len(p["items"]) == n, len(p["items"])
    assert int(p["value"]) == value


@then(parsers.parse('no request to the GA4 mock carries client id "{cid}"'))
def no_client_id(world: World, cid: str) -> None:
    _sentinel(world)
    hits = [
        r
        for r in list(_MOCK.requests)[_x(world)["ga4_mock_start"] :]
        if r["body"].get("client_id") == cid
    ]
    assert not hits, f"{len(hits)} requests carried client id {cid}"


# ── DLQ ──────────────────────────────────────────────────────────────────
@then(parsers.parse('order "{order}" reaches the relay DLQ'))
def order_in_dlq(world: World, order: str) -> None:
    assert ga.dlq_records(_n(world, order)), f"order {order} never reached {ga.dlq_topic()}"


@then("both malformed records reach the relay DLQ")
def malformed_in_dlq(world: World) -> None:
    for marker in _x(world)["ga4_malformed"]:
        assert ga.dlq_records(marker), f"record for {marker} never reached {ga.dlq_topic()}"


# ── timing ───────────────────────────────────────────────────────────────
def _gaps(world: World, order: str) -> list[float]:
    ts = [r["t"] for r in _MOCK.for_order(_n(world, order))]
    return [b - a for a, b in zip(ts, ts[1:], strict=False)]


@then(parsers.parse('the gaps between requests for order "{order}" are at least "{mins}" seconds'))
def gaps_at_least(world: World, order: str, mins: str) -> None:
    gaps = _gaps(world, order)
    for gap, lo in zip(gaps, (float(m) for m in mins.split(",")), strict=False):
        assert gap >= lo - 0.1, (gaps, mins)


@then(
    parsers.parse(
        'the gaps between requests for order "{order}" are at least "{mins}" seconds and none '
        "exceeds {cap:d} seconds"
    )
)
def gaps_schedule(world: World, order: str, mins: str, cap: int) -> None:
    gaps = _gaps(world, order)
    assert len(gaps) == len(mins.split(",")), gaps
    for gap, lo in zip(gaps, (float(m) for m in mins.split(",")), strict=False):
        assert lo - 0.1 <= gap <= cap, (gaps, mins)


@then(
    parsers.parse(
        'the second request for order "{order}" arrived about 5 seconds after the first, not '
        "after the mock answered"
    )
)
def timeout_boundary(world: World, order: str) -> None:
    (gap,) = _gaps(world, order)
    assert (
        4.9 <= gap < 8
    ), f"retry came {gap:.1f}s after the hanging attempt (timeout 5s, mock hangs 8s)"


@then(parsers.parse('the last request for order "{order}" was answered 2xx'))
def last_answered_2xx(world: World, order: str) -> None:
    # the mock answers 204 once its script for the order is empty; a recovered mock was used
    assert len(_MOCK.for_order(_n(world, order))) == 2


@then(parsers.parse('redelivering order "{order}" for buyer "{buyer}" sends nothing more'))
def redelivery_sends_nothing(world: World, order: str, buyer: str) -> None:
    oid = _n(world, order)
    before = len(_MOCK.for_order(oid))
    ga.produce(ga.get_settings().kafka_order_topic, oid, _x(world)["ga4_env"][order])
    _sentinel(world)
    assert len(_MOCK.quiet_for(oid, 5)) == before


# ── inbound surface / forged writer ──────────────────────────────────────
@when(
    parsers.parse(
        'an unauthenticated client requests the paths "{paths}" on the relay\'s listening port'
    )
)
def probe_paths(world: World, paths: str) -> None:
    x = _x(world)
    x["ga4_before"] = len(_MOCK.requests)
    results = []
    for path in (p.strip() for p in paths.split(",")):
        for method in ("GET", "POST"):
            try:
                r = httpx.request(method, ga.relay_url() + path, timeout=5)
                results.append((method, path, r.status_code))
            except httpx.TransportError:
                results.append((method, path, "refused"))
    x["ga4_probe"] = results


@then("every response is 404 or the connection is refused")
def probe_results(world: World) -> None:
    bad = [r for r in _x(world)["ga4_probe"] if r[2] not in (404, "refused")]
    assert not bad, f"unexpected inbound surface: {bad}"


@then("the GA4 mock received 0 requests from those calls")
def mock_untouched(world: World) -> None:
    time.sleep(2)
    assert len(_MOCK.requests) == _x(world)["ga4_before"]


# ── disabled mode / secret / google ──────────────────────────────────────
@when("the relay is started with an empty GA4_API_SECRET")
def start_disabled(world: World, request) -> None:  # noqa: ANN001
    x = _x(world)
    main = ga.relay_container()
    info = ga.docker("inspect", main, "--format", "{{json .}}", check=False)
    assert info.returncode == 0, f"relay container {main!r} missing: {info.stderr.strip()}"
    import json

    meta = json.loads(info.stdout)
    env = ga.container_env(main)
    env["GA4_API_SECRET"] = ""
    name = f"e2e-ga4-relay-disabled-{_rid(world)}"
    port = ga.relay_url().rsplit(":", 1)[-1]
    network = next(iter(meta["NetworkSettings"]["Networks"]))
    with tempfile.NamedTemporaryFile("w", suffix=".env", delete=False) as f:
        f.write("\n".join(f"{k}={v}" for k, v in env.items()))
    groups_before = ga.consumer_groups()
    request.addfinalizer(lambda: ga.docker("rm", "-f", name, check=False))
    entry = meta["Config"].get("Entrypoint") or []
    cmd = list(entry[1:]) + (meta["Config"].get("Cmd") or [])
    run = [
        "run", "-d", "--name", name, "--network", network, "--env-file", f.name,
        "--add-host", "host.docker.internal:host-gateway",
        "-p", f"127.0.0.1::{port}",
    ]  # fmt: skip
    if entry:
        run += ["--entrypoint", entry[0]]
    ga.docker(*run, meta["Config"]["Image"], *cmd)
    mapped = ga.docker("port", name, port).stdout.strip().splitlines()[0].rsplit(":", 1)[-1]
    before = len(_MOCK.requests)
    _wait_ready(f"http://127.0.0.1:{mapped}", 60)
    time.sleep(10)  # a consumer, if it wrongly started, would have joined its group by now
    x["ga4_dis"] = {
        "logs": ga.docker("logs", name, check=False),
        "new_groups": ga.consumer_groups() - groups_before,
        "mock_delta": len(_MOCK.requests) - before,
    }
    os.unlink(f.name)


@then(
    parsers.parse(
        'it logs "{line}" once, answers its readiness probe, joins no consumer group and the GA4 '
        "mock received 0 requests"
    )
)
def disabled_checks(world: World, line: str) -> None:
    d = _x(world)["ga4_dis"]
    logs = d["logs"].stdout + d["logs"].stderr
    assert logs.count(line) == 1, f"{line!r} logged {logs.count(line)} times: {logs[-500:]}"
    assert not d["new_groups"], f"disabled relay joined consumer groups {d['new_groups']}"
    assert d["mock_delta"] == 0


def _relay_logs() -> str:
    res = ga.docker("logs", ga.relay_container(), check=False)
    return res.stdout + res.stderr


@then("no relay log line contains the secret value")
def secret_not_logged(world: World) -> None:
    secret = _x(world)["ga4_secret"]
    assert secret not in _relay_logs(), "the GA4 API secret appears in the relay logs"


@then("the relay is configured with the mock endpoint and not a google-analytics.com host")
def endpoint_is_mock(world: World) -> None:
    endpoint = ga.container_env(ga.relay_container()).get("GA4_ENDPOINT", "")
    assert f":{ga.MOCK_PORT}{ga.MOCK_PATH}" in endpoint, endpoint
    assert "google" not in endpoint.lower(), endpoint


@then("the relay logs mention no google-analytics.com host")
def no_google_in_logs() -> None:
    assert "google-analytics.com" not in _relay_logs()
