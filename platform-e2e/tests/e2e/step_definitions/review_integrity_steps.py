"""Steps for frontend/review_integrity_ui.feature (frontend-review-integrity).

Listings and sellers come from the PDP seeding helpers; reviews are created through completed
orders (`review_order_support`); the legacy unverified row is inserted by the engagement fixture.
The only review entry point in the UI is "Đánh giá" on a completed order at /account/orders.
"""

from __future__ import annotations

from playwright.sync_api import Locator, expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService
from src.constants import timeouts
from src.models import User
from src.utils import data as fake
from src.utils import get_test_data_manager
from tests.e2e.flows import login_via_api
from tests.e2e.step_definitions.d1_pdp_steps import (
    BASE,
    _listing_id,
    _open,
    _seed,
    _seed_verified_review,
    _sign_in,
    _x,
)
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support import review_order_support as ros
from tests.e2e.support.world import World

SETTINGS = get_settings()
_LEGACY = "Review cũ 1 sao"


def _seller(world: World) -> User:
    return User("ri-seller", SETTINGS.seed_password, "seller", token=_x(world)["pdp_seller_token"])


def _reviews(world: World) -> Locator:
    return world.page.locator("#reviews")


def _items(world: World) -> Locator:
    return world.page.get_by_test_id("review-item")


def _open_listing(world: World) -> None:
    world.page.goto(f"{BASE}/listing/{_listing_id(world)}", wait_until="domcontentloaded")
    expect(world.page.locator("h1").first).to_be_visible(timeout=timeouts.NAVIGATION)


def _as_guest(world: World) -> None:
    world.context.clear_cookies()


# ── Givens ───────────────────────────────────────────────────────────────
@given("a listing with one verified review and a signed-in buyer on its page")
def one_review_buyer(world: World) -> None:
    _seed(world)
    _seed_verified_review(world, 4, "Review 4 sao")
    _sign_in(world)
    _open(world)


@given("a listing and a signed-in buyer with a completed order for it")
def buyer_completed_order(world: World) -> None:
    _seed(world)
    _sign_in(world)
    buyer = world.state.current_user
    _x(world)["ri_order_id"] = ros.completed_order_for(
        world, buyer, _seller(world), _listing_id(world)
    )


@given("the buyer already reviewed that order item through the API")
def buyer_already_reviewed(world: World) -> None:
    buyer = world.state.current_user
    ros.ok(
        ros.create_review(
            buyer.token, _listing_id(world), 5, "Đánh giá lần một", _x(world)["ri_order_id"]
        )
    )


@given(
    "a listing with verified reviews of 5 and 3 stars and one legacy unverified review of 1 star"
)
def verified_and_legacy(world: World) -> None:
    _seed(world)
    _seed_verified_review(world, 5, "Review 5 sao")
    _seed_verified_review(world, 3, "Review 3 sao")
    legacy_author = AuthService().register(
        fake.unique_username("ri_legacy"), SETTINGS.seed_password, "buyer"
    )
    ros.seed_legacy_unverified_review(
        _listing_id(world),
        _x(world)["pdp_seller_id"],
        _principal_id(legacy_author),
        1,
        _LEGACY,
    )


@given("the seeded admin is signed in on that listing page")
@when("the seeded admin opens that listing page")
def admin_on_listing(world: World) -> None:
    admin = get_test_data_manager().get_user_by_role("admin")
    login_via_api(world, admin)
    _open_listing(world)
    expect(_items(world).first).to_be_visible(timeout=timeouts.NAVIGATION)


