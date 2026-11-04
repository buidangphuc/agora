"""Steps for the hand-off code scenarios (change identity-sso-handoff).

API steps drive the gateway; handler steps drive the buyer and seller Next apps over plain HTTP
(no redirect following) so headers, cookies and Location are asserted exactly; browser steps drive
the real pages. Persisted state is read back from identity_db (read only) and QueryAuditLog.
"""

from __future__ import annotations

import json
import os
import re
import time
import uuid
from datetime import datetime
from urllib.parse import parse_qs, quote, unquote, urlparse

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.core.base_page import wait_streamed
from tests.e2e.support import audit_support as aud
from tests.e2e.support import handoff_support as ho
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

SETTINGS = get_settings()
CONFIRM_TEXT = "Tiếp tục với tài khoản"
SELLER_COOKIES = ("seller_session", "seller_refresh")


def X(world: World) -> dict:
    return world.state.extra


def user(world: World) -> dict:
    return X(world)["u"]


def _remember_mint(world: World, out: ho.Outcome) -> None:
    x = X(world)
    stamp = out.date or time.time()
    x.setdefault("first_mint_at", stamp)
    x["minted_at"] = stamp


def _fresh(world: World, app: str, owner: dict | None = None) -> str:
    owner = owner or user(world)
    out = ho.mint(owner["token"], app)
    _remember_mint(world, out)
    code = ho.minted_code(out)
    X(world)["code"] = code
    X(world)["code_app"] = app
    return code


def _new_session_owner(world: World, role: str, username: str | None = None) -> dict:
    if username is None:
        owner = ho.new_user(role)
    else:
        password = SETTINGS.seed_password
        resp = ue.call(ue.REGISTER, {"username": username, "password": password, "role": role})
        owner = {"username": username, "password": password, "role": role}
        if resp.status_code != 200:
            assert ue.login_raw(username, password).status_code == 200, f"cannot sign in {username[:20]}"
        owner.update(ue.login(owner))
    obs = ue.login(owner)
    x = X(world)
    x["u"] = owner
    x["obs"] = obs["token"]
    x["baseline"] = ue.active_session_count(obs["token"])
    return owner


def _path(location: str) -> str:
    parsed = urlparse(location)
    return parsed.path + (f"?{parsed.query}" if parsed.query else "")


def _expect_failure(out: ho.Outcome, code: str) -> None:
    assert not out.ok, f"expected failure {code}, call succeeded: {out.body}"
    assert out.code == code, f"expected {code}, got {out.status} {out.code} {out.message!r}"


def _headers_from(spec: str) -> dict[str, str]:
    headers: dict[str, str] = {}
    for part in [p.strip() for p in spec.split(",") if p.strip()]:
        name, _, value = part.partition(":")
        headers[name.strip()] = value.strip()
    return headers


def _form_for(world: World, app: str, code: str, next_path: str) -> dict[str, str]:
    x = X(world)
    key = (app, code, next_path)
    if x.get("fields_key") == key:
        return dict(x["fields"])
    fields = ho.confirmation_fields(app, code, next_path)
    x["fields_key"] = key
    x["fields"] = dict(fields)
    return dict(fields)


def _code_field(fields: dict[str, str], code: str) -> str:
    for name, value in fields.items():
        if value == code:
            return name
    return "code"


def _assert_no_cookie(resp: httpx.Response) -> None:
    assert not ho.session_cookies_set(resp), f"no cookie expected, got {list(ho.cookies_set(resp))}"


def _redeemable(code: str, app: str = ho.SELLER) -> bool:
    return ho.redeem(code, app).ok


def _expires(out_body: dict) -> float:
    raw = out_body.get("expiresAt")
    assert raw, f"expiresAt missing: {out_body}"
    return datetime.fromisoformat(raw.replace("Z", "+00:00")).timestamp()


# ── users and codes ──────────────────────────────────────────────────────────
@given("a signed-in buyer")
@when("a signed-in buyer")
@then("a signed-in buyer")
def signed_in_buyer(world: World) -> None:
    _new_session_owner(world, "buyer")


def _settle(page) -> None:
    page.wait_for_load_state("load")
    wait_streamed(page)


@given(parsers.parse('a signed-in buyer named mallory with a fresh seller-target code'))
@when(parsers.parse('a signed-in buyer named mallory with a fresh seller-target code'))
@then(parsers.parse('a signed-in buyer named mallory with a fresh seller-target code'))
def mallory(world: World) -> None:
    owner = _new_session_owner(world, "buyer")
    _fresh(world, ho.SELLER, owner)


@given("a signed-in buyer who is a seller on the buyer app")
@when("a signed-in buyer who is a seller on the buyer app")
@then("a signed-in buyer who is a seller on the buyer app")
def seller_on_buyer_app(world: World) -> None:
    owner = _new_session_owner(world, "seller")
    world.context.add_cookies([{"name": "session", "value": owner["token"], "url": SETTINGS.base_url}])
    X(world)["buyer_session_cookie"] = owner["token"]


@given("a signed-in seller on the seller app")
@when("a signed-in seller on the seller app")
@then("a signed-in seller on the seller app")
def seller_on_seller_app(world: World) -> None:
    owner = _new_session_owner(world, "seller")
    world.context.add_cookies([{"name": "seller_session", "value": owner["token"], "url": SETTINGS.seller_base_url}])


@given(parsers.parse('a signed-in user named with markup "{name}"'))
@when(parsers.parse('a signed-in user named with markup "{name}"'))
@then(parsers.parse('a signed-in user named with markup "{name}"'))
def markup_user(world: World, name: str) -> None:
    _new_session_owner(world, "buyer", name + uuid.uuid4().hex[:8])


@given(parsers.parse("a signed-in user named with {n:d} characters"))
@when(parsers.parse("a signed-in user named with {n:d} characters"))
@then(parsers.parse("a signed-in user named with {n:d} characters"))
def long_user(world: World, n: int) -> None:
    _new_session_owner(world, "buyer", "u" * (n - 10) + uuid.uuid4().hex[:10])


@given("a signed-in seller named bob who holds seller cookies")
@when("a signed-in seller named bob who holds seller cookies")
@then("a signed-in seller named bob who holds seller cookies")
def bob(world: World) -> None:
    owner = ho.new_user("seller", "bob")
    X(world)["bob"] = owner
    X(world)["bob_cookies"] = {"seller_session": owner["token"], "seller_refresh": owner["refresh"]}


@given("a fresh seller-target code of the buyer")
@when("a fresh seller-target code of the buyer")
@then("a fresh seller-target code of the buyer")
def fresh_seller_code(world: World) -> None:
    _fresh(world, ho.SELLER)


@given("a fresh buyer-target code of the buyer")
@when("a fresh buyer-target code of the buyer")
@then("a fresh buyer-target code of the buyer")
def fresh_buyer_code(world: World) -> None:
    _fresh(world, ho.BUYER)


@given(parsers.parse("the buyer has minted {n:d} live codes"))
@when(parsers.parse("the buyer has minted {n:d} live codes"))
@then(parsers.parse("the buyer has minted {n:d} live codes"))
@given(parsers.parse("the buyer has minted {n:d} more live codes"))
@when(parsers.parse("the buyer has minted {n:d} more live codes"))
@then(parsers.parse("the buyer has minted {n:d} more live codes"))
def mint_live(world: World, n: int) -> None:
    for _ in range(n):
        _remember_mint(world, out := ho.mint(user(world)["token"]))
        ho.minted_code(out)


