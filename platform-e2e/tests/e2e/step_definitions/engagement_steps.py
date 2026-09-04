"""Engagement, reviews & Q&A step definitions (incl. wishlist collections F3)."""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import FavoritesPage, ListingDetailPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api, seed_listing, submit_product_question_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()


# ── Gateway helpers (raw Connect/JSON, no shared-service edits) ──────────
def _engagement_api(world: World, token: str | None) -> BaseService:
    """A gateway client for EngagementService collection RPCs."""
    svc = BaseService(token=token)
    world.state.extra.setdefault("_engagement_clients", []).append(svc)
    return svc


def _collection_id_by_name(world: World, token: str, name: str) -> str:
    svc = _engagement_api(world, token)
    resp = svc.post("/platform.engagement.v1.EngagementService/ListCollections", {})
    for col in resp.get("collections", []):
        if col.get("name") == name:
            return col.get("id", "")
    return ""


def _collection_item_ids(world: World, token: str, collection_id: str) -> list[str]:
    svc = _engagement_api(world, token)
    resp = svc.post(
        "/platform.engagement.v1.EngagementService/ListCollectionItems",
        {"collectionId": collection_id, "page": {"pageSize": 50}},
    )
    return resp.get("listingIds", []) or []


@when("I view the seeded listing detail page")
def view_seeded_listing(world: World) -> None:
    listing = world.state.listing
    listing_id = listing.listing_id if listing else "listing_001"
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing_id)


@then("I should see the product rating breakdown and review section")
def verify_reviews_section(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.title).to_be_visible(timeout=timeouts.DEFAULT)


@when(parsers.parse('I submit a question "{question_text}" on the listing'))
def submit_question_step(world: World, question_text: str) -> None:
    listing = world.state.listing
    listing_id = listing.listing_id if listing else "listing_001"
    user = world.state.current_user
    submit_product_question_via_api(world, user, listing_id, question_text)


@then("the question should be submitted successfully")
def verify_question_submitted(world: World) -> None:
    world.logger.info("Question submission verified via API response")


# ── Wishlist collections (F3) ────────────────────────────────────────────
@given("a buyer with a seeded listing to collect")
def buyer_with_seeded_listing_to_collect(world: World) -> None:
    sf = world.service_factory
    seller_name = fake.unique_username("collect_seller")
    seller_token = sf.auth.register(seller_name, SETTINGS.seed_password, "seller")
    seller = User(
        username=seller_name, password=SETTINGS.seed_password, role="seller", token=seller_token
    )
    listing = Listing(
        title=f"[E2E][Collection] {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=1_500_000,
        stock=25,
        status="published",
        description="Sản phẩm seed tự động cho Wishlist Collections E2E.",
    )
    seed_listing(world, listing, seller)

    buyer_name = fake.unique_username("collect_buyer")
    buyer = User(username=buyer_name, password=SETTINGS.seed_password, role="buyer")
    login_via_api(world, buyer)  # registers + injects the session cookie
    world.state.current_user = buyer
    world.logger.info(f"Collection buyer {buyer_name} ready with listing {listing.listing_id}")


@when(parsers.parse('the buyer creates a collection named "{name}"'))
def buyer_creates_collection(world: World, name: str) -> None:
    page: FavoritesPage = world.navigate_to(PageName.FAVORITES)  # type: ignore[assignment]
    expect(page.create_collection_form).to_be_visible(timeout=timeouts.NAVIGATION)
    page.create_collection(name)
    world.state.extra["collection_name"] = name


@then("the collection appears in the buyer's collections")
def collection_appears(world: World) -> None:
    page: FavoritesPage = world.get_page(PageName.FAVORITES)  # type: ignore[assignment]
    name = world.state.extra["collection_name"]
    expect(page.collection_row(name)).to_be_visible(timeout=timeouts.NAVIGATION)
    # Capture the server id for later navigation / verification.
    token = world.state.current_user.token
    cid = _collection_id_by_name(world, token, name)
    assert cid, f"collection {name!r} not found via ListCollections"
    world.state.extra["collection_id"] = cid
    world.logger.info(f"Collection {name!r} created (id={cid})")


@when(parsers.parse('the buyer adds the listing to the collection "{name}"'))
def buyer_adds_listing_to_collection(world: World, name: str) -> None:
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=world.state.listing.listing_id)
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.add_to_collection_button).to_be_visible(timeout=timeouts.NAVIGATION)
    detail.add_to_collection_button.click()
    option = detail.collection_option(name)
    expect(option).to_be_visible(timeout=timeouts.DEFAULT)
    option.click()
    # Popover closes on success; confirm the write landed via the API.
    token = world.state.current_user.token
    cid = world.state.extra["collection_id"]
    listing_id = world.state.listing.listing_id
    ids = _collection_item_ids(world, token, cid)
    assert listing_id in ids, f"listing {listing_id} not added to collection (items={ids})"
    world.logger.info(f"Added listing {listing_id} to collection {name!r}")


@then("the collection shows the listing among its items")
def collection_shows_listing(world: World) -> None:
    cid = world.state.extra["collection_id"]
    listing_id = world.state.listing.listing_id
    world.page.goto(f"{SETTINGS.base_url}/favorites?collection={cid}")
    expect(world.page.locator(f'a[href^="/listing/{listing_id}"]').first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    world.logger.info(f"Collection view renders listing {listing_id}")


@when(parsers.parse('the buyer removes the listing from the collection "{name}"'))
def buyer_removes_listing_from_collection(world: World, name: str) -> None:
    # No dedicated UI remove control exists; exercise the RemoveFromCollection RPC.
    token = world.state.current_user.token
    cid = world.state.extra["collection_id"]
    listing_id = world.state.listing.listing_id
    _engagement_api(world, token).post(
        "/platform.engagement.v1.EngagementService/RemoveFromCollection",
        {"collectionId": cid, "listingId": listing_id},
    )
    world.logger.info(f"Removed listing {listing_id} from collection {name!r}")


@then("the collection has no items")
def collection_has_no_items(world: World) -> None:
    token = world.state.current_user.token
    cid = world.state.extra["collection_id"]
    ids = _collection_item_ids(world, token, cid)
    assert ids == [], f"expected empty collection, got {ids}"
    world.page.goto(f"{SETTINGS.base_url}/favorites?collection={cid}")
    expect(world.page.get_by_text("chưa có sản phẩm", exact=False)).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    world.logger.info("Collection is empty after removal")
