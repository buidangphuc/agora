"""Steps for the four `user_erasure.feature` files (change commerce-user-erasure).

A `UserErased` envelope is produced to the real `identity.events` topic; every observation is
through public gateway RPCs (GetOrder, GetReturnRequest, GetCart, ListPayoutHistory,
ListMyVouchers, CreateOrder with a voucher, Register + the `referral.events` stream) or the
broker (DLQ topics, consumer-group lag). No database is read or written.
"""

from __future__ import annotations

import time
import uuid

from pytest_bdd import given, parsers, then, when

from tests.e2e.step_definitions import referral_reward_steps as rr
from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import new_user_voucher_support as nuv
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as rma
from tests.e2e.support import referral_events_support as rev

MASK = ev.MASK
PRICE = 600_000
RECIPIENT = "Nguyễn Văn A"
PHONE = "0901234567"
STREET = "29 Liễu Giai"
RETURN_REASON = "Hàng bị vỡ"
ADDRESS_FIELDS = ("recipientName", "phone", "street", "ward", "district", "city")
KEPT_ORDER_FIELDS = (
    "id",
    "buyerId",
    "sellerId",
    "status",
    "totalAmount",
    "items",
    "trackingNumber",
)
GROUPS = {
    "order": ev.ORDER_GROUP,
    "payment": ev.PAYMENT_GROUP,
    "promotion": ev.PROMOTION_GROUP,
    "referral": ev.REFERRAL_GROUP,
}
DLQS = {
    "order": ev.ORDER_DLQ,
    "payment": ev.PAYMENT_DLQ,
    "promotion": ev.PROMOTION_DLQ,
    "referral": ev.REFERRAL_DLQ,
}
BANK_ACCOUNT = "0123456789"
BANK_NAME = "NGUYEN VAN A"
PAYOUT = 50_000


def _d(oic) -> dict:
    return oic.data.setdefault("erase", {})


def _user(oic, name: str) -> s.Actor:
    return oic.actors[name]


# ── order seeding ────────────────────────────────────────────────────────
@given('a seller "S" with two stocked listings')
def given_seller_two_listings(oic) -> None:
    seller = oic.actors.get("S") or s.register(oic, "S", "seller")
    s.create_listing(oic, "L1", seller, 200, price=PRICE)
    s.create_listing(oic, "L2", seller, 200, price=PRICE)


def _address(oic, buyer: s.Actor) -> None:
    s.ok(
        s.post(
            oic,
            buyer,
            s.ADDRESS,
            "CreateAddress",
            {
                "recipientName": RECIPIENT,
                "phone": PHONE,
                "street": STREET,
                "city": "Hà Nội",
                "ward": "Phường Liễu Giai",
                "district": "Quận Ba Đình",
                "isDefault": True,
            },
        )
    )


def _order_snapshot(oic, name: str) -> dict[str, dict]:
    buyer = _user(oic, name)
    return {oid: s.get_order(oic, buyer, oid) for oid in _d(oic)["orders"][name]}


@given(
    parsers.parse(
        'a buyer "{name}" with 3 orders, one return with reason "{reason}" and 2 cart items'
    )
)
def given_buyer_with_data(oic, name: str, reason: str) -> None:
    buyer = oic.actors.get(name) or s.register(oic, name, "buyer")
    _address(oic, buyer)
    seller = oic.actors["S"]
    ids = []
    for _ in range(3):
        ids.append(ol.place(oic, buyer, "L1", 1, "PAYMENT_METHOD_COD"))
    ol.ship(oic, ids[0], seller)
    ret = rma.new_return(oic, buyer, ids[0], 10_000, reason)
    assert ret.get("id"), ret
    s.post(oic, buyer, s.CART, "ClearCart", {})
    s.add_to_cart(oic, buyer, "L1", 1)
    s.add_to_cart(oic, buyer, "L2", 1)
    assert len(s.cart_items(oic, buyer)) == 2
    d = _d(oic)
    d.setdefault("orders", {})[name] = ids
    d.setdefault("returns", {})[name] = ret["id"]
    d.setdefault("before", {})[name] = _order_snapshot(oic, name)


def _publish(oic, name: str) -> None:
    uid = _user(oic, name).user_id
    d = _d(oic)
    d.setdefault("events", {})[name] = ev.publish_user_erased(uid)


@when(parsers.parse('a UserErased event for "{name}" is consumed'))
def when_event(oic, name: str) -> None:
    _publish(oic, name)