@given(parsers.parse("the buyer has minted {n:d} codes that are left to expire"))
@when(parsers.parse("the buyer has minted {n:d} codes that are left to expire"))
@then(parsers.parse("the buyer has minted {n:d} codes that are left to expire"))
def mint_to_expire(world: World, n: int) -> None:
    mint_live(world, n)


@given(parsers.parse("the buyer has minted and redeemed {n:d} codes"))
@when(parsers.parse("the buyer has minted and redeemed {n:d} codes"))
@then(parsers.parse("the buyer has minted and redeemed {n:d} codes"))
def mint_and_redeem(world: World, n: int) -> None:
    for _ in range(n):
        code = _fresh(world, ho.SELLER)
        ho.result_of(ho.redeem(code))


@given(parsers.parse("{s:d} seconds have passed since the first mint"))
@when(parsers.parse("{s:d} seconds have passed since the first mint"))
@then(parsers.parse("{s:d} seconds have passed since the first mint"))
def wait_since_first(world: World, s: int) -> None:
    ho.wait_until_age(X(world)["first_mint_at"], s)


@given(parsers.parse('an eleventh mint by the buyer is refused with "{code}"'))
@when(parsers.parse('an eleventh mint by the buyer is refused with "{code}"'))
@then(parsers.parse('an eleventh mint by the buyer is refused with "{code}"'))
def eleventh_refused(world: World, code: str) -> None:
    _expect_failure(ho.mint(user(world)["token"]), code)


@given("an admin suspends the buyer")
@when("an admin suspends the buyer")
@then("an admin suspends the buyer")
def suspend(world: World) -> None:
    resp = ue.set_user_status(world, user(world)["id"], "suspended", "scam", ue.in_future(3600))
    assert resp.status_code == 200, f"suspend failed: {resp.status_code} {resp.text[:200]}"


@given("team-audit is stopped")
@when("team-audit is stopped")
@then("team-audit is stopped")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@given("team-audit is started again")
@when("team-audit is started again")
@then("team-audit is started again")
def audit_started(world: World) -> None:
    aud.start_audit(world)


@given("the source session is logged out")
@when("the source session is logged out")
@then("the source session is logged out")
def source_logout(world: World) -> None:
    ho.revoke_source(user(world))


# ── mint ─────────────────────────────────────────────────────────────────────
@given("the buyer mints a code for the seller app")
@when("the buyer mints a code for the seller app")
@then("the buyer mints a code for the seller app")
@given("the buyer mints a code for the seller app with the still-valid access token")
@when("the buyer mints a code for the seller app with the still-valid access token")
@then("the buyer mints a code for the seller app with the still-valid access token")
def mint_seller(world: World) -> None:
    out = ho.mint(user(world)["token"])
    _remember_mint(world, out)
    X(world)["out"] = out
    if out.ok:
        X(world)["code"] = out.body.get("code", "")


@given("the buyer mints a code with an unspecified target")
@when("the buyer mints a code with an unspecified target")
@then("the buyer mints a code with an unspecified target")
def mint_unspecified(world: World) -> None:
    X(world)["out"] = ho.mint(user(world)["token"], ho.UNSPECIFIED)


@given("an anonymous caller mints a code for the seller app")
@when("an anonymous caller mints a code for the seller app")
@then("an anonymous caller mints a code for the seller app")
def mint_anonymous(world: World) -> None:
    X(world)["out"] = ho.mint(None)


@given(parsers.parse("the buyer mints {n:d} codes at once"))
@when(parsers.parse("the buyer mints {n:d} codes at once"))
@then(parsers.parse("the buyer mints {n:d} codes at once"))
def mint_concurrently(world: World, n: int) -> None:
    token = user(world)["token"]
    X(world)["outs"] = ho.concurrently(n, lambda: ho.mint(token))


@given("the mint succeeds with a 43-character base64url code")
@when("the mint succeeds with a 43-character base64url code")
@then("the mint succeeds with a 43-character base64url code")
def mint_ok(world: World) -> None:
    out = X(world)["out"]
    assert out.ok, f"mint failed: {out.status} {out.body}"
    assert ho.CODE_RE.match(out.body.get("code", "")), f"bad code shape: {out.body}"


@given("the code expires 60 seconds after the gateway clock")
@when("the code expires 60 seconds after the gateway clock")
@then("the code expires 60 seconds after the gateway clock")
def ttl_60(world: World) -> None:
    out = X(world)["out"]
    assert out.date, "gateway sent no Date header"
    delta = _expires(out.body) - out.date
    assert abs(delta - ho.TTL_S) <= 2, f"expires_at is {delta}s after the gateway clock"


@given("the hand-off table holds one row keyed by the SHA-256 of the code and no row contains the plaintext")
@when("the hand-off table holds one row keyed by the SHA-256 of the code and no row contains the plaintext")
@then("the hand-off table holds one row keyed by the SHA-256 of the code and no row contains the plaintext")
def table_hash_only(world: World) -> None:
    code = X(world)["code"]
    assert ho.rows_with_hash(code) == 1
    assert ho.rows_containing_plaintext(code) == 0


@given("the hand-off table holds one row for the buyer")
@when("the hand-off table holds one row for the buyer")
@then("the hand-off table holds one row for the buyer")
def one_row(world: World) -> None:
    assert ho.rows_for(user(world)["id"]) == 1


@given("the hand-off table holds no row for the buyer")
@when("the hand-off table holds no row for the buyer")
@then("the hand-off table holds no row for the buyer")
def no_row(world: World) -> None:
    assert ho.rows_for(user(world)["id"]) == 0


@given(parsers.parse("the hand-off table holds exactly {n:d} rows for the buyer"))
@when(parsers.parse("the hand-off table holds exactly {n:d} rows for the buyer"))
@then(parsers.parse("the hand-off table holds exactly {n:d} rows for the buyer"))
@given(parsers.parse("the hand-off table holds exactly {n:d} row for the buyer"))
@when(parsers.parse("the hand-off table holds exactly {n:d} row for the buyer"))
@then(parsers.parse("the hand-off table holds exactly {n:d} row for the buyer"))
def n_rows(world: World, n: int) -> None:
    assert ho.rows_for(user(world)["id"]) == n


@given(parsers.parse('team-audit holds one "{action}" entry for the buyer'))
@when(parsers.parse('team-audit holds one "{action}" entry for the buyer'))
@then(parsers.parse('team-audit holds one "{action}" entry for the buyer'))
def one_entry(world: World, action: str) -> None:
    found = ho.audit_entries(world, user(world)["id"], action)
    assert len(found) == 1, f"expected one {action} entry, got {found}"
    X(world)["entry"] = found[0]


@given(parsers.parse('team-audit holds no "{action}" entry for the buyer'))
@when(parsers.parse('team-audit holds no "{action}" entry for the buyer'))
@then(parsers.parse('team-audit holds no "{action}" entry for the buyer'))
def no_entry(world: World, action: str) -> None:
    assert ho.audit_entries(world, user(world)["id"], action) == []


@given(parsers.parse('team-audit holds exactly {n:d} "{action}" entries for the buyer'))
@when(parsers.parse('team-audit holds exactly {n:d} "{action}" entries for the buyer'))
@then(parsers.parse('team-audit holds exactly {n:d} "{action}" entries for the buyer'))
@given(parsers.parse('team-audit holds exactly {n:d} "{action}" entry for the buyer'))
@when(parsers.parse('team-audit holds exactly {n:d} "{action}" entry for the buyer'))
@then(parsers.parse('team-audit holds exactly {n:d} "{action}" entry for the buyer'))
def n_entries(world: World, n: int, action: str) -> None:
    assert len(ho.audit_entries(world, user(world)["id"], action)) == n


