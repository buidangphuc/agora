"""Steps for frontend/uif_product_detail.feature (ui-phase-product-detail failure paths)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.constants import timeouts
from tests.e2e.step_definitions.review_ratings_steps import _create_review, _register_buyer
from tests.e2e.support import d2_support as d2
from tests.e2e.support.uif_support import ALERTS
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


def _url(world: World) -> str:
    return f"{world.settings.base_url.rstrip('/')}/listing/{_x(world)['uif_shop']['listing_id']}"


@given("a listing with reviews exists for the failure paths")
def listing_with_reviews(world: World) -> None:
    shop = d2.seed_shop(price=100_000, stock=20)
    _x(world)["uif_shop"] = shop
    for i, stars in enumerate((5, 4)):
        author = _register_buyer(world, f"uifpdp{i}")
        _create_review(world, author.token, shop["listing_id"], stars, f"Rất tốt lần {i} {stars}")


@when("a guest opens the listing page without waiting for its streamed sections")
def open_without_waiting(world: World) -> None:
    world.page.goto(_url(world), wait_until="commit")


@when("a guest opens the listing page")
def open_listing(world: World) -> None:
    world.page.goto(_url(world), wait_until="domcontentloaded")


@when("the guest reloads the listing page")
def reload_listing(world: World) -> None:
    world.page.goto(_url(world), wait_until="domcontentloaded")


def _reviews_section(world: World):
    return world.page.locator("section", has=world.page.locator("#reviews-heading"))


def _header_visible(world: World) -> None:
    page = world.page
    shop = _x(world)["uif_shop"]
    expect(page.locator("h1").first).to_have_text(shop["title"], timeout=timeouts.NAVIGATION)
    expect(page.get_by_test_id("pdp-price")).to_be_visible(timeout=timeouts.DEFAULT)
    expect(page.get_by_role("button", name="Thêm vào giỏ").first).to_be_visible(
        timeout=timeouts.DEFAULT
    )


@then("the listing header, price and actions are visible while the AI summary shows a skeleton")
def header_before_summary(world: World) -> None:
    _header_visible(world)
    reviews = _reviews_section(world)
    expect(reviews.locator('[aria-busy="true"]').first).to_be_visible(timeout=timeouts.DEFAULT)
    expect(world.page.get_by_test_id("ai-review-summary")).to_have_count(0)


@then("the AI review summary card is shown in place of the skeleton")
def summary_shown(world: World) -> None:
    expect(world.page.get_by_test_id("ai-review-summary")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(_reviews_section(world).locator('[aria-busy="true"]')).to_have_count(0)


@then(
    "the reviews section renders with its reviews and no AI summary, error text or leftover skeleton"
)
def reviews_without_ai(world: World) -> None:
    page = world.page
    _header_visible(world)
    reviews = _reviews_section(world)
    expect(reviews.get_by_test_id("review-item")).to_have_count(2, timeout=timeouts.NAVIGATION)
    expect(page.get_by_test_id("ai-review-summary")).to_have_count(0)
    expect(page.get_by_text("Tóm tắt đánh giá bằng AI")).to_have_count(0)
    expect(reviews.locator('[aria-busy="true"]')).to_have_count(0)
    expect(page.locator(ALERTS)).to_have_count(0)


@then(
    "the listing header, price and actions are visible while the recommendations show one skeleton row"
)
def header_before_recs(world: World) -> None:
    _header_visible(world)
    rows = world.page.locator('section[aria-busy="true"]')
    expect(rows).to_have_count(1, timeout=timeouts.DEFAULT)
    expect(rows.first.locator('[data-variant="card"]')).to_have_count(6)
    expect(world.page.get_by_text("Gợi ý cho bạn", exact=True)).to_have_count(0)


@then('the page has no "Gợi ý cho bạn" row, no recommendations skeleton and no error state')
def recs_hidden(world: World) -> None:
    page = world.page
    _header_visible(world)
    expect(_reviews_section(world).get_by_test_id("review-item")).to_have_count(
        2, timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_text("Gợi ý cho bạn", exact=True)).to_have_count(0)
    expect(page.locator('section[aria-busy="true"]')).to_have_count(0)
    expect(page.locator(ALERTS)).to_have_count(0)


@then(parsers.parse('an error alert "{title}" with a "{label}" button is shown'))
def error_alert(world: World, title: str, label: str) -> None:
    alert = world.page.get_by_role("alert").filter(has_text=title)
    expect(alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(alert.get_by_role("button", name=label)).to_be_visible()


@when(parsers.parse('the guest clicks "{label}"'))
def click_label(world: World, label: str) -> None:
    world.page.get_by_role("button", name=label).click()
    world.page.wait_for_timeout(6000)


@then("the listing page renders its title again")
def title_again(world: World) -> None:
    shop = _x(world)["uif_shop"]
    expect(world.page.locator("h1").first).to_have_text(shop["title"], timeout=timeouts.NAVIGATION)
    expect(world.page.get_by_role("alert").filter(has_text="Không thể tải sản phẩm")).to_have_count(
        0
    )
