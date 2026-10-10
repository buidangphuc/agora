"""Flash-sale campaign ownership (cross-seller regression).

Drives FlashSaleService/CreateCampaign through the gateway Connect API and asserts
the HTTP status team-promotion's ownership check produced (200 owner, 403 another
seller, 400 unknown listing). Self-contained: sellers and the listing are seeded
through the gateway.
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import BaseService
from src.models import Listing
from src.utils import data as fake
from tests.e2e.support.world import World

SETTINGS = get_settings()
_CREATE = "/platform.promotion.v1.FlashSaleService/CreateCampaign"


def _register_seller(world: World, prefix: str) -> str:
    token = world.service_factory.auth.register(
        fake.unique_username(prefix), SETTINGS.seed_password, "seller"
    )
    world.service_factory.set_token(token)
    return token


def _campaign_response(token: str, listing_id: str) -> httpx.Response:
    svc = BaseService(token=token)
    try:
        return svc.send(
            "POST",
            _CREATE,
            json_body={
                "listingId": listing_id,
                "salePrice": 499_000,
                "stockCap": 10,
                "startsAt": "2026-09-01T00:00:00Z",
                "endsAt": "2026-12-31T23:59:59Z",
            },
        )
    finally:
        svc.close()


@given("a flash-sale seller with a published listing")
def flash_seller_with_listing(world: World) -> None:
    token = _register_seller(world, "flash_owner")
    listing = Listing(
        title=f"[E2E][FlashOwn] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=5_000_000,
        stock=100,
        status="published",
        description="Sản phẩm seed tự động cho Flash-sale ownership E2E.",
    )
    world.service_factory.listing.create_listing(listing)
    world.state.extra["flash_owner_token"] = token
    world.state.extra["flash_listing_id"] = listing.listing_id


@given("a second flash-sale seller")
def second_flash_seller(world: World) -> None:
    world.state.extra["flash_other_token"] = _register_seller(world, "flash_other")


@when("that seller creates a flash-sale campaign for the listing")
def owner_creates_campaign(world: World) -> None:
    world.state.extra["campaign_resp"] = _campaign_response(
        world.state.extra["flash_owner_token"], world.state.extra["flash_listing_id"]
    )


@when("the second seller creates a flash-sale campaign for the first seller's listing")
def other_creates_campaign_on_foreign_listing(world: World) -> None:
    world.state.extra["campaign_resp"] = _campaign_response(
        world.state.extra["flash_other_token"], world.state.extra["flash_listing_id"]
    )


@when("the second seller creates a flash-sale campaign for an unknown listing")
def other_creates_campaign_on_unknown_listing(world: World) -> None:
    world.state.extra["campaign_resp"] = _campaign_response(
        world.state.extra["flash_other_token"], f"no-such-listing-{uuid.uuid4().hex[:8]}"
    )


@then(parsers.parse("the campaign call returns {status:d}"))
def campaign_call_returns(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["campaign_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
