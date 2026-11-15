"""Steps for order/user_obligations.feature and payment/payout_obligations.feature.

Change commerce-user-erasure. The two obligation RPCs are service-only: they are called with
grpcurl and `x-principal-*` headers (see `erasure_events_support`), and through the gateway to
prove the gateway answers 501. State is seeded only through public gateway RPCs.
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import plp_support as p
from tests.e2e.support import prm_rma_support as rma

PRICE = 600_000
SERVICE_SCOPE = {"order": "order.internal.read", "payment": "payment.internal.read"}
TARGET = {"order": ev.ORDER_OBLIGATIONS, "payment": ev.PAYMENT_OBLIGATIONS}
METHOD = {"order": "GetUserObligations", "payment": "GetPayoutObligations"}
GATEWAY_PATH = {
    "order": "/platform.order.v1.OrderService/GetUserObligations",
    "payment": "/platform.payment.v1.PaymentService/GetPayoutObligations",
}
PENDING_PAYOUT = 50_000


def _shop(oic, seller_name: str = "S", listing: str = "L", stock: int = 200) -> None:
    seller = oic.actors.get(seller_name) or s.register(oic, seller_name, "seller")
    s.create_listing(oic, listing, seller, stock, price=PRICE)


def _buyer(oic, name: str, role: str = "buyer") -> s.Actor:
    actor = oic.actors.get(name) or s.register(oic, name, role)
    s.ensure_address(oic, actor)
    return actor


def _place_cod(oic, buyer: s.Actor, listing: str) -> str:
    return ol.place(oic, buyer, listing, 1, "PAYMENT_METHOD_COD")


def _seller_of(oic, listing: str) -> s.Actor:
    return oic.listings[listing]["seller"]


def _move(oic, order_id: str, buyer: s.Actor, listing: str, state: str) -> None:
    seller = _seller_of(oic, listing)
    if state == "shipped":
        ol.ship(oic, order_id, seller)
    elif state == "completed":
        ol.ship(oic, order_id, seller)
        s.ok(s.post(oic, buyer, s.ORDER, "ConfirmReceipt", {"id": order_id}))
        s.wait_status(oic, buyer, order_id, s.COMPLETED)
    elif state == "cancelled":
        s.ok(s.post(oic, buyer, s.ORDER, "CancelOrder", {"id": order_id}))
        s.wait_status(oic, buyer, order_id, s.CANCELLED)
    elif state == "paid":
        p.pay(oic, buyer, order_id)
        s.wait_status(oic, buyer, order_id, s.PAID)
    else:
        assert state == "pending", state


# ── order obligations: seeding ───────────────────────────────────────────
@given(parsers.parse('a seller "{name}" with a stocked listing "{listing}"'))
def given_seller_listing(oic, name: str, listing: str) -> None:
    _shop(oic, name, listing)


@given(
    parsers.parse(
        'a user "{name}" who is both a buyer and a seller with a stocked listing "{listing}"'
    )
)
def given_dual_user(oic, name: str, listing: str) -> None:
    user = _buyer(oic, name, role="seller")
    s.create_listing(oic, listing, user, 200, price=PRICE)


@given(parsers.parse('a buyer "{name}"'))
def given_buyer(oic, name: str) -> None:
    _buyer(oic, name)


@given(parsers.parse('"{name}" is buyer of {count:d} {state} orders of "{listing}"'))
@given(parsers.parse('"{name}" is buyer of {count:d} {state} order of "{listing}"'))
def given_buyer_orders(oic, name: str, count: int, state: str, listing: str) -> None:
    buyer = oic.actors[name]
    for _ in range(count):
        order_id = _place_cod(oic, buyer, listing)
        _move(oic, order_id, buyer, listing, state)
        oic.data.setdefault("obl_orders", {}).setdefault((name, state), []).append(order_id)


@given(
    parsers.parse(
        '"{seller}" is seller of {count:d} {state} order of "{listing}" bought by "{buyer}"'
    )
)
def given_seller_orders(oic, seller: str, count: int, state: str, listing: str, buyer: str) -> None:
    b = oic.actors[buyer]
    for _ in range(count):
        order_id = _place_cod(oic, b, listing)
        _move(oic, order_id, b, listing, state)


@given(parsers.parse('"{name}" has a pending return on its shipped order'))
def given_pending_return(oic, name: str) -> None:
    buyer = oic.actors[name]
    order_id = oic.data["obl_orders"][(name, "shipped")][-1]
    ret = rma.new_return(oic, buyer, order_id, 10_000)
    assert ret.get("id"), ret


@given(parsers.parse('"{name}" has a rejected return on its completed order'))
def given_rejected_return(oic, name: str) -> None:
    buyer = oic.actors[name]
    order_id = oic.data["obl_orders"][(name, "completed")][-1]
    ret = rma.new_return(oic, buyer, order_id, 10_000)
    seller = _seller_of(oic, "L")
    s.ok(rma.set_return_status(oic, seller, ret["id"], rma.REJECTED))


@given(
    parsers.parse('"{name}" has a refunded return on an online-paid completed order of "{listing}"')
)
def given_refunded_return(oic, name: str, listing: str) -> None:
    buyer = oic.actors[name]
    order_id, _ = p.place_and_pay(oic, buyer, listing)
    seller = _seller_of(oic, listing)
    ol.ship(oic, order_id, seller)
    s.ok(s.post(oic, buyer, s.ORDER, "ConfirmReceipt", {"id": order_id}))
    s.wait_status(oic, buyer, order_id, s.COMPLETED)
    ret = rma.new_return(oic, buyer, order_id, 10_000)
    rma.approve_and_refund(oic, seller, ret["id"])


# ── payout obligations: seeding ──────────────────────────────────────────
@given(parsers.parse('a seller "{name}" with a wallet credit that has aged past the hold window'))
def given_aged_credit(oic, name: str) -> None:
    stack.require_hold_wait()
    seller = p.seller_with_listing(oic, PRICE, "L")
    oic.actors[name] = seller
    oic.actors["seller"] = seller
    b = p.buyer(oic, "b1")
    oic.data["aged_credit"] = p.credit_and_age(oic, seller, b, "L", PRICE)


@given(parsers.parse('"{name}" has a fresh wallet credit inside the hold window'))
def given_fresh_credit(oic, name: str) -> None:
    seller = oic.actors[name]
    b = p.buyer(oic, "b2")
    p.place_and_pay(oic, b, "L")
    ev.wait_for(
        lambda: len([e for e in p.ledger(oic, seller) if e.get("type") == p.SETTLEMENT]),
        lambda n: n >= 2,
        "the second settlement credit did not appear",
        timeout=p.SETTLE_WAIT_S,
    )


@given(parsers.parse('"{name}" has {count:d} pending bank payouts of 50000'))
def given_pending_payouts(oic, name: str, count: int) -> None:
    seller = oic.actors[name]
    for _ in range(count):
        s.ok(p.bank_payout(oic, seller, PENDING_PAYOUT))


@given(parsers.parse('a user "{name}" with no orders, returns, wallet entries or payouts'))
def given_stranger(oic, name: str) -> None:
    oic.actors[name] = s.Actor(f"nobody-{uuid.uuid4().hex[:8]}", "", "buyer", str(uuid.uuid4()))


# ── the calls ────────────────────────────────────────────────────────────
def _call(oic, kind: str, name: str, principal: str | None, scope: str | None = None) -> None:
    uid = oic.actors[name].user_id
    code, body = ev.call_internal(
        TARGET[kind],
        METHOD[kind],
        {"userId": uid},
        principal,
        scope or SERVICE_SCOPE[kind],
        principal_id=uid if principal == "user" else "",
    )
    oic.data["obl_result"] = (code, body)


@when(parsers.parse('the identity service calls GetUserObligations for "{name}"'))
def when_order_service_call(oic, name: str) -> None:
    _call(oic, "order", name, "service")


@when(parsers.parse('the identity service calls GetPayoutObligations for "{name}"'))
def when_payment_service_call(oic, name: str) -> None:
    _call(oic, "payment", name, "service")


@when(parsers.parse('a signed-in user principal calls {method} for "{name}"'))
def when_user_call(oic, method: str, name: str) -> None:
    _call(oic, "order" if method == "GetUserObligations" else "payment", name, "user")


@when(parsers.parse('a service principal holding only order.read calls {method} for "{name}"'))
def when_wrong_scope_call(oic, method: str, name: str) -> None:
    _call(oic, "order" if method == "GetUserObligations" else "payment", name, "service-no-scope")


@when(parsers.parse('a call without a principal reaches {method} for "{name}"'))
def when_anonymous_call(oic, method: str, name: str) -> None:
    _call(oic, "order" if method == "GetUserObligations" else "payment", name, None)


@when(parsers.parse("a browser client calls {service}/{method} through the gateway {how}"))
def when_gateway_call(oic, service: str, method: str, how: str) -> None:
    kind = "order" if method == "GetUserObligations" else "payment"
    headers = {"Content-Type": "application/json"}
    if how == "with a token":
        buyer = oic.actors.get("U") or s.register(oic, "U", "buyer")
        headers["Authorization"] = f"bearer {buyer.token}"
    resp = httpx.post(
        f"{oic.gateway}{GATEWAY_PATH[kind]}", json={"userId": "x"}, headers=headers, timeout=30
    )
    oic.data["obl_http"] = resp.status_code


# ── assertions ───────────────────────────────────────────────────────────
def _body(oic) -> dict:
    code, body = oic.data["obl_result"]
    assert code == "ok", f"expected an OK response, got {code}: {body}"
    return body


@then(
    parsers.parse(
        "the obligations are open buyer orders {buyer:d}, open seller orders {seller:d}, "
        "open returns {returns:d}"
    )
)
def then_order_counts(oic, buyer: int, seller: int, returns: int) -> None:
    body = _body(oic)
    got = (
        int(body.get("openBuyerOrders", 0)),
        int(body.get("openSellerOrders", 0)),
        int(body.get("openReturns", 0)),
    )
    assert got == (
        buyer,
        seller,
        returns,
    ), f"obligations {got}, want {(buyer, seller, returns)}: {body}"


@then("the response is OK with all three counts 0")
def then_all_zero(oic) -> None:
    then_order_counts(oic, 0, 0, 0)


@then(
    parsers.parse(
        "the payout obligations are held {held}, available {avail}, pending payouts {pending:d}"
    )
)
def then_payout_numbers(oic, held: str, avail: str, pending: int) -> None:
    body = _body(oic)
    seller = oic.actors["S"]
    credits = [e for e in p.ledger(oic, seller) if e.get("type") == p.SETTLEMENT]
    assert len(credits) == 2, credits
    fresh = max(credits, key=lambda e: p.parse_ts(e["createdAt"]))
    want_held = int(fresh["amount"])
    want_avail = max(0, p.balance(oic, seller) - want_held)
    assert (held, avail) == ("the fresh credit", "the balance minus held"), (held, avail)
    got = (
        int(body.get("availableBalance", 0)),
        int(body.get("heldBalance", 0)),
        int(body.get("pendingPayouts", 0)),
    )
    assert got == (
        want_avail,
        want_held,
        pending,
    ), f"{got}, want {(want_avail, want_held, pending)}: {body}"


@then("the payout obligations are zero")
def then_payout_zero(oic) -> None:
    body = _body(oic)
    got = (
        int(body.get("availableBalance", 0)),
        int(body.get("heldBalance", 0)),
        int(body.get("pendingPayouts", 0)),
    )
    assert got == (0, 0, 0), f"{got}: {body}"


@then(parsers.parse('the call fails with "{code}"'))
def then_fails(oic, code: str) -> None:
    got, body = oic.data["obl_result"]
    assert got == code, f"call answered {got!r} ({str(body)[:200]}), want {code!r}"


@then("the gateway answers HTTP 501")
def then_501(oic) -> None:
    assert oic.data["obl_http"] == 501, f"gateway answered {oic.data['obl_http']}"
