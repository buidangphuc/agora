"""Steps for tracking/loop_events.feature (frontend-loop-events).

Black box: the UI is driven through the real pages; the outcome is read from analytics.events,
window.dataLayer and document.cookie (see support/loop_events_support.py).
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services.referral_service import ReferralService
from src.constants import PageName, timeouts
from src.pages import LoginPage, RegisterPage
from src.pages.loyalty_checkin_widget import LoyaltyCheckinWidget
from src.pages.shop_follow_page import ShopFollowPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support import loop_events_support as lp
from tests.e2e.support.world import World

_EVENT_TYPES = {
    "favorite": lp.FAVORITE,
    "share": lp.SHARE,
    "sign_up": lp.SIGN_UP,
    "login": lp.LOGIN,
    "follow_seller": lp.FOLLOW_SELLER,
    "referral_redeem": lp.REFERRAL_REDEEM,
    "daily_check_in": lp.DAILY_CHECK_IN,
    "alert_subscribe": lp.ALERT_SUBSCRIBE,
}
_REFERRAL_PLACEHOLDER = "Nhập mã của bạn bè"
_HEART = "Yêu thích"
_UNHEART = "Bỏ thích"


def _x(world: World) -> dict:
    return world.state.extra


def _buyer(world: World):  # noqa: ANN202
    buyer = _x(world).get("seeded_buyer")
    assert buyer and buyer.token, "scenario must be tagged @needsBuyer"
    return buyer


def _principal(world: World) -> str:
    """The principal the scenario's events are attributed to."""
    return _x(world).get("loop_principal") or lp.jwt_subject(_buyer(world).token)


def _listing_id(world: World) -> str:
    assert world.state.listing and world.state.listing.listing_id, "needs @needsListing"
    return world.state.listing.listing_id


def _goto(world: World, path: str) -> None:
    world.page.goto(world.settings.base_url + path, wait_until="domcontentloaded")
    world.page.wait_for_load_state("load", timeout=timeouts.NAVIGATION)


def _open_listing(world: World) -> None:
    _goto(world, f"/listing/{_listing_id(world)}")


def _heart(world: World, name: str = _HEART):  # noqa: ANN202
    return world.page.get_by_role("button", name=name, exact=True).first


def _click_heart(world: World) -> None:
    heart = _heart(world)
    expect(heart).to_be_visible(timeout=timeouts.NAVIGATION)
    LoginPage(world.page).wait_until_interactive(heart)
    heart.click()


def _sessions(world: World) -> list[str]:
    sessions = sorted(lp.recorded_session_ids(world))
    assert sessions, "the page sent no beacon, so its session cannot be observed"
    return sessions


# ── Setup ────────────────────────────────────────────────────────────────
@given("the browser records tracking beacons")
def browser_records_beacons(world: World) -> None:
    lp.start_beacon_recording(world)


@given("another user has created a referral code through the API")
def other_user_referral_code(world: World) -> None:
    other = world.service_factory.auth.register(
        fake.unique_username("buyer"), world.settings.seed_password, "buyer"
    )
    code = ReferralService(token=other).create_referral_code().get("code", "")
    assert code, "CreateReferralCode returned no code"
    _x(world)["loop_referral_code"] = code
    # the service factory keeps the token of the last registration; restore the scenario buyer
    world.service_factory.set_token(_buyer(world).token)


@when("the buyer is signed in within the same browser session")
def buyer_signed_in_same_session(world: World) -> None:
    login_via_api(world, _buyer(world))


# ── Activation ───────────────────────────────────────────────────────────
@when("a visitor registers a new buyer account on the register page and lands on the home page")
def visitor_registers(world: World) -> None:
    username = fake.unique_username("buyer")
    page: RegisterPage = world.navigate_to(PageName.REGISTER)  # type: ignore[assignment]
    page.register(username, world.settings.seed_password, "buyer")
    world.page.wait_for_url(f"{world.settings.base_url}/", timeout=timeouts.NAVIGATION)
    token = lp.session_token(world)
    assert token, "registration did not start a session"
    _x(world)["loop_principal"] = lp.jwt_subject(token)


