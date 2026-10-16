"""Binds engagement/listing_reports.feature (engagement-listing-reports, initiative q6).

Black box through the gateway: CreateReport / ListReports / ResolveReport and QueryAuditLog are
Connect JSON calls; the only SQL is a read of seller_listings to wait for ownership propagation.
"""

from __future__ import annotations

import time

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import audit_support as aud
from tests.e2e.support import report_support as rs
from tests.e2e.support import review_order_support as ros
from tests.e2e.support.oic_order_support import AUTH, Actor, OicWorld, create_listing, register
from tests.e2e.support.world import World

scenarios("engagement/listing_reports.feature")


@pytest.fixture
def rl() -> OicWorld:
    return OicWorld()


def _lid(rl: OicWorld, name: str) -> str:
    return rl.listings[name]["id"]


def _sub(token: str) -> str:
    import base64
    import json

    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload)).get("sub", "")


def _code(rl: OicWorld, key: str) -> str:
    return ros.code_of(rl.responses[key])


# -- Given ------------------------------------------------------------------
@given(parsers.parse('a seller with a published listing "{name}"'))
def seller_listing(rl, name):
    seller = register(rl, "seller", "seller")
    create_listing(rl, name, seller, 50)


@given(parsers.re(r'a buyer "(?P<name>[^"]+)"'))
def a_buyer(rl, name):
    register(rl, name, "buyer")


@given("an admin")
def an_admin(rl):
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = ros.post(None, AUTH, "Login", {"username": admin.username, "password": admin.password})
    token = (ros.ok(resp).get("result") or {}).get("token", "")
    assert token, f"admin login failed: {resp.text[:200]}"
    rl.actors["admin"] = Actor(admin.username, token, "admin", _sub(token))


@given(parsers.parse('the engagement service knows the seller of "{name}"'))
def knows_seller(rl, name):
    rs.wait_seller_listing_known(_lid(rl, name))


@given(parsers.parse('"{who}" has reported "{listing}" with reason "{reason}"'))
def has_reported(rl, who, listing, reason):
    resp = rs.create_report(rl.actors[who], _lid(rl, listing), reason)
    report = ros.ok(resp)["report"]
    rl.data["report_id"] = report["id"]
    rl.data["reporter"] = who


@given("team-audit is stopped")
def audit_stopped(rl, world: World):
    aud.stop_audit(world)


# -- When -------------------------------------------------------------------
def _report(rl: OicWorld, actor: Actor | None, listing: str, reason: str, note: str = "") -> None:
    resp = rs.create_report(actor, _lid(rl, listing), reason, note)
    rl.responses["report"] = resp
    if resp.status_code == 200:
        rl.data.setdefault("report_ids", []).append(resp.json()["report"]["id"])


@when(parsers.re(r'"(?P<who>[^"]+)" reports "(?P<listing>[^"]+)" with reason "(?P<reason>[^"]+)"'))
def reports(rl, who, listing, reason):
    _report(rl, rl.actors[who], listing, reason)


@when(
    parsers.re(
        r'"(?P<who>[^"]+)" reports "(?P<listing>[^"]+)" with reason "(?P<reason>[^"]+)"'
        r' and note "(?P<note>[^"]+)"'
    )
)
def reports_note(rl, who, listing, reason, note):
    _report(rl, rl.actors[who], listing, reason, note)
    rl.data["note"] = note
    rl.data["reporter"] = who


@when(parsers.re(r'an anonymous client reports "(?P<listing>[^"]+)" with reason "(?P<reason>[^"]+)"'))
def anon_reports(rl, listing, reason):
    _report(rl, None, listing, reason)


@when(parsers.parse('the seller reports "{listing}" with reason "{reason}"'))
def seller_reports(rl, listing, reason):
    _report(rl, rl.actors["seller"], listing, reason)


@when(parsers.parse('the seller lists reports for "{listing}"'))
def seller_lists(rl, listing):
    rl.responses["list"] = rs.list_reports(rl.actors["seller"], target_id=_lid(rl, listing))


@when(parsers.parse('"{who}" lists reports for "{listing}"'))
def who_lists(rl, who, listing):
    rl.responses["list"] = rs.list_reports(rl.actors[who], target_id=_lid(rl, listing))


@when(parsers.parse('the admin lists open reports for "{listing}"'))
def admin_lists_open(rl, listing):
    rl.responses["list"] = rs.list_reports(
        rl.actors["admin"], target_id=_lid(rl, listing), status=rs.OPEN
    )


@when(parsers.parse('the admin closes that report as ACTIONED with the note "{note}"'))
def admin_closes(rl, note):
    rl.responses["close"] = rs.resolve_report(
        rl.actors["admin"], rl.data["report_id"], rs.ACTIONED, note
    )


