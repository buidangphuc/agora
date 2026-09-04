"""Step definitions for promotion e2e: voucher redemption + flash-sale storefront.

Self-contained: every precondition (seller, listing, buyer, address, cart, and the
flash-sale campaign) is seeded through the gateway API by the same principal that the
browser is then logged in as, so the checkout server component renders the seeded
cart/address. Mirrors the seeding + api-login pattern used by buyer/order steps.
"""

from __future__ import annotations

import re

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.constants import PageName, timeouts
from src.models import Listing, User
from src.pages import CheckoutPage, ListingDetailPage, VouchersPage
from src.utils import data as fake
from tests.e2e.flows import login_via_api
from tests.e2e.support.world import World

SETTINGS = get_settings()

# Platform voucher already present in the promotion DB: 10% off, no min-spend.
_SAVE10_CODE = "SAVE10"
_SAVE10_PERCENT = 10
_QUALIFYING_SUBTOTAL = 1_000_000  # >= 500k -> free shipping, so total = subtotal - discount
_FLASH_SALE_PRICE = 499_000
_FLASH_STOCK_CAP = 50


def _vnd(text: str) -> int:
    """Parse the VND amount from a summary line, e.g. 'Mã giảm giá (SAVE10):\\n- 100.000 VND'
    -> 100000. Reads the number immediately before 'VND' so labels like '(SAVE10)'
    never leak into the amount; falls back to all digits when no 'VND' suffix exists."""
    m = re.search(r"([\d.]+)\s*VND", text or "")
    cleaned = re.sub(r"[^\d]", "", m.group(1) if m else (text or ""))
    return int(cleaned) if cleaned else -1


def _promotion_api(world: World) -> BaseService:
    """A public gateway client for team-promotion RPCs (no wrapper service exists yet)."""
    svc = world.state.extra.get("_promo_api")
    if svc is None:
        svc = BaseService(token=None)
        world.state.extra["_promo_api"] = svc
    return svc


def _seed_seller_listing(world: World, price: int) -> Listing:
    sf = world.service_factory
    seller_name = fake.unique_username("promo_seller")
    seller_token = sf.auth.register(seller_name, SETTINGS.seed_password, "seller")
    sf.set_token(seller_token)
    listing = Listing(
        title=f"[E2E][Promo] {fake.price_vnd():d}",
        category_id="cat-electronics",
        price=price,
        stock=100,
        status="published",
        description="Sản phẩm seed tự động cho Promotion E2E.",
    )
    sf.listing.create_listing(listing)
    world.state.seeded_seller = User(
        username=seller_name, password=SETTINGS.seed_password, role="seller", token=seller_token
    )
    world.state.listing = listing
    world.logger.info(f"Seeded promo listing {listing.listing_id} @ {price}")
    return listing


# ── Voucher redemption at checkout ───────────────────────────────────────
@given("a promotion buyer has a qualifying cart and a saved address")
def promo_buyer_qualifying_cart(world: World) -> None:
    listing = _seed_seller_listing(world, _QUALIFYING_SUBTOTAL)

    sf = world.service_factory
    buyer_name = fake.unique_username("promo_buyer")
    buyer_token = sf.auth.register(buyer_name, SETTINGS.seed_password, "buyer")
    sf.set_token(buyer_token)
    sf.address.create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city="Ha Noi",
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=True,
    )
    sf.cart.add_to_cart(listing_id=listing.listing_id, quantity=1)

    buyer = User(
        username=buyer_name, password=SETTINGS.seed_password, role="buyer", token=buyer_token
    )
    login_via_api(world, buyer)  # injects the session cookie for the browser
    world.state.extra["subtotal"] = _QUALIFYING_SUBTOTAL
    world.logger.info(f"Promo buyer {buyer_name} has cart subtotal {_QUALIFYING_SUBTOTAL}")


@when(parsers.parse('the buyer applies the voucher code "{code}" at checkout'))
def buyer_applies_voucher(world: World, code: str) -> None:
    world.navigate_to(PageName.CHECKOUT)
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.voucher_input).to_be_visible(timeout=timeouts.NAVIGATION)
    world.state.extra["applied_code"] = code
    checkout.apply_voucher(code)