def _entry_is_clean(world: World) -> dict:
    entry = X(world)["entry"]
    blob = json.dumps(entry.get("metadata") or {})
    from tests.e2e.flows import session_id_of

    for secret in ho.forbidden_forms(X(world)["code"], "", session_id_of(user(world)["token"])):
        if secret:
            assert secret not in blob, f"audit metadata leaks a secret: {blob}"
    return entry


@given("that entry names the seller app and carries no code, hash or sid")
@when("that entry names the seller app and carries no code, hash or sid")
@then("that entry names the seller app and carries no code, hash or sid")
def entry_names_seller(world: World) -> None:
    entry = _entry_is_clean(world)
    values = [str(v).lower() for v in (entry.get("metadata") or {}).values()]
    assert any("seller" in v for v in values), f"target app missing from metadata: {entry}"


@given('that entry names the seller app with result "ok" and carries no code, hash or sid')
@when('that entry names the seller app with result "ok" and carries no code, hash or sid')
@then('that entry names the seller app with result "ok" and carries no code, hash or sid')
def entry_redeem(world: World) -> None:
    entry = _entry_is_clean(world)
    values = [str(v).lower() for v in (entry.get("metadata") or {}).values()]
    assert any("seller" in v for v in values), f"target app missing from metadata: {entry}"
    assert "ok" in values, f"result ok missing from metadata: {entry}"
    assert entry.get("actorId") == user(world)["id"]


@given(parsers.parse('the call fails with "{code}"'))
@when(parsers.parse('the call fails with "{code}"'))
@then(parsers.parse('the call fails with "{code}"'))
def call_fails(world: World, code: str) -> None:
    _expect_failure(X(world)["out"], code)


@given(parsers.parse('the last call is 401 "{code}"'))
@when(parsers.parse('the last call is 401 "{code}"'))
@then(parsers.parse('the last call is 401 "{code}"'))
def last_call_401(world: World, code: str) -> None:
    out = X(world)["out"]
    assert out.status == 401 and out.code == code, f"got {out.status} {out.code} {out.message!r}"


@given("a mint for the seller app by the same buyer then succeeds")
@when("a mint for the seller app by the same buyer then succeeds")
@then("a mint for the seller app by the same buyer then succeeds")
def mint_control(world: World) -> None:
    ho.minted_code(ho.mint(user(world)["token"]))


@given(parsers.parse('a mint for the seller app by the same buyer then fails with "{code}"'))
@when(parsers.parse('a mint for the seller app by the same buyer then fails with "{code}"'))
@then(parsers.parse('a mint for the seller app by the same buyer then fails with "{code}"'))
def mint_then_fails(world: World, code: str) -> None:
    _expect_failure(ho.mint(user(world)["token"]), code)


@given("exactly 10 mints succeed and 10 fail with \"resource_exhausted\"")
@when("exactly 10 mints succeed and 10 fail with \"resource_exhausted\"")
@then("exactly 10 mints succeed and 10 fail with \"resource_exhausted\"")
def concurrent_split(world: World) -> None:
    outs = X(world)["outs"]
    assert sum(o.ok for o in outs) == 10, [o.code or o.status for o in outs]
    assert sum(o.code == "resource_exhausted" for o in outs) == 10


@given(parsers.parse("the buyer has exactly {n:d} live codes"))
@when(parsers.parse("the buyer has exactly {n:d} live codes"))
@then(parsers.parse("the buyer has exactly {n:d} live codes"))
def live_codes(world: World, n: int) -> None:
    assert ho.live_rows_for(user(world)["id"]) == n


# ── peek ─────────────────────────────────────────────────────────────────────
@given(parsers.parse("the code is peeked {n:d} times"))
@when(parsers.parse("the code is peeked {n:d} times"))
@then(parsers.parse("the code is peeked {n:d} times"))
@given(parsers.parse("the code is peeked {n:d} time"))
@when(parsers.parse("the code is peeked {n:d} time"))
@then(parsers.parse("the code is peeked {n:d} time"))
def peek_n(world: World, n: int) -> None:
    x = X(world)
    x["peeks"] = [ho.peek(x["code"], x.get("code_app", ho.SELLER)) for _ in range(n)]


@given("every peek returns the buyer's username and an expiry")
@when("every peek returns the buyer's username and an expiry")
@then("every peek returns the buyer's username and an expiry")
def peeks_ok(world: World) -> None:
    for out in X(world)["peeks"]:
        assert out.ok, f"peek failed: {out.status} {out.body}"
        assert out.body.get("username") == user(world)["username"]
        assert out.body.get("expiresAt")


@given("the peek returns the buyer's username")
@when("the peek returns the buyer's username")
@then("the peek returns the buyer's username")
def peek_username(world: World) -> None:
    out = X(world).get("out") or X(world)["peeks"][-1]
    assert out.ok and out.body.get("username") == user(world)["username"], f"{out.status} {out.body}"


@given("the code then redeems successfully")
@when("the code then redeems successfully")
@then("the code then redeems successfully")
@given("the code redeems successfully within its TTL")
@when("the code redeems successfully within its TTL")
@then("the code redeems successfully within its TTL")
@given("the code still redeems through the gateway")
@when("the code still redeems through the gateway")
@then("the code still redeems through the gateway")
def code_redeems(world: World) -> None:
    out = ho.redeem(X(world)["code"], X(world).get("code_app", ho.SELLER))
    assert out.ok, f"redeem failed: {out.status} {out.body}"


@given("the code no longer redeems through the gateway")
@when("the code no longer redeems through the gateway")
@then("the code no longer redeems through the gateway")
def code_gone(world: World) -> None:
    out = ho.redeem(X(world)["code"])
    assert out.status == 401, f"expected 401 for a consumed code, got {out.status}"


@given("the code is still unredeemed in the table")
@when("the code is still unredeemed in the table")
@then("the code is still unredeemed in the table")
@given("the code stays unredeemed in the table")
@when("the code stays unredeemed in the table")
@then("the code stays unredeemed in the table")
def unredeemed(world: World) -> None:
    assert ho.redeemed_at_is_null(X(world)["code"])


@given("the code is redeemed")
@when("the code is redeemed")
@then("the code is redeemed")
def redeemed(world: World) -> None:
    deadline = time.time() + 10
    while time.time() < deadline:
        if not ho.redeemed_at_is_null(X(world)["code"]):
            return
        time.sleep(0.3)
    raise AssertionError("code was never redeemed")


@given("the code redeems for the buyer app as a control")
@when("the code redeems for the buyer app as a control")
@then("the code redeems for the buyer app as a control")
def buyer_control(world: World) -> None:
    assert ho.redeem(X(world)["code"], ho.BUYER).ok


@given("the buyer's audit entry count is noted")
@when("the buyer's audit entry count is noted")
@then("the buyer's audit entry count is noted")
def note_audit(world: World) -> None:
    X(world)["audit_before"] = ho.audit_total(world, user(world)["id"])


@given("the buyer's audit entry count is unchanged")
@when("the buyer's audit entry count is unchanged")
@then("the buyer's audit entry count is unchanged")
def audit_unchanged(world: World) -> None:
    assert ho.audit_total(world, user(world)["id"]) == X(world)["audit_before"]


@given("an anonymous caller peeks the code")
@when("an anonymous caller peeks the code")
@then("an anonymous caller peeks the code")
def peek_anonymous(world: World) -> None:
    x = X(world)
    x["out"] = ho.peek(x["code"])


