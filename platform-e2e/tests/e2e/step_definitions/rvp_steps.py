"""Steps for the reviews-pagination change (ListReviews paging + PDP `?rpage=`).

One listing with 105 reviews is seeded through the gateway once per session and
shared by the scenarios (they only read). Review comments are `rvp-001`..`rvp-105`
in creation order, so "the 5 oldest" are rvp-001..rvp-005.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, ListingService
from src.api.services.base_service import BaseService, GatewayError
from src.api.services.engagement_service import EngagementService
from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import ListingDetailPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()
_RPC = "/platform.engagement.v1.EngagementService/ListReviews"
_TOTAL = 105
_seeded: dict[str, str] = {}


def _listing_with_reviews() -> str:
    if "listing_id" in _seeded:
        return _seeded["listing_id"]
    seller = AuthService().register(
        fake.unique_username("rvp_seller"), SETTINGS.seed_password, "seller"
    )
    svc = ListingService(token=seller)
    listing_id = svc.create_listing(
        Listing(
            title=f"[E2E][RVP] {fake.price_vnd():d}",
            category_id="cat-electronics",
            price=100_000,
            stock=10,
            status="published",
            description="Seed for reviews pagination.",
        )
    )
    author = AuthService().register(
        fake.unique_username("rvp_buyer"), SETTINGS.seed_password, "buyer"
    )
    reviews = EngagementService(token=author)
    for i in range(1, _TOTAL + 1):
        reviews.add_review(listing_id, 1 + i % 5, f"rvp-{i:03d}")
    _seeded["listing_id"] = listing_id
    return listing_id


def _list(page: dict) -> dict:
    return BaseService().post(_RPC, {"listingId": _listing_with_reviews(), "page": page})


def _detail(world: World) -> ListingDetailPage:
    return world.get_page(PageName.LISTING_DETAIL)  # type: ignore[return-value]


# ── API scenarios ────────────────────────────────────────────────────────
@given(parsers.parse("a listing with {count:d} reviews"))
def api_listing(world: World, count: int) -> None:
    assert count == _TOTAL
    world.state.extra["rvp_listing_id"] = _listing_with_reviews()


@when("a client requests pages of 10 following next_cursor from the start")
def walk_pages(world: World) -> None:
    pages, cursor = [], ""
    for _ in range(30):
        res = _list({"cursor": cursor, "pageSize": 10})
        pages.append(res)
        cursor = res.get("page", {}).get("nextCursor", "")
        if not cursor:
            break
    world.state.extra["rvp_pages"] = pages


@then(
    "every review appears exactly once newest first and the 11th page holds the 5 oldest "
    "with an empty next_cursor"
)
def check_walk(world: World) -> None:
    pages = world.state.extra["rvp_pages"]
    assert len(pages) == 11, len(pages)
    assert all(int(p["page"]["total"]) == _TOTAL for p in pages)
    comments = [r["comment"] for p in pages for r in p.get("reviews", [])]
    assert comments == [f"rvp-{i:03d}" for i in range(_TOTAL, 0, -1)]
    assert len(pages[-1]["reviews"]) == 5
    assert not pages[-1].get("page", {}).get("nextCursor")


@when(parsers.parse("a client requests page_size {size:d} for that listing"))
def big_page(world: World, size: int) -> None:
    world.state.extra["rvp_big"] = _list({"pageSize": size})


@then("at most 100 reviews are returned and next_cursor is set")
def check_big(world: World) -> None:
    res = world.state.extra["rvp_big"]
    assert len(res["reviews"]) == 100, len(res["reviews"])
    assert res["page"]["nextCursor"] == "100"


@when(parsers.parse('a client sends the cursors "{a}" and "{b}"'))
def bad_cursors(world: World, a: str, b: str) -> None:
    codes = []
    for cursor in (a, b):
        try:
            _list({"cursor": cursor, "pageSize": 10})
            codes.append(200)
        except GatewayError as err:
            codes.append(err.status)
    world.state.extra["rvp_codes"] = codes


@then("each call fails with InvalidArgument")
def check_bad(world: World) -> None:
    # Connect maps InvalidArgument to HTTP 400.
    assert world.state.extra["rvp_codes"] == [400, 400], world.state.extra["rvp_codes"]


# ── UI scenarios ─────────────────────────────────────────────────────────
@given(parsers.parse("a listing with {count:d} reviews and a buyer signed in"))
def ui_listing(world: World, count: int) -> None:
    assert count == _TOTAL
    world.state.extra["rvp_listing_id"] = _listing_with_reviews()
    login_via_api(world, User(fake.unique_username("rvp_view"), SETTINGS.seed_password, "buyer"))


def _goto(world: World, query: str) -> None:
    base = SETTINGS.base_url.rstrip("/")
    listing_id = world.state.extra["rvp_listing_id"]
    world.page.goto(f"{base}/listing/{listing_id}{query}", wait_until="domcontentloaded")
    expect(_detail(world).review_items.first).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the buyer opens the listing page")
def open_listing(world: World) -> None:
    _goto(world, "")


@when(parsers.parse("the buyer opens the reviews page {page:d} of the listing"))
def open_page(world: World, page: int) -> None:
    _goto(world, f"?rpage={page}")


@then("the reviews list shows 10 reviews and the pagination offers 11 pages")
def check_eleven_pages(world: World) -> None:
    expect(_detail(world).review_items).to_have_count(10, timeout=timeouts.NAVIGATION)
    nav = world.page.locator("#reviews").get_by_role("navigation", name="Phân trang")
    expect(nav.get_by_role("link", name="Trang 11")).to_be_visible()
    expect(nav.get_by_role("link", name="Trang 12")).to_have_count(0)


@then("5 reviews are listed, they are the 5 oldest and page 11 is current")
def check_last_page(world: World) -> None:
    expect(_detail(world).review_items).to_have_count(5, timeout=timeouts.NAVIGATION)
    text = " ".join(_detail(world).review_items.all_inner_texts())
    assert sorted(re.findall(r"rvp-(\d{3})", text)) == [f"{i:03d}" for i in range(1, 6)], text
    current = world.page.locator("#reviews").get_by_role("link", name="Trang 11")
    expect(current).to_have_attribute("aria-current", "page")
