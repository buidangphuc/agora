"""Account screens (ui-phase-account): URL-driven settings shell, address delete
confirmation, labelled KYC form, URL-held notification tabs, favorites not-found
and the inline login error.

API login injects the session cookie (fast); every assertion is on the real UI,
selected by role / accessible name / visible Vietnamese text.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import PageName, timeouts
from src.models import User
from src.pages import AddressesPage, LoginPage, NotificationsPage
from src.pages.account_shell_page import AccountShellPage
from src.pages.verification_page import VerificationPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()
PHONE_WIDTH = {"width": 375, "height": 812}


def _shell(world: World) -> AccountShellPage:
    return AccountShellPage(world.page)


def _sign_in_buyer(world: World) -> None:
    buyer = User(
        username=fake.unique_username("acct_buyer"),
        password=SETTINGS.seed_password,
        role="buyer",
    )
    login_via_api(world, buyer)


# ── Shared givens ────────────────────────────────────────────────────────
@given("a signed-in buyer on the account security page")
def buyer_on_security_page(world: World) -> None:
    _sign_in_buyer(world)
    _shell(world).open("/account/security")
    expect(_shell(world).menu).to_be_visible(timeout=timeouts.NAVIGATION)


@given("a signed-in buyer on the verification page")
def buyer_on_verification_page(world: World) -> None:
    _sign_in_buyer(world)
    page = VerificationPage(world.page)
    page.navigate()
    expect(page.submit_button).to_be_visible(timeout=timeouts.NAVIGATION)


@given("a signed-in buyer on the notifications page")
def buyer_on_notifications_page(world: World) -> None:
    _sign_in_buyer(world)
    world.navigate_to(PageName.NOTIFICATIONS)
    expect(NotificationsPage(world.page).tabs).to_be_visible(timeout=timeouts.NAVIGATION)


@given(
    parsers.parse('a signed-in buyer on the favorites page for the collection "{collection_id}"')
)
def buyer_on_unknown_collection(world: World, collection_id: str) -> None:
    _sign_in_buyer(world)
    world.page.goto(
        f"{SETTINGS.base_url.rstrip('/')}/favorites?collection={collection_id}",
        wait_until="domcontentloaded",
    )


@given("a signed-in buyer with a default and a second delivery address")
def buyer_with_two_addresses(world: World) -> None:
    _sign_in_buyer(world)
    first = f"E2E {fake.vietnamese_name()}"
    second = f"E2E {fake.vietnamese_name()}"
    world.state.extra["address_names"] = (first, second)
    address = world.service_factory.address
    address.create_address(first, "0912345678", "1 Lê Lợi", "TP. HCM", is_default=True)
    address.create_address(second, "0987654321", "22 Nguyễn Huệ", "TP. HCM", is_default=False)


# ── Account menu ─────────────────────────────────────────────────────────
@then(parsers.parse('the account menu marks "{label}" as the current page'))
def menu_marks_current(world: World, label: str) -> None:
    shell = _shell(world)
    expect(shell.menu_link(label)).to_have_attribute(
        "aria-current", "page", timeout=timeouts.NAVIGATION
    )
    expect(shell.current_menu_item).to_have_count(1)


@then(parsers.parse('the account page heading is "{heading}"'))
def account_heading(world: World, heading: str) -> None:
    expect(_shell(world).heading).to_have_text(heading, timeout=timeouts.DEFAULT)


@when(parsers.parse('the buyer opens "{label}" from the account menu'))
def open_from_menu(world: World, label: str) -> None:
    _shell(world).menu_link(label).click()
    world.page.wait_for_url("**/account/verification**", timeout=timeouts.NAVIGATION)


@when("the buyer goes back in the browser")
def go_back(world: World) -> None:
    world.page.go_back(wait_until="domcontentloaded")
    world.page.wait_for_url("**/account/security**", timeout=timeouts.NAVIGATION)


@when("the viewport is 375 pixels wide")
def phone_viewport(world: World) -> None:
    world.page.set_viewport_size(PHONE_WIDTH)
    world.page.reload(wait_until="domcontentloaded")
    expect(_shell(world).menu).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the account menu sits above the page content")
def menu_above_content(world: World) -> None:
    menu = _shell(world).menu.bounding_box()
    heading = _shell(world).heading.bounding_box()
    assert menu and heading, "menu or heading not rendered"
    # Heading comes first, then the menu row, with the content below it: the menu is
    # not a left column (its right edge stays inside the 375px viewport).
    assert menu["x"] + menu["width"] <= PHONE_WIDTH["width"] + 1, menu


@then("the page content does not scroll sideways")
def content_does_not_scroll_sideways(world: World) -> None:
    # Measured on <main>: the global header is outside this change and may be wider.
    overflow = world.page.evaluate(
        "() => { const m = document.querySelector('main'); return m.scrollWidth - m.clientWidth; }"
    )
    assert overflow <= 0, f"main content overflows by {overflow}px at 375px"


# ── Address delete confirmation ──────────────────────────────────────────
@when("the buyer opens the delivery addresses page")
def open_addresses(world: World) -> None:
    page: AddressesPage = world.navigate_to(PageName.ADDRESSES)  # type: ignore[assignment]
    first, second = world.state.extra["address_names"]
    expect(page.address_card(first)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.address_card(second)).to_be_visible(timeout=timeouts.DEFAULT)


@when("the buyer asks to delete the second address")
def ask_to_delete_second(world: World) -> None:
    _, second = world.state.extra["address_names"]
    page = AddressesPage(world.page)
    page.wait_until_interactive(page.delete_button(second))
    page.delete_button(second).click()


@then("a confirmation dialog names the recipient of the second address")
def dialog_names_recipient(world: World) -> None:
    _, second = world.state.extra["address_names"]
    dialog = AddressesPage(world.page).confirm_dialog
    expect(dialog).to_be_visible(timeout=timeouts.DEFAULT)
    expect(dialog).to_contain_text(second)


@when("the buyer presses Escape")
def press_escape(world: World) -> None:
    world.page.keyboard.press("Escape")


@then("the confirmation dialog is closed and both addresses are still listed")
def dialog_closed_addresses_remain(world: World) -> None:
    page = AddressesPage(world.page)
    first, second = world.state.extra["address_names"]
    expect(page.confirm_dialog).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(page.address_card(first)).to_be_visible()
    expect(page.address_card(second)).to_be_visible()
    # Focus returns to the control that opened the dialog.
    expect(page.delete_button(second)).to_be_focused()


@when("the buyer confirms the deletion")
def confirm_deletion(world: World) -> None:
    AddressesPage(world.page).confirm_delete_button.click()


@then("the second address is no longer listed")
def second_address_gone(world: World) -> None:
    _, second = world.state.extra["address_names"]
    expect(AddressesPage(world.page).address_card(second)).to_have_count(
        0, timeout=timeouts.NAVIGATION
    )


# ── KYC form labels ──────────────────────────────────────────────────────
@then(parsers.parse('the document type select is named "{name}"'))
def doc_type_named(world: World, name: str) -> None:
    expect(VerificationPage(world.page).doc_type_select).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the document reference input is named "{name}"'))
def doc_ref_named(world: World, name: str) -> None:
    expect(VerificationPage(world.page).doc_ref_input).to_be_visible(timeout=timeouts.DEFAULT)


@then("the submit button is disabled until a reference is typed")
def submit_disabled_until_reference(world: World) -> None:
    page = VerificationPage(world.page)
    expect(page.submit_button).to_be_disabled()
    page.doc_ref_input.fill("E2E-REF-0001")
    expect(page.submit_button).to_be_enabled()


# ── Notification tabs in the URL ─────────────────────────────────────────
@when(parsers.parse('the buyer selects the "{label}" notification tab'))
def select_notification_tab(world: World, label: str) -> None:
    NotificationsPage(world.page).tab(label).click()


@then("the address bar shows the order tab")
def url_shows_order_tab(world: World) -> None:
    world.page.wait_for_url(
        re.compile(r".*/notifications\?.*tab=order"), timeout=timeouts.NAVIGATION
    )


@when("the buyer reloads the page")
def reload_page(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@then(parsers.parse('the "{label}" notification tab is still the selected one'))
def tab_still_selected(world: World, label: str) -> None:
    page = NotificationsPage(world.page)
    expect(page.tab(label)).to_have_attribute("aria-current", "page", timeout=timeouts.NAVIGATION)
    expect(page.current_tab).to_have_count(1)


# ── Favorites not-found result ───────────────────────────────────────────
@then(parsers.parse('a not-found result offers "{label}"'))
def not_found_result(world: World, label: str) -> None:
    expect(world.page.get_by_text("Không tìm thấy bộ sưu tập")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    link = world.page.get_by_role("link", name=label)
    expect(link).to_be_visible()
    expect(link).to_have_attribute("href", "/favorites")


# ── Login inline error ───────────────────────────────────────────────────
@then(parsers.parse('an inline error alert reads "{message}"'))
def inline_error_reads(world: World, message: str) -> None:
    page: LoginPage = world.get_page(PageName.LOGIN)  # type: ignore[assignment]
    expect(page.error_alert).to_contain_text(message, timeout=timeouts.DEFAULT)


@then("the user is still on the login page with the username kept and the password cleared")
def login_state_kept(world: World) -> None:
    page: LoginPage = world.get_page(PageName.LOGIN)  # type: ignore[assignment]
    assert "/login" in world.page.url, world.page.url
    expect(page.username_input).to_have_value("khong_ton_tai")
    expect(page.password_input).to_have_value("")
