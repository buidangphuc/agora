from __future__ import annotations

import time
import uuid

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import new_user_voucher_support as nu
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import referral_events_support as rev
from tests.e2e.support import voucher_scope_support as vs

REFERRAL = "/platform.referral.v1.ReferralService"
VERDICT_WAIT_S = 10.0
RESTART_WAIT_S = 60.0
ORDER_LISTING = "NUVL"
ORDER_SELLER = "nuvseller"
SHOP_SUBTOTAL = 200000


@pytest.fixture(autouse=True)
def referral_restore(oic):  # noqa: F811
    yield
    nu.stopped_referral_guard(oic)


def _tap(oic) -> rev.ReferralEventsTap:  # noqa: F811
    if "nuv_tap" not in oic.data:
        oic.data["nuv_tap"] = rev.ReferralEventsTap()
    return oic.data["nuv_tap"]


def _voucher_of(oic, name: str) -> dict:  # noqa: F811
    found = nu.my_vouchers(oic, oic.actors[name])
    assert len(found) == 1, f"{name} lists {len(found)} vouchers: {found}"
    return found[0]


@given(parsers.parse("the stack uses a 2 second verdict timeout and a 1 second sweep"))
def given_fast_verdict(oic) -> None:  # noqa: F811
    oic.data["nuv_timeout"], oic.data["nuv_sweep"] = nu.require_fast_verdict()


@given("the referral events topic is being observed for new-user vouchers")
def given_tap(oic) -> None:  # noqa: F811
    _tap(oic)


@given(parsers.parse('an inviter "{name}" with a referral code for new-user vouchers'))
def given_inviter(oic, name: str) -> None:  # noqa: F811
    actor = s.register(oic, name, "buyer")
    body = s.ok(s.post(oic, actor, REFERRAL, "CreateReferralCode", {}))
    assert body.get("code"), body
    oic.data.setdefault("nuv_codes", {})[name] = body["code"]


@given("team-referral is stopped")
def given_referral_stopped(oic) -> None:  # noqa: F811
    stack.docker("stop", stack.container_name(nu.REFERRAL_SERVICE))
    oic.data["nuv_referral_stopped"] = True


@when("team-referral is started again")
def when_referral_started(oic) -> None:  # noqa: F811
    stack.docker("start", stack.container_name(nu.REFERRAL_SERVICE))
    oic.data["nuv_referral_stopped"] = False


@given(
    parsers.parse(
        '"{name}" registered through the gateway with no referral code and holds a voucher'
    )
)
def given_registered_with_voucher(oic, name: str) -> None:  # noqa: F811
    actor = nu.register(oic, name)
    found = nu.wait_for_vouchers(oic, actor)
    oic.data.setdefault("nuv_voucher", {})[name] = found[0]


@when(parsers.parse('"{name}" registers through the gateway with no referral code'))
def when_register_plain(oic, name: str) -> None:  # noqa: F811
    nu.register(oic, name)


@when(parsers.parse('"{name}" registers through the gateway with the code of "{inviter}"'))
def when_register_with_code(oic, name: str, inviter: str) -> None:  # noqa: F811
    _tap(oic)
    nu.register(oic, name, oic.data["nuv_codes"][inviter])


@when(
    parsers.parse(
        '"{name}" registers through the gateway with an unknown well-formed referral code'
    )
)
def when_register_unknown(oic, name: str) -> None:  # noqa: F811
    _tap(oic)
    nu.register(oic, name, f"NOSUCH{uuid.uuid4().hex[:8]}")


@then(parsers.parse('ReferralAttributed for "{name}" is observed within {wait:d} seconds'))
def then_attributed(oic, name: str, wait: int) -> None:  # noqa: F811
    found = _tap(oic).wait_for(rev.ATTRIBUTED, oic.actors[name].user_id, float(wait))
    assert found, f"no ReferralAttributed for {name} within {wait} s"


@then(parsers.parse('ReferralRejected for "{name}" is observed within {wait:d} seconds'))
def then_rejected(oic, name: str, wait: int) -> None:  # noqa: F811
    found = _tap(oic).wait_for(rev.REJECTED, oic.actors[name].user_id, float(wait))
    assert found, f"no ReferralRejected for {name} within {wait} s"


@then(
    parsers.parse(
        '"{name}" owns exactly one new-user voucher with discount {amount:d} within 10 seconds'
    )
)
def then_owns_one(oic, name: str, amount: int) -> None:  # noqa: F811
    actor = oic.actors[name]
    found = nu.wait_for_vouchers(oic, actor)
    nu.assert_new_user_voucher(found[0], amount)
    time.sleep(nu.QUIET_S)
    again = nu.my_vouchers(oic, actor)
    assert len(again) == 1, f"{name} lists {len(again)} vouchers: {again}"
    nu.assert_new_user_voucher(again[0], amount)


@then(parsers.parse('"{name}" owns no voucher yet'))
def then_owns_none(oic, name: str) -> None:  # noqa: F811
    elapsed = time.monotonic() - oic.data["nuv_registered_at"][name]
    assert elapsed < oic.data["nuv_timeout"], f"{elapsed:.1f} s already passed, timeout too short"
    assert nu.my_vouchers(oic, oic.actors[name]) == []


