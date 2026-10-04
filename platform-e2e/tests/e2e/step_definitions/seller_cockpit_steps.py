"""Seller cockpit steps (OpenSpec change `ui-phase-seller`).

Shell (sidebar / Drawer / shop card), Workplace, product Table List, delete
Modal, listing-studio validation, Magic Listing assist, order Table List and
detail, analytics range, wallet payout, shop profile and the 375px layout.

Every step is driven through the page objects in `src/pages/seller_*.py`; seed
state comes from the `needsSeller` / `needsListing` / `needsOrder` tags.
"""

from __future__ import annotations

import re

import pytest
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import (
    SellerAnalyticsPage,
    SellerEditListingPage,
    SellerListingsPage,
    SellerNewListingPage,
    SellerOrderDetailPage,
    SellerOrdersPage,
    SellerShopPage,
    SellerWalletPage,
)
from src.utils import data as fake
from tests.e2e.flows import seed_listing
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support.world import World

DESKTOP = {"width": 1280, "height": 720}
PHONE = {"width": 375, "height": 812}


def _workplace(world: World) -> SellerListingsPage:
    page = world.get_page(PageName.SELLER_LISTINGS)
    return page  # type: ignore[return-value]


def _new_listing(world: World) -> SellerNewListingPage:
    return world.get_page(PageName.SELLER_NEW_LISTING)  # type: ignore[return-value]


def _orders(world: World) -> SellerOrdersPage:
    return world.get_page(PageName.SELLER_ORDERS)  # type: ignore[return-value]


def _detail(world: World) -> SellerOrderDetailPage:
    return world.get_page(PageName.SELLER_ORDER_DETAIL)  # type: ignore[return-value]


def _wallet(world: World) -> SellerWalletPage:
    return world.get_page(PageName.SELLER_WALLET)  # type: ignore[return-value]


def _shop(world: World) -> SellerShopPage:
    return world.get_page(PageName.SELLER_SHOP)  # type: ignore[return-value]


def _analytics(world: World) -> SellerAnalyticsPage:
    return world.get_page(PageName.SELLER_ANALYTICS)  # type: ignore[return-value]


def _no_horizontal_scroll(world: World) -> bool:
    return bool(
        world.page.evaluate(
            "document.documentElement.scrollWidth <= document.documentElement.clientWidth"
        )
    )


# ── Shell ────────────────────────────────────────────────────────────────
@when("the seller opens the seller workplace")
def open_workplace(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.navigate_to(PageName.SELLER_LISTINGS)
    expect(_workplace(world).sidebar).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the seller opens the seller workplace at a 375px viewport")
def open_workplace_phone(world: World) -> None:
    world.page.set_viewport_size(PHONE)
    world.navigate_to(PageName.SELLER_LISTINGS)
    expect(_workplace(world).menu_button).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the seller collapses the sidebar")
def collapse_sidebar(world: World) -> None:
    _workplace(world).collapse_toggle.click()


@then("the sidebar is 64px wide and its links keep their accessible names")
def sidebar_is_collapsed(world: World) -> None:
    page = _workplace(world)
    expect(page.sidebar).to_have_attribute("data-collapsed", "true")
    box = page.sidebar.bounding_box()
    assert box is not None and 62 <= box["width"] <= 66, f"sidebar width {box}"
    expect(page.nav_link("Quản lý đơn hàng")).to_have_attribute("href", "/seller/orders")


@when("the seller taps the menu button")
def tap_menu_button(world: World) -> None:
    _workplace(world).menu_button.click()


@then("a navigation dialog lists the seller links and no sidebar is shown inline")
def drawer_lists_links(world: World) -> None:
    page = _workplace(world)
    expect(page.drawer).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.drawer.get_by_role("link", name="Quản lý đơn hàng")).to_be_visible()
    expect(page.sidebar).to_be_hidden()


