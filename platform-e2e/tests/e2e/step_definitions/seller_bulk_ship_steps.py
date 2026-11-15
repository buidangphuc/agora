"""Steps for frontend/seller_bulk_ship.feature (seller-bulk-ship, initiative seller-centre).

Black box in the browser on the seller app. Sellers, listings, buyers, orders and the buyer
cancel go through the real gateway API; the batch itself runs through the real pages.
"""

from __future__ import annotations

import re
import uuid

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import AddressService, CartService, OrderService
from src.constants import timeouts
from src.pages.seller_bulk_ship_page import CANCELLED_REASON, SellerBulkShipPage
from tests.e2e.support import d2_support as d2
from tests.e2e.support.apps import app_url
from tests.e2e.support.world import World

DESKTOP = {"width": 1280, "height": 800}


def _st(world: World) -> dict:
    return world.state.extra.setdefault("sbs", {"orders": [], "names": {}})


def _page(world: World) -> SellerBulkShipPage:
    return SellerBulkShipPage(world.page)


def _order_for(seller: dict, recipient: str) -> tuple[str, str]:
    username, token = d2.register("buyer")
    AddressService(token=token).create_address(
        recipient_name=recipient,
        phone="0912345678",
        street="29 Lieu Giai",
        city="Hà Nội",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )
    CartService(token=token).add_to_cart(seller["listings"][0]["id"], 1)
    res = OrderService(token=token).create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    orders = res.get("orders", [])
    return (orders[0]["id"] if orders else res["order"]["id"]), token


def _seed(world: World, count: int) -> None:
    st = _st(world)
    seller = d2.seed_seller_session(world, listings=1, stock=100)
    st["seller"] = seller
    for _ in range(count):
        recipient = f"Nguoi Nhan {uuid.uuid4().hex[:6]}"
        order_id, buyer_token = _order_for(seller, recipient)
        st["orders"].append(order_id)
        st["names"][order_id] = recipient
        st.setdefault("buyer_tokens", {})[order_id] = buyer_token


def _open_list(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.page.goto(app_url(world.settings, "/seller/orders"))
    expect(world.page.locator("tbody tr").first).to_be_visible(timeout=timeouts.NAVIGATION)


@given(parsers.parse("a bulk-ship seller has {count:d} shippable orders"))
def seller_with_orders(world: World, count: int) -> None:
    _seed(world, count)


@given(
    parsers.parse(
        "a bulk-ship seller has {count:d} shippable orders and another seller is signed in"
    )
)
def foreign_orders(world: World, count: int) -> None:
    _seed(world, count)
    d2.seed_seller_session(world, listings=0)


@when("the buyer of the second bulk-ship order cancels it")
def buyer_cancels_second(world: World) -> None:
    st = _st(world)
    order_id = st["orders"][1]
    OrderService(token=st["buyer_tokens"][order_id]).cancel_order(order_id, "e2e cancel")
    st["cancelled"] = order_id


@when("the seller opens the order list")
def open_list(world: World) -> None:
    _open_list(world)


@when("the seller selects all the bulk-ship orders")
def select_all(world: World) -> None:
    page = _page(world)
    for order_id in _st(world)["orders"]:
        page.checkbox(order_id).check()


@when("the seller selects the first two bulk-ship orders")
def select_two(world: World) -> None:
    page = _page(world)
    for order_id in _st(world)["orders"][:2]:
        page.checkbox(order_id).check()


@when(
    parsers.parse(
        'the seller types the tracking number "{prefix}" for the first order and confirms'
    )
)
def ship_with_tracking(world: World, prefix: str) -> None:
    st = _st(world)
    st["tracking"] = f"{prefix}-{uuid.uuid4().hex[:6]}".upper()
    page = _page(world)
    page.ship_button.click()
    expect(page.modal).to_be_visible(timeout=timeouts.DEFAULT)
    page.tracking_inputs.first.fill(st["tracking"])
    page.confirm_button.click()


@when("the seller confirms the bulk ship")
def confirm_ship(world: World) -> None:
    page = _page(world)
    page.ship_button.click()
    expect(page.modal).to_be_visible(timeout=timeouts.DEFAULT)
    page.confirm_button.click()


@when("the seller opens the batch print for the selection")
def open_print(world: World) -> None:
    page = _page(world)
    page.print_button.first.click()
    world.page.wait_for_url(
        re.compile(r".*/seller/orders/print\?.*ids=.*"), timeout=timeouts.NAVIGATION
    )


@when("the other seller opens the batch print for the first bulk-ship order")
def foreign_print(world: World) -> None:
    order_id = _st(world)["orders"][0]
    world.page.set_viewport_size(DESKTOP)
    world.page.goto(app_url(world.settings, f"/seller/orders/print?ids={order_id}"))


@then("the bulk-ship result list shows no failure reason")
def no_failure(world: World) -> None:
    modal = _page(world).modal
    expect(modal).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(modal).not_to_contain_text(CANCELLED_REASON)


@then("the bulk-ship result list shows the cancelled-order reason once")
def cancelled_reason(world: World) -> None:
    modal = _page(world).modal
    expect(modal).to_contain_text(CANCELLED_REASON, timeout=timeouts.NAVIGATION)
    assert modal.inner_text().count(CANCELLED_REASON) == 1


def _statuses(world: World) -> dict[str, str]:
    seller = _st(world)["seller"]
    svc = OrderService(token=seller["token"])
    return {o: svc.get_order(o).get("order", {}).get("status", "") for o in _st(world)["orders"]}


@then(parsers.parse('{count:d} of the bulk-ship orders show "Đang giao" on the order list'))
def shipped_on_list(world: World, count: int) -> None:
    _open_list(world)
    page = _page(world)
    shipped = [o for o in _st(world)["orders"] if "Đang giao" in page.row(o).inner_text()]
    assert len(shipped) == count, shipped


@then("the first bulk-ship order shows the typed tracking number on its detail page")
def tracking_on_detail(world: World) -> None:
    st = _st(world)
    world.page.goto(app_url(world.settings, f"/seller/orders/{st['orders'][0]}"))
    expect(world.page.locator("body")).to_contain_text(st["tracking"], timeout=timeouts.NAVIGATION)


@then('the cancelled bulk-ship order is not shipped and the others show "Đang giao"')
def cancelled_not_shipped(world: World) -> None:
    st = _st(world)
    _open_list(world)
    page = _page(world)
    for order_id in st["orders"]:
        shipped = (
            "Đang giao" in page.row(order_id).inner_text() if page.row(order_id).count() else False
        )
        assert shipped == (order_id != st["cancelled"]), order_id


@then(
    parsers.parse(
        "the print page shows {count:d} packing slips with the recipient names of the selection"
    )
)
def slips_with_names(world: World, count: int) -> None:
    st = _st(world)
    slips = _page(world).slips
    expect(slips).to_have_count(count, timeout=timeouts.NAVIGATION)
    for order_id in st["orders"][:count]:
        expect(world.page.locator("body")).to_contain_text(st["names"][order_id])


@then('the print page shows no packing slip and names the order in the "Không in được" notice')
def foreign_notice(world: World) -> None:
    page = _page(world)
    order_id = _st(world)["orders"][0]
    expect(page.print_notice.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.slips).to_have_count(0)
    expect(world.page.locator("body")).to_contain_text(order_id)