@then(
    parsers.parse(
        '"{name}" owns exactly one new-user voucher with discount {amount:d} within the verdict timeout plus two sweeps'
    )
)
def then_owns_one_after_timeout(oic, name: str, amount: int) -> None:  # noqa: F811
    budget = oic.data["nuv_timeout"] + 2 * oic.data["nuv_sweep"] + 3.0
    found = nu.wait_for_vouchers(oic, oic.actors[name], budget)
    assert len(found) == 1, found
    nu.assert_new_user_voucher(found[0], amount)


@then(parsers.parse('"{name}" still owns exactly one new-user voucher with discount {amount:d}'))
def then_still_one(oic, name: str, amount: int) -> None:  # noqa: F811
    time.sleep(nu.QUIET_S)
    found = nu.my_vouchers(oic, oic.actors[name])
    assert len(found) == 1, f"{name} lists {len(found)} vouchers: {found}"
    nu.assert_new_user_voucher(found[0], amount)


@then(parsers.parse('ReferralAttributed for "{name}" is observed after the restart'))
def then_attributed_after_restart(oic, name: str) -> None:  # noqa: F811
    found = _tap(oic).wait_for(rev.ATTRIBUTED, oic.actors[name].user_id, RESTART_WAIT_S)
    assert found, f"no late ReferralAttributed for {name} within {RESTART_WAIT_S} s"


def _reserve(
    oic,  # noqa: F811
    buyer: str,
    code: str,
    subtotal: int,
    seller_id: str = "",
) -> tuple[str, str, object]:
    rid = vs.fresh_reservation("nuv")
    status, body = vs.validate_and_reserve(
        code, rid, subtotal, seller_id=seller_id, buyer_id=oic.actors[buyer].user_id
    )
    oic.data.setdefault("nuv_rids", []).append(rid)
    oic.data["nuv_last"] = (status, body)
    return rid, status, body


@when(
    parsers.parse(
        '"{buyer}" reserves the voucher of "{owner}" as the order service with subtotal {subtotal:d}'
    )
)
def when_reserve_other(oic, buyer: str, owner: str, subtotal: int) -> None:  # noqa: F811
    _reserve(oic, buyer, oic.data["nuv_voucher"][owner]["code"], subtotal)


@when(
    parsers.parse(
        '"{buyer}" reserves her own voucher as the order service with subtotal {subtotal:d}'
    )
)
def when_reserve_own(oic, buyer: str, subtotal: int) -> None:  # noqa: F811
    _reserve(oic, buyer, oic.data["nuv_voucher"][buyer]["code"], subtotal)


@when(
    parsers.parse(
        '"{buyer}" reserves the shop voucher as the order service with subtotal {subtotal:d}'
    )
)
def when_reserve_shop(oic, buyer: str, subtotal: int) -> None:  # noqa: F811
    seller = oic.actors[ORDER_SELLER]
    _reserve(oic, buyer, oic.data["nuv_shop_code"], subtotal, seller_id=seller.user_id)


@when("the order service commits the first reservation")
def when_commit_first(oic) -> None:  # noqa: F811
    status, body = av.commit(oic.data["nuv_rids"][0])
    assert status == "ok" and body.get("committed"), (status, body)


@then(parsers.parse("the reservation is valid with discount {amount:d}"))
def then_valid_discount(oic, amount: int) -> None:  # noqa: F811
    status, body = oic.data["nuv_last"]
    assert status == "ok", (status, body)
    assert body.get("valid") is True, body
    assert int(body.get("discountAmount") or 0) == amount, body


@then("the reservation is valid")
def then_valid(oic) -> None:  # noqa: F811
    status, body = oic.data["nuv_last"]
    assert status == "ok", (status, body)
    assert body.get("valid") is True, body


@then(parsers.parse('the reservation is refused with reason "{reason}" and no error'))
def then_refused(oic, reason: str) -> None:  # noqa: F811
    status, body = oic.data["nuv_last"]
    assert status == "ok", f"want a refusal without a gRPC error, got {status}: {body}"
    assert not body.get("valid"), body
    assert reason in str(body.get("reason", "")), body


@then(parsers.parse('"{owner}" still lists her voucher with nothing used'))
def then_still_listed(oic, owner: str) -> None:  # noqa: F811
    code = oic.data["nuv_voucher"][owner]["code"]
    listed = [v for v in nu.my_vouchers(oic, oic.actors[owner]) if v.get("code") == code]
    assert len(listed) == 1, listed
    assert nu.num(listed[0], "used") == 0, listed


@then(parsers.parse('"{owner}" no longer lists the voucher'))
def then_no_longer_listed(oic, owner: str) -> None:  # noqa: F811
    code = oic.data["nuv_voucher"][owner]["code"]
    left = nu.wait_until_gone(oic, oic.actors[owner], code)
    assert all(v.get("code") != code for v in left), left