@when("the buyer logs in on the login page and lands on the home page")
def buyer_logs_in_ui(world: World) -> None:
    buyer = _buyer(world)
    page: LoginPage = world.navigate_to(PageName.LOGIN)  # type: ignore[assignment]
    page.login(buyer.username, buyer.password)
    world.page.wait_for_url(f"{world.settings.base_url}/", timeout=timeouts.NAVIGATION)


@when("the page is reloaded twice")
def page_reloaded_twice(world: World) -> None:
    for _ in range(2):
        world.page.reload(wait_until="load")
        world.page.wait_for_timeout(1_500)


@when("a visitor submits a wrong password on the login page")
def wrong_password(world: World) -> None:
    page: LoginPage = world.navigate_to(PageName.LOGIN)  # type: ignore[assignment]
    page.login(_buyer(world).username, "definitely-not-the-password")


@then("the login error is displayed")
def login_error_displayed(world: World) -> None:
    page: LoginPage = world.get_page(PageName.LOGIN)  # type: ignore[assignment]
    expect(page.error_message).to_be_visible(timeout=timeouts.DEFAULT)


@when("another visitor registers the buyer's existing username on the register page")
def duplicate_registration(world: World) -> None:
    page: RegisterPage = world.navigate_to(PageName.REGISTER)  # type: ignore[assignment]
    page.register(_buyer(world).username, world.settings.seed_password, "buyer")


@then("the registration error is displayed")
def registration_error_displayed(world: World) -> None:
    # the server action returns an inline field error and the visitor stays on /register
    expect(world.page.get_by_text("đã tồn tại", exact=False).first).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    assert world.page.url.rstrip("/").endswith("/register"), world.page.url


@when("the visitor opens the home page")
def visitor_opens_home(world: World) -> None:
    _goto(world, "/")
    for _ in range(4):  # impressions are viewability-gated: scroll so the session sends a beacon
        world.page.mouse.wheel(0, 700)
        world.page.wait_for_timeout(150)
    world.page.wait_for_timeout(4_000)  # beacon queue flushes after ~2 s idle


@then(parsers.parse('the "{name}" cookie is absent'))
def cookie_absent(world: World, name: str) -> None:
    assert name not in lp.document_cookies(world.page), lp.document_cookies(world.page)
    assert name not in {c["name"] for c in world.context.cookies()}, "cookie still in the jar"


# ── Engagement ───────────────────────────────────────────────────────────
@when("the buyer clicks the heart on the seeded listing page")
def buyer_clicks_heart(world: World) -> None:
    _open_listing(world)
    _click_heart(world)
    expect(_heart(world, _UNHEART)).to_be_visible(timeout=timeouts.DEFAULT)
    expect(_heart(world, _UNHEART)).to_be_enabled(timeout=timeouts.DEFAULT)


@when("the buyer clicks the heart again to remove the favorite")
def buyer_unfavorites(world: World) -> None:
    heart = _heart(world, _UNHEART)
    expect(heart).to_be_enabled(timeout=timeouts.DEFAULT)
    heart.click()
    expect(_heart(world, _HEART)).to_be_visible(timeout=timeouts.DEFAULT)
    expect(_heart(world, _HEART)).to_be_enabled(timeout=timeouts.DEFAULT)


@when("the buyer opens the share panel on the seeded listing page")
def buyer_opens_share(world: World) -> None:
    _open_listing(world)
    button = world.page.get_by_role("button", name=re.compile("Chia sẻ"))
    expect(button).to_be_visible(timeout=timeouts.NAVIGATION)
    LoginPage(world.page).wait_until_interactive(button)
    button.click()
    shown = world.page.get_by_text(re.compile(r"/s/[A-Za-z0-9_-]+$")).first
    expect(shown).to_be_visible(timeout=timeouts.DEFAULT)
    _x(world)["loop_short_code"] = shown.inner_text().strip().rsplit("/s/", 1)[-1]


@when(parsers.parse('the buyer clicks "{label}" once'))
def buyer_clicks_labelled(world: World, label: str) -> None:
    button = world.page.get_by_role("button", name=label)
    expect(button).to_be_visible(timeout=timeouts.DEFAULT)
    button.click()
    world.page.wait_for_timeout(500)


