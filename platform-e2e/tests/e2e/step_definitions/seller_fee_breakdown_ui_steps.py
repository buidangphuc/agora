"""UI steps for seller-fee-breakdown-ui (initiative monetization).

Setup goes through the gateway (the `plp` world of psc_steps: real shop, voucher, buyer, online
payment and refund flows); the browser is signed in by injecting the actor's session cookie.
The card is located by its title; its rows by the amounts they carry, since the spec fixes the
title, the amounts, the rate and the two messages but no row label.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator, expect
from pytest_bdd import parsers, then, when

from src.constants import PageName, timeouts
from src.pages import SellerOrderDetailPage, SellerWalletPage
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.step_definitions.psc_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.psc_steps import _facts, _principal
from tests.e2e.support.apps import app_url
from tests.e2e.support.world import World

CARD_TITLE = "Đối soát thanh toán"
NOT_SETTLED = "Đơn chưa được đối soát vào ví"
AMOUNT = re.compile(r"₫\s*([\d.]+)")


def _sign_in(world: World, actor) -> None:
    world.context.clear_cookies()
    world.context.add_cookies(session_cookies(world.settings, actor.token))
    world.service_factory.set_token(actor.token)


def _order_id(plp) -> str:
    return plp.orders["order"]


def _open_order(world: World, plp) -> SellerOrderDetailPage:
    page = world.navigate_to(PageName.SELLER_ORDER_DETAIL, order_id=_order_id(plp))
    assert isinstance(page, SellerOrderDetailPage)
    return page


def _card(world: World, marker: str = "₫") -> Locator:
    title = f"//*[normalize-space(text())='{CARD_TITLE}']"
    return world.page.locator(f"xpath={title}/ancestor::*[contains(., '{marker}')][1]").first


def _amounts(text: str) -> list[int]:
    return sorted(int(m.replace(".", "")) for m in AMOUNT.findall(text))


@when("the seller opens the order page in the seller app")
def seller_opens_order(world: World, plp) -> None:
    _sign_in(world, _principal(plp, "the seller"))
    _open_order(world, plp)


@when("another seller opens the order page in the seller app")
def other_seller_opens_order(world: World, plp) -> None:
    _sign_in(world, _principal(plp, "another seller"))
    _open_order(world, plp)


@when("a visitor without a session opens the order page in the seller app")
def guest_opens_order(world: World, plp) -> None:
    world.context.clear_cookies()
    world.page.goto(
        app_url(world.settings, f"/seller/orders/{_order_id(plp)}"), wait_until="domcontentloaded"
    )


@when("the seller opens the wallet in the seller app")
def seller_opens_wallet(world: World, plp) -> None:
    _sign_in(world, _principal(plp, "the seller"))
    page = world.navigate_to(PageName.SELLER_WALLET)
    assert isinstance(page, SellerWalletPage)
    expect(page.balance_label).to_be_visible(timeout=timeouts.NAVIGATION)


@then(
    parsers.parse(
        "the settlement card lists the credit, commission {commission:d} labelled "
        '"{rate}", returned {reversed_:d}, deducted {deducted:d} and the net'
    )
)
def card_lists_breakdown(world: World, plp, commission, rate, reversed_, deducted) -> None:
    credit = _facts(plp)["credit"]
    net = credit - commission + reversed_ - deducted
    card = _card(world)
    expect(card).to_contain_text(rate, timeout=timeouts.NAVIGATION)
    expect(card).to_contain_text(f"{net:,}".replace(",", "."), timeout=timeouts.DEFAULT)
    assert _amounts(card.inner_text()) == sorted(
        [credit, commission, reversed_, deducted, net]
    ), card.inner_text()


@then(f'the settlement card says "{NOT_SETTLED}" and shows no amount')
def card_not_settled(world: World) -> None:
    card = _card(world, NOT_SETTLED)
    expect(card).to_contain_text(NOT_SETTLED, timeout=timeouts.NAVIGATION)
    assert "₫" not in card.inner_text(), card.inner_text()


@then("the order page still renders its other content")
def order_page_renders(world: World) -> None:
    expect(SellerOrderDetailPage(world.page).packing_slip).to_be_visible(timeout=timeouts.DEFAULT)


@then("the not-found page is shown and no settlement card or amount appears")
def not_found_no_card(world: World) -> None:
    expect(SellerOrderDetailPage(world.page).not_found).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(CARD_TITLE)).to_have_count(0)
    assert "₫" not in world.page.locator("body").inner_text()


@then("the visitor is redirected to login and the page holds no settlement card or amount")
def guest_redirected(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r"/login"), timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_text(CARD_TITLE)).to_have_count(0)
    assert "₫" not in world.page.locator("body").inner_text()


@then(
    parsers.parse(
        'the ledger shows a "{settled}" row and a "{label}" row of "{amount}" '
        'and no cell reads "COMMISSION"'
    )
)
def ledger_labelled(world: World, settled, label, amount) -> None:
    rows = world.page.get_by_role("row")
    expect(rows.filter(has_text=settled).first).to_be_visible(timeout=timeouts.NAVIGATION)
    sign, digits = amount[0], amount[2:]
    pattern = re.compile(re.escape(sign) + r"\s*₫\s*" + re.escape(digits))
    expect(rows.filter(has_text=label).filter(has_text=pattern).first).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    assert "COMMISSION" not in " ".join(rows.all_inner_texts())
