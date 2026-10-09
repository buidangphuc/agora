"""Steps for the seller cockpit (OpenSpec change ui-phase-seller), features
frontend/ui_seller_*.feature. Sellers, listings, buyers and orders are seeded through the
gateway; the browser is logged in as the seller."""

from __future__ import annotations

import re
import time

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages import (
    SellerListingsPage,
    SellerNewListingPage,
    SellerOrderDetailPage,
    SellerOrdersPage,
)
from tests.e2e.step_definitions.d2_cart_steps import toast
from tests.e2e.step_definitions.d2_checkout_steps import slow_actions, track_actions
from tests.e2e.support import d2_support as d2
from tests.e2e.support.world import World

DESKTOP = {"width": 1280, "height": 800}
PHONE = {"width": 375, "height": 812}


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def go(world: World, path: str, viewport: dict | None = None) -> None:
    world.page.set_viewport_size(viewport or DESKTOP)
    world.page.goto(f"{base(world)}{path}", wait_until="domcontentloaded")


def work(world: World) -> SellerListingsPage:
    return SellerListingsPage(world.page)


def sorders(world: World) -> SellerOrdersPage:
    return SellerOrdersPage(world.page)


def sdetail(world: World) -> SellerOrderDetailPage:
    return SellerOrderDetailPage(world.page)


def studio(world: World) -> SellerNewListingPage:
    return SellerNewListingPage(world.page)


def seller(world: World) -> dict:
    return world.state.extra["d2_seller"]


def kpi_value(world: World, title: str) -> int:
    """The value of a KPI cell: the last number of its text (titles can carry one too)."""
    cell = work(world).kpi_row.locator(":scope > div", has_text=title)
    if cell.count() == 0:
        return -2
    numbers = re.findall(r"\d+", cell.first.inner_text().replace(".", ""))
    return int(numbers[-1]) if numbers else -1


# ── Seeding ──────────────────────────────────────────────────────────────
@given(parsers.parse("a d2 seller has {count:d} published listings, one with stock {low:d}"))
def seller_with_low_stock(world: World, count: int, low: int) -> None:
    s = d2.seed_seller_session(world, listings=count)
    from src.api.services import ListingService

    ListingService(token=s["token"]).update_listing(s["listings"][0]["id"], stock=low)


@given(parsers.parse("a d2 seller has {count:d} published listings"))
def seller_with_listings(world: World, count: int) -> None:
    d2.seed_seller_session(world, listings=count)


@given(parsers.parse("a d2 seller has {count:d} published listings with thumbnails"))
def seller_with_listings_images(world: World, count: int) -> None:
    d2.seed_seller_session(world, listings=count, image=True)


@given("a d2 seller has a listing and a paid order")
def seller_with_order(world: World) -> None:
    s = d2.seed_seller_session(world, listings=1)
    d2.seller_order(world, s)


# ── Workplace ────────────────────────────────────────────────────────────
@when("the d2 seller opens the workplace")
def open_workplace(world: World) -> None:
    go(world, "/seller")
    expect(work(world).sidebar).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the d2 seller opens "{path}"'))
def open_path(world: World, path: str) -> None:
    go(world, path)


@then("the KPI row shows total 3, published 3 and low stock 1 derived from the listings")
def kpi_values(world: World) -> None:
    expect(work(world).kpi_row).to_be_visible(timeout=timeouts.NAVIGATION)
    assert kpi_value(world, "Tổng sản phẩm") == 3
    assert kpi_value(world, "Đang bán") == 3
    assert kpi_value(world, "Sắp hết hàng") == 1


@then("every statistic is derived from the empty response and shows zero listings")
def kpi_zero(world: World) -> None:
    expect(work(world).kpi_row).to_be_visible(timeout=timeouts.NAVIGATION)
    assert kpi_value(world, "Tổng sản phẩm") == 0
    assert kpi_value(world, "Đang bán") == 0
    assert kpi_value(world, "Sắp hết hàng") == 0
    assert kpi_value(world, "Đơn chờ xử lý") == 0


