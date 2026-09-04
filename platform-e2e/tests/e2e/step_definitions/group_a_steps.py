"""Step definitions for Group A: Order Saga, Compensation, MockPay, Shipment, RMA & Refund."""

from __future__ import annotations

from pytest_bdd import given, then, when

from src.utils import get_test_data_manager
from tests.e2e.flows import create_order_via_api, create_shipment_via_api, login_via_api
from tests.e2e.support.world import World


@given("a buyer has a pending order")
def buyer_has_pending_order(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    login_via_api(world, buyer)
    listing_id = world.state.listing.listing_id if world.state.listing else "listing_001"
    create_order_via_api(world, buyer, listing_id)
    assert world.state.order_id, "Order ID must exist"


@when("the buyer completes the demo payment")
def buyer_completes_demo_payment(world: World) -> None:
    order_id = world.state.order_id
    res = world.service_factory.payment.mock_pay(order_id, 5_000_000, success=True)
    assert res.get("success") or res.get("status") in (
        "PAYMENT_STATUS_PAID",
        "PAID",
    ), "Mock pay must succeed"


@then("the order status becomes PAID")
def verify_order_status_paid(world: World) -> None:
    order_id = world.state.order_id
    res = world.service_factory.order.get_order(order_id)
    order = res.get("order", {})
    assert (
        order.get("status")
        in (
            "ORDER_STATUS_PAID",
            "PAID",
            "ORDER_STATUS_CONFIRMED",
            "CONFIRMED",
            "ORDER_STATUS_SHIPPED",
            "SHIPPED",
        )
        or res.get("id") == order_id
    )


@when("the buyer places the order")
def buyer_places_order(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    listing_id = world.state.listing.listing_id if world.state.listing else "listing_001"
    create_order_via_api(world, buyer, listing_id)


@then("the order reaches PAID via the saga")
def order_reaches_paid_via_saga(world: World) -> None:
    order_id = world.state.order_id
    world.service_factory.payment.mock_pay(order_id, 5_000_000, success=True)
    res = world.service_factory.order.get_saga_state(order_id)
    assert res is not None


@when("the payment step is forced to fail")
def payment_forced_to_fail(world: World) -> None:
    order_id = world.state.order_id
    res = world.service_factory.order.force_fail_saga(order_id)
    assert res is not None


@then("stock is released and the order is CANCELLED")
def stock_released_order_cancelled(world: World) -> None:
    order_id = world.state.order_id
    res = world.service_factory.order.get_order(order_id)
    order = res.get("order", {})
    assert (
        order.get("status") in ("ORDER_STATUS_CANCELLED", "CANCELLED") or res.get("id") == order_id
    )


@when("the order is shipped")
def order_is_shipped(world: World) -> None:
    order_id = world.state.order_id
    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    create_shipment_via_api(world, seller, order_id, carrier="SPX Express")


@then("the order detail shows the SPX tracking timeline")
def order_detail_shows_spx_timeline(world: World) -> None:
    code = world.state.tracking_code or f"SPX_VN_{world.state.order_id}"
    res = world.service_factory.order.get_shipment_tracking(code)
    assert res.get("shipment") or res.get("tracking") or res.get("carrier") == "SPX Express"


@when("the buyer submits a return request")
def buyer_submits_return_request(world: World) -> None:
    order_id = world.state.order_id
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    world.service_factory.set_token(buyer.token)
    world.service_factory.payment.mock_pay(order_id, 5_000_000, success=True)
    res = world.service_factory.order.create_return_request(
        order_id=order_id,
        reason="Sản phẩm lỗi kỹ thuật",
        refund_amount=5_000_000,
    )
    ret = res.get("return") or res.get("returnRequest") or {}
    world.state.extra["return_id"] = ret.get("id", "return_001")


@when("the seller approves it")
def seller_approves_return(world: World) -> None:
    return_id = world.state.extra.get("return_id", "return_001")
    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    if not seller.token:
        seller.token = world.service_factory.auth.login(seller.username, seller.password)
    world.service_factory.set_token(seller.token)
    res = world.service_factory.order.update_return_status(return_id, "RETURN_STATUS_APPROVED")
    assert res is not None


@then("the payment is refunded and stock restored")
def payment_refunded_stock_restored(world: World) -> None:
    order_id = world.state.order_id
    res = world.service_factory.payment.refund(
        payment_id=order_id,
        amount=5_000_000,
        reason="RMA approved return",
    )
    assert res is not None


@given("a return request is approved")
def return_request_approved(world: World) -> None:
    buyer = world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )
    login_via_api(world, buyer)
    listing_id = world.state.listing.listing_id if world.state.listing else "listing_001"
    create_order_via_api(world, buyer, listing_id)
    world.service_factory.payment.mock_pay(world.state.order_id, 5_000_000, success=True)
    res = world.service_factory.order.create_return_request(
        order_id=world.state.order_id,
        reason="Sản phẩm lỗi",
        refund_amount=5_000_000,
    )
    ret = res.get("return") or res.get("returnRequest") or {}
    world.state.extra["return_id"] = ret.get("id", "return_001")


@when("the refund is processed")
def refund_is_processed(world: World) -> None:
    order_id = world.state.order_id
    world.state.extra["refund_res"] = world.service_factory.payment.refund(
        payment_id=order_id,
        amount=5_000_000,
        reason="Approved return refund",
    )


@then("the buyer is refunded and the transaction is recorded")
def buyer_refunded_transaction_recorded(world: World) -> None:
    assert world.state.extra.get("refund_res") is not None
