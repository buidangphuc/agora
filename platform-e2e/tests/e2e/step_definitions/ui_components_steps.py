"""Steps for ui_components.feature: the dev-only /dev/ui component catalogue.

The catalogue needs no backend, so these run against a local `next dev`.
"""

from __future__ import annotations

import re

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.support.world import World

_FOCUS_RING = "rgb(255, 181, 160)"  # --color-focus-ring (primary-300)


def _tid(world: World, testid: str) -> Locator:
    return world.page.get_by_test_id(testid)


def _box(locator: Locator) -> dict[str, float]:
    box = locator.bounding_box()
    assert box is not None, "element has no layout box"
    return box


def _active_testid(world: World) -> str | None:
    return world.page.evaluate(
        "() => document.activeElement && document.activeElement.getAttribute('data-testid')"
    )


@given("the UI catalogue is open")
def open_catalogue(world: World) -> None:
    base = world.settings.base_url.rstrip("/")
    response = world.page.goto(f"{base}/dev/ui", wait_until="domcontentloaded")
    assert response is not None and response.status == 200, (
        "/dev/ui must be reachable: run the frontend with `next dev` "
        f"(got {response.status if response else 'no response'})"
    )
    expect(_tid(world, "ui-section-button")).to_be_visible(timeout=timeouts.DEFAULT)
    # Interactive demos are client islands: wait until React has hydrated them.
    world.page.wait_for_function(
        "() => document.querySelector('[data-testid=\"ui-click-count\"]') !== null"
    )
    world.page.wait_for_timeout(500)


# ---------------------------------------------------------------- Button states
@then("the loading button has the same width as the idle button")
def loading_same_width(world: World) -> None:
    idle = _box(_tid(world, "ui-button-idle"))["width"]
    loading = _box(_tid(world, "ui-button-loading"))["width"]
    assert abs(idle - loading) < 0.5, f"idle width {idle}px != loading width {loading}px"


@then("the loading button is busy and shows a spinner")
def loading_busy(world: World) -> None:
    button = _tid(world, "ui-button-loading")
    expect(button).to_have_attribute("aria-busy", "true")
    expect(button.locator(".animate-spin")).to_have_count(1)


@then("activating the loading button does not trigger its click handler")
def loading_inert(world: World) -> None:
    _activate_and_expect_no_click(world, "ui-probe-loading")


@then("the disabled button is disabled and announced as aria-disabled")
def disabled_announced(world: World) -> None:
    button = _tid(world, "ui-button-disabled")
    expect(button).to_be_disabled()
    expect(button).to_have_attribute("aria-disabled", "true")


@then("activating the disabled button does not trigger its click handler")
def disabled_inert(world: World) -> None:
    _activate_and_expect_no_click(world, "ui-probe-disabled")


def _activate_and_expect_no_click(world: World, testid: str) -> None:
    counter = _tid(world, "ui-click-count")
    before = counter.inner_text()
    target = _tid(world, testid)
    # Pointer: a programmatic click (pointer-events:none blocks real mouse hits).
    target.evaluate("el => el.click()")
    # Keyboard: Enter and Space on the element when it can take focus.
    target.evaluate("el => el.focus()")
    world.page.keyboard.press("Enter")
    world.page.keyboard.press("Space")
    world.page.wait_for_timeout(200)
    assert counter.inner_text() == before, f"{testid} fired its click handler"
    # Sanity: the enabled probe does count, so the counter is wired.
    _tid(world, "ui-probe-enabled").click()
    expect(counter).not_to_have_text(before)


# ---------------------------------------------------------------- Focus ring
def _ring(world: World) -> str:
    # Buttons transition their shadow (duration-150): wait for it to settle.
    world.page.wait_for_timeout(400)
    return _tid(world, "ui-button-primary").evaluate("el => getComputedStyle(el).boxShadow")


@when("the user tabs to the primary button")
def tab_to_primary(world: World) -> None:
    world.page.mouse.click(1, 1)  # start from a neutral focus position
    for _ in range(120):
        world.page.keyboard.press("Tab")
        if _active_testid(world) == "ui-button-primary":
            return
    raise AssertionError("could not Tab to the primary button")