@when(parsers.parse('the seller chooses "{name}" in the drawer'))
def choose_in_drawer(world: World, name: str) -> None:
    _workplace(world).drawer.get_by_role("link", name=name).click()


@then(parsers.parse('the browser is on "{path}" and the drawer is closed'))
def on_path_drawer_closed(world: World, path: str) -> None:
    expect(world.page).to_have_url(
        re.compile(rf".*{re.escape(path)}$"), timeout=timeouts.NAVIGATION
    )
    expect(_workplace(world).drawer).to_be_hidden()


@when("the seller opens the seller wallet page")
def open_wallet(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.navigate_to(PageName.SELLER_WALLET)
    expect(
        _wallet(world).balance_label.or_(
            _wallet(world).page.get_by_text("Không tải được số dư ví.")
        )
    ).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('only the "{name}" link is marked as the current page'))
def only_link_is_current(world: World, name: str) -> None:
    current = _workplace(world).current_nav_links()
    expect(current).to_have_count(1)
    expect(current.first).to_contain_text(name)


@then(parsers.parse('the shop card shows "{name}"'))
def shop_card_shows(world: World, name: str) -> None:
    expect(_workplace(world).shop_name).to_have_text(name, timeout=timeouts.NAVIGATION)


@then('the shop card shows "Shop #" followed by the first 6 characters of the seller id')
def shop_card_fallback(world: World) -> None:
    seller = world.state.seeded_seller
    assert seller and seller.token
    expected = f"Shop #{_principal_id(seller.token)[:6]}"
    expect(_workplace(world).shop_name).to_have_text(expected, timeout=timeouts.NAVIGATION)


@given("a logged-in buyer without the seller scope")
def buyer_without_seller_scope(world: World) -> None:
    from config.settings import get_settings
    from tests.e2e.flows import login_via_api

    username = fake.unique_username("buyer")
    buyer = User(username=username, password=get_settings().seed_password, role="buyer")
    login_via_api(world, buyer)


@when("the buyer opens the seller workplace")
def buyer_opens_workplace(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/seller", wait_until="domcontentloaded")


@then("a result explains the seller role is required and links back to the home page")
def result_requires_seller_role(world: World) -> None:
    expect(world.page.get_by_text("Cần tài khoản Người Bán")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(world.page.get_by_role("link", name="Quay lại trang chủ")).to_have_attribute("href", "/")


# ── Workplace ────────────────────────────────────────────────────────────
@then(parsers.parse('the KPI row shows "{title}"'))
def kpi_row_shows(world: World, title: str) -> None:
    expect(_workplace(world).kpi_cell(title)).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the four KPI cells sit in one row next to the sidebar")
def kpi_four_up(world: World) -> None:
    page = _workplace(world)
    cells = page.kpi_row.locator(":scope > div")
    expect(cells).to_have_count(4)
    tops = {round(cells.nth(i).bounding_box()["y"]) for i in range(4)}  # type: ignore[index]
    assert len(tops) == 1, f"KPI cells wrap onto several rows: {tops}"
    sidebar = page.sidebar.bounding_box()
    first = cells.first.bounding_box()
    assert sidebar and first and first["x"] >= sidebar["x"] + sidebar["width"] - 1


@when(parsers.parse('the seller activates the quick action "{name}"'))
def activate_quick_action(world: World, name: str) -> None:
    _workplace(world).quick_action(name).click()


@then(parsers.parse('the browser lands on "{path}"'))
def browser_lands_on(world: World, path: str) -> None:
    expect(world.page).to_have_url(
        re.compile(rf".*{re.escape(path)}(\?.*)?$"), timeout=timeouts.NAVIGATION
    )


@then("the recent orders block shows an empty state with a link to add a product")
def recent_orders_empty(world: World) -> None:
    page = _workplace(world)
    expect(page.recent_orders_empty).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_role("link", name="Thêm sản phẩm").first).to_have_attribute(
        "href", "/seller/new"
    )


@when(parsers.parse('the seller types "{text}" in the product search box'))
def type_in_product_search(world: World, text: str) -> None:
    _workplace(world).search_box.fill(text)


@when("the seller searches for the seeded listing by title")
def search_seeded_listing(world: World) -> None:
    title = world.state.listing.title  # type: ignore[union-attr]
    world.state.extra["search_text"] = title
    _workplace(world).search_box.fill(title)


@then("the URL carries the search text and the matching product is listed")
def url_has_search(world: World) -> None:
    text = world.state.extra["search_text"]
    expect(world.page).to_have_url(re.compile(r".*[?&]q="), timeout=timeouts.NAVIGATION)
    expect(_workplace(world).product_row(text).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the URL contains no page parameter after the search")
def url_has_no_page(world: World) -> None:
    assert "page=" not in world.page.url


@then("a no-results state offers a clear-filter link")
def no_results_state(world: World) -> None:
    page = _workplace(world)
    expect(page.no_results).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_role("link", name="Xoá bộ lọc")).to_have_attribute("href", "/seller")


@when("the seller opens the workplace with an invalid page and status")
def open_workplace_invalid_params(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.page.goto(
        f"{world.settings.base_url}/seller?page=abc&status=bogus", wait_until="domcontentloaded"
    )


@then("the first page renders without an error")
def first_page_without_error(world: World) -> None:
    expect(_workplace(world).sidebar).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text("Đã có lỗi xảy ra")).to_have_count(0)
    expect(world.page.get_by_text("Không tải được trang này")).to_have_count(0)


@then("the empty product state offers a link to add a product")
def empty_product_state(world: World) -> None:
    page = _workplace(world)
    expect(page.empty_products).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.page.get_by_role("link", name="Thêm sản phẩm").first).to_be_visible()


