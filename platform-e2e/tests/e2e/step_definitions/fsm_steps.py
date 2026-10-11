"""Steps for featurestore-materialization (area fsm-e2e).

Black box: tracking beacons through the gateway edge (`POST /api/track`), favourites through the
gateway RPCs, the job as the real image on the stack network (fsm_job_flow). The analytics export
is observed on the analytics volume, the online features in the stack Redis (DB 2), the offline
output in a per-scenario temp dir. No scenario touches a container of the stack.

One batch of tracked activity is shared by the whole module (see `_shared`): the first scenario
that runs creates it and waits for the export cycle (PARQUET_EXPORT_INTERVAL_SECONDS, 300 s locally);
later scenarios find the export already holding it. Override the wait with FSM_EXPORT_WAIT_S.

Feature names inside the stored values are the ones the registry is expected to declare; they are
centralised in the constants below because the spec names the quantities, not the keys.
"""

from __future__ import annotations

import json
import os
import re
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import fsm_job_flow as job
from tests.e2e.support import efe_support as e
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import Actor, OicWorld, register

EXPORT_WAIT_S = float(os.getenv("FSM_EXPORT_WAIT_S", "390"))
POLL_S = 10.0

# Feature names in the stored JSON (spec: quantities only; keys follow the registry naming).
USER_VIEWS = "views_7d"
USER_FAVOURITES = "favorites_current"
ITEM_VIEWS = "views_7d"
ITEM_FAVOURITES = "favorites_current"

ENTITIES = {"user_activity": "user_key", "item_popularity": "listing_id"}


def _iso(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _parse(ts: str) -> datetime:
    dt = datetime.fromisoformat(ts.replace("Z", "+00:00"))
    return dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)


@dataclass
class Shared:
    world: OicWorld
    buyer: Actor
    listing_id: str
    visitor_listing: str
    first_event_at: datetime
    exported: bool = False


_SHARED: Shared | None = None


def _post_view(listing: str, token: str | None = None, **extra) -> None:
    resp = tii.post_track([tii.view(f"e2e-fsm-{tii.run_id()}", listingId=listing, **extra)], token)
    assert resp.status_code == 202 and resp.json().get("accepted") == 1, (
        resp.status_code,
        resp.text[:300],
    )


def _shared() -> Shared:
    """Create the shared activity once: a visitor view, and a new buyer's 3 views + 1 favourite."""
    global _SHARED
    if _SHARED is None:
        w = OicWorld()
        e.seller_with_listings(w, 1)
        buyer = register(w, "b1", "buyer")
        lid = e.listing_id(w, "L1")
        visitor_listing = f"e2e-fsm-{uuid.uuid4().hex[:10]}"
        _post_view(visitor_listing, anonymousId=f"e2e-anon-{uuid.uuid4()}")
        first = datetime.now(timezone.utc)
        for _ in range(3):
            _post_view(lid, token=buyer.token)
        e.favorite(w, buyer, "L1")
        _SHARED = Shared(w, buyer, lid, visitor_listing, first)
    return _SHARED


def _wait_export(s: Shared) -> dict:
    """Poll the analytics volume until the export holds the shared activity."""
    if s.exported:
        return job.inspect("probe", **_plan(s))
    deadline = time.monotonic() + EXPORT_WAIT_S
    last: dict = {}
    while time.monotonic() < deadline:
        last = job.inspect("probe", **_plan(s))
        resolved_ok = any(k for k in last["resolved"].get(s.visitor_listing, {}))
        buyer_ok = last["users"].get(s.buyer.user_id, 0) >= 3
        fact_ok = last["facts"].get(f"{s.buyer.user_id}|{s.listing_id}", 0) >= 1
        if resolved_ok and buyer_ok and fact_ok:
            s.exported = True
            return last
        time.sleep(POLL_S)
    raise AssertionError(
        f"the analytics export did not hold the activity in {EXPORT_WAIT_S}s: {last}"
    )


def _plan(s: Shared) -> dict:
    return {
        "listings": [s.visitor_listing, s.listing_id],
        "users": [s.buyer.user_id],
        "facts": [[s.buyer.user_id, s.listing_id]],
    }


