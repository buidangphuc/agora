"""GA4 export import, attributed channel funnel and reconciliation (add-ga4-export-import).

Black box. The import runs inside the consumer; the job is triggered with the thin CLI client:

    docker exec <TEAM_ANALYTICS_CONTAINER> /ga4import --date YYYY-MM-DD

(exit 0 on HTTP 200 from the consumer's internal trigger, non-zero + stderr text otherwise).
The consumer must run with GA4_EXPORT_SOURCE=fixture, GA4_EXPORT_DATASET=fixture and
GA4_EXPORT_FIXTURE_DIR=/data/ga4_fixture (stack override must set them; the e2e cannot change the
consumer's env). Override the command with GA4_IMPORT_CMD (shell template, `{date}` substituted).
The fixture is a Parquet file named `events_YYYYMMDD.parquet` in GA4 export schema (event_date,
event_timestamp micros, event_name, user_pseudo_id, event_params[{key,value{string_value,int_value}}],
traffic_source{name,medium,source}) docker-cp'd into the consumer's /data. Warehouse state is read
back with tii_support.warehouse_query.
Spec scenario names are echoed by the feature scenarios.
"""

from __future__ import annotations

import os
import random
import shlex
import shutil
import statistics
import subprocess
import tempfile
import time
import uuid
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import adq_support as adq
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

_FUNNEL = "/platform.analytics.v1.AnalyticsQueryService/GetChannelFunnel"
FIXTURE_DIR = "/data/ga4_fixture"
JOB_TIMEOUT_S = 900
UTC = timezone.utc


def _x(world: World) -> dict:
    return world.state.extra


def _compact(d: date) -> str:
    return d.strftime("%Y%m%d")


def _fresh_date(taken: set[date] | None = None) -> date:
    """A date far in the past: no live traffic and no daily run ever touches it."""
    while True:
        d = date(2001, 1, 1) + timedelta(days=random.randint(0, 3000))
        if not taken or d not in taken:
            return d


# ── fixture ──────────────────────────────────────────────────────────────
def _session(user: str, sid: int, campaign: str, minutes: list[datetime], **kw: Any) -> dict:
    return {
        "user": user,
        "sid": sid,
        "source": kw.get("source", "google"),
        "medium": kw.get("medium", "cpc"),
        "campaign": campaign,
        "page": kw.get("page", "https://agora.test/p/1"),
        "times": minutes,
    }


def _write_fixture(day: date, sessions: list[dict], extra_rows: list[tuple] | None = None) -> Path:
    import duckdb

    root = Path(tempfile.mkdtemp(prefix="ga4-fx-"))
    out = root / "ga4_fixture"
    out.mkdir()
    con = duckdb.connect()
    con.execute(
        "CREATE TABLE ev(event_date VARCHAR, event_timestamp BIGINT, event_name VARCHAR, "
        "user_pseudo_id VARCHAR, event_params STRUCT(key VARCHAR, value STRUCT(string_value "
        "VARCHAR, int_value BIGINT, float_value DOUBLE, double_value DOUBLE))[], traffic_source STRUCT(name VARCHAR, medium VARCHAR, "
        "source VARCHAR))"
    )

    def sp(key: str, s: str | None = None, i: int | None = None) -> dict:
        return {
            "key": key,
            "value": {"string_value": s, "int_value": i, "float_value": None, "double_value": None},
        }

    rows = []
    for s in sessions:
        params = [
            sp("ga_session_id", i=s["sid"]),
            sp("source", s["source"]),
            sp("medium", s["medium"]),
            sp("campaign", s["campaign"]),
            sp("page_location", s["page"]),
        ]
        ts = {"name": s["campaign"], "medium": s["medium"], "source": s["source"]}
        for t in s["times"]:
            rows.append(
                (
                    _compact(day),
                    int(t.timestamp() * 1_000_000),
                    "view_item",
                    s["user"],
                    params,
                    ts,
                )
            )
    for r in extra_rows or []:
        rows.append(r)
    if rows:
        con.executemany("INSERT INTO ev VALUES (?,?,?,?,?,?)", rows)
    con.execute(f"COPY ev TO '{out / ('events_' + _compact(day) + '.parquet')}' (FORMAT PARQUET)")
    con.close()
    return out


