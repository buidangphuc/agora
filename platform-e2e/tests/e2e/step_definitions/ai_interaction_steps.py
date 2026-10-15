"""Steps for tracking/ai_interaction_events.feature (frontend-ai-interaction-events).

Black box: the AI surfaces are driven through the real pages (team-ai in mock mode); the outcome is
the set of `ai_interaction` beacons the browser sends to /api/track (support/ai_events_support.py).
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from src.pages import AssistantPage, SellerNewListingPage
from src.api.services import ChatService
from tests.e2e.flows.auth_flow import SESSION_COOKIE
from tests.e2e.support import ai_events_support as ai
from tests.e2e.support import d2_support as d2
from tests.e2e.support import review_order_support as ros
from tests.e2e.support.world import World

_QUESTION = "Tìm laptop dưới 20 triệu"
_SECOND_QUESTION = "Gợi ý cho tôi một chiếc áo thun"
_COMPOSER = "Nhập nội dung tin nhắn"
_COPILOT = "Gợi ý AI"
_BUYER_MESSAGE = "Sản phẩm này còn hàng không shop?"


def _x(world: World) -> dict:
    return world.state.extra


def _base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


# ── Setup ────────────────────────────────────────────────────────────────
@given("the browser records AI beacons")
def browser_records(world: World) -> None:
    ai.start_recording(world)


# ── Assistant ────────────────────────────────────────────────────────────
def _assistant(world: World) -> AssistantPage:
    return AssistantPage(world.page)


def _ask(world: World, question: str) -> None:
    page = _assistant(world)
    page.wait_until_interactive(page.chat_input)
    page.chat_input.fill(question)
    expect(page.send_button).to_be_enabled(timeout=timeouts.DEFAULT)
    page.send_button.click()


def _cards(world: World):  # noqa: ANN202
    return world.page.get_by_role("link", name="Xem Ngay")


def _open_assistant_and_ask(world: World, question: str) -> None:
    world.page.goto(f"{_base(world)}/assistant", wait_until="domcontentloaded")
    _ask(world, question)
    expect(_cards(world).first).to_be_visible(timeout=timeouts.LONG)
    _x(world)["ai_reply_text"] = _assistant(world).ai_replies.last.inner_text()


@when("the buyer asks the assistant for a product and a product card is shown")
@when("the guest asks the assistant for a product and a product card is shown")
def ask_for_product(world: World) -> None:
    _open_assistant_and_ask(world, _QUESTION)


@when(parsers.parse('the buyer asks the assistant "{question}" and a product card is shown'))
def ask_with_marker(world: World, question: str) -> None:
    _open_assistant_and_ask(world, question)


@when("the buyer types in the assistant input box without sending")
def type_in_assistant(world: World) -> None:
    page = _assistant(world)
    page.chat_input.press_sequentially("abc")
    expect(page.chat_input).to_have_value("abc")


@when("the buyer asks the assistant a second question and more product cards are shown")
def ask_second(world: World) -> None:
    before = _cards(world).count()
    _ask(world, _SECOND_QUESTION)
    expect(_cards(world)).not_to_have_count(before, timeout=timeouts.LONG)


def _first_card_listing(world: World) -> str:
    href = _cards(world).first.get_attribute("href") or ""
    assert "/listing/" in href, f"the Xem Ngay link does not point at a listing: {href!r}"
    return href.rstrip("/").split("/")[-1].split("?")[0]


@when('the buyer clicks "Xem Ngay" on the first product card')
def click_xem_ngay(world: World) -> None:
    listing_id = _first_card_listing(world)
    _x(world).setdefault("ai_clicked", []).append(listing_id)
    _cards(world).first.click()


@then("the buyer lands on the listing page of that card")
def lands_on_listing(world: World) -> None:
    expect(world.page).to_have_url(
        re.compile(rf".*/listing/{re.escape(_x(world)['ai_clicked'][-1])}"), timeout=timeouts.NAVIGATION
    )


@when('the buyer asks the assistant for a product again and clicks "Mua Ngay" on the first product card')
def click_mua_ngay(world: World) -> None:
    _open_assistant_and_ask(world, _QUESTION)
    _x(world)["ai_clicked"].append(_first_card_listing(world))
    buy = world.page.get_by_role("link", name="Mua Ngay").first
    expect(buy).to_be_visible(timeout=timeouts.DEFAULT)
    buy.click()


@then("the buyer lands on the checkout page")
def lands_on_checkout(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/checkout"), timeout=timeouts.NAVIGATION)


# ── Generic event assertions ─────────────────────────────────────────────
@then(parsers.parse('exactly {n:d} "{surface}" "{action}" AI events are captured'))
def exactly_n(world: World, n: int, surface: str, action: str) -> None:
    found = ai.settled(world, n, surface, action)
    assert len(found) == n, f"expected {n} {surface}/{action}, got {len(found)}: {ai.describe(found)}"


@then(parsers.parse('at least {n:d} "{surface}" "{action}" AI events are captured'))
def at_least_n(world: World, n: int, surface: str, action: str) -> None:
    found = ai.wait_for(world, n, surface, action)
    assert len(found) >= n, f"expected >= {n} {surface}/{action}, got {len(found)}"


@then(parsers.parse('zero "{surface}" "{action}" AI events are captured'))
def zero_surface_action(world: World, surface: str, action: str) -> None:
    found = ai.quiet(world, surface, action)
    assert not found, f"expected no {surface}/{action} event, got {ai.describe(found)}"


@then(parsers.parse('zero "{surface}" AI events are captured'))
def zero_surface(world: World, surface: str) -> None:
    found = ai.quiet(world, surface)
    assert not found, f"expected no {surface} event, got {ai.describe(found)}"


@then(parsers.parse('every captured "{surface}" "{action}" AI event carries the id of the clicked card\'s listing'))
def carries_clicked_listing(world: World, surface: str, action: str) -> None:
    found = ai.ai_events(world, surface, action)
    got = sorted(e.get("listingId") for e in found)
    assert got == sorted(_x(world)["ai_clicked"][: len(found)]), (
        f"listingIds {got} != clicked {_x(world)['ai_clicked']}"
    )


@then(parsers.parse('every captured "{surface}" "{action}" AI event carries the listing id of that listing'))
def carries_listing(world: World, surface: str, action: str) -> None:
    found = ai.ai_events(world, surface, action)
    assert found and all(e.get("listingId") == _x(world)["ai_listing_id"] for e in found), (
        f"expected listingId {_x(world)['ai_listing_id']}, got {ai.describe(found)}"
    )


@then("every /api/track batch holding an ai_interaction beacon was answered 2xx by the gateway")
def collector_accepted(world: World) -> None:
    ai.settled(world, 1, "assistant", "shown")
    answered = [s for s in _x(world)["ai_statuses"] if s["has_ai"] or not s["body"]]
    assert answered, "no /api/track response was observed"
    bad = [s["status"] for s in answered if not 200 <= s["status"] < 300]
    assert not bad, f"gateway answered {bad} to batches holding ai_interaction beacons"


@then(parsers.parse('no captured beacon body contains "{marker}" or the assistant reply text'))
def no_free_text(world: World, marker: str) -> None:
    reply = _x(world).get("ai_reply_text", "")
    reply_lines = [ln.strip() for ln in reply.splitlines() if len(ln.strip()) >= 12]
    assert reply_lines, "the assistant reply text was not captured"
    bodies = [s["body"] for s in _x(world)["ai_statuses"]]
    bodies += [str(b) for b in ai.all_beacons(world)]
    for body in bodies:
        assert marker not in body, f"beacon body leaks the prompt: {body[:200]}"
        for line in reply_lines:
            assert line not in body, f"beacon body leaks reply text: {line}"


@then("every captured AI event has exactly the keys surface and action in its properties")
def exact_keys(world: World) -> None:
    events = ai.ai_events(world)
    assert events, "no ai_interaction beacon captured"
    for e in events:
        assert set((e.get("properties") or {}).keys()) == ai.ALLOWED_KEYS, ai.describe([e])


# ── Seller chat copilot ──────────────────────────────────────────────────
def _composer(world: World):  # noqa: ANN202
    return world.page.get_by_placeholder(_COMPOSER, exact=False)


def _copilot_button(world: World):  # noqa: ANN202
    return world.page.get_by_role("button", name=_COPILOT)


def _chips(world: World):  # noqa: ANN202
    strip = _copilot_button(world).locator("xpath=..")
    return strip.get_by_role("button").filter(has_not_text=_COPILOT)


@given("a seller has a chat thread with a buyer who asked about a listing")
def seller_thread(world: World) -> None:
    shop = d2.seed_shop(price=100_000, stock=5)
    _, buyer_token = d2.register("buyer")
    chat = ChatService(token=buyer_token)
    thread = chat.get_or_create_thread(d2.principal_id(shop["token"]), shop["listing_id"])
    assert thread.get("id"), thread
    chat.send_message(thread["id"], _BUYER_MESSAGE)
    x = _x(world)
    x.update(ai_thread=thread["id"], ai_shop=shop, ai_buyer_token=buyer_token)
    world.context.add_cookies(
        [{"name": SESSION_COOKIE, "value": shop["token"], "url": world.settings.base_url}]
    )
    world.service_factory.set_token(shop["token"])


def _open_thread(world: World) -> None:
    world.page.goto(f"{_base(world)}/chat?thread={_x(world)['ai_thread']}", wait_until="domcontentloaded")
    expect(world.page.get_by_text(_BUYER_MESSAGE).first).to_be_visible(timeout=timeouts.NAVIGATION)


@when('the seller opens the thread and clicks "Gợi ý AI"')
def seller_opens_and_clicks(world: World) -> None:
    _open_thread(world)
    button = _copilot_button(world)
    expect(button).to_be_visible(timeout=timeouts.DEFAULT)
    world.page.wait_for_timeout(1_500)  # hydration: a click before it is dropped
    button.click()
    expect(_chips(world).first).to_be_visible(timeout=timeouts.LONG)


@when("the seller types in the composer without inserting a suggestion")
def seller_types_plain(world: World) -> None:
    _composer(world).press_sequentially("abc")


@then(parsers.parse("{n:d} copilot suggestions are shown"))
def n_suggestions(world: World, n: int) -> None:
    expect(_chips(world)).to_have_count(n, timeout=timeouts.DEFAULT)


def _chip_text(world: World, index: int) -> str:
    return _chips(world).nth(index).inner_text().strip()


@when("the seller clicks the first copilot suggestion")
def click_first_chip(world: World) -> None:
    _x(world)["ai_inserted"] = _chip_text(world, 0)
    _chips(world).nth(0).click()


@when("the seller clicks the second copilot suggestion")
def click_second_chip(world: World) -> None:
    _x(world)["ai_inserted"] = _chip_text(world, 1)
    _chips(world).nth(1).click()


@then("the composer holds the first copilot suggestion")
def composer_holds(world: World) -> None:
    expect(_composer(world)).to_have_value(_x(world)["ai_inserted"], timeout=timeouts.DEFAULT)


@when(parsers.parse("the seller types {n:d} more characters in the composer"))
def type_more(world: World, n: int) -> None:
    box = _composer(world)
    box.press("End")
    box.press_sequentially("x" * n)


@when("the seller sends the composer text")
def send_composer(world: World) -> None:
    world.page.get_by_role("button", name=re.compile(r"^Gửi")).last.click()
    expect(_composer(world)).to_have_value("", timeout=timeouts.DEFAULT)


@when("the buyer of that thread opens it")
def buyer_opens_thread(world: World) -> None:
    token = _x(world)["ai_buyer_token"]
    world.context.clear_cookies()
    world.context.add_cookies([{"name": SESSION_COOKIE, "value": token, "url": world.settings.base_url}])
    world.service_factory.set_token(token)
    world.page.goto(f"{_base(world)}/chat?thread={_x(world)['ai_thread']}", wait_until="domcontentloaded")
    expect(world.page.get_by_text(_BUYER_MESSAGE).first).to_be_visible(timeout=timeouts.NAVIGATION)


@then('the "Gợi ý AI" button is absent')
def copilot_absent(world: World) -> None:
    world.page.wait_for_timeout(1_500)
    expect(_copilot_button(world)).to_have_count(0)


# ── Magic Listing ────────────────────────────────────────────────────────
def _form(world: World) -> SellerNewListingPage:
    return SellerNewListingPage(world.page)


@when("the seller opens the new listing form for Magic Listing")
def open_form(world: World) -> None:
    world.page.goto(f"{_base(world)}/seller/new", wait_until="domcontentloaded")
    form = _form(world)
    expect(form.title_input).to_be_visible(timeout=timeouts.NAVIGATION)
    form.wait_until_interactive(form.title_input)
    form.title_input.fill("Laptop Dell XPS 13 9340 Chính Hãng")


@when("the seller generates a Magic Listing draft")
def generate(world: World) -> None:
    form = _form(world)
    expect(form.magic_generate).to_be_enabled(timeout=timeouts.DEFAULT)
    n = _x(world).get("ai_drafts", 0)
    form.magic_generate.click()
    expect(form.magic_suggestion).to_be_visible(timeout=timeouts.LONG)
    _x(world)["ai_drafts"] = n + 1
    world.page.wait_for_timeout(500)


@when("the seller types in the stock field")
def type_stock(world: World) -> None:
    _form(world).stock_input.fill("7")


@when("the seller applies all Magic Listing suggestions")
def apply_all(world: World) -> None:
    _form(world).magic_apply_all.click()
    expect(_form(world).description_input).not_to_have_value("", timeout=timeouts.DEFAULT)


@when("the seller edits the title")
def edit_title(world: World) -> None:
    box = _form(world).title_input
    box.press("End")
    box.press_sequentially(" x")


@when("the seller edits the description")
def edit_description(world: World) -> None:
    box = _form(world).description_input
    box.press("End")
    box.press_sequentially(" x")


@then("the description field is filled")
def description_filled(world: World) -> None:
    expect(_form(world).description_input).not_to_have_value("", timeout=timeouts.DEFAULT)


@when("a guest requests the new listing page")
def guest_requests_new_listing(world: World) -> None:
    world.page.goto(f"{_base(world)}/seller/new", wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)


@then("the guest is redirected to the login page and no Magic Listing card is shown")
def guest_redirected(world: World) -> None:
    expect(world.page).to_have_url(re.compile(r".*/login"), timeout=timeouts.NAVIGATION)
    expect(_form(world).magic_generate).to_have_count(0)


# ── PDP review summary ───────────────────────────────────────────────────
@given("a listing with verified reviews exists for AI events")
def listing_with_reviews(world: World) -> None:
    shop = d2.seed_shop(price=100_000, stock=20)
    # Rows go straight into engagement_db (precedent: rvp_steps): the order service the API route
    # (`seed_verified_review`) needs to complete an order is not part of this stack.
    ros.seed_verified_reviews_sql(
        shop["listing_id"], d2.principal_id(shop["token"]), 2, comment_prefix="aiev"
    )
    _x(world)["ai_listing_id"] = shop["listing_id"]


@given("a listing without reviews exists for AI events")
def listing_without_reviews(world: World) -> None:
    _x(world)["ai_bare_listing_id"] = d2.seed_shop(price=100_000, stock=20)["listing_id"]


def _summary(world: World):  # noqa: ANN202
    return world.page.get_by_test_id("ai-review-summary")


@when("a guest opens that listing page and the AI review summary card is shown")
def open_with_summary(world: World) -> None:
    world.page.goto(f"{_base(world)}/listing/{_x(world)['ai_listing_id']}", wait_until="domcontentloaded")
    expect(_summary(world)).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the page is resized and left idle")
def resized(world: World) -> None:
    world.page.set_viewport_size({"width": 900, "height": 700})
    world.page.set_viewport_size({"width": 1280, "height": 800})
    world.page.wait_for_timeout(1_500)


@when("a guest opens the listing without reviews and its reviews section has loaded")
def open_bare(world: World) -> None:
    world.page.goto(f"{_base(world)}/listing/{_x(world)['ai_bare_listing_id']}", wait_until="domcontentloaded")
    section = world.page.locator("section", has=world.page.locator("#reviews-heading"))
    expect(section).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(section.locator('[aria-busy="true"]')).to_have_count(0, timeout=timeouts.NAVIGATION)


@then("no AI review summary card is shown")
def no_summary(world: World) -> None:
    expect(_summary(world)).to_have_count(0)
