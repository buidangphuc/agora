"""Steps of frontend/buyer_sponsored_slots.feature (change buyer-sponsored-slots)."""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.flows.oic_inv_flow import poll
from tests.e2e.support import buyer_sponsored_slots_support as sup
from tests.e2e.support.advertised_price_ui_support import sign_in_browser
from tests.e2e.support.world import World

_SECTION_FIRST_BEFORE = """(args) => {
  const [slotSel, scopeSel] = args;
  const scope = scopeSel ? document.querySelector(scopeSel) : document;
  const slot = scope.querySelector(slotSel);
  const organic = [...scope.querySelectorAll('a[href^="/listing/"]')]
    .filter((a) => !a.closest('[data-testid="sponsored-slot"]'));
  if (!slot) return 'no-slot';
  if (organic.length === 0) return 'no-organic';
  return (slot.compareDocumentPosition(organic[0]) & Node.DOCUMENT_POSITION_FOLLOWING) ? 'ok' : 'after';
}"""


@given(
    "a sponsored-slot seller with a sponsored listing and an organic listing in a fresh category"
)
def seller(world: World) -> None:
    sup.seed(world)


@given(
    parsers.parse(
        "the seller has an ACTIVE sponsored-slot campaign with bid {bid:d} and budget {budget:d}"
    )
)
def campaign(world: World, bid: int, budget: int) -> None:
    sup.create_campaign(world, bid, budget)


@given("both listings are searchable in that category")
def searchable(world: World) -> None:
    sup.wait_searchable(world)


def _track_beacons(world: World) -> None:
    st = sup.state(world)
    st["beacons"] = []
    world.page.on(
        "request",
        lambda req: st["beacons"].append(req.url) if sup.CLICK_PATH in req.url else None,
    )


def _open_search(world: World, token: str | None) -> None:
    st = sup.state(world)
    sign_in_browser(world, token)
    _track_beacons(world)
    url = f"{world.settings.base_url}/search?category={st['category']}"
    world.page.goto(url, wait_until="domcontentloaded")
    st["results_url"] = url


@when("a guest opens the search page of that category")
def guest_search(world: World) -> None:
    _open_search(world, None)


@when("the owning seller opens the search page of that category")
def seller_search(world: World) -> None:
    _open_search(world, sup.state(world)["seller"])


@when("a guest opens the home page")
def guest_home(world: World) -> None:
    sign_in_browser(world, None)
    _track_beacons(world)
    world.page.goto(f"{world.settings.base_url}/", wait_until="domcontentloaded")


def _slot(world: World):
    return world.page.get_by_test_id("sponsored-slot").first


def _click_slot(world: World) -> None:
    link = _slot(world).locator('a[href^="/listing/"]').first
    expect(link).to_be_visible(timeout=timeouts.DEFAULT)
    link.click()


@when("the visitor clicks the sponsored slot")
def click_slot(world: World) -> None:
    _click_slot(world)


@when("the visitor returns to the results page without reloading and clicks the same slot again")
def click_again(world: World) -> None:
    world.page.go_back(wait_until="domcontentloaded")
    _click_slot(world)


@then(
    parsers.parse(
        'the sponsored listing is shown in a "{testid}" labelled "{label}" before the first organic card'
    )
)
def slot_before_organic(world: World, testid: str, label: str) -> None:
    st = sup.state(world)
    slot = world.page.get_by_test_id(testid).first
    expect(slot).to_be_visible(timeout=timeouts.DEFAULT)
    expect(slot).to_contain_text(label)
    expect(slot.locator(f'a[href*="{st["sponsored"]}"]').first).to_be_visible()
    order = world.page.evaluate(_SECTION_FIRST_BEFORE, ['[data-testid="sponsored-slot"]', None])
    assert order == "ok", f"sponsored slot is not before the first organic card: {order}"


@then(
    parsers.parse(
        'the feed section "{heading}" has a "{testid}" labelled "{label}" before its first organic card'
    )
)
def home_slot(world: World, heading: str, testid: str, label: str) -> None:
    section = world.page.locator("section").filter(has_text=heading).first
    expect(section).to_be_visible(timeout=timeouts.DEFAULT)
    slot = section.get_by_test_id(testid).first
    expect(slot).to_be_visible(timeout=timeouts.DEFAULT)
    expect(slot).to_contain_text(label)
    order = world.page.evaluate(
        """(heading) => {
  const section = [...document.querySelectorAll('section')].find((s) => s.textContent.includes(heading));
  const slot = section.querySelector('[data-testid="sponsored-slot"]');
  const organic = [...section.querySelectorAll('a[href^="/listing/"]')]
    .filter((a) => !a.closest('[data-testid="sponsored-slot"]'));
  if (organic.length === 0) return 'ok';
  return (slot.compareDocumentPosition(organic[0]) & Node.DOCUMENT_POSITION_FOLLOWING) ? 'ok' : 'after';
}""",
        heading,
    )
    assert order == "ok", f"sponsored slot is after the first organic card: {order}"


@then(
    parsers.parse(
        'the sponsored listing appears once on the page, inside "{inside}" and not inside "{outside}"'
    )
)
def deduped(world: World, inside: str, outside: str) -> None:
    st = sup.state(world)
    expect(world.page.get_by_test_id(inside).first).to_be_visible(timeout=timeouts.DEFAULT)
    anchor = f'a[href*="{st["sponsored"]}"]'
    assert world.page.locator(anchor).count() == 1, "listing is not on the page exactly once"
    assert world.page.get_by_test_id(inside).locator(anchor).count() == 1
    assert world.page.get_by_test_id(outside).locator(anchor).count() == 0


@then("the browser lands on the sponsored listing page")
def landed(world: World) -> None:
    listing = sup.state(world)["sponsored"]
    world.page.wait_for_url(re.compile(rf"/listing/{re.escape(listing)}"), timeout=timeouts.DEFAULT)


@then(
    parsers.parse("the sponsored-slot campaign reads spent {amount:d} once the click is recorded")
)
def spent_after_click(world: World, amount: int) -> None:
    last = {}

    def reached() -> bool:
        last["spent"] = sup.spent(world)
        return last["spent"] == amount

    assert poll(reached, timeout_s=15), f"spent never reached {amount}: {last}"


@then(parsers.parse("the sponsored-slot campaign still reads spent {amount:d}"))
def spent_still(world: World, amount: int) -> None:
    world.page.wait_for_load_state("load")
    assert sup.spent(world) == amount


@then("the browser sent no click beacon")
def no_beacon(world: World) -> None:
    assert sup.state(world)["beacons"] == []
