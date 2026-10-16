"""Binds admin/user_enforcement.feature (change identity-user-enforcement, initiative q6-trust-enforcement).

Black box through the gateway as the seeded admin; helpers in support/user_enforcement_support.
"""

from __future__ import annotations

import time

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import audit_support as aud
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

scenarios("admin/user_enforcement.feature")


def _s(world: World) -> dict:
    return world.state.extra.setdefault("ue", {})


def _msg(resp: httpx.Response) -> str:
    try:
        return str(resp.json().get("message", ""))
    except ValueError:
        return ""


def _expect_code(resp: httpx.Response, code: str) -> None:
    assert ue.code_of(resp) == code, f"expected {code}, got {resp.status_code} {resp.text[:300]}"


def _resp_ok(world: World, key: str = "resp") -> None:
    resp = _s(world)[key]
    assert resp.status_code == 200, f"the admin call itself failed: {resp.status_code} {resp.text[:200]}"


# ── Givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("the seeded admin and a buyer signed in on {n:d} devices"))
def admin_and_buyer(world: World, n: int) -> None:
    s = _s(world)
    s["admin_token"] = ue.admin_token(world)
    s["admin_id"] = ue.user_id_of(s["admin_token"])
    buyer = ue.register_user("buyer", "ue_buyer")
    for _ in range(n - 1):
        buyer.update(ue.login(buyer))
    s["buyer"] = buyer


@given("a signed-in seller")
def signed_in_seller(world: World) -> None:
    s = _s(world)
    s.setdefault("admin_token", ue.admin_token(world))
    s["seller"] = ue.register_user("seller", "ue_seller")


@given("the seeded admin and a signed-in seller")
def admin_and_seller(world: World) -> None:
    signed_in_seller(world)
    s = _s(world)
    s["admin_id"] = ue.user_id_of(s["admin_token"])


@given("another active buyer")
def other_buyer(world: World) -> None:
    _s(world)["other"] = ue.register_user("buyer", "ue_other")


@given("a second admin")
def second_admin(world: World) -> None:
    _s(world)["admin2"] = ue.register_second_admin()


@given("team-audit is down for this scenario")
def audit_down(world: World) -> None:
    aud.stop_audit(world)


@given("an active buyer, a suspended buyer and a banned buyer")
def three_buyers(world: World) -> None:
    s = _s(world)
    s["active"] = ue.register_user("buyer", "ue_act")
    s["suspended"] = ue.register_user("buyer", "ue_sus")
    s["banned"] = ue.register_user("buyer", "ue_ban")
    resp = ue.set_user_status(world, s["suspended"]["id"], "suspended", "scam", ue.in_future(3600))
    assert resp.status_code == 200, f"suspend setup failed: {resp.status_code} {resp.text}"
    resp = ue.set_user_status(world, s["banned"]["id"], "banned", "scam")
    assert resp.status_code == 200, f"ban setup failed: {resp.status_code} {resp.text}"


@given("the status events on identity.events are being tapped")
def tap_events(world: World) -> None:
    tap = ue.StatusEventTap()
    world.add_cleanup(tap.close)
    _s(world)["tap"] = tap


@given("both buyers hold an open notification stream")
def open_streams(world: World) -> None:
    s = _s(world)
    s["stream"] = ue.Stream(s["buyer"]["token"], f"user:{s['buyer']['id']}")
    s["other_stream"] = ue.Stream(s["other"]["token"], f"user:{s['other']['id']}")


# ── Whens ────────────────────────────────────────────────────────────────
@when(parsers.parse('the admin bans the buyer with reason code "{reason}"'))
def admin_bans_buyer(world: World, reason: str) -> None:
    s = _s(world)
    s["before"] = ue.active_session_count(s["buyer"]["token"])
    s["resp"] = ue.set_user_status(world, s["buyer"]["id"], "banned", reason)


@when(parsers.parse('the admin bans the admin\'s own account with reason code "{reason}"'))
def admin_bans_self(world: World, reason: str) -> None:
    s = _s(world)
    s["resp"] = ue.set_user_status(world, s["admin_id"], "banned", reason)


@when(parsers.parse('the admin bans the second admin with reason code "{reason}"'))
def admin_bans_admin2(world: World, reason: str) -> None:
    s = _s(world)
    s["resp"] = ue.set_user_status(world, s["admin2"]["id"], "banned", reason)


@when(parsers.parse('the admin suspends the buyer for {hours:d} hour with reason code "{reason}"'))
def admin_suspends_buyer(world: World, hours: int, reason: str) -> None:
    s = _s(world)
    s["resp"] = ue.set_user_status(
        world, s["buyer"]["id"], "suspended", reason, ue.in_future(hours * 3600)
    )