def _stage(fixture_dir: Path, container: str) -> None:
    res = subprocess.run(
        ["docker", "cp", str(fixture_dir), f"{container}:/data/"],
        capture_output=True,
        text=True,
        timeout=600,
    )
    assert res.returncode == 0, f"docker cp of the GA4 fixture failed: {res.stderr.strip()}"
    shutil.rmtree(fixture_dir.parent, ignore_errors=True)


def _register(world: World, day: date, sessions: list[dict], **kw: Any) -> None:
    x = _x(world)
    d = _write_fixture(day, sessions, **kw)
    _stage(d, tii.analytics_container())
    x.setdefault("g4_dates", []).append(day)
    x["g4_date"] = day


def _t(day: date, hh: int, mm: int) -> datetime:
    return datetime(day.year, day.month, day.day, hh, mm, tzinfo=UTC)


# ── job runner ───────────────────────────────────────────────────────────
def _run_job(day: date | str, overrides: dict[str, str | None] | None = None) -> tuple[Any, float]:
    """Run the thin CLI inside the consumer container. `overrides` only reach the CLI process
    (e.g. GA4_RECONCILE_MAX_DIFF_RATIO); the consumer's own env is fixed by the stack."""
    iso = day if isinstance(day, str) else day.isoformat()
    template = os.getenv("GA4_IMPORT_CMD")
    run_env = None
    if template:
        cmd = shlex.split(template.format(date=iso))
        run_env = {**os.environ, **{k: v for k, v in (overrides or {}).items() if v is not None}}
    else:
        cmd = ["docker", "exec"]
        for k, v in (overrides or {}).items():
            if v is not None:
                cmd += ["-e", f"{k}={v}"]
        cmd += [tii.analytics_container(), "/ga4import", "--date", iso]
    t0 = time.monotonic()
    proc = subprocess.run(
        cmd, capture_output=True, text=True, timeout=JOB_TIMEOUT_S, env=run_env, check=False
    )
    return proc, time.monotonic() - t0


def _run(world: World, day: date | str, overrides: dict[str, str | None] | None = None) -> None:
    proc, elapsed = _run_job(day, overrides)
    x = _x(world)
    x["g4_proc"], x["g4_elapsed"] = proc, elapsed


def _ok(world: World) -> None:
    proc = _x(world)["g4_proc"]
    assert proc.returncode == 0, (
        f"ga4import exited {proc.returncode}: {(proc.stderr or proc.stdout).strip()[:500]}"
    )


def _sessions_rows(day: date) -> list[tuple]:
    return sorted(
        tii.warehouse_query(
            "SELECT export_date, user_pseudo_id, session_id, source, medium, campaign, "
            "landing_page, ts, ended_at FROM ga4_sessions WHERE export_date = ?",
            [day],
        ),
        key=repr,
    )


# ── Givens: fixtures ─────────────────────────────────────────────────────
@given(parsers.parse("a GA4 fixture day with {n:d} sessions of {u:d} distinct users"))
def fixture_day(world: World, n: int, u: int) -> None:
    day = _fresh_date()
    users = [f"{random.randint(10**8, 10**9)}.{uuid.uuid4().hex[:6]}" for _ in range(u)]
    sessions = [
        _session(users[i % u], 1000 + i, f"camp-{i}", [_t(day, 10, i), _t(day, 10, i + 5)])
        for i in range(n)
    ]
    _register(world, day, sessions)


@given(parsers.parse("a second GA4 fixture day with {n:d} sessions of {u:d} user"))
def second_fixture_day(world: World, n: int, u: int) -> None:
    first = _x(world)["g4_dates"][0]
    day = _fresh_date({first})
    user = f"{random.randint(10**8, 10**9)}.{uuid.uuid4().hex[:6]}"
    _register(
        world,
        day,
        [
            _session(user, 2000 + i, f"camp2-{i}", [_t(day, 9, i), _t(day, 9, i + 3)])
            for i in range(n)
        ],
    )
    _x(world)["g4_date"] = _x(world)["g4_dates"][0]  # "the fixture date" stays the first one