@when("a guest opens the seeded listing page and clicks the heart")
def guest_clicks_heart(world: World) -> None:
    _open_listing(world)
    _click_heart(world)
    world.page.wait_for_timeout(3_000)


@when("a guest opens the seeded seller's shop page and clicks follow")
def guest_clicks_follow(world: World) -> None:
    seller = world.state.seeded_seller
    assert seller and seller.token, "needs @needsSeller"
    shop = ShopFollowPage(world.page)
    world.set_current_page(shop)
    shop.open(lp.jwt_subject(seller.token))
    expect(shop.follow_toggle).to_be_visible(timeout=timeouts.NAVIGATION)
    shop.wait_until_interactive(shop.follow_toggle)
    shop.toggle_follow()
    world.page.wait_for_timeout(3_000)


@then("the guest is answered Unauthenticated and nothing is favorited or followed")
def guest_rejected(world: World) -> None:
    # the optimistic follow rolled back: the shop still offers "+ Theo dõi", not "Đang theo dõi"
    expect(ShopFollowPage(world.page).following_state).to_have_count(0)


@when('the buyer turns on the "price_drop" alert on the seeded listing page')
def buyer_alert_on(world: World) -> None:
    _open_listing(world)
    toggle = world.page.locator('[data-testid="alert-toggle"][data-type="price_drop"]')
    expect(toggle).to_be_visible(timeout=timeouts.NAVIGATION)
    LoginPage(world.page).wait_until_interactive(toggle)
    toggle.click()
    expect(toggle).to_have_attribute("data-active", "true", timeout=timeouts.DEFAULT)


@when('the buyer turns the "price_drop" alert off')
def buyer_alert_off(world: World) -> None:
    toggle = world.page.locator('[data-testid="alert-toggle"][data-type="price_drop"]')
    expect(toggle).to_be_enabled(timeout=timeouts.DEFAULT)
    toggle.click()
    expect(toggle).to_have_attribute("data-active", "false", timeout=timeouts.DEFAULT)


# ── Account ──────────────────────────────────────────────────────────────
def _redeem(world: World, code: str) -> None:
    _goto(world, "/account/referral")
    field = world.page.get_by_placeholder(_REFERRAL_PLACEHOLDER)
    LoginPage(world.page).wait_until_interactive(field)
    field.fill(code)
    world.page.get_by_role("button", name="Nhập mã").click()


@when("the buyer redeems that code on the referral page")
def redeem_that_code(world: World) -> None:
    _redeem(world, _x(world)["loop_referral_code"])
    world.page.wait_for_timeout(1_500)


@when(parsers.parse('the buyer redeems the nonexistent code "{code}" on the referral page'))
def redeem_nonexistent(world: World, code: str) -> None:
    _redeem(world, code)


@then("the referral error is displayed")
def referral_error_displayed(world: World) -> None:
    field = world.page.get_by_placeholder(_REFERRAL_PLACEHOLDER)
    expect(field).to_have_attribute("aria-invalid", "true", timeout=timeouts.NAVIGATION)