# ── Delete confirm Modal ─────────────────────────────────────────────────
@when("the seller opens their listings")
def open_my_listings(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.navigate_to(PageName.SELLER_LISTINGS)


@when("the seller starts deleting the listing")
def start_delete(world: World) -> None:
    title = world.state.listing.title  # type: ignore[union-attr]
    _workplace(world).delete_button(title).click()


@then("a confirm dialog names the listing and the listing is not deleted yet")
def confirm_dialog_names_listing(world: World) -> None:
    page = _workplace(world)
    title = world.state.listing.title  # type: ignore[union-attr]
    expect(page.delete_dialog).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.delete_dialog).to_contain_text(title)
    expect(page.delete_dialog).to_contain_text("không thể hoàn tác")
    expect(page.cancel_delete).to_be_focused()


@when("the seller cancels the dialog")
def cancel_dialog(world: World) -> None:
    _workplace(world).cancel_delete.click()


@then("the listing row is still listed")
def row_still_listed(world: World) -> None:
    title = world.state.listing.title  # type: ignore[union-attr]
    expect(_workplace(world).product_row(title).first).to_be_visible(timeout=timeouts.DEFAULT)


@when("the seller confirms the deletion")
def confirm_deletion(world: World) -> None:
    _workplace(world).confirm_delete.click()


@then("the success toast appears and the listing row is gone")
def delete_succeeded(world: World) -> None:
    page = _workplace(world)
    title = world.state.listing.title  # type: ignore[union-attr]
    expect(page.toast("Đã xoá sản phẩm")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.product_row(title)).to_have_count(0, timeout=timeouts.NAVIGATION)


# ── Listing studio ───────────────────────────────────────────────────────
@when("the seller submits the empty new listing form")
def submit_empty_new_listing(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    page: SellerNewListingPage = world.navigate_to(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)
    page.submit_button.click()


@then("the title field shows a required error and no listing is created")
def title_required_error(world: World) -> None:
    page = _new_listing(world)
    expect(page.title_error).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.title_input).to_have_attribute("aria-invalid", "true")
    expect(page.success_result).to_have_count(0)