@given("a GA4 fixture session with events at 10:00, 10:07 and 10:15")
def fixture_ended_at(world: World) -> None:
    day = _fresh_date()
    _register(
        world,
        day,
        [_session("5.5", 1, "c", [_t(day, 10, 0), _t(day, 10, 7), _t(day, 10, 15)])],
    )


@given(parsers.parse('a GA4 fixture session whose page_location is "{url}"'))
def fixture_landing(world: World, url: str) -> None:
    day = _fresh_date()
    _register(world, day, [_session("6.6", 1, "c", [_t(day, 10, 0)], page=url)])


@given("a GA4 fixture day with zero events")
def fixture_empty(world: World) -> None:
    _register(world, _fresh_date(), [])


@given(parsers.parse("a GA4 fixture day with {n:d} events"))
def fixture_big(world: World, n: int) -> None:
    import duckdb

    day = _fresh_date()
    root = Path(tempfile.mkdtemp(prefix="ga4-fx-"))
    out = root / "ga4_fixture"
    out.mkdir()
    base = int(_t(day, 0, 0).timestamp() * 1_000_000)
    con = duckdb.connect()
    con.execute(
        f"""COPY (SELECT '{_compact(day)}' AS event_date,
            {base} + i * 17 AS event_timestamp, 'view_item' AS event_name,
            CAST(i % 500000 AS VARCHAR) || '.1' AS user_pseudo_id,
            [{{'key': 'ga_session_id', 'value': {{'string_value': NULL::VARCHAR, 'int_value': (i % 500000)::BIGINT, 'float_value': NULL::DOUBLE, 'double_value': NULL::DOUBLE}}}}]
              AS event_params,
            {{'name': 'bulk', 'medium': 'cpc', 'source': 'google'}} AS traffic_source
          FROM range({n}) t(i))
          TO '{out / ("events_" + _compact(day) + ".parquet")}' (FORMAT PARQUET)"""
    )
    con.close()
    _stage(out, tii.analytics_container())
    _x(world)["g4_dates"] = [day]
    _x(world)["g4_date"] = day


@given(
    parsers.parse(
        'a GA4 export for today with a "spring" session and a "summer" session for two visitors'
    )
)
def fixture_today_channels(world: World) -> None:
    x = _x(world)
    rid = tii.run_id()
    now = datetime.now(UTC)
    day = now.date()
    x["g4_clients"] = {
        "spring": f"{random.randint(10**8, 10**9)}.{rid}1",
        "summer": f"{random.randint(10**8, 10**9)}.{rid}2",
    }
    x["g4_campaigns"] = {"spring": f"spring-{rid}", "summer": f"summer-{rid}"}
    sessions = [
        _session(
            x["g4_clients"][k],
            3000 + i,
            x["g4_campaigns"][k],
            [now - timedelta(minutes=10), now - timedelta(minutes=2)],
        )
        for i, k in enumerate(("spring", "summer"))
    ]
    _register(world, day, sessions)


@given("the ga4import job has run for the fixture date")
def job_has_run(world: World) -> None:
    _run(world, _x(world)["g4_date"])
    _ok(world)


# ── Whens: job ───────────────────────────────────────────────────────────
@when("the ga4import job runs for the fixture date")
def run_fixture_date(world: World) -> None:
    _run(world, _x(world)["g4_date"])


@when("the ga4import job runs for the first and then the second fixture date")
def run_both(world: World) -> None:
    d1, d2 = _x(world)["g4_dates"][:2]
    _run(world, d1)
    _ok(world)
    _run(world, d2)
    _ok(world)


@when("the ga4import job runs for the first fixture date")
@when("the ga4import job runs for the fixture date again")
def run_first_again(world: World) -> None:
    _run(world, _x(world)["g4_dates"][0])


@when("the ga4import job runs for a date whose export table does not exist")
def run_missing_table(world: World) -> None:
    missing = _fresh_date(set(_x(world)["g4_dates"]))
    _x(world)["g4_missing"] = missing
    _run(world, missing)


@when("the ga4import job starts with GA4_RECONCILE_MAX_DIFF_RATIO=-1")
def run_bad_threshold(world: World) -> None:
    _run(world, _fresh_date().isoformat(), {"GA4_RECONCILE_MAX_DIFF_RATIO": "-1"})