@given("the same peek with a garbage bearer token is also answered")
@when("the same peek with a garbage bearer token is also answered")
@then("the same peek with a garbage bearer token is also answered")
def peek_garbage_token(world: World) -> None:
    out = ho.rpc(ho.PEEK, {"code": X(world)["code"], "targetApp": ho.SELLER}, "not-a-jwt")
    assert out.status != 500, f"gateway failed on garbage bearer: {out.status} {out.body}"


# ── invalid code families ────────────────────────────────────────────────────
@given(parsers.re(r"the invalid codes: (?P<names>.+)"))
@when(parsers.re(r"the invalid codes: (?P<names>.+)"))
@then(parsers.re(r"the invalid codes: (?P<names>.+)"))
def invalid_codes(world: World, names: str) -> None:
    wanted = [n.strip() for n in re.split(r",| and ", names) if n.strip()]
    owner = user(world)
    codes: dict[str, str] = {}
    expired_minted_at = 0.0
    if "expired" in wanted:
        out = ho.mint(owner["token"])
        expired_minted_at = out.date or time.time()
        codes["expired"] = ho.minted_code(out)
    for name in wanted:
        if name == "unknown":
            codes[name] = uuid.uuid4().hex + uuid.uuid4().hex[:11]
        elif name == "used":
            codes[name] = ho.mint_code(owner)
            ho.result_of(ho.redeem(codes[name]))
        elif name == "wrong-target":
            codes[name] = ho.mint_code(owner, ho.BUYER)
        elif name == "malformed":
            codes[name] = "short"
        elif name == "revoked-source":
            other = ho.new_user("buyer", "hoother")
            codes[name] = ho.mint_code(other)
            ho.revoke_source(other)
    if expired_minted_at:
        ho.wait_until_age(expired_minted_at, 61)
    X(world)["invalid"] = codes


@given("each invalid code is peeked")
@when("each invalid code is peeked")
@then("each invalid code is peeked")
def peek_each(world: World) -> None:
    X(world)["outs"] = [ho.peek(c) for c in X(world)["invalid"].values()]


@given("each invalid code is redeemed")
@when("each invalid code is redeemed")
@then("each invalid code is redeemed")
def redeem_each(world: World) -> None:
    X(world)["outs"] = [ho.redeem(c) for c in X(world)["invalid"].values()]


@given('every response is 401 "unauthenticated" with one identical message')
@when('every response is 401 "unauthenticated" with one identical message')
@then('every response is 401 "unauthenticated" with one identical message')
def identical_failures(world: World) -> None:
    outs = X(world)["outs"]
    assert len(outs) >= 5
    assert {(o.status, o.code) for o in outs} == {(401, "unauthenticated")}, [(o.status, o.code) for o in outs]
    assert len({o.message for o in outs}) == 1, {o.message for o in outs}
    assert outs[0].message, "failure message must be a fixed non-empty string"


@given("the valid code is peeked successfully as a control")
@when("the valid code is peeked successfully as a control")
@then("the valid code is peeked successfully as a control")
def peek_control(world: World) -> None:
    assert ho.peek(X(world)["code"]).ok


@given("the valid code redeems successfully as a control")
@when("the valid code redeems successfully as a control")
@then("the valid code redeems successfully as a control")
def redeem_control(world: World) -> None:
    assert ho.redeem(X(world)["code"]).ok


@given("another buyer's fresh code still redeems as a control")
@when("another buyer's fresh code still redeems as a control")
@then("another buyer's fresh code still redeems as a control")
def other_control(world: World) -> None:
    other = ho.new_user("buyer", "hoctl")
    assert ho.redeem(ho.mint_code(other)).ok


# ── redeem ───────────────────────────────────────────────────────────────────
@given("the code is redeemed for the seller app")
@when("the code is redeemed for the seller app")
@then("the code is redeemed for the seller app")
@given("an anonymous caller redeems the code through the gateway")
@when("an anonymous caller redeems the code through the gateway")
@then("an anonymous caller redeems the code through the gateway")
def redeem_seller(world: World) -> None:
    x = X(world)
    out = ho.redeem(x["code"])
    x["out"] = out
    if out.ok:
        x["derived"] = out.body.get("result") or {}


@given("the code is redeemed for the seller app again")
@when("the code is redeemed for the seller app again")
@then("the code is redeemed for the seller app again")
def redeem_again(world: World) -> None:
    X(world)["out"] = ho.redeem(X(world)["code"])


@given("the redeem returns an access token and a refresh token")
@when("the redeem returns an access token and a refresh token")
@then("the redeem returns an access token and a refresh token")
def redeem_tokens(world: World) -> None:
    out = X(world)["out"]
    result = ho.result_of(out)
    assert result.get("token") and result.get("refreshToken"), f"tokens missing: {list(result)}"


@given("the new session's sid differs from the source sid")
@when("the new session's sid differs from the source sid")
@then("the new session's sid differs from the source sid")
def sid_differs(world: World) -> None:
    from tests.e2e.flows import session_id_of

    derived = session_id_of(X(world)["derived"]["token"])
    assert derived and derived != session_id_of(user(world)["token"])


@given("the source session still refreshes normally")
@when("the source session still refreshes normally")
@then("the source session still refreshes normally")
def source_refreshes(world: World) -> None:
    assert ho.refresh(user(world)["refresh"]).ok


@given("the derived session still refreshes successfully")
@when("the derived session still refreshes successfully")
@then("the derived session still refreshes successfully")
def derived_refreshes(world: World) -> None:
    out = ho.refresh(X(world)["derived"]["refreshToken"])
    assert out.ok, f"derived session cannot refresh: {out.status} {out.body}"


@given(parsers.parse("the code is redeemed {n:d} times at once"))
@when(parsers.parse("the code is redeemed {n:d} times at once"))
@then(parsers.parse("the code is redeemed {n:d} times at once"))
def redeem_concurrently(world: World, n: int) -> None:
    code = X(world)["code"]
    X(world)["outs"] = ho.concurrently(n, lambda: ho.redeem(code))


@given(parsers.parse('exactly 1 redeem succeeds and {n:d} fail with "unauthenticated"'))
@when(parsers.parse('exactly 1 redeem succeeds and {n:d} fail with "unauthenticated"'))
@then(parsers.parse('exactly 1 redeem succeeds and {n:d} fail with "unauthenticated"'))
def one_winner(world: World, n: int) -> None:
    outs = X(world)["outs"]
    assert sum(o.ok for o in outs) == 1, [o.code or o.status for o in outs]
    losers = [o for o in outs if not o.ok]
    assert len(losers) == n and all(o.code == "unauthenticated" for o in losers)
    X(world)["losers"] = losers


@given(parsers.parse("the {n:d} losers each answered within {ms:d} ms"))
@when(parsers.parse("the {n:d} losers each answered within {ms:d} ms"))
@then(parsers.parse("the {n:d} losers each answered within {ms:d} ms"))
def losers_fast(world: World, n: int, ms: int) -> None:
    slow = [round(o.elapsed_s * 1000) for o in X(world)["losers"] if o.elapsed_s * 1000 > ms]
    assert not slow, f"losers slower than {ms} ms: {slow}"


@given(parsers.parse("the buyer's active sessions changed by {delta:d}"))
@when(parsers.parse("the buyer's active sessions changed by {delta:d}"))
@then(parsers.parse("the buyer's active sessions changed by {delta:d}"))
def sessions_delta(world: World, delta: int) -> None:
    now = ue.active_session_count(X(world)["obs"])
    assert now - X(world)["baseline"] == delta, f"active sessions {X(world)['baseline']} -> {now}, want {delta:+d}"


