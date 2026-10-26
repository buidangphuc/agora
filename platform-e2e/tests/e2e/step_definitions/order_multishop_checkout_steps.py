"""Steps of order/multishop_checkout.feature (change order-multishop-checkout).

Black box through the gateway: PreviewCheckout, CreateOrder, GetOrder, CancelOrder, payments and
VoucherService. Every actor is a separate registered user.
"""

from __future__ import annotations

import time
from typing import Any

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import multishop_support as ms
from tests.e2e.support import oic_order_support as s
from tests.e2e.support import voucher_scope_support as vs


@pytest.fixture
def msw():
    return s.OicWorld()


def _d(w: s.OicWorld) -> dict[str, Any]:
    return w.data.setdefault("ms", {"req": {}, "preview": {}, "placed": {}, "vouchers": {}})


def _req(w: s.OicWorld, label: str) -> dict[str, Any]:
    return _d(w)["req"].setdefault(label, {"shop_vouchers": [], "platform": "", "item_ids": None})


def _shop_id(w: s.OicWorld, shop: str) -> str:
    return w.actors[f"shop-{shop}"].user_id


def _admin(w: s.OicWorld) -> s.Actor:
    return w.actors.get("admin") or s.login_admin(w)


def _preview_ok(w: s.OicWorld, label: str) -> dict[str, Any]:
    resp = _d(w)["preview"][label]
    return s.ok(resp)


def _placed(w: s.OicWorld, label: str) -> list[dict[str, Any]]:
    return _d(w)["placed"][label]


def _placed_by_shop(w: s.OicWorld, label: str, shop: str) -> dict[str, Any]:
    sid = _shop_id(w, shop)
    found = [o for o in _placed(w, label) if o.get("sellerId") == sid]
    assert len(found) == 1, f"no single placed order for shop {shop}: {_placed(w, label)}"
    return found[0]


@given(parsers.parse('a shop "{shop}" selling a listing priced {price:d}'))
def shop_listing(msw: s.OicWorld, shop: str, price: int) -> None:
    seller = s.register(msw, "shop", "seller")
    msw.actors[f"shop-{shop}"] = seller
    s.create_listing(msw, f"shop-{shop}", seller, 50, price=price)


@given(parsers.parse('a buyer "{label}" in "{city}" with 1 of each shop\'s listing in the cart'))
def buyer_with_cart(msw: s.OicWorld, label: str, city: str) -> None:
    buyer = s.register(msw, label, "buyer")
    msw.actors[label] = buyer
    ms.create_address(msw, buyer, city)
    s.post(msw, buyer, s.CART, "ClearCart", {})
    for name in [k for k in msw.listings if k.startswith("shop-")]:
        s.add_to_cart(msw, buyer, name, 1)
    assert len(s.cart_items(msw, buyer)) == len([k for k in msw.listings if k.startswith("shop-")])


@given(parsers.parse('a shop voucher "{name}" of shop "{shop}" worth {pct:d} percent off'))
def shop_voucher_percent(msw: s.OicWorld, name: str, shop: str, pct: int) -> None:
    code = ms.fresh_code("MSV")
    _d(msw)["vouchers"][name] = ms.seed_voucher(
        msw, msw.actors[f"shop-{shop}"], "VOUCHER_SCOPE_SHOP", code, percent=pct
    )


@given(parsers.parse('a shop voucher "{name}" of shop "{shop}" worth {amount:d} off'))
def shop_voucher_fixed(msw: s.OicWorld, name: str, shop: str, amount: int) -> None:
    code = ms.fresh_code("MSV")
    _d(msw)["vouchers"][name] = ms.seed_voucher(
        msw, msw.actors[f"shop-{shop}"], "VOUCHER_SCOPE_SHOP", code, fixed=amount
    )


@given(parsers.parse('a platform voucher "{name}" worth {amount:d} off with min spend {minimum:d}'))
def platform_voucher(msw: s.OicWorld, name: str, amount: int, minimum: int) -> None:
    code = ms.fresh_code("MSP")
    _d(msw)["vouchers"][name] = ms.seed_voucher(
        msw, _admin(msw), "VOUCHER_SCOPE_PLATFORM", code, fixed=amount, min_spend=minimum
    )


