"""identity-account-deletion: request, cancel, status, obligations, sessions, sweeper, gateway.

Black box through the gateway (Connect JSON). See `account_deletion_support` for the two reads that go
around the edge (identity_db SELECT, `identity.events`) and the service-principal grpcurl call.
Erasure scenarios run on the short-window stack (compose/account-deletion.override.yaml).
"""

from __future__ import annotations

import re
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import legal_versions as legal
from src.utils import data as fake
from tests.e2e.support import account_deletion_support as ad
from tests.e2e.support import oic_order_support as o
from tests.e2e.support import plp_support as plp
from tests.e2e.support import seller_onboarding_support as so
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

SLACK_S = 5.0
SETTLE_WAIT_S = 60.0


def _x(world: World) -> dict:
    return world.state.extra


def _users(world: World) -> dict[str, dict]:
    return _x(world).setdefault("users", {})


def _user(world: World, name: str) -> dict:
    return _users(world)[name]


def _oic(world: World) -> o.OicWorld:
    return _x(world).setdefault("oic", o.OicWorld())


def _from_actor(actor: o.Actor) -> dict:
    user = {"username": actor.username, "password": get_settings().seed_password}
    user.update(ue.login(user))
    return user


def _resp(world: World) -> httpx.Response:
    return _x(world)["resp"]


def _request(world: World, name: str, password: str | None = None) -> httpx.Response:
    user = _user(world, name)
    before = time.time()
    resp = ad.request_deletion(user["token"], user["password"] if password is None else password)
    after = time.time()
    x = _x(world)
    x.update(resp=resp, request_before=before, request_after=after, request_user=name)
    if resp.status_code == 200:
        erase = ad.erase_after_of(ue.body_of(resp))
        x["last_erase"] = erase
        x.setdefault("first_erase", erase)
    return resp


# ── accounts ─────────────────────────────────────────────────────────────────
@given(parsers.parse('a signed-in buyer "{name}" with no obligations'))
def buyer_no_obligations(world: World, name: str) -> None:
    _users(world)[name] = ue.register_user("buyer", "del_b")


@given(parsers.parse('a signed-in seller "{name}" with no obligations'))
def seller_no_obligations(world: World, name: str) -> None:
    _users(world)[name] = ue.register_user("seller", "del_s")


def _buyer_with_open_order(world: World, name: str) -> dict:
    w = _oic(world)
    plp.seller_with_listing(w, 600_000, name=f"L-{name}")
    buyer = plp.buyer(w, name=f"buyer-{name}")
    o.place_order(w, buyer, f"L-{name}", 1)
    user = _from_actor(buyer)
    _users(world)[name] = user
    return user


@given(parsers.parse('a signed-in buyer "{name}" with an open order'))
def buyer_open_order(world: World, name: str) -> None:
    _buyer_with_open_order(world, name)


@given(parsers.parse('a signed-in seller "{name}" with a settled wallet balance'))
def seller_with_balance(world: World, name: str) -> None:
    w = _oic(world)
    seller = plp.seller_with_listing(w, 600_000, name="L")
    buyer = plp.buyer(w)
    plp.place_and_pay(w, buyer, "L", 1)
    deadline = time.monotonic() + SETTLE_WAIT_S
    balance = 0
    while time.monotonic() < deadline:
        balance = plp.balance(w, seller)
        if balance > 0:
            break
        time.sleep(1)
    assert balance > 0, "the paid order never credited the seller's wallet"
    _x(world)["balance"] = balance
    _users(world)[name] = _from_actor(seller)


@given(parsers.parse('a signed-in buyer "{name}" with three active sessions'))
def buyer_three_sessions(world: World, name: str) -> None:
    user = ue.register_user("buyer", "del_ses")
    _users(world)[name] = user
    _more_sessions(world, name, user)


@given(parsers.parse('a signed-in buyer "{name}" with an open order and three active sessions'))
def buyer_open_order_three_sessions(world: World, name: str) -> None:
    user = _buyer_with_open_order(world, name)
    _more_sessions(world, name, user)


def _more_sessions(world: World, name: str, user: dict) -> None:
    extra_logins = [ue.login(user), ue.login(user)]
    x = _x(world)
    x["second"], x["third"] = extra_logins
    x["sessions_before"] = ad.sessions_active(user["token"])
    assert x["sessions_before"] >= 3, f"expected at least 3 active sessions for {name}"


@given(parsers.parse('"{name}" has requested account deletion'))
def has_requested(world: World, name: str) -> None:
    resp = _request(world, name)
    assert resp.status_code == 200, f"setup request failed: {resp.status_code} {resp.text[:300]}"


