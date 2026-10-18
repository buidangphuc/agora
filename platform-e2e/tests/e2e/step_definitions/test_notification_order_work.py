# ruff: noqa: F811
"""Binds notification/order_work.feature (notification-order-work, q10-order-lifecycle).

Black box through the gateway with real, distinct users: orders are driven through team-order,
and each user's feed is read with ListNotifications. Every "no notification" assertion sits in a
scenario that also asserts the positive notification for the same event, so it cannot pass
vacuously.
"""

from __future__ import annotations

from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401  (fixture)
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import order_lifecycle_support as ol
from tests.e2e.support import order_work_support as nw
from tests.e2e.support import prm_rma_support as r

scenarios("notification/order_work.feature")

_METHOD = {"MOCK_MOMO": "PAYMENT_METHOD_MOCK_MOMO", "COD": "PAYMENT_METHOD_COD"}


def _link(oic, template: str) -> str:
    return template.replace("{order_id}", oic.orders.get("order", ""))


def _edit_link(oic, listing: str) -> str:
    return f"/seller/{oic.listings[listing]['id']}/edit"


# ── orders ───────────────────────────────────────────────────────────────
@given(parsers.parse('a seller "{name}" with a listing "{listing}" priced {price:d}'))
def given_seller_priced(oic, name, listing, price):
    seller = oic.actors.get(name) or s.register(oic, name, "seller")
    s.create_listing(oic, listing, seller, 50, price=price)


@when(parsers.parse('"{b}" places a {method} order for {qty:d} of "{listing}"'))
def when_place(oic, b, method, qty, listing):
    buyer = oic.actors[b]
    oic.orders["order"] = ol.place(oic, buyer, listing, qty, _METHOD[method])
    oic.data["order:buyer"] = buyer


@given(parsers.parse('"{b}" has a shipped COD order for "{listing}"'))
def given_shipped_cod(oic, b, listing):
    buyer = oic.actors[b]
    seller = oic.listings[listing]["seller"]
    oic.orders["order"] = ol.place(oic, buyer, listing, 1, "PAYMENT_METHOD_COD")
    oic.data["order:buyer"] = buyer
    ol.ship(oic, oic.orders["order"], seller)


@when(parsers.parse('"{b}" pays the order'))
def when_pays(oic, b):
    buyer = oic.actors[b]
    tx_id = s.open_payment(oic, buyer, oic.orders["order"])
    s.ok(s.settle_payment(oic, buyer, tx_id))
    s.wait_status(oic, buyer, oic.orders["order"], s.PAID)


@when(parsers.parse('"{b}" requests a return of {amount:d} on the order'))
def when_return(oic, b, amount):
    resp = r.request_return(oic, oic.actors[b], oic.orders["order"], amount)
    s.ok(resp)


@when(parsers.parse('"{b}" confirms receipt of the order'))
def when_confirm(oic, b):
    s.ok(s.post(oic, oic.actors[b], s.ORDER, "ConfirmReceipt", {"id": oic.orders["order"]}))


@when("the payment deadline passes without payment")
def when_deadline_passes(oic):
    ol.wait_cancelled(oic, oic.data["order:buyer"], oic.orders["order"], ol.EXPIRY_WAIT_S)


@then(parsers.parse('the stock of "{listing}" is {n:d}'))
def then_stock(oic, listing, n):
    s.wait_stock(oic, listing, n, timeout=15)


# ── feeds ────────────────────────────────────────────────────────────────
@then(
    parsers.parse(
        '"{who}" has exactly one "{title}" notification linking to "{link}"',
    )
)
def then_exactly_one(oic, who, title, link):
    actor, url = oic.actors[who], _link(oic, link)
    found = nw.wait_count(oic, actor, title=title, link=url, timeout=nw.APPEAR_S)
    assert found, f'{who} never received "{title}" -> {url}; feed={nw.feed(oic, actor)}'
    found = nw.settle(oic, actor, title=title, link=url)
    assert len(found) == 1, f"{who}: expected exactly one {title!r} for {url}, got {found}"


@then(parsers.parse('"{who}" has no "{title}" notification linking to "{link}"'))
def then_none_titled_linked(oic, who, title, link):
    url = _link(oic, link)
    found = nw.settle(oic, oic.actors[who], title=title, link=url)
    assert found == [], f"{who} unexpectedly has {title!r} for {url}: {found}"


@then(parsers.parse('"{who}" has no notification linking to "{link}"'))
def then_none_linked(oic, who, link):
    url = _link(oic, link)
    found = nw.settle(oic, oic.actors[who], title=None, link=url)
    assert found == [], f"{who} unexpectedly has a notification for {url}: {found}"


@then(parsers.parse('"{who}" has no notification titled "{title}" for the order'))
def then_none_titled(oic, who, title):
    oid = oic.orders["order"]
    found = [
        n
        for n in nw.feed(oic, oic.actors[who])
        if n.get("title") == title and oid in (n.get("linkUrl") or "")
    ]
    assert found == [], f"{who} unexpectedly has {title!r} for the order: {found}"


@then(
    parsers.parse(
        '"{who}" has exactly one "{title}" notification linking to the edit page of "{l}"'
    )
)
def then_low_stock_one(oic, who, title, l):  # noqa: E741
    actor, url = oic.actors[who], _edit_link(oic, l)
    found = nw.wait_count(oic, actor, title=title, link=url)
    assert found, f'{who} never received "{title}" -> {url}; feed={nw.feed(oic, actor)}'
    found = nw.settle(oic, actor, title=title, link=url)
    assert len(found) == 1, f"{who}: expected exactly one {title!r} for {url}, got {found}"


@then(
    parsers.parse(
        '"{who}" still has exactly one "{title}" notification linking to the edit page of "{l}"'
    )
)
def then_low_stock_still_one(oic, who, title, l):  # noqa: E741
    actor, url = oic.actors[who], _edit_link(oic, l)
    found = nw.settle(oic, actor, title=title, link=url)
    assert len(found) == 1, f"{who}: expected still exactly one {title!r} for {url}, got {found}"
