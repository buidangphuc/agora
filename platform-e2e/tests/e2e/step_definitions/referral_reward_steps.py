from __future__ import annotations

import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import prm_rma_support as rma
from tests.e2e.support import referral_events_support as rev

REFERRAL = "/platform.referral.v1.ReferralService"
ATTRIBUTION_WAIT_S = 10.0
CONSUME_WAIT_S = 15.0
CONSUME_SETTLE_S = 6.0
RELEASE_SLACK_S = 3.0
SELLER = "refseller"
LISTING = "REFL"
PRICE = 600000
REWARD_XU = 10000


def _tap(oic) -> rev.ReferralEventsTap:
    if "ref_tap" not in oic.data:
        oic.data["ref_tap"] = rev.ReferralEventsTap()
    return oic.data["ref_tap"]


def _n(body: dict, key: str) -> int:
    return int(body.get(key) or 0)


def _summary(oic, name: str) -> dict:
    return s.ok(s.post(oic, oic.actors[name], REFERRAL, "GetMyReferral", {}))


def _rewards(oic, name: str) -> list[dict]:
    body = s.ok(
        s.post(oic, oic.actors[name], REFERRAL, "ListReferralRewards", {"page": {"pageSize": 50}})
    )
    return body.get("rewards", [])


def _status(row: dict) -> str:
    return str(row.get("status", "")).replace("REFERRAL_REWARD_STATUS_", "")


def _poll(fn, ok, wait_s: float):
    deadline = time.monotonic() + wait_s
    value = fn()
    while not ok(value) and time.monotonic() < deadline:
        time.sleep(0.5)
        value = fn()
    return value


def _register(oic, name: str, code: str | None = None) -> s.Actor:
    body = {
        "username": f"ref_{uuid.uuid4().hex[:10]}",
        "password": get_settings().seed_password,
        "role": "buyer",
    }
    if code is not None:
        body["referralCode"] = code
    resp = s.post(oic, None, s.AUTH, "Register", body)
    token = (s.ok(resp).get("result") or {}).get("token", "")
    assert token, f"register {name}: {resp.text[:300]}"
    actor = s.Actor(body["username"], token, "buyer", s._sub(token))
    oic.actors[name] = actor
    return actor


def _complete_order(oic, name: str) -> str:
    buyer = oic.actors[name]
    seller = oic.actors[SELLER]
    order_id = ol.place(oic, buyer, LISTING, 1, "PAYMENT_METHOD_COD")
    ol.ship(oic, order_id, seller)
    s.ok(s.post(oic, buyer, s.ORDER, "ConfirmReceipt", {"id": order_id}))
    s.wait_status(oic, buyer, order_id, s.COMPLETED)
    oic.data.setdefault("ref_orders", {}).setdefault(name, []).append(order_id)
    oic.data.setdefault("ref_completed_at", {})[order_id] = time.time()
    return order_id


@given("the referral events topic is being observed")
def given_tap(oic) -> None:
    _tap(oic)


@given("the stack sweeps releases every second and closes the return window after 3 seconds")
def given_fast_stack(oic) -> None:
    oic.data["ref_interval"], oic.data["ref_window"] = rev.require_fast_stack()


@given(
    "the stack sweeps releases every second, closes the return window after 3 seconds and "
    "caps releases at 2 per day"
)
def given_fast_stack_cap(oic) -> None:
    oic.data["ref_interval"], oic.data["ref_window"] = rev.require_fast_stack(daily_cap=2)


@given("a seller with a stocked listing for referral orders")
def given_seller(oic) -> None:
    seller = s.register(oic, SELLER, "seller")
    s.create_listing(oic, LISTING, seller, 50, price=PRICE)


@given(parsers.parse('an inviter "{name}" with a referral code'))
def given_inviter(oic, name: str) -> None:
    actor = s.register(oic, name, "buyer")
    body = s.ok(s.post(oic, actor, REFERRAL, "CreateReferralCode", {}))
    assert body.get("code"), body
    oic.data.setdefault("ref_codes", {})[name] = body["code"]