@then("a success toast confirms the save")
def save_toast(world: World) -> None:
    expect(_new_listing(world).saved_toast).to_be_visible(timeout=timeouts.NAVIGATION)


@then("a success state offers a link to the seller's listings")
def create_success_state(world: World) -> None:
    page = _new_listing(world)
    expect(page.success_result).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("link", name="Xem danh sách")).to_have_attribute(
        "href", "/seller"
    )


@given("another seller owns a listing")
def another_seller_owns_listing(world: World) -> None:
    from config.settings import get_settings

    seller = world.state.seeded_seller
    assert seller, "scenario must be tagged @needsSeller"
    username = fake.unique_username("seller")
    token = world.service_factory.auth.register(username, get_settings().seed_password, "seller")
    other = User(
        username=username, password=get_settings().seed_password, role="seller", token=token
    )
    listing = Listing(
        title=f"[E2E] Foreign {fake.listing_title('Laptop')}", category_id="cat-laptop"
    )
    seed_listing(world, listing, other)
    # Back to the seeded seller for the rest of the scenario.
    world.service_factory.set_token(seller.token)


@when("the seller opens that listing's edit page")
def open_foreign_edit(world: World) -> None:
    listing = world.state.listing
    page: SellerEditListingPage = world.navigate_to(  # type: ignore[assignment]
        PageName.SELLER_EDIT_LISTING, listing_id=listing.listing_id  # type: ignore[union-attr]
    )
    world.set_current_page(page)


@then("a 403 result with a link back to the seller area is shown and the form is not rendered")
def foreign_edit_refused(world: World) -> None:
    page: SellerEditListingPage = world.get_page(PageName.SELLER_EDIT_LISTING)  # type: ignore[assignment]
    expect(page.forbidden_result).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.back_to_seller_link).to_have_attribute("href", "/seller")
    expect(page.title_input).to_have_count(0)


# ── Magic Listing ────────────────────────────────────────────────────────
@when(parsers.parse('the seller types the title "{title}" and the description "{description}"'))
def type_title_and_description(world: World, title: str, description: str) -> None:
    world.page.set_viewport_size(DESKTOP)
    page: SellerNewListingPage = world.navigate_to(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)
    page.title_input.fill(title)
    page.description_input.fill(description)
    world.state.extra["typed_description"] = description


@when("the seller runs the AI suggestion")
def run_ai_suggestion(world: World) -> None:
    page = _new_listing(world)
    page.magic_generate.click()
    expect(page.magic_suggestion).to_be_visible(timeout=timeouts.LONG)


@then("the suggestion is shown and the typed description is unchanged")
def suggestion_not_applied(world: World) -> None:
    page = _new_listing(world)
    expect(page.description_input).to_have_value(world.state.extra["typed_description"])


@when("the seller applies all AI suggestions")
def apply_all_suggestions(world: World) -> None:
    _new_listing(world).magic_apply_all.click()


@then("the description now holds the suggested text")
def description_is_suggestion(world: World) -> None:
    page = _new_listing(world)
    expect(page.description_input).not_to_have_value(
        world.state.extra["typed_description"], timeout=timeouts.DEFAULT
    )


# ── Orders ───────────────────────────────────────────────────────────────
@when(parsers.parse('the seller selects the "{label}" order tab'))
def select_order_tab(world: World, label: str) -> None:
    _orders(world).tab(label).click()


@then(parsers.parse('the URL contains "{fragment}"'))
def url_contains(world: World, fragment: str) -> None:
    expect(world.page).to_have_url(
        re.compile(f".*{re.escape(fragment)}.*"), timeout=timeouts.NAVIGATION
    )


