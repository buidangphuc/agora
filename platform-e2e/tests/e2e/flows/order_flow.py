"""Order, Saga & Logistics flows."""

from __future__ import annotations

from typing import Any

from src.models import User


def create_order_via_api(
    world,
    buyer: User,
    listing_id: str,
    quantity: int = 1,
    variant_id: str = "",
    voucher_code: str = "FREESHIP",
) -> dict[str, Any]:
    """Execute order creation via Gateway API."""
    world.service_factory.set_token(buyer.token)
    try:
        world.service_factory.address.create_address(
            recipient_name="Nguyen Van A",
            phone="0912345678",
            street="29 Lieu Giai",
            city="Ha Noi",
            ward="Phuong Lieu Giai",
            district="Quan Ba Dinh",
            is_default=True,
        )
    except Exception:  # noqa: BLE001
        pass

    try:
        world.service_factory.cart.clear_cart()
    except Exception:  # noqa: BLE001
        pass

    try:
        world.service_factory.cart.add_to_cart(listing_id=listing_id, quantity=quantity)
    except Exception:  # noqa: BLE001
        pass

    res = world.service_factory.order.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    orders = res.get("orders", [])
    order_id = orders[0].get("id") if orders else res.get("order", {}).get("id", "order_001")
    world.state.order_id = order_id
    world.state.extra["order_id"] = order_id
    world.logger.info(f"Created order {order_id} via API")
    return res


def pay_order_via_api(world, buyer: User, order_id: str, amount: int = 29990000) -> dict[str, Any]:
    """Execute mock payment via Gateway API."""
    world.service_factory.set_token(buyer.token)
    res = world.service_factory.payment.mock_pay(order_id, amount, success=True)
    world.logger.info(f"Paid order {order_id} via MockPay API")
    return res


def create_shipment_via_api(
    world, seller: User, order_id: str, carrier: str = "SPX Express", tracking_code: str = ""
) -> dict[str, Any]:
    """Create shipment fulfillment tracking via Gateway API."""
    if not seller.token:
        try:
            seller.token = world.service_factory.auth.login(seller.username, seller.password)
        except Exception:  # noqa: BLE001
            seller.token = world.service_factory.auth.register(
                seller.username, seller.password, "seller"
            )
    world.service_factory.set_token(seller.token)
    code = tracking_code or f"SPX_VN_{order_id}"
    res = world.service_factory.order.create_shipment(order_id, carrier, code)
    world.state.tracking_code = code
    world.logger.info(f"Created shipment {code} for order {order_id}")
    return res


def complete_order_as_seller(
    world, seller: User, order_id: str, restore_token: str | None = None
) -> None:
    """Drive an order PENDING -> SHIPPED -> COMPLETED as the order's seller.

    team-order only lets the seller (or an admin) call UpdateOrderStatus, and only
    along PENDING|PAID -> SHIPPED and SHIPPED -> COMPLETED. ``restore_token`` is put
    back as the active token afterwards (pass the buyer's when later steps act as them).
    """
    assert seller and seller.token, "no seeded seller; tag the scenario @needsListing"
    world.service_factory.set_token(seller.token)
    try:
        order = world.service_factory.order.get_order(order_id).get("order", {})
        # A scenario may already have shipped it (CreateShipment); SHIPPED -> SHIPPED
        # is not a valid transition, so only ship what is not shipped yet.
        if order.get("status") != "ORDER_STATUS_SHIPPED":
            world.service_factory.order.update_order_status(order_id, "ORDER_STATUS_SHIPPED")
        world.service_factory.order.update_order_status(order_id, "ORDER_STATUS_COMPLETED")
    finally:
        if restore_token:
            world.service_factory.set_token(restore_token)