@when(parsers.parse('"{who}" closes that report as ACTIONED with the note "{note}"'))
def who_closes(rl, who, note):
    rl.responses["close"] = rs.resolve_report(
        rl.actors[who], rl.data["report_id"], rs.ACTIONED, note
    )


@when("team-audit is started again")
def audit_started(rl, world: World):
    aud.start_audit(world)


# -- Then -------------------------------------------------------------------
@then(parsers.parse('the report call succeeds with status OPEN reported by "{who}"'))
def report_ok_open(rl, who):
    resp = rl.responses["report"]
    report = ros.ok(resp).get("report") or {}
    assert report.get("status") == rs.OPEN, report
    assert report.get("reporterId") == rl.actors[who].user_id, report
    assert report.get("id"), report
    assert report.get("createdAt"), report


@then("the report call returns the same report id as before")
def same_report_id(rl):
    ids = rl.data["report_ids"]
    assert len(ids) == 2 and ids[0] == ids[1], ids


@then(parsers.parse('the report call is refused with HTTP {status:d}'))
def report_http(rl, status):
    assert rl.responses["report"].status_code == status, rl.responses["report"].text


@then(parsers.parse('the report call fails with "{code}"'))
def report_fails(rl, code):
    assert _code(rl, "report") == code, rl.responses["report"].text


@then(parsers.parse('the list call fails with "{code}"'))
def list_fails(rl, code):
    assert _code(rl, "list") == code, rl.responses["list"].text


@then(parsers.parse('the close call fails with "{code}"'))
def close_fails(rl, code):
    assert _code(rl, "close") == code, rl.responses["close"].text


@then("the close call succeeds with status ACTIONED")
def close_ok(rl):
    report = ros.ok(rl.responses["close"]).get("report") or {}
    assert report.get("status") == rs.ACTIONED, report
    assert report.get("resolvedAt"), report


@then("the list response body contains neither the reporter id of \"b1\" nor the note")
def body_clean(rl):
    body = rl.responses["list"].text
    assert rl.actors["b1"].user_id and rl.actors["b1"].user_id not in body, body
    assert rl.data["note"] not in body and "secret-note" not in body, body


@then(parsers.parse('the admin sees exactly {n:d} report for "{listing}"'))
def admin_sees(rl, n, listing):
    reports = rs.reports_of(rl.actors["admin"], _lid(rl, listing))
    assert len(reports) == n, reports
    if n == 1 and rl.data.get("reporter"):
        assert reports[0].get("reporterId") == rl.actors[rl.data["reporter"]].user_id, reports
    if n == 1 and rl.data.get("note"):
        assert reports[0].get("note") == rl.data["note"], reports


@then(parsers.parse('the list contains the report of "{who}"'))
def list_contains(rl, who):
    reports = ros.ok(rl.responses["list"]).get("reports", [])
    mine = [r for r in reports if r.get("id") == rl.data["report_id"]]
    assert len(mine) == 1, reports
    assert mine[0].get("reporterId") == rl.actors[who].user_id, mine
    assert mine[0].get("status") == rs.OPEN, mine


@then(parsers.parse('the open reports for "{listing}" no longer contain the report'))
def open_no_longer(rl, listing):
    reports = rs.reports_of(rl.actors["admin"], _lid(rl, listing), rs.OPEN)
    assert rl.data["report_id"] not in [r.get("id") for r in reports], reports


@then(parsers.parse('the open reports for "{listing}" still contain the report'))
def open_still(rl, listing):
    reports = rs.reports_of(rl.actors["admin"], _lid(rl, listing), rs.OPEN)
    assert rl.data["report_id"] in [r.get("id") for r in reports], reports


@then(
    parsers.parse(
        'the audit log holds exactly {n:d} "{action}" event for the report by the admin '
        'with the note "{note}"'
    )
)
def audit_holds(rl, n, action, note):
    admin = rl.actors["admin"]
    report_id = rl.data["report_id"]
    deadline = time.monotonic() + 15
    events: list[dict] = []
    while time.monotonic() < deadline:
        events = rs.audit_events(admin, report_id)
        if len(events) >= n:
            break
        time.sleep(1)
    assert len(events) == n, events
    for event in events:
        assert event.get("actorId") == admin.user_id, event
        assert event.get("targetType") == "report", event
        meta = event.get("metadata") or {}
        assert meta.get("status") == "ACTIONED", event
        assert meta.get("resolution_note") == note, event
        assert meta.get("listing_id"), event
        assert "reporter_id" not in meta and "note" not in meta, event