@when("the ga4_sessions rows of the fixture date are noted")
def note_rows(world: World) -> None:
    _ok(world)
    _x(world)["g4_noted"] = _sessions_rows(_x(world)["g4_date"])


@when("the ga4_sessions rows of the second fixture date are noted")
def note_rows_second(world: World) -> None:
    _x(world)["g4_noted2"] = _sessions_rows(_x(world)["g4_dates"][1])


# ── Thens: import ────────────────────────────────────────────────────────
@then("the job succeeds")
def job_succeeds(world: World) -> None:
    _ok(world)


@then(parsers.parse("ga4_sessions holds exactly {n:d} rows for that date"))
def sessions_count(world: World, n: int) -> None:
    _ok(world)
    rows = _sessions_rows(_x(world)["g4_date"])
    assert len(rows) == n, rows


@then("every one of those rows has a source, a medium and a session id")
def sessions_nonempty(world: World) -> None:
    _ok(world)
    for r in _sessions_rows(_x(world)["g4_date"]):
        assert r[2] not in (None, "") and r[3] and r[4], r


@then("the ga4_sessions rows of the fixture date equal the noted rows")
def rows_equal_noted(world: World) -> None:
    _ok(world)
    now = _sessions_rows(_x(world)["g4_date"])
    noted = _x(world).get("g4_noted") or _x(world).get("g4_noted2")
    assert now == noted and noted, (now, noted)


@then("the ga4_sessions rows of the second fixture date equal the noted rows")
def second_rows_equal(world: World) -> None:
    _ok(world)
    assert _sessions_rows(_x(world)["g4_dates"][1]) == _x(world)["g4_noted2"]


@then("every ga4_sessions row of both dates has a non-null export_date")
def export_date_nonnull(world: World) -> None:
    _ok(world)
    for d in _x(world)["g4_dates"][:2]:
        rows = _sessions_rows(d)
        assert rows and all(r[0] == d for r in rows), rows


@then("the session row has ts 10:00 and ended_at 10:15")
def ts_ended_at(world: World) -> None:
    _ok(world)
    rows = _sessions_rows(_x(world)["g4_date"])
    assert len(rows) == 1, rows
    assert (rows[0][7].hour, rows[0][7].minute) == (10, 0), rows
    assert (rows[0][8].hour, rows[0][8].minute) == (10, 15), rows


@then(parsers.parse('the stored landing_page equals "{url}"'))
def landing_equals(world: World, url: str) -> None:
    _ok(world)
    rows = _sessions_rows(_x(world)["g4_date"])
    assert [r[6] for r in rows] == [url], rows


@then(parsers.parse("an import run with rows {n:d} is recorded for that date"))
def import_run_rows(world: World, n: int) -> None:
    rows = tii.warehouse_query(
        "SELECT rows FROM ga4_import_runs WHERE export_date = ?", [_x(world)["g4_date"]]
    )
    assert rows and all(int(r[0]) == n for r in rows), rows


@then("an import run is recorded for that date")
def import_run_recorded(world: World) -> None:
    rows = tii.warehouse_query(
        "SELECT count(*) FROM ga4_import_runs WHERE export_date = ?", [_x(world)["g4_date"]]
    )
    assert rows[0][0] >= 1, rows


@then("the job finished in 10 minutes or less")
def within_budget(world: World) -> None:
    _ok(world)
    assert _x(world)["g4_elapsed"] <= 600, _x(world)["g4_elapsed"]


@then("the job exits with an error naming that date")
def error_names_date(world: World) -> None:
    proc = _x(world)["g4_proc"]
    d = _x(world)["g4_missing"]
    assert proc.returncode != 0, "job succeeded for a missing export table"
    out = (proc.stderr or "") + (proc.stdout or "")
    assert d.isoformat() in out or _compact(d) in out, out[:500]


@then("the ga4_sessions rows of the fixture date are unchanged")
def rows_unchanged(world: World) -> None:
    x = _x(world)
    first = x["g4_dates"][0]
    rows = _sessions_rows(first)
    assert len(rows) == 3 and all(r[0] == first for r in rows), rows


