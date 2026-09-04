"""Price-drop / back-in-stock alert step definitions (F3 alerts).

Two shapes:
  * subscribe / list / unsubscribe driven through the real UI (toggle on the
    listing page, subscription row on the notifications center);
  * end-to-end alert *delivery* — subscribe, the seller changes price/stock via
    the gateway (team-domain emits a listing event), then the notification should
    appear after async propagation (bounded poll).

Alert subscriptions and notifications are keyed to team-notification's demo user
(handler placeholder `khach_hang_shopee`, ADR-0003 principal wiring pending), so
assertions are scoped to the freshly-seeded listing id to stay independent of
other scenarios sharing that demo user.
"""

from __future__ import annotations

import time

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import NotificationsPage
from src.pages.listing_detail_page import ListingDetailPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api, seed_listing
from tests.e2e.support.world import World

SETTINGS = get_settings()

_ALERT_ENUM = {
    "price_drop": "ALERT_TYPE_PRICE_DROP",
    "back_in_stock": "ALERT_TYPE_BACK_IN_STOCK",
}
_NOTIF_ENUM = {
    "price_drop": "NOTIFICATION_TYPE_PRICE_DROP",
    "back_in_stock": "NOTIFICATION_TYPE_BACK_IN_STOCK",
}
_DELIVERY_POLL_SECONDS = 75


# ── Gateway helpers ──────────────────────────────────────────────────────
def _api(world: World, token: str | None = None) -> BaseService:
    svc = BaseService(token=token)
    world.state.extra.setdefault("_alert_clients", []).append(svc)
    return svc


def _seed_seller_and_listing(
    world: World, price: int = 1_000_000, stock: int = 10
) -> tuple[User, Listing]:
    sf = world.service_factory
    seller_name = fake.unique_username("alert_seller")
    seller_token = sf.auth.register(seller_name, SETTINGS.seed_password, "seller")
    seller = User(
        username=seller_name, password=SETTINGS.seed_password, role="seller", token=seller_token
    )
    listing = Listing(
        title=f"[E2E][Alert] {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=price,
        stock=stock,
        status="published",
        description="Sản phẩm seed tự động cho Alert E2E.",
    )
    seed_listing(world, listing, seller)
    world.state.seeded_seller = seller
    return seller, listing


def _login_fresh_buyer(world: World) -> User:
    name = fake.unique_username("alert_buyer")
    buyer = User(username=name, password=SETTINGS.seed_password, role="buyer")
    login_via_api(world, buyer)
    world.state.current_user = buyer
    return buyer


def _subscribe_alert(world: World, token: str, listing_id: str, kind: str) -> str:
    resp = _api(world, token).post(
        "/platform.notification.v1.NotificationService/SubscribeAlert",
        {"listingId": listing_id, "type": _ALERT_ENUM[kind]},
    )
    return (resp.get("subscription") or {}).get("id", "")


def _update_listing(world: World, seller_token: str, listing_id: str, **changes) -> dict:
    """Full-replacement UpdateListing: fetch the current listing, apply changes."""
    svc = _api(world, seller_token)
    cur = (
        svc.post("/platform.listing.v1.ListingService/GetListing", {"id": listing_id}).get(
            "listing"
        )
        or {}
    )
    cur.update(changes)
    return svc.post("/platform.listing.v1.ListingService/UpdateListing", {"listing": cur})


def _poll_for_notification(world: World, kind: str, listing_id: str, seconds: int) -> bool:
    """Bounded poll of ListNotifications for a notification of `kind` linking to the
    listing. Returns True as soon as one appears."""
    want_type = _NOTIF_ENUM[kind]
    link = f"/listing/{listing_id}"
    svc = _api(world, world.state.current_user.token)
    deadline = time.time() + seconds
    while time.time() < deadline:
        resp = svc.post(
            "/platform.notification.v1.NotificationService/ListNotifications",
            {"pageSize": 50},
        )
        for n in resp.get("notifications", []):
            if str(n.get("type")) == want_type and n.get("linkUrl") == link:
                return True
        time.sleep(3)
    return False


# ── Subscribe / list / unsubscribe (UI) ──────────────────────────────────
@given("a buyer viewing a seeded listing they want alerts for")
def buyer_viewing_listing_for_alerts(world: World) -> None:
    _seller, listing = _seed_seller_and_listing(world)
    _login_fresh_buyer(world)
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing.listing_id)