# ── stack configuration ──────────────────────────────────────────────────────
@given("the stack runs with the default 48 hour cancel window")
def default_window(world: World) -> None:
    window = ad.configured_window_s()
    assert window in (None, ad.DEFAULT_WINDOW_S), (
        f"team-identity runs with ACCOUNT_DELETION_CANCEL_WINDOW={window}s; "
        "use the default stack for this scenario"
    )


@given("the stack runs with the short deletion window override")
def short_window(world: World) -> None:
    window, sweep = ad.configured_window_s(), ad.configured_sweep_s()
    assert window == ad.OVERRIDE_WINDOW_S and sweep == ad.OVERRIDE_SWEEP_S, (
        f"team-identity runs with window={window} sweep={sweep}; start it with "
        "platform-e2e/compose/account-deletion.override.yaml"
    )
    tap = ad.ErasedTap()
    _x(world)["tap"] = tap
    world.add_cleanup(tap.close)


@given(parsers.parse('a signed-in buyer "{name}" with an address and two sessions'))
def buyer_address_sessions(world: World, name: str) -> None:
    user = ue.register_user("buyer", "del_e")
    _address_and_sessions(world, name, user)


@given(parsers.parse('a signed-in seller "{name}" with an address and two sessions'))
def seller_address_sessions(world: World, name: str) -> None:
    user = ue.register_user("seller", "del_es")
    _address_and_sessions(world, name, user)


def _address_and_sessions(world: World, name: str, user: dict) -> None:
    resp = ue.call(
        "/platform.identity.v1.AddressService/CreateAddress",
        {
            "recipientName": "Nguyen Van A",
            "phone": "0912345678",
            "street": "29 Lieu Giai",
            "city": "Ha Noi",
            "ward": "Phuong Lieu Giai",
            "district": "Quan Ba Dinh",
            "isDefault": True,
        },
        user["token"],
    )
    assert resp.status_code == 200, f"CreateAddress failed: {resp.status_code} {resp.text[:200]}"
    ue.login(user)
    _users(world)[name] = user
    counts = ad.erased_table_counts(user["id"])
    assert counts["addresses"] >= 1 and counts["sessions"] >= 2, counts
    _x(world)["roles_before"] = ad.user_row(user["id"])["roles"]


@given(parsers.parse('the consent record count of "{name}" is recorded'))
def record_consents(world: World, name: str) -> None:
    user = _user(world, name)
    assert so.become_seller(user["token"]).status_code in (200, 400, 412), "BecomeSeller"
    _x(world)["consents_before"] = ad.consent_count(user["id"])
    assert _x(world)["consents_before"] >= 2


# ── when: calls ──────────────────────────────────────────────────────────────
@when(parsers.parse('"{name}" requests account deletion with the correct password'))
def request_correct(world: World, name: str) -> None:
    _request(world, name)


@when(parsers.parse('"{name}" requests account deletion with the password "{password}"'))
def request_wrong(world: World, name: str, password: str) -> None:
    _request(world, name, password)


@when(parsers.parse('"{name}" requests account deletion with an empty password'))
def request_empty(world: World, name: str) -> None:
    _request(world, name, "")


@when(parsers.parse('"{name}" requests account deletion from the first session'))
def request_first_session(world: World, name: str) -> None:
    _request(world, name)


@when("a caller without a token requests account deletion through the gateway")
def anon_request(world: World) -> None:
    _x(world)["resp"] = ad.request_deletion(None, "whatever-password")


@when("a caller without a token cancels account deletion through the gateway")
def anon_cancel(world: World) -> None:
    _x(world)["resp"] = ad.cancel_deletion(None)


@when("a caller without a token reads the deletion status through the gateway")
def anon_status(world: World) -> None:
    _x(world)["resp"] = ad.status_of(None)


@when(
    parsers.parse(
        'a service principal requests account deletion for "{name}" directly on team-identity'
    )
)
def service_principal(world: World, name: str) -> None:
    user = _user(world, name)
    code, text = ad.service_principal_request(user["id"], user["password"])
    _x(world)["direct"] = (code, text)


@when(parsers.parse('"{name}" cancels account deletion'))
def cancel(world: World, name: str) -> None:
    _x(world)["resp"] = ad.cancel_deletion(_user(world, name)["token"])


@when(parsers.parse('"{name}" reads the deletion status'))
def read_status(world: World, name: str) -> None:
    _x(world)["resp"] = ad.status_of(_user(world, name)["token"])


