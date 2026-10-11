"""Steps for frontend/uif_seller.feature (ui-phase-seller failure paths)."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.support import d2_support as d2
from tests.e2e.support.uif_support import ALERTS, base
from tests.e2e.support.world import World


def _seller(world: World) -> dict:
    return world.state.extra["d2_seller"]


@given(parsers.parse("a uif seller has {count:d} published listings"))
def seller_with_listings(world: World, count: int) -> None:
    d2.seed_seller_session(world, listings=count)


@when("the uif seller opens the workplace")
def open_workplace(world: World) -> None:
    world.page.goto(f"{base(world)}/seller", wait_until="domcontentloaded")


@then(
    "the product KPIs render, the open-orders KPI is absent and no placeholder zero stands in for it"
)
def kpi_hidden(world: World) -> None:
    row = world.page.get_by_test_id("kpi-row")
    expect(row).to_be_visible(timeout=timeouts.NAVIGATION)
    for title in ("Tổng sản phẩm", "Đang bán"):
        expect(row.get_by_text(title)).to_be_visible()
    expect(row.get_by_text(re.compile(r"Sắp hết hàng"))).to_be_visible()
    expect(row.get_by_text("Đơn chờ xử lý")).to_have_count(0)
    # three cells only, and none of them is a stand-in zero for the missing source
    expect(row.locator("> *")).to_have_count(3)
    assert "Đơn chờ xử lý" not in row.inner_text()


@when("the uif seller asks for an AI suggestion for a typed title")
def ask_magic(world: World) -> None:
    page = world.page
    page.goto(f"{base(world)}/seller/new", wait_until="domcontentloaded")
    page.locator("#title").fill("Laptop Dell XPS 13")
    button = page.get_by_role("button", name="Tạo gợi ý")
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    button.click()


@then(
    "an alert with a retry button and an error toast are shown and the seller can still submit manually"
)
def magic_failed(world: World) -> None:
    page = world.page
    alerts = page.locator(ALERTS)
    form_alert = alerts.filter(has=page.get_by_role("button", name="Thử lại"))
    expect(form_alert.first).to_be_visible(timeout=timeouts.NAVIGATION)
    # the toast is a second role=alert carrying the same message
    expect(alerts.filter(has_text=re.compile(r"\S{4,}"))).not_to_have_count(0)
    expect(page.get_by_test_id("magic-suggestion")).to_have_count(0)
    page.locator("#title").fill("Laptop Dell XPS 13 nhập tay")
    page.locator("#description").fill("Mô tả nhập tay")
    page.locator("#price").fill("1500000")
    page.locator("#stock").fill("3")
    page.locator("#categoryId").select_option("cat-electronics")
    submit = page.get_by_role("button", name="Đăng bán ngay")
    expect(submit).to_be_enabled()
    submit.click()
    expect(page.get_by_role("link", name="Xem sản phẩm")).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the uif seller opens the analytics page")
def open_analytics(world: World) -> None:
    world.page.goto(f"{base(world)}/seller/analytics", wait_until="domcontentloaded")


@then(
    "the funnel shows an alert with a retry link to the same URL and the rest of the page still renders"
)
def analytics_failed(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Báo cáo doanh thu")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    alert = page.locator(ALERTS).filter(has_text="Không tải được dữ liệu phễu chuyển đổi")
    expect(alert).to_be_visible()
    # the retry link points back at this page (same range)
    expect(alert.get_by_role("link", name="Thử lại")).to_have_attribute(
        "href", re.compile(r"^/seller/analytics\?range=")
    )
    expect(page.get_by_text("Doanh thu theo ngày", exact=True)).to_be_visible()
    expect(page.get_by_role("navigation", name="Tabs")).to_be_visible()


@when("the uif seller opens the edit page of the listing")
def open_edit(world: World) -> None:
    listing_id = _seller(world)["listings"][0]["id"]
    world.page.goto(f"{base(world)}/seller/{listing_id}/edit", wait_until="domcontentloaded")


@then('the route error result offers a retry button and a link to "/seller"')
def route_error(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Không tải được trang này")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("button", name="Thử lại")).to_be_visible()
    expect(page.get_by_role("link", name="Về Kênh người bán")).to_have_attribute("href", "/seller")


@when("the seller presses the retry button")
def press_retry(world: World) -> None:
    world.page.get_by_role("button", name="Thử lại").click()


@then("the edit form of the listing is rendered")
def edit_form(world: World) -> None:
    title = _seller(world)["listings"][0]["title"]
    expect(world.page.locator("#title")).to_have_value(title, timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text("Không tải được trang này")).to_have_count(0)


@then(
    "the product list shows an error alert with a retry link above no table rows and no raw error text"
)
def list_failed(world: World) -> None:
    page = world.page
    alert = page.locator(ALERTS).filter(has_text="Không tải được danh sách sản phẩm")
    expect(alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(alert.get_by_role("link", name="Thử lại")).to_have_attribute("href", "/seller")
    card = page.locator("section, div", has=alert).last
    expect(card.locator("tbody tr")).to_have_count(0)
    body = page.inner_text("main")
    assert not re.search(r"\[(unavailable|internal|unknown)\]|ConnectError|Error:", body), body