@then("the voucher discount is shown and the order total is reduced")
def voucher_discount_shown(world: World) -> None:
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    subtotal = world.state.extra["subtotal"]
    expected_discount = subtotal * _SAVE10_PERCENT // 100

    expect(checkout.voucher_discount).to_be_visible(timeout=timeouts.DEFAULT)
    assert (
        _vnd(checkout.voucher_discount.inner_text()) == expected_discount
    ), f"discount line != {expected_discount}: {checkout.voucher_discount.inner_text()!r}"

    total = _vnd(checkout.order_total.inner_text())
    # Subtotal >= 500k => free shipping, so total = subtotal - discount.
    assert (
        total == subtotal - expected_discount
    ), f"order total {total} != {subtotal - expected_discount}"
    assert total < subtotal, "order total was not reduced by the voucher"
    world.logger.info(f"Discount {expected_discount} applied; total {total} (< {subtotal})")


@then("placing the order creates the order")
def placing_order_creates_order(world: World) -> None:
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.place_order_button).to_be_enabled(timeout=timeouts.DEFAULT)
    checkout.place_order_button.click()
    # COD success routes to /account/orders?success=1; a mock-pay method routes to /checkout/pay.
    world.page.wait_for_url(
        re.compile(r".*/(account/orders|checkout/pay).*"), timeout=timeouts.NAVIGATION
    )
    assert re.search(
        r"/(account/orders|checkout/pay)", world.page.url
    ), f"order was not created; landed on {world.page.url}"
    world.logger.info(f"Order created; navigated to {world.page.url}")


@then("a voucher error reason is shown and no discount is applied")
def voucher_error_shown(world: World) -> None:
    checkout: CheckoutPage = world.get_page(PageName.CHECKOUT)  # type: ignore[assignment]
    expect(checkout.voucher_error).to_be_visible(timeout=timeouts.DEFAULT)
    reason = checkout.voucher_error.inner_text().strip()
    assert reason, "voucher error alert was empty"
    # No discount line is rendered when the code is invalid.
    expect(checkout.voucher_discount).to_have_count(0)
    subtotal = world.state.extra["subtotal"]
    assert (
        _vnd(checkout.order_total.inner_text()) == subtotal
    ), "order total changed despite an invalid voucher"
    world.logger.info(f"Invalid voucher rejected with reason: {reason!r}")


# ── Flash-sale storefront ────────────────────────────────────────────────
@given("a listing with an active flash-sale campaign")
def listing_with_flash_sale(world: World) -> None:
    listing = _seed_seller_listing(world, 5_000_000)
    resp = _promotion_api(world).post(
        "/platform.promotion.v1.FlashSaleService/CreateCampaign",
        {
            "listingId": listing.listing_id,
            "salePrice": _FLASH_SALE_PRICE,
            "stockCap": _FLASH_STOCK_CAP,
            "startsAt": "2026-09-01T00:00:00Z",
            "endsAt": "2026-12-31T23:59:59Z",
        },
    )
    campaign_id = (resp.get("campaign") or {}).get("id", "")
    assert campaign_id, f"CreateCampaign returned no id: {resp}"
    world.state.extra["campaign_id"] = campaign_id
    world.logger.info(f"Active flash-sale campaign {campaign_id} for listing {listing.listing_id}")


@when("a visitor views that listing")
def visitor_views_listing(world: World) -> None:
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=world.state.listing.listing_id)


@then("the flash-sale meter shows the sale price and remaining stock")
def flash_sale_meter_shown(world: World) -> None:
    detail: ListingDetailPage = world.get_page(PageName.LISTING_DETAIL)  # type: ignore[assignment]
    expect(detail.flash_sale_meter).to_be_visible(timeout=timeouts.NAVIGATION)
    assert (
        detail.flash_sale_stock_cap() == _FLASH_STOCK_CAP
    ), f"stock cap {detail.flash_sale_stock_cap()} != {_FLASH_STOCK_CAP}"
    remaining = detail.flash_sale_remaining()
    assert 0 <= remaining <= _FLASH_STOCK_CAP, f"remaining {remaining} out of range"
    # Sale price rendered in the banner (formatPrice -> '₫499.000').
    expect(detail.flash_sale_banner).to_contain_text("499.000", timeout=timeouts.DEFAULT)
    world.logger.info(f"Flash-sale meter: remaining={remaining}/{_FLASH_STOCK_CAP}")


# ── Vouchers hub (existing coverage, kept) ───────────────────────────────
@then("the vouchers hub lists available vouchers")
def vouchers_hub_lists(world: World) -> None:
    page: VouchersPage = world.get_page(PageName.VOUCHERS)  # type: ignore[assignment]
    assert page.is_displayed()
    expect(world.page.get_by_role("button", name="Tất cả voucher", exact=False)).to_be_visible(
        timeout=timeouts.DEFAULT
    )
    assert page.save_voucher_buttons.count() >= 1, "no savable vouchers on the hub"