def _masked(oic, name: str) -> bool:
    buyer = _user(oic, name)
    orders = _d(oic)["orders"][name]
    return all(
        all(
            s.get_order(oic, buyer, oid).get("shippingAddress", {}).get(f) == MASK
            for f in ADDRESS_FIELDS
        )
        for oid in orders
    )


@then(
    parsers.parse(
        'every order of "{name}" reads recipient name, phone, street, ward, district and city "Đã xóa"'
    )
)
def then_orders_masked(oic, name: str) -> None:
    ev.wait_for(lambda: _masked(oic, name), bool, f"orders of {name} were not masked")


@then(parsers.parse('the return reason of "{name}" reads "Đã xóa"'))
def then_return_masked(oic, name: str) -> None:
    buyer = _user(oic, name)
    rid = _d(oic)["returns"][name]
    ev.wait_for(
        lambda: s.ok(s.post(oic, buyer, s.ORDER, "GetReturnRequest", {"id": rid}))
        .get("returnRequest", {})
        .get("reason"),
        lambda reason: reason == MASK,
        "return reason was not masked",
    )


@then(parsers.parse('"{name}" has 0 cart items'))
def then_cart_empty(oic, name: str) -> None:
    buyer = _user(oic, name)
    ev.wait_for(
        lambda: len(s.cart_items(oic, buyer)), lambda n: n == 0, f"cart of {name} is not empty"
    )


@then(
    parsers.parse(
        'the order ids, buyer, seller, totals, items and status of "{name}" are unchanged'
    )
)
def then_orders_kept(oic, name: str) -> None:
    before = _d(oic)["before"][name]
    after = _order_snapshot(oic, name)
    for oid, old in before.items():
        for field in KEPT_ORDER_FIELDS:
            assert after[oid].get(field) == old.get(field), f"order {oid} {field} changed"


@then(parsers.parse('the orders, return and cart of "{name}" are unchanged'))
def then_untouched(oic, name: str) -> None:
    buyer = _user(oic, name)
    before = _d(oic)["before"][name]
    for oid, old in before.items():
        got = s.get_order(oic, buyer, oid).get("shippingAddress", {})
        assert got == old.get("shippingAddress", {}), f"order {oid} of {name} was changed: {got}"
    rid = _d(oic)["returns"][name]
    reason = s.ok(s.post(oic, buyer, s.ORDER, "GetReturnRequest", {"id": rid})).get(
        "returnRequest", {}
    )
    assert reason.get("reason") == RETURN_REASON, f"return of {name} was changed: {reason}"
    assert len(s.cart_items(oic, buyer)) == 2, f"cart of {name} was changed"


# ── redelivery ───────────────────────────────────────────────────────────
@when(
    parsers.parse(
        'the same UserErased event for "{name}" and then one with a new event id are consumed'
    )
)
def when_redelivery(oic, name: str) -> None:
    d = _d(oic)
    if name in d.get("orders", {}):
        d["settled"] = {"order": _order_snapshot(oic, name)}
    uid = _user(oic, name).user_id
    ev.publish_user_erased(uid, event_id=d["events"][name])
    d["second_event"] = ev.publish_user_erased(uid)


@then(
    parsers.parse(
        'the {service} erasure consumer completes both without error and "{name}" is unchanged'
    )
)
def then_redelivery_noop(oic, service: str, name: str) -> None:
    d = _d(oic)
    ev.wait_group_caught_up(GROUPS[service])
    if service == "order":
        assert (
            _order_snapshot(oic, name) == d["settled"]["order"]
        ), "order rows changed on redelivery"
    elif service == "payment":
        assert _payout_state(oic, name) == d["settled_payment"], "payout rows changed on redelivery"
    ev.assert_dlq_empty(DLQS[service], d["events"][name].encode())
    ev.assert_dlq_empty(DLQS[service], d["second_event"].encode())


# ── malformed events ─────────────────────────────────────────────────────
@when("a UserErased event with an empty user_id and a record that does not decode are produced")
def when_malformed(oic) -> None:
    d = _d(oic)
    d["bad_event_id"] = ev.publish_user_erased("")
    d["bad_marker"] = f"e2e-garbage-{uuid.uuid4().hex}"
    ev.publish_garbage(d["bad_marker"])


@then(
    parsers.parse(
        'both records are written to the {service} DLQ and nothing is applied to "{name}"'
    )
)
def then_dlq(oic, service: str, name: str) -> None:
    d = _d(oic)
    topic = DLQS[service]
    assert ev.wait_dlq_record(
        topic, d["bad_event_id"].encode()
    ), f"empty-user_id event not in {topic}"
    assert ev.wait_dlq_record(topic, d["bad_marker"].encode()), f"undecodable record not in {topic}"
    ev.wait_group_caught_up(GROUPS[service])
    if service == "order":
        then_untouched(oic, name)
    elif service == "payment":
        assert _payout_state(oic, name) == d["before_payment"][name], "payouts changed"


