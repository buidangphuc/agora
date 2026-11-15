"""Steps for change analytics-experiment-reports (retention, experiment readout, AI surface, exposure)."""

from __future__ import annotations

import json
import time
from datetime import timedelta
from typing import Any

from pytest_bdd import given, then, when

from tests.e2e.support import adq_support as adq
from tests.e2e.support import efe_support as e
from tests.e2e.support import experiment_reports_support as x
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

P95_BUDGET_S = 2.0
CALLS = 30


def _bag(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("exr", {})


@given("the seeded admin for experiment reports")
def seeded_admin(world: World) -> None:
    _bag(world)["admin"] = adq.admin_token(world)


@given("a buyer and a seller for experiment reports")
def buyer_and_seller(world: World) -> None:
    w = oic.OicWorld()
    _bag(world)["buyer"] = oic.register(w, "exr-buyer", "buyer").token
    _bag(world)["seller"] = oic.register(w, "exr-seller", "seller").token


def _week_cohort(report: dict[str, Any], week: str) -> dict[str, Any]:
    for cohort in report.get("cohorts", []):
        if str(cohort.get("cohortWeek", ""))[:10] == week:
            return cohort
    return {}


@when("3 buyers sign up through the gateway, one of them posting a second sign_up beacon")
def three_sign_ups(world: World) -> None:
    bag = _bag(world)
    week = x.monday(x.now())
    bag["week"] = week.date().isoformat()
    body = lambda: x.retention_body(week, 1)  # noqa: E731
    before = x.ok_json(x.call(x.RETENTION, body(), bag["admin"]))
    bag["size_before"] = x.num(_week_cohort(before, bag["week"]), "cohortSize")
    tokens = [tii.register_buyer()[0] for _ in range(3)]
    for token in tokens:
        x.post_beacons([x.beacon("sign_up")], token)
    x.post_beacons([x.beacon("sign_up")], tokens[0])


@then(
    "the admin's retention report for the current week has a cohort whose size grew by 3 and "
    "whose cohort_week is this week's UTC Monday"
)
def cohort_grew(world: World) -> None:
    bag = _bag(world)
    week = x.monday(x.now())
    want = bag["size_before"] + 3

    def ready(r: dict[str, Any]) -> bool:
        return x.num(_week_cohort(r, bag["week"]), "cohortSize") >= want

    report = x.poll(x.RETENTION, lambda: x.retention_body(week, 1), bag["admin"], ready, "cohort")
    time.sleep(x.HOLD_S)
    report = x.ok_json(x.call(x.RETENTION, x.retention_body(week, 1), bag["admin"]))
    cohort = _week_cohort(report, bag["week"])
    assert x.num(cohort, "cohortSize") == want, (bag["size_before"], cohort)


@when("the admin requests 1 retention cohort starting at a Monday two years ago")
def empty_retention(world: World) -> None:
    bag = _bag(world)
    start = x.monday(x.now() - timedelta(days=730))
    bag["resp"] = x.call(x.RETENTION, x.retention_body(start, 1), bag["admin"])


@then("the retention response succeeds with 0 cohorts")
def retention_empty(world: World) -> None:
    body = x.ok_json(_bag(world)["resp"])
    assert body.get("cohorts", []) == [], body


@when(
    'a logged-in buyer is exposed to variant "control" of a fresh flag and then pays an order, and '
    "another visitor is exposed to the same variant and never pays"
)
def exposed_and_pays(world: World) -> None:
    bag = _bag(world)
    bag["flag"] = x.unique("flag")
    w = oic.OicWorld()
    e.seller_with_listings(w, 1)
    buyer = oic.register(w, "b1", "buyer")
    oic.post(w, buyer, oic.CART, "ClearCart", {})
    oic.ensure_address(w, buyer)
    oic.add_to_cart(w, buyer, "L1", 1)
    orders = oic.ok(oic.checkout(w, buyer)).get("orders", [])
    assert len(orders) == 1, orders
    x.post_beacons([x.exposure(bag["flag"], "control")], buyer.token)
    x.post_beacons([x.exposure(bag["flag"], "control", x.unique("anon"))])
    time.sleep(1.5)
    oic.pay_order(w, buyer, orders[0]["id"])
    oic.wait_status(w, buyer, orders[0]["id"], oic.PAID)


@then(
    'the readout of that flag has variant "control" with exposed 2, converted 1 and rate 0.5 '
    "inside its Wilson interval and window_days 7"
)
def readout_converted(world: World) -> None:
    bag = _bag(world)

    def ready(r: dict[str, Any]) -> bool:
        row = x.variant_row(r, "control")
        return x.num(row, "exposed") == 2 and x.num(row, "converted") == 1

    report = x.poll(
        x.READOUT, lambda: x.readout_body(bag["flag"]), bag["admin"], ready, "readout"
    )
    row = x.variant_row(report, "control")
    assert abs(float(row.get("rate", 0)) - 0.5) < 1e-9, row
    assert float(row.get("ciLow", 0)) < 0.5 < float(row.get("ciHigh", 0)), row
    assert int(report.get("windowDays", 0)) == 7, report
    assert [r.get("variant") for r in report["variants"]] == ["control"], report


@when('one anonymous visitor is exposed to variant "a" of a fresh flag and then to variant "b"')
def first_then_second(world: World) -> None:
    bag = _bag(world)
    bag["flag"] = x.unique("flag")
    anon = x.unique("anon")
    x.post_beacons([x.exposure(bag["flag"], "a", anon)])
    time.sleep(1.5)
    x.post_beacons([x.exposure(bag["flag"], "b", anon)])


@then('the readout of that flag has variant "a" with exposed 1 and no row for variant "b"')
def only_first(world: World) -> None:
    bag = _bag(world)
    x.poll(
        x.READOUT,
        lambda: x.readout_body(bag["flag"]),
        bag["admin"],
        lambda r: x.num(x.variant_row(r, "a"), "exposed") >= 1,
        "readout",
    )
    time.sleep(x.HOLD_S)
    report = x.ok_json(x.call(x.READOUT, x.readout_body(bag["flag"]), bag["admin"]))
    assert x.num(x.variant_row(report, "a"), "exposed") == 1, report
    assert x.variant_row(report, "b") == {}, report


@when("the admin reads the readout of a flag that was never exposed")
def unknown_flag(world: World) -> None:
    bag = _bag(world)
    bag["resp"] = x.call(x.READOUT, x.readout_body(x.unique("never")), bag["admin"])


@then("the readout response succeeds with 0 variants and window_days 7")
def readout_empty(world: World) -> None:
    body = x.ok_json(_bag(world)["resp"])
    assert body.get("variants", []) == [], body
    assert int(body.get("windowDays", 0)) == 7, body


@when(
    "visitors post 5 shown, 2 accepted, 1 edited and 1 dismissed assistant events under one fresh "
    "model version and 3 shown chat_copilot events under another"
)
def post_ai(world: World) -> None:
    bag = _bag(world)
    bag["m1"], bag["m2"] = x.unique("mock:v1|hybrid"), x.unique("rules-v1")
    events = [("shown", 5), ("accepted", 2), ("edited", 1), ("dismissed", 1)]
    batch = [x.ai_event("assistant", a, bag["m1"]) for a, n in events for _ in range(n)]
    batch += [x.ai_event("chat_copilot", "shown", bag["m2"]) for _ in range(3)]
    x.post_beacons(batch)


@then(
    "the AI surface report has exactly the assistant row 5, 2, 1, 1 and the chat_copilot row "
    "3, 0, 0, 0 for those model versions"
)
def ai_counts(world: World) -> None:
    bag = _bag(world)

    def ready(r: dict[str, Any]) -> bool:
        return (
            x.num(x.ai_row(r, "assistant", bag["m1"]), "shown") >= 5
            and x.num(x.ai_row(r, "chat_copilot", bag["m2"]), "shown") >= 3
        )

    x.poll(x.AI_SURFACE, x.ai_body, bag["admin"], ready, "ai surface")
    time.sleep(x.HOLD_S)
    report = x.ok_json(x.call(x.AI_SURFACE, x.ai_body(), bag["admin"]))
    a = x.ai_row(report, "assistant", bag["m1"])
    c = x.ai_row(report, "chat_copilot", bag["m2"])
    got_a = tuple(x.num(a, k) for k in ("shown", "accepted", "edited", "dismissed"))
    got_c = tuple(x.num(c, k) for k in ("shown", "accepted", "edited", "dismissed"))
    assert got_a == (5, 2, 1, 1), a
    assert got_c == (3, 0, 0, 0), c
    mine = [r for r in report.get("rows", []) if r.get("modelVersion") in (bag["m1"], bag["m2"])]
    assert len(mine) == 2, mine


@when("the admin reads the AI surface report over a window two years ago lasting one hour")
def empty_ai(world: World) -> None:
    bag = _bag(world)
    start = x.now() - timedelta(days=730)
    body = {"from": x.iso(start), "to": x.iso(start + timedelta(hours=1))}
    bag["resp"] = x.call(x.AI_SURFACE, body, bag["admin"])


@then("the AI surface response succeeds with 0 rows")
def ai_empty(world: World) -> None:
    body = x.ok_json(_bag(world)["resp"])
    assert body.get("rows", []) == [], body


def _procedures() -> list[tuple[str, str, Any]]:
    return [
        (x.RETENTION, "cohorts", lambda: x.retention_body(x.monday(x.now()), 4)),
        (x.READOUT, "windowDays", lambda: x.readout_body(x.unique("access"))),
        (x.AI_SURFACE, "rows", x.ai_body),
    ]


@when("the admin calls the three report procedures through the gateway")
def admin_calls_all(world: World) -> None:
    bag = _bag(world)
    bag["resps"] = {p: x.call(p, make(), bag["admin"]) for p, _, make in _procedures()}


@then("each procedure answers HTTP 200 with its response field")
def each_200(world: World) -> None:
    for path, field, _ in _procedures():
        resp = _bag(world)["resps"][path]
        assert resp.status_code == 200, f"{path}: {resp.status_code} {resp.text[:300]}"
        body = resp.json()
        if field == "windowDays":
            assert int(body.get(field, 0)) == 7, (path, body)
        else:
            assert isinstance(body.get(field, []), list), (path, body)


@when("the buyer, then the seller, call each of the three report procedures through the gateway")
def non_admins_call(world: World) -> None:
    bag = _bag(world)
    bag["denied"] = {
        (who, p): x.call(p, make(), bag[who])
        for who in ("buyer", "seller")
        for p, _, make in _procedures()
    }


@then("every one of those calls answers HTTP 403")
def all_403(world: World) -> None:
    denied = _bag(world)["denied"]
    assert len(denied) == 6, list(denied)
    for key, resp in denied.items():
        assert resp.status_code == 403, f"{key}: {resp.status_code} {resp.text[:300]}"


@when("the admin calls each report 30 times over its widest window")
def latency_calls(world: World) -> None:
    bag = _bag(world)
    wide = [
        (x.RETENTION, lambda: x.retention_body(x.monday(x.now()) - timedelta(weeks=26), 26)),
        (x.READOUT, lambda: x.readout_body(x.unique("lat"), days=365)),
        (x.AI_SURFACE, lambda: x.ai_body(days=365)),
    ]
    bag["latency"] = {}
    for path, make in wide:
        samples = []
        for _ in range(CALLS):
            started = time.monotonic()
            resp = x.call(path, make(), bag["admin"])
            samples.append(time.monotonic() - started)
            assert resp.status_code == 200, f"{path}: {resp.status_code} {resp.text[:300]}"
        bag["latency"][path] = sorted(samples)


@then("the p95 latency of each report is at most 2 seconds")
def latency_p95(world: World) -> None:
    for path, samples in _bag(world)["latency"].items():
        p95 = samples[int(0.95 * len(samples)) - 1]
        assert p95 <= P95_BUDGET_S, f"{path}: p95 {p95:.2f}s over {len(samples)} calls"


@when('a visitor posts an experiment_exposure beacon with a fresh flag and variant "treatment"')
def post_exposure(world: World) -> None:
    bag = _bag(world)
    bag["flag"] = x.unique("flag")
    beacon = x.exposure(bag["flag"], "treatment", x.unique("anon"))
    bag["session"] = beacon["sessionId"]
    x.post_beacons([beacon])


@then(
    'the warehouse holds one row with event_type "experiment_exposure" and exactly those two properties'
)
def exposure_stored(world: World) -> None:
    bag = _bag(world)
    rows = tii.warehouse_rows(
        "SELECT event_type, properties FROM tracking_events WHERE session_id = ?",
        [bag["session"]],
        until=lambda r: len(r) >= 1,
        settle_s=3.0,
    )
    assert len(rows) == 1, rows
    event_type, props = rows[0]
    assert event_type == "experiment_exposure", rows
    parsed = json.loads(props) if isinstance(props, str) else dict(props)
    assert parsed == {"flag": bag["flag"], "variant": "treatment"}, parsed
