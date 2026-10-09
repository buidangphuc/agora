"""Steps for frontend/uif_account.feature (ui-phase-account failure paths)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from tests.e2e.support import d2_support as d2
from tests.e2e.support.uif_support import ALERTS, base
from tests.e2e.support.world import World


@given("a uif buyer is logged in")
def buyer_logged_in(world: World) -> None:
    d2.seed_buyer(world)


@when("the uif buyer opens the security page")
def open_security(world: World) -> None:
    world.page.goto(f"{base(world)}/account/security", wait_until="domcontentloaded")


@then(
    "the sessions section shows an inline error alert with a retry action and the login history section is rendered"
)
def sessions_failed(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Phiên đăng nhập", exact=True)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    alert = page.locator(ALERTS).filter(has_text="Không thể tải danh sách phiên đăng nhập")
    expect(alert).to_be_visible()
    expect(alert.get_by_role("button", name="Thử lại")).to_be_visible()
    expect(page.get_by_role("heading", name="Lịch sử đăng nhập", exact=True)).to_be_visible()
    expect(page.get_by_role("heading", name="Bảo mật tài khoản")).to_be_visible()


@when("the buyer presses the retry action of the sessions alert")
def press_sessions_retry(world: World) -> None:
    world.page.locator(ALERTS).filter(
        has_text="Không thể tải danh sách phiên đăng nhập"
    ).get_by_role("button", name="Thử lại").click()


@then("the sessions section lists the buyer's session")
def sessions_listed(world: World) -> None:
    page = world.page
    expect(page.locator(ALERTS).filter(has_text="Không thể tải")).to_have_count(
        0, timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_role("table", name="Phiên đăng nhập").locator("tbody tr")).not_to_have_count(
        0
    )


@when("the uif buyer opens the notifications page")
def open_notifications(world: World) -> None:
    world.page.goto(f"{base(world)}/notifications", wait_until="domcontentloaded")


@then("the route error result offers a retry button")
def route_error(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Đã có lỗi xảy ra")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("button", name="Thử lại")).to_be_visible()


@then("the notifications page is rendered")
def notifications_again(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Đã có lỗi xảy ra")).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(page.get_by_role("heading", name="Thông báo").first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