@given(parsers.parse('"{name}" registered through the gateway with the code of "{inviter}"'))
@when(parsers.parse('"{name}" registers through the gateway with the code of "{inviter}"'))
def when_register_with_code(oic, name: str, inviter: str) -> None:
    _tap(oic)
    actor = _register(oic, name, oic.data["ref_codes"][inviter])
    oic.data.setdefault("ref_registered_at", {})[name] = time.time()
    assert actor.user_id


@given(parsers.parse('"{name}" registered through the gateway with no referral code'))
def given_register_plain(oic, name: str) -> None:
    _tap(oic)
    _register(oic, name)


@given(parsers.parse('"{name}" completed an order'))
@when(parsers.parse('"{name}" completes an order'))
def when_complete(oic, name: str) -> None:
    _complete_order(oic, name)


@given(parsers.parse('"{name}" completed an order and the order events were consumed'))
def given_completed_consumed(oic, name: str) -> None:
    _complete_order(oic, name)
    time.sleep(CONSUME_SETTLE_S)


@when(parsers.parse('"{name}" requests a return on that order'))
def when_return(oic, name: str) -> None:
    order_id = oic.data["ref_orders"][name][-1]
    oic.data["ref_return"] = rma.request_return(
        oic, oic.actors[name], order_id, None, "e2e referral return"
    )
    s.ok(oic.data["ref_return"])


@when(parsers.parse('"{name}" redeems the code of "{inviter}"'))
def when_redeem(oic, name: str, inviter: str) -> None:
    oic.data["ref_redeem"] = s.post(
        oic, oic.actors[name], REFERRAL, "RedeemReferral", {"code": oic.data["ref_codes"][inviter]}
    )


@when(parsers.parse('"{name}" redeems the code of "{inviter}" once the registration is known'))
def when_redeem_known(oic, name: str, inviter: str) -> None:
    code = oic.data["ref_codes"][inviter]

    def attempt() -> httpx.Response:
        return s.post(oic, oic.actors[name], REFERRAL, "RedeemReferral", {"code": code})

    oic.data["ref_redeem"] = _poll(attempt, lambda r: r.status_code == 200, CONSUME_WAIT_S)


@then(
    parsers.parse(
        'ReferralAttributed for "{name}" naming "{inviter}" is observed within 10 seconds'
    )
)
def then_attributed(oic, name: str, inviter: str) -> None:
    referee = oic.actors[name].user_id
    found = _tap(oic).wait_for(rev.ATTRIBUTED, referee, ATTRIBUTION_WAIT_S)
    assert found, f"no ReferralAttributed for {referee} within {ATTRIBUTION_WAIT_S}s"
    assert found[0].key == referee, f"event key {found[0].key!r}, want the referee id"
    assert found[0].fields["referrer_id"] == oic.actors[inviter].user_id
    waited = time.time() - oic.data["ref_registered_at"][name]
    assert waited <= ATTRIBUTION_WAIT_S + 5, f"attribution took {waited:.1f}s"


@then(parsers.parse('no ReferralAttributed for "{name}" is observed'))
def then_not_attributed(oic, name: str) -> None:
    tap = _tap(oic)
    tap.drain(3.0)
    assert not tap.matching(rev.ATTRIBUTED, oic.actors[name].user_id)


@then(parsers.parse('"{name}" shows pending_xu {pending:d} and released_xu {released:d}'))
def then_totals(oic, name: str, pending: int, released: int) -> None:
    got = _poll(
        lambda: _summary(oic, name),
        lambda b: _n(b, "pendingXu") == pending and _n(b, "releasedXu") == released,
        CONSUME_WAIT_S,
    )
    assert (_n(got, "pendingXu"), _n(got, "releasedXu")) == (pending, released), got


@then(parsers.parse('"{name}" lists exactly one {status} reward of {amount:d} xu'))
def then_one_reward(oic, name: str, status: str, amount: int) -> None:
    rows = _poll(
        lambda: _rewards(oic, name),
        lambda r: len(r) == 1 and _status(r[0]) == status,
        CONSUME_WAIT_S,
    )
    assert len(rows) == 1 and _status(rows[0]) == status, rows
    assert _n(rows[0], "amount") == amount, rows[0]


