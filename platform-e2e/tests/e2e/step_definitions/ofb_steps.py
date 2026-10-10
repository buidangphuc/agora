"""Steps for order-facts-buyer (area ofb).

Real stack: a buyer pays a two-line order through the gateway; the order reaches `order_facts` via
Kafka and is exported to `order_facts.parquet` on the analytics volume (read through the job image).
The job runs as the real image on the stack network and writes Redis DB 3. The window, count and
missing-buyer scenarios feed that same image synthetic order lines (ofb_flow.make_inputs).

One paid order is shared by the two real-stack scenarios (see `_shared`); the first scenario that
needs it creates it and waits for the export (PARQUET_EXPORT_INTERVAL_SECONDS, 300 s locally).
Override the wait with OFB_EXPORT_WAIT_S.
"""

from __future__ import annotations

import json
import os
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest
from pytest_bdd import then, when

from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.flows import ofb_flow as ofb
from tests.e2e.support import efe_support as e
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support.oic_order_support import Actor, OicWorld, register

EXPORT_WAIT_S = float(os.getenv("OFB_EXPORT_WAIT_S", "390"))
POLL_S = 10.0
FEATURE = "paid_orders_30d"


def _iso(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _naive(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S")


@dataclass
class Shared:
    buyer: Actor
    order_id: str
    listings: list[str]
    rows: list[dict] = field(default_factory=list)
    exported: bool = False


_SHARED: Shared | None = None


def _shared() -> Shared:
    """A new buyer pays one order of two listings (one seller), once per module."""
    global _SHARED
    if _SHARED is None:
        w = OicWorld()
        e.seller_with_listings(w, 2)
        buyer = register(w, "b1", "buyer")
        oic.post(w, buyer, oic.CART, "ClearCart", {})
        oic.ensure_address(w, buyer)
        oic.add_to_cart(w, buyer, "L1", 1)
        oic.add_to_cart(w, buyer, "L2", 1)
        orders = oic.ok(oic.checkout(w, buyer)).get("orders", [])
        assert len(orders) == 1, f"one seller, one checkout -> one order, got {orders}"
        order_id = orders[0]["id"]
        oic.pay_order(w, buyer, order_id)
        oic.wait_status(w, buyer, order_id, oic.PAID)
        _SHARED = Shared(buyer, order_id, [e.listing_id(w, "L1"), e.listing_id(w, "L2")])
    return _SHARED


def _wait_export(s: Shared) -> Shared:
    """Poll the analytics volume until order_facts.parquet holds both lines of the order."""
    if s.exported:
        return s
    deadline = time.monotonic() + EXPORT_WAIT_S
    last: dict = {}
    while time.monotonic() < deadline:
        last = ofb.inspect_orders([s.order_id])
        if len(last["rows"]) >= 2:
            s.rows, s.exported = last["rows"], True
            return s
        time.sleep(POLL_S)
    raise AssertionError(
        f"order_facts.parquet did not hold order {s.order_id} in {EXPORT_WAIT_S}s: {last}"
    )


@dataclass
class Ctx:
    offline: Path
    inputs: Path | None = None
    as_of: str = ""
    user: str = ""
    result: fsm.JobResult | None = None
    data: dict = field(default_factory=dict)


@pytest.fixture
def ofbx():
    c = Ctx(offline=fsm.new_offline_dir())
    yield c
    fsm.drop_offline_dir(c.offline)
    if c.inputs is not None:
        ofb.drop(c.inputs)


def _online_features(user: str) -> dict | None:
    key = f"fs:user_activity:v2:{user}"
    cli = ofb.online()
    try:
        raw = cli.get(key)
    finally:
        cli.close()
    if raw is None:
        return None
    doc = json.loads(raw)
    return doc.get("features", doc)


def _materialize(c: Ctx, *, synthetic: bool) -> None:
    r = ofb.run_job("materialize", c.offline, as_of=c.as_of, inputs=c.inputs if synthetic else None)
    c.result = r
    assert r.exit_code == 0, f"materialize exited {r.exit_code}:\n{r.output[-3000:]}"


def _synthetic(c: Ctx, lines_for_user) -> None:
    """Run the job over synthetic order lines; lines_for_user(user, as_of) builds them."""
    now = datetime.now(timezone.utc).replace(microsecond=0)
    c.as_of = _iso(now)
    c.user = f"ofb-{uuid.uuid4().hex[:12]}"
    c.inputs = ofb.make_inputs(lines_for_user(c.user, now))
    _materialize(c, synthetic=True)


def _line(order: str, buyer: str | None, at: datetime, status: str = "PAID") -> dict:
    return {"order": order, "buyer": buyer, "at": _naive(at), "status": status}


# ── order-facts: A paid order's lines carry the buyer ────────────────────
@when(
    "a buyer pays an order with two lines of different listings and the next export cycle completes"
)
def ofb_buyer_pays_two_lines(ofbx):
    ofbx.data["shared"] = _wait_export(_shared())


@then(
    "order_facts.parquet on the analytics volume has two rows for that order, each with buyer_id "
    "equal to that buyer's user id"
)
def ofb_rows_carry_buyer(ofbx):
    s: Shared = ofbx.data["shared"]
    assert s.buyer.user_id, "the registered buyer has no user id"
    assert len(s.rows) == 2, s.rows
    assert {r["listing_id"] for r in s.rows} == set(s.listings), s.rows
    assert [r["buyer_id"] for r in s.rows] == [s.buyer.user_id] * 2, s.rows


# ── feature-materialization: A paid order becomes a user feature ─────────
@when("a new buyer pays one order, the export cycle completes, and the materialisation job runs")
def ofb_paid_order_job(ofbx):
    ofbx.data["shared"] = _wait_export(_shared())
    ofbx.as_of = ""
    _materialize(ofbx, synthetic=False)


@then(
    "the online paid_orders_30d of that buyer under user_activity@v2 is 1, and that buyer has a row "
    "although they posted no tracking event"
)
def ofb_online_paid_orders(ofbx):
    uid = ofbx.data["shared"].buyer.user_id
    f = _online_features(uid)
    assert f is not None, f"fs:user_activity:v2:{uid} is not in Redis DB {ofb.REDIS_DB}"
    assert f[FEATURE] == 1, f
    assert f["views_7d"] == 0, f"the buyer posted no tracking event but has views: {f}"


# ── Only orders in the 30 days before AS_OF count ────────────────────────
@when(
    "a user has paid orders 31 days, 29 days and 1 day before AS_OF and one after AS_OF, and the "
    "job runs"
)
def ofb_window(ofbx):
    _synthetic(
        ofbx,
        lambda u, now: [
            _line("o31", u, now - timedelta(days=31)),
            _line("o29", u, now - timedelta(days=29)),
            _line("o1", u, now - timedelta(days=1)),
            _line("ofuture", u, now + timedelta(hours=1)),
        ],
    )


@when(
    "a user has one paid order with three lines and one paid order with one line within the window, "
    "and the job runs"
)
def ofb_multi_line(ofbx):
    _synthetic(
        ofbx,
        lambda u, now: [
            _line("big", u, now - timedelta(days=2)),
            _line("big", u, now - timedelta(days=2)),
            _line("big", u, now - timedelta(days=2)),
            _line("small", u, now - timedelta(days=3)),
        ],
    )


@then("the user's paid_orders_30d is 2")
def ofb_user_count_two(ofbx):
    f = _online_features(ofbx.user)
    assert f is not None, f"fs:user_activity:v2:{ofbx.user} is not online"
    assert f[FEATURE] == 2, f


# ── An order without a buyer counts for nobody ───────────────────────────
@when("the order facts hold a paid order line with no buyer_id, and the job runs")
def ofb_no_buyer(ofbx):
    _synthetic(
        ofbx,
        lambda u, now: [
            _line("anon-null", None, now - timedelta(days=2)),
            _line("anon-empty", "", now - timedelta(days=2)),
            _line("control", u, now - timedelta(days=1)),
        ],
    )


@then("no user_activity@v2 row has a paid_orders_30d that includes that order")
def ofb_no_row_for_unattributed(ofbx):
    snap = fsm.inspect("snapshot", ofbx.offline, entities={"user_activity": "user_key"})
    runs = snap["snapshots"].get("user_activity", [])
    assert runs, f"no user_activity snapshot was written: {snap}"
    for run in runs:
        assert run["ids"] == [ofbx.user], f"only the control buyer may have a row: {run}"
    f = _online_features(ofbx.user)
    assert f is not None and f[FEATURE] == 1, f"control buyer must count only its own order: {f}"


# ── A tampered order count fails the parity check ────────────────────────
@when(
    "after a run, one buyer's user_activity@v2 online paid_orders_30d is overwritten with a "
    "different value, and python -m featurestore parity runs"
)
def ofb_tamper(ofbx):
    _synthetic(ofbx, lambda u, now: [_line("t1", u, now - timedelta(days=1))])
    key = f"fs:user_activity:v2:{ofbx.user}"
    cli = ofb.online()
    try:
        doc = json.loads(cli.get(key) or "null")
        assert doc, f"{key} is not online after the run"
        f = doc.get("features", doc)
        assert f[FEATURE] == 1, f
        f[FEATURE] = 7
        cli.set_keepttl(key, json.dumps(doc))
    finally:
        cli.close()
    ofbx.result = ofb.run_job("parity", ofbx.offline, as_of=ofbx.as_of, inputs=ofbx.inputs)


@then("the command exits non-zero and its output names that buyer and paid_orders_30d")
def ofb_parity_names_buyer(ofbx):
    r = ofbx.result
    assert r is not None and r.exit_code != 0, f"parity must fail, got {r}"
    assert ofbx.user in r.output and FEATURE in r.output, r.output[-1500:]