@given("log capture is started for identity and gateway")
@when("log capture is started for identity and gateway")
@then("log capture is started for identity and gateway")
def log_capture(world: World) -> None:
    time.sleep(1.2)
    X(world)["log_since"] = time.time()
    time.sleep(1.2)


@given("the captured logs contain neither the code, its hash, the user id nor the sid")
@when("the captured logs contain neither the code, its hash, the user id nor the sid")
@then("the captured logs contain neither the code, its hash, the user id nor the sid")
def logs_clean(world: World) -> None:
    from tests.e2e.flows import session_id_of

    x = X(world)
    identity = os.getenv("IDENTITY_CONTAINER", "team-identity-svc")
    gateway = os.getenv("GATEWAY_CONTAINER", "team-gateway-svc")
    logs = ho.container_logs(identity, x["log_since"]) + ho.container_logs(gateway, x["log_since"])
    for secret in ho.forbidden_forms(x["code"], user(world)["id"], session_id_of(user(world)["token"])):
        assert secret not in logs, f"a secret of {len(secret)} chars appears in the logs"


# ── handler over HTTP ────────────────────────────────────────────────────────
@given(parsers.parse('the {app} app receives GET "/auth/handoff" for the code with next "{next_path}"'))
@when(parsers.parse('the {app} app receives GET "/auth/handoff" for the code with next "{next_path}"'))
@then(parsers.parse('the {app} app receives GET "/auth/handoff" for the code with next "{next_path}"'))
def get_handoff(world: World, app: str, next_path: str) -> None:
    x = X(world)
    with ho.app_client() as client:
        x["resp"] = client.get(ho.handoff_url(app, x["code"], next_path))
    if app == "seller" and x["resp"].status_code == 200:
        forms = [f for f in ho.parse_page(x["resp"].text).forms if f["method"] == "post"]
        if forms:
            x["fields_key"] = (app, x["code"], next_path)
            x["fields"] = dict(forms[0]["fields"])


@given("the seller app receives a HEAD and a prefetch GET for the code")
@when("the seller app receives a HEAD and a prefetch GET for the code")
@then("the seller app receives a HEAD and a prefetch GET for the code")
def head_and_prefetch(world: World) -> None:
    url = ho.handoff_url("seller", X(world)["code"], "/seller")
    with ho.app_client() as client:
        client.head(url)
        client.get(url, headers={"Purpose": "prefetch", "Sec-Purpose": "prefetch", "X-Purpose": "prefetch"})


@given(parsers.parse('the response is 200 containing "{text}" and the buyer\'s username'))
@when(parsers.parse('the response is 200 containing "{text}" and the buyer\'s username'))
@then(parsers.parse('the response is 200 containing "{text}" and the buyer\'s username'))
def page_names_buyer(world: World, text: str) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.headers.get('location')}"
    assert f"{text} {user(world)['username']}?" in ho.page_text(resp.text), ho.page_text(resp.text)[:300]


@given("the page holds a POST form and sets no cookie")
@when("the page holds a POST form and sets no cookie")
@then("the page holds a POST form and sets no cookie")
def page_form_no_cookie(world: World) -> None:
    resp = X(world)["resp"]
    assert any(f["method"] == "post" for f in ho.parse_page(resp.text).forms)
    assert not ho.cookies_set(resp), f"unexpected cookies: {list(ho.cookies_set(resp))}"


def _submit(world: World, next_path: str, headers: dict[str, str], host: str | None = None) -> None:
    x = X(world)
    fields = _form_for(world, "seller", x["code"], next_path)
    if "next" in fields:
        fields["next"] = next_path
    sent = dict(headers)
    if host:
        sent["Host"] = host
    x["resp"] = ho.post_handoff("seller", fields, sent)


@given(parsers.parse('the confirmation form of the seller app is submitted from its own origin with next "{next_path}"'))
@when(parsers.parse('the confirmation form of the seller app is submitted from its own origin with next "{next_path}"'))
@then(parsers.parse('the confirmation form of the seller app is submitted from its own origin with next "{next_path}"'))
def submit_own_origin(world: World, next_path: str) -> None:
    _submit(world, next_path, ho.own_origin_headers("seller"))


@given(parsers.parse('the confirmation form is posted with headers "{spec}"'))
@when(parsers.parse('the confirmation form is posted with headers "{spec}"'))
@then(parsers.parse('the confirmation form is posted with headers "{spec}"'))
def post_with_headers(world: World, spec: str) -> None:
    _submit(world, "/seller", _headers_from(spec))


@given('the confirmation form is posted with headers ""')
@when('the confirmation form is posted with headers ""')
@then('the confirmation form is posted with headers ""')
def post_without_headers(world: World) -> None:
    _submit(world, "/seller", {})


@given(parsers.parse('the confirmation form is posted with next "{next_path}" with headers "{spec}"'))
@when(parsers.parse('the confirmation form is posted with next "{next_path}" with headers "{spec}"'))
@then(parsers.parse('the confirmation form is posted with next "{next_path}" with headers "{spec}"'))
def post_with_next_headers(world: World, next_path: str, spec: str) -> None:
    _submit(world, next_path, _headers_from(spec))


@given(parsers.parse('the confirmation form with next "{next_path}" is posted with headers "{spec}"'))
@when(parsers.parse('the confirmation form with next "{next_path}" is posted with headers "{spec}"'))
@then(parsers.parse('the confirmation form with next "{next_path}" is posted with headers "{spec}"'))
def post_next_headers(world: World, next_path: str, spec: str) -> None:
    _submit(world, next_path, _headers_from(spec))


@given(parsers.parse('the confirmation form is posted claiming host "{host}" with headers "{spec}"'))
@when(parsers.parse('the confirmation form is posted claiming host "{host}" with headers "{spec}"'))
@then(parsers.parse('the confirmation form is posted claiming host "{host}" with headers "{spec}"'))
def post_host(world: World, host: str, spec: str) -> None:
    _submit(world, "/seller", _headers_from(spec), host)


@given(parsers.parse('the response is a 303 to "{location}"'))
@when(parsers.parse('the response is a 303 to "{location}"'))
@then(parsers.parse('the response is a 303 to "{location}"'))
def is_303(world: World, location: str) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 303, f"expected 303, got {resp.status_code}"
    assert unquote(_path(ho.location_of(resp))) == location, ho.location_of(resp)


@given(parsers.parse('the response is a 303 to "{location}" with no cookie'))
@when(parsers.parse('the response is a 303 to "{location}" with no cookie'))
@then(parsers.parse('the response is a 303 to "{location}" with no cookie'))
def is_303_no_cookie(world: World, location: str) -> None:
    is_303(world, location)
    _assert_no_cookie(X(world)["resp"])


@given(parsers.parse('the 303 Location is exactly "{location}"'))
@when(parsers.parse('the 303 Location is exactly "{location}"'))
@then(parsers.parse('the 303 Location is exactly "{location}"'))
def location_exact(world: World, location: str) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 303 and _path(ho.location_of(resp)) == location, ho.location_of(resp)


@given("the response sets seller_session and seller_refresh cookies")
@when("the response sets seller_session and seller_refresh cookies")
@then("the response sets seller_session and seller_refresh cookies")
def sets_seller_cookies(world: World) -> None:
    cookies = ho.cookies_set(X(world)["resp"])
    for name in SELLER_COOKIES:
        assert cookies.get(name), f"{name} not set, got {list(cookies)}"


