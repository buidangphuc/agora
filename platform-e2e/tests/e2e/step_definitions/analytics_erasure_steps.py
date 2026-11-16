"""Steps for analytics/user_erasure.feature and analytics/user_erasure_stores.feature
(change analytics-user-erasure).

Real users are registered through the gateway and seeded through public RPCs and the tracking beacon;
`UserErased` is produced to the real `identity.events` topic. Observations are read-only copies of the
DuckDB warehouse, the Parquet exports (job image), the broker, Redis and Qdrant, and the gateway's
analytics RPCs. Nothing is mocked. Redis/Qdrant/team-ai users are synthetic ids (no RPC creates those
keys); every key and collection a scenario creates is removed at teardown.
"""

from __future__ import annotations

import re
import time
import uuid
from urllib.parse import quote

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.support import analytics_erasure_support as aue
from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import tii_support as tii

ANALYTICS_SVC = "/platform.analytics.v1.AnalyticsQueryService"
ENG = "/platform.engagement.v1.EngagementService"
TRACK_BATCH = 25
LISTING_FACTS = ("favorite_added", "favorite_removed", "seller_followed", "seller_unfollowed")


@pytest.fixture
def aue_world(oic):
    oic.data["aue"] = {"run": uuid.uuid4().hex[:8], "ids": {}, "cleanups": []}
    yield oic
    for fn in reversed(oic.data["aue"]["cleanups"]):
        try:
            fn()
        except Exception:  # noqa: BLE001 - teardown must not mask the scenario result
            pass


def _d(w) -> dict:
    return w.data["aue"]


def _cleanup(w, fn) -> None:
    _d(w)["cleanups"].append(fn)


def _uid(w, name: str) -> str:
    if name in w.actors:
        return w.actors[name].user_id
    ids = _d(w)["ids"]
    if name not in ids:
        ids[name] = f"aue{_d(w)['run']}{name}"
    return ids[name]


def _beacon(listing_id: str, marker: str, anon: str = "") -> dict:
    body = {
        "type": "view",
        "listingId": listing_id,
        "sessionId": f"e2e-aue-sess-{marker}",
        "path": f"/e2e/aue/{marker}",
    }
    if anon:
        body["anonymousId"] = anon
    return body


def _post_views(beacons: list[dict], token: str | None) -> None:
    for i in range(0, len(beacons), TRACK_BATCH):
        chunk = beacons[i : i + TRACK_BATCH]
        resp = tii.post_track(chunk, token=token)
        assert resp.status_code == 202, (resp.status_code, resp.text[:300])
        assert resp.json().get("accepted") == len(chunk), resp.text[:300]


def _listing(w, name: str) -> str:
    return w.listings[name]["id"]


def _admin(w) -> s.Actor:
    return w.actors.get("admin") or s.login_admin(w)


def _funnel(w, seller_id: str) -> dict:
    body = s.ok(s.post(w, _admin(w), ANALYTICS_SVC, "GetSellerFunnel", {"sellerId": seller_id}))
    return {k: int(body.get(k, 0)) for k in ("impressions", "views", "adds", "orders", "purchases")} | {
        "beginCheckouts": int(body.get("beginCheckouts", 0))
    }


def _gmv(order_ids: list[str]) -> tuple[int, int]:
    """(distinct orders, SUM(quantity * unit_price)) of the given orders, as GetPlatformOrderSummary defines GMV."""
    marks = ", ".join("?" for _ in order_ids)
    row = tii.warehouse_query(
        f"SELECT COUNT(DISTINCT order_id), COALESCE(SUM(quantity * unit_price), 0) FROM order_facts "
        f"WHERE order_id IN ({marks})",
        order_ids,
    )[0]
    return int(row[0]), int(row[1])


def _poll(fn, ok, what: str, timeout: float = aue.WITHIN_S):
    return aue.wait_for(fn, ok, what, timeout)


# ── seeding ──────────────────────────────────────────────────────────────
@given(parsers.parse('a seller "{name}" with listings "L1", "L2" and "L3"'))
@given(parsers.parse('a seller "{name}" with a listing "L1"'))
def seller_three(aue_world, name):
    seller = s.register(aue_world, name, "seller")
    for ln in ("L1", "L2", "L3"):
        s.create_listing(aue_world, ln, seller, 200)