@then("the job exits with an error naming GA4_RECONCILE_MAX_DIFF_RATIO")
def error_names_setting(world: World) -> None:
    proc = _x(world)["g4_proc"]
    out = (proc.stderr or "") + (proc.stdout or "")
    assert proc.returncode != 0, "job accepted a negative threshold"
    assert "GA4_RECONCILE_MAX_DIFF_RATIO" in out, out[:500]


# ── Channel funnel ───────────────────────────────────────────────────────
def _admin(world: World) -> str:
    return adq.admin_token(world)


def _funnel(token: str | None, frm: datetime, to: datetime) -> httpx.Response:
    fmt = "%Y-%m-%dT%H:%M:%SZ"
    return pe.post_json(_FUNNEL, {"from": frm.strftime(fmt), "to": to.strftime(fmt)}, token)


@when("one visitor posts 4 views and the other visitor posts 2 views, each with their GA client id")
def post_attributed_views(world: World) -> None:
    x = _x(world)
    cookie = f"agora_consent=v=1&a=1&m=0&p=0&ts={int(time.time())}"
    for key, n in (("spring", 4), ("summer", 2)):
        anon = f"e2e-g4-anon-{uuid.uuid4()}"
        for _ in range(n):
            beacon = tii.view(
                f"e2e-g4-sess-{tii.run_id()}",
                eventId=str(uuid.uuid4()),
                anonymousId=anon,
                gaClientId=x["g4_clients"][key],
            )
            resp = httpx.post(
                f"{tii.gateway_url()}/api/track",
                json=[beacon],
                headers={"Cookie": cookie},
                timeout=15,
            )
            assert resp.status_code == 202, (resp.status_code, resp.text[:300])


@when("the admin requests the channel funnel around now")
def funnel_around_now(world: World) -> None:
    x = _x(world)
    token = _admin(world)
    want = x["g4_campaigns"]
    deadline = time.monotonic() + tii.WAREHOUSE_TIMEOUT_S
    resp = None
    while time.monotonic() < deadline:
        now = datetime.now(UTC)
        resp = _funnel(token, now - timedelta(hours=1), now + timedelta(hours=1))
        if resp.status_code == 200:
            rows = {
                r.get("campaign"): int(r.get("views", 0) or 0) for r in resp.json().get("rows", [])
            }
            if rows.get(want["spring"], 0) >= 4 and rows.get(want["summer"], 0) >= 2:
                break
        time.sleep(3.0)
    x["g4_funnel"] = resp


@then("the funnel has a spring row with 4 views and a summer row with 2 views")
def funnel_split(world: World) -> None:
    x = _x(world)
    resp = x["g4_funnel"]
    assert resp is not None and resp.status_code == 200, (
        resp.text if resp is not None else "no call"
    )
    rows = {r.get("campaign"): r for r in resp.json().get("rows", [])}
    for key, n in (("spring", 4), ("summer", 2)):
        row = rows.get(x["g4_campaigns"][key])
        assert row is not None, resp.text
        assert int(row.get("views", 0)) == n, row
        assert row.get("source") == "google" and row.get("medium") == "cpc", row


@when(
    "an anonymous client and then a logged-in buyer request the channel funnel through the gateway"
)
def funnel_anon_then_buyer(world: World) -> None:
    x = _x(world)
    now = datetime.now(UTC)
    x["g4_anon"] = _funnel(None, now - timedelta(days=1), now)
    x["g4_buyer"] = _funnel(x["adq_buyer"], now - timedelta(days=1), now)


@then("the gateway answers HTTP 401 and then HTTP 403 for the channel funnel")
def funnel_401_403(world: World) -> None:
    x = _x(world)
    assert x["g4_anon"].status_code == 401, (x["g4_anon"].status_code, x["g4_anon"].text[:300])
    assert x["g4_buyer"].status_code == 403, (x["g4_buyer"].status_code, x["g4_buyer"].text[:300])
    assert "rows" not in x["g4_buyer"].text