# ── PDP hint ─────────────────────────────────────────────────────────────
@then(
    "the reviews section has no write-review button and shows the hint linking to the orders page"
)
def no_write_button_hint(world: World) -> None:
    section = _reviews(world)
    expect(_items(world).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(section.get_by_test_id("write-review")).to_have_count(0)
    expect(section.get_by_role("button", name="Viết đánh giá")).to_have_count(0)
    expect(section.get_by_text("Để đánh giá, hãy vào")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(section.locator('a[href="/account/orders"]')).to_be_visible()


@when("the buyer follows the orders hint")
def follow_hint(world: World) -> None:
    link = _reviews(world).locator('a[href="/account/orders"]').first
    expect(link).to_be_visible(timeout=timeouts.DEFAULT)
    link.click()


@then("the browser is on the orders page")
def on_orders_page(world: World) -> None:
    world.page.wait_for_url("**/account/orders", timeout=timeouts.NAVIGATION)


@when("a guest follows the orders hint on the same listing")
def guest_follows_hint(world: World) -> None:
    _as_guest(world)
    _open_listing(world)
    link = _reviews(world).locator('a[href="/account/orders"]').first
    expect(link).to_be_visible(timeout=timeouts.DEFAULT)
    link.click()


@then("the guest is on the login page and no review dialog is open")
def guest_on_login(world: World) -> None:
    world.page.wait_for_url("**/login**", timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("dialog", name="Đánh giá sản phẩm")).to_have_count(0)


# ── Reviewing from the orders page ───────────────────────────────────────
@when(
    parsers.parse('the buyer reviews the order item with {stars:d} stars and the comment "{text}"')
)
def review_order_item(world: World, stars: int, text: str) -> None:
    world.page.goto(f"{BASE}/account/orders", wait_until="domcontentloaded")
    button = world.page.get_by_role("button", name="Đánh giá").first
    expect(button).to_be_visible(timeout=timeouts.NAVIGATION)
    button.click()
    dialog = world.page.get_by_role("dialog", name="Đánh giá sản phẩm")
    expect(dialog).to_be_visible()
    dialog.locator(f'label:has(input[aria-label="{stars} sao"])').click()
    dialog.get_by_label("Nhận xét chi tiết").fill(text)
    _x(world)["ri_dialog"] = dialog
    _x(world)["ri_comment"] = text
    dialog.get_by_role("button", name="Hoàn thành").click()


@then(parsers.parse('a success toast "{text}" shows and the review modal closes'))
def review_success(world: World, text: str) -> None:
    toast = world.page.get_by_role("status").filter(has_text=text)
    expect(toast.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.NAVIGATION)


@then(
    "the listing page shows that review with the verified purchase badge and no write-review button"
)
def review_listed_verified(world: World) -> None:
    _open_listing(world)
    item = _items(world).filter(has_text=_x(world)["ri_comment"])
    expect(item).to_have_count(1, timeout=timeouts.NAVIGATION)
    expect(item.get_by_test_id("verified-purchase")).to_have_text("✓ Đã mua hàng")
    expect(item.get_by_test_id("unverified-purchase")).to_have_count(0)
    expect(_reviews(world).get_by_test_id("write-review")).to_have_count(0)


@then(parsers.parse('the review modal stays open showing "{text}"'))
def modal_error(world: World, text: str) -> None:
    dialog = _x(world)["ri_dialog"]
    expect(dialog.get_by_text(text)).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(dialog).to_be_visible()


@then("only one review by the buyer is stored for the listing")
def one_review_stored(world: World) -> None:
    buyer_id = _principal_id(world.state.current_user.token)
    mine = [
        r
        for r in ros.list_reviews(_listing_id(world)).get("reviews", [])
        if r.get("userId") == buyer_id
    ]
    assert len(mine) == 1, f"expected one stored review by the buyer, got {mine}"


# ── Legacy label and summary ─────────────────────────────────────────────
@when("a guest opens that listing page")
def guest_opens(world: World) -> None:
    _as_guest(world)
    _open_listing(world)


@then(parsers.parse('the summary shows "{avg}" and "{label}"'))
def summary_shows(world: World, avg: str, label: str) -> None:
    section = _reviews(world)
    expect(section).to_contain_text(avg, timeout=timeouts.NAVIGATION)
    expect(section).to_contain_text(label, timeout=timeouts.DEFAULT)


@then(parsers.parse('the list shows {count:d} reviews and exactly one carries "{label}"'))
def list_with_unverified(world: World, count: int, label: str) -> None:
    expect(_items(world)).to_have_count(count, timeout=timeouts.NAVIGATION)
    expect(_items(world).filter(has_text=label)).to_have_count(1)
    expect(
        _items(world).filter(has_text=_LEGACY).get_by_test_id("unverified-purchase")
    ).to_have_count(1)
    expect(_items(world).get_by_test_id("verified-purchase")).to_have_count(count - 1)


# ── Admin hide ───────────────────────────────────────────────────────────
@then("every review shows a hide control")
def every_review_hide(world: World) -> None:
    count = _items(world).count()
    assert count >= 1, "no review rendered"
    expect(world.page.get_by_test_id("hide-review")).to_have_count(count)


@when(parsers.parse('the admin hides the review "{comment}" with the reason "{reason}"'))
def admin_hides(world: World, comment: str, reason: str) -> None:
    item = _items(world).filter(has_text=comment)
    expect(item).to_have_count(1)
    _x(world)["ri_hidden"] = comment
    item.get_by_test_id("hide-review").click()
    dialog = world.page.get_by_role("dialog", name="Ẩn đánh giá")
    expect(dialog).to_be_visible()
    _x(world)["ri_dialog"] = dialog
    dialog.get_by_role("textbox").fill(reason)
    dialog.get_by_role("button", name="Ẩn đánh giá", exact=True).click()


@then(
    'the dialog closes, the toast "Đã ẩn đánh giá." shows and the review is gone without a reload'
)
def hide_done(world: World) -> None:
    toast = world.page.get_by_role("status").filter(has_text="Đã ẩn đánh giá.")
    expect(toast.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("dialog")).to_have_count(0, timeout=timeouts.NAVIGATION)
    expect(_items(world).filter(has_text=_x(world)["ri_hidden"])).to_have_count(
        0, timeout=timeouts.NAVIGATION
    )
    expect(_items(world)).to_have_count(2)


@then("the hidden review is no longer returned by the reviews API")
def hidden_not_in_api(world: World) -> None:
    comments = [r.get("comment") for r in ros.list_reviews(_listing_id(world)).get("reviews", [])]
    assert _x(world)["ri_hidden"] not in comments, comments
    assert len(comments) == 2, comments


@then("the hidden review is absent for the guest and the other reviews remain")
def hidden_absent_guest(world: World) -> None:
    expect(_items(world)).to_have_count(2, timeout=timeouts.NAVIGATION)
    expect(_items(world).filter(has_text=_x(world)["ri_hidden"])).to_have_count(0)
    expect(_items(world).filter(has_text="Review 5 sao")).to_have_count(1)


# ── Non-admins ───────────────────────────────────────────────────────────
@then("no hide control is in the page")
def no_hide_control(world: World) -> None:
    expect(_items(world).first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_test_id("hide-review")).to_have_count(0)
    expect(world.page.get_by_role("button", name="Ẩn", exact=True)).to_have_count(0)