@then("the four KPI cells appear in one row next to the sidebar")
def four_up(world: World) -> None:
    page = work(world)
    expect(page.kpi_row).to_be_visible(timeout=timeouts.NAVIGATION)
    cells = page.kpi_row.locator(":scope > div")
    expect(cells).to_have_count(4)
    tops = {round(cells.nth(i).bounding_box()["y"]) for i in range(4)}
    assert len(tops) == 1, tops
    assert cells.first.bounding_box()["x"] >= page.sidebar.bounding_box()["width"] - 1


@when(parsers.parse('the d2 seller activates the quick action "{name}"'))
def quick_action(world: World, name: str) -> None:
    page = work(world)
    page.wait_until_interactive(page.quick_action(name))
    page.quick_action(name).click()


@then(parsers.parse('the seller browser lands on "{path}"'))
def seller_lands(world: World, path: str) -> None:
    expect(world.page).to_have_url(
        re.compile(rf".*{re.escape(path)}(\?.*)?$"), timeout=timeouts.NAVIGATION
    )


# ── Table list: pagination, links ────────────────────────────────────────
@then(
    "rows 21 to 40 are shown, page 2 is current and the previous and next links point at pages 1 and 3"
)
def seller_page_two(world: World) -> None:
    page = world.page
    expect(page.locator("tbody tr")).to_have_count(20, timeout=timeouts.NAVIGATION)
    nav = page.get_by_role("navigation", name="Phân trang", exact=True)
    expect(nav.locator('[aria-current="page"]')).to_have_text("2")
    prev = nav.get_by_role("link", name="Trang trước")
    nxt = nav.get_by_role("link", name="Trang sau")
    assert "page=3" in nxt.get_attribute("href"), nxt.get_attribute("href")
    prev_href = prev.get_attribute("href")
    assert "page=2" not in prev_href and "page=3" not in prev_href, prev_href

    def titles_on(path: str) -> list[str]:
        page.goto(f"{base(world)}{path}", wait_until="domcontentloaded")
        rows = page.locator("tbody tr")
        expect(rows.first).to_be_visible(timeout=timeouts.NAVIGATION)
        return [t for t in (row.get_by_role("link").first.inner_text() for row in rows.all()) if t]

    second = titles_on("/seller?page=2")
    first = titles_on(prev_href)
    third = titles_on("/seller?page=3")
    assert (len(first), len(second), len(third)) == (20, 20, 5), (
        len(first),
        len(second),
        len(third),
    )
    everything = first + second + third
    assert len(set(everything)) == 45, "pages overlap"
    assert set(everything) == {item["title"] for item in seller(world)["listings"]}


@when("the d2 seller activates the title link of the first product row")
def click_row_link(world: World) -> None:
    link = world.page.locator("tbody tr").first.locator('a[href^="/listing/"]').first
    world.state.extra["d2_row_href"] = link.get_attribute("href")
    link.click()


@then("the browser opens the public listing page of that product")
def row_link_target(world: World) -> None:
    href = world.state.extra["d2_row_href"]
    world.page.wait_for_url(re.compile(rf".*{re.escape(href)}$"), timeout=timeouts.NAVIGATION)
    assert re.fullmatch(r"/listing/[0-9a-f-]{36}", href), href


@then("the browser lands on the new listing studio")
def lands_studio(world: World) -> None:
    world.page.wait_for_url(re.compile(r".*/seller/new$"), timeout=timeouts.NAVIGATION)
    expect(studio(world).title_input).to_be_visible(timeout=timeouts.DEFAULT)


# ── Delete Modal ─────────────────────────────────────────────────────────
@when("the d2 seller opens the delete Modal of the first listing")
def open_delete_modal(world: World) -> None:
    item = seller(world)["listings"][0]
    page = work(world)
    button = page.delete_button(item["title"])
    expect(button).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_until_interactive(button)
    button.click()
    expect(page.delete_dialog).to_be_visible(timeout=timeouts.DEFAULT)
    world.state.extra["d2_deleted"] = item


@when("the d2 seller confirms the deletion while the server is slow")
def confirm_delete_slow(world: World) -> None:
    track_actions(world)
    slow_actions(world, 1.5, "**/seller**")
    work(world).confirm_delete.click()


