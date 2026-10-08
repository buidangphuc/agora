"""UI steps for payment-refund-model / seller-return-refund-ui (area prm-rma).

Setup goes through the gateway (the `prm` world of prm_rma_steps, real distinct users); the
browser is signed in by injecting the actor's session cookie. Every click on a client control
waits for hydration first (`BasePage.wait_until_interactive`), never a sleep.
"""

from __future__ import annotations

import re
import time

from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from src.constants import PageName, timeouts
from src.pages import SellerOrderReturnsPage
from tests.e2e.flows.auth_flow import SESSION_COOKIE
from tests.e2e.step_definitions.prm_rma_steps import (  # noqa: F401
    _order,
    _rid,
    _seller,
    any_step,
    prm,
)
from tests.e2e.support import plp_support as p
from tests.e2e.support.world import World

_ACTIONS = {"Duyệt": "approve", "Từ chối": "reject", "Hoàn tiền": "refund"}
_SETTLE_S = p.SETTLE_WAIT_S


def _norm(text: str) -> str:
    """Whitespace-free text, so `200.000 ₫` (nbsp) and `200.000₫` compare equal."""
    return re.sub(r"\s+", "", text or "")


def _sign_in(world: World, actor) -> None:
    world.context.clear_cookies()
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": actor.token, "url": world.settings.base_url}]
    )
    world.service_factory.set_token(actor.token)


def _returns_page(world: World) -> SellerOrderReturnsPage:
    page = world.get_page(PageName.SELLER_ORDER_RETURNS)
    assert isinstance(page, SellerOrderReturnsPage)
    return page


@any_step("the seller opens the returns tab of the order")
def seller_opens_returns(world: World, prm) -> None:  # noqa: F811
    _sign_in(world, _seller(prm))
    page = world.navigate_to(PageName.SELLER_ORDER_RETURNS, order_id=_order(prm))
    expect(page.section).to_be_visible(timeout=timeouts.NAVIGATION)


@any_step(parsers.parse('"{b}" opens the returns tab of the order'))
def buyer_opens_returns(world: World, prm, b) -> None:  # noqa: F811
    _sign_in(world, prm.actors[b])
    world.page.goto(
        f"{world.settings.base_url}/account/orders/{_order(prm)}?tab=returns",
        wait_until="domcontentloaded",
    )
    prm.data["buyer_page"] = True


@any_step(parsers.re(r"the seller reloads the page|the page is reloaded"))
def reload_page(world: World) -> None:
    world.page.reload(wait_until="domcontentloaded")


@when(parsers.parse('the seller clicks "{label}" on the return "{name}"'))
def click_action(world: World, prm, label, name) -> None:  # noqa: F811
    _returns_page(world).click_action(_rid(prm, name), _ACTIONS[label])


@when(parsers.parse('the seller clicks "Hoàn tiền" on the return "{name}" and confirms'))
def click_refund_and_confirm(world: World, prm, name) -> None:  # noqa: F811
    page = _returns_page(world)
    page.click_action(_rid(prm, name), "refund")
    page.confirm_refund()


@then(parsers.parse('the return "{name}" reads "{label}" and offers the actions "{actions}"'))
def reads_and_offers(world: World, prm, name, label, actions) -> None:  # noqa: F811
    page = _returns_page(world)
    rid = _rid(prm, name)
    expect(page.status(rid)).to_contain_text(label, timeout=timeouts.NAVIGATION)
    want = {_ACTIONS[a.strip()] for a in actions.split(",") if a.strip()}
    for action in _ACTIONS.values():
        button = page.action(rid, action)
        if action in want:
            expect(button).to_be_visible(timeout=timeouts.DEFAULT)
        else:
            expect(button).to_have_count(0, timeout=timeouts.DEFAULT)


@then(parsers.parse('the return "{name}" shows the message "{text}"'))
def shows_cod_message(world: World, prm, name, text) -> None:  # noqa: F811
    message = _returns_page(world).cod_message(_rid(prm, name))
    expect(message).to_be_visible(timeout=timeouts.DEFAULT)
    assert _norm(message.inner_text()) == _norm(text), message.inner_text()


@then("the page shows an error")
def page_shows_error(world: World) -> None:
    expect(_returns_page(world).error_message.first).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the return "{name}" shows the refund state "{text}"'))
def shows_refund_state(world: World, prm, name, text) -> None:  # noqa: F811
    state = _returns_page(world).refund_state(_rid(prm, name))
    expect(state).to_be_visible(timeout=timeouts.NAVIGATION)
    assert _norm(text) in _norm(state.inner_text()), state.inner_text()


@when(
    parsers.parse('the page is reloaded until the return "{name}" shows the refund state "{text}"')
)
def reload_until_state(world: World, prm, name, text) -> None:  # noqa: F811
    """Reload within the settle window until the per-return refund state reads `text`."""
    page = _returns_page(world)
    rid = _rid(prm, name)
    deadline = time.monotonic() + _SETTLE_S
    seen = ""
    while time.monotonic() < deadline:
        world.page.reload(wait_until="domcontentloaded")
        state = page.refund_state(rid)
        try:
            state.wait_for(state="visible", timeout=timeouts.SHORT)
            seen = state.inner_text()
        except Exception:  # noqa: BLE001 - not rendered yet: reload again
            seen = ""
        if _norm(text) in _norm(seen):
            return
    raise AssertionError(f"return {name} refund state is {seen!r}, want {text!r}")


@then("the payment summary says the payment could not be loaded")
def summary_unavailable(world: World) -> None:
    expect(_returns_page(world).payment_unavailable).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the payment summary reads "{refunded}" refunded of "{amount}"'))
def summary_reads(world: World, refunded, amount) -> None:
    summary = _returns_page(world).payment_summary
    expect(summary).to_be_visible(timeout=timeouts.DEFAULT)
    text = _norm(summary.inner_text())
    assert _norm(refunded) in text and _norm(amount) in text, summary.inner_text()


@then(parsers.parse('the return "{name}" reads "{label}" on the buyer\'s page'))
def buyer_reads(world: World, prm, name, label) -> None:  # noqa: F811
    statuses = world.get_page(PageName.ORDER_DETAIL).return_statuses
    expect(statuses.first).to_contain_text(label, timeout=timeouts.NAVIGATION)


@then(parsers.parse('the buyer\'s page has no "{label}" button'))
def buyer_no_refund(world: World, label) -> None:
    page = world.get_page(PageName.ORDER_DETAIL)
    expect(page.return_refund_buttons).to_have_count(0)
    expect(world.page.get_by_test_id("return-refund")).to_have_count(0)