@given(parsers.parse('"{label}" applies shop voucher "{name}" to shop "{shop}"'))
@when(parsers.parse('"{label}" applies shop voucher "{name}" to shop "{shop}"'))
def apply_shop_voucher(msw: s.OicWorld, label: str, name: str, shop: str) -> None:
    _req(msw, label)["shop_vouchers"].append((_shop_id(msw, shop), _d(msw)["vouchers"][name]))


@given(parsers.parse('"{label}" applies platform voucher "{name}"'))
@when(parsers.parse('"{label}" applies platform voucher "{name}"'))
def apply_platform_voucher(msw: s.OicWorld, label: str, name: str) -> None:
    _req(msw, label)["platform"] = _d(msw)["vouchers"][name]


@when(parsers.parse('"{label}" clears its voucher request'))
def clear_vouchers(msw: s.OicWorld, label: str) -> None:
    _req(msw, label).update({"shop_vouchers": [], "platform": ""})


@when(parsers.parse('"{label}" previews the checkout'))
def preview(msw: s.OicWorld, label: str) -> None:
    _d(msw)["preview"][label] = ms.preview(msw, msw.actors[label], _req(msw, label))


@when("an anonymous visitor previews the checkout")
def anonymous_preview(msw: s.OicWorld) -> None:
    _d(msw)["preview"]["anonymous"] = ms.preview(msw, None, {})


@when(parsers.parse('"{label}" previews with the cart line ids of "{owner}"'))
def preview_with_ids(msw: s.OicWorld, label: str, owner: str) -> None:
    ids = [i["id"] for i in s.cart_items(msw, msw.actors[owner])]
    assert ids, "the cart owner has no lines"
    _d(msw)["cart_before"] = ids
    _d(msw)["preview"][label] = ms.preview(msw, msw.actors[label], {"item_ids": ids})


@when(parsers.parse('"{label}" places the order with the same selection'))
def place(msw: s.OicWorld, label: str) -> None:
    resp = ms.place(msw, msw.actors[label], _req(msw, label))
    orders = s.ok(resp).get("orders", [])
    assert orders, f"CreateOrder returned no orders: {resp.text[:300]}"
    _d(msw)["placed"][label] = orders


@when(parsers.parse('"{label}" pays the first placed order'))
def pay_first(msw: s.OicWorld, label: str) -> None:
    order = _placed(msw, label)[0]
    s.pay_order(msw, msw.actors[label], order["id"])
    s.wait_status(msw, msw.actors[label], order["id"], s.PAID)


@when(parsers.parse('"{label}" pays the second placed order'))
def pay_second(msw: s.OicWorld, label: str) -> None:
    order = _placed(msw, label)[1]
    s.pay_order(msw, msw.actors[label], order["id"])
    s.wait_status(msw, msw.actors[label], order["id"], s.PAID)


@when(parsers.parse('"{label}" cancels the first placed order'))
def cancel_first(msw: s.OicWorld, label: str) -> None:
    order = _placed(msw, label)[0]
    s.ok(s.post(msw, msw.actors[label], s.ORDER, "CancelOrder", {"id": order["id"]}))


@then(
    parsers.parse(
        "the preview holds {count:d} groups each with items subtotal {sub:d}, "
        "shipping fee {fee:d} and no free shipping"
    )
)
def groups_quoted(msw: s.OicWorld, count: int, sub: int, fee: int) -> None:
    body = _preview_ok(msw, "b1")
    groups = body.get("groups", [])
    assert len(groups) == count, body
    assert {g["sellerId"] for g in groups} == {
        a.user_id for k, a in msw.actors.items() if k.startswith("shop-")
    }, body
    for g in groups:
        assert ms.n(g.get("itemsSubtotal")) == sub, g
        assert ms.n(g.get("shippingFee")) == fee, g
        assert not g.get("isFreeShipping", False), g
    assert body.get("shippingQuoted") is True, body


@then(parsers.parse("the preview total amount is {total:d}"))
def preview_total(msw: s.OicWorld, total: int) -> None:
    assert ms.n(_preview_ok(msw, "b1").get("totalAmount")) == total