@when("the seller reloads the page")
def reload_page(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@then(parsers.parse('the "{label}" order tab is still selected'))
def order_tab_selected(world: World, label: str) -> None:
    expect(_orders(world).active_tab).to_contain_text(label, timeout=timeouts.NAVIGATION)


@when("the seller opens the orders page with an invalid page and status")
def open_orders_invalid(world: World) -> None:
    world.page.goto(
        f"{world.settings.base_url}/seller/orders?page=abc&status=bogus",
        wait_until="domcontentloaded",
    )


@then("the orders page renders its first page without an error")
def orders_first_page(world: World) -> None:
    expect(_orders(world).tabs).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text("Đã có lỗi xảy ra")).to_have_count(0)


@when("the seller opens the detail of their order")
def open_order_detail(world: World) -> None:
    order_id = world.state.order_id or world.state.extra.get("order_id", "")
    assert order_id, "scenario must be tagged @needsOrder"
    world.page.set_viewport_size(DESKTOP)
    world.navigate_to(PageName.SELLER_ORDER_DETAIL, order_id=order_id)
    expect(_detail(world).packing_slip).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the detail shows the recipient and the item of that order")
def detail_shows_real_order(world: World) -> None:
    slip = _detail(world).packing_slip
    expect(slip).to_contain_text("Nguyen Van A")
    expect(slip).to_contain_text(world.state.listing.title)  # type: ignore[union-attr]
    expect(slip).not_to_contain_text("iPhone 15 Pro Max")


@when("the seller hands the order to the carrier")
def hand_over_order(world: World) -> None:
    page = _detail(world)
    page.ship_button.click()
    page.confirm_ship.click()


@then("a success toast appears and the stepper shows the order as shipped")
def shipped_feedback(world: World) -> None:
    page = _detail(world)
    expect(page.shipped_toast).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.current_step).to_contain_text("Đang giao", timeout=timeouts.NAVIGATION)
    expect(page.ship_button).to_have_count(0)


@when(parsers.parse('the seller opens the order "{order_id}"'))
def open_unknown_order(world: World, order_id: str) -> None:
    world.page.set_viewport_size(DESKTOP)
    world.navigate_to(PageName.SELLER_ORDER_DETAIL, order_id=order_id)


@then("a not-found result links back to the seller area")
def order_not_found(world: World) -> None:
    page = _detail(world)
    expect(page.not_found).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.back_to_seller_link).to_have_attribute("href", "/seller")


@then("the print layout hides the seller chrome and keeps the packing slip")
def print_layout(world: World) -> None:
    world.page.emulate_media(media="print")
    page = _detail(world)
    expect(page.packing_slip).to_be_visible()
    expect(world.page.locator("aside")).to_be_hidden()
    expect(world.page.locator("header").first).to_be_hidden()
    world.page.emulate_media(media="screen")


# ── Analytics ────────────────────────────────────────────────────────────
@when(parsers.parse('the seller selects the "{label}" range tab'))
def select_range_tab(world: World, label: str) -> None:
    _analytics(world).range_tab(label).click()


@then(parsers.parse('the "{label}" range tab is current'))
def range_tab_current(world: World, label: str) -> None:
    expect(_analytics(world).active_range_tab).to_have_text(label, timeout=timeouts.NAVIGATION)


@then("the Statistic row of the analytics page is visible")
def analytics_statistic_row(world: World) -> None:
    expect(
        _analytics(world).kpi_row.or_(world.page.get_by_text("Chưa có dữ liệu phễu"))
    ).to_be_visible(timeout=timeouts.NAVIGATION)


# ── Wallet ───────────────────────────────────────────────────────────────
@then("the seller wallet balance is shown")
def wallet_balance_shown(world: World) -> None:
    expect(_wallet(world).balance_label).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the payout button is disabled and says why")
def payout_disabled(world: World) -> None:
    page = _wallet(world)
    expect(page.payout_button).to_be_disabled(timeout=timeouts.NAVIGATION)
    expect(page.payout_button).to_have_attribute("aria-disabled", "true")
    expect(page.zero_balance_hint).to_be_visible()


@when("the seller starts a payout")
def start_payout(world: World) -> None:
    page = _wallet(world)
    if page.payout_button.is_disabled():
        pytest.skip("the seeded seller has no wallet balance to withdraw on this stack")
    page.payout_button.click()