@given(parsers.parse('a seller "{name}" with 4 listings "L1" to "L4"'))
def seller_four(aue_world, name):
    seller = s.register(aue_world, name, "seller")
    for i in range(1, 5):
        s.create_listing(aue_world, f"L{i}", seller, 200)


@given(parsers.parse('a second seller "{name}"'))
def second_seller(aue_world, name):
    s.register(aue_world, name, "seller")


@given(parsers.parse('a buyer "{name}"'))
def a_buyer(aue_world, name):
    s.register(aue_world, name, "buyer")


@given(parsers.parse('a user "{name}" that has no rows in any table'))
def user_without_rows(aue_world, name):
    _d(aue_world)["ids"][name] = str(uuid.uuid4())


@given(parsers.parse('"{name}" has {k:d} tracking views of "L1" in the warehouse'))
def tracking_views(aue_world, name, k):
    actor, marker = aue_world.actors[name], uuid.uuid4().hex[:10]
    _post_views([_beacon(_listing(aue_world, "L1"), f"{marker}-{i}") for i in range(k)], actor.token)
    aue.wait_rows(
        "SELECT event_id FROM tracking_events WHERE principal_id = ?",
        [actor.user_id],
        k,
        f"tracking rows of {name}",
    )


@given(
    parsers.parse(
        '"{name}" has {k:d} tracking views of "L1" under anonymous ids "A1" and "A2", '
        "each also used by {m:d} anonymous views before login"
    )
)
def tracking_views_stitched(aue_world, name, k, m):
    actor = aue_world.actors[name]
    a1, a2 = (f"e2e-aue-anon-{uuid.uuid4().hex[:10]}-{n}" for n in (1, 2))
    _d(aue_world)["anon"] = {"A1": a1, "A2": a2}
    lid = _listing(aue_world, "L1")
    pre = [_beacon(lid, uuid.uuid4().hex[:10], anon) for anon in (a1, a2) for _ in range(m)]
    if pre:
        _post_views(pre, None)
    own = [_beacon(lid, uuid.uuid4().hex[:10], (a1, a2)[i % 2]) for i in range(k)]
    _post_views(own, actor.token)
    expected = k + len(pre)
    sql = "SELECT event_id FROM tracking_events WHERE principal_id = ? OR anonymous_id IN (?, ?)"
    rows = aue.wait_rows(sql, [actor.user_id, a1, a2], expected, "stitched tracking rows")
    assert len(rows) == expected, f"expected {expected} rows, found {len(rows)}"
    _d(aue_world)["tracking_event_ids"] = sorted(r[0] for r in rows)


@given(parsers.parse('"{name}" has a paid order of "L1", "L2" and "L3" in the warehouse'))
def paid_order(aue_world, name):
    buyer = aue_world.actors[name]
    s.post(aue_world, buyer, s.CART, "ClearCart", {})
    s.ensure_address(aue_world, buyer)
    for ln in ("L1", "L2", "L3"):
        s.add_to_cart(aue_world, buyer, ln, 1)
    orders = s.ok(s.checkout(aue_world, buyer)).get("orders", [])
    assert orders, "checkout created no order"
    for order in orders:
        s.pay_order(aue_world, buyer, order["id"])
        s.wait_status(aue_world, buyer, order["id"], s.PAID)
    _d(aue_world).setdefault("orders", {})[name] = [o["id"] for o in orders]
    aue.wait_rows(
        "SELECT order_id FROM order_facts WHERE buyer_id = ?", [buyer.user_id], 3, "order_facts"
    )


@given(parsers.parse('"{name}" has a favourite of "L1" in the warehouse'))
def one_favourite(aue_world, name):
    actor = aue_world.actors[name]
    s.ok(s.post(aue_world, actor, ENG, "AddFavorite", {"listingId": _listing(aue_world, "L1")}))
    aue.wait_rows(
        "SELECT fact FROM engagement_facts WHERE user_id = ?", [actor.user_id], 1, "favourite fact"
    )


@given(parsers.parse('"{name}" has 4 favourite facts and 2 follow facts in the warehouse'))
def favs_and_follows(aue_world, name):
    actor = aue_world.actors[name]
    for i in range(1, 5):
        s.ok(s.post(aue_world, actor, ENG, "AddFavorite", {"listingId": _listing(aue_world, f"L{i}")}))
    for seller in ("S", "S2"):
        s.ok(
            s.post(
                aue_world, actor, ENG, "FollowSeller", {"sellerId": aue_world.actors[seller].user_id}
            )
        )
    aue.wait_rows(
        "SELECT fact FROM engagement_facts WHERE user_id = ?", [actor.user_id], 6, "facts"
    )


