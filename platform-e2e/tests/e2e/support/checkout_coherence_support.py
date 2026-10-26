"""Helpers of frontend/checkout_coherence_ui.feature (change frontend-checkout-coherence).

Black box: shops, vouchers, addresses and carts are seeded through the gateway; the figures a
page must show are read from the real `PreviewCheckout` the frontend itself calls, never
recomputed here. Payment transactions are counted with a read-only query on payment_db.
"""

from __future__ import annotations

import re
import uuid
from datetime import datetime, timedelta
from typing import Any
from zoneinfo import ZoneInfo

from src.api.services import AddressService, OrderService
from src.api.services.base_service import BaseService
from src.core import ServiceFactory
from src.utils import get_test_data_manager
from tests.e2e.support import d2_support as d2
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import voucher_scope_support as vs
from tests.e2e.support.world import World

PREVIEW_PATH = "/platform.order.v1.OrderService/PreviewCheckout"
PAYMENT_PATH = "/platform.payment.v1.PaymentService/"
MOMO = "PAYMENT_METHOD_MOCK_MOMO"
COD = "PAYMENT_METHOD_COD"
HCM = ZoneInfo("Asia/Ho_Chi_Minh")


def ctx(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("cc", {})


def vnd(amount: int) -> str:
    return f"{amount:,}".replace(",", ".")


def fresh_code(prefix: str) -> str:
    return f"{prefix}{uuid.uuid4().hex[:8].upper()}"


def seed_cart(world: World, city: str, prices: list[int]) -> list[dict[str, Any]]:
    """A logged-in buyer whose default address is in `city`, one item from each of len(prices) shops."""
    d2.seed_buyer(world, address_city=city)
    shops = []
    for price in prices:
        shop = d2.seed_shop(price=price, stock=50)
        d2.add_to_cart(world, shop)
        shops.append(shop)
    c = ctx(world)
    c["shops"] = shops
    c["buyer"] = world.state.extra["d2_buyer"]
    return shops


def buyer_token(world: World) -> str:
    return ctx(world)["buyer"].token


def default_address(world: World) -> dict[str, Any]:
    svc = AddressService(token=buyer_token(world))
    try:
        addrs = svc.list_addresses().get("addresses", [])
    finally:
        svc.close()
    assert addrs, "buyer has no address"
    return next((a for a in addrs if a.get("isDefault")), addrs[0])


def preview(
    world: World, shop_vouchers: list[tuple[str, str]] | None = None, platform: str = ""
) -> dict[str, Any]:
    """The server's quote for the buyer's cart: the figures the checkout pages must show."""
    body: dict[str, Any] = {"addressId": default_address(world)["id"]}
    if shop_vouchers:
        body["shopVouchers"] = [{"sellerId": sid, "code": code} for sid, code in shop_vouchers]
    if platform:
        body["platformVoucherCode"] = platform
    svc = BaseService(token=buyer_token(world))
    try:
        return svc.post(PREVIEW_PATH, body)
    finally:
        svc.close()


def n(value: Any) -> int:
    return int(value or 0)


def admin_token() -> str:
    admin = get_test_data_manager().get_user_by_role("admin")
    sf = ServiceFactory()
    try:
        return sf.auth.login(admin.username, admin.password)
    finally:
        sf.close()


def seed_shop_voucher(shop: dict[str, Any], prefix: str = "CCS") -> str:
    return vs.seed_shop_voucher(shop["token"], fresh_code(prefix))


def seed_platform_voucher(prefix: str = "CCP") -> str:
    return vs.seed_platform_voucher(admin_token(), fresh_code(prefix))


def buyer_orders(token: str) -> list[dict[str, Any]]:
    svc = OrderService(token=token)
    try:
        return svc.list_buyer_orders().get("orders", [])
    finally:
        svc.close()


def get_order(token: str, order_id: str) -> dict[str, Any]:
    svc = OrderService(token=token)
    try:
        return svc.get_order(order_id).get("order", {})
    finally:
        svc.close()


def payment_count(order_id: str) -> int:
    """Number of payment transactions of an order (read-only)."""
    out = stack.docker(
        "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", "payment_db",
        "-v", "ON_ERROR_STOP=1", "-At", "-c",
        f"SELECT count(*) FROM payment_transactions WHERE order_id = '{order_id}'",
    )  # fmt: skip
    return int(out.stdout.strip() or 0)


def eta_pattern(days: int) -> re.Pattern[str]:
    day = (datetime.now(HCM) + timedelta(days=days)).strftime("%d/%m")
    return re.compile(r"Nhận hàng dự kiến\s*" + day)


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")