@then("a focus ring is drawn around the primary button")
def ring_drawn(world: World) -> None:
    shadow = _ring(world)
    assert _FOCUS_RING in shadow, f"expected focus ring colour in box-shadow, got {shadow!r}"


@when("the primary button is clicked with the mouse")
def click_primary_with_mouse(world: World) -> None:
    world.page.mouse.click(1, 1)  # drop keyboard focus first
    _tid(world, "ui-button-primary").click()


@then("no focus ring is drawn around the primary button")
def no_ring(world: World) -> None:
    shadow = _ring(world)
    assert _FOCUS_RING not in shadow, f"focus ring drawn after a mouse click: {shadow!r}"


# ---------------------------------------------------------------- Data states
@then(parsers.parse('the empty table keeps its column headers and shows the "{text}" description'))
def empty_table(world: World, text: str) -> None:
    table = world.page.get_by_role("table", name="Bảng trống")
    expect(table.get_by_role("columnheader")).to_have_count(2)
    expect(table.get_by_text(text)).to_be_visible()


_STAT_HEIGHTS = "stat_heights"


def _stat_cards(world: World) -> Locator:
    return _tid(world, "ui-loading-toggle").locator("> div").first.locator("> *")


@when("the statistics switch from loading to their values")
def statistics_switch(world: World) -> None:
    cards = _stat_cards(world)
    expect(cards).to_have_count(3)
    expect(cards.first).to_have_attribute("aria-busy", "true")
    world.state.extra[_STAT_HEIGHTS] = [_box(cards.nth(i))["height"] for i in range(3)]
    _tid(world, "ui-loading-toggle-button").click()
    expect(_tid(world, "ui-loading-toggle").get_by_text("12.500.000")).to_be_visible()


@then("the height of every statistic card is unchanged")
def statistic_heights_unchanged(world: World) -> None:
    before = world.state.extra[_STAT_HEIGHTS]
    cards = _stat_cards(world)
    after = [_box(cards.nth(i))["height"] for i in range(3)]
    for b, a in zip(before, after, strict=True):
        assert abs(b - a) < 0.5, f"card height changed {before} -> {after}"


# ---------------------------------------------------------------- Modal
@when("the user opens the modal and presses Tab 8 times")
def open_modal_and_tab(world: World) -> None:
    _tid(world, "ui-modal-open").click()
    dialog = world.page.get_by_role("dialog")
    expect(dialog).to_be_visible()
    world.state.extra["focus_inside"] = []
    for _ in range(8):
        world.page.keyboard.press("Tab")
        world.state.extra["focus_inside"].append(
            world.page.evaluate(
                "() => { const d = document.querySelector('dialog[open]');"
                " return !!d && d.contains(document.activeElement); }"
            )
        )


@then("focus stays inside the dialog")
def focus_inside(world: World) -> None:
    trail = world.state.extra["focus_inside"]
    assert all(trail), f"focus left the dialog during Tab cycling: {trail}"


@when("the user presses Escape")
def press_escape(world: World) -> None:
    world.page.keyboard.press("Escape")


@then("the dialog is closed and focus is back on the opening button")
def dialog_closed_focus_restored(world: World) -> None:
    expect(world.page.get_by_role("dialog")).to_have_count(0)
    assert _active_testid(world) == "ui-modal-open", "focus did not return to the opener"


# ---------------------------------------------------------------- Tabs
@when("focus is on the first tab and the user presses ArrowRight")
def tabs_arrow(world: World) -> None:
    tabs = _tid(world, "ui-tabs").get_by_role("tab")
    tabs.first.focus()
    world.page.keyboard.press("ArrowRight")


@then("the second tab is selected and its panel is shown")
def second_tab_selected(world: World) -> None:
    tabs = _tid(world, "ui-tabs").get_by_role("tab")
    expect(tabs.nth(1)).to_have_attribute("aria-selected", "true")
    expect(tabs.first).to_have_attribute("aria-selected", "false")
    expect(_tid(world, "ui-tabs").get_by_role("tabpanel")).to_have_text("Đơn đang giao")


