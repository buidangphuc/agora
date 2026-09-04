"""Order fulfillment, tracking and RMA step definitions."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.pages import OrderDetailPage
from src.utils import get_test_data_manager
from tests.e2e.flows import create_order_via_api, create_shipment_via_api, login_via_api
from tests.e2e.support.world import World


@given("I am logged in as a buyer via API")
def logged_in_as_buyer_api(world: World) -> None:
    buyer = get_test_data_manager().get_user_by_role("buyer")
    login_via_api(world, buyer)


@given("I have an active order with SPX shipment tracking")
def active_order_with_tracking(world: World) -> None:
    buyer = world.state.current_user or get_test_data_manager().get_user_by_role("buyer")
    listing_id = world.state.listing.listing_id if world.state.listing else "listing_001"
    create_order_via_api(world, buyer, listing_id)
    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    create_shipment_via_api(world, seller, world.state.order_id, carrier="SPX Express")


@then("I should see the order delivery timeline")
def verify_order_timeline(world: World) -> None:
    order_page: OrderDetailPage = world.get_page(PageName.ORDER_DETAIL)  # type: ignore[assignment]
    expect(order_page.timeline_container).to_be_visible(timeout=timeouts.DEFAULT)


@when(parsers.parse('I submit an RMA refund request with reason "{reason}"'))
def submit_rma_refund_request(world: World, reason: str) -> None:
    order_page: OrderDetailPage = world.get_page(PageName.ORDER_DETAIL)  # type: ignore[assignment]
    if order_page.rma_refund_button.is_visible():
        order_page.open_rma_modal()
        order_page.submit_rma_request(reason)


@then("I should see the RMA refund success confirmation")
def verify_rma_success(world: World) -> None:
    order_page: OrderDetailPage = world.get_page(PageName.ORDER_DETAIL)  # type: ignore[assignment]
    if order_page.rma_success_alert.is_visible():
        expect(order_page.rma_success_alert).to_be_visible(timeout=timeouts.DEFAULT)
