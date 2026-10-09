"""Steps for frontend/account_ui.feature (ui-phase-account).

Scenario names echo the spec scenarios of openspec/changes/ui-phase-account. A signed-in
buyer is registered through the gateway and the session cookie is injected (fast); the
assertions are on the rendered UI, on what the browser sent (counted Server Action POSTs)
and on the gateway state the mutation left behind.
"""

from __future__ import annotations

import re
import subprocess

from playwright.sync_api import Page, expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AddressService, AuthService, ListingService
from src.api.services.base_service import BaseService
from src.api.services.verification_service import VerificationService
from src.constants import timeouts
from src.models import Listing, User
from src.pages import AddressesPage, FavoritesPage, LoginPage, NotificationsPage
from src.pages.verification_page import VerificationPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.account_steps import _shell, _sign_in_buyer
from tests.e2e.step_definitions.d1_gates_steps import FRONTEND, _run
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support.world import World

SETTINGS = get_settings()
BASE = SETTINGS.base_url.rstrip("/")
_ENGAGEMENT = "/platform.engagement.v1.EngagementService"
_PHONE = {"width": 375, "height": 812}
_DESKTOP = {"width": 1280, "height": 900}


# ── helpers ──────────────────────────────────────────────────────────────
def _goto(world: World, route: str) -> None:
    world.page.goto(f"{BASE}{route}", wait_until="domcontentloaded")


def _track_posts(world: World) -> list[str]:
    """Record every Server Action POST (the Next-Action header) the browser sends."""
    posts: list[str] = []
    world.state.extra["action_posts"] = posts

    def on_request(request) -> None:  # noqa: ANN001
        if request.method == "POST" and request.headers.get("next-action"):
            posts.append(request.url)

    world.page.on("request", on_request)
    return posts


def _toast(world: World, text: str | None = None):  # noqa: ANN202
    toast = world.page.get_by_role("status")
    return toast.filter(has_text=text) if text else toast


def _api(token: str) -> BaseService:
    return BaseService(token=token)


# ── Session redirect ─────────────────────────────────────────────────────
@when(parsers.parse('an anonymous visitor opens "{route}"'))
def anonymous_opens(world: World, route: str) -> None:
    bodies: list[str] = []

    def on_response(response) -> None:  # noqa: ANN001
        if (
            response.url.rstrip("/").endswith(route)
            and response.request.resource_type == "document"
        ):
            try:
                bodies.append(response.text())
            except Exception:  # noqa: BLE001 - a redirect response has no body
                bodies.append("")

    world.page.on("response", on_response)
    world.page.goto(f"{BASE}{route}", wait_until="domcontentloaded")
    world.state.extra["route_bodies"] = bodies


@then("they are redirected to the login page and no address data is in the response")
def redirected_without_data(world: World) -> None:
    world.page.wait_for_url(re.compile(r".*/login"), timeout=timeouts.NAVIGATION)
    bodies = world.state.extra["route_bodies"]
    assert bodies, "the account route was never requested"
    for body in bodies + [world.page.content()]:
        for marker in ("address-card", "Thêm địa chỉ mới", "Thiết lập mặc định"):
            assert marker not in body, f"address UI leaked into the response: {marker}"


# ── Login / register forms ───────────────────────────────────────────────
@given("a registered buyer account")
def registered_buyer(world: World) -> None:
    username = fake.unique_username("acct_login")
    world.service_factory.auth.register(username, SETTINGS.seed_password, "buyer")
    world.state.extra["credentials"] = (username, SETTINGS.seed_password)