def _link_tabs(world: World) -> Locator:
    return world.page.get_by_role("navigation", name="Tabs")


@then(parsers.parse('the link tabs are anchors and "{label}" is the current page'))
def link_tabs_anchors(world: World, label: str) -> None:
    links = _link_tabs(world).get_by_role("link")
    expect(links).to_have_count(3)
    expect(links.first).to_have_attribute("href", re.compile(r"/dev/ui\?status=all$"))
    expect(_link_tabs(world).get_by_role("link", name=re.compile(label))).to_have_attribute(
        "aria-current", "page"
    )


@when(parsers.parse('the link tab "{label}" is clicked and the user goes back'))
def link_tab_click_back(world: World, label: str) -> None:
    target = _link_tabs(world).get_by_role("link", name=re.compile(label))
    target.click()
    expect(world.page).to_have_url(re.compile(r"status=done"))
    expect(_link_tabs(world).get_by_role("link", name=re.compile(label))).to_have_attribute(
        "aria-current", "page"
    )
    world.page.go_back()


@then(parsers.parse('"{label}" is the current link tab again'))
def link_tab_restored(world: World, label: str) -> None:
    expect(_link_tabs(world).get_by_role("link", name=re.compile(label))).to_have_attribute(
        "aria-current", "page", timeout=timeouts.DEFAULT
    )


# ---------------------------------------------------------------- Input
@then(parsers.parse('the "{label}" input is invalid and described by "{message}"'))
def input_error_announced(world: World, label: str, message: str) -> None:
    field = world.page.get_by_label(label, exact=True)
    expect(field).to_have_attribute("aria-invalid", "true")
    description = field.evaluate(
        "el => (el.getAttribute('aria-describedby') || '').split(' ')"
        ".map(id => document.getElementById(id)?.textContent || '').join(' ').trim()"
    )
    assert description == message, f"description {description!r} != {message!r}"


# ---------------------------------------------------------------- QuantityPicker
@then("the quantity picker at its maximum has a disabled increment button")
def quantity_at_max(world: World) -> None:
    picker = _tid(world, "ui-quantity")
    expect(picker.get_by_role("spinbutton")).to_have_value("3")
    expect(picker.get_by_role("button", name="Tăng số lượng")).to_be_disabled()


@when("5 is typed into the quantity picker")
def type_five(world: World) -> None:
    field = _tid(world, "ui-quantity").get_by_role("spinbutton")
    field.click()
    field.press("Control+a")
    field.press_sequentially("5")


@then("the quantity picker shows 3 and its decrement button is enabled")
def quantity_clamped(world: World) -> None:
    picker = _tid(world, "ui-quantity")
    expect(picker.get_by_role("spinbutton")).to_have_value("3")
    expect(picker.get_by_role("button", name="Giảm số lượng")).to_be_enabled()


# ---------------------------------------------------------------- Pagination / Image
@then("pagination links to pages 1 to 5 are shown and page 2 is the current page")
def pagination_links(world: World) -> None:
    nav = world.page.get_by_role("navigation", name="Phân trang").first
    for page in range(1, 6):
        link = nav.get_by_role("link", name=f"Trang {page}")
        expect(link).to_have_attribute("href", f"/dev/ui?page={page}")
    expect(nav.get_by_role("link", name="Trang 2")).to_have_attribute("aria-current", "page")


@then("the broken image shows the fallback placeholder inside a square box")
def image_fallback(world: World) -> None:
    box = _tid(world, "ui-image-broken").locator("> div").first
    expect(box).to_have_css("aspect-ratio", "1 / 1")
    expect(box.get_by_role("img", name="Không có ảnh")).to_be_visible(timeout=timeouts.DEFAULT)
    size = _box(box)
    assert abs(size["width"] - size["height"]) < 1, f"box is not square: {size}"
