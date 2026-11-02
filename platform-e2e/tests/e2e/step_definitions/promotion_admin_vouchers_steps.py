"""Steps for promo/promotion_admin_vouchers.feature (change promotion-admin-vouchers).

Black box through the gateway (Connect JSON). Holds use the service principal over grpcurl. Stored
state is read back with GetVoucher and the audit log with QueryAuditLog.
"""

from __future__ import annotations

import re
import subprocess
from datetime import datetime
from pathlib import Path

import httpx
from pytest_bdd import given, parsers, then, when

from src.utils import data as fake
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import audit_support as aud
from tests.e2e.support.oic_order_support import race
from tests.e2e.support.world import World

_ROOT = Path(__file__).resolve().parents[4]
_GAPS = "fe-platform/docs/gaps.md"
_REMOVED_GAPS = {"G9", "G10", "G11"}


def _x(world: World) -> dict:
    return av.state(world)


def _seen(world: World, resp: httpx.Response) -> httpx.Response:
    st = _x(world)
    st["resp"] = resp
    st.setdefault("resps", []).append(resp)
    return resp


def _parse_ts(value: str | None) -> datetime | None:
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def _norm(voucher: dict) -> dict:
    ends = _parse_ts(voucher.get("endsAt"))
    return {
        "id": voucher.get("id"),
        "code": voucher.get("code"),
        "scope": voucher.get("scope"),
        "discountType": voucher.get("discountType"),
        "discountValue": str(voucher.get("discountValue", "0")),
        "minSpend": str(voucher.get("minSpend", "0")),
        "maxDiscount": str(voucher.get("maxDiscount", "0")),
        "quota": str(voucher.get("quota", "0")),
        "used": str(voucher.get("used", "0")),
        "endsAt": int(ends.timestamp()) if ends else None,
        "active": bool(voucher.get("active", False)),
        "deactivatedAt": voucher.get("deactivatedAt") or None,
    }


def _record(world: World) -> dict:
    st = _x(world)
    stored = av.get_voucher(st["admin"], st["code"])
    assert stored, f"voucher {st['code']} not found"
    st["initial"] = _norm(stored)
    st["voucher"] = stored
    return stored


def _token(world: World, role: str) -> str | None:
    st = _x(world)
    if role == "anonymous":
        return None
    assert role in st, f"no {role} token seeded"
    return st[role]


# ── Givens ───────────────────────────────────────────────────────────────
@given("the seeded admin for the voucher admin checks")
def seeded_admin(world: World) -> None:
    st = _x(world)
    st["admin"] = aud.admin_token(world)
    st["admin_id"] = av.vs.token_sub(st["admin"])
    st["since"] = av.iso(offset_s=-5)


@given(parsers.parse("a registered {role} for the voucher admin checks"))
def registered_role(world: World, role: str) -> None:
    token = world.service_factory.auth.register(
        fake.unique_username(f"av_{role}"), world.settings.seed_password, role
    )
    st = _x(world)
    st[role] = token
    st[f"{role}_id"] = av.vs.token_sub(token)


def _new_voucher(world: World, quota: int, used: int, **kw) -> None:
    st = _x(world)
    st["code"] = av.fresh_code()
    voucher = av.create_voucher(st["admin"], st["code"], quota=quota, **kw)
    st["voucher_id"] = voucher["id"]
    if used:
        av.redeem(st["code"], used)
    _record(world)


@given(parsers.parse("an active platform voucher with quota {quota:d} and {used:d} used"))
def active_voucher(world: World, quota: int, used: int) -> None:
    _new_voucher(world, quota, used)


@given("an active platform voucher with unlimited quota")
def unlimited_voucher(world: World) -> None:
    _new_voucher(world, 0, 0)


@given(parsers.parse("an active platform voucher that starts in {s:d} days and ends in {e:d} days"))
def future_voucher(world: World, s: int, e: int) -> None:
    st = _x(world)
    st["start_iso"] = av.iso(days=s)
    _new_voucher(world, 5, 0, starts_at=st["start_iso"], ends_at=av.iso(days=e))


@given("an active platform voucher whose end date has passed")
def expired_voucher(world: World) -> None:
    st = _x(world)
    st["code"] = av.fresh_code()
    st["voucher_id"] = av.create_voucher(st["admin"], st["code"], quota=5)["id"]
    av.backdate_end(st["code"])
    _record(world)