@then(parsers.parse("the preview total amount is below {total:d}"))
def preview_total_below(msw: s.OicWorld, total: int) -> None:
    got = ms.n(_preview_ok(msw, "b1").get("totalAmount"))
    assert 0 < got < total, got


@then(parsers.parse("the preview total amount equals the sum of the {count:d} placed orders' total amount"))
def preview_equals_placed(msw: s.OicWorld, count: int) -> None:
    placed = _placed(msw, "b1")
    assert len(placed) == count, placed
    quoted = ms.n(_preview_ok(msw, "b1").get("totalAmount"))
    assert quoted == sum(ms.n(o.get("totalAmount")) for o in placed), (quoted, placed)


@then(parsers.parse('the preview of "{label}" succeeds'))
def preview_succeeds(msw: s.OicWorld, label: str) -> None:
    assert len(_preview_ok(msw, label).get("groups", [])) >= 1


@then(parsers.parse('the preview of "{label}" is refused with "{code}" and message "{message}"'))
def preview_refused(msw: s.OicWorld, label: str, code: str, message: str) -> None:
    resp = _d(msw)["preview"][label]
    assert resp.status_code != 200, resp.text[:300]
    body = resp.json()
    assert (body.get("code"), body.get("message")) == (code, message), resp.text[:300]


@then(parsers.parse('the cart of "{label}" still holds {count:d} line'))
def cart_unchanged(msw: s.OicWorld, label: str, count: int) -> None:
    ids = [i["id"] for i in s.cart_items(msw, msw.actors[label])]
    assert len(ids) == count, ids
    assert set(_d(msw)["cart_before"]) <= set(ids), (ids, _d(msw)["cart_before"])


@then(parsers.parse('the anonymous preview is rejected as "{code}"'))
def anonymous_rejected(msw: s.OicWorld, code: str) -> None:
    resp = _d(msw)["preview"]["anonymous"]
    assert s.code_of(resp) == code, resp.text[:300]


@then(
    parsers.parse(
        'the group of shop "{a}" has shop discount {da:d} and the group of shop "{b}" has {db:d}'
    )
)
def shop_discounts(msw: s.OicWorld, a: str, da: int, b: str, db: int) -> None:
    groups = ms.groups_by_seller(_preview_ok(msw, "b1"))
    assert ms.n(groups[_shop_id(msw, a)].get("shopDiscountAmount")) == da, groups
    assert ms.n(groups[_shop_id(msw, b)].get("shopDiscountAmount")) == db, groups


@then(parsers.parse('the placed order of shop "{shop}" stores voucher "{name}" with discount {amount:d}'))
def placed_voucher(msw: s.OicWorld, shop: str, name: str, amount: int) -> None:
    order = _placed_by_shop(msw, "b1", shop)
    got = s.get_order(msw, msw.actors["b1"], order["id"])
    assert got.get("voucherCode") == _d(msw)["vouchers"][name], got
    assert ms.n(got.get("discountAmount")) == amount, got


def _voucher_quote(msw: s.OicWorld, name: str) -> dict[str, Any]:
    code = _d(msw)["vouchers"][name]
    found = [v for v in _preview_ok(msw, "b1").get("vouchers", []) if v.get("code") == code]
    assert len(found) == 1, _preview_ok(msw, "b1")
    return found[0]


@then(parsers.parse('the preview reports voucher "{name}" valid=false with a reason'))
def voucher_invalid(msw: s.OicWorld, name: str) -> None:
    quote = _voucher_quote(msw, name)
    assert not quote.get("valid", False), quote
    assert quote.get("reason"), quote


@then(parsers.parse('the preview reports voucher "{name}" valid=true'))
def voucher_valid(msw: s.OicWorld, name: str) -> None:
    assert _voucher_quote(msw, name).get("valid") is True


@then("no group of the preview carries a shop discount")
def no_shop_discount(msw: s.OicWorld) -> None:
    body = _preview_ok(msw, "b1")
    assert body.get("groups"), body
    assert all(ms.n(g.get("shopDiscountAmount")) == 0 for g in body["groups"]), body


@then("no group of the preview carries a platform discount")
def no_platform_discount(msw: s.OicWorld) -> None:
    body = _preview_ok(msw, "b1")
    assert body.get("groups"), body
    assert all(ms.n(g.get("platformDiscountAmount")) == 0 for g in body["groups"]), body