# ── order: seller reads the masked order ─────────────────────────────────
@then(
    parsers.parse(
        'seller "{seller}" reads an order of "{name}" through GetOrder with recipient name "Đã xóa" and its original total and items'
    )
)
def then_seller_reads(oic, seller: str, name: str) -> None:
    seller_actor = oic.actors[seller]
    oid = _d(oic)["orders"][name][0]
    old = _d(oic)["before"][name][oid]
    order = ev.wait_for(
        lambda: s.get_order(oic, seller_actor, oid),
        lambda o: o.get("shippingAddress", {}).get("recipientName") == MASK,
        "seller still reads the unmasked recipient",
    )
    assert order.get("totalAmount") == old.get("totalAmount")
    assert order.get("items") == old.get("items")


# ── payment seeding ──────────────────────────────────────────────────────
def _payout_state(oic, name: str) -> dict:
    seller = _user(oic, name)
    rows = s.ok(s.post(oic, seller, s.PAYMENT, "ListPayoutHistory", {})).get("payouts", [])
    return {
        "payouts": sorted(
            (
                {
                    k: r.get(k)
                    for k in ("id", "bankCode", "amount", "status", "accountNumber", "accountName")
                }
                for r in rows
            ),
            key=lambda r: r["id"],
        ),
        "balance": p.balance(oic, seller),
    }


@given(parsers.parse('a seller "{name}" with a payout history of {count:d} pending payouts'))
def given_payout_seller(oic, name: str, count: int) -> None:
    stack.require_hold_wait()
    seller = s.register(oic, name, "seller")
    s.create_listing(oic, f"L-{name}", seller, 200, price=PRICE)
    oic.listings["L"] = oic.listings[f"L-{name}"]
    oic.actors["seller"] = seller
    b = p.buyer(oic, f"buyer-{name}")
    p.credit_and_age(oic, seller, b, f"L-{name}", PRICE)
    for _ in range(count):
        s.ok(
            s.post(
                oic,
                seller,
                s.PAYMENT,
                "RequestPayout",
                {
                    "amount": PAYOUT,
                    "bankCode": "VCB",
                    "accountNumber": BANK_ACCOUNT,
                    "accountName": BANK_NAME,
                },
            )
        )
    d = _d(oic)
    d.setdefault("before_payment", {})[name] = _payout_state(oic, name)
    assert len(d["before_payment"][name]["payouts"]) == count


@when(parsers.parse('a UserErased event for seller "{name}" is consumed'))
def when_event_seller(oic, name: str) -> None:
    _publish(oic, name)


def _payouts_masked(oic, name: str) -> bool:
    rows = _payout_state(oic, name)["payouts"]
    return bool(rows) and all(r["accountNumber"] == MASK and r["accountName"] == MASK for r in rows)


@then(
    parsers.parse(
        'the payout requests of "{name}" read account number and account name "Đã xóa" with ids, bank code, amount, status and ledger balance unchanged'
    )
)
def then_payouts_masked(oic, name: str) -> None:
    ev.wait_for(lambda: _payouts_masked(oic, name), bool, f"payouts of {name} were not masked")
    before = _d(oic)["before_payment"][name]
    after = _payout_state(oic, name)
    assert after["balance"] == before["balance"], "wallet balance changed"
    for old, new in zip(before["payouts"], after["payouts"], strict=True):
        for key in ("id", "bankCode", "amount", "status"):
            assert new[key] == old[key], f"payout {old['id']} {key} changed"
    _d(oic)["settled_payment"] = after


@then(parsers.parse('the payout requests of "{name}" still show their original bank details'))
def then_payouts_untouched(oic, name: str) -> None:
    assert _payout_state(oic, name) == _d(oic)["before_payment"][name], f"payouts of {name} changed"


@then(
    parsers.parse(
        'an admin lists the payout history of "{name}" and each entry shows account number and account name "Đã xóa" with its original amount and status'
    )
)
def then_admin_history(oic, name: str) -> None:
    admin = oic.actors.get("admin") or s.login_admin(oic)
    seller = _user(oic, name)
    before = _d(oic)["before_payment"][name]["payouts"]
    rows = ev.wait_for(
        lambda: s.ok(
            s.post(oic, admin, s.PAYMENT, "ListPayoutHistory", {"sellerId": seller.user_id})
        ).get("payouts", []),
        lambda r: bool(r)
        and all(x.get("accountNumber") == MASK and x.get("accountName") == MASK for x in r),
        "admin still reads unmasked bank details",
    )
    assert sorted((r["id"], r.get("amount"), r.get("status")) for r in rows) == sorted(
        (b["id"], b["amount"], b["status"]) for b in before
    )