@then(parsers.parse('"{name}" lists no reward'))
def then_no_reward(oic, name: str) -> None:
    assert _rewards(oic, name) == []


@then(parsers.parse('"{name}" lists no void reward and no pending reward'))
def then_no_void_pending(oic, name: str) -> None:
    statuses = [_status(r) for r in _rewards(oic, name)]
    assert "VOID" not in statuses and "PENDING" not in statuses, statuses


@then(
    parsers.parse('"{name}" shows released_xu {released:d} within the return window plus 2 seconds')
)
def then_released_in_time(oic, name: str, released: int) -> None:
    order_id = oic.data["ref_orders"]["bob"][-1]
    completed = oic.data["ref_completed_at"][order_id]
    deadline = completed + oic.data["ref_window"] + 2.0 + RELEASE_SLACK_S
    got = _poll(
        lambda: _summary(oic, name),
        lambda b: _n(b, "releasedXu") == released,
        max(deadline - time.time(), 0.5),
    )
    assert _n(got, "releasedXu") == released, (
        f"released_xu {_n(got, 'releasedXu')} after {time.time() - completed:.1f}s"
    )


@then(parsers.parse('"{name}" shows rewards_total {total:d}'))
def then_rewards_total(oic, name: str, total: int) -> None:
    assert _n(_summary(oic, name), "rewardsTotal") == total


@then(
    parsers.parse(
        'ReferralRewardReleased of {amount:d} xu for "{name}" naming "{inviter}" is observed'
    )
)
def then_released_event(oic, name: str, inviter: str, amount: int) -> None:
    referee = oic.actors[name].user_id
    found = _tap(oic).wait_for(rev.RELEASED, referee, ATTRIBUTION_WAIT_S)
    assert found, f"no ReferralRewardReleased for {referee}"
    assert found[0].fields["amount_xu"] == amount
    assert found[0].fields["referrer_id"] == oic.actors[inviter].user_id
    assert len(found) == 1, f"{len(found)} ReferralRewardReleased events for one reward"


@then(parsers.parse('"{name}" shows invited_count {count:d}'))
def then_invited(oic, name: str, count: int) -> None:
    got = _poll(lambda: _summary(oic, name), lambda b: _n(b, "invitedCount") == count, 5.0)
    assert _n(got, "invitedCount") == count, got


@then("the redeem is accepted")
def then_redeem_ok(oic) -> None:
    resp = oic.data["ref_redeem"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text[:300]}"


@then("the redeem is refused with FAILED_PRECONDITION and a message")
def then_redeem_refused(oic) -> None:
    resp = oic.data["ref_redeem"]
    assert resp.status_code != 200, "the redeem was accepted"
    assert s.code_of(resp) == "failed_precondition", f"{resp.status_code} {resp.text[:300]}"
    assert resp.json().get("message"), resp.text[:300]


@when(parsers.parse('three invitees "{a}", "{b}" and "{c}" register with the code of "{inviter}"'))
def when_three(oic, a: str, b: str, c: str, inviter: str) -> None:
    for invitee in (a, b, c):
        when_register_with_code(oic, invitee, inviter)


@when("the three invitees each complete an order")
def when_three_complete(oic) -> None:
    for invitee in ("b1", "b2", "b3"):
        _complete_order(oic, invitee)


@then(
    parsers.parse(
        '"{name}" ends with two released rewards and one pending reward after the release sweeps'
    )
)
def then_cap(oic, name: str) -> None:
    got = _poll(
        lambda: _summary(oic, name),
        lambda b: _n(b, "releasedXu") == 2 * REWARD_XU,
        CONSUME_WAIT_S,
    )
    assert _n(got, "releasedXu") == 2 * REWARD_XU, got
    time.sleep(2 * oic.data["ref_interval"] + RELEASE_SLACK_S)
    got = _summary(oic, name)
    assert (_n(got, "pendingXu"), _n(got, "releasedXu")) == (REWARD_XU, 2 * REWARD_XU), got
    statuses = sorted(_status(r) for r in _rewards(oic, name))
    assert statuses == ["PENDING", "RELEASED", "RELEASED"], statuses
