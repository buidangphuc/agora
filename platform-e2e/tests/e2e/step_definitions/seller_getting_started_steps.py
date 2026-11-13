"""Steps for frontend/seller_getting_started.feature (seller-getting-started, initiative seller-centre).

Black box in the browser on the seller app. Accounts and fixtures go through the gateway; the
steps the seller completes in the browser use the real shop, new-listing and marketing pages.
"""

from __future__ import annotations

import re
import tempfile
from pathlib import Path

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages import SellerMarketingPage, SellerNewListingPage, SellerShopPage
from src.pages.seller_getting_started_card import (
    CARD_TITLE,
    DONE_LABEL,
    NOT_DONE_LABEL,
    STEPS,
    SellerGettingStartedPage,
)
from src.models import Listing
from tests.e2e.support import admin_campaigns_support as ac
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import seller_marketing_hub_support as sh
from tests.e2e.support.world import World

_PNG = (
    b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00"
    b"\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9a\xa0\xa0"
    b"\x00\x00\x00\x00IEND\xaeB`\x82"
)
DESKTOP = {"width": 1280, "height": 720}


def _centre(world: World) -> SellerGettingStartedPage:
    return SellerGettingStartedPage(world.page)


def _step(world: World, index: int):
    return _centre(world).step(STEPS[index - 1][0])


def _assert_state(world: World, index: int, done: bool) -> None:
    item = _step(world, index)
    expect(item).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(item).to_contain_text(DONE_LABEL if done else NOT_DONE_LABEL)
    if not done:
        expect(item).not_to_contain_text(DONE_LABEL)


@given("the seller owns a published listing with a scheduled flash sale and nothing else")
def seller_flash_sale_only(world: World) -> None:
    st = sh.state(world)
    listing_id, _ = sh.make_listing(world, st["token"], "gs")
    ac.seller_campaign(
        st["token"], listing_id, starts_in_s=86400 * 2, ends_in_s=86400 * 4
    )


@when("the seller opens the seller centre")
def open_centre(world: World) -> None:
    world.page.set_viewport_size(DESKTOP)
    page = _centre(world)
    page.goto(page.url())


@when(
    parsers.parse('the seller saves the shop name "{name}" and a banner on the shop page')
)
def save_shop(world: World, name: str) -> None:
    page = SellerShopPage(world.page)
    page.goto(page.url())
    expect(page.name_input).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_until_interactive(page.save_button)
    banner = Path(tempfile.mkdtemp()) / "banner.png"
    banner.write_bytes(_PNG)
    page.banner_file_input.set_input_files(str(banner))
    expect(page.banner_preview).to_be_visible(timeout=timeouts.NAVIGATION)
    page.name_input.fill(name)
    page.save_button.click()
    expect(page.saved_toast).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the seller creates a listing on the new-listing page")
def create_listing(world: World) -> None:
    page = SellerNewListingPage(world.page)
    page.goto(page.url())
    expect(page.title_input).to_be_visible(timeout=timeouts.NAVIGATION)
    page.fill_and_submit(
        Listing(
            title="[E2E] Getting started listing",
            category_id="cat-electronics",
            price=1_000_000,
            stock=5,
            description="Seed for seller-getting-started e2e.",
        )
    )


@when("the seller creates a shop voucher on the marketing hub")
def create_voucher(world: World) -> None:
    page = SellerMarketingPage(world.page)
    page.goto(page.url())
    code = av.fresh_code("GS")
    page.fill_voucher_form(code, 100, av.iso(days=1), av.iso(days=30))
    page.submit_voucher()
    expect(page.row(code)).to_be_visible(timeout=timeouts.NAVIGATION)


@then(
    parsers.parse(
        'the getting-started card shows exactly four steps, all not done, with links '
        '"{a}", "{b}", "{c}", "{d}"'
    )
)
def four_not_done(world: World, a: str, b: str, c: str, d: str) -> None:
    page = _centre(world)
    expect(page.title).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.card.get_by_role("listitem")).to_have_count(4)
    for i, href in enumerate((a, b, c, d), start=1):
        assert href == STEPS[i - 1][1], (href, STEPS[i - 1])
        _assert_state(world, i, done=False)
        expect(_step(world, i).get_by_role("link").first).to_have_attribute(
            "href", re.compile(f"{re.escape(href)}$")
        )


@then("no getting-started step mentions KYC")
def no_kyc(world: World) -> None:
    expect(_centre(world).card.get_by_text(re.compile("kyc|xác minh danh tính", re.I))).to_have_count(0)


@then("getting-started steps 1 and 2 are done and steps 3 and 4 are not done")
def s12_done(world: World) -> None:
    for i, done in ((1, True), (2, True), (3, False), (4, False)):
        _assert_state(world, i, done)


@then("getting-started steps 1, 2 and 3 are done and step 4 is not done")
def s123_done(world: World) -> None:
    for i, done in ((1, True), (2, True), (3, True), (4, False)):
        _assert_state(world, i, done)


@then("getting-started steps 3 and 4 are done and steps 1 and 2 are not done")
def s34_done(world: World) -> None:
    for i, done in ((1, False), (2, False), (3, True), (4, True)):
        _assert_state(world, i, done)


@then("the getting-started card is shown")
def card_shown(world: World) -> None:
    expect(_centre(world).title).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the getting-started card is absent")
def card_absent(world: World) -> None:
    expect(_centre(world).kpi_heading).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(_centre(world).title).to_have_count(0)


@then("the seller centre KPI row is present")
def kpi_present(world: World) -> None:
    expect(_centre(world).kpi_heading).to_be_visible()


@then(parsers.parse('the onboarding button "{label}" is shown'))
def onboarding_shown(world: World, label: str) -> None:
    expect(world.page.get_by_role("button", name=label)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@then("no getting-started card text is shown")
def no_card_text(world: World) -> None:
    expect(world.page.get_by_text(CARD_TITLE, exact=False)).to_have_count(0)