@given("the admin has stopped the voucher")
def admin_has_stopped(world: World) -> None:
    st = _x(world)
    resp = av.deactivate(st["admin"], st["voucher_id"])
    av.voucher_of(resp)
    st["stopped_events"] = av.audit_count(world, st["code"], "voucher.deactivate")
    _record(world)


@given("a control reservation of the voucher is accepted")
def control_accepted(world: World) -> None:
    st = _x(world)
    rid, status, body = av.reserve(st["code"])
    assert status == "ok" and body.get("valid"), f"control reservation refused: {status} {body}"
    assert (
        len(av.vs.reservation_rows(rid)) == 1
    ), "reservation rows are not readable for the control"


@given("a control reservation of the voucher is refused for another reason than inactive")
def control_refused_other(world: World) -> None:
    st = _x(world)
    _, status, body = av.reserve(st["code"])
    assert status == "ok" and not body.get("valid"), f"control should be refused: {status} {body}"
    assert body.get("reason") and body.get("reason") != "inactive", body


@given("an order holds the voucher under a reservation")
def order_holds(world: World) -> None:
    st = _x(world)
    rid, status, body = av.reserve(st["code"])
    assert status == "ok" and body.get("valid"), f"hold refused: {status} {body}"
    st["held"] = rid


@given("team-audit is stopped for the voucher admin checks")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@given("team-promotion is stopped for the voucher admin checks")
def promotion_stopped(world: World) -> None:
    av.stop_promotion(world, _x(world)["admin"])


# ── Whens ────────────────────────────────────────────────────────────────
@when(parsers.parse("the {role} stops the voucher"))
def role_stops(world: World, role: str) -> None:
    st = _x(world)
    _seen(world, av.deactivate(_token(world, role), st["voucher_id"]))


@when("an anonymous caller stops the voucher")
def anon_stops(world: World) -> None:
    _seen(world, av.deactivate(None, _x(world)["voucher_id"]))


@when("the admin stops a voucher with an unknown id")
def admin_stops_unknown(world: World) -> None:
    st = _x(world)
    st["unknown_id"] = "unknown-" + av.fresh_code("")
    _seen(world, av.deactivate(st["admin"], st["unknown_id"]))


@when(parsers.parse("the {role} updates the voucher with quota {quota:d}"))
def role_updates_quota(world: World, role: str, quota: int) -> None:
    st = _x(world)
    _seen(world, av.update(_token(world, role), st["voucher_id"], quota=quota))


@when(parsers.parse("an anonymous caller updates the voucher with quota {quota:d}"))
def anon_updates(world: World, quota: int) -> None:
    _seen(world, av.update(None, _x(world)["voucher_id"], quota=quota))


@when(parsers.parse("the admin updates the voucher with quota {quota:d} while audit is down"))
def update_quota_audit_down(world: World, quota: int) -> None:
    st = _x(world)
    _seen(world, av.update(st["admin"], st["voucher_id"], quota=quota))


@when(parsers.parse("the admin updates the voucher with an end date {offset:d} seconds from now"))
def update_end_seconds(world: World, offset: int) -> None:
    st = _x(world)
    st["sent_end"] = av.iso(offset_s=offset)
    _seen(world, av.update(st["admin"], st["voucher_id"], ends_at=st["sent_end"]))


@when(parsers.parse("the admin updates the voucher with an end date {days:d} days from now"))
def update_end_days(world: World, days: int) -> None:
    st = _x(world)
    st["sent_end"] = av.iso(days=days)
    _seen(world, av.update(st["admin"], st["voucher_id"], ends_at=st["sent_end"]))


@when(
    parsers.parse(
        "the admin updates the voucher with quota {quota:d} and an end date {days:d} days from now"
    )
)
def update_quota_and_end(world: World, quota: int, days: int) -> None:
    st = _x(world)
    st["sent_end"] = av.iso(days=days)
    _seen(world, av.update(st["admin"], st["voucher_id"], quota=quota, ends_at=st["sent_end"]))


@when(
    parsers.parse(
        "the admin updates the voucher with the current quota and an end date {days:d} days from now"
    )
)
def update_current_quota_and_end(world: World, days: int) -> None:
    st = _x(world)
    st["sent_end"] = av.iso(days=days)
    quota = int(st["initial"]["quota"])
    _seen(world, av.update(st["admin"], st["voucher_id"], quota=quota, ends_at=st["sent_end"]))


