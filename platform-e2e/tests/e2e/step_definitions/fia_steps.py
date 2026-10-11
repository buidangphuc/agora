"""Steps for featurestore-item-attributes (area fia).

Black box: a seller creates and edits listings and a buyer posts views through the gateway edge; the
listing export is read on the analytics volume and the job runs as the real image (fia_flow, which
reuses fsm_job_flow). The real-stack scenarios share one batch of activity (see `_shared`); the first
that runs creates it and waits for the export cycle (PARQUET_EXPORT_INTERVAL_SECONDS, 300 s locally;
override the wait with FIA_EXPORT_WAIT_S). The point-in-time, parity and input-guard scenarios feed the
same image synthetic inputs. Nothing touches a container of the stack.
"""

from __future__ import annotations

import json
import os
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta

import pytest
from pytest_bdd import parsers, then, when

from tests.e2e.flows import fia_flow as flow
from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import Actor, OicWorld, register

EXPORT_WAIT_S = float(os.getenv("FIA_EXPORT_WAIT_S", "390"))
POLL_S = 10.0
AS_OF = "2026-10-09T12:00:00Z"
AS_OF_NAIVE = datetime(2026, 10, 9, 12, 0, 0)


def _naive(dt: datetime) -> str:
    return dt.strftime("%Y-%m-%dT%H:%M:%S")


@dataclass
class Shared:
    seller: Actor
    buyer: Actor
    a: str  # listing of category A, 3 views by the buyer
    b: str  # listing of category B, 1 view
    c: str  # created, then edited before the first export
    cat_a: str
    cat_b: str
    price_a: int
    price_c_new: int
    exported: bool = False


_SHARED: Shared | None = None


def _create(w: OicWorld, seller: Actor, name: str, category: str, price: int) -> str:
    resp = oic.post(
        w,
        seller,
        oic.LISTING,
        "CreateListing",
        {
            "listing": {
                "title": f"[FIA] {name} {uuid.uuid4().hex[:6]}",
                "categoryId": category,
                "price": price,
                "stock": 5,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
                "description": "featurestore item attributes e2e",
            }
        },
    )
    listing = oic.ok(resp).get("listing") or {}
    assert listing.get("id"), resp.text
    return listing["id"]


def _edit_price(w: OicWorld, seller: Actor, listing_id: str, price: int) -> None:
    current = oic.ok(oic.post(w, None, oic.LISTING, "GetListing", {"id": listing_id})).get(
        "listing"
    )
    assert current, listing_id
    current["price"] = price
    oic.ok(oic.post(w, seller, oic.LISTING, "UpdateListing", {"listing": current}))


def _shared() -> Shared:
    global _SHARED
    if _SHARED is None:
        w = OicWorld()
        tag = uuid.uuid4().hex[:8]
        seller = register(w, "seller", "seller")
        buyer = register(w, "buyer", "buyer")
        cat_a, cat_b = f"cat-fia-{tag}-a", f"cat-fia-{tag}-b"
        a = _create(w, seller, "A", cat_a, 111_000)
        b = _create(w, seller, "B", cat_b, 222_000)
        c = _create(w, seller, "C", cat_a, 100_000)
        _edit_price(w, seller, c, 150_000)
        for lid, n in ((a, 3), (b, 1)):
            for _ in range(n):
                resp = tii.post_track([tii.view(f"e2e-fia-{tag}", listingId=lid)], buyer.token)
                assert resp.status_code == 202, resp.text[:300]
        _SHARED = Shared(seller, buyer, a, b, c, cat_a, cat_b, 111_000, 150_000)
    return _SHARED


def _wait_export(s: Shared) -> dict:
    """Poll the analytics volume until the export holds the listings and the buyer's four views."""
    deadline = time.monotonic() + (0 if s.exported else EXPORT_WAIT_S)
    last: dict = {}
    while True:
        last = flow.probe_export([s.a, s.b, s.c], [s.buyer.user_id])
        ok = (
            all(last["listings"].get(x) for x in (s.a, s.b, s.c))
            and last["users"].get(s.buyer.user_id, 0) >= 4
        )
        if ok:
            s.exported = True
            return last
        if time.monotonic() >= deadline:
            raise AssertionError(f"the export did not hold the batch in {EXPORT_WAIT_S}s: {last}")
        time.sleep(POLL_S)


@dataclass
class Ctx:
    offline: object = None
    data: dict = field(default_factory=dict)
    inputs: list = field(default_factory=list)


@pytest.fixture
def fia():
    c = Ctx(offline=fsm.new_offline_dir())
    yield c
    fsm.drop_offline_dir(c.offline)
    for p in c.inputs:
        flow.drop(p)


def _materialize(c: Ctx, **kw) -> fsm.JobResult:
    r = flow.run_job("materialize", c.offline, **kw)
    c.data["result"] = r
    return r


def _online(key: str) -> dict:
    cli = fsm.Online()
    try:
        raw = cli.get(key)
    finally:
        cli.close()
    assert raw is not None, f"{key} is not in Redis DB {fsm.redis_db()}"
    return json.loads(raw)


# ── the listing export ──────────────────────────────────────────────────────
@when("a seller creates a listing with a category and a price and the next export cycle completes")
def listing_exported(fia):
    s = _shared()
    fia.data["shared"], fia.data["probe"] = s, _wait_export(s)


@then(
    "listing_sellers.parquet on the analytics volume has a row for that listing with that "
    "category_id and price"
)
def export_row(fia):
    s = fia.data["shared"]
    rows = fia.data["probe"]["listings"][s.a]
    assert rows == [{"seller": s.seller.user_id, "category": s.cat_a, "price": s.price_a}], rows