@given(parsers.parse('"{name}" follows the seller "{seller}" in the warehouse'))
def follows(aue_world, name, seller):
    actor, sid = aue_world.actors[name], aue_world.actors[seller].user_id
    s.ok(s.post(aue_world, actor, ENG, "FollowSeller", {"sellerId": sid}))
    aue.wait_rows(
        "SELECT user_id FROM follows_current WHERE seller_id = ?", [sid], 1, "follows_current"
    )


@given(parsers.parse('a visitor has viewed "L1" and the seller funnel of "{name}" shows at least 1 view'))
def visitor_view(aue_world, name):
    _post_views([_beacon(_listing(aue_world, "L1"), uuid.uuid4().hex[:10])], None)
    sid = aue_world.actors[name].user_id
    _poll(lambda: _funnel(aue_world, sid)["views"], lambda v: v >= 1, "seller funnel views")


# ── recording ────────────────────────────────────────────────────────────
@given("the warehouse table sizes are recorded")
@when("the warehouse table sizes are recorded")
def record_sizes(aue_world):
    _d(aue_world)["sizes"] = aue.stable_table_sizes()


@given(parsers.parse('the seller funnel of "{name}" is recorded'))
def record_funnel(aue_world, name):
    sid = aue_world.actors[name].user_id
    _d(aue_world)["funnel"] = _poll(
        lambda: _funnel(aue_world, sid), lambda f: f["views"] >= 50, "seller funnel views of the seeded rows"
    )


@given(parsers.parse('the tracking rows of "{name}" are recorded'))
def record_rows(aue_world, name):
    _d(aue_world).setdefault("rows", {})[name] = _tracking_rows(_uid(aue_world, name))
    assert _d(aue_world)["rows"][name], f"no tracking rows recorded for {name}"


def _tracking_rows(uid: str) -> list[tuple]:
    rows = tii.warehouse_query("SELECT * FROM tracking_events WHERE principal_id = ?", [uid])
    return sorted(rows, key=repr)


@given(parsers.parse('the order lines of "{name}" and their order count and GMV are recorded'))
def record_orders(aue_world, name):
    uid = aue_world.actors[name].user_id
    sql = "SELECT order_id, quantity, unit_price, occurred_at FROM order_facts WHERE buyer_id = ?"
    lines = sorted(tii.warehouse_query(sql, [uid]), key=repr)
    assert len(lines) == 3, lines
    _d(aue_world)["order_lines"] = lines
    _d(aue_world)["summary"] = _gmv(sorted({line[0] for line in lines}))


@given(parsers.parse('the listing_sellers rows of "{name}" are recorded'))
def record_listing_sellers(aue_world, name):
    sid = aue_world.actors[name].user_id
    rows = aue.wait_rows(
        "SELECT listing_id, status FROM listing_sellers WHERE seller_id = ?", [sid], 3, "listings"
    )
    _d(aue_world)["listing_sellers"] = sorted(rows, key=repr)


# ── events ───────────────────────────────────────────────────────────────
@when(parsers.parse('a UserErased event for "{name}" is produced'))
@when(parsers.parse('a UserErased event for the synthetic user "{name}" is produced'))
@when(parsers.parse('a UserErased event for the seeded user "{name}" is produced'))
def produce_erased(aue_world, name):
    _d(aue_world).setdefault("events", {})[name] = ev.publish_user_erased(_uid(aue_world, name))


@when(parsers.parse('a UserErased event for the synthetic user "{name}" with no keys is produced'))
def produce_erased_no_keys(aue_world, name):
    produce_erased(aue_world, name)


@when(parsers.parse('a UserErased event for the seller "{name}" is produced'))
def produce_erased_seller(aue_world, name):
    _d(aue_world).setdefault("events", {})[name] = ev.publish_user_erased(
        _uid(aue_world, name), roles=("seller",)
    )


@when(parsers.parse('the same UserErased event for "{name}" is produced again'))
@when(parsers.parse('the same UserErased event for the synthetic user "{name}" is produced again'))
def produce_again(aue_world, name):
    eid = _d(aue_world)["events"][name]
    ev.publish_user_erased(_uid(aue_world, name), event_id=eid)