@then("the confirm button is busy and disabled and the dialog cannot be dismissed")
def delete_pending(world: World) -> None:
    page = work(world)
    expect(page.confirm_delete).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(page.confirm_delete).to_be_disabled()
    expect(page.cancel_delete).to_be_disabled()
    world.page.keyboard.press("Escape")
    expect(page.delete_dialog).to_be_visible()


@then("the Modal closes, a success toast appears and the row is gone from the table")
def delete_done(world: World) -> None:
    page = work(world)
    item = world.state.extra["d2_deleted"]
    expect(page.toast("Đã xoá sản phẩm")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.delete_dialog).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(page.product_row(item["title"])).to_have_count(0, timeout=timeouts.NAVIGATION)


@given("the first listing was already deleted from another session")
@when("the first listing is deleted from another session")
def deleted_elsewhere(world: World) -> None:
    from src.api.services import ListingService

    s = seller(world)
    ListingService(token=s["token"]).delete_listing(s["listings"][0]["id"])


@when("the d2 seller confirms the deletion")
def confirm_delete(world: World) -> None:
    work(world).confirm_delete.click()


@then(
    "the Modal stays open with an alert, an error toast appears and the confirm button is enabled"
)
def delete_failed(world: World) -> None:
    page = work(world)
    expect(page.delete_dialog.get_by_role("alert")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(
        world.page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible()
    expect(page.delete_dialog).to_be_visible()
    expect(page.confirm_delete).to_be_enabled()


# ── Listing studio ───────────────────────────────────────────────────────
@when("the d2 seller opens the new listing studio")
def open_studio(world: World) -> None:
    go(world, "/seller/new")
    page = studio(world)
    expect(page.title_input).to_be_visible(timeout=timeouts.DEFAULT)
    page.wait_until_interactive(page.submit_button)


@when("the d2 seller submits the form with an empty title")
def submit_empty_title(world: World) -> None:
    track_actions(world)
    studio(world).submit_button.click()


@then(
    "the title form item shows an error linked by aria-describedby, the input is invalid and no request was sent"
)
def title_error_linked(world: World) -> None:
    page = studio(world)
    expect(page.title_error).to_be_visible(timeout=timeouts.DEFAULT)
    title = page.title_input
    expect(title).to_have_attribute("aria-invalid", "true")
    described = title.get_attribute("aria-describedby")
    assert described, "the input has no aria-describedby"
    target = world.page.locator(f'[id="{described.split()[0]}"]')
    expect(target).to_have_text("Tiêu đề bắt buộc.")
    assert world.state.extra["d2_posts"] == []


def fill_valid_listing(world: World, title: str) -> None:
    page = studio(world)
    page.title_input.fill(title)
    page.category_select.select_option("cat-electronics")
    page.price_input.fill("250000")
    page.stock_input.fill("7")


@when("the d2 seller submits a valid listing while the server is slow")
def submit_valid_slow(world: World) -> None:
    page = studio(world)
    title = f"[E2E][d2] new listing {int(time.time())}"
    world.state.extra["d2_new_title"] = title
    fill_valid_listing(world, title)
    world.state.extra["d2_width"] = page.submit_button.bounding_box()["width"]
    track_actions(world)
    slow_actions(world, 1.5, "**/seller/new**")
    page.submit_button.click()


@then(
    "the submit button shows a spinner with unchanged width, a second click does nothing and a saved toast appears"
)
def submit_pending(world: World) -> None:
    page = studio(world)
    button = page.submit_button
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(button).to_be_disabled()
    assert abs(button.bounding_box()["width"] - world.state.extra["d2_width"]) < 1
    button.click(force=True)
    expect(page.saved_toast).to_be_visible(timeout=timeouts.NAVIGATION)
    assert len(world.state.extra["d2_posts"]) == 1, world.state.extra["d2_posts"]


@when("the d2 seller creates a listing with valid data")
def create_valid(world: World) -> None:
    page = studio(world)
    title = f"[E2E][d2] created listing {int(time.time())}"
    world.state.extra["d2_new_title"] = title
    fill_valid_listing(world, title)
    page.submit_button.click()


@then("a success state offers a list link and the new listing appears on the seller list")
def created_listed(world: World) -> None:
    page = world.page
    expect(studio(world).success_result).to_be_visible(timeout=timeouts.NAVIGATION)
    link = page.get_by_role("link", name="Xem danh sách")
    expect(link).to_have_attribute("href", "/seller")
    link.click()
    page.wait_for_url(re.compile(r".*/seller$"), timeout=timeouts.NAVIGATION)
    expect(work(world).product_row(world.state.extra["d2_new_title"]).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the d2 seller opens the edit page of the first listing")
def open_edit(world: World) -> None:
    item = seller(world)["listings"][0]
    go(world, f"/seller/{item['id']}/edit")
    expect(studio(world).title_input).to_be_visible(timeout=timeouts.NAVIGATION)
    studio(world).wait_until_interactive(world.page.get_by_role("button", name="Lưu thay đổi"))


@when("the d2 seller changes the title and saves")
def edit_and_save(world: World) -> None:
    page = studio(world)
    world.state.extra["d2_typed"] = f"[E2E][d2] edited {int(time.time())}"
    page.title_input.fill(world.state.extra["d2_typed"])
    world.page.get_by_role("button", name="Lưu thay đổi").click()


@then(
    "an alert at the top shows the message, an error toast appears, the typed values are kept and submit is enabled"
)
def server_error_surfaced(world: World) -> None:
    page = world.page
    expect(page.locator("form").get_by_role("alert").first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible()
    expect(studio(world).title_input).to_have_value(world.state.extra["d2_typed"])
    expect(page.get_by_role("button", name="Lưu thay đổi")).to_be_enabled()


@when("the d2 seller types a title, applies every AI suggestion and reads the fields")
def apply_all_and_read(world: World) -> None:
    page = studio(world)
    page.title_input.fill("Laptop Dell XPS 13")
    page.description_input.fill("Mô tả của tôi")
    expect(page.magic_generate).to_be_enabled(timeout=timeouts.DEFAULT)
    page.magic_generate.click()
    expect(page.magic_suggestion).to_be_visible(timeout=timeouts.LONG)
    page.magic_apply_all.click()
    world.state.extra["d2_applied"] = {
        "title": page.title_input.input_value(),
        "description": page.description_input.input_value(),
        "price": page.price_input.input_value(),
    }


@then("the title, description and price fields show the suggestion and submitting sends them")
def applied_and_sent(world: World) -> None:
    from src.api.services import ListingService

    page = studio(world)
    applied = world.state.extra["d2_applied"]
    assert applied["description"] != "Mô tả của tôi", applied
    assert applied["price"], applied
    page.category_select.select_option("cat-electronics")
    page.stock_input.fill("3")
    page.submit_button.click()
    expect(page.success_result).to_be_visible(timeout=timeouts.NAVIGATION)
    href = world.page.get_by_role("link", name="Xem sản phẩm").get_attribute("href")
    listing = ListingService(token=seller(world)["token"]).get_listing(href.rsplit("/", 1)[-1])
    assert listing["title"] == applied["title"], (listing["title"], applied)

    def norm(text: str) -> str:
        return text.replace("\r\n", "\n").strip()

    # form posts normalise line endings; the content is the suggestion's
    assert norm(listing["description"]) == norm(applied["description"]), (listing, applied)
    assert int(listing["price"]) == int(applied["price"]), (listing["price"], applied)


# ── Seller orders ────────────────────────────────────────────────────────
@when("the d2 seller opens the detail of their order")
def open_order_detail(world: World) -> None:
    go(world, f"/seller/orders/{world.state.extra['d2_order_id']}")
    expect(sdetail(world).packing_slip).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the d2 seller confirms the handover while the server is slow")
def handover_slow(world: World) -> None:
    page = sdetail(world)
    page.wait_until_interactive(page.ship_button)
    page.ship_button.click()
    expect(page.confirm_dialog).to_be_visible(timeout=timeouts.DEFAULT)
    track_actions(world)
    slow_actions(world, 1.5, "**/seller/orders/**")
    page.confirm_ship.click()


@then(
    "the confirm button is pending then disabled, a success toast appears and the tag and stepper show shipped"
)
def handover_feedback(world: World) -> None:
    page = sdetail(world)
    expect(page.confirm_ship).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(page.confirm_ship).to_be_disabled()
    expect(page.shipped_toast).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.current_step).to_contain_text("Đang giao", timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text("Đang giao hàng").first).to_be_visible()
    assert len(world.state.extra["d2_posts"]) == 1


@when("the order is shipped from another session")
def shipped_elsewhere(world: World) -> None:
    from src.api.services import OrderService

    OrderService(token=seller(world)["token"]).update_order_status(
        world.state.extra["d2_order_id"], "ORDER_STATUS_SHIPPED"
    )


@when("the d2 seller confirms the handover")
def handover(world: World) -> None:
    page = sdetail(world)
    page.wait_until_interactive(page.ship_button)
    page.ship_button.click()
    expect(page.confirm_dialog).to_be_visible(timeout=timeouts.DEFAULT)
    page.confirm_ship.click()


@then("an error toast and an alert show the error and the order status is unchanged")
def handover_failed(world: World) -> None:
    page = world.page
    expect(sdetail(world).confirm_dialog.get_by_role("alert")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(
        page.get_by_role("alert")
        .filter(has_text=re.compile(r"\S{4,}"))
        .filter(has_not_text="Đóng")
        .first
    ).to_be_visible()
    expect(sdetail(world).shipped_toast).to_have_count(0)
    from src.api.services import OrderService

    order = OrderService(token=seller(world)["token"]).get_order(world.state.extra["d2_order_id"])
    assert order["order"]["status"] == "ORDER_STATUS_SHIPPED"


@then(
    "at most one brand-filled primary button is visible in the page header and the other actions are outline or ghost"
)
def one_primary(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.NAVIGATION)
    primary = page.evaluate("""() => [...document.querySelectorAll('main a, main button')]
            .filter(e => e.classList.contains('bg-action-primary') && e.offsetParent !== null)
            .map(e => e.innerText.trim())""")
    assert len(primary) <= 1, f"more than one brand-filled primary action: {primary}"


# ── Wallet, plans, bundles ───────────────────────────────────────────────
@given("a d2 seller has a settled order and a positive wallet balance")
def seller_with_balance(world: World) -> None:
    from src.api.services import PaymentService
    from tests.e2e.support import plp_stack as stack

    window = stack.require_hold_wait()  # fails fast unless the short-window overlay is active
    s = d2.seed_seller_session(world, listings=1)
    order_id = d2.seller_order(world, s)
    PaymentService(token=world.state.extra["d2_buyer_token"]).mock_pay(order_id, 120_000, True)
    pay = PaymentService(token=s["token"])
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        if int(pay.wallet_balance() or 0) > 0:
            break
        time.sleep(0.5)
    else:
        raise AssertionError("the paid order never credited the seller's wallet")
    # proceeds inside the refund window cannot be paid out: wait the window out
    time.sleep(window + 5)


@when("the d2 seller opens the wallet")
def open_wallet(world: World) -> None:
    go(world, "/seller/wallet")
    expect(world.page.get_by_text("Số dư khả dụng", exact=True).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the d2 seller activates payout and confirms while the server is slow")
def payout_slow(world: World) -> None:
    page = world.page
    button = page.get_by_role("button", name="Rút tiền", exact=True)
    expect(button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    world.state.extra["d2_balance_before"] = page.locator("main").inner_text()
    button.click()
    expect(page.get_by_role("dialog")).to_be_visible(timeout=timeouts.DEFAULT)
    slow_actions(world, 1.5, "**/seller/wallet**")
    page.get_by_role("dialog").get_by_role("button", name="Xác nhận rút tiền").click()


@then(
    "the confirm button is pending then disabled, a success toast appears and the balance and ledger refresh"
)
def payout_feedback(world: World) -> None:
    page = world.page
    confirm = page.get_by_role("dialog").get_by_role("button", name="Xác nhận rút tiền")
    expect(confirm).to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(confirm).to_be_disabled()
    expect(toast(page, "Đã tạo lệnh rút tiền")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.DEFAULT)
    expect(page.get_by_role("button", name="Rút tiền", exact=True)).to_be_disabled(
        timeout=timeouts.NAVIGATION
    )
    assert world.state.extra["d2_balance_before"] != page.locator("main").inner_text()


@when("the d2 seller opens the plans page")
def open_plans(world: World) -> None:
    go(world, "/seller/plans")
    expect(world.page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(
    'the current plan shows a "Gói hiện tại" tag and a disabled button and the other plans an enabled subscribe button'
)
def plans_state(world: World) -> None:
    page = world.page
    cards = page.locator("main .grid > div")
    expect(cards.first).to_be_visible(timeout=timeouts.NAVIGATION)
    assert cards.count() >= 2, "fewer than two plans seeded"
    current = cards.filter(has=page.locator("span", has_text="Gói hiện tại"))
    expect(current).to_have_count(1)
    current_button = current.get_by_role("button")
    expect(current_button).to_be_disabled()
    others = cards.filter(has_not=page.locator("span", has_text="Gói hiện tại"))
    for i in range(others.count()):
        expect(others.nth(i).get_by_role("button", name="Đăng ký")).to_be_enabled()


@when("the d2 seller opens the bundles page")
def open_bundles(world: World) -> None:
    go(world, "/seller/bundles")
    expect(world.page.get_by_text("Tạo combo mới")).to_be_visible(timeout=timeouts.NAVIGATION)
    world.page.wait_for_timeout(500)


@when(
    parsers.parse(
        'the d2 seller submits a bundle "{title}" priced {price:d} with {count:d} selected products'
    )
)
def submit_bundle(world: World, title: str, price: int, count: int) -> None:
    page = world.page
    world.state.extra["d2_bundle_title"] = title
    page.get_by_label("Tên combo").fill(title)
    page.get_by_label("Giá combo (₫)").fill(str(price))
    for item in seller(world)["listings"][:count]:
        page.get_by_label(item["title"]).check()
    page.get_by_role("button", name="Tạo combo").click()


@then("the message shows in the form and an error toast appears")
def bundle_error(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Chọn ít nhất 2 sản phẩm cho combo.").first).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    expect(
        page.get_by_role("alert").filter(has_text="Chọn ít nhất 2 sản phẩm cho combo.")
    ).to_be_visible()


@then("a success toast appears, the form resets and the bundle is listed")
def bundle_created(world: World) -> None:
    page = world.page
    expect(toast(page, "Đã tạo combo")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_label("Tên combo")).to_have_value("", timeout=timeouts.DEFAULT)
    expect(
        page.get_by_role("table", name="Combo hiện có").get_by_text(
            world.state.extra["d2_bundle_title"]
        )
    ).to_be_visible(timeout=timeouts.NAVIGATION)


# ── Layout shift, images, tracking ───────────────────────────────────────
@when("the d2 seller opens the workplace from the wallet page while the data is slow")
def workplace_slow_soft(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import install_cls

    page = world.page
    install_cls(world)
    go(world, "/seller/wallet")
    page.wait_for_load_state("networkidle")

    def slow(route) -> None:  # noqa: ANN001
        time.sleep(1.5)
        route.continue_()

    page.route(re.compile(r".*/seller\?_rsc=.*"), slow)
    page.evaluate("window.__cls = 0")
    page.get_by_role("navigation", name="Kênh người bán").locator('a[href="/seller"]').first.click()


@then(
    "a skeleton with the KPI row and table rows shows first and the page then replaces it without a layout shift"
)
def seller_skeleton(world: World) -> None:
    from tests.e2e.step_definitions.d2_quality_steps import cls

    page = world.page
    expect(page.get_by_test_id("seller-skeleton")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(work(world).kpi_row).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_for_timeout(1000)
    soft = cls(page)
    page.goto(f"{base(world)}/seller", wait_until="load")
    expect(work(world).kpi_row).to_be_visible(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(1500)
    hard = cls(page)
    assert soft == 0 and hard == 0, f"cumulative layout shift: soft {soft}, hard {hard}"


@when("the d2 seller opens the workplace with the thumbnails loading slowly")
def workplace_slow_images(world: World) -> None:
    page = world.page
    page.set_viewport_size(DESKTOP)

    def slow(route) -> None:  # noqa: ANN001
        time.sleep(1.5)
        route.continue_()

    page.route(re.compile(r".*/listing-images/.*"), slow)
    page.goto(f"{base(world)}/seller", wait_until="domcontentloaded")
    expect(page.locator("tbody tr").first).to_be_visible(timeout=timeouts.NAVIGATION)
    world.state.extra["d2_rows_before"] = page.locator("tbody tr").evaluate_all(
        "rows => rows.map(r => Math.round(r.getBoundingClientRect().height))"
    )


@then(
    "each thumbnail box is already 1:1 and the row height does not change when the picture fails or loads"
)
def seller_thumbs(world: World) -> None:
    page = world.page
    boxes = page.locator("tbody tr .w-12").evaluate_all(
        "els => els.map(e => [Math.round(e.getBoundingClientRect().width), Math.round(e.getBoundingClientRect().height)])"
    )
    assert boxes and all(w == h for w, h in boxes), boxes
    page.wait_for_load_state("networkidle")
    after = page.locator("tbody tr").evaluate_all(
        "rows => rows.map(r => Math.round(r.getBoundingClientRect().height))"
    )
    assert after == world.state.extra["d2_rows_before"], (
        world.state.extra["d2_rows_before"],
        after,
    )


@then("the thumbnails after the first screen are lazy")
def seller_lazy(world: World) -> None:
    from tests.e2e.step_definitions.d2_orders_steps import server_html

    html = server_html(world, "/seller")
    titles = {item["title"] for item in seller(world)["listings"]}
    loading = []
    for tag in re.findall(r"<img[^>]*>", html):
        alt = re.search(r'alt="([^"]*)"', tag)
        if alt and alt.group(1) in titles:
            loading.append((re.search(r'loading="(\w+)"', tag) or [None, ""])[1])
    assert len(loading) == 20
    assert set(loading[:5]) == {"eager"} and set(loading[5:]) == {"lazy"}, loading


@given("a d2 seller has a listing and a buyer views it in the browser")
def seller_listing_buyer_views(world: World) -> None:
    s = d2.seed_seller_session(world, listings=1)
    world.state.extra["d2_listing_id"] = s["listings"][0]["id"]
    # the buyer's browser session (the seller's cookie is replaced)
    d2.seed_buyer(world)


@when("the buyer opens the listing page and adds the product to the cart")
def buyer_views_and_adds(world: World) -> None:
    page = world.page
    page.goto(
        f"{base(world)}/listing/{world.state.extra['d2_listing_id']}", wait_until="domcontentloaded"
    )
    page.wait_for_load_state("networkidle")
    add = page.get_by_role("button", name=re.compile("Thêm vào giỏ")).first
    expect(add).to_be_enabled(timeout=timeouts.DEFAULT)
    page.wait_for_timeout(500)
    add.click()
    expect(page.get_by_role("status").filter(has_text=re.compile("giỏ", re.I)).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then(
    "the page emitted view and add to cart events for that listing and the seller funnel counts the view"
)
def events_and_funnel(world: World) -> None:
    page = world.page
    listing_id = world.state.extra["d2_listing_id"]
    events = page.evaluate(
        "() => (window.dataLayer || []).map(e => ({event: e.event, blob: JSON.stringify(e)}))"
    )
    names = {e["event"] for e in events if listing_id in e["blob"]}
    assert "view_item" in names, names
    assert "add_to_cart" in names, names
    s = seller(world)
    world.service_factory.set_token(s["token"])
    deadline = time.time() + 90
    views = 0
    while time.time() < deadline:
        resp = world.service_factory.analytics.funnel_response(s["seller_id"])
        assert resp.status_code == 200, resp.text
        views = int(resp.json().get("views", 0))
        if views >= 1:
            break
        time.sleep(2)
    assert views >= 1, "the seller funnel never counted the buyer's view"
