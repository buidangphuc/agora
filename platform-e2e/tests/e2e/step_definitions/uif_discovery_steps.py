"""Steps for frontend/uif_discovery.feature (ui-phase-discovery failure paths)."""

from __future__ import annotations

import uuid

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from src.constants import timeouts
from src.pages.saved_searches_page import SavedSearchesPage
from tests.e2e.support import d2_support as d2
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.uif_support import ALERTS
from tests.e2e.support.world import World


def _x(world: World) -> dict:
    return world.state.extra


def _base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


@given("a published listing exists for the discovery failure paths")
def published_listing(world: World) -> None:
    _x(world)["uif_shop"] = d2.seed_shop(price=100_000, stock=20)


@when("a visitor opens the home page")
def visitor_home(world: World) -> None:
    world.page.goto(f"{_base(world)}/", wait_until="domcontentloaded")


@then('the feed area shows an error alert with a retry link to "/" instead of an empty grid')
def feed_failure(world: World) -> None:
    page = world.page
    feed = page.locator("section", has=page.locator("#home-feed-title"))
    expect(feed.get_by_role("heading", name="Gợi ý hôm nay")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    alert = feed.locator(ALERTS)
    expect(alert).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(alert).to_contain_text("Không tải được danh sách sản phẩm")
    expect(alert.get_by_role("link", name="Thử lại")).to_have_attribute("href", "/")
    expect(feed.locator('a[href^="/listing/"]')).to_have_count(0)
    expect(feed.get_by_text("Hiện chưa có sản phẩm nào được đăng bán.")).to_have_count(0)


@given('a buyer has viewed a listing and sees the "Vừa xem" block on the home page')
def buyer_viewed(world: World) -> None:
    shop = d2.seed_shop(price=100_000, stock=20)
    _x(world)["uif_shop"] = shop
    buyer = d2.seed_buyer(world)
    resp = pe.post_json(
        "/platform.engagement.v1.EngagementService/RecordView",
        {"listingId": shop["listing_id"]},
        buyer.token,
    )
    assert resp.status_code == 200, resp.text
    world.page.goto(f"{_base(world)}/", wait_until="domcontentloaded")
    expect(world.page.get_by_role("heading", name="Vừa xem")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the buyer opens the home page again")
def buyer_home_again(world: World) -> None:
    world.page.goto(f"{_base(world)}/", wait_until="domcontentloaded")


@then(
    'neither the "Vừa xem" nor the "Gợi ý cho bạn" block is rendered and the feed and the hero still are'
)
def blocks_hidden(world: World) -> None:
    page = world.page
    expect(page.get_by_role("heading", name="Gợi ý hôm nay")).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(page.get_by_role("link", name="Mua ngay")).to_be_visible()
    expect(page.get_by_role("heading", name="Vừa xem")).to_have_count(0)
    expect(page.get_by_text("Gợi ý cho bạn", exact=True)).to_have_count(0)
    expect(page.locator('section[aria-busy="true"]')).to_have_count(0)
    expect(page.locator(ALERTS)).to_have_count(0)


@given("a buyer opens the search results for a keyword")
def buyer_search(world: World) -> None:
    d2.seed_buyer(world)
    keyword = f"uifsave{uuid.uuid4().hex[:8]}"
    _x(world)["uif_keyword"] = keyword
    world.page.goto(f"{_base(world)}/search?q={keyword}", wait_until="domcontentloaded")
    page = SavedSearchesPage(world.page)
    page.wait_until_interactive(page.save_button)


@when("the buyer activates the save search button")
def activate_save(world: World) -> None:
    SavedSearchesPage(world.page).save_button.click()


@then("an error toast is shown, the save button is enabled again and nothing was saved")
def save_failed(world: World) -> None:
    page = world.page
    toast = page.locator(ALERTS).filter(has_text="")
    expect(toast.first).to_be_visible(timeout=timeouts.NAVIGATION)
    saved = SavedSearchesPage(page)
    expect(saved.save_button).not_to_have_attribute("aria-busy", "true", timeout=timeouts.DEFAULT)
    expect(saved.save_button).to_be_enabled()
    expect(saved.saved_item(_x(world)["uif_keyword"])).to_have_count(0)
    expect(saved.empty_state).to_be_visible()