@given("a seller with a stocked listing for new-user voucher orders")
def given_seller(oic) -> None:  # noqa: F811
    seller = s.register(oic, ORDER_SELLER, "seller")
    s.create_listing(oic, ORDER_LISTING, seller, 50, price=600000)


@given(
    parsers.parse(
        "a shop voucher of that seller limited to one use per buyer and to buyers without a completed order"
    )
)
def given_shop_voucher(oic) -> None:  # noqa: F811
    seller = oic.actors[ORDER_SELLER]
    code = av.fresh_code("NUVSHOP")
    body = av.create_body(code, scope=av.SHOP, quota=100, discount_value=10, max_discount=50000)
    body["perUserLimit"] = 1
    body["newUserOnly"] = True
    created = av.voucher_of(av.gw(seller.token, "CreateVoucher", body))
    assert created.get("code") == code, created
    oic.data["nuv_shop_code"] = code


@given(parsers.parse('a buyer "{name}" with no completed order'))
def given_buyer_fresh(oic, name: str) -> None:  # noqa: F811
    s.register(oic, name, "buyer")


@given(parsers.parse('a buyer "{name}" with a completed order'))
def given_buyer_completed(oic, name: str) -> None:  # noqa: F811
    buyer = s.register(oic, name, "buyer")
    seller = oic.actors[ORDER_SELLER]
    order_id = ol.place(oic, buyer, ORDER_LISTING, 1, "PAYMENT_METHOD_COD")
    ol.ship(oic, order_id, seller)
    s.ok(s.post(oic, buyer, s.ORDER, "ConfirmReceipt", {"id": order_id}))
    s.wait_status(oic, buyer, order_id, s.COMPLETED)


@when(parsers.parse('"{name}" lists her vouchers through the gateway'))
def when_list_mine(oic, name: str) -> None:  # noqa: F811
    oic.data["nuv_mine"] = nu.my_vouchers(oic, oic.actors[name])


@then(parsers.parse('the list has exactly one voucher and it is owned by "{name}"'))
def then_list_owned(oic, name: str) -> None:  # noqa: F811
    mine = oic.data["nuv_mine"]
    assert len(mine) == 1, mine
    assert mine[0]["code"] == oic.data["nuv_voucher"][name]["code"], mine
    assert mine[0].get("ownerUserId") in (None, "", oic.actors[name].user_id), mine


@then(parsers.parse('no listed voucher belongs to "{other}"'))
def then_not_other(oic, other: str) -> None:  # noqa: F811
    other_id = oic.actors[other].user_id
    other_code = oic.data["nuv_voucher"][other]["code"]
    for v in oic.data["nuv_mine"]:
        assert v.get("ownerUserId") != other_id, v
        assert v.get("code") != other_code, v


@when("an anonymous caller lists vouchers through the gateway")
def when_anonymous_list(oic) -> None:  # noqa: F811
    oic.responses["last"] = nu.list_my_response(oic, None)


@then("the gateway answers UNAUTHENTICATED")
def then_unauthenticated(oic) -> None:  # noqa: F811
    resp: httpx.Response = oic.responses["last"]
    assert resp.status_code == 401, f"{resp.status_code} {resp.text[:300]}"
    assert resp.json().get("code") == "unauthenticated", resp.text[:300]
    assert "vouchers" not in resp.json(), resp.text[:300]


@when("an anonymous visitor reads the public voucher list through the gateway")
def when_public_list(oic) -> None:  # noqa: F811
    found: list[dict] = []
    cursor = ""
    for _ in range(100):
        resp = s.post(
            oic, None, nu.VOUCHER, "ListVouchers", {"page": {"pageSize": 100, "cursor": cursor}}
        )
        body = s.ok(resp)
        found.extend(body.get("vouchers") or [])
        cursor = (body.get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    oic.data["nuv_public"] = found


@then("no public voucher is user-bound and none of those new-user vouchers is listed")
def then_public_clean(oic) -> None:  # noqa: F811
    held = {v["code"] for v in oic.data["nuv_voucher"].values()}
    assert held, "no new-user voucher was seeded"
    for v in oic.data["nuv_public"]:
        assert not v.get("ownerUserId"), v
        assert v.get("kind") != nu.KIND_NEW_USER, v
        assert v.get("code") not in held, v


@given(parsers.parse('a signed-in buyer "{name}"'))
def given_signed_in(oic, name: str) -> None:  # noqa: F811
    s.register(oic, name, "buyer")


@when(parsers.parse('"{name}" posts HasCompletedOrder to the gateway'))
def when_post_has_completed(oic, name: str) -> None:  # noqa: F811
    actor = oic.actors[name]
    oic.responses["last"] = s.post(
        oic, actor, s.ORDER, "HasCompletedOrder", {"buyerId": actor.user_id}
    )


@then("the gateway answers UNIMPLEMENTED with HTTP 404 or 501")
def then_unimplemented(oic) -> None:  # noqa: F811
    resp: httpx.Response = oic.responses["last"]
    assert resp.status_code in (404, 501), f"{resp.status_code} {resp.text[:300]}"
    assert "hasCompleted" not in resp.text, resp.text[:300]