@when(parsers.parse('a UserStatusChanged envelope for "{name}" is produced'))
def produce_status_changed(aue_world, name):
    _d(aue_world)["status_event"] = aue.publish_status_changed(_uid(aue_world, name))


@when("a record that is not an EventEnvelope and a UserErased with an empty user_id are produced")
def produce_poison(aue_world):
    marker = f"e2e-aue-poison-{uuid.uuid4().hex[:10]}"
    ev.publish_garbage(marker)
    _d(aue_world)["poison"] = {"garbage": marker, "empty": ev.publish_user_erased("")}


@when(parsers.parse('a UserErased event naming the id of "{name}" without its last character is produced'))
def produce_prefix(aue_world, name):
    _d(aue_world)["events"] = {name: ev.publish_user_erased(aue_world.actors[name].user_id[:-1])}


# ── late events ──────────────────────────────────────────────────────────
def _late_view(w, name: str) -> str:
    marker = uuid.uuid4().hex[:12]
    _post_views([_beacon(_listing(w, "L1"), marker)], w.actors[name].token)
    _d(w)["late_marker"] = marker
    return marker


@when(parsers.parse('"{name}" posts a tracking view of "L1" with the token issued before the erasure'))
@when(parsers.parse('"{name}" posts a tracking view of "L1" with their own token'))
def late_view(aue_world, name):
    _late_view(aue_world, name)


@when(parsers.parse('within 60 seconds no warehouse table contains the id of "{name}"'))
def when_no_hits(aue_world, name):
    aue.wait_no_warehouse_hits([_uid(aue_world, name)])


@when("the erased_users size is recorded")
def record_tombstones(aue_world):
    _d(aue_world)["tombstones"] = aue.table_count("erased_users")


@when(parsers.parse('the analytics erasure consumer group has no lag'))
def when_analytics_caught_up(aue_world):
    ev.wait_group_caught_up(aue.ANALYTICS_GROUP)


# ── Then: warehouse ──────────────────────────────────────────────────────
@then(parsers.parse('within 60 seconds no warehouse table contains the id of "{name}"'))
def then_no_hits(aue_world, name):
    aue.wait_no_warehouse_hits([_uid(aue_world, name)])


@then(
    parsers.parse(
        'within 60 seconds no warehouse table contains the id of "{name}", "A1" or "A2"'
    )
)
def then_no_hits_anon(aue_world, name):
    anon = _d(aue_world)["anon"]
    aue.wait_no_warehouse_hits([_uid(aue_world, name), anon["A1"], anon["A2"]])


@then("the analytics erasure consumer group has no lag")
def then_analytics_caught_up(aue_world):
    ev.wait_group_caught_up(aue.ANALYTICS_GROUP)


@then("no warehouse table changed size")
def no_size_change(aue_world):
    before, after = _d(aue_world)["sizes"], aue.stable_table_sizes()
    assert after == before, f"table sizes changed: before={before} after={after}"


@then("only erased_users grew, by one row")
def only_tombstone_grew(aue_world):
    before, after = _d(aue_world)["sizes"], aue.stable_table_sizes()
    for table, size in after.items():
        want = before.get(table, 0) + (1 if table == "erased_users" else 0)
        assert size == want, f"{table}: {size} rows, expected {want}"
    assert "erased_users" in after, "no erased_users table"


@then(parsers.parse('the id of "{name}" is still present in tracking_events'))
def still_present(aue_world, name):
    n = aue.count(
        "SELECT COUNT(*) FROM tracking_events WHERE principal_id = ?", [_uid(aue_world, name)]
    )
    assert n >= 1, "a UserStatusChanged event must not re-key anything"


@then("the analytics DLQ holds no record for that event")
def analytics_dlq_empty(aue_world):
    d = _d(aue_world)
    eids = list(d.get("events", {}).values()) or [d["status_event"]]
    for eid in eids:
        ev.assert_dlq_empty(aue.ANALYTICS_DLQ, eid.encode())


@then("both records are on identity.events.analytics.dlq with an error header")
def poison_on_analytics_dlq(aue_world):
    poison = _d(aue_world)["poison"]
    for needle in (poison["garbage"], poison["empty"]):
        found = ev.wait_dlq_record(aue.ANALYTICS_DLQ, needle.encode())
        assert found, f"{needle} is not on {aue.ANALYTICS_DLQ}"
        assert all(aue.has_error_header(r) for r in found), f"no error header on {needle} record"


