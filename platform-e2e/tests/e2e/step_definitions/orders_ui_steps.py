"""Steps for frontend/orders_ui.feature (and the order-detail navigation shared with the RMA journey)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from src.models import User
from src.pages import OrderDetailPage, OrdersListPage
from src.utils import data as fake
from src.utils import get_test_data_manager
from tests.e2e.flows import complete_order_as_seller, create_order_via_api, login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()


# ── Helpers ──────────────────────────────────────────────────────────────
def _buyer(world: World) -> User:
    return world.state.extra.get("seeded_buyer") or get_test_data_manager().get_user_by_role(
        "buyer"
    )


def _listing_id(world: World) -> str:
    return world.state.listing.listing_id if world.state.listing else "listing_001"


def _place_order(world: World) -> str:
    """Create one order for the buyer and remember it as the scenario's target order."""
    create_order_via_api(world, _buyer(world), _listing_id(world))
    order_id = world.state.order_id
    world.state.extra["target_order_id"] = order_id
    world.state.extra.setdefault("order_ids", []).append(order_id)
    return order_id


def _complete(world: World, order_id: str) -> None:
    """Ship and complete the order as its seller, then act as the buyer again."""
    seller = world.state.seeded_seller or get_test_data_manager().get_user_by_role("seller")
    complete_order_as_seller(world, seller, order_id, restore_token=_buyer(world).token)


def _orders(world: World) -> OrdersListPage:
    return world.get_page(PageName.ACCOUNT_ORDERS)  # type: ignore[return-value]


def _detail(world: World) -> OrderDetailPage:
    return world.get_page(PageName.ORDER_DETAIL)  # type: ignore[return-value]


def _url(world: World, path: str) -> str:
    return f"{world.settings.base_url.rstrip('/')}{path}"


# ── Given: order data ────────────────────────────────────────────────────
@given("the buyer has a pending, a delivered and a cancelled order")
def buyer_has_three_orders(world: World) -> None:
    sf = world.service_factory
    sf.set_token(_buyer(world).token)
    _place_order(world)  # stays pending
    delivered = _place_order(world)
    _complete(world, delivered)
    cancelled = _place_order(world)
    sf.set_token(_buyer(world).token)
    sf.order.cancel_order(cancelled, "e2e")


@given(parsers.re(r"the buyer has (?P<count>\d+) orders?"))
def buyer_has_n_orders(world: World, count: str) -> None:
    for _ in range(int(count)):
        _place_order(world)


@given("the buyer has an order whose payment failed")
def buyer_has_failed_payment_order(world: World) -> None:
    order_id = _place_order(world)
    sf = world.service_factory
    # ForceFailSaga is admin-only; act as the seeded admin, then back as the buyer.
    admin = get_test_data_manager().get_user_by_role("admin")
    sf.set_token(sf.auth.login(admin.username, admin.password))
    try:
        sf.order.force_fail_saga(order_id)
    finally:
        sf.set_token(_buyer(world).token)


@given("the buyer has a delivered order")
def buyer_has_delivered_order(world: World) -> None:
    _complete(world, _place_order(world))


@given("an order belongs to a different buyer")
def order_of_another_buyer(world: World) -> None:
    sf = world.service_factory
    name = fake.unique_username("other_buyer")
    token = sf.auth.register(name, SETTINGS.seed_password, "buyer")
    other = User(username=name, password=SETTINGS.seed_password, role="buyer", token=token)
    create_order_via_api(world, other, _listing_id(world))
    world.state.extra["target_order_id"] = world.state.order_id
    # Back to the signed-in buyer: the other buyer's token must not stay active.
    login_via_api(world, _buyer(world))


@given(parsers.parse("the viewport is {width:d} by {height:d}"))
def set_viewport(world: World, width: int, height: int) -> None:
    world.page.set_viewport_size({"width": width, "height": height})


# ── When: list ───────────────────────────────────────────────────────────
@when(parsers.parse('I select the "{label}" order tab'))
def select_order_tab(world: World, label: str) -> None:
    orders = _orders(world)
    orders.tab(label).click()
    world.page.wait_for_url("**/account/orders?*", timeout=timeouts.DEFAULT)


@when(parsers.parse('I open the orders list with the query "{query}"'))
def open_orders_with_query(world: World, query: str) -> None:
    world.page.goto(_url(world, f"/account/orders?{query}"), wait_until="domcontentloaded")