@when("the admin updates the voucher with the current quota and no end date")
def update_current_quota_only(world: World) -> None:
    st = _x(world)
    _seen(world, av.update(st["admin"], st["voucher_id"], quota=int(st["initial"]["quota"])))


@when("the admin updates the voucher with neither quota nor end date")
def update_nothing(world: World) -> None:
    st = _x(world)
    _seen(world, av.update(st["admin"], st["voucher_id"]))


@when(parsers.parse("the admin updates an unknown voucher id with quota {quota:d}"))
def update_unknown(world: World, quota: int) -> None:
    st = _x(world)
    st["unknown_id"] = "unknown-" + av.fresh_code("")
    _seen(world, av.update(st["admin"], st["unknown_id"], quota=quota))


@when("the admin updates the voucher with an end date equal to the start date")
def update_end_eq_start(world: World) -> None:
    st = _x(world)
    _seen(world, av.update(st["admin"], st["voucher_id"], ends_at=st["start_iso"]))


@when("the admin updates the voucher with an end date one second before the start date")
def update_end_before_start(world: World) -> None:
    st = _x(world)
    start = _parse_ts(st["start_iso"])
    before = datetime.fromtimestamp(start.timestamp() - 1, tz=start.tzinfo)
    _seen(
        world,
        av.update(st["admin"], st["voucher_id"], ends_at=before.strftime("%Y-%m-%dT%H:%M:%SZ")),
    )


@when("the admin creates a platform voucher with the same code")
def create_same_code(world: World) -> None:
    st = _x(world)
    _seen(world, av.gw(st["admin"], "CreateVoucher", av.create_body(st["code"])))


def _create_stop_update(world: World, role: str) -> None:
    st = _x(world)
    token = _token(world, role)
    st["new_code"] = av.fresh_code("AVNEW")
    _seen(world, av.gw(token, "CreateVoucher", av.create_body(st["new_code"])))
    _seen(world, av.deactivate(token, st["voucher_id"]))
    _seen(world, av.update(token, st["voucher_id"], quota=8))


@when("the admin creates a new platform voucher, stops the voucher and updates its quota to 8")
def admin_three_writes(world: World) -> None:
    _create_stop_update(world, "admin")


@when("the buyer creates a platform voucher, stops the voucher and updates its quota to 8")
def buyer_three_writes(world: World) -> None:
    _create_stop_update(world, "buyer")


@when("two admins create the same new platform code at once")
def two_creates(world: World) -> None:
    st = _x(world)
    st["code"] = av.fresh_code("AVRACE")
    body = av.create_body(st["code"])
    results = race([lambda: av.gw(st["admin"], "CreateVoucher", body) for _ in range(2)])
    st["race"] = results
    st["resps"] = list(results)


@when("the buyer previews the voucher through the gateway")
def buyer_previews(world: World) -> None:
    st = _x(world)
    resp = av.gw(
        st["buyer"],
        "ValidateAndReserve",
        {
            "reservationId": f"preview:{st['buyer_id']}:{st['code']}",
            "code": st["code"],
            "buyerId": st["buyer_id"],
            "cartSubtotal": "1000000",
        },
    )
    assert resp.status_code == 200, f"preview: {resp.status_code} {resp.text}"
    st["reservation"] = ("ok", resp.json(), None)


@when("a reservation of the voucher is requested")
def reservation_requested(world: World) -> None:
    st = _x(world)
    rid, status, body = av.reserve(st["code"])
    st["reservation"] = (status, body, rid)


@when("the held reservation is committed")
def held_committed(world: World) -> None:
    st = _x(world)
    st["commit"] = av.commit(st["held"])


@when("the held reservation is released")
def held_released(world: World) -> None:
    st = _x(world)
    st["release"] = av.release(st["held"])


@when("the admin queries the audit log through the gateway")
def query_audit(world: World) -> None:
    st = _x(world)
    resp = av.gw_path(
        st["admin"], "/platform.audit.v1.AuditService/QueryAuditLog", {"page": {"pageSize": 1}}
    )
    _seen(world, resp)


