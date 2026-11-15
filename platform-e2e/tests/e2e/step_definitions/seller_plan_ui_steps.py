"""Steps for seller-plan-ui: the /seller/plans page and the listing-cap upgrade link.

Money and sellers are seeded through the gateway (plp steps); the browser is logged in as that
seller and everything asserted is what the page shows plus the persisted ledger/subscription.
"""

from __future__ import annotations

import re
from datetime import datetime, timedelta, timezone

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.constants.pages import PageName
from src.models import Listing
from src.pages import SellerNewListingPage
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.step_definitions.listing_plan_cap_steps import _my_listing_ids
from tests.e2e.step_definitions.plp_steps import _seller
from tests.e2e.support import d2_support as d2
from tests.e2e.support import plp_support as p
from tests.e2e.support.apps import app_url
from tests.e2e.support.world import World

CAP_MESSAGE = "Bạn đã đạt giới hạn số tin đăng của gói hiện tại."
INSUFFICIENT = "Số dư có thể rút không đủ để trả gói này."
_DATE = re.compile(r"Hết hạn ngày (\d{2})/(\d{2})/(\d{4})")


def _open(world: World, path: str) -> None:
    world.page.set_viewport_size({"width": 1280, "height": 800})
    world.page.goto(app_url(world.settings, path))


def _cards(world: World):
    return world.page.locator("main .grid > div.grid")


_TIER_HEADINGS = {"FREE": "Miễn phí", "PRO": "Pro", "PREMIUM": "Premium"}


def _card(world: World, name: str):
    heading = world.page.get_by_role("heading", name=_TIER_HEADINGS.get(name, name), exact=True)
    return _cards(world).filter(has=heading)


def _confirm(world: World):
    dialog = world.page.get_by_role("dialog")
    expect(dialog).to_be_visible(timeout=timeouts.DEFAULT)
    return dialog.get_by_role("button", name=re.compile(r"^Xác nhận"))


@given("the plan-ui browser is logged in as the seller")
def browser_as_plp_seller(world: World, plp) -> None:
    world.context.add_cookies(session_cookies(world.settings, _seller(plp).token))


@when("the plan-ui seller opens the plans page")
@when("the plan-ui seller reloads the plans page")
def open_plans(world: World) -> None:
    _open(world, "/seller/plans")
    expect(world.page.get_by_role("heading", level=1)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_cards(world).first).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the plan-ui seller upgrades to PRO and confirms")
def upgrade_to_pro(world: World) -> None:
    _card(world, "PRO").get_by_role("button", name="Nâng cấp").click()
    dialog = world.page.get_by_role("dialog")
    expect(dialog).to_contain_text("₫199.000", timeout=timeouts.DEFAULT)
    _confirm(world).click()


@when("the plan-ui seller cancels renewal and confirms")
def cancel_renewal(world: World) -> None:
    world.page.get_by_role("button", name="Hủy gia hạn").click()
    _confirm(world).click()
    expect(world.page.get_by_role("status").filter(has_text="Đã hủy gia hạn")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("the plan-ui insufficient balance message links to the wallet and an error toast shows")
def insufficient(world: World) -> None:
    page = world.page
    expect(page.get_by_text(INSUFFICIENT).first).to_be_visible(timeout=timeouts.NAVIGATION)
    link = page.get_by_role("link", name="Xem ví")
    expect(link).to_have_attribute("href", "/seller/wallet")
    expect(
        page.get_by_role("alert")
        .filter(has_text="Đăng ký gói")
        .or_(page.get_by_role("status").filter(has_text="Đăng ký gói"))
        .first
    ).to_be_visible(timeout=timeouts.DEFAULT)


@then("the plan-ui FREE card is the current plan")
def free_is_current(world: World) -> None:
    current = _cards(world).filter(has=world.page.get_by_text("Gói hiện tại", exact=True))
    expect(current).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(current).to_contain_text("Miễn phí")
    expect(world.page.get_by_text("Hết hạn ngày")).to_have_count(0)


def _period_end(world: World) -> datetime:
    text = _card(world, "PRO").inner_text()
    m = _DATE.search(text)
    assert m, f"no 'Hết hạn ngày dd/MM/yyyy' on the PRO card: {text!r}"
    return datetime(int(m[3]), int(m[2]), int(m[1]), tzinfo=timezone.utc)


@then("the plan-ui PRO card is the current plan with a period end about 30 days ahead")
def pro_current(world: World) -> None:
    pro = _card(world, "PRO")
    expect(pro.get_by_text("Gói hiện tại", exact=True)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(pro.get_by_role("button").first).to_be_disabled()
    expect(pro).to_contain_text("Tự động gia hạn: Bật")
    delta = _period_end(world) - datetime.now(timezone.utc)
    assert timedelta(days=28) < delta < timedelta(days=31), delta


@then("the plan-ui PRO card is the current plan with renewal off and a lapse date")
def pro_renewal_off(world: World) -> None:
    pro = _card(world, "PRO")
    expect(pro.get_by_text("Gói hiện tại", exact=True)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(pro).to_contain_text("Tự động gia hạn: Tắt")
    expect(pro).to_contain_text("Sẽ chuyển về Miễn phí ngày")
    expect(pro.get_by_role("button", name="Hủy gia hạn")).to_have_count(0)


@then("the plan-ui wallet has no refund entry")
def no_refund(plp) -> None:
    refunds = [e for e in p.ledger(plp, _seller(plp)) if "REFUND" in e.get("type", "")]
    assert not refunds, refunds


@given(parsers.parse("a plan-ui FREE seller owns {count:d} published listings"))
def free_seller_with_listings(world: World, count: int) -> None:
    d2.seed_seller_session(world, listings=count)


@when("the plan-ui seller submits a new listing")
def submit_new_listing(world: World) -> None:
    title = f"[E2E][plan-ui] typed title {datetime.now().strftime('%H%M%S%f')}"
    world.state.extra["spu_title"] = title
    page: SellerNewListingPage = world.navigate_to(PageName.SELLER_NEW_LISTING)  # type: ignore[assignment]
    expect(page.title_input).to_be_visible(timeout=timeouts.NAVIGATION)
    page.fill_and_submit(Listing(title=title, category_id="cat-laptop", price=1_000_000, stock=5))


@then("the plan-ui form shows the cap message and an upgrade link to the plans page")
def cap_message(world: World) -> None:
    page = world.page
    expect(page.get_by_text(CAP_MESSAGE).first).to_be_visible(timeout=timeouts.NAVIGATION)
    link = page.get_by_role("link", name="Nâng cấp gói")
    expect(link).to_have_attribute("href", "/seller/plans")
    expect(page.locator("#title")).to_have_value(world.state.extra["spu_title"])
    link.click()
    expect(page).to_have_url(re.compile(r"/seller/plans$"), timeout=timeouts.NAVIGATION)


@then(parsers.parse("the plan-ui seller owns {count:d} listings"))
def owns_count(world: World, count: int) -> None:
    token = world.state.extra["d2_seller"]["token"]
    assert len(_my_listing_ids(token)) == count


@then("the plan-ui listing is created and no upgrade link is shown")
def created_no_link(world: World) -> None:
    page = world.page
    expect(SellerNewListingPage(page).success_result).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_role("link", name="Nâng cấp gói")).to_have_count(0)