@given(parsers.parse('the response has Referrer-Policy "{value}"'))
@when(parsers.parse('the response has Referrer-Policy "{value}"'))
@then(parsers.parse('the response has Referrer-Policy "{value}"'))
def referrer_policy(world: World, value: str) -> None:
    assert X(world)["resp"].headers.get("referrer-policy") == value


@given("the Location contains no code")
@when("the Location contains no code")
@then("the Location contains no code")
def location_no_code(world: World) -> None:
    location = ho.location_of(X(world)["resp"])
    assert X(world)["code"] not in location and "code=" not in location


@given("the response headers frame-deny, no-store and no-referrer are present")
@when("the response headers frame-deny, no-store and no-referrer are present")
@then("the response headers frame-deny, no-store and no-referrer are present")
def hardening_headers(world: World) -> None:
    h = X(world)["resp"].headers
    assert "frame-ancestors 'none'" in h.get("content-security-policy", ""), h.get("content-security-policy")
    assert h.get("x-frame-options", "").upper() == "DENY"
    assert "no-store" in h.get("cache-control", "")
    assert h.get("referrer-policy") == "same-origin"


@given(parsers.parse('the form action is "{action}" with no query'))
@when(parsers.parse('the form action is "{action}" with no query'))
@then(parsers.parse('the form action is "{action}" with no query'))
def form_action(world: World, action: str) -> None:
    forms = [f for f in ho.parse_page(X(world)["resp"].text).forms if f["method"] == "post"]
    assert forms and forms[0]["action"] == action, forms


@given("the code is a hidden field of the form")
@when("the code is a hidden field of the form")
@then("the code is a hidden field of the form")
def code_hidden(world: World) -> None:
    forms = [f for f in ho.parse_page(X(world)["resp"].text).forms if f["method"] == "post"]
    assert X(world)["code"] in forms[0]["fields"].values()
    assert X(world)["code"] not in forms[0]["action"]


@given("the page references no external script, style, image or font")
@when("the page references no external script, style, image or font")
@then("the page references no external script, style, image or font")
def no_external(world: World) -> None:
    html = X(world)["resp"].text
    assert ho.parse_page(html).external == []
    assert not re.search(r"url\(\s*['\"]?(https?:)?//", html)


@given(parsers.parse('the HTML holds "{escaped}" and no executable script element for the name'))
@when(parsers.parse('the HTML holds "{escaped}" and no executable script element for the name'))
@then(parsers.parse('the HTML holds "{escaped}" and no executable script element for the name'))
def escaped(world: World, escaped: str) -> None:
    html = X(world)["resp"].text
    assert X(world)["resp"].status_code == 200
    assert escaped in html
    assert "<script>alert(1)</script>" not in html


@given(parsers.parse("the displayed name is at most {n:d} characters"))
@when(parsers.parse("the displayed name is at most {n:d} characters"))
@then(parsers.parse("the displayed name is at most {n:d} characters"))
def displayed_name(world: World, n: int) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 200
    match = re.search(rf"{CONFIRM_TEXT} (.*?)\?", ho.page_text(resp.text))
    assert match, ho.page_text(resp.text)[:200]
    assert 0 < len(match.group(1)) <= n, len(match.group(1))


@given("bob opens the hand-off link of the buyer with his seller cookies")
@when("bob opens the hand-off link of the buyer with his seller cookies")
@then("bob opens the hand-off link of the buyer with his seller cookies")
def bob_opens(world: World) -> None:
    x = X(world)
    with ho.app_client() as client:
        x["resp"] = client.get(
            ho.handoff_url("seller", x["code"], "/seller"), headers={"Cookie": ho.cookie_header(x["bob_cookies"])}
        )


@given(parsers.parse('the page reads "{text}" and the buyer\'s username'))
@when(parsers.parse('the page reads "{text}" and the buyer\'s username'))
@then(parsers.parse('the page reads "{text}" and the buyer\'s username'))
def page_reads(world: World, text: str) -> None:
    page_names_buyer(world, text)


@given("bob confirms from the seller app's own origin")
@when("bob confirms from the seller app's own origin")
@then("bob confirms from the seller app's own origin")
def bob_confirms(world: World) -> None:
    x = X(world)
    fields = ho.confirmation_fields("seller", x["code"], "/seller", x["bob_cookies"])
    x["resp"] = ho.post_handoff("seller", fields, ho.own_origin_headers("seller"), x["bob_cookies"])


@given("the seller cookies now belong to the buyer and no longer to bob")
@when("the seller cookies now belong to the buyer and no longer to bob")
@then("the seller cookies now belong to the buyer and no longer to bob")
def cookies_replaced(world: World) -> None:
    x = X(world)
    cookies = ho.cookies_set(x["resp"])
    assert cookies.get("seller_session"), f"no seller_session set: {list(cookies)}"
    claims = ho.jwt_claims(cookies["seller_session"])
    assert claims["sub"] == user(world)["id"]
    assert claims["sub"] != x["bob"]["id"]


@given(parsers.parse('a POST from the seller app\'s own origin carries an unknown code with next "{next_path}"'))
@when(parsers.parse('a POST from the seller app\'s own origin carries an unknown code with next "{next_path}"'))
@then(parsers.parse('a POST from the seller app\'s own origin carries an unknown code with next "{next_path}"'))
def post_unknown(world: World, next_path: str) -> None:
    x = X(world)
    fields = _form_for(world, "seller", x["code"], next_path)
    fields[_code_field(fields, x["code"])] = uuid.uuid4().hex + uuid.uuid4().hex[:11]
    if "next" in fields:
        fields["next"] = next_path
    x["resp"] = ho.post_handoff("seller", fields, ho.own_origin_headers("seller"))


@given('the seller app receives GET without a code')
@when('the seller app receives GET without a code')
@then('the seller app receives GET without a code')
def get_no_code(world: World) -> None:
    with ho.app_client() as client:
        X(world)["resp"] = client.get(f"{ho.app_origin('seller')}/auth/handoff?next=/seller")


@given("the seller app receives GET with a 65-character code")
@when("the seller app receives GET with a 65-character code")
@then("the seller app receives GET with a 65-character code")
def get_oversized(world: World) -> None:
    with ho.app_client() as client:
        X(world)["resp"] = client.get(ho.handoff_url("seller", "a" * 65, "/seller"))


@given("the response is a 303 to a login path with no cookie")
@when("the response is a 303 to a login path with no cookie")
@then("the response is a 303 to a login path with no cookie")
def login_no_cookie(world: World) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 303, f"expected 303, got {resp.status_code}"
    assert urlparse(ho.location_of(resp)).path == "/login", ho.location_of(resp)
    _assert_no_cookie(resp)


@given("the valid code opens the confirmation page as a control")
@when("the valid code opens the confirmation page as a control")
@then("the valid code opens the confirmation page as a control")
def confirmation_control(world: World) -> None:
    with ho.app_client() as client:
        resp = client.get(ho.handoff_url("seller", X(world)["code"], "/seller"))
    assert resp.status_code == 200 and CONFIRM_TEXT in ho.page_text(resp.text)


@given("the redirect target is a local path and never an external URL")
@when("the redirect target is a local path and never an external URL")
@then("the redirect target is a local path and never an external URL")
def local_target(world: World) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 303, f"expected 303, got {resp.status_code}"
    location = ho.location_of(resp)
    parsed = urlparse(location)
    assert "evil.example" not in location
    assert parsed.netloc in ("", urlparse(ho.app_origin("seller")).netloc), location
    assert parsed.path.startswith("/") and not parsed.path.startswith("//"), location