@when(parsers.parse('the admin suspends the buyer until {sec:d} seconds from now with reason code "{reason}"'))
def admin_suspends_short(world: World, sec: int, reason: str) -> None:
    s = _s(world)
    s["resp"] = ue.set_user_status(world, s["buyer"]["id"], "suspended", reason, ue.in_future(sec))
    assert s["resp"].status_code == 200, f"suspend failed: {s['resp'].status_code} {s['resp'].text}"


@when(parsers.parse('the admin restricts the seller with reason code "{reason}"'))
def admin_restricts_seller(world: World, reason: str) -> None:
    s = _s(world)
    s["resp"] = ue.set_user_status(world, s["seller"]["id"], "restricted", reason)
    assert s["resp"].status_code == 200, f"restrict failed: {s['resp'].status_code} {s['resp'].text}"


@when("team-audit is started again")
def audit_up(world: World) -> None:
    aud.start_audit(world)


@when("the seller calls SetUserStatus on the buyer")
def seller_sets(world: World) -> None:
    s = _s(world)
    s["seller_resp"] = ue.set_user_status(
        world, s["buyer"]["id"], "banned", "scam", token=s["seller"]["token"]
    )


@when("an anonymous caller calls SetUserStatus on the buyer")
def anon_sets(world: World) -> None:
    s = _s(world)
    s["anon_resp"] = ue.call(
        ue.SET_STATUS,
        {"userId": s["buyer"]["id"], "status": "USER_STATUS_BANNED", "reasonCode": "scam"},
    )


@when("the buyer logs in with the correct password")
def buyer_logs_in(world: World) -> None:
    b = _s(world)["buyer"]
    _s(world)["login_resp"] = ue.login_raw(b["username"], b["password"])


@when("the seller logs in with the correct password")
def seller_logs_in(world: World) -> None:
    s = _s(world)
    s["seller"].update(ue.login(s["seller"]))


@when("each of the three logs in with a wrong password")
def wrong_password(world: World) -> None:
    s = _s(world)
    s["wrong"] = {
        k: ue.login_raw(s[k]["username"], "definitely-not-the-password")
        for k in ("active", "suspended", "banned")
    }


@when("an anonymous caller calls ListUsers and SetUserStatus")
def anon_both(world: World) -> None:
    s = _s(world)
    s["both"] = [
        ue.list_users(None),
        ue.call(ue.SET_STATUS, {"userId": s["buyer"]["id"], "status": "USER_STATUS_BANNED",
                                "reasonCode": "scam"}),
    ]


@when("the seller calls ListUsers and SetUserStatus")
def seller_both(world: World) -> None:
    s = _s(world)
    tok = s["seller"]["token"]
    s["both"] = [
        ue.list_users(tok),
        ue.call(ue.SET_STATUS, {"userId": s["admin_id"], "status": "USER_STATUS_BANNED",
                                "reasonCode": "scam"}, tok),
    ]


# ── Thens ────────────────────────────────────────────────────────────────
@then("the ban succeeds and reports as many sessions revoked as the buyer had")
def ban_ok(world: World) -> None:
    s = _s(world)
    resp = s["resp"]
    assert resp.status_code == 200, f"SetUserStatus failed: {resp.status_code} {resp.text}"
    assert s["before"] >= 3, f"the buyer should have had 3 active sessions, had {s['before']}"
    revoked = int(ue.body_of(resp).get("sessionsRevoked", 0))
    assert revoked == s["before"], f"sessionsRevoked={revoked}, active sessions before={s['before']}"


@then(parsers.parse('the admin sees the buyer as "{status}" with reason "{reason}"'))
def buyer_status(world: World, status: str, reason: str) -> None:
    row = ue.stored_user(world, _s(world)["buyer"]["id"])
    assert row.get("status") == f"USER_STATUS_{status.upper()}", row
    assert row.get("statusReason", "") == reason, row


@then(parsers.parse('team-audit holds exactly 1 user.set_status event for the buyer by the admin with status "{status}"'))
def audit_one(world: World, status: str) -> None:
    s = _s(world)
    aud.assert_one_event(
        world, target_type="user", target_id=s["buyer"]["id"], action="user.set_status",
        actor_id=s["admin_id"], metadata={"status": status},
    )


@then("team-audit holds no user.set_status event for the buyer")
def audit_none_buyer(world: World) -> None:
    aud.assert_no_event(
        world, target_type="user", target_id=_s(world)["buyer"]["id"], action="user.set_status"
    )