@when(parsers.parse("the seller changes that listing's price and the next export cycle completes"))
def edited_exported(fia):
    s = _shared()  # listing C was created and edited before the first export
    fia.data["shared"], fia.data["probe"] = s, _wait_export(s)


@then("listing_sellers.parquet holds the new price for that listing and exactly one row for it")
def export_new_price(fia):
    s = fia.data["shared"]
    rows = fia.data["probe"]["listings"][s.c]
    assert [r["price"] for r in rows] == [s.price_c_new], rows


# ── materialised features on the real export ────────────────────────────────
@when(
    "a seller creates a listing with a category and a price, the export cycle completes, and the "
    "materialisation job runs"
)
def listing_features(fia):
    s = _shared()
    _wait_export(s)
    fia.data["shared"] = s
    r = _materialize(fia)
    assert r.exit_code == 0, f"materialize exited {r.exit_code}:\n{r.output[-3000:]}"


@then(
    "the online features of that listing under item_attributes@v1 carry that category and price "
    "and the seller"
)
def listing_online(fia):
    s = fia.data["shared"]
    assert _online(f"fs:item_attributes:v1:{s.a}") == {
        "seller_id": s.seller.user_id,
        "category_id": s.cat_a,
        "price": s.price_a,
    }


@when(
    "a buyer views a listing of category A three times and a listing of category B once, the export "
    "cycle completes, and the materialisation job runs"
)
def preferences_features(fia):
    listing_features(fia)


@then(
    parsers.parse(
        'the online preferred_categories of that buyer under user_preferences@v1 is "{expected}"'
    )
)
def preferences_online(fia, expected):
    s = fia.data["shared"]
    want = expected.replace("A", s.cat_a).replace("B", s.cat_b)
    got = _online(f"fs:user_preferences:v1:{s.buyer.user_id}")
    assert got == {"preferred_categories": want}, got


# ── synthetic inputs ────────────────────────────────────────────────────────
def _synthetic(fia, **kw):
    p = flow.make_inputs(**kw)
    fia.inputs.append(p)
    return p


@when(
    "the listing export holds one listing last changed before AS_OF and one changed after it, and the job runs"
)
def point_in_time(fia):
    tag = uuid.uuid4().hex[:8]
    fia.data["before"], fia.data["after"] = f"fia-before-{tag}", f"fia-after-{tag}"
    listings = [
        {
            "id": fia.data["before"],
            "category": "cat-x",
            "price": 10,
            "at": _naive(AS_OF_NAIVE - timedelta(hours=2)),
        },
        {
            "id": fia.data["after"],
            "category": "cat-x",
            "price": 10,
            "at": _naive(AS_OF_NAIVE + timedelta(hours=2)),
        },
    ]
    inputs = _synthetic(fia, listings=listings)
    r = _materialize(fia, as_of=AS_OF, inputs=inputs)
    assert r.exit_code == 0, r.output[-3000:]


@then("the item_attributes@v1 snapshot has a row for the first and none for the second")
def point_in_time_rows(fia):
    snap = flow.snapshot_rows(fia.offline, "item_attributes")
    ids = {row["listing_id"] for row in snap["rows"]}
    assert snap["found"] and ids == {fia.data["before"]}, snap


@when(
    "after a run, one listing's item_attributes@v1 online category is overwritten with a different "
    "value, and python -m featurestore parity runs"
)
def tamper(fia):
    lid = f"fia-tamper-{uuid.uuid4().hex[:8]}"
    fia.data["listing"] = lid
    at = _naive(AS_OF_NAIVE - timedelta(hours=1))
    inputs = _synthetic(fia, listings=[{"id": lid, "category": "cat-x", "price": 10, "at": at}])
    r = _materialize(fia, as_of=AS_OF, inputs=inputs)
    assert r.exit_code == 0, r.output[-3000:]
    key = f"fs:item_attributes:v1:{lid}"
    doc = _online(key)
    doc["category_id"] = "cat-tampered"
    cli = fsm.Online()
    try:
        cli.set_keepttl(key, json.dumps(doc))
    finally:
        cli.close()
    fia.data["parity"] = flow.run_job("parity", fia.offline)


@then("the command exits non-zero and its output names that listing and category_id")
def parity_fails(fia):
    r = fia.data["parity"]
    assert r.exit_code != 0, f"parity passed on a tampered value:\n{r.output[-2000:]}"
    assert fia.data["listing"] in r.output and "category_id" in r.output, r.output[-2000:]


@when("the job runs over inputs without listing_sellers.parquet")
def no_listing_export(fia):
    _materialize(fia, as_of=AS_OF, inputs=_synthetic(fia, listings=None))


@then(
    "it exits 0 with a warning naming the file, and the item_attributes@v1 and user_preferences@v1 "
    "snapshots have no rows"
)
def empty_views(fia):
    r = fia.data["result"]
    assert r.exit_code == 0 and "listing_sellers.parquet" in r.output, r.output[-2000:]
    for view in ("item_attributes", "user_preferences"):
        snap = flow.snapshot_rows(fia.offline, view)
        assert snap["found"] and snap["rows"] == [], (view, snap)


@when("the job runs over a listing_sellers.parquet that has no category_id column")
def old_exporter(fia):
    at = _naive(AS_OF_NAIVE - timedelta(hours=1))
    inputs = _synthetic(
        fia,
        listings=[{"id": "fia-old", "category": "cat-x", "price": 1, "at": at}],
        drop_columns=["category_id"],
    )
    _materialize(fia, as_of=AS_OF, inputs=inputs)


@then("it exits 2 and its output names category_id")
def exits_two(fia):
    r = fia.data["result"]
    assert r.exit_code == 2 and "category_id" in r.output, f"{r.exit_code}: {r.output[-2000:]}"