@dataclass
class Ctx:
    offline: object = None
    results: list = field(default_factory=list)
    as_ofs: list = field(default_factory=list)
    data: dict = field(default_factory=dict)


@pytest.fixture
def fsm():
    c = Ctx(offline=job.new_offline_dir())
    yield c
    job.drop_offline_dir(c.offline)


def _materialize(c: Ctx, as_of: str = "") -> job.JobResult:
    r = job.run_job("materialize", c.offline, as_of=as_of)
    c.results.append(r)
    c.as_ofs.append(as_of)
    assert r.exit_code == 0, f"materialize exited {r.exit_code}:\n{r.output[-3000:]}"
    return r


def _online(key: str) -> str | None:
    cli = job.Online()
    try:
        return cli.get(key)
    finally:
        cli.close()


def _features(raw: str | None, key: str) -> dict:
    assert raw is not None, f"{key} is not in Redis DB {job.redis_db()}"
    doc = json.loads(raw)
    return doc.get("features", doc) if isinstance(doc, dict) else doc


# ── scenario 1 ───────────────────────────────────────────────────────────
@when("a visitor posts a view of a unique listing and the next export cycle completes")
def visitor_view_exported(fsm):
    s = _shared()
    fsm.data["probe"] = _wait_export(s)
    fsm.data["shared"] = s


@then(
    "tracking_events_resolved.parquet on the analytics volume contains that listing with a user_key"
)
def resolved_has_listing(fsm):
    s = fsm.data["shared"]
    keys = fsm.data["probe"]["resolved"].get(s.visitor_listing, {})
    assert keys and all(k for k in keys), f"no resolved row with a user_key: {fsm.data['probe']}"


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse('a seller with a published listing "{name}"'))
def seller_listing(fsm, name):
    _shared()


@given(
    parsers.parse(
        'a new buyer "{name}" has viewed "{listing}" three times and favourited it, '
        "and the export cycle has completed"
    )
)
@given(
    parsers.parse('a new buyer "{name}" has viewed a listing and the export cycle has completed')
)
def buyer_exported(fsm, name, listing="L1"):
    s = _shared()
    _wait_export(s)
    fsm.data["shared"] = s


# ── scenario 2 ───────────────────────────────────────────────────────────
@when(
    parsers.parse(
        'a new buyer "{name}" views "{listing}" three times and favourites it, the export cycle '
        "completes, and the materialisation job runs"
    )
)
def buyer_activity_job(fsm, name, listing):
    s = _shared()
    _wait_export(s)
    fsm.data["shared"] = s
    _materialize(fsm)


@then(
    parsers.parse(
        'the online features of "{name}" under user_activity@v2 have 3 views in the last 7 days '
        "and 1 current favourite"
    )
)
def user_features(fsm, name):
    uid = fsm.data["shared"].buyer.user_id
    key = f"fs:user_activity:v2:{uid}"
    f = _features(_online(key), key)
    assert f[USER_VIEWS] == 3, f
    assert f[USER_FAVOURITES] == 1, f


@then(
    parsers.parse(
        'the item_popularity@v1 features of "{listing}" have at least 3 views and at least 1 '
        "current favourite"
    )
)
def item_features(fsm, listing):
    key = f"fs:item_popularity:v1:{fsm.data['shared'].listing_id}"
    f = _features(_online(key), key)
    assert f[ITEM_VIEWS] >= 3, f
    assert f[ITEM_FAVOURITES] >= 1, f


# ── scenario 3 ───────────────────────────────────────────────────────────
@when(parsers.parse('the job runs with AS_OF set to a time just before "{name}"\'s first event'))
def job_before_first_event(fsm, name):
    s = fsm.data["shared"]
    # 5 s of margin for clock skew between the host and the stack; b1 has no event before it.
    as_of = _iso(s.first_event_at - timedelta(seconds=5))
    fsm.data["as_of"] = as_of
    _materialize(fsm, as_of)