# ── promotion ────────────────────────────────────────────────────────────
def _voucher_state(oic, name: str) -> list[dict]:
    return nuv.my_vouchers(oic, _user(oic, name))


@given(parsers.parse('"{name}" registered through the gateway and holds an active welcome voucher'))
def given_welcome_voucher(oic, name: str) -> None:
    nuv.register(oic, name)
    vouchers = nuv.wait_for_vouchers(oic, _user(oic, name))
    voucher = vouchers[0]
    assert voucher.get("active") is True, voucher
    _d(oic).setdefault("voucher", {})[name] = voucher["code"]


@given("a seller with a stocked listing for voucher checkouts")
def given_voucher_seller(oic) -> None:
    seller = s.register(oic, "VS", "seller")
    s.create_listing(oic, "VL", seller, 50, price=PRICE)


@then(parsers.parse('the welcome voucher of "{name}" is no longer active'))
def then_voucher_inactive(oic, name: str) -> None:
    code = _d(oic)["voucher"][name]
    ev.wait_for(
        lambda: [v for v in _voucher_state(oic, name) if v.get("code") == code and v.get("active")],
        lambda active: not active,
        f"voucher {code} of {name} is still active",
    )


@then(parsers.parse('"{name}" cannot check out with that voucher and no order is created'))
def then_voucher_refused(oic, name: str) -> None:
    buyer = _user(oic, name)
    code = _d(oic)["voucher"][name]
    s.post(oic, buyer, s.CART, "ClearCart", {})
    s.ensure_address(oic, buyer)
    s.add_to_cart(oic, buyer, "VL", 1)
    resp = s.checkout(oic, buyer, voucher=code)
    assert resp.status_code != 200, f"checkout with the erased voucher succeeded: {resp.text[:300]}"
    assert not s.buyer_orders(oic, buyer), "an order was created with the erased voucher"


# ── referral ─────────────────────────────────────────────────────────────
@given("a seller with a stocked listing for referral orders")
def given_ref_seller(oic) -> None:
    rr.given_seller(oic)


@given(
    parsers.parse(
        'an inviter "{name}" with a referral code and an invitee "{ref}" who registered with it and completed a first order'
    )
)
def given_inviter_with_reward(oic, name: str, ref: str) -> None:
    rr.given_inviter(oic, name)
    rr.when_register_with_code(oic, ref, name)
    referee = oic.actors[ref].user_id
    assert rr._tap(oic).wait_for(
        rev.ATTRIBUTED, referee, rr.ATTRIBUTION_WAIT_S
    ), "control: the invitee registered before the erasure was not attributed"
    rr._complete_order(oic, ref)
    d = _d(oic)
    d.setdefault("rewards", {})[name] = ev.wait_for(
        lambda: rr._rewards(oic, name),
        bool,
        "the inviter has no reward row after the invitee's first completed order",
        timeout=rr.CONSUME_WAIT_S,
    )


@then(parsers.parse('the rewards of "{name}" are still listed with the same ids and amounts'))
def then_rewards_kept(oic, name: str) -> None:
    before = _d(oic)["rewards"][name]
    after = rr._rewards(oic, name)
    assert [(r["id"], r["amount"]) for r in before] == [(r["id"], r["amount"]) for r in after]


@when(
    parsers.parse(
        'a new user "{late}" registers through the gateway with the code of "{inviter}" after its erasure'
    )
)
def when_late_register(oic, late: str, inviter: str) -> None:
    rr._tap(oic)
    rr._register(oic, late, oic.data["ref_codes"][inviter])
    oic.data.setdefault("ref_registered_at", {})[late] = time.time()


@then(parsers.parse('"{late}" is not attributed to "{inviter}"'))
def then_not_attributed(oic, late: str, inviter: str) -> None:
    tap = rr._tap(oic)
    tap.drain(rr.CONSUME_SETTLE_S + 4)
    referee = oic.actors[late].user_id
    assert not tap.matching(rev.ATTRIBUTED, referee), f"{late} was attributed after the erasure"
    summary = rr._summary(oic, inviter)
    assert int(summary.get("invitedCount") or 0) == 1, summary