@when("the buyer checks in on the home page and reads the streak")
def buyer_checks_in(world: World) -> None:
    widget = LoyaltyCheckinWidget(world.page)
    world.set_current_page(widget)
    widget.navigate()
    expect(widget.check_in_button).to_be_visible(timeout=timeouts.NAVIGATION)
    widget.wait_until_interactive(widget.check_in_button)
    widget.check_in()
    expect(world.page.get_by_text("Điểm danh thành công", exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(widget.earned_marker).to_be_visible(timeout=timeouts.NAVIGATION)
    _x(world)["loop_streak"] = widget.streak_value.inner_text().strip()
    assert _x(world)["loop_streak"].isdigit(), _x(world)["loop_streak"]


# ── Observation: analytics.events ────────────────────────────────────────
def _store(world: World, events: list[dict]) -> None:
    _x(world)["loop_events"] = events


@then(
    parsers.parse(
        'exactly {n:d} "{name}" event is on analytics.events for the principal within 10 seconds'
    )
)
@then(
    parsers.parse(
        'exactly {n:d} "{name}" events are on analytics.events for the principal within 10 seconds'
    )
)
def exact_events_for_principal(world: World, n: int, name: str) -> None:
    events = lp.settled_events(_principal(world), _EVENT_TYPES[name], n)
    assert len(events) == n, f"expected {n} {name} event(s) for {_principal(world)}, got {events}"
    _store(world, events)


@then(
    parsers.parse(
        'exactly {n:d} "{name}" event for the seeded listing is on analytics.events '
        "for the principal within 10 seconds"
    )
)
@then(
    parsers.parse(
        'exactly {n:d} "{name}" events for the seeded listing are on analytics.events '
        "for the principal within 10 seconds"
    )
)
def exact_events_for_listing(world: World, n: int, name: str) -> None:
    listing = _listing_id(world)
    events = lp.settled_events(_principal(world), _EVENT_TYPES[name], n, listing_id=listing)
    assert len(events) == n, f"expected {n} {name} event(s) for {listing}, got {events}"
    _store(world, events)


@then(
    parsers.parse(
        'the count of "{name}" events for the seeded listing stays {n:d} after 10 seconds'
    )
)
def count_stays(world: World, name: str, n: int) -> None:
    events = lp.quiet_events(_principal(world), _EVENT_TYPES[name], listing_id=_listing_id(world))
    assert len(events) == n, f"expected {n} {name} event(s) after 10 s, got {events}"


@then(
    parsers.parse(
        'zero "{name}" events are on analytics.events for the recorded sessions after 10 seconds'
    )
)
def zero_for_sessions(world: World, name: str) -> None:
    events = lp.quiet_events(_sessions(world), _EVENT_TYPES[name])
    assert events == [], f"expected no {name} event, got {events}"


@then(
    parsers.parse('zero "{name}" events are on analytics.events for the principal after 10 seconds')
)
def zero_for_principal(world: World, name: str) -> None:
    events = lp.quiet_events(_principal(world), _EVENT_TYPES[name])
    assert events == [], f"expected no {name} event, got {events}"


@then(parsers.parse('the event property "{key}" equals "{value}"'))
def event_property_equals(world: World, key: str, value: str) -> None:
    (event,) = _x(world)["loop_events"]
    assert event["properties"].get(key) == value, event["properties"]


@then(parsers.parse('every event property "{key}" equals "{value}"'))
def every_event_property_equals(world: World, key: str, value: str) -> None:
    for event in _x(world)["loop_events"]:
        assert event["properties"].get(key) == value, event["properties"]


@then(parsers.parse('the event property "{key}" equals the shop id'))
def event_property_is_shop(world: World, key: str) -> None:
    seller = world.state.seeded_seller
    (event,) = _x(world)["loop_events"]
    assert event["properties"].get(key) == lp.jwt_subject(seller.token), event["properties"]


@then(parsers.parse('the event property "{key}" equals the streak shown on the widget'))
def event_property_is_streak(world: World, key: str) -> None:
    (event,) = _x(world)["loop_events"]
    assert event["properties"].get(key) == _x(world)["loop_streak"], event["properties"]


@then("no event contains the share short code")
def no_short_code(world: World) -> None:
    code = _x(world)["loop_short_code"].encode()
    for event in _x(world)["loop_events"]:
        assert code not in event["raw"], "a share event carries the short code"


@then("no event contains the referral code")
def no_referral_code(world: World) -> None:
    code = _x(world)["loop_referral_code"].encode()
    for event in _x(world)["loop_events"]:
        assert code not in event["raw"], "a referral_redeem event carries the code"


# ── Observation: window.dataLayer ────────────────────────────────────────
@then(parsers.parse('window.dataLayer holds exactly {n:d} "{name}" entry with method "{method}"'))
def datalayer_method(world: World, n: int, name: str, method: str) -> None:
    entries = lp.datalayer_entries(world.page, name)
    assert len(entries) == n, f"dataLayer {name} entries: {entries}"
    assert entries[0].get("properties") == {"method": method}, entries[0]


@then(parsers.parse('window.dataLayer holds exactly {n:d} "{name}" entry for the seeded listing'))
def datalayer_listing(world: World, n: int, name: str) -> None:
    entries = lp.datalayer_entries(world.page, name)
    assert len(entries) == n, f"dataLayer {name} entries: {entries}"
    items = entries[0].get("ecommerce", {}).get("items", [])
    assert items and items[0].get("item_id") == _listing_id(world), entries[0]
