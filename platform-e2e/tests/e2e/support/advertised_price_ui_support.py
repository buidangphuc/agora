"""Helpers of frontend/advertised_price_ui.feature (change frontend-advertised-price).

Black box: accounts, listings, flash campaigns and carts are seeded through the gateway
(`d2_support`, `flash_support`); the shipping rule is read from the real `CalculateShippingFee`
through the gateway, the same route the frontend uses; the browser is the system under test.
"""

from __future__ import annotations

from typing import Any

from playwright.sync_api import Locator, Page

from src.api.services import AddressService, OrderService
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World

SHIPPING_FEE_PATH = "/platform.order.v1.OrderService/CalculateShippingFee"

# The three texts the PDP must never render again.
REMOVED_PDP_TEXTS = (
    "Miễn phí vận chuyển cho đơn hàng từ 0Đ",
    "SPX Express (1-2 ngày)",
    "Bảo hiểm thiết bị và bảo vệ người mua",
)

PRICE_CHANGED_MESSAGE = (
    "Giá một số sản phẩm trong giỏ đã thay đổi. Giỏ hàng đã được cập nhật, "
    "vui lòng kiểm tra tổng tiền mới rồi đặt hàng lại."
)


def vnd(amount: int) -> str:
    """20000 -> "20.000" (the Vietnamese grouping the pages use)."""
    return f"{amount:,}".replace(",", ".")


def state(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("adv_ui", {"buyers": {}})


def register_buyer(city: str) -> dict[str, str]:
    """A fresh buyer whose only (default) address is in `city`."""
    username, token = d2.register("buyer")
    AddressService(token=token).create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city=city,
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )
    return {"username": username, "token": token, "city": city}


def sign_in_browser(world: World, token: str | None) -> None:
    """Make the browser this buyer (token) or an anonymous visitor (None)."""
    world.context.clear_cookies()
    if token:
        world.context.add_cookies(
            session_cookies(world.settings, token)
        )


def shipping_fee(token: str, city: str, items_subtotal: int) -> dict[str, Any]:
    """The real CalculateShippingFee through the gateway: {fee, free, message}."""
    svc = OrderService(token=token)
    try:
        res = svc.post(SHIPPING_FEE_PATH, {"city": city, "itemsSubtotal": items_subtotal})
    finally:
        svc.close()
    return {
        "fee": int(res.get("shippingFee", 0) or 0),  # proto3 JSON omits zero
        "free": bool(res.get("isFreeShipping", False)),
        "message": res.get("message", ""),
    }


def total_with_shipping(token: str, city: str, subtotal: int) -> int:
    """subtotal + the server's shipping fee for it: the total checkout must show."""
    return subtotal + shipping_fee(token, city, subtotal)["fee"]


def shipping_row(page: Page) -> Locator:
    """The PDP "Vận chuyển" row (label and value)."""
    label = page.locator("dt", has_text="Vận chuyển")
    if label.count():
        return label.first.locator("xpath=..")
    return page.get_by_text("Vận chuyển", exact=True).first.locator("xpath=..")


def buyer_orders(token: str) -> list[dict[str, Any]]:
    svc = OrderService(token=token)
    try:
        return svc.list_buyer_orders().get("orders", [])
    finally:
        svc.close()
