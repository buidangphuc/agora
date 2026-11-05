"""Steps for ui_foundation.feature: design-token colours and the 404 shell."""

from __future__ import annotations

from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect
from pytest_bdd import parsers, then, when

from src.constants import PageName, timeouts
from src.pages import HomePage
from tests.e2e.support.world import World

_TO_RGB = """
(v) => {
  const m = v.trim().match(/^color\\(srgb ([\\d.]+) ([\\d.]+) ([\\d.]+)(?: \\/ ([\\d.]+))?\\)$/);
  if (!m) return v.trim();
  const [r, g, b] = [m[1], m[2], m[3]].map((x) => Math.round(parseFloat(x) * 255));
  if (m[4] !== undefined && parseFloat(m[4]) !== 1) return `rgba(${r}, ${g}, ${b}, ${m[4]})`;
  return `rgb(${r}, ${g}, ${b})`;
}
"""

_ALIAS_PROBE = """
(name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()
"""

# For every element whose class list uses the action-primary alias, read the
# property that alias drives (bg-* -> background, text-* -> colour).
_ALIAS_CONSUMERS_TEMPLATE = """
() => {
  const toRgb = ${TO_RGB};
  const out = [];
  for (const el of document.querySelectorAll('[class*="action-primary"]')) {
    const cs = getComputedStyle(el);
    const cls = el.className.toString().split(/\\s+/);
    if (cls.some((c) => /^(hover:|focus:)?bg-action-primary$/.test(c)))
      out.push(["background", toRgb(cs.backgroundColor)]);
    if (cls.includes("text-action-primary")) out.push(["colour", toRgb(cs.color)]);
  }
  return out;
}
"""
_ALIAS_CONSUMERS = _ALIAS_CONSUMERS_TEMPLATE.replace("${TO_RGB}", _TO_RGB.strip())


@when(parsers.parse('the visitor opens the unknown route "{route}"'))
def open_unknown_route(world: World, route: str) -> None:
    base = world.settings.base_url.rstrip("/")
    response = world.page.goto(f"{base}{route}", wait_until="domcontentloaded")
    world.state.extra["last_response"] = response


@then(parsers.parse("the response status is {status:d}"))
def response_status_is(world: World, status: int) -> None:
    response = world.state.extra["last_response"]
    assert response is not None, "navigation produced no response"
    assert response.status == status, f"expected HTTP {status}, got {response.status}"


@then("a not-found result with a link back to home is shown")
def not_found_result_shown(world: World) -> None:
    expect(world.page.get_by_text("Không tìm thấy trang")).to_be_visible(timeout=timeouts.DEFAULT)
    home_link = world.page.get_by_role("link", name="Về trang chủ")
    expect(home_link).to_be_visible(timeout=timeouts.DEFAULT)
    expect(home_link).to_have_attribute("href", "/")


@then(parsers.parse('the header search button renders the brand colour "{colour}"'))
def header_button_brand_colour(world: World, colour: str) -> None:
    home: HomePage = world.get_page(PageName.HOME)  # type: ignore[assignment]
    button = home.header.search_button
    expect(button).to_be_visible(timeout=timeouts.DEFAULT)
    got = world.page.evaluate(
        f"(el) => ({_TO_RGB.strip()})(getComputedStyle(el).backgroundColor)",
        button.element_handle(),
    )
    assert got == colour, f"search button background is {got!r}, expected {colour!r}"


@then(parsers.parse('the "{alias}" alias resolves to the brand colour "{value}"'))
def alias_resolves_to_brand(world: World, alias: str, value: str) -> None:
    resolved = world.page.evaluate(_ALIAS_PROBE, alias)
    assert (
        resolved.lower() == value.lower()
    ), f"{alias} resolved to {resolved!r}, expected {value!r}"


@when(parsers.parse('the "{alias}" alias is overridden with "{colour}"'))
def override_alias(world: World, alias: str, colour: str) -> None:
    world.page.add_style_tag(content=f":root {{ {alias}: {colour}; }}")


@then(parsers.parse('every element styled by the action-primary alias renders "{colour}"'))
def alias_consumers_render(world: World, colour: str) -> None:
    consumers = world.page.evaluate(_ALIAS_CONSUMERS)
    assert consumers, "no element on the page uses the action-primary alias"
    try:
        # Buttons animate colour changes (transition-all), so poll until settled.
        world.page.wait_for_function(
            f"(c) => ({_ALIAS_CONSUMERS.strip()})().every(([, got]) => got === c)",
            arg=colour,
            timeout=timeouts.DEFAULT,
        )
    except PlaywrightTimeoutError:
        wrong = [
            (prop, got) for prop, got in world.page.evaluate(_ALIAS_CONSUMERS) if got != colour
        ]
        raise AssertionError(
            f"elements ignored the alias override (expected {colour}): {wrong[:5]}"
        ) from None