@then(parsers.parse('"{name}" has no user_activity@v2 row in the offline snapshot of that run'))
def no_row(fsm, name):
    s = fsm.data["shared"]
    snap = job.inspect("snapshot", fsm.offline, entities=ENTITIES)
    runs = snap["snapshots"].get("user_activity", [])
    assert runs, f"no user_activity snapshot was written: {snap}"
    assert [m["as_of"] for m in snap["manifests"]], snap["manifests"]
    assert _parse(snap["manifests"][0]["as_of"]) == _parse(fsm.data["as_of"]), snap["manifests"]
    for run in runs:
        assert run["rows"] == 0 or "user_key" in run["columns"], run
        assert s.buyer.user_id not in run["ids"], f"{s.buyer.user_id} is in {run['path']}"


# ── scenario 4 ───────────────────────────────────────────────────────────
@when("the materialisation job runs twice with different AS_OF values")
def job_twice(fsm):
    _materialize(fsm, _iso(datetime.now(timezone.utc)))
    _materialize(fsm, "")  # now: a later second, since a container run takes seconds


@then(
    "two snapshots exist for each view, and each manifest names its as_of, a positive row count "
    "and a 64-hex definition hash"
)
def two_snapshots(fsm):
    snap = job.inspect("snapshot", fsm.offline, entities=ENTITIES)
    manifests = snap["manifests"]
    assert len(manifests) == 2, manifests
    assert len({_parse(m["as_of"]) for m in manifests}) == 2, [m["as_of"] for m in manifests]
    for view in ENTITIES:
        assert len(snap["snapshots"].get(view, [])) == 2, (view, snap["snapshots"])
    for m in manifests:
        assert {v["name"] for v in m["views"]} >= set(ENTITIES), m
        for v in m["views"]:
            assert v["rows"] > 0, (m["as_of"], v)
            assert re.fullmatch(r"[0-9a-f]{64}", v["definition_sha256"]), v


# ── scenario 5 ───────────────────────────────────────────────────────────
@when("the materialisation job finishes")
def job_finishes(fsm):
    as_of = _iso(datetime.now(timezone.utc))
    fsm.data["as_of"] = as_of
    _materialize(fsm, as_of)


@then(
    'fs:user_activity:current is "2" and fs:user_activity:meta carries the run\'s as_of and an '
    "input watermark no later than it"
)
def online_meta(fsm):
    assert _online("fs:user_activity:current") == "2"
    meta = json.loads(_online("fs:user_activity:meta") or "null")
    assert meta, "fs:user_activity:meta is missing"
    as_of = _parse(fsm.data["as_of"])
    assert _parse(meta["as_of"]) == as_of, meta
    assert meta.get("materialized_at"), meta
    assert meta.get("input_watermark"), meta
    assert _parse(meta["input_watermark"]) <= as_of, meta


# ── scenario 6 ───────────────────────────────────────────────────────────
@when(
    parsers.parse(
        'after a run, "{name}"\'s user_activity@v2 online value is overwritten with a different '
        "view count, and python -m featurestore parity runs"
    )
)
def tamper_and_parity(fsm, name):
    s = fsm.data["shared"]
    _materialize(fsm, _iso(datetime.now(timezone.utc)))
    key = f"fs:user_activity:v2:{s.buyer.user_id}"
    raw = _online(key)
    doc = json.loads(raw or "null")
    assert doc, f"{key} is not online after the run"
    f = doc.get("features", doc)
    f[USER_VIEWS] = f[USER_VIEWS] + 100
    cli = job.Online()
    try:
        cli.set_keepttl(key, json.dumps(doc))
    finally:
        cli.close()
    fsm.data["parity"] = job.run_job("parity", fsm.offline)


@then(parsers.parse('the command exits non-zero and its output names "{name}"'))
def parity_fails(fsm, name):
    r = fsm.data["parity"]
    uid = fsm.data["shared"].buyer.user_id
    assert r.exit_code != 0, f"parity passed on a tampered value:\n{r.output[-2000:]}"
    assert uid in r.output, f"the output does not name {uid}:\n{r.output[-2000:]}"
