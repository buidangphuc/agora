"""Review & rating step definitions: existing breakdown filter + richer reviews (F4).

Richer reviews (media, helpful votes, verified-purchase, shop rating rollup) are
seeded through the gateway by the same principal the browser is logged in as, then
asserted in the real listing UI — mirroring the promo e2e seed-via-API/verify-via-UI
pattern.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import ListingDetailPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api, seed_listing
from tests.e2e.support.world import World

SETTINGS = get_settings()

_REVIEW_PHOTO = "https://picsum.photos/seed/e2e-review/200/200"


# ── existing coverage (kept) ─────────────────────────────────────────────
@then("the listing page displays the reviews breakdown section and rating filter buttons")
def reviews_breakdown_visible(world: World) -> None:
    expect(world.page.locator("body")).to_contain_text(
        "ĐÁNH GIÁ SẢN PHẨM", timeout=timeouts.DEFAULT
    )
    expect(world.page.locator("body")).to_contain_text("/ 5", timeout=timeouts.DEFAULT)
    expect(world.page.get_by_role("button", name="Tất Cả").first).to_be_visible(
        timeout=timeouts.DEFAULT
    )


# ── Gateway helpers ──────────────────────────────────────────────────────
def _api(world: World, token: str | None) -> BaseService:
    svc = BaseService(token=token)
    world.state.extra.setdefault("_review_clients", []).append(svc)
    return svc


def _helpful_count(text: str) -> int:
    """Parse the helpful button label '👍 Hữu ích (3)' -> 3."""
    m = re.search(r"\((\d+)\)", text or "")
    return int(m.group(1)) if m else -1


def _seed_seller_and_listing(world: World, prefix: str) -> tuple[User, Listing]:
    sf = world.service_factory
    seller_name = fake.unique_username(f"{prefix}_seller")
    seller_token = sf.auth.register(seller_name, SETTINGS.seed_password, "seller")
    seller = User(
        username=seller_name, password=SETTINGS.seed_password, role="seller", token=seller_token
    )
    listing = Listing(
        title=f"[E2E][Review] {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=2_000_000,
        stock=50,
        status="published",
        description="Sản phẩm seed tự động cho Richer Reviews E2E.",
    )
    seed_listing(world, listing, seller)
    return seller, listing


def _register_buyer(world: World, prefix: str) -> User:
    sf = world.service_factory
    name = fake.unique_username(f"{prefix}_buyer")
    token = sf.auth.register(name, SETTINGS.seed_password, "buyer")
    return User(username=name, password=SETTINGS.seed_password, role="buyer", token=token)


def _create_review(
    world: World,
    token: str,
    listing_id: str,
    rating: int,
    comment: str,
    media_urls: list[str] | None = None,
    order_id: str = "",
) -> dict:
    resp = _api(world, token).post(
        "/platform.engagement.v1.EngagementService/CreateReview",
        {
            "listingId": listing_id,
            "rating": rating,
            "comment": comment,
            "orderId": order_id,
            "mediaUrls": media_urls or [],
        },
    )
    return resp.get("review", {})


def _complete_order_for(world: World, buyer: User, listing_id: str) -> str:
    """Create a COD order for the buyer and drive it to COMPLETED so the review
    verifies as a real purchase (team-order.GetOrder must report COMPLETED)."""
    sf = world.service_factory
    sf.set_token(buyer.token)
    try:
        sf.address.create_address(
            recipient_name="Nguyen Van A",
            phone="0912345678",
            street="29 Lieu Giai",
            city="Ha Noi",
            ward="Phuong Lieu Giai",
            district="Quan Ba Dinh",
            is_default=True,
        )
    except Exception:  # noqa: BLE001 - address may already exist
        pass
    try:
        sf.cart.clear_cart()
    except Exception:  # noqa: BLE001
        pass
    sf.cart.add_to_cart(listing_id=listing_id, quantity=1)
    res = sf.order.create_order({"paymentMethod": "PAYMENT_METHOD_COD"})
    orders = res.get("orders", [])
    order_id = orders[0].get("id") if orders else res.get("order", {}).get("id", "")
    assert order_id, f"order not created: {res}"
    # Buyer owns the order, so the buyer principal may drive its status to COMPLETED.
    _api(world, buyer.token).post(
        "/platform.order.v1.OrderService/UpdateOrderStatus",
        {"id": order_id, "status": "ORDER_STATUS_COMPLETED"},
    )
    return order_id


# ── Post a review with a photo ───────────────────────────────────────────
@given("a buyer has posted a review with a photo on a seeded listing")
def buyer_posted_review_with_photo(world: World) -> None:
    _seller, listing = _seed_seller_and_listing(world, "photo")
    author = _register_buyer(world, "photo")
    _create_review(
        world,
        author.token,
        listing.listing_id,
        rating=5,
        comment="Sản phẩm rất tốt, đóng gói cẩn thận!",
        media_urls=[_REVIEW_PHOTO],
    )
    # View as any logged-in buyer.
    viewer = _register_buyer(world, "photo_view")
    login_via_api(world, viewer)
    world.state.current_user = viewer


@when("the buyer opens the reviewed listing")
def open_reviewed_listing(world: World) -> None:
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=world.state.listing.listing_id)


@then("the review and its photo are shown")
def review_and_photo_shown(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.review_items.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(detail.review_photos.first).to_be_visible(timeout=timeouts.DEFAULT)
    world.logger.info("Review with photo rendered on the listing")


# ── Helpful vote (increments once per user) ──────────────────────────────
@given("a buyer is viewing a listing with another user's review")
def buyer_viewing_others_review(world: World) -> None:
    _seller, listing = _seed_seller_and_listing(world, "helpful")
    author = _register_buyer(world, "helpful_author")
    _create_review(
        world,
        author.token,
        listing.listing_id,
        rating=4,
        comment="Ổn trong tầm giá.",
    )
    viewer = _register_buyer(world, "helpful_viewer")
    login_via_api(world, viewer)
    world.state.current_user = viewer
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing.listing_id)


@when("the buyer marks the review as helpful")
def mark_review_helpful(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    btn = detail.review_helpful_buttons.first
    expect(btn).to_be_visible(timeout=timeouts.NAVIGATION)
    world.state.extra["helpful_before"] = _helpful_count(btn.inner_text())
    btn.click()


@then("the helpful count increases by one and cannot be voted again")
def helpful_count_increments_once(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    btn = detail.review_helpful_buttons.first
    before = world.state.extra["helpful_before"]
    expect(btn).to_contain_text(f"({before + 1})", timeout=timeouts.DEFAULT)
    # A second vote by the same user is blocked (the button disables after voting).
    expect(btn).to_be_disabled(timeout=timeouts.DEFAULT)
    world.logger.info(f"Helpful count {before} -> {before + 1}; re-vote blocked")


# ── Verified-purchase badge for a delivered order ────────────────────────
@given("a buyer has reviewed a listing they completed an order for")
def buyer_reviewed_completed_order(world: World) -> None:
    _seller, listing = _seed_seller_and_listing(world, "verified")
    buyer = _register_buyer(world, "verified")
    order_id = _complete_order_for(world, buyer, listing.listing_id)
    _create_review(
        world,
        buyer.token,
        listing.listing_id,
        rating=5,
        comment="Đã mua và dùng thử, rất hài lòng!",
        order_id=order_id,
    )
    login_via_api(world, buyer)
    world.state.current_user = buyer


@then("the review shows a verified-purchase badge")
def verified_badge_shown(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.review_items.first).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(detail.verified_purchase_badges.first).to_be_visible(timeout=timeouts.DEFAULT)
    world.logger.info("Verified-purchase badge rendered")


# ── Shop rating summary ──────────────────────────────────────────────────
@then("the shop rating summary is shown on the listing")
def shop_rating_summary_shown(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.shop_rating_summary).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(detail.shop_rating_summary).to_contain_text("/ 5.0", timeout=timeouts.DEFAULT)
    world.logger.info("Shop rating summary rendered")