@when("the admin calls GetChannelFunnel 100 times over a 30-day window")
def funnel_100(world: World) -> None:
    x = _x(world)
    token = _admin(world)
    now = datetime.now(UTC)
    lat, statuses = [], []
    for _ in range(100):
        t0 = time.monotonic()
        r = _funnel(token, now - timedelta(days=30), now)
        lat.append(time.monotonic() - t0)
        statuses.append(r.status_code)
    x["g4_lat"], x["g4_statuses"] = lat, statuses


@then("every call succeeded")
def all_calls_ok(world: World) -> None:
    assert set(_x(world)["g4_statuses"]) == {200}, set(_x(world)["g4_statuses"])


@then("the p95 latency is under 2 seconds")
def p95(world: World) -> None:
    lat = sorted(_x(world)["g4_lat"])
    p = statistics.quantiles(lat, n=100)[94]
    assert p < 2.0, f"p95={p:.2f}s"


# ── Reconciliation ───────────────────────────────────────────────────────
@when("the admin reads the tracking quality report for the last hour")
def read_report_hour(world: World) -> None:
    x = _x(world)
    x["g4_report"] = adq.report_json(x["adq_admin"], 1)


@given("no import run exists for yesterday")
def no_import_yesterday(world: World) -> None:
    yday = datetime.now(UTC).date() - timedelta(days=1)
    try:
        rows = tii.warehouse_query(
            "SELECT count(*) FROM ga4_import_runs WHERE export_date = ?", [yday]
        )
        runs = rows[0][0]
    except Exception:  # noqa: BLE001 - table not there yet: no run exists
        runs = 0
    if runs:
        pytest.skip(f"yesterday ({yday}) already has an import run on this stack")


@then("the report carries reason ga4_missing and no ga4_divergence reason")
def reason_missing_only(world: World) -> None:
    reasons = _x(world)["g4_report"].get("reasons", [])
    assert "ga4_missing" in reasons and "ga4_divergence" not in reasons, reasons


@then("the report contains reason ga4_missing")
def reason_missing(world: World) -> None:
    assert "ga4_missing" in _x(world)["g4_report"].get("reasons", []), _x(world)["g4_report"]


@then("the report status is not degraded because of ga4")
def status_ok_for_ga4(world: World) -> None:
    rep = _x(world)["g4_report"]
    if rep.get("status") == "DEGRADED":
        assert set(rep.get("reasons", [])) - {"ga4_missing"}, rep  # degraded only by other reasons


@given("a GA4 export for yesterday with 100000 view events, far above the first-party count")
def export_yesterday_far_above(world: World) -> None:
    """The service reconciles YESTERDAY (UTC): GA4 views >> first-party views, ratio >> threshold."""
    import duckdb

    day = datetime.now(UTC).date() - timedelta(days=1)
    root = Path(tempfile.mkdtemp(prefix="ga4-fx-"))
    out = root / "ga4_fixture"
    out.mkdir()
    base = int(_t(day, 0, 0).timestamp() * 1_000_000)
    con = duckdb.connect()
    con.execute(
        f"""COPY (SELECT '{_compact(day)}' AS event_date,
            {base} + i * 17 AS event_timestamp, 'view_item' AS event_name,
            CAST(i % 1000 AS VARCHAR) || '.1' AS user_pseudo_id,
            [{{'key': 'ga_session_id', 'value': {{'string_value': NULL::VARCHAR, 'int_value': (i % 1000)::BIGINT, 'float_value': NULL::DOUBLE, 'double_value': NULL::DOUBLE}}}}]
              AS event_params,
            {{'name': '(direct)', 'medium': '(none)', 'source': '(direct)'}} AS traffic_source
          FROM range(100000) t(i))
          TO '{out / ("events_" + _compact(day) + ".parquet")}' (FORMAT PARQUET)"""
    )
    con.close()
    _stage(out, tii.analytics_container())
    _x(world)["g4_dates"] = [day]
    _x(world)["g4_date"] = day


@then("the report status is DEGRADED with reason ga4_divergence")
def degraded_divergence(world: World) -> None:
    _ok(world)
    rep = adq.poll_report(
        _x(world)["adq_admin"], lambda r: "ga4_divergence" in r.get("reasons", [])
    )
    assert rep.get("status") == "DEGRADED", rep
