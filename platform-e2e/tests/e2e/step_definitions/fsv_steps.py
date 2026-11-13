"""Steps for analytics-forecast-serving (forecast_serving.feature).

Black box: keys `fc:v1:seller:<seller>:<listing>` are written to the stack Redis DB 3 with the layout
the nightly job documents; GetDemandForecast is called through the gateway and the response, never the
service internals, is asserted. Every key a scenario seeds is deleted afterwards.
"""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta, timezone

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import fnj_job_flow as job
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support.oic_order_support import Actor, OicWorld

ANALYTICS = "/platform.analytics.v1.AnalyticsQueryService"
START = date(2026, 11, 13)


@dataclass
class Ctx:
    w: OicWorld = field(default_factory=OicWorld)
    seller: Actor | None = None
    other: Actor | None = None
    listing: str = ""
    points: list = field(default_factory=list)
    resp: httpx.Response | None = None
    resps: list = field(default_factory=list)
    latencies: list = field(default_factory=list)
    keys: set = field(default_factory=set)


@pytest.fixture
def fsv():
    c = Ctx()
    yield c
    if c.keys:
        r = job.Redis()
        try:
            r.delete(*c.keys)
        finally:
            r.close()


def _key(c: Ctx) -> str:
    assert c.seller is not None
    return f"fc:v1:seller:{c.seller.user_id}:{c.listing}"


def _put(c: Ctx, raw: str) -> None:
    key = _key(c)
    r = job.Redis()
    try:
        r.call("SET", key, raw, "EX", "900")
    finally:
        r.close()
    c.keys.add(key)


def _stored_points() -> list[dict]:
    pts = []
    for i in range(14):
        p10 = float(i % 3)
        pts.append(
            {
                "date": (START + timedelta(days=i)).isoformat(),
                "p10": p10,
                "p50": p10 + 2.0 + (i % 2),
                "p90": p10 + 4.0 + (i % 2) * 2,
            }
        )
    return pts


def _payload(c: Ctx, model: str, generated_at: str, points: list[dict]) -> str:
    assert c.seller is not None
    return json.dumps(
        {
            "seller_id": c.seller.user_id,
            "listing_id": c.listing,
            "model_version": model,
            "generated_at": generated_at,
            "is_cold_start": False,
            "daily_forecasts": points,
        }
    )


def _call(c: Ctx, actor: Actor | None, seller_id: str, listing: str, horizon: int = 14, client=None):
    body = {"seller_id": seller_id, "listing_id": listing, "horizon_days": horizon}
    if client is not None:
        hdrs = {"Content-Type": "application/json"}
        if actor:
            hdrs["Authorization"] = f"bearer {actor.token}"
        return client.post(f"{c.w.gateway}{ANALYTICS}/GetDemandForecast", json=body, headers=hdrs)
    return oic.post(c.w, actor, ANALYTICS, "GetDemandForecast", body)


def _ok_json(c: Ctx) -> dict:
    assert c.resp is not None
    assert c.resp.status_code == 200, f"HTTP {c.resp.status_code}: {c.resp.text[:400]}"
    return c.resp.json()


def _assert_points(got: list[dict], want: list[dict]) -> None:
    assert len(got) == len(want), f"{len(got)} points, wanted {len(want)}: {got}"
    for g, s in zip(got, want):
        assert g.get("date") == s["date"], f"date {g} != {s}"
        for k in ("p10", "p50", "p90"):
            assert float(g.get(k, 0)) == s[k], f"{k} {g} != {s}"


def seed_forecast_key(seller_id: str, listing_id: str) -> str:
    """Write a 14-day key for the pair (used by journeys that need a stored forecast); returns the key."""
    key = f"fc:v1:seller:{seller_id}:{listing_id}"
    raw = json.dumps(
        {
            "seller_id": seller_id,
            "listing_id": listing_id,
            "model_version": "seasonal_naive_v1",
            "generated_at": "2026-11-13T02:45:00Z",
            "is_cold_start": False,
            "daily_forecasts": _stored_points(),
        }
    )
    r = job.Redis()
    try:
        r.call("SET", key, raw, "EX", "900")
    finally:
        r.close()
    return key


def drop_key(key: str) -> None:
    r = job.Redis()
    try:
        r.delete(key)
    finally:
        r.close()


@given(
    parsers.parse(
        'a seller whose listing has a forecast key of 14 days with model_version "{model}" generated at {at}'
    )
)
def seeded_key(fsv: Ctx, model: str, at: str) -> None:
    fsv.seller = oic.register(fsv.w, "seller", "seller")
    fsv.listing = f"fsv-l-{uuid.uuid4().hex[:10]}"
    fsv.points = _stored_points()
    _put(fsv, _payload(fsv, model, at, fsv.points))


@given(parsers.parse("a seller whose listing key holds {kind}"))
def seeded_malformed(fsv: Ctx, kind: str) -> None:
    fsv.seller = oic.register(fsv.w, "seller", "seller")
    fsv.listing = f"fsv-l-{uuid.uuid4().hex[:10]}"
    if kind == "invalid JSON":
        _put(fsv, "{not json")
    else:
        pts = _stored_points()
        pts[3]["p10"] = pts[3]["p50"] + 1.0
        _put(fsv, _payload(fsv, "seasonal_naive_v1", "2026-11-13T02:45:00Z", pts))