@when("I reload the page")
def reload_page(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@when(parsers.parse('I follow the pagination link "{number:d}"'))
def follow_pagination_link(world: World, number: int) -> None:
    _orders(world).page_link(number).click()
    world.page.wait_for_url(f"**/account/orders?*page={number}*", timeout=timeouts.DEFAULT)


@when("I follow the link to all orders")
def follow_link_to_all_orders(world: World) -> None:
    _orders(world).view_all_link.click()
    world.page.wait_for_url(_url(world, "/account/orders"), timeout=timeouts.DEFAULT)


@when("I open the order detail from the list")
def open_order_detail_from_list(world: World) -> None:
    orders = _orders(world)
    expect(orders.detail_links.first).to_be_visible(timeout=timeouts.DEFAULT)
    orders.detail_links.first.click()
    world.page.wait_for_url("**/account/orders/*", timeout=timeouts.DEFAULT)


@when(parsers.parse('I click "{label}" on the first order'))
def click_on_first_order(world: World, label: str) -> None:
    card = _orders(world).order_cards.first
    expect(card).to_be_visible(timeout=timeouts.DEFAULT)
    card.get_by_role("button", name=label, exact=True).click()


# ── When: detail ─────────────────────────────────────────────────────────
@when("I open the order detail page")
def open_order_detail_page(world: World) -> None:
    order_id = world.state.extra["target_order_id"]
    world.navigate_to(PageName.ORDER_DETAIL, order_id=order_id)


@when("I open the return Modal")
def open_return_modal(world: World) -> None:
    detail = _detail(world)
    expect(detail.rma_refund_button).to_be_visible(timeout=timeouts.DEFAULT)
    detail.open_rma_modal()
    expect(detail.rma_dialog).to_be_visible(timeout=timeouts.DEFAULT)


@when("I submit the return form without a reason")
def submit_return_without_reason(world: World) -> None:
    _detail(world).rma_confirm_button.click()


@when(parsers.parse('I submit the return form with reason "{reason}"'))
def submit_return_with_reason(world: World, reason: str) -> None:
    _detail(world).submit_rma_request(reason)


# ── Then: list ───────────────────────────────────────────────────────────
@then(parsers.parse('the orders URL is "{path}"'))
def orders_url_is(world: World, path: str) -> None:
    expect(world.page).to_have_url(_url(world, path), timeout=timeouts.DEFAULT)


@then(parsers.parse('every listed order has the status "{text}"'))
def every_order_has_status(world: World, text: str) -> None:
    orders = _orders(world)
    expect(orders.order_cards.first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(orders.order_statuses).to_have_text([text] * orders.order_cards.count())


@then(parsers.re(r"the orders list shows (?P<count>\d+) orders?"))
def orders_list_shows(world: World, count: str) -> None:
    expect(_orders(world).order_cards).to_have_count(int(count), timeout=timeouts.DEFAULT)


@then(parsers.parse('the "{label}" order tab is the current tab'))
def order_tab_is_current(world: World, label: str) -> None:
    expect(_orders(world).tab(label)).to_have_attribute(
        "aria-current", "page", timeout=timeouts.DEFAULT
    )


@then("the empty tab message is shown with a link to all orders")
def empty_tab_message(world: World) -> None:
    orders = _orders(world)
    expect(orders.empty_text).to_be_visible(timeout=timeouts.DEFAULT)
    expect(orders.view_all_link).to_have_attribute("href", "/account/orders")


# ── Then: detail ─────────────────────────────────────────────────────────
@then("the timeline shows the failure checkpoint")
def timeline_shows_failure(world: World) -> None:
    expect(_detail(world).timeline_failure.first).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the failure alert offers "{label}"'))
def failure_alert_offers(world: World, label: str) -> None:
    alert = world.page.get_by_role("alert").filter(has_text="Đơn hàng không hoàn tất")
    expect(alert.get_by_role("button", name=label)).to_be_visible(timeout=timeouts.DEFAULT)


@then("the return form shows the reason error and stays open")
def return_form_shows_reason_error(world: World) -> None:
    detail = _detail(world)
    expect(world.page.get_by_text("Vui lòng chọn lý do trả hàng.")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    expect(detail.rma_dialog).to_be_visible()


@then("the return section shows the request as pending")
def return_section_shows_pending(world: World) -> None:
    detail = _detail(world)
    expect(detail.rma_dialog).to_have_count(0, timeout=timeouts.DEFAULT)
    detail.tab("Trả hàng / Hoàn tiền").click()
    expect(detail.rma_status).to_have_text("Chờ duyệt", timeout=timeouts.DEFAULT)


@then("the 403 page is shown with a link back to my orders")
def forbidden_page_shown(world: World) -> None:
    detail = _detail(world)
    expect(detail.forbidden_result).to_be_visible(timeout=timeouts.DEFAULT)
    expect(detail.back_to_orders_link).to_have_attribute("href", "/account/orders")


@then("no item of that order is rendered")
def no_order_items_rendered(world: World) -> None:
    expect(world.page.get_by_role("table")).to_have_count(0)
    expect(world.page.get_by_test_id("order-status")).to_have_count(0)


# ── Then: layout / tracking ──────────────────────────────────────────────
@then("the page has no horizontal scroll")
def no_horizontal_scroll(world: World) -> None:
    world.page.wait_for_load_state("networkidle")
    widths = world.page.evaluate(
        "() => ({scroll: document.documentElement.scrollWidth, inner: window.innerWidth})"
    )
    assert widths["scroll"] <= widths["inner"], f"horizontal scroll: {widths}"


@then("the buyer lands on the cart")
def buyer_lands_on_cart(world: World) -> None:
    world.page.wait_for_url("**/cart", timeout=timeouts.DEFAULT)


@then("the analytics data layer is initialised")
def data_layer_initialised(world: World) -> None:
    ready = world.page.evaluate(
        "() => Array.isArray(window.dataLayer) && window.dataLayer.length > 0"
    )
    assert ready, "window.dataLayer is missing or empty on the cart page"