@then("team-audit holds no user.set_status event for the admin")
def audit_none_admin(world: World) -> None:
    aud.assert_no_event(
        world, target_type="user", target_id=_s(world)["admin_id"], action="user.set_status"
    )


@then("team-audit holds no user.set_status event for the second admin")
def audit_none_admin2(world: World) -> None:
    aud.assert_no_event(
        world, target_type="user", target_id=_s(world)["admin2"]["id"], action="user.set_status"
    )


@then(parsers.parse('the call is refused as "{code}"'))
def call_refused(world: World, code: str) -> None:
    _expect_code(_s(world)["resp"], code)


@then(parsers.parse('the seller call is refused as "{code}"'))
def seller_refused(world: World, code: str) -> None:
    _expect_code(_s(world)["seller_resp"], code)


@then(parsers.parse('the anonymous call is refused as "{code}"'))
def anon_refused(world: World, code: str) -> None:
    _expect_code(_s(world)["anon_resp"], code)


@then(parsers.parse('both calls are refused as "{code}"'))
def both_refused(world: World, code: str) -> None:
    for resp in _s(world)["both"]:
        _expect_code(resp, code)


@then("the buyer still logs in with the correct password")
def buyer_still_logs_in(world: World) -> None:
    b = _s(world)["buyer"]
    resp = ue.login_raw(b["username"], b["password"])
    assert resp.status_code == 200, f"login should succeed: {resp.status_code} {resp.text}"


@then(parsers.parse('suspending the buyer for {hours:d} hour now succeeds'))
def suspend_now_ok(world: World, hours: int) -> None:
    s = _s(world)
    resp = ue.set_user_status(world, s["buyer"]["id"], "suspended", "scam", ue.in_future(hours * 3600))
    assert resp.status_code == 200, f"SetUserStatus failed: {resp.status_code} {resp.text}"
    aud.assert_one_event(
        world, target_type="user", target_id=s["buyer"]["id"], action="user.set_status",
        actor_id=s["admin_id"], metadata={"status": "suspended"},
    )


@then(parsers.parse('the admin can set the buyer to "{status}"'))
def admin_can_set(world: World, status: str) -> None:
    s = _s(world)
    resp = ue.set_user_status(world, s["buyer"]["id"], status, "appeal_accepted")
    assert resp.status_code == 200, f"control SetUserStatus failed: {resp.status_code} {resp.text}"


@then("the admin can still log in")
def admin_logs_in(world: World) -> None:
    from config.settings import get_settings
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    resp = ue.login_raw(admin.username, admin.password)
    assert resp.status_code == 200, f"admin login failed: {resp.status_code} {resp.text}"


@then("the second admin can still log in")
def admin2_logs_in(world: World) -> None:
    a = _s(world)["admin2"]
    resp = ue.login_raw(a["username"], a["password"])
    assert resp.status_code == 200, f"second admin login failed: {resp.status_code} {resp.text}"


@then(parsers.parse('the login is refused as "{code}" with the generic locked message'))
def login_refused(world: World, code: str) -> None:
    _resp_ok(world)
    resp = _s(world)["login_resp"]
    _expect_code(resp, code)
    assert _msg(resp) == ue.LOCKED_MESSAGE, f"message {_msg(resp)!r}"
    assert "scam" not in resp.text, f"refusal leaks the reason code: {resp.text}"


@then("the other active buyer logs in with the correct password successfully")
def other_logs_in(world: World) -> None:
    o = _s(world)["other"]
    resp = ue.login_raw(o["username"], o["password"])
    assert resp.status_code == 200, f"control login failed: {resp.status_code} {resp.text}"


@then("the active buyer logs in with the correct password successfully")
def active_logs_in(world: World) -> None:
    a = _s(world)["active"]
    resp = ue.login_raw(a["username"], a["password"])
    assert resp.status_code == 200, f"control login failed: {resp.status_code} {resp.text}"


@then("the suspended buyer is refused with the correct password")
def suspended_refused(world: World) -> None:
    a = _s(world)["suspended"]
    resp = ue.login_raw(a["username"], a["password"])
    _expect_code(resp, "permission_denied")
    assert _msg(resp) == ue.LOCKED_MESSAGE, _msg(resp)


@then("all three refusals have the identical code and message and none says the account is locked")
def wrong_pw_identical(world: World) -> None:
    results = {k: (r.status_code, ue.code_of(r), _msg(r)) for k, r in _s(world)["wrong"].items()}
    assert len(set(results.values())) == 1, f"wrong-password answers differ by status: {results}"
    status, code, msg = next(iter(results.values()))
    assert status in (401, 403) or code, results
    assert code != "permission_denied" and ue.LOCKED_MESSAGE not in msg, results
    assert code, f"wrong password must be a Connect error: {results}"