@given("a seller with a paid order line for a listing that has no forecast key")
def paid_line_no_key(fsv: Ctx) -> None:
    from tests.e2e.support import efe_support as e

    w = fsv.w
    fsv.seller = e.seller_with_listings(w, 1)
    fsv.listing = e.listing_id(w, "L1")
    buyer = oic.register(w, "buyer", "buyer")
    oic.post(w, buyer, oic.CART, "ClearCart", {})
    oic.ensure_address(w, buyer)
    oic.add_to_cart(w, buyer, "L1", 1)
    order_id = oic.order_ids(oic.checkout(w, buyer))[0]
    oic.pay_order(w, buyer, order_id)
    oic.wait_status(w, buyer, order_id, oic.PAID)
    deadline = time.monotonic() + 90
    seen = ""
    while time.monotonic() < deadline:
        resp = oic.post(w, fsv.seller, ANALYTICS, "GetRevenueBreakdown", {"seller_id": fsv.seller.user_id})
        seen = resp.text
        if resp.status_code == 200 and fsv.listing in seen or "topSkus" in seen:
            return
        time.sleep(3)
    raise AssertionError(f"order_facts never held the paid line of {fsv.listing}: {seen[:300]}")


@given("another seller exists")
def other_seller(fsv: Ctx) -> None:
    fsv.other = oic.register(fsv.w, "other", "seller")


@when("the seller calls GetDemandForecast for that listing")
def call_own(fsv: Ctx) -> None:
    fsv.resp = _call(fsv, fsv.seller, fsv.seller.user_id, fsv.listing)


@when(parsers.parse("the seller calls GetDemandForecast for that listing with horizon_days {h:d}"))
def call_horizon(fsv: Ctx, h: int) -> None:
    fsv.resp = _call(fsv, fsv.seller, fsv.seller.user_id, fsv.listing, horizon=h)


@when("the other seller calls GetDemandForecast for the first seller's listing")
def call_other(fsv: Ctx) -> None:
    fsv.resp = _call(fsv, fsv.other, fsv.seller.user_id, fsv.listing)


@when("an anonymous caller calls GetDemandForecast for the first seller's listing")
def call_anon(fsv: Ctx) -> None:
    fsv.resp = _call(fsv, None, fsv.seller.user_id, fsv.listing)


@when("an admin calls GetDemandForecast for the first seller's listing")
def call_admin(fsv: Ctx) -> None:
    admin = oic.login_admin(fsv.w)
    fsv.resp = _call(fsv, admin, fsv.seller.user_id, fsv.listing)


@when(parsers.parse("the seller calls GetDemandForecast for that listing {n:d} times in sequence"))
def call_many(fsv: Ctx, n: int) -> None:
    with httpx.Client(timeout=10) as client:
        _call(fsv, fsv.seller, fsv.seller.user_id, fsv.listing, client=client)
        for _ in range(n):
            t0 = time.perf_counter()
            resp = _call(fsv, fsv.seller, fsv.seller.user_id, fsv.listing, client=client)
            fsv.latencies.append((time.perf_counter() - t0) * 1000)
            assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:300]}"


@then(
    parsers.parse(
        'the response has the 14 stored daily forecasts, model_version "{model}", generated_at {at} and is_cold_start false'
    )
)
def served_as_written(fsv: Ctx, model: str, at: str) -> None:
    body = _ok_json(fsv)
    _assert_points(body.get("dailyForecasts", []), fsv.points)
    assert body.get("modelVersion") == model, body
    got = datetime.fromisoformat(body["generatedAt"].replace("Z", "+00:00"))
    assert got == datetime.fromisoformat(at.replace("Z", "+00:00")).astimezone(timezone.utc), body
    assert not body.get("isColdStart", False), body


@then(parsers.parse("the response has the first {n:d} stored daily forecasts"))
def first_n(fsv: Ctx, n: int) -> None:
    _assert_points(_ok_json(fsv).get("dailyForecasts", []), fsv.points[:n])


@then(parsers.parse("the response has the {n:d} stored daily forecasts"))
def all_n(fsv: Ctx, n: int) -> None:
    _assert_points(_ok_json(fsv).get("dailyForecasts", []), fsv.points[:n])


@then(
    "the response is a cold start with no daily forecasts, reorder point 0, safety stock 0 and no generated_at"
)
def cold_start(fsv: Ctx) -> None:
    body = _ok_json(fsv)
    assert body.get("isColdStart") is True, body
    assert not body.get("dailyForecasts"), body
    assert float(body.get("suggestedReorderPoint", 0)) == 0, body
    assert float(body.get("safetyStock", 0)) == 0, body
    assert not body.get("generatedAt"), body


@then(parsers.parse('the call fails with code "{code}" and carries no forecast numbers'))
def fails_with(fsv: Ctx, code: str) -> None:
    assert fsv.resp is not None
    assert oic.code_of(fsv.resp) == code, f"HTTP {fsv.resp.status_code}: {fsv.resp.text[:300]}"
    text = fsv.resp.text
    for field_name in ("dailyForecasts", "suggestedReorderPoint", "safetyStock", "modelVersion"):
        assert field_name not in text, text


@then(parsers.parse("the 95th percentile call latency is at most {ms:d} ms"))
def p95(fsv: Ctx, ms: int) -> None:
    ordered = sorted(fsv.latencies)
    p = ordered[int(len(ordered) * 0.95) - 1]
    assert p <= ms, f"p95 {p:.1f} ms over {len(ordered)} calls (max {ordered[-1]:.1f} ms)"