@given("each invalid code is opened with GET on the seller app with next \"/seller/orders\"")
@when("each invalid code is opened with GET on the seller app with next \"/seller/orders\"")
@then("each invalid code is opened with GET on the seller app with next \"/seller/orders\"")
def open_each(world: World) -> None:
    results = []
    with ho.app_client() as client:
        for code in X(world)["invalid"].values():
            results.append(client.get(ho.handoff_url("seller", code, "/seller/orders")))
    X(world)["resps"] = results


@given('every response is a 303 to "/login?next=/seller/orders" with no cookie and no-referrer')
@when('every response is a 303 to "/login?next=/seller/orders" with no cookie and no-referrer')
@then('every response is a 303 to "/login?next=/seller/orders" with no cookie and no-referrer')
def every_login(world: World) -> None:
    for resp in X(world)["resps"]:
        assert resp.status_code == 303, f"expected 303, got {resp.status_code}"
        assert unquote(_path(ho.location_of(resp))) == "/login?next=/seller/orders", ho.location_of(resp)
        _assert_no_cookie(resp)
        assert resp.headers.get("referrer-policy") == "same-origin"


@given(parsers.parse('the admin app receives GET "/auth/handoff" for the code with next "{next_path}"'))
@when(parsers.parse('the admin app receives GET "/auth/handoff" for the code with next "{next_path}"'))
@then(parsers.parse('the admin app receives GET "/auth/handoff" for the code with next "{next_path}"'))
def admin_get(world: World, next_path: str) -> None:
    get_handoff(world, "admin", next_path)


@given(parsers.parse("the response is {status:d}"))
@when(parsers.parse("the response is {status:d}"))
@then(parsers.parse("the response is {status:d}"))
def status_is(world: World, status: int) -> None:
    assert X(world)["resp"].status_code == status


# ── access logs of the buyer app ─────────────────────────────────────────────
def _buyer_get(world: World, code: str, cookies: dict[str, str] | None) -> None:
    x = X(world)
    marker = f"/x-{uuid.uuid4().hex[:12]}"
    x["marker"] = marker
    x["log_since"] = time.time() - 2
    headers = {"Cookie": ho.cookie_header(cookies)} if cookies else {}
    with ho.app_client() as client:
        x["resp"] = client.get(f"{ho.app_origin('buyer')}/auth/handoff?code={code}&next={marker}", headers=headers)


def _frontend_logs(world: World, wait_for_marker: bool = True) -> str:
    x = X(world)
    container = os.getenv("FRONTEND_CONTAINER", "team-frontend-svc")
    deadline = time.time() + 10
    logs = ""
    while time.time() < deadline:
        logs = ho.container_logs(container, x["log_since"])
        if quote(x["marker"], safe="") in logs or not wait_for_marker:
            break
        time.sleep(0.5)
    return logs


@given('the buyer app receives GET "/auth/handoff" for the code with a marker next and the buyer\'s session')
@when('the buyer app receives GET "/auth/handoff" for the code with a marker next and the buyer\'s session')
@then('the buyer app receives GET "/auth/handoff" for the code with a marker next and the buyer\'s session')
def buyer_get_session(world: World) -> None:
    _buyer_get(world, X(world)["code"], {"session": user(world)["token"]})


@given('the buyer app receives GET "/auth/handoff" for the code with a marker next and no session')
@when('the buyer app receives GET "/auth/handoff" for the code with a marker next and no session')
@then('the buyer app receives GET "/auth/handoff" for the code with a marker next and no session')
def buyer_get_anon(world: World) -> None:
    _buyer_get(world, X(world)["code"], None)


@given('the buyer app receives GET "/auth/handoff" for the invalid code "BAD" with a marker next')
@when('the buyer app receives GET "/auth/handoff" for the invalid code "BAD" with a marker next')
@then('the buyer app receives GET "/auth/handoff" for the invalid code "BAD" with a marker next')
def buyer_get_bad(world: World) -> None:
    code = "BADCODE" + uuid.uuid4().hex[:10]
    X(world)["bad"] = code
    _buyer_get(world, code, None)


@given("the buyer app answers with a 303 to login")
@when("the buyer app answers with a 303 to login")
@then("the buyer app answers with a 303 to login")
def buyer_303(world: World) -> None:
    resp = X(world)["resp"]
    assert resp.status_code == 303 and urlparse(ho.location_of(resp)).path == "/login", f"{resp.status_code}"


@given("no buyer app log line contains the code")
@when("no buyer app log line contains the code")
@then("no buyer app log line contains the code")
def no_code_in_logs(world: World) -> None:
    assert X(world)["code"] not in _frontend_logs(world)


@given("no buyer app log line contains the invalid code")
@when("no buyer app log line contains the invalid code")
@then("no buyer app log line contains the invalid code")
def no_bad_in_logs(world: World) -> None:
    assert X(world)["bad"] not in _frontend_logs(world)


def _marker_lines(world: World) -> list[str]:
    marker = quote(X(world)["marker"], safe="")
    lines = [ln for ln in _frontend_logs(world).splitlines() if marker in ln]
    assert lines, f"no buyer app log line mentions the request ({marker})"
    return lines


@given('the buyer app log line for the marker shows "code=REDACTED"')
@when('the buyer app log line for the marker shows "code=REDACTED"')
@then('the buyer app log line for the marker shows "code=REDACTED"')
def shows_redacted(world: World) -> None:
    assert any("code=REDACTED" in ln for ln in _marker_lines(world)), _marker_lines(world)


@given("the buyer app log line for the marker still shows the marker next value")
@when("the buyer app log line for the marker still shows the marker next value")
@then("the buyer app log line for the marker still shows the marker next value")
def shows_next(world: World) -> None:
    marker = quote(X(world)["marker"], safe="")
    assert any(f"next={marker}" in ln for ln in _marker_lines(world)), _marker_lines(world)


# ── browser ──────────────────────────────────────────────────────────────────
def _confirm_button(world: World):  # noqa: ANN202
    return world.page.locator("form button, form input[type=submit]").first


def _login_form(page):  # noqa: ANN001, ANN202
    return page.locator("input[type=password]").first


def _seller_cookies(world: World) -> dict[str, str]:
    return {c["name"]: c["value"] for c in world.context.cookies(SETTINGS.seller_base_url)}


@given("the buyer app renders the home page")
@when("the buyer app renders the home page")
@then("the buyer app renders the home page")
def buyer_renders(world: World) -> None:
    resp = world.page.goto(f"{SETTINGS.base_url}/")
    _settle(world.page)
    X(world)["html"] = resp.text() if resp else world.page.content()


@given("the rendered HTML contains no hand-off code")
@when("the rendered HTML contains no hand-off code")
@then("the rendered HTML contains no hand-off code")
def html_no_code(world: World) -> None:
    html = X(world)["html"]
    assert "/auth/handoff" not in html or "code=" not in html
    assert not re.search(r"code=[A-Za-z0-9_-]{43}", html)


@given("the buyer clicks the seller channel link")
@when("the buyer clicks the seller channel link")
@then("the buyer clicks the seller channel link")
def click_seller_channel(world: World) -> None:
    page = world.page
    if SETTINGS.base_url not in page.url:
        page.goto(f"{SETTINGS.base_url}/")
        _settle(page)
    target = page.get_by_text(re.compile("Kênh Người Bán", re.I)).first
    expect(target).to_be_visible(timeout=15_000)
    target.click()
    page.wait_for_url(re.compile(re.escape(SETTINGS.seller_base_url)), timeout=20_000)


@given("the browser reaches the seller app confirmation page")
@when("the browser reaches the seller app confirmation page")
@then("the browser reaches the seller app confirmation page")
def seller_confirmation(world: World) -> None:
    expect(world.page.get_by_text(re.compile(CONFIRM_TEXT))).to_be_visible(timeout=15_000)
    assert "/auth/handoff" in world.page.url