@when(
    parsers.parse(
        '"{name}" and an anonymous caller post GetUserObligations and GetPayoutObligations through the gateway'
    )
)
def post_obligations(world: World, name: str) -> None:
    token = _user(world, name)["token"]
    out = []
    for path in (ad.ORDER_OBLIGATIONS, ad.PAYMENT_OBLIGATIONS):
        for tok in (token, None):
            out.append(ue.call(path, {"userId": _user(world, name)["id"]}, tok))
    _x(world)["obligation_resps"] = out


# ── when: time and sweeper ───────────────────────────────────────────────────
def _wait_erased(world: World, name: str) -> None:
    user = _user(world, name)
    bound = ad.OVERRIDE_WINDOW_S + 2 * ad.OVERRIDE_SWEEP_S + 15
    deadline = time.monotonic() + bound
    started = time.monotonic()
    while time.monotonic() < deadline:
        if ad.user_row(user["id"])["status"] == "deleted":
            _x(world)["erased_after_s"] = time.time() - _x(world)["request_before"]
            _x(world)["username_before"] = user["username"]
            return
        time.sleep(1)
    raise AssertionError(f"account not erased {time.monotonic() - started:.0f}s after the request")


@when(
    parsers.parse(
        '"{name}" requests account deletion with the correct password and the window passes'
    )
)
def request_and_wait(world: World, name: str) -> None:
    resp = _request(world, name)
    assert resp.status_code == 200, f"request failed: {resp.status_code} {resp.text[:300]}"
    _wait_erased(world, name)


@when(parsers.parse("{seconds:d} seconds pass inside the window"))
def pass_inside_window(world: World, seconds: int) -> None:
    user = _user(world, _x(world)["request_user"])
    for _ in range(seconds):
        assert ad.user_row(user["id"])["status"] != "deleted", "erased before erase_after"
        time.sleep(1)
    _x(world)["tap"].poll(2.0)


@when("the former erase_after has passed by two sweep intervals")
def pass_former_deadline(world: World) -> None:
    target = _x(world)["last_erase"] + 2 * ad.OVERRIDE_SWEEP_S + 1
    while time.time() < target:
        time.sleep(1)
    _x(world)["tap"].poll(3.0)


@when(parsers.parse("three more sweep intervals pass"))
def three_more_sweeps(world: World) -> None:
    tap = _x(world)["tap"]
    tap.wait_for(_user(world, _x(world)["request_user"])["id"], 10.0)
    tap.poll(3 * ad.OVERRIDE_SWEEP_S + 1)


@when(parsers.parse('someone logs in with the former username and password of "{name}"'))
def login_former(world: World, name: str) -> None:
    user = _user(world, name)
    _x(world)["login_former"] = ue.login_raw(user["username"], user["password"])


@when("someone logs in with an unknown username")
def login_unknown(world: World) -> None:
    _x(world)["login_unknown"] = ue.login_raw(fake.unique_username("del_nobody"), "wrong-pass-123")


@when(parsers.parse('a visitor registers the former username of "{name}"'))
def register_former(world: World, name: str) -> None:
    user = _user(world, name)
    _x(world)["register_resp"] = ue.call(
        ue.REGISTER,
        {
            "username": user["username"],
            "password": user["password"],
            "role": "buyer",
            **legal.versions(),
        },
    )


# ── then: responses ──────────────────────────────────────────────────────────
@then(parsers.parse('the call fails with code "{expected}"'))
def call_fails(world: World, expected: str) -> None:
    if "direct" in _x(world) and "resp" not in _x(world):
        code, text = _x(world)["direct"]
        assert code == expected, f"expected {expected}, got {code}: {text[:300]}"
        return
    resp = _resp(world)
    assert resp.status_code >= 400, f"expected failure {expected}, got 200: {resp.text[:300]}"
    assert ad.code(resp) == expected, f"expected {expected}, got {ad.code(resp)}: {resp.text[:300]}"


@then("the call is rejected as unauthenticated")
def rejected_unauth(world: World) -> None:
    resp = _resp(world)
    assert resp.status_code == 401, f"expected 401, got {resp.status_code}: {resp.text[:300]}"
    assert ad.code(resp) in ("", "unauthenticated"), resp.text[:300]


@then("the gateway answers 401")
def gateway_401(world: World) -> None:
    assert _resp(world).status_code == 401, _resp(world).text[:300]


@then("the gateway answers 200 with the status body")
def gateway_200(world: World) -> None:
    resp = _resp(world)
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    assert ad.erase_after_of(ue.body_of(resp)) is None and not ue.body_of(resp).get(
        "pending", False
    )