@then(parsers.parse('the 70 recorded tracking rows still exist and all carry the same single erased pseudonym'))
def stitched_rows_rekeyed(aue_world):
    ids = _d(aue_world)["tracking_event_ids"]
    assert len(ids) == 70, len(ids)
    marks = ", ".join("?" for _ in ids)
    rows = tii.warehouse_rows(
        f"SELECT principal_id, session_id, anonymous_id FROM tracking_events WHERE event_id IN ({marks})",
        ids,
        until=lambda r: len(r) == 70 and all(aue.PSEUDONYM.match(v or "") for row in r for v in row),
    )
    pseudonyms = {v for row in rows for v in row}
    assert len(rows) == 70, f"{len(rows)} of 70 rows remain"
    assert len(pseudonyms) == 1 and aue.PSEUDONYM.match(next(iter(pseudonyms))), pseudonyms
    leftovers = tii.warehouse_query(
        f"SELECT COUNT(*) FROM tracking_events WHERE event_id IN ({marks}) "
        "AND (ga_client_id IS NOT NULL OR properties IS NOT NULL)",
        ids,
    )[0][0]
    assert leftovers == 0, f"{leftovers} rows keep ga_client_id or properties"


@then(parsers.parse('the seller funnel of "{name}" is unchanged'))
def funnel_unchanged(aue_world, name):
    want = _d(aue_world)["funnel"]
    got = _poll(
        lambda: _funnel(aue_world, aue_world.actors[name].user_id),
        lambda f: f == want,
        "seller funnel unchanged",
    )
    assert got == want


@then(parsers.parse('the seller funnel of "{name}" has zero counts'))
def funnel_zero(aue_world, name):
    _poll(
        lambda: _funnel(aue_world, aue_world.actors[name].user_id),
        lambda f: not any(f.values()),
        "seller funnel zero",
    )


@then(parsers.parse('the tracking rows of "{name}" are identical to the recorded ones'))
def rows_identical(aue_world, name):
    want = _d(aue_world)["rows"][name]
    got = _tracking_rows(_uid(aue_world, name))
    assert got == want, f"rows of {name} changed: {len(want)} before, {len(got)} after"


@then("the 3 recorded order lines keep order_id, quantity, unit_price and occurred_at and have an erased buyer_id")
def order_lines_kept(aue_world):
    lines = _d(aue_world)["order_lines"]
    order_ids = sorted({line[0] for line in lines})
    marks = ", ".join("?" for _ in order_ids)
    sql = (
        "SELECT order_id, quantity, unit_price, occurred_at, buyer_id FROM order_facts "
        f"WHERE order_id IN ({marks})"
    )
    rows = tii.warehouse_rows(
        sql, order_ids, until=lambda r: len(r) == 3 and all(aue.PSEUDONYM.match(x[4] or "") for x in r)
    )
    assert sorted((r[:4] for r in rows), key=repr) == lines, (rows, lines)


@then("the order count and GMV of those orders are unchanged")
def summary_unchanged(aue_world):
    want = _d(aue_world)["summary"]
    ids = sorted({line[0] for line in _d(aue_world)["order_lines"]})
    got = _poll(lambda: _gmv(ids), lambda v: v == want, "order count and GMV")
    assert got == want


@then(parsers.parse('engagement_facts, favorites_current and follows_current have no row for "{name}"'))
def no_engagement_rows(aue_world, name):
    uid = aue_world.actors[name].user_id
    marks = ", ".join(f"'{f}'" for f in LISTING_FACTS)
    assert (
        aue.count(f"SELECT COUNT(*) FROM engagement_facts WHERE user_id = ? AND fact IN ({marks})", [uid])
        == 0
    )
    assert aue.count("SELECT COUNT(*) FROM favorites_current WHERE user_id = ?", [uid]) == 0
    assert aue.count("SELECT COUNT(*) FROM follows_current WHERE user_id = ?", [uid]) == 0


@then(parsers.parse('"{name}" still has 1 favourite fact and 1 current favourite'))
def other_user_facts(aue_world, name):
    uid = aue_world.actors[name].user_id
    assert aue.count("SELECT COUNT(*) FROM engagement_facts WHERE user_id = ?", [uid]) == 1
    assert aue.count("SELECT COUNT(*) FROM favorites_current WHERE user_id = ?", [uid]) == 1