# ── Thens: call outcome ──────────────────────────────────────────────────
@then("the call succeeds")
def call_ok(world: World) -> None:
    resp = _x(world)["resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the call fails with code "{code}"'))
def call_fails(world: World, code: str) -> None:
    resp = _x(world)["resp"]
    assert av.code_of(resp) == code, f"{resp.status_code} {resp.text}"


@then(parsers.parse('every call fails with code "{code}"'))
def every_call_fails(world: World, code: str) -> None:
    resps = _x(world)["resps"]
    assert resps, "no call was made"
    got = [av.code_of(r) for r in resps]
    assert got == [code] * len(resps), f"{got}: {[r.text for r in resps]}"


@then(parsers.parse('the calls fail with "{a}", "{b}" and "{c}" respectively'))
def calls_fail_in_order(world: World, a: str, b: str, c: str) -> None:
    got = [av.code_of(r) for r in _x(world)["resps"]]
    assert got == [a, b, c], f"{got}: {[r.text for r in _x(world)['resps']]}"


@then(parsers.parse('the gateway answers with Connect code "{want}" and not "{other}"'))
def gateway_code(world: World, want: str, other: str) -> None:
    resp = _x(world)["resp"]
    got = av.code_of(resp)
    assert got == want and got != other, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the error message says "{text}"'))
def error_message_says(world: World, text: str) -> None:
    msg = av.message_of(_x(world)["resp"])
    assert text in msg.lower(), f"message {msg!r} lacks {text!r}"


@then("the error message names the code field")
def error_names_code(world: World) -> None:
    msg = av.message_of(_x(world)["resp"])
    assert re.search(r"\bcode\b", msg.lower()), f"message {msg!r} does not name the code field"


@then(parsers.parse('the error carries reason "{reason}" and domain "{domain}"'))
def error_reason(world: World, reason: str, domain: str) -> None:
    resp = _x(world)["resp"]
    info = av.error_info(resp)
    assert info.get("reason") == reason and info.get("domain") == domain, f"{info}: {resp.text}"


@then(parsers.parse('every error carries reason "{reason}" and domain "{domain}"'))
def every_error_reason(world: World, reason: str, domain: str) -> None:
    for resp in _x(world)["resps"]:
        info = av.error_info(resp)
        assert info.get("reason") == reason and info.get("domain") == domain, f"{info}: {resp.text}"


@then("the error carries no ErrorInfo reason")
def error_no_reason(world: World) -> None:
    resp = _x(world)["resp"]
    assert not av.error_info(resp).get("reason"), resp.text


@then('the error message is the generic "service unavailable" text')
def error_generic(world: World) -> None:
    msg = av.message_of(_x(world)["resp"])
    assert msg == av.GENERIC_UNAVAILABLE, f"message {msg!r}"


@then("the error message does not contain the upstream audit text")
def error_not_raw(world: World) -> None:
    msg = av.message_of(_x(world)["resp"]).lower()
    assert msg == av.GENERIC_UNAVAILABLE and "audit" not in msg, f"message {msg!r}"


@then("exactly one of the calls succeeds")
def exactly_one(world: World) -> None:
    results = _x(world)["race"]
    ok = [r for r in results if r.status_code == 200]
    assert len(ok) == 1, [(r.status_code, r.text) for r in results]


@then(parsers.parse('the failing call returns "{want}", never "{other}"'))
def failing_call(world: World, want: str, other: str) -> None:
    failed = [r for r in _x(world)["race"] if r.status_code != 200]
    assert len(failed) == 1, [(r.status_code, r.text) for r in _x(world)["race"]]
    got = av.code_of(failed[0])
    assert got == want and got != other, failed[0].text


# ── Thens: returned and stored voucher ───────────────────────────────────
@then("the returned voucher is inactive with a deactivated_at")
def returned_inactive(world: World) -> None:
    voucher = av.voucher_of(_x(world)["resp"])
    assert voucher.get("active") is not True, voucher
    assert voucher.get("deactivatedAt"), voucher


@then("the returned voucher keeps the original deactivated_at")
def returned_same_deactivated(world: World) -> None:
    st = _x(world)
    voucher = av.voucher_of(st["resp"])
    assert voucher.get("deactivatedAt"), voucher
    assert _parse_ts(voucher["deactivatedAt"]) == _parse_ts(st["initial"]["deactivatedAt"]), (
        voucher,
        st["initial"],
    )


@then(parsers.parse("the returned voucher has quota {quota:d}"))
def returned_quota(world: World, quota: int) -> None:
    voucher = av.voucher_of(_x(world)["resp"])
    assert str(voucher.get("quota", "0")) == str(quota), voucher


@then("the returned voucher has the new end date")
def returned_new_end(world: World) -> None:
    st = _x(world)
    voucher = av.voucher_of(st["resp"])
    got, want = _parse_ts(voucher.get("endsAt")), _parse_ts(st["sent_end"])
    assert got and want and abs((got - want).total_seconds()) < 1, (voucher, st["sent_end"])


@then("the returned voucher keeps its money fields and end date")
def returned_money_and_end(world: World) -> None:
    st = _x(world)
    got = _norm(av.voucher_of(st["resp"]))
    for key in (*av.MONEY_FIELDS, "endsAt"):
        assert got[key] == st["initial"][key], (key, got, st["initial"])


@then("the returned voucher keeps its money fields")
def returned_money(world: World) -> None:
    st = _x(world)
    got = _norm(av.voucher_of(st["resp"]))
    for key in av.MONEY_FIELDS:
        assert got[key] == st["initial"][key], (key, got, st["initial"])


@then("the stored voucher keeps its money fields")
def stored_money(world: World) -> None:
    st = _x(world)
    got = _norm(av.get_voucher(st["admin"], st["code"]) or {})
    for key in av.MONEY_FIELDS:
        assert got[key] == st["initial"][key], (key, got, st["initial"])


@then("the stored voucher is unchanged")
def stored_unchanged(world: World) -> None:
    st = _x(world)
    got = _norm(av.get_voucher(st["admin"], st["code"]) or {})
    assert got == st["initial"], f"changed: {got} vs {st['initial']}"


@then(parsers.parse("the stored voucher has quota {quota:d}"))
def stored_quota(world: World, quota: int) -> None:
    st = _x(world)
    got = av.get_voucher(st["admin"], st["code"]) or {}
    assert str(got.get("quota", "0")) == str(quota), got


@then("the admin can still stop the voucher")
def control_stop(world: World) -> None:
    st = _x(world)
    resp = av.deactivate(st["admin"], st["voucher_id"])
    assert resp.status_code == 200, f"positive control failed: {resp.status_code} {resp.text}"


@then(parsers.parse("the admin can still update the voucher with quota {quota:d}"))
def control_update_quota(world: World, quota: int) -> None:
    st = _x(world)
    resp = av.update(st["admin"], st["voucher_id"], quota=quota)
    assert resp.status_code == 200, f"positive control failed: {resp.status_code} {resp.text}"


@then("the admin can still update the voucher with an end date")
def control_update_end(world: World) -> None:
    st = _x(world)
    resp = av.update(st["admin"], st["voucher_id"], ends_at=av.iso(days=45))
    assert resp.status_code == 200, f"positive control failed: {resp.status_code} {resp.text}"


@then("the buyer's new voucher does not exist")
def buyer_voucher_absent(world: World) -> None:
    st = _x(world)
    assert av.get_voucher(st["admin"], st["new_code"]) is None, "a buyer created a platform voucher"


# ── Thens: audit ─────────────────────────────────────────────────────────
@then(parsers.parse('the audit log holds exactly {n:d} "{action}" event for the voucher'))
def audit_exactly(world: World, n: int, action: str) -> None:
    st = _x(world)
    assert av.audit_count(world, st["code"], action) == n, aud.events_for(
        world, "voucher", st["code"]
    )


@then(parsers.parse('the audit log holds no "{action}" event for the voucher'))
def audit_none(world: World, action: str) -> None:
    st = _x(world)
    assert av.audit_count(world, st["code"], action) == 0, aud.events_for(
        world, "voucher", st["code"]
    )


@then(parsers.parse('the audit log holds exactly one more "{action}" event for the voucher'))
def audit_one_more(world: World, action: str) -> None:
    st = _x(world)
    assert av.audit_count(world, st["code"], action) == st["stopped_events"] + 1, aud.events_for(
        world, "voucher", st["code"]
    )


@then(parsers.parse('no "{action}" audit entry mentions the unknown voucher id'))
def audit_unknown(world: World, action: str) -> None:
    st = _x(world)
    found = av.audit_mentions(world, action, st["unknown_id"], st["since"])
    assert not found, found


@then("after team-audit is started again nothing was created or changed")
def audit_down_nothing_changed(world: World) -> None:
    st = _x(world)
    aud.start_audit(world)
    if st.get("new_code"):
        assert (
            av.get_voucher(st["admin"], st["new_code"]) is None
        ), "voucher created while audit was down"
    got = _norm(av.get_voucher(st["admin"], st["code"]) or {})
    assert got == st["initial"], f"changed while audit was down: {got} vs {st['initial']}"


# ── Thens: reservations ──────────────────────────────────────────────────
@then(parsers.parse('the last reservation is refused with reason "{reason}"'))
def reservation_refused(world: World, reason: str) -> None:
    status, body, _ = _x(world)["reservation"]
    assert status == "ok" and body.get("valid") is not True, f"{status} {body}"
    assert body.get("reason") == reason, body


@then("the last reservation is accepted")
def reservation_accepted(world: World) -> None:
    status, body, _ = _x(world)["reservation"]
    assert status == "ok" and body.get("valid") is True, f"{status} {body}"


@then("no reservation row exists for the refused reservation")
def no_reservation_row(world: World) -> None:
    _, _, rid = _x(world)["reservation"]
    assert rid, "no reservation id recorded"
    assert av.vs.reservation_rows(rid) == [], av.vs.reservation_rows(rid)


@then(parsers.parse("the commit reports committed and the voucher used count is {used:d}"))
def commit_reported(world: World, used: int) -> None:
    st = _x(world)
    status, body = st["commit"]
    assert status == "ok" and body.get("committed") is True, f"{status} {body}"
    assert str((av.get_voucher(st["admin"], st["code"]) or {}).get("used", "0")) == str(used)


@then(parsers.parse("the release reports released and the voucher used count is {used:d}"))
def release_reported(world: World, used: int) -> None:
    st = _x(world)
    status, body = st["release"]
    assert status == "ok" and body.get("released") is True, f"{status} {body}"
    assert str((av.get_voucher(st["admin"], st["code"]) or {}).get("used", "0")) == str(used)


# ── docs ─────────────────────────────────────────────────────────────────
def _rows(text: str) -> dict[str, str]:
    rows: dict[str, str] = {}
    for line in text.splitlines():
        m = re.match(r"^\|\s*(G\d+)\s*\|", line)
        if m:
            rows[m.group(1)] = line
    return rows


def _base_text() -> str:
    proc = subprocess.run(
        ["git", "-C", str(_ROOT), "show", f"origin/main:{_GAPS}"],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert proc.returncode == 0, proc.stderr
    return proc.stdout


@given("the gaps document of the change")
def gaps_doc(world: World) -> None:
    st = _x(world)
    st["gaps_now"] = (_ROOT / _GAPS).read_text("utf-8")
    st["gaps_base"] = _base_text()


@then("the gaps document has no row whose id is G9, G10 or G11")
def gaps_removed(world: World) -> None:
    st = _x(world)
    assert _REMOVED_GAPS <= set(
        _rows(st["gaps_base"])
    ), "control: the base document must list G9, G10, G11"
    assert not (_REMOVED_GAPS & set(_rows(st["gaps_now"]))), sorted(
        _REMOVED_GAPS & set(_rows(st["gaps_now"]))
    )


@then("the gaps document differs from the base only by the removal of rows G9, G10 and G11")
def gaps_removed_only(world: World) -> None:
    st = _x(world)
    base, now = _rows(st["gaps_base"]), _rows(st["gaps_now"])
    assert set(base) - set(now) == _REMOVED_GAPS, sorted(set(base) - set(now))
    assert not set(now) - set(base), sorted(set(now) - set(base))


@then("every other row is byte-identical and every row has the table format of the header")
def gaps_identical(world: World) -> None:
    st = _x(world)
    base, now = _rows(st["gaps_base"]), _rows(st["gaps_now"])
    for gid, line in now.items():
        assert base.get(gid) == line, f"row {gid} changed"
    lines = st["gaps_now"].splitlines()
    header = next(line for line in lines if line.startswith("| ID"))
    cells = header.count("|")
    for line in lines:
        if line.startswith("|"):
            assert line.count("|") == cells, f"malformed table row: {line[:80]}"