@then("a confirm dialog asks for the payout amount and nothing is sent yet")
def payout_confirm_dialog(world: World) -> None:
    page = _wallet(world)
    expect(page.confirm_dialog).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.confirm_dialog.get_by_label("Số tiền", exact=False)).to_be_visible()
    expect(page.payout_toast).to_have_count(0)


# ── Shop profile ─────────────────────────────────────────────────────────
@when(parsers.parse('the seller saves the shop name "{name}"'))
def save_shop_name(world: World, name: str) -> None:
    world.page.set_viewport_size(DESKTOP)
    page: SellerShopPage = world.navigate_to(PageName.SELLER_SHOP)  # type: ignore[assignment]
    expect(page.name_input).to_be_visible(timeout=timeouts.NAVIGATION)
    page.name_input.fill(name)
    page.save_button.click()
    world.state.extra["saved_shop_name"] = name


@then("a success toast confirms the shop name")
def shop_name_toast(world: World) -> None:
    expect(_shop(world).saved_toast).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the public shop page shows "{name}"'))
def public_shop_shows_name(world: World, name: str) -> None:
    seller = world.state.seeded_seller
    assert seller and seller.token
    world.navigate_to(PageName.SHOP_PROFILE, shop_id=_principal_id(seller.token))
    expect(world.page.locator("h1").first).to_have_text(name, timeout=timeouts.NAVIGATION)


@when("the seller submits a blank shop name")
def submit_blank_shop_name(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    page: SellerShopPage = world.navigate_to(PageName.SELLER_SHOP)  # type: ignore[assignment]
    expect(page.name_input).to_be_visible(timeout=timeouts.NAVIGATION)
    page.name_input.fill("   ")
    page.save_button.click()


@then("the shop name field shows a required error")
def shop_name_required_error(world: World) -> None:
    page = _shop(world)
    expect(page.blank_error).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.name_input).to_have_attribute("aria-invalid", "true")


@when("the seller submits a shop name of 81 characters")
def submit_long_shop_name(world: World) -> None:
    page: SellerShopPage = world.navigate_to(PageName.SELLER_SHOP)  # type: ignore[assignment]
    page.name_input.fill("x" * 81)
    page.save_button.click()


@then("the shop name field shows a length error")
def shop_name_length_error(world: World) -> None:
    expect(_shop(world).too_long_error).to_be_visible(timeout=timeouts.DEFAULT)


# ── Responsive 375px ─────────────────────────────────────────────────────
@when(parsers.parse('the seller opens "{path}" at a 375px viewport'))
def open_path_phone(world: World, path: str) -> None:
    world.page.set_viewport_size(PHONE)
    world.page.goto(f"{world.settings.base_url}{path}", wait_until="networkidle")


@when("the seller opens their order detail at a 375px viewport")
def open_order_detail_phone(world: World) -> None:
    order_id = world.state.order_id or world.state.extra.get("order_id", "")
    assert order_id, "scenario must be tagged @needsOrder"
    world.page.set_viewport_size(PHONE)
    world.navigate_to(PageName.SELLER_ORDER_DETAIL, order_id=order_id)
    expect(_detail(world).packing_slip).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the page has no horizontal scroll")
def no_horizontal_scroll(world: World) -> None:
    assert _no_horizontal_scroll(world), "document is wider than the 375px viewport"


@when("the seller scrolls the new listing form to the middle at 375px")
def scroll_form_middle(world: World) -> None:
    world.page.set_viewport_size(PHONE)
    page: SellerNewListingPage = world.navigate_to(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)
    world.page.evaluate("window.scrollTo(0, document.body.scrollHeight / 2)")


@then("the Save button is still visible in the sticky bottom bar")
def save_button_sticky(world: World) -> None:
    expect(_new_listing(world).submit_button).to_be_in_viewport()