@then("the recorded listings keep their listing_id and status in listing_sellers")
def listings_kept(aue_world):
    want = _d(aue_world)["listing_sellers"]
    ids = [r[0] for r in want]
    marks = ", ".join("?" for _ in ids)
    rows = tii.warehouse_rows(
        f"SELECT listing_id, status, seller_id FROM listing_sellers WHERE listing_id IN ({marks})",
        ids,
        until=lambda r: len(r) == len(ids) and all(aue.PSEUDONYM.match(x[2] or "") for x in r),
    )
    assert sorted((r[:2] for r in rows), key=repr) == want, (rows, want)


@then("the stored row of that view carries an erased pseudonym")
def late_row_pseudonym(aue_world):
    marker = _d(aue_world)["late_marker"]
    rows = tii.warehouse_rows(
        "SELECT principal_id FROM tracking_events WHERE page_path = ?", [f"/e2e/aue/{marker}"]
    )
    assert len(rows) == 1 and aue.PSEUDONYM.match(rows[0][0] or ""), rows


@then(parsers.parse('the stored row of that view keeps the id of "{name}"'))
def late_row_keeps(aue_world, name):
    marker = _d(aue_world)["late_marker"]
    rows = tii.warehouse_rows(
        "SELECT principal_id FROM tracking_events WHERE page_path = ?", [f"/e2e/aue/{marker}"]
    )
    assert rows == [(aue_world.actors[name].user_id,)], rows


@then("the erased_users size is unchanged")
def tombstones_unchanged(aue_world):
    assert aue.table_count("erased_users") == _d(aue_world)["tombstones"]


@then("erased_users has a row whose user_hmac is 64 hex characters")
def tombstone_hmac(aue_world):
    rows = tii.warehouse_query("SELECT user_hmac FROM erased_users")
    assert any(re.fullmatch(r"[0-9a-f]{64}", r[0] or "") for r in rows), rows[:3]


@then(parsers.parse('no column of erased_users equals the id of "{name}"'))
def tombstone_no_id(aue_world, name):
    uid = _uid(aue_world, name)
    rows = tii.warehouse_query("SELECT * FROM erased_users")
    assert rows, "erased_users is empty"
    assert not any(uid == str(v) for row in rows for v in row), "erased_users holds a clear id"


# ── Parquet ──────────────────────────────────────────────────────────────
@given(parsers.parse('the Parquet export files contain the id of "{name}"'))
def parquet_has(aue_world, name):
    aue.wait_parquet([_uid(aue_world, name)], present=True)


@when(parsers.parse('after one export cycle the Parquet files hold no id of "{name}"'))
def parquet_clean_when(aue_world, name):
    aue.wait_parquet([_uid(aue_world, name)], present=False)


@then(
    parsers.parse(
        'after one export cycle a scan of the Parquet files for the id of "{name}", "A1" and "A2" '
        "finds zero matches"
    )
)
def parquet_clean_anon(aue_world, name):
    anon = _d(aue_world)["anon"]
    aue.wait_parquet([_uid(aue_world, name), anon["A1"], anon["A2"]], present=False)


# ── recsys / team-ai / featurestore ─────────────────────────────────────
def _recsys_keys(uid: str) -> list[str]:
    return [
        f"recs:v1:gen:e2eg1:user:{uid}",
        f"recs:v1:gen:e2eg2:user:{uid}",
        f"recs:v1:user:{uid}",
        f"recs:nearline:user:{uid}:items",
        f"recs:nearline:user:{uid}:cats",
    ]


def _collection(w) -> str:
    d = _d(w)
    if "collection" not in d:
        d["collection"] = aue.new_user_collection()
        _cleanup(w, lambda: aue.drop_collection(d["collection"]))
    return d["collection"]


def _redis(w, db: int | None = None) -> aue.Redis:
    r = aue.Redis(db)
    _cleanup(w, r.close)
    return r


def _seed_recsys(w, name: str) -> None:
    uid = _uid(w, name)
    r = _redis(w)
    keys = _recsys_keys(uid)
    for key in keys:
        r.set(key)
    _cleanup(w, lambda: aue.Redis().delete(*keys))
    aue.put_user_point(_collection(w), uid)