@then(parsers.parse("every group of the preview carries a platform discount of {amount:d}"))
def platform_discount_each(msw: s.OicWorld, amount: int) -> None:
    body = _preview_ok(msw, "b1")
    assert body.get("groups"), body
    assert all(ms.n(g.get("platformDiscountAmount")) == amount for g in body["groups"]), body


@then(
    parsers.parse(
        'each of the {count:d} placed orders has platform discount {amount:d}, voucher "{name}", '
        "total amount {total:d} and one shared non-empty checkout id"
    )
)
def placed_platform_split(msw: s.OicWorld, count: int, amount: int, name: str, total: int) -> None:
    placed = _placed(msw, "b1")
    assert len(placed) == count, placed
    for order in placed:
        got = s.get_order(msw, msw.actors["b1"], order["id"])
        assert ms.n(got.get("platformDiscountAmount")) == amount, got
        assert got.get("platformVoucherCode") == _d(msw)["vouchers"][name], got
        assert ms.n(got.get("totalAmount")) == total, got
    ids = {s.get_order(msw, msw.actors["b1"], o["id"]).get("checkoutId") for o in placed}
    assert len(ids) == 1 and next(iter(ids)), ids


@then(parsers.parse("the {count:d} placed orders carry one shared non-empty checkout id"))
def shared_checkout_id(msw: s.OicWorld, count: int) -> None:
    placed = _placed(msw, "b1")
    assert len(placed) == count, placed
    ids = {o.get("checkoutId") for o in placed}
    assert len(ids) == 1 and next(iter(ids)), placed
    _d(msw)["checkout_id"] = next(iter(ids))


@then("GetOrder returns that checkout id for each placed order")
def get_order_checkout_id(msw: s.OicWorld) -> None:
    for order in _placed(msw, "b1"):
        got = s.get_order(msw, msw.actors["b1"], order["id"])
        assert got.get("checkoutId") == _d(msw)["checkout_id"], got


@then(parsers.parse('the used count of voucher "{name}" becomes {count:d}'))
def used_becomes(msw: s.OicWorld, name: str, count: int) -> None:
    code = _d(msw)["vouchers"][name]
    admin = _admin(msw)
    deadline = time.monotonic() + 25
    used = ms.voucher_used(msw, admin, code)
    while used != count and time.monotonic() < deadline:
        time.sleep(0.5)
        used = ms.voucher_used(msw, admin, code)
    assert used == count, used


@then("both placed orders are paid")
def both_paid(msw: s.OicWorld) -> None:
    for order in _placed(msw, "b1"):
        assert s.wait_status(msw, msw.actors["b1"], order["id"], s.PAID) == s.PAID


@then(parsers.parse('the used count of voucher "{name}" stays {count:d}'))
def used_stays(msw: s.OicWorld, name: str, count: int) -> None:
    code = _d(msw)["vouchers"][name]
    admin = _admin(msw)
    time.sleep(6)
    assert ms.voucher_used(msw, admin, code) == count


@then("the first placed order is cancelled")
def first_cancelled(msw: s.OicWorld) -> None:
    order = _placed(msw, "b1")[0]
    assert s.order_status(msw, msw.actors["b1"], order["id"]) == s.CANCELLED


@then("the platform voucher hold of the checkout is still held")
def platform_hold_kept(msw: s.OicWorld) -> None:
    checkout_id = _placed(msw, "b1")[0].get("checkoutId")
    assert checkout_id, _placed(msw, "b1")
    rows = vs.reservation_rows(f"platform:{checkout_id}")
    assert len(rows) == 1, rows
    assert rows[0][2].lower() not in ("released", "cancelled", "expired"), rows


@then(parsers.parse("the second placed order keeps total amount {total:d} and platform discount {amount:d}"))
def sibling_unchanged(msw: s.OicWorld, total: int, amount: int) -> None:
    order = _placed(msw, "b1")[1]
    got = s.get_order(msw, msw.actors["b1"], order["id"])
    assert ms.n(got.get("totalAmount")) == total, got
    assert ms.n(got.get("platformDiscountAmount")) == amount, got