@when(parsers.parse('the buyer enables the "{kind}" alert on the listing'))
def enable_alert_toggle(world: World, kind: str) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    toggle = detail.alert_toggle(kind)
    expect(toggle).to_be_visible(timeout=timeouts.NAVIGATION)
    if toggle.get_attribute("data-active") != "true":
        toggle.click()


@then(parsers.parse('the "{kind}" alert toggle is active'))
def alert_toggle_active(world: World, kind: str) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.alert_toggle(kind)).to_have_attribute(
        "data-active", "true", timeout=timeouts.DEFAULT
    )


def _subscription_row(world: World, kind: str, listing_id: str):
    return world.page.locator(f'[data-testid="alert-subscription"][data-type="{kind}"]').filter(
        has=world.page.locator(f'a[href="/listing/{listing_id}"]')
    )


@then(parsers.parse('the notifications center lists a "{kind}" alert subscription'))
def notifications_center_lists_subscription(world: World, kind: str) -> None:
    world.navigate_to(PageName.NOTIFICATIONS)
    listing_id = world.state.listing.listing_id
    expect(_subscription_row(world, kind, listing_id).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    world.logger.info(f"{kind} subscription row visible for listing {listing_id}")


@when(parsers.parse('the buyer cancels the "{kind}" alert subscription'))
def cancel_alert_subscription(world: World, kind: str) -> None:
    listing_id = world.state.listing.listing_id
    row = _subscription_row(world, kind, listing_id).first
    expect(row).to_be_visible(timeout=timeouts.NAVIGATION)
    row.get_by_role("button", name="Hủy").click()


@then(
    parsers.parse('the notifications center lists no "{kind}" alert subscription for that listing')
)
def notifications_center_no_subscription(world: World, kind: str) -> None:
    listing_id = world.state.listing.listing_id
    expect(_subscription_row(world, kind, listing_id)).to_have_count(0, timeout=timeouts.DEFAULT)
    world.logger.info(f"{kind} subscription removed for listing {listing_id}")


# ── Alert delivery (async event flow) ────────────────────────────────────
@given(parsers.parse('a buyer subscribed to a "{kind}" alert on a seeded listing'))
def buyer_subscribed_for_delivery(world: World, kind: str) -> None:
    # Back-in-stock only fires on a 0 -> positive transition the consumer has a
    # prior 0 baseline for; seed such a listing with positive stock, subscribe,
    # then the When step drops stock to 0 before restocking.
    _seller, listing = _seed_seller_and_listing(world, price=2_000_000, stock=5)
    buyer = _login_fresh_buyer(world)
    _subscribe_alert(world, buyer.token, listing.listing_id, kind)
    world.state.extra["alert_kind"] = kind


@when("the seller lowers the listing price")
def seller_lowers_price(world: World) -> None:
    seller = world.state.seeded_seller
    listing_id = world.state.listing.listing_id
    _update_listing(world, seller.token, listing_id, price=500_000)
    world.logger.info(f"Seller lowered price for {listing_id} to 500000")


@when("the seller restocks the out-of-stock listing")
def seller_restocks(world: World) -> None:
    seller = world.state.seeded_seller
    listing_id = world.state.listing.listing_id
    # Establish the 0 baseline the consumer needs, then restock.
    _update_listing(world, seller.token, listing_id, stock=0)
    time.sleep(2)
    _update_listing(world, seller.token, listing_id, stock=8)
    world.logger.info(f"Seller restocked {listing_id} (0 -> 8)")


@then(parsers.parse('a "{kind}" notification appears in the notifications center'))
def notification_appears(world: World, kind: str) -> None:
    listing_id = world.state.listing.listing_id
    found = _poll_for_notification(world, kind, listing_id, _DELIVERY_POLL_SECONDS)
    assert found, (
        f"no {kind} notification for listing {listing_id} within "
        f"{_DELIVERY_POLL_SECONDS}s — team-domain emits platform.listing.v1.ListingChanged "
        f"but team-notification's consumer only reacts to ListingPricingChanged/"
        f"ListingStockChanged, so the alert is never created"
    )
    world.navigate_to(PageName.NOTIFICATIONS)
    page: NotificationsPage = world.get_page(PageName.NOTIFICATIONS)  # type: ignore[assignment]
    expect(page.notifications_of_type(kind).first).to_be_visible(timeout=timeouts.NAVIGATION)