@given("the register page is open")
def register_page_open(world: World) -> None:
    _goto(world, "/register")
    expect(world.page.get_by_role("button", name="Đăng ký")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the visitor submits the login form with an empty username")
def submit_empty_username(world: World) -> None:
    page = LoginPage(world.page)
    _track_posts(world)
    page.wait_until_interactive(page.submit_button)
    page.submit_button.click()


@then("no request is sent and the username field shows help, is invalid and has focus")
def username_invalid_and_focused(world: World) -> None:
    page = LoginPage(world.page)
    expect(page.username_input).to_have_attribute("aria-invalid", "true", timeout=timeouts.DEFAULT)
    expect(page.username_input).to_be_focused()
    help_id = page.username_input.get_attribute("aria-describedby")
    assert help_id, "the username input is not described by its help text"
    expect(world.page.locator(f'[id="{help_id}"]')).to_have_text("Vui lòng nhập tên đăng nhập.")
    assert world.state.extra["action_posts"] == [], "a request was sent for an invalid form"


@when("the visitor signs in with that account and presses the submit button twice")
def sign_in_and_double_click(world: World) -> None:
    username, password = world.state.extra["credentials"]
    page = LoginPage(world.page)
    posts = _track_posts(world)
    page.wait_until_interactive(page.submit_button)
    page.username_input.fill(username)
    page.password_input.fill(password)
    world.state.extra["submit_width"] = page.submit_button.bounding_box()["width"]
    page.submit_button.click()
    world.state.extra["busy_probe"] = page.submit_button.get_attribute("aria-busy")
    page.submit_button.click(force=True, no_wait_after=True)
    world.state.extra["login_posts"] = posts


@then("the submit button shows a spinner, is busy and keeps its width")
def button_pending(world: World) -> None:
    button = LoginPage(world.page).submit_button
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(button.locator(".animate-spin")).to_have_count(1)
    width = button.bounding_box()["width"]
    assert abs(width - world.state.extra["submit_width"]) < 1, "button width changed while pending"


@then("exactly one login request was sent")
def one_login_request(world: World) -> None:
    world.page.wait_for_url(re.compile(r".*/(?!login).*"), timeout=timeouts.NAVIGATION)
    assert len(world.state.extra["login_posts"]) == 1, world.state.extra["login_posts"]


@when("the visitor submits a 2-character username and a 3-character password")
def submit_short_register(world: World) -> None:
    _track_posts(world)
    page = world.page
    page.locator("#username").wait_for()
    LoginPage(world.page).wait_until_interactive(page.get_by_role("button", name="Đăng ký"))
    page.locator("#username").fill("ab")
    page.locator("#password").fill("abc")
    page.get_by_role("button", name="Đăng ký").click()


@then("both fields show their minimum-length help and no request is sent")
def min_length_help(world: World) -> None:
    page = world.page
    expect(page.get_by_text("Tên đăng nhập tối thiểu 3 ký tự.")).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    expect(page.get_by_text("Mật khẩu tối thiểu 4 ký tự.")).to_be_visible()
    expect(page.locator("#username")).to_have_attribute("aria-invalid", "true")
    expect(page.locator("#password")).to_have_attribute("aria-invalid", "true")
    assert world.state.extra["action_posts"] == []
    assert "/register" in page.url


# ── Address book ─────────────────────────────────────────────────────────
@given("a buyer is signed in on the addresses page")
def buyer_on_addresses(world: World) -> None:
    _sign_in_buyer(world)
    _goto(world, "/account/addresses")
    expect(world.page.get_by_role("button", name="Thêm địa chỉ mới")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


def _fill_address(world: World, recipient: str) -> None:
    page = AddressesPage(world.page)
    page.wait_until_interactive(page.add_button)
    page.add_button.click()
    dialog = world.page.get_by_role("dialog")
    for name, value in [
        ("recipientName", recipient),
        ("phone", "0912345678"),
        ("street", "123 Đường Test"),
        ("ward", "P. Test"),
        ("district", "Quận 1"),
        ("city", "TP. HCM"),
    ]:
        dialog.locator(f'input[name="{name}"]').fill(value)
    world.state.extra["new_recipient"] = recipient


@when("the buyer submits a valid address in the add modal")
def submit_address(world: World) -> None:
    _fill_address(world, f"E2E {fake.vietnamese_name()}")
    world.state.extra["action_posts"] = _track_posts(world)
    world.page.get_by_role("dialog").get_by_role("button", name="Thêm mới").click()


@when("the buyer submits a valid address in the add modal and clicks the submit button twice")
def submit_address_twice(world: World) -> None:
    _fill_address(world, f"E2E {fake.vietnamese_name()}")
    _track_posts(world)
    button = world.page.get_by_role("dialog").get_by_role("button", name="Thêm mới")
    button.click()
    button.click(force=True, no_wait_after=True)


@then("the submit button is pending during the request")
def address_button_pending(world: World) -> None:
    button = world.page.get_by_role("dialog").get_by_role("button", name="Thêm mới")
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(button).to_be_disabled()


@then("the modal closes, a success toast appears and the new address card is listed")
def address_added(world: World) -> None:
    expect(world.page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.NAVIGATION)
    expect(_toast(world, "Đã thêm địa chỉ thành công.")).to_be_visible(timeout=timeouts.DEFAULT)
    page = AddressesPage(world.page)
    expect(page.address_card(world.state.extra["new_recipient"])).to_have_count(
        1, timeout=timeouts.NAVIGATION
    )


@then("exactly one mutation request was sent and one address was created")
def one_mutation(world: World) -> None:
    page = AddressesPage(world.page)
    expect(page.address_cards).to_have_count(1, timeout=timeouts.NAVIGATION)
    assert len(world.state.extra["action_posts"]) == 1, world.state.extra["action_posts"]
    listed = world.service_factory.address.list_addresses().get("addresses", [])
    assert len(listed) == 1, listed


@when("the second address is deleted behind the page's back")
def delete_second_behind_back(world: World) -> None:
    _, second = world.state.extra["address_names"]
    svc: AddressService = world.service_factory.address
    for item in svc.list_addresses().get("addresses", []):
        if item.get("recipientName") == second:
            world.state.extra["second_id"] = item["id"]
            svc.post("/platform.identity.v1.AddressService/DeleteAddress", {"id": item["id"]})
            return
    raise AssertionError(f"second address {second!r} not found via the API")


@when("the buyer sets the second address as default")
def set_second_default(world: World) -> None:
    _, second = world.state.extra["address_names"]
    page = AddressesPage(world.page)
    world.state.extra["action_posts"] = _track_posts(world)
    world.state.extra["errors"] = []
    world.page.on("pageerror", lambda e: world.state.extra["errors"].append(str(e)))
    button = page.address_card(second).get_by_role("button", name="Thiết lập mặc định")
    page.wait_until_interactive(button)
    button.click()
    world.state.extra["set_default_button"] = button


@then("an error toast is shown and the first address keeps its default tag")
def error_toast_keeps_default(world: World) -> None:
    first, _ = world.state.extra["address_names"]
    expect(world.page.get_by_role("alert").filter(has_text=re.compile(r"\S"))).not_to_have_count(
        0, timeout=timeouts.DEFAULT
    )
    card = AddressesPage(world.page).address_card(first)
    expect(card).to_contain_text("Mặc định")


@then("the set-default button is enabled again")
def set_default_enabled(world: World) -> None:
    expect(world.state.extra["set_default_button"]).to_be_enabled(timeout=timeouts.DEFAULT)


@then("the action resolved without throwing and a readable error is shown, not the error page")
def action_resolved(world: World) -> None:
    alerts = world.page.get_by_role("alert").filter(has_text=re.compile(r"\S"))
    expect(alerts.first).to_be_visible(timeout=timeouts.DEFAULT)
    text = alerts.first.inner_text()
    assert re.search(r"[A-Za-zÀ-ỹ]{3,}", text), f"error toast is not readable text: {text!r}"
    assert "Đã có lỗi xảy ra" not in world.page.content(), "the route error boundary took over"
    assert world.state.extra["errors"] == [], world.state.extra["errors"]
    assert len(world.state.extra["action_posts"]) == 1


@then(parsers.parse('an Empty block says "{text}" with the action "{action}"'))
def address_empty(world: World, text: str, action: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.DEFAULT)
    expect(world.page.get_by_role("button", name=action)).to_be_visible()


# ── Security ─────────────────────────────────────────────────────────────
@then(parsers.parse('the history table shows Empty "{text}"'))
def history_empty(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)


# ── KYC ──────────────────────────────────────────────────────────────────
def _status_tag(world: World):  # noqa: ANN202
    """The Tag in the "Trạng thái" row of the Descriptions block."""
    return world.page.locator("main dd, main [data-testid=descriptions-value]").first


@when(parsers.parse('the buyer enters a reference and presses "{label}"'))
def submit_kyc(world: World, label: str) -> None:
    page = VerificationPage(world.page)
    page.wait_until_interactive(page.doc_ref_input)
    page.doc_ref_input.fill("E2E-KYC-0001")
    page.submit_button.click()


@then("a success toast appears, the field is cleared and the status tag reads as pending")
def kyc_submitted(world: World) -> None:
    page = VerificationPage(world.page)
    expect(_toast(world, "Đã gửi hồ sơ xác minh.")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.doc_ref_input).to_have_value("")
    expect(world.page.get_by_text(re.compile(r"chờ duyệt", re.I)).first).to_be_visible()


@then("the status tag contains text that states the status")
def status_has_text(world: World) -> None:
    tag = world.page.get_by_text(re.compile(r"chờ duyệt|Đã xác minh|Bị từ chối|Chưa", re.I)).first
    expect(tag).to_be_visible(timeout=timeouts.NAVIGATION)
    assert tag.inner_text().strip(), "status tag has no text"


def _tag_colours(page: Page) -> dict[str, str]:
    return page.evaluate("""() => {
          const tag = [...document.querySelectorAll('main span')]
            .find((s) => /chờ duyệt|Đã xác minh|Bị từ chối/i.test(s.textContent || '')
                         && s.children.length === 0);
          const cs = getComputedStyle(tag);
          const root = getComputedStyle(document.documentElement);
          return {text: tag.textContent.trim(), colour: cs.color, background: cs.backgroundColor,
                  border: cs.borderTopColor, cls: tag.className,
                  danger: root.getPropertyValue('--color-danger'),
                  success: root.getPropertyValue('--color-success'),
                  promo: root.getPropertyValue('--color-promo'),
                  brand: root.getPropertyValue('--color-action-primary')};
        }""")


def _admin_review(world: World, submission_id: str, decision: str) -> None:
    admin = AuthService().login("admin", SETTINGS.seed_password)
    resp = VerificationService(token=admin).review_kyc_response(submission_id, decision)
    assert resp.status_code == 200, (resp.status_code, resp.text)


@given("an admin has verified one buyer and rejected another")
def admin_reviews(world: World) -> None:
    outcomes: dict[str, dict[str, str]] = {}
    admin_user = get_admin(world)
    for decision, key in (("approve", "verified"), ("reject", "rejected")):
        buyer_name = fake.unique_username("kyc")
        token = AuthService().register(buyer_name, SETTINGS.seed_password, "buyer")
        sub = VerificationService(token=token).submit_kyc("DOC_TYPE_ID_CARD", "E2E-REF")["id"]
        resp = VerificationService(token=admin_user).review_kyc_response(sub, decision)
        assert resp.status_code == 200, (decision, resp.status_code, resp.text)
        outcomes[key] = {"token": token}
    pending = AuthService().register(fake.unique_username("kyc"), SETTINGS.seed_password, "buyer")
    VerificationService(token=pending).submit_kyc("DOC_TYPE_ID_CARD", "E2E-REF")
    outcomes["pending"] = {"token": pending}
    world.state.extra["kyc_outcomes"] = outcomes


def get_admin(world: World) -> str:
    from src.utils import get_test_data_manager

    admin = get_test_data_manager().get_user_by_role("admin")
    return AuthService().login(admin.username, admin.password)


@then(
    "the pending, verified and rejected status tags use the promo, success and danger tokens "
    "and not the brand colour"
)
def status_tokens(world: World) -> None:
    expected = {"pending": "promo", "verified": "success", "rejected": "danger"}
    for key, token in expected.items():
        world.context.clear_cookies()
        world.context.add_cookies(
            [
                {
                    "name": "session",
                    "value": world.state.extra["kyc_outcomes"][key]["token"],
                    "url": BASE,
                }
            ]
        )
        _goto(world, "/account/verification")
        expect(world.page.get_by_role("button", name="Gửi hồ sơ xác minh")).to_be_visible(
            timeout=timeouts.NAVIGATION
        )
        info = _tag_colours(world.page)
        assert (
            token in info["cls"]
        ), f"{key}: tag classes {info['cls']!r} do not use the {token} token"
        assert "action-primary" not in info["cls"] and "primary-" not in info["cls"], info["cls"]


# ── Referral ─────────────────────────────────────────────────────────────
@given("a signed-in buyer on the referral page")
def buyer_on_referral(world: World) -> None:
    _sign_in_buyer(world)
    _goto(world, "/account/referral")
    expect(world.page.get_by_test_id("referral-code")).to_be_visible(timeout=timeouts.NAVIGATION)


@when(parsers.parse('the buyer redeems the code "{code}"'))
def redeem_code(world: World, code: str) -> None:
    field = world.page.get_by_placeholder("Nhập mã của bạn bè")
    LoginPage(world.page).wait_until_interactive(field)
    field.fill(code)
    world.page.get_by_role("button", name="Nhập mã").click()


@then(
    "an error toast is shown and the redeem field shows the server message as help and is invalid"
)
def redeem_rejected(world: World) -> None:
    field = world.page.get_by_placeholder("Nhập mã của bạn bè")
    expect(field).to_have_attribute("aria-invalid", "true", timeout=timeouts.NAVIGATION)
    help_id = field.get_attribute("aria-describedby")
    assert help_id, "redeem field has no help text"
    help_text = world.page.locator(f'[id="{help_id}"]').inner_text().strip()
    assert help_text, "help text is empty"
    toast = world.page.get_by_role("alert").filter(has_text=help_text)
    expect(toast.first).to_be_visible(timeout=timeouts.DEFAULT)


@then(parsers.parse('the rewards area shows Empty "{text}"'))
def rewards_empty(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.DEFAULT)


# ── Following ────────────────────────────────────────────────────────────
@given("a signed-in buyer")
def signed_in_buyer(world: World) -> None:
    _sign_in_buyer(world)


@when(parsers.parse('the buyer opens "{route}" with JavaScript disabled'))
def open_without_js(world: World, route: str) -> None:
    cookies = world.context.cookies()
    nojs = world.page.context.browser.new_context(java_script_enabled=False)
    world.add_cleanup(nojs.close)
    nojs.add_cookies([c for c in cookies if c["name"] == "session"])
    page = nojs.new_page()
    page.goto(f"{BASE}{route}", wait_until="domcontentloaded")
    world.state.extra["nojs_page"] = page


@then(parsers.parse('the "{label}" tab is selected on first render'))
def tab_selected(world: World, label: str) -> None:
    page: Page = world.state.extra["nojs_page"]
    tab = page.get_by_role("navigation", name="Tabs").get_by_role("link", name=re.compile(label))
    # Link tabs carry their selection as aria-current (the spec's aria-selected is the
    # role=tab variant; a link tab is announced as the current page).
    expect(tab.first).to_have_attribute("aria-current", "page", timeout=timeouts.NAVIGATION)


def _seller_with_name(world: World, name: str | None) -> str:
    username = fake.unique_username("follow_seller")
    token = AuthService().register(username, SETTINGS.seed_password, "seller")
    if name:
        ListingService(token=token).upsert_storefront(f"e2e-shop-{username[-8:]}", name)
    return token


def _buyer_follows(world: World, seller_token: str) -> str:
    _sign_in_buyer(world)
    seller_id = _principal_id(seller_token)
    _api(world.state.current_user.token).post(
        f"{_ENGAGEMENT}/FollowSeller", {"sellerId": seller_id}
    )
    world.state.extra["followed_shop_id"] = seller_id
    return seller_id


@given(parsers.parse('a buyer who follows a shop named "{name}"'))
def buyer_follows_named(world: World, name: str) -> None:
    _buyer_follows(world, _seller_with_name(world, name))


@given("a buyer who follows a shop with no display name")
def buyer_follows_unnamed(world: World) -> None:
    _buyer_follows(world, _seller_with_name(world, None))


@when("the buyer opens the following page")
def open_following(world: World) -> None:
    _goto(world, "/account/following")
    expect(_shell(world).menu).to_be_visible(timeout=timeouts.NAVIGATION)


@then(parsers.parse('the shop card shows "{name}" and not "Shop #"'))
def shop_card_named(world: World, name: str) -> None:
    card = world.page.locator(f'a[href="/shop/{world.state.extra["followed_shop_id"]}"]')
    expect(card.first).to_contain_text(name, timeout=timeouts.NAVIGATION)
    assert "Shop #" not in card.first.inner_text()


@then('the shop card shows "Shop #" and the first 6 characters of the shop id')
def shop_card_fallback(world: World) -> None:
    shop_id = world.state.extra["followed_shop_id"]
    card = world.page.locator(f'a[href="/shop/{shop_id}"]')
    expect(card.first).to_contain_text(f"Shop #{shop_id[:6]}", timeout=timeouts.NAVIGATION)


@then(
    parsers.parse('the shops tab shows Empty "{text}" with a "{action}" action linking to "{href}"')
)
def following_empty(world: World, text: str, action: str, href: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("link", name=action)).to_have_attribute("href", href)


# ── Favorites ────────────────────────────────────────────────────────────
@given("a signed-in buyer on the favorites page")
def buyer_on_favorites(world: World) -> None:
    _sign_in_buyer(world)
    _goto(world, "/favorites")
    expect(FavoritesPage(world.page).collection_name_input).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


def _published_listing(world: World) -> str:
    seller = AuthService().register(
        fake.unique_username("fav_seller"), SETTINGS.seed_password, "seller"
    )
    listing = Listing(
        title=f"[E2E][Fav] {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=250_000,
        stock=5,
        status="published",
        description="Seed for favorites e2e.",
    )
    return ListingService(token=seller).create_listing(listing)


@given("the buyer has a favourite listing")
def buyer_has_favourite(world: World) -> None:
    listing_id = _published_listing(world)
    _api(world.state.current_user.token).post(
        f"{_ENGAGEMENT}/AddFavorite", {"listingId": listing_id}
    )
    world.state.extra["fav_listing"] = listing_id


@given(parsers.parse('the buyer has a collection "{name}" holding one favourite'))
def buyer_has_collection(world: World, name: str) -> None:
    listing_id = _published_listing(world)
    api = _api(world.state.current_user.token)
    api.post(f"{_ENGAGEMENT}/AddFavorite", {"listingId": listing_id})
    coll = api.post(f"{_ENGAGEMENT}/CreateCollection", {"name": name})
    cid = (coll.get("collection") or coll).get("id")
    assert cid, coll
    api.post(f"{_ENGAGEMENT}/AddToCollection", {"collectionId": cid, "listingId": listing_id})
    world.state.extra.update(collection_id=cid, collection_name=name, fav_listing=listing_id)
    world.page.reload(wait_until="domcontentloaded")


@when("the buyer selects that collection")
def select_collection(world: World) -> None:
    row = FavoritesPage(world.page).collection_row(world.state.extra["collection_name"])
    row.get_by_role("link").click()


@then('the URL is "/favorites?collection=<id>" and the header names the collection')
def collection_url(world: World) -> None:
    cid = world.state.extra["collection_id"]
    world.page.wait_for_url(
        re.compile(rf".*/favorites\?collection={cid}"), timeout=timeouts.NAVIGATION
    )
    expect(
        world.page.get_by_role("heading", name=re.compile(world.state.extra["collection_name"]))
    ).to_be_visible()


@then("reloading the page shows the same item")
def collection_reload(world: World) -> None:
    listing_id = world.state.extra["fav_listing"]
    world.page.reload(wait_until="domcontentloaded")
    expect(world.page.locator(f'a[href="/listing/{listing_id}"]').first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when(parsers.parse('the buyer creates a collection "{name}"'))
def create_collection(world: World, name: str) -> None:
    page = FavoritesPage(world.page)
    world.state.extra["collection_name"] = name
    page.wait_until_interactive(page.collection_name_input)
    page.collection_name_input.fill(name)
    page.create_collection_submit.click()


@then("the create button is pending and disabled")
def create_pending(world: World) -> None:
    button = FavoritesPage(world.page).create_collection_submit
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(button).to_be_disabled()


@then("a success toast appears and the new collection is listed")
def collection_created(world: World) -> None:
    expect(_toast(world, "Đã tạo bộ sưu tập")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(
        FavoritesPage(world.page).collection_row(world.state.extra["collection_name"])
    ).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the buyer opens the favorites page and clicks the favourite listing")
def click_favourite(world: World) -> None:
    listing_id = world.state.extra["fav_listing"]
    _goto(world, "/favorites")
    link = world.page.locator(f'a[href="/listing/{listing_id}"]').first
    expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
    link.scroll_into_view_if_needed()
    world.page.wait_for_timeout(600)  # IntersectionObserver threshold 0.3
    link.click()
    world.page.wait_for_url(re.compile(rf".*/listing/{listing_id}"), timeout=timeouts.NAVIGATION)


@then("a view_item_list impression and a select_item click are sent for that listing")
def favourite_events(world: World) -> None:
    listing_id = world.state.extra["fav_listing"]
    events = world.page.evaluate("() => (window.dataLayer || []).filter(e => e && e.event)")

    def of(name: str) -> list[dict]:
        return [
            e
            for e in events
            if e["event"] == name
            and any(
                i.get("item_id") == listing_id for i in (e.get("ecommerce") or {}).get("items", [])
            )
        ]

    assert len(of("view_item_list")) == 1, [e["event"] for e in events]
    clicks = of("select_item")
    assert len(clicks) == 1, [e["event"] for e in events]
    assert clicks[0]["ecommerce"]["items"][0].get("index") == 1


# ── Notifications ────────────────────────────────────────────────────────
@when('the buyer opens the "Đơn hàng" notification tab and reloads the page')
def open_order_tab_and_reload(world: World) -> None:
    NotificationsPage(world.page).tab("Đơn hàng").click()
    world.page.wait_for_url(re.compile(r".*tab=order"), timeout=timeouts.NAVIGATION)
    world.page.reload(wait_until="domcontentloaded")


@then(
    'the URL contains "tab=order", the "Đơn hàng" notification tab is selected and only order '
    "notifications are listed"
)
def order_tab_only(world: World) -> None:
    page = NotificationsPage(world.page)
    expect(world.page).to_have_url(re.compile(r".*tab=order"), timeout=timeouts.NAVIGATION)
    expect(page.tab("Đơn hàng")).to_have_attribute("aria-current", "page")
    others = world.page.locator('[data-testid="notification-item"]:not([data-type="order"])')
    assert others.count() == 0, "non-order notifications are listed on the order tab"


@then("no mark-all-read control is rendered")
def no_mark_all_read(world: World) -> None:
    expect(NotificationsPage(world.page).tabs).to_be_visible(timeout=timeouts.NAVIGATION)
    for pattern in (r"đánh dấu.*đã đọc", r"mark all", r"đọc tất cả"):
        assert world.page.get_by_text(re.compile(pattern, re.I)).count() == 0, pattern
        assert world.page.get_by_role("button", name=re.compile(pattern, re.I)).count() == 0


@then("each notification and alert-subscription row exposes its data-testid and a data-type")
def rows_expose_hooks(world: World) -> None:
    page = NotificationsPage(world.page)
    expect(page.notification_items.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.alert_subscriptions.first).to_be_visible()
    for rows in (page.notification_items, page.alert_subscriptions):
        for i in range(rows.count()):
            assert rows.nth(i).get_attribute("data-type"), f"row {i} has no data-type"


@when(parsers.parse('the buyer toggles a preference and presses "{label}"'))
def toggle_preference(world: World, label: str) -> None:
    box = world.page.get_by_role("checkbox").first
    LoginPage(world.page).wait_until_interactive(box)
    box.click()
    world.page.get_by_role("button", name=label).click()


@then("the save button is pending and then a success toast appears")
def preferences_saved(world: World) -> None:
    button = world.page.get_by_role("button", name="Lưu tùy chọn")
    expect(button).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(button).to_be_disabled()
    expect(_toast(world, "Đã lưu tùy chọn thông báo.")).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(button).to_be_enabled()


# ── Layout, CLS and tokens ───────────────────────────────────────────────
_ROUTES = [
    "/login",
    "/register",
    "/account/addresses",
    "/account/security",
    "/account/verification",
    "/account/referral",
    "/account/following",
    "/favorites",
    "/notifications",
]


@when(parsers.parse('the buyer loads "{route}" over a throttled network'))
def load_throttled(world: World, route: str) -> None:
    cdp = world.context.new_cdp_session(world.page)
    cdp.send("Network.enable")
    cdp.send(
        "Network.emulateNetworkConditions",
        {
            "offline": False,
            "latency": 300,
            "downloadThroughput": 200_000,
            "uploadThroughput": 100_000,
        },
    )
    cdp.send("Emulation.setCPUThrottlingRate", {"rate": 4})
    world.page.add_init_script("""window.__cls = 0;
        new PerformanceObserver((l) => { for (const e of l.getEntries())
          if (!e.hadRecentInput) window.__cls += e.value; })
          .observe({type: 'layout-shift', buffered: true});""")
    world.page.goto(f"{BASE}{route}", wait_until="load")
    expect(_shell(world).menu).to_be_visible(timeout=timeouts.NAVIGATION)
    world.page.wait_for_timeout(3000)


@then("the measured layout shift of the page is 0")
def cls_zero(world: World) -> None:
    cls = world.page.evaluate("() => window.__cls")
    assert cls == 0, f"cumulative layout shift is {cls}, expected 0"


@when("each of the nine account and auth routes is rendered at 375px wide")
def render_nine_routes(world: World) -> None:
    world.page.set_viewport_size(_PHONE)
    widths: dict[str, int] = {}
    for route in _ROUTES:
        _goto(world, route)
        world.page.wait_for_load_state("networkidle")
        widths[route] = world.page.evaluate("() => document.documentElement.scrollWidth")
    world.state.extra["scroll_widths"] = widths


@then("none of them is wider than 375px")
def none_wider(world: World) -> None:
    wide = {r: w for r, w in world.state.extra["scroll_widths"].items() if w > _PHONE["width"]}
    assert not wide, f"routes with horizontal page scroll at 375px: {wide}"


@when(parsers.parse('the buyer opens "{route}" at 1280px wide'))
def open_at_desktop(world: World, route: str) -> None:
    world.page.set_viewport_size(_DESKTOP)
    _goto(world, route)
    expect(_shell(world).menu).to_be_visible(timeout=timeouts.NAVIGATION)


@then("the 240px menu column is visible to the left of the content")
def desktop_menu_left(world: World) -> None:
    menu = _shell(world).menu.bounding_box()
    content = world.page.get_by_role("heading", level=1).bounding_box()
    assert menu and content
    assert 230 <= menu["width"] <= 250, f"menu column is {menu['width']}px wide, expected 240"
    cards = world.page.get_by_role("heading", name="Sổ địa chỉ")
    expect(cards.first).to_be_visible()
    body = world.page.get_by_role("button", name="Thêm địa chỉ mới").bounding_box()
    assert body and menu["x"] + menu["width"] <= body["x"], (menu, body)


@when("the token lint runs on the account, auth, favorites and notifications files")
def lint_account_files(world: World) -> None:
    prefixes = [
        "src/app/(shop)/account",
        "src/app/(shop)/login",
        "src/app/(shop)/register",
        "src/app/(shop)/favorites",
        "src/app/(shop)/notifications",
        "src/features/account",
        "src/features/auth",
        "src/features/favorites",
        "src/features/notifications",
    ]
    world.state.extra["lint"] = _run(["node", "scripts/check-tokens.mjs", *prefixes], FRONTEND)


@then("the token lint reports no violation in those files")
def lint_clean_for_files(world: World) -> None:
    result: subprocess.CompletedProcess[str] = world.state.extra["lint"]
    assert result.returncode == 0 and "0 violation(s)" in result.stdout, result.stdout[-1500:]


@then("the whole document does not scroll sideways")
def document_does_not_scroll(world: World) -> None:
    width = world.page.evaluate("() => document.documentElement.scrollWidth")
    assert width <= _PHONE["width"], f"document is {width}px wide at 375px"


# ── Revoking a session ───────────────────────────────────────────────────
@given("a buyer signed in on this device and on another device")
def buyer_on_two_devices(world: World) -> None:
    from tests.e2e.flows import login_with_headers

    username = fake.unique_username("acct_revoke")
    AuthService().register(username, SETTINGS.seed_password, "buyer")
    login_with_headers(username, SETTINGS.seed_password, {"User-Agent": "E2E-Other-Device"})
    login_via_api(world, User(username=username, password=SETTINGS.seed_password, role="buyer"))
    _goto(world, "/account/security")
    expect(world.page.get_by_role("button", name="Thu hồi").first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when(parsers.parse('the buyer confirms "{label}" on the other device\'s session'))
def confirm_revoke_other(world: World, label: str) -> None:
    row = world.page.get_by_role("row").filter(has_text="E2E-Other-Device")
    button = row.get_by_role("button", name=label)
    LoginPage(world.page).wait_until_interactive(button)
    button.click()
    dialog = world.page.get_by_role("dialog", name="Thu hồi phiên đăng nhập?")
    expect(dialog).to_be_visible()
    world.state.extra["revoke_confirm"] = dialog.get_by_role("button", name="Thu hồi phiên")
    world.state.extra["revoke_confirm"].click()


@then(
    'the confirm button is pending, a success toast appears and that row shows "Đã thu hồi" '
    "without a Revoke button"
)
def revoked(world: World) -> None:
    confirm = world.state.extra["revoke_confirm"]
    expect(confirm).to_have_attribute("aria-busy", "true", timeout=timeouts.SHORT)
    expect(_toast(world, "Đã thu hồi phiên đăng nhập.")).to_be_visible(timeout=timeouts.NAVIGATION)
    row = (
        world.page.get_by_role("row")
        .filter(has_text="E2E-Other-Device")
        .filter(has_text="Đã thu hồi")
    )
    expect(row).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(row.get_by_role("button", name="Thu hồi")).to_have_count(0)


@then(parsers.parse('the notifications list area shows Empty "{text}"'))
def notifications_empty(world: World, text: str) -> None:
    expect(world.page.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    assert NotificationsPage(world.page).notification_items.count() == 0