@then("every one of those gateway calls answers 501")
def all_501(world: World) -> None:
    resps = _x(world)["obligation_resps"]
    assert len(resps) == 4
    assert [r.status_code for r in resps] == [501] * 4, [
        (r.status_code, r.text[:100]) for r in resps
    ]


@then("the request succeeds")
def request_succeeds(world: World) -> None:
    resp = _resp(world)
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    assert _x(world).get("last_erase"), "no erase_after in the response"


@then(
    "the request succeeds with erase_after equal to the request time plus 48 hours within 5 seconds"
)
def request_48h(world: World) -> None:
    resp = _resp(world)
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    erase = _x(world)["last_erase"]
    low = _x(world)["request_before"] + ad.DEFAULT_WINDOW_S - ad.TIMESTAMP_SKEW_S
    high = _x(world)["request_after"] + ad.DEFAULT_WINDOW_S + ad.TIMESTAMP_SKEW_S
    assert low <= erase <= high, f"erase_after {erase} outside [{low}, {high}]"


@then("the request succeeds with the erase_after of the first request")
def request_keeps_first(world: World) -> None:
    resp = _resp(world)
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    again = ad.erase_after_of(ue.body_of(resp))
    assert again is not None and abs(again - _x(world)["first_erase"]) < 0.001, (
        again,
        _x(world)["first_erase"],
    )


@then(parsers.parse('the deletion status of "{name}" is not pending'))
def status_not_pending(world: World, name: str) -> None:
    assert not ad.is_pending(_user(world, name)["token"]), f"{name} has a pending deletion"


@then(parsers.parse('the deletion status of "{name}" is pending with the same erase_after'))
def status_pending_same(world: World, name: str) -> None:
    body = ad.status_body(_user(world, name)["token"])
    assert body.get("pending") is True, body
    erase = ad.erase_after_of(body)
    assert erase is not None and abs(erase - _x(world)["last_erase"]) < 0.001, (body, _x(world))


@then(parsers.parse('the deletion status of "{name}" is pending'))
def status_pending(world: World, name: str) -> None:
    assert ad.is_pending(_user(world, name)["token"]), f"{name} has no pending deletion"


@then("the status response has pending false and no erase_after")
def status_empty(world: World) -> None:
    body = ue.body_of(_resp(world))
    assert _resp(world).status_code == 200, _resp(world).text[:300]
    assert not body.get("pending", False) and ad.erase_after_of(body) is None, body


@then("the status response has pending true and the erase_after of the first request")
def status_first(world: World) -> None:
    body = ue.body_of(_resp(world))
    assert body.get("pending") is True, body
    erase = ad.erase_after_of(body)
    assert erase is not None and abs(erase - _x(world)["first_erase"]) < 0.001, body


@then(parsers.parse('"{name}" can still sign in'))
def can_sign_in(world: World, name: str) -> None:
    user = _user(world, name)
    resp = ue.login_raw(user["username"], user["password"])
    assert resp.status_code == 200, f"login failed: {resp.status_code} {resp.text[:300]}"


# ── then: obligations ────────────────────────────────────────────────────────
@then("the failure message names open orders")
def names_orders(world: World) -> None:
    msg = ad.message_of(_resp(world))
    assert ad.names_category(msg, "open orders"), f"message does not name open orders: {msg!r}"


@then("the failure message names the wallet balance and contains no amount")
def names_balance(world: World) -> None:
    msg = ad.message_of(_resp(world))
    assert ad.names_category(msg, "wallet balance"), f"message does not name the balance: {msg!r}"
    assert str(_x(world)["balance"]) not in msg, f"message leaks the amount: {msg!r}"
    assert not re.search(r"\d{4,}", msg), f"message carries a number: {msg!r}"


# ── then: sessions ───────────────────────────────────────────────────────────
@then("the first session stays active and the other two are revoked")
def other_sessions_revoked(world: World) -> None:
    user = _users(world)["A"]
    assert _resp(world).status_code == 200, _resp(world).text[:300]
    assert ad.sessions_active(user["token"]) == 1, "only the calling session may stay active"


@then(
    parsers.parse(
        'refreshing with the second session\'s refresh token fails with code "{expected}"'
    )
)
def refresh_second_fails(world: World, expected: str) -> None:
    resp = ad.refresh_with(_x(world)["second"]["refresh"])
    assert (
        resp.status_code >= 400 and ad.code(resp) == expected
    ), f"{resp.status_code} {resp.text[:300]}"
    third = ad.refresh_with(_x(world)["third"]["refresh"])
    assert (
        third.status_code >= 400 and ad.code(third) == expected
    ), f"{third.status_code} {third.text[:300]}"