@then("the buyer is refused at login while the suspension runs")
def refused_while_suspended(world: World) -> None:
    b = _s(world)["buyer"]
    resp = ue.login_raw(b["username"], b["password"])
    _expect_code(resp, "permission_denied")
    assert _msg(resp) == ue.LOCKED_MESSAGE, _msg(resp)


@then(parsers.parse("within {sec:d} seconds a login with the correct password succeeds"))
def login_after_expiry(world: World, sec: int) -> None:
    b = _s(world)["buyer"]
    ok = ue.wait_until(lambda: ue.login_raw(b["username"], b["password"]).status_code == 200, sec)
    assert ok, "the login never succeeded after the suspension ended"


@then(parsers.parse('exactly 1 UserStatusChanged event has reason code "{reason}" from "{prev}" to "{new}"'))
def lifted_event(world: World, reason: str, prev: str, new: str) -> None:
    s = _s(world)
    events = s["tap"].status_events(s["buyer"]["id"], wait_s=10)
    lifted = [e for e in events if e["reason_code"] == reason]
    assert len(lifted) == 1, f"expected exactly 1 '{reason}' event, got {events}"
    assert lifted[0]["previous_status"] == ue.STATUS_ENUM[prev], lifted
    assert lifted[0]["status"] == ue.STATUS_ENUM[new], lifted


@then("the seller's token carries listing.write")
def seller_has_write(world: World) -> None:
    scopes = set(ue.claims(_s(world)["seller"]["token"]).get("scopes", []))
    assert "listing.write" in scopes, f"a fresh seller must carry listing.write: {scopes}"


@then("the seller's token lacks listing.write but keeps listing.read, engagement:write and search:write")
def seller_restricted_scopes(world: World) -> None:
    scopes = set(ue.claims(_s(world)["seller"]["token"]).get("scopes", []))
    assert "listing.write" not in scopes, f"restricted seller still has listing.write: {scopes}"
    assert {"listing.read", "engagement:write", "search:write"} <= scopes, scopes


@then(parsers.parse('the buyer\'s earlier refresh token is refused as "{code}"'))
def refresh_dead(world: World, code: str) -> None:
    _resp_ok(world)
    b = _s(world)["buyer"]
    assert b["refresh"], "the buyer's login returned no refresh token"
    _expect_code(ue.call(ue.REFRESH, {"refreshToken": b["refresh"]}), code)


@then("the other active buyer's refresh token still works")
def other_refresh_ok(world: World) -> None:
    o = _s(world)["other"]
    resp = ue.call(ue.REFRESH, {"refreshToken": o["refresh"]})
    assert resp.status_code == 200, f"control refresh failed: {resp.status_code} {resp.text}"


@then("the admin can list users")
def admin_lists(world: World) -> None:
    resp = ue.list_users(_s(world)["admin_token"])
    assert resp.status_code == 200, f"admin ListUsers failed: {resp.status_code} {resp.text}"
    assert ue.body_of(resp).get("users"), resp.text[:300]


@then("the buyer's access token is accepted")
def buyer_token_ok(world: World) -> None:
    resp = ue.call(ue.SESSIONS, {}, _s(world)["buyer"]["token"])
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then(parsers.parse("within {sec:d} seconds the buyer's old access token is rejected as unauthenticated"))
def token_rejected(world: World, sec: int) -> None:
    tok = _s(world)["buyer"]["token"]
    assert _s(world)["resp"].status_code == 200, f"the ban itself failed: {_s(world)['resp'].text}"
    got = ue.wait_until(lambda: ue.call(ue.SESSIONS, {}, tok).status_code == 401, sec)
    assert got, "the banned user's access token was still accepted after the deadline"


@then("the other active buyer's access token is still accepted")
def other_token_ok(world: World) -> None:
    resp = ue.call(ue.SESSIONS, {}, _s(world)["other"]["token"])
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then(parsers.parse("within {sec:d} seconds the banned buyer's stream ends"))
def stream_ends(world: World, sec: int) -> None:
    s = _s(world)
    assert s["resp"].status_code == 200, f"the ban itself failed: {s['resp'].text}"
    assert s["stream"].wait_closed(sec), "the banned buyer's stream is still open"


@then("the other active buyer's stream is still open")
def other_stream_open(world: World) -> None:
    assert not _s(world)["other_stream"].closed.is_set(), "the control buyer's stream was cut"