@given(
    parsers.parse(
        'a recsys user "{name}" with recommendation keys under two generations, a legacy key, '
        "two nearline keys and a Qdrant vector"
    )
)
@given(parsers.parse('a recsys user "{name}" with the same keys'))
def recsys_user(aue_world, name):
    _seed_recsys(aue_world, name)


def _recsys_left(w, name: str) -> tuple[list[str], int]:
    uid = _uid(w, name)
    r = aue.Redis()
    try:
        return r.containing(uid), aue.user_points(_collection(w), uid)
    finally:
        r.close()


@then(parsers.parse('within 60 seconds zero Redis keys and zero Qdrant points contain the id of "{name}"'))
def recsys_gone_within(aue_world, name):
    _poll(lambda: _recsys_left(aue_world, name), lambda v: not v[0] and v[1] == 0, "recsys state of U")


@then(parsers.parse('zero Redis keys and zero Qdrant points contain the id of "{name}"'))
def recsys_gone(aue_world, name):
    keys, points = _recsys_left(aue_world, name)
    assert not keys and points == 0, (keys, points)


@then(parsers.parse('the keys and the Qdrant point of "{name}" are unchanged'))
def recsys_kept(aue_world, name):
    keys, points = _recsys_left(aue_world, name)
    assert sorted(keys) == sorted(_recsys_keys(_uid(aue_world, name))), keys
    assert points == 1, points


@then("the recsys erasure consumer group has no lag")
def recsys_caught_up(aue_world):
    ev.wait_group_caught_up(aue.RECSYS_GROUP)


@then("the recsys DLQ holds no record for those events")
def recsys_dlq_empty(aue_world):
    for eid in _d(aue_world)["events"].values():
        ev.assert_dlq_empty(aue.RECSYS_DLQ, eid.encode())


def _poison_on(topic: str, w) -> None:
    poison = _d(w)["poison"]
    for needle in (poison["garbage"], poison["empty"]):
        assert ev.wait_dlq_record(topic, needle.encode()), f"{needle} is not on {topic}"


@then("both records are on identity.events.recsys.dlq")
def poison_recsys(aue_world):
    _poison_on(aue.RECSYS_DLQ, aue_world)


@then("both records are on identity.events.team-ai.dlq")
def poison_team_ai(aue_world):
    _poison_on(aue.TEAM_AI_DLQ, aue_world)


def _ai_keys(w, uid: str) -> list[str]:
    quota = aue.team_ai_env("QUOTA_REDIS_PREFIX", "quota")
    rate = aue.team_ai_env("RATE_LIMIT_REDIS_PREFIX", "rate-limit")
    sessions = [f"chat:session:{quote(uid, safe='')}:{i}" for i in (1, 2, 3)]
    return sessions + [f"{quota}:c:user:{uid}:e2e:1", f"{rate}:principal:user:{uid}:1"]


@given(parsers.parse('a team-ai user "{name}" with three chat sessions, a quota counter and a rate-limit counter'))
@given(parsers.parse('a team-ai user "{name}" with the same keys'))
def team_ai_user(aue_world, name):
    keys = _ai_keys(aue_world, _uid(aue_world, name))
    r = _redis(aue_world)
    for key in keys:
        r.set(key)
    _cleanup(aue_world, lambda: aue.Redis().delete(*keys))


@given(parsers.parse('a team-ai user "{name}" with a chat session'))
def team_ai_session(aue_world, name):
    key = _ai_keys(aue_world, _uid(aue_world, name))[0]
    _redis(aue_world).set(key)
    _cleanup(aue_world, lambda: aue.Redis().delete(key))
    _d(aue_world).setdefault("session_keys", {})[name] = key


def _present(w, name: str) -> list[str]:
    r = aue.Redis()
    try:
        return [k for k in _ai_keys(w, _uid(w, name)) if r.exists(k)]
    finally:
        r.close()


@then(parsers.parse('within 60 seconds none of the team-ai keys of "{name}" exists'))
def team_ai_gone(aue_world, name):
    _poll(lambda: _present(aue_world, name), lambda v: not v, f"team-ai keys of {name}")


@then(parsers.parse('the team-ai keys of "{name}" are present'))
def team_ai_kept(aue_world, name):
    want = _d(aue_world).get("session_keys", {}).get(name)
    present = _present(aue_world, name)
    if want:
        assert want in present, f"{want} was deleted"
    else:
        assert len(present) == 5, present