@then("all three sessions are still active")
def all_sessions_active(world: World) -> None:
    user = _users(world)["A"]
    assert ad.sessions_active(user["token"]) == _x(world)["sessions_before"]


# ── then: sweeper ────────────────────────────────────────────────────────────
@then(
    parsers.parse(
        'the user row of "{name}" has status deleted, a deleted_ and 16 hex username, an empty password hash and no roles'
    )
)
def user_row_erased(world: World, name: str) -> None:
    row = ad.user_row(_user(world, name)["id"])
    assert row["status"] == "deleted", row
    assert re.fullmatch(r"deleted_[0-9a-f]{16}", row["username"]), row["username"]
    assert row["password_hash"] == "", "password hash was not cleared"
    assert row["roles"] == "", row["roles"]


@then(parsers.parse('the user row of "{name}" has status deleted'))
def user_row_deleted(world: World, name: str) -> None:
    assert ad.user_row(_user(world, name)["id"])["status"] == "deleted"


@then(
    parsers.parse(
        'no addresses, sessions, login history, handoff codes or reset tokens remain for "{name}"'
    )
)
def nothing_remains(world: World, name: str) -> None:
    counts = ad.erased_table_counts(_user(world, name)["id"])
    assert all(v == 0 for v in counts.values()), counts


@then(
    parsers.parse(
        'exactly one UserErased event for "{name}" is on identity.events carrying the roles held before erasure'
    )
)
def one_event_with_roles(world: World, name: str) -> None:
    uid = _user(world, name)["id"]
    events = _x(world)["tap"].wait_for(uid, 20.0)
    assert len(events) == 1, events
    assert events[0]["key"] == uid and events[0]["has_erased_at"], events[0]
    before = {r for r in _x(world)["roles_before"].split(",") if r}
    assert before and set(events[0]["roles"]) == before, (events[0]["roles"], before)


@then(parsers.parse('exactly one UserErased event for "{name}" is on identity.events'))
def exactly_one_event(world: World, name: str) -> None:
    uid = _user(world, name)["id"]
    _x(world)["tap"].poll(2.0)
    events = _x(world)["tap"].events_for(uid)
    assert len(events) == 1, events


@then(parsers.parse('no UserErased event for "{name}" is on identity.events'))
def no_event(world: World, name: str) -> None:
    tap = _x(world)["tap"]
    tap.poll(2.0)
    assert tap.events_for(_user(world, name)["id"]) == []


@then(parsers.parse('the consent record count of "{name}" is unchanged and at least 2'))
def consents_survive(world: World, name: str) -> None:
    after = ad.consent_count(_user(world, name)["id"])
    assert after == _x(world)["consents_before"] and after >= 2, (
        after,
        _x(world)["consents_before"],
    )


@then(parsers.parse('the user row of "{name}" is still active with its address and sessions'))
def still_active(world: World, name: str) -> None:
    uid = _user(world, name)["id"]
    row = ad.user_row(uid)
    assert row["status"] == "active" and row["username"] == _user(world, name)["username"], row
    counts = ad.erased_table_counts(uid)
    assert counts["addresses"] >= 1 and counts["sessions"] >= 2, counts


@then(
    parsers.parse(
        'the account of "{name}" was erased no later than the window plus two sweep intervals after the request'
    )
)
def erased_in_bound(world: World, name: str) -> None:
    bound = ad.OVERRIDE_WINDOW_S + 2 * ad.OVERRIDE_SWEEP_S + SLACK_S
    assert _x(world)["erased_after_s"] <= bound, (_x(world)["erased_after_s"], bound)


@then(parsers.parse('a UserErased event for "{name}" is on identity.events within the same bound'))
def event_in_bound(world: World, name: str) -> None:
    uid = _user(world, name)["id"]
    events = _x(world)["tap"].wait_for(uid, 10.0)
    assert len(events) >= 1, "no UserErased on identity.events"


@then("both logins fail with the same code and message")
def same_failure(world: World) -> None:
    former, unknown = _x(world)["login_former"], _x(world)["login_unknown"]
    assert former.status_code >= 400 and unknown.status_code >= 400, (former.text, unknown.text)
    assert former.status_code == unknown.status_code
    assert ad.code(former) == ad.code(unknown)
    assert ad.message_of(former) == ad.message_of(unknown), (
        ad.message_of(former),
        ad.message_of(unknown),
    )


@then("the registration succeeds")
def registration_succeeds(world: World) -> None:
    resp = _x(world)["register_resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text[:300]}"
    assert uuid.UUID(ue.user_id_of(ue.body_of(resp)["token"]))
