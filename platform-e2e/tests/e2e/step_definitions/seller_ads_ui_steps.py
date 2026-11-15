"""UI steps for seller-ads-ui (initiative monetization).

Campaigns are created, clicked and paused through the gateway (steps reused from the sponsored
ads and campaign management bindings); the browser is signed in by injecting the seller's session
cookie and reads /seller/ads.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator, expect
from pytest_bdd import parsers, then, when

from config.settings import get_settings
from src.constants import timeouts
from src.utils import data as fake
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.step_definitions.ad_campaign_management_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.sponsored_ads_steps import _x
from tests.e2e.support.apps import app_url
from tests.e2e.support.world import World

ADS_PATH = "/seller/ads"
EMPTY = "Chưa có chiến dịch nào"


def _sign_in(world: World, token: str) -> None:
    world.context.clear_cookies()
    world.context.add_cookies(session_cookies(world.settings, token))
    world.service_factory.set_token(token)


def _open(world: World) -> None:
    world.page.goto(app_url(world.settings, ADS_PATH), wait_until="domcontentloaded")


def _norm(text: str) -> str:
    return re.sub(r"₫\s+", "₫", text.replace("\xa0", " ").replace(" ", " ")).strip()


def _data_rows(world: World) -> Locator:
    return world.page.get_by_role("row").filter(has=world.page.get_by_role("cell"))


def _row(world: World) -> Locator:
    rows = _data_rows(world)
    expect(rows.first).to_be_visible(timeout=timeouts.NAVIGATION)
    return rows.first


def _cells(row: Locator) -> list[str]:
    return [_norm(t) for t in row.get_by_role("cell").all_inner_texts()]


def _numeric(cells: list[str]) -> list[int]:
    return [int(c) for c in cells if re.fullmatch(r"\d+", c)]


@when("the seller opens the ads page in the seller app")
def seller_opens(world: World) -> None:
    _sign_in(world, _x(world)["ad_seller"])
    _open(world)


@when("the second seller opens the ads page in the seller app")
def other_opens(world: World) -> None:
    _sign_in(world, _x(world)["ad_other"])
    _open(world)


@when("a visitor without a session opens the ads page in the seller app")
def guest_opens(world: World) -> None:
    world.context.clear_cookies()
    _open(world)


@when("a signed-in buyer opens the ads page in the seller app")
def buyer_opens(world: World) -> None:
    token = world.service_factory.auth.register(
        fake.unique_username("ads_ui_buyer"), get_settings().seed_password, "buyer"
    )
    _sign_in(world, token)
    _open(world)


@when("the seller reloads the ads page")
def seller_reloads(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@when(parsers.parse('the seller activates "{label}" on the campaign row'))
def activate(world: World, label: str) -> None:
    _row(world).get_by_role("button", name=label, exact=True).click()


@when(parsers.parse("the seller submits the campaign form with bid {bid:d} and budget {budget:d}"))
def submit_form(world: World, bid: int, budget: int) -> None:
    page = world.page
    listing = page.get_by_label(re.compile("tin đăng|sản phẩm", re.I)).first
    if listing.count():
        tag = listing.evaluate("e => e.tagName")
        if tag == "SELECT":
            listing.select_option(index=1 if listing.locator("option").count() > 1 else 0)
    page.get_by_label(re.compile("giá thầu", re.I)).first.fill(str(bid))
    page.get_by_label(re.compile("ngân sách", re.I)).first.fill(str(budget))
    page.get_by_role("button", name=re.compile("tạo chiến dịch|tạo", re.I)).first.click()


@then(parsers.parse("the ads table shows exactly {count:d} campaign row"))
def table_rows(world: World, count: int) -> None:
    expect(_data_rows(world)).to_have_count(count, timeout=timeouts.NAVIGATION)


@then(
    parsers.parse(
        'the campaign row shows "{bid}" bid, "{budget}" budget, "{spent}" spent, '
        '"{remaining}" remaining, {impressions:d} impressions, {clicks:d} clicks and "{status}"'
    )
)
def row_initial(world: World, bid, budget, spent, remaining, impressions, clicks, status) -> None:
    row = _row(world)
    expect(row).to_contain_text(status, timeout=timeouts.NAVIGATION)
    cells = _cells(row)
    for money in (bid, budget, spent, remaining):
        assert money in cells, (money, cells)
    assert _numeric(cells) == [impressions, clicks], cells


@then(
    parsers.parse(
        'the campaign row shows "{spent}" spent, "{remaining}" remaining, 1 click '
        "and at least 1 impression"
    )
)
def row_spent(world: World, spent: str, remaining: str) -> None:
    row = _row(world)
    expect(row).to_contain_text("₫49", timeout=timeouts.NAVIGATION)
    cells = _cells(row)
    assert cells.count(spent) == 2, cells
    assert remaining in cells, cells
    numbers = _numeric(cells)
    assert len(numbers) == 2 and min(numbers) == 1 and max(numbers) >= 1, cells


@then(parsers.parse('the campaign row shows the status "{status}" and the button "{button}"'))
def row_status(world: World, status: str, button: str) -> None:
    row = _row(world)
    expect(row.get_by_role("button", name=button, exact=True)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(row).to_contain_text(status)


@then(parsers.parse('the ads table shows "{text}" and no campaign row'))
def table_empty(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_data_rows(world)).to_have_count(0)


@then(parsers.parse('the visitor is redirected to login with "{path}" as the return path'))
def guest_redirected(world: World, path: str) -> None:
    encoded = re.escape(path).replace("/", r"(?:/|%2F)")
    expect(world.page).to_have_url(
        re.compile(rf"/login.*{encoded}", re.I), timeout=timeouts.NAVIGATION
    )


@then(parsers.parse('the page shows no campaign row and no "{amount}"'))
def no_campaign_data(world: World, amount: str) -> None:
    world.page.wait_for_load_state("domcontentloaded")
    expect(_data_rows(world)).to_have_count(0)
    assert amount not in _norm(world.page.locator("body").inner_text())