@then("the team-ai erasure consumer group has no lag")
def team_ai_caught_up(aue_world):
    ev.wait_group_caught_up(aue.TEAM_AI_GROUP)


# ── featurestore ─────────────────────────────────────────────────────────
@given(parsers.parse('the featurestore online store holds user_activity and user_preferences keys of "{name}"'))
def fs_keys(aue_world, name):
    uid = aue_world.actors[name].user_id
    keys = [f"fs:user_activity:v2:{uid}", f"fs:user_preferences:v1:{uid}"]
    r = _redis(aue_world, fsm.redis_db())
    for key in keys:
        r.set(key)
    _cleanup(aue_world, lambda: aue.Redis(fsm.redis_db()).delete(*keys))
    _d(aue_world)["fs_keys"] = keys


@given(
    parsers.parse(
        'the seeded user "{name}" also has recommendation keys, a Qdrant vector, featurestore keys '
        "and a chat session"
    )
)
def seeded_everywhere(aue_world, name):
    uid = aue_world.actors[name].user_id
    _seed_recsys(aue_world, name)
    fs_keys(aue_world, name)
    key = f"chat:session:{quote(uid, safe='')}:1"
    _redis(aue_world).set(key)
    _cleanup(aue_world, lambda: aue.Redis().delete(key))


@when("the materialize job runs")
def run_materialize(aue_world):
    offline = fsm.new_offline_dir()
    _cleanup(aue_world, lambda: fsm.drop_offline_dir(offline))
    _d(aue_world)["offline"] = offline
    res = fsm.run_job("materialize", offline)
    assert res.exit_code == 0, f"materialize exited {res.exit_code}:\n{res.output[-3000:]}"


@then(parsers.parse('both featurestore keys of "{name}" are gone and no featurestore key has an erased entity'))
def fs_keys_gone(aue_world, name):
    r = aue.Redis(fsm.redis_db())
    try:
        left = [k for k in _d(aue_world)["fs_keys"] if r.exists(k)]
        erased = r.containing("erased:")
        others = [k for k in r.containing("fs:user_activity:v2:") if aue_world.actors[name].user_id not in k]
    finally:
        r.close()
    assert not left, f"stale keys survived the reconcile: {left}"
    assert not [k for k in erased if k.startswith("fs:")], erased
    assert others, "the reconcile removed every other user's keys"


@then("the parity check passes")
def parity_passes(aue_world):
    res = fsm.run_job("parity", _d(aue_world)["offline"])
    assert res.exit_code == 0, f"parity exited {res.exit_code}:\n{res.output[-2000:]}"


# ── end to end and stack wiring ──────────────────────────────────────────
@then(parsers.parse('the warehouse, Parquet files, Redis and Qdrant contain zero occurrences of the id of "{name}"'))
def everywhere_clean(aue_world, name):
    uid = aue_world.actors[name].user_id
    aue.wait_no_warehouse_hits([uid])
    assert sum(aue.parquet_hits([uid]).values()) == 0
    for db in (None, fsm.redis_db()):
        r = aue.Redis(db)
        try:
            assert not r.containing(uid), f"Redis DB {db} still holds {r.containing(uid)}"
        finally:
            r.close()
    _poll(
        lambda: aue.user_points(_collection(aue_world), uid), lambda n: n == 0, "Qdrant points"
    )


@then("the three erasure DLQ topics hold zero records for that event")
def three_dlqs_empty(aue_world):
    eid = next(iter(_d(aue_world)["events"].values()))
    for topic in aue.ALL_DLQS:
        ev.assert_dlq_empty(topic, eid.encode())


@then("the three erasure DLQ topics exist on the broker")
def dlq_topics_exist(aue_world):
    missing = [t for t in aue.ALL_DLQS if not ev.topic_exists(t)]
    assert not missing, f"DLQ topics missing on the broker: {missing}"


@then("the analytics, recsys and team-ai erasure consumer groups exist")
def groups_exist(aue_world):
    from tests.e2e.support import plp_stack as stack

    missing = [
        g for g in (aue.ANALYTICS_GROUP, aue.RECSYS_GROUP, aue.TEAM_AI_GROUP) if not stack.group_exists(g)
    ]
    assert not missing, f"erasure consumer groups not running: {missing}"
    time.sleep(0)