@given("the browser reaches the buyer app confirmation page")
@when("the browser reaches the buyer app confirmation page")
@then("the browser reaches the buyer app confirmation page")
def buyer_confirmation(world: World) -> None:
    expect(world.page.get_by_text(re.compile(CONFIRM_TEXT))).to_be_visible(timeout=15_000)


@given("the buyer clicks the confirmation button")
@when("the buyer clicks the confirmation button")
@then("the buyer clicks the confirmation button")
def click_confirm(world: World) -> None:
    _confirm_button(world).click()
    _settle(world.page)


@given("the browser ends on a seller app page without a login form")
@when("the browser ends on a seller app page without a login form")
@then("the browser ends on a seller app page without a login form")
def ends_on_seller(world: World) -> None:
    page = world.page
    page.wait_for_url(lambda url: url.startswith(SETTINGS.seller_base_url) and "/auth/handoff" not in url, timeout=20_000)
    assert "/login" not in page.url, page.url
    expect(_login_form(page)).to_have_count(0)


@given("the seller_session cookie is set and the URL holds no code")
@when("the seller_session cookie is set and the URL holds no code")
@then("the seller_session cookie is set and the URL holds no code")
def seller_cookie_set(world: World) -> None:
    assert _seller_cookies(world).get("seller_session")
    assert "code=" not in world.page.url


@given("the seller clicks the link to the buyer storefront")
@when("the seller clicks the link to the buyer storefront")
@then("the seller clicks the link to the buyer storefront")
def click_buyer_storefront(world: World) -> None:
    page = world.page
    page.goto(f"{SETTINGS.seller_base_url}/seller")
    _settle(page)
    target = page.get_by_role("link", name=re.compile("mua sắm|người mua|storefront|cửa hàng|trang chủ", re.I)).first
    expect(target).to_be_visible(timeout=15_000)
    target.click()
    page.wait_for_url(re.compile(re.escape(SETTINGS.base_url)), timeout=20_000)


@given("the browser ends on the buyer app signed in")
@when("the browser ends on the buyer app signed in")
@then("the browser ends on the buyer app signed in")
def ends_on_buyer(world: World) -> None:
    page = world.page
    page.wait_for_url(lambda url: url.startswith(SETTINGS.base_url) and "/auth/handoff" not in url, timeout=20_000)
    expect(page.get_by_role("button", name="Đăng xuất")).to_be_visible(timeout=15_000)


@given("the session cookie is set and the URL holds no code")
@when("the session cookie is set and the URL holds no code")
@then("the session cookie is set and the URL holds no code")
def buyer_cookie_set(world: World) -> None:
    names = {c["name"] for c in world.context.cookies(SETTINGS.base_url)}
    assert "session" in names
    assert "code=" not in world.page.url


@given("the hand-off link is opened and confirmed in a browser")
@when("the hand-off link is opened and confirmed in a browser")
@then("the hand-off link is opened and confirmed in a browser")
def open_and_confirm(world: World) -> None:
    x = X(world)
    world.page.goto(ho.handoff_url("seller", x["code"], "/seller"))
    expect(world.page.get_by_text(re.compile(CONFIRM_TEXT))).to_be_visible(timeout=15_000)
    _confirm_button(world).click()
    _settle(world.page)


@given("the same hand-off URL is opened in a fresh browser context")
@when("the same hand-off URL is opened in a fresh browser context")
@then("the same hand-off URL is opened in a fresh browser context")
def open_fresh_context(world: World) -> None:
    x = X(world)
    ctx = world.context.browser.new_context()
    world._cleanups.append(ctx.close)
    page = ctx.new_page()
    page.goto(ho.handoff_url("seller", x["code"], "/seller"))
    _settle(page)
    x["fresh_page"], x["fresh_ctx"] = page, ctx


@given("the hand-off URL is opened in a browser")
@when("the hand-off URL is opened in a browser")
@then("the hand-off URL is opened in a browser")
@given("the hand-off URL is opened in a browser on the seller app")
@when("the hand-off URL is opened in a browser on the seller app")
@then("the hand-off URL is opened in a browser on the seller app")
def open_in_browser(world: World) -> None:
    x = X(world)
    world.page.goto(ho.handoff_url("seller", x["code"], "/seller"))
    _settle(world.page)
    x["fresh_page"], x["fresh_ctx"] = world.page, world.context


@given("the seller app shows its login form and sets no seller_session cookie")
@when("the seller app shows its login form and sets no seller_session cookie")
@then("the seller app shows its login form and sets no seller_session cookie")
def shows_login(world: World) -> None:
    x = X(world)
    expect(_login_form(x["fresh_page"])).to_be_visible(timeout=15_000)
    names = {c["name"]: c["value"] for c in x["fresh_ctx"].cookies(SETTINGS.seller_base_url)}
    assert not names.get("seller_session"), "seller_session must not be set"


@given("a victim's browser opens the link on the seller app")
@when("a victim's browser opens the link on the seller app")
@then("a victim's browser opens the link on the seller app")
def victim_opens(world: World) -> None:
    x = X(world)
    world.page.goto(ho.handoff_url("seller", x["code"], "/seller"))
    _settle(world.page)


@given("the page names mallory and the browser holds no seller session cookie")
@when("the page names mallory and the browser holds no seller session cookie")
@then("the page names mallory and the browser holds no seller session cookie")
def names_mallory(world: World) -> None:
    expect(world.page.get_by_text(f"{CONFIRM_TEXT} {user(world)['username']}?")).to_be_visible(timeout=15_000)
    assert not _seller_cookies(world).get("seller_session")


@given("the victim clicks the confirmation button")
@when("the victim clicks the confirmation button")
@then("the victim clicks the confirmation button")
def victim_clicks(world: World) -> None:
    click_confirm(world)


@given(parsers.parse('the browser opens the seller app login with next "{next_path}"'))
@when(parsers.parse('the browser opens the seller app login with next "{next_path}"'))
@then(parsers.parse('the browser opens the seller app login with next "{next_path}"'))
def opens_login(world: World, next_path: str) -> None:
    page = world.page
    page.wait_for_url(re.compile(re.escape(SETTINGS.seller_base_url) + r"/login"), timeout=20_000)
    assert parse_qs(urlparse(page.url).query).get("next") == [next_path], page.url
    expect(_login_form(page)).to_be_visible(timeout=15_000)
    assert "code=" not in page.url


@given("the user logs out of the seller app")
@when("the user logs out of the seller app")
@then("the user logs out of the seller app")
def seller_logout(world: World) -> None:
    button = world.page.get_by_role("button", name=re.compile("Đăng xuất"))
    expect(button.first).to_be_visible(timeout=15_000)
    button.first.click()
    _settle(world.page)


@given("the buyer app session cookie is unchanged and still signed in")
@when("the buyer app session cookie is unchanged and still signed in")
@then("the buyer app session cookie is unchanged and still signed in")
def buyer_untouched(world: World) -> None:
    cookies = {c["name"]: c["value"] for c in world.context.cookies(SETTINGS.base_url)}
    assert cookies.get("session") == X(world)["buyer_session_cookie"]
    assert not {c["name"]: c["value"] for c in world.context.cookies(SETTINGS.seller_base_url)}.get("seller_session")
    world.page.goto(f"{SETTINGS.base_url}/")
    expect(world.page.get_by_role("button", name="Đăng xuất")).to_be_visible(timeout=15_000)
